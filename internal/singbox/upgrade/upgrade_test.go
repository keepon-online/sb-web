package upgrade

import (
	"strings"
	"testing"

	"miaomiaowu/internal/systemops"
)

func TestBuildUpgradePlan_StableChannel_HasExpectedShape(t *testing.T) {
	req := UpgradeRequest{Channel: ChannelStable, Arch: "amd64"}
	plan, err := BuildUpgradePlan(req, "1.10.7", "1.13.0-alpha.1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if plan.Name != PlanNameUpgrade {
		t.Errorf("Name = %q, want %q", plan.Name, PlanNameUpgrade)
	}
	if !plan.RollbackOnFailure {
		t.Error("RollbackOnFailure must be true")
	}
	if plan.DryRun {
		t.Error("DryRun must default to false")
	}
	if len(plan.Steps) != 9 {
		t.Errorf("step count = %d, want 9", len(plan.Steps))
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan.Validate(): %v", err)
	}
}

func TestBuildUpgradePlan_AllChannelsResolveCorrectVersion(t *testing.T) {
	cases := []struct {
		name        string
		req         UpgradeRequest
		stable, pre string
		wantInURL   string
	}{
		{
			name:      "stable picks latestStable",
			req:       UpgradeRequest{Channel: ChannelStable, Arch: "amd64"},
			stable:    "1.10.7",
			pre:       "1.13.0-alpha.1",
			wantInURL: "v1.10.7/",
		},
		{
			name:      "pre picks latestPre",
			req:       UpgradeRequest{Channel: ChannelPre, Arch: "amd64"},
			stable:    "1.10.7",
			pre:       "1.13.0-alpha.1",
			wantInURL: "v1.13.0-alpha.1/",
		},
		{
			name:      "pinned uses PinnedVersion",
			req:       UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.30.0", Arch: "arm64"},
			stable:    "",
			pre:       "",
			wantInURL: "v1.30.0/",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, err := BuildUpgradePlan(c.req, c.stable, c.pre)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			download := plan.Steps[2]
			if download.ID != "download-tarball" {
				t.Fatalf("step[2].ID = %q, want download-tarball", download.ID)
			}
			url := download.Metadata["url"]
			if !strings.Contains(url, c.wantInURL) {
				t.Errorf("download URL %q missing %q", url, c.wantInURL)
			}
		})
	}
}

func TestBuildUpgradePlan_RejectsInvalidChannel(t *testing.T) {
	for _, ch := range []Channel{0, -1, 4, 99} {
		_, err := BuildUpgradePlan(UpgradeRequest{Channel: ch, Arch: "amd64"}, "1.10.7", "1.13.0-alpha.1")
		if err == nil {
			t.Errorf("channel %d should be rejected", ch)
		}
	}
}

func TestBuildUpgradePlan_StableLookupEmpty_Rejects(t *testing.T) {
	_, err := BuildUpgradePlan(UpgradeRequest{Channel: ChannelStable, Arch: "amd64"}, "", "1.13.0-alpha.1")
	if err == nil {
		t.Error("expected rejection when latestStable empty for stable channel")
	}
	if err != nil && !strings.Contains(err.Error(), "Pinned") {
		t.Errorf("error %q should hint at Pinned fallback", err.Error())
	}
}

func TestBuildUpgradePlan_PreLookupEmpty_Rejects(t *testing.T) {
	_, err := BuildUpgradePlan(UpgradeRequest{Channel: ChannelPre, Arch: "amd64"}, "1.10.7", "")
	if err == nil {
		t.Error("expected rejection when latestPre empty for pre channel")
	}
}

func TestBuildUpgradePlan_PinnedRequiresVersion(t *testing.T) {
	_, err := BuildUpgradePlan(UpgradeRequest{Channel: ChannelPinned, Arch: "amd64"}, "", "")
	if err == nil {
		t.Error("expected rejection of empty PinnedVersion")
	}
}

func TestBuildUpgradePlan_RejectsInjectionInPinnedVersion(t *testing.T) {
	rejects := []string{
		"1.10.7;rm",
		"1.10.7$(whoami)",
		"../../etc/passwd",
		"v1.10.7",
		"",
		"1.10",
	}
	for _, v := range rejects {
		req := UpgradeRequest{Channel: ChannelPinned, PinnedVersion: v, Arch: "amd64"}
		_, err := BuildUpgradePlan(req, "", "")
		if err == nil {
			t.Errorf("PinnedVersion=%q should be rejected", v)
		}
	}
}

func TestBuildUpgradePlan_RejectsInjectionInUpstreamVersion(t *testing.T) {
	// Even if upstream "fetch" returned a hostile string, ValidateVersion
	// must still gate it before it lands in the URL or sh -c body.
	_, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelStable, Arch: "amd64"},
		"1.10.7;rm -rf /", "1.13.0-alpha.1",
	)
	if err == nil {
		t.Error("expected upstream-version validation to reject hostile fetch result")
	}
}

func TestBuildUpgradePlan_ArchWhitelist(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64", "386", "armv7"} {
		_, err := BuildUpgradePlan(
			UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: arch},
			"", "",
		)
		if err != nil {
			t.Errorf("arch %q should be accepted: %v", arch, err)
		}
	}
	rejects := []string{
		"",
		"x86",
		"amd64; rm",
		"amd64$(whoami)",
		"riscv64",
		"AMD64", // case-sensitive whitelist
		"../amd64",
	}
	for _, arch := range rejects {
		_, err := BuildUpgradePlan(
			UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: arch},
			"", "",
		)
		if err == nil {
			t.Errorf("arch %q should be rejected", arch)
		}
	}
}

func TestBuildUpgradePlan_StepOrderAndIDs(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	wantIDs := []string{
		"backup-binary",
		"backup-config",
		"download-tarball",
		"extract-tarball",
		"install-binary",
		"cleanup-tarball",
		"switch-config",
		"restart-singbox-apk",
		"restart-singbox-systemd",
	}
	if len(plan.Steps) != len(wantIDs) {
		t.Fatalf("step count = %d, want %d", len(plan.Steps), len(wantIDs))
	}
	for i, s := range plan.Steps {
		if s.ID != wantIDs[i] {
			t.Errorf("step[%d].ID = %q, want %q", i, s.ID, wantIDs[i])
		}
	}
}

func TestBuildUpgradePlan_BackupStepsHaveRunIfGuard(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, idx := range []int{0, 1} {
		s := plan.Steps[idx]
		if s.RunIfCommand != "test" {
			t.Errorf("step[%d] RunIfCommand = %q, want test", idx, s.RunIfCommand)
		}
		if len(s.RunIfArgs) != 2 || s.RunIfArgs[0] != "-f" {
			t.Errorf("step[%d] RunIfArgs = %v, want [-f, <path>]", idx, s.RunIfArgs)
		}
	}
}

func TestBuildUpgradePlan_RestartApkBranchHasRunIf(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	apkStep := plan.Steps[7]
	if apkStep.ID != "restart-singbox-apk" {
		t.Fatalf("step[7].ID = %q, want restart-singbox-apk", apkStep.ID)
	}
	if apkStep.RunIfCommand != "which" || len(apkStep.RunIfArgs) != 1 || apkStep.RunIfArgs[0] != "apk" {
		t.Errorf("restart-singbox-apk gate mis-wired: cmd=%q args=%v", apkStep.RunIfCommand, apkStep.RunIfArgs)
	}
}

func TestBuildUpgradePlan_SystemdBranchNegatesApk(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	systemdStep := plan.Steps[8]
	if systemdStep.ID != "restart-singbox-systemd" {
		t.Fatalf("step[8].ID = %q, want restart-singbox-systemd", systemdStep.ID)
	}
	if systemdStep.Command != "sh" {
		t.Errorf("systemd step Command = %q, want sh", systemdStep.Command)
	}
	if len(systemdStep.Args) < 2 || systemdStep.Args[0] != "-c" {
		t.Fatalf("systemd step Args = %v", systemdStep.Args)
	}
	body := systemdStep.Args[1]
	if !strings.Contains(body, "if ! command -v apk") {
		t.Errorf("systemd branch body should negate apk presence, got: %s", body)
	}
	if !strings.Contains(body, "systemctl enable") {
		t.Errorf("systemd branch body should enable: %s", body)
	}
	if !strings.Contains(body, "systemctl restart") {
		t.Errorf("systemd branch body should restart: %s", body)
	}
}

func TestBuildUpgradePlan_TerminalStepsHaveNoRollback(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Sprint 3 routing convention: final steps have no RollbackCommand.
	for _, idx := range []int{7, 8} {
		if cmd := plan.Steps[idx].RollbackCommand; cmd != "" {
			t.Errorf("step[%d].RollbackCommand = %q, want empty (terminal step)", idx, cmd)
		}
	}
}

func TestBuildUpgradePlan_CleanupContinuesOnError(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	cleanup := plan.Steps[5]
	if cleanup.ID != "cleanup-tarball" {
		t.Fatalf("step[5].ID = %q, want cleanup-tarball", cleanup.ID)
	}
	if !cleanup.ContinueOnError {
		t.Error("cleanup-tarball must ContinueOnError")
	}
}

func TestBuildUpgradePlan_RollbackRunIfReplacesShellWrapper_Sprint5(t *testing.T) {
	// Sprint 5 refactor: install-binary and switch-config rollbacks no longer
	// shell out to `test -f && cp || true`. They use RollbackRunIfCommand to
	// gate the rollback purely through systemops core semantics.
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	expect := map[string]struct {
		rollbackCmd       string
		rollbackRunIfCmd  string
		rollbackRunIfArgs []string
	}{
		"install-binary": {
			rollbackCmd:       "cp",
			rollbackRunIfCmd:  "test",
			rollbackRunIfArgs: []string{"-f", SingBoxBin + backupExt},
		},
		"switch-config": {
			rollbackCmd:       "cp",
			rollbackRunIfCmd:  "test",
			rollbackRunIfArgs: []string{"-f", SBJSONPath + backupExt},
		},
	}

	indexed := map[string]systemops.OperationStep{}
	for _, s := range plan.Steps {
		indexed[s.ID] = s
	}
	for id, want := range expect {
		s, ok := indexed[id]
		if !ok {
			t.Errorf("missing step %s", id)
			continue
		}
		if s.RollbackCommand != want.rollbackCmd {
			t.Errorf("step %s RollbackCommand = %q, want %q (Sprint 5: no more sh -c wrapper)", id, s.RollbackCommand, want.rollbackCmd)
		}
		if s.RollbackRunIfCommand != want.rollbackRunIfCmd {
			t.Errorf("step %s RollbackRunIfCommand = %q, want %q", id, s.RollbackRunIfCommand, want.rollbackRunIfCmd)
		}
		if len(s.RollbackRunIfArgs) != len(want.rollbackRunIfArgs) {
			t.Errorf("step %s RollbackRunIfArgs len = %d, want %d", id, len(s.RollbackRunIfArgs), len(want.rollbackRunIfArgs))
			continue
		}
		for i := range want.rollbackRunIfArgs {
			if s.RollbackRunIfArgs[i] != want.rollbackRunIfArgs[i] {
				t.Errorf("step %s RollbackRunIfArgs[%d] = %q, want %q", id, i, s.RollbackRunIfArgs[i], want.rollbackRunIfArgs[i])
			}
		}
	}
}

func TestBuildUpgradePlan_RollbackCommandsCoverCriticalSteps(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	expectRollback := map[string]bool{
		"backup-binary":    true,
		"backup-config":    true,
		"download-tarball": true,
		"extract-tarball":  true,
		"install-binary":   true,
		"switch-config":    true,
	}
	for _, s := range plan.Steps {
		want := expectRollback[s.ID]
		got := s.RollbackCommand != ""
		if want != got {
			t.Errorf("step %s: RollbackCommand present=%v, want %v", s.ID, got, want)
		}
	}
}

func TestBuildUpgradePlan_DownloadURLUsesSingleQuoteAndPathSafety(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	dl := plan.Steps[2]
	if dl.Command != "curl" {
		t.Errorf("download Command = %q, want curl", dl.Command)
	}
	// args should include the URL verbatim (no sh -c wrapping)
	joined := strings.Join(dl.Args, " ")
	if !strings.Contains(joined, "https://github.com/SagerNet/sing-box/releases/download/v1.10.7/sing-box-1.10.7-linux-amd64.tar.gz") {
		t.Errorf("download args missing expected URL: %v", dl.Args)
	}
}

func TestBuildUpgradePlan_InstallStepShellQuotesPaths(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	install := plan.Steps[4]
	body := install.Args[1]
	if !strings.Contains(body, "chown root:root '/etc/s-box/sing-box'") {
		t.Errorf("install body missing chown with quoted path: %s", body)
	}
	if !strings.Contains(body, "chmod +x '/etc/s-box/sing-box'") {
		t.Errorf("install body missing chmod: %s", body)
	}
	if !strings.Contains(body, "mv '/etc/s-box/sing-box-1.10.7-linux-amd64/sing-box' '/etc/s-box/sing-box'") {
		t.Errorf("install body missing mv: %s", body)
	}
}

func TestBuildUpgradePlan_SwitchConfigPicksByMajor(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	switchStep := plan.Steps[6]
	if switchStep.ID != "switch-config" {
		t.Fatalf("step[6].ID = %q, want switch-config", switchStep.ID)
	}
	body := switchStep.Args[1]
	if !strings.Contains(body, `SRC='/etc/s-box/sb10.json'`) {
		t.Errorf("switch body missing sb10 branch: %s", body)
	}
	if !strings.Contains(body, `SRC='/etc/s-box/sb11.json'`) {
		t.Errorf("switch body missing sb11 fallback: %s", body)
	}
	if !strings.Contains(body, `cp -p "$SRC" '/etc/s-box/sb.json'`) {
		t.Errorf("switch body missing cp to sb.json: %s", body)
	}
}

// TestBuildUpgradePlan_AcceptsSystemopsCoreContract validates that the plan
// produced is consumable by the systemops core — every step has all required
// kinds, risks, and IDs are unique. This is the second line of defence on
// top of plan.Validate().
func TestBuildUpgradePlan_AcceptsSystemopsCoreContract(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.10.7", Arch: "amd64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	seen := map[string]struct{}{}
	for _, s := range plan.Steps {
		if !s.Kind.Valid() {
			t.Errorf("step %s invalid kind %q", s.ID, s.Kind)
		}
		if !s.Risk.Valid() {
			t.Errorf("step %s invalid risk %q", s.ID, s.Risk)
		}
		if _, dup := seen[s.ID]; dup {
			t.Errorf("duplicate step ID %s", s.ID)
		}
		seen[s.ID] = struct{}{}
	}
	// And one more: the systemops core's own Validate must accept the plan.
	if err := plan.Validate(); err != nil {
		t.Errorf("systemops core Validate rejected plan: %v", err)
	}
}

func TestShellQuote_Internal(t *testing.T) {
	cases := map[string]string{
		"":              `''`,
		"plain":         `'plain'`,
		"with space":    `'with space'`,
		"a'b":           `'a'\''b'`,
		"/etc/sing-box": `'/etc/sing-box'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveVersion_Direct(t *testing.T) {
	cases := []struct {
		name        string
		req         UpgradeRequest
		stable, pre string
		want        string
		wantErr     bool
	}{
		{"stable ok", UpgradeRequest{Channel: ChannelStable}, "1.10.7", "", "1.10.7", false},
		{"pre ok", UpgradeRequest{Channel: ChannelPre}, "", "1.13.0-alpha.1", "1.13.0-alpha.1", false},
		{"pinned ok", UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.30.0"}, "", "", "1.30.0", false},
		{"stable missing", UpgradeRequest{Channel: ChannelStable}, "", "1.13.0-alpha.1", "", true},
		{"pre missing", UpgradeRequest{Channel: ChannelPre}, "1.10.7", "", "", true},
		{"pinned missing", UpgradeRequest{Channel: ChannelPinned}, "", "", "", true},
		{"unknown channel", UpgradeRequest{Channel: Channel(42)}, "", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveVersion(c.req, c.stable, c.pre)
			if c.wantErr && err == nil {
				t.Errorf("expected err, got nil")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected err: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestBuildUpgradePlan_DescriptionIncludesContext is a low-cost smoke for the
// audit/observability surface: the plan description must include channel,
// version, and arch so an operator scanning audit logs can identify it.
func TestBuildUpgradePlan_DescriptionIncludesContext(t *testing.T) {
	plan, err := BuildUpgradePlan(
		UpgradeRequest{Channel: ChannelPinned, PinnedVersion: "1.30.0", Arch: "arm64"},
		"", "",
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	desc := plan.Description
	for _, want := range []string{"1.30.0", "arm64", "channel=3"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description %q missing %q", desc, want)
		}
	}
}

// Sanity check that systemops package is imported successfully (catches
// transitively-broken imports during refactors).
var _ = systemops.OperationPlan{}
