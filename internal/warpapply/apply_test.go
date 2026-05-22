package warpapply

import (
	"strings"
	"testing"

	"miaomiaowu/internal/warp"
)

// validAccount returns a syntactically valid Account suitable for plan
// construction. Reserved values pulled from sb.sh:3356 so byte-equality
// with the legacy script is preserved.
func validAccount() warp.Account {
	return warp.Account{
		PrivateKey: "g9I2sgUH6OCbIBTehkEfVEnuvInHYZvPOFhWchMLSc4=",
		IPv6:       "2606:4700:110:860e:738f:b37:f15:d38d",
		Reserved:   [3]int{33, 217, 129},
	}
}

func TestValidatePrivateKey_AcceptsWARPFormat(t *testing.T) {
	cases := []string{
		"g9I2sgUH6OCbIBTehkEfVEnuvInHYZvPOFhWchMLSc4=",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq=", // 44 chars
		"0123456789+/0123456789+/0123456789+/0123456=",
	}
	for _, c := range cases {
		if err := validatePrivateKey(c); err != nil {
			t.Errorf("validatePrivateKey(%q) unexpected error: %v", c, err)
		}
	}
}

func TestValidatePrivateKey_RejectsInjection(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"too short":          "abc=",
		"too long":           strings.Repeat("A", 100) + "=",
		"single quote":       "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'=",
		"double quote":       `AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"=`,
		"semicolon":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA;=",
		"command substitute": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA$()=",
		"space":              "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA =",
		"newline":            "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n=",
		"non-ascii":          "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA你=",
		"no trailing pad":    strings.Repeat("A", 44),
		"two pad chars":      strings.Repeat("A", 42) + "==",
	}
	for label, c := range cases {
		if err := validatePrivateKey(c); err == nil {
			t.Errorf("validatePrivateKey(%s = %q) should reject", label, c)
		}
	}
}

func TestValidateIPv6_Accepts(t *testing.T) {
	cases := []string{
		"2606:4700:110:860e:738f:b37:f15:d38d",
		"::1",
		"::cafe",
		"2606:4700::abcd",
		"fe80::1",
	}
	for _, c := range cases {
		if err := validateIPv6(c); err != nil {
			t.Errorf("validateIPv6(%q) unexpected error: %v", c, err)
		}
	}
}

func TestValidateIPv6_Rejects(t *testing.T) {
	cases := map[string]string{
		"empty":        "",
		"ipv4":         "192.168.1.1",
		"semicolon":    "::;rm -rf /",
		"single quote": "::'",
		"newline":      "::\n",
		"backtick":     "::`",
		"dollar":       "$()",
		"space":        "fe80:: 1",
		"non-hex":      "::g",
		"non-ascii":    "::你好",
		"too long":     strings.Repeat("f", 50),
		"garbage":      "not-an-ip",
	}
	for label, c := range cases {
		if err := validateIPv6(c); err == nil {
			t.Errorf("validateIPv6(%s = %q) should reject", label, c)
		}
	}
}

func TestValidateReserved_Rejects(t *testing.T) {
	bad := [][3]int{
		{-1, 0, 0},
		{0, 256, 0},
		{0, 0, 999},
		{-128, 200, 100},
	}
	for _, r := range bad {
		if err := validateReserved(r); err == nil {
			t.Errorf("validateReserved(%v) should reject", r)
		}
	}
}

func TestValidateReserved_Accepts(t *testing.T) {
	good := [][3]int{
		{0, 0, 0},
		{255, 255, 255},
		{33, 217, 129},
	}
	for _, r := range good {
		if err := validateReserved(r); err != nil {
			t.Errorf("validateReserved(%v) unexpected error: %v", r, err)
		}
	}
}

func TestBuildApplyPlan_HappyPath(t *testing.T) {
	plan, err := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	if err != nil {
		t.Fatalf("BuildApplyPlan: %v", err)
	}
	if plan.Name != PlanNameApply {
		t.Errorf("Name = %q, want %q", plan.Name, PlanNameApply)
	}
	if !plan.RollbackOnFailure {
		t.Error("RollbackOnFailure must be true")
	}
	if len(plan.Steps) != 4 {
		t.Fatalf("step count = %d, want 4", len(plan.Steps))
	}
	wantIDs := []string{"backup-config", "modify-config", "swap-active-config", "restart-singbox"}
	for i, w := range wantIDs {
		if plan.Steps[i].ID != w {
			t.Errorf("step[%d] ID = %q, want %q", i, plan.Steps[i].ID, w)
		}
	}
}

func TestBuildApplyPlan_StepCommandsAreShelled(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	if plan.Steps[0].Command != "sh" {
		t.Errorf("backup step command = %q, want sh", plan.Steps[0].Command)
	}
	if plan.Steps[1].Command != "sh" {
		t.Errorf("modify step command = %q, want sh", plan.Steps[1].Command)
	}
	if plan.Steps[2].Command != "sh" {
		t.Errorf("swap step command = %q, want sh", plan.Steps[2].Command)
	}
	if plan.Steps[3].Command != "systemctl" {
		t.Errorf("restart step command = %q, want systemctl", plan.Steps[3].Command)
	}
}

func TestBuildApplyPlan_RestartStepHasNoRollback(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	last := plan.Steps[3]
	if last.RollbackCommand != "" {
		t.Errorf("terminal step should have no RollbackCommand (Sprint 3 routing convention), got %q", last.RollbackCommand)
	}
}

func TestBuildApplyPlan_SwapStepUsesRollbackRunIf(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	swap := plan.Steps[2]
	if swap.RollbackRunIfCommand != "test" {
		t.Errorf("swap rollback should use RollbackRunIfCommand=test, got %q", swap.RollbackRunIfCommand)
	}
	if len(swap.RollbackRunIfArgs) != 2 || swap.RollbackRunIfArgs[0] != "-f" {
		t.Errorf("swap rollback RunIfArgs = %v, want [-f .bak path]", swap.RollbackRunIfArgs)
	}
}

func TestBuildApplyPlan_ModifyStepReferencesAllSixEdits(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	raw := plan.Steps[1].Metadata["raw_command"]
	// Each of the six (file, line) targets should appear in the sed pipeline.
	want := []struct{ snippet string }{
		{"163s#"}, // sb10 private_key
		{"132s#"}, // sb11 private_key
		{"161s#"}, // sb10 ipv6
		{"130s#"}, // sb11 ipv6
		{"165s#"}, // sb10 reserved
		{"142s#"}, // sb11 reserved
	}
	for _, w := range want {
		if !strings.Contains(raw, w.snippet) {
			t.Errorf("modify step missing %q in:\n%s", w.snippet, raw)
		}
	}
	if !strings.Contains(raw, SB10Path) {
		t.Errorf("modify step missing sb10 path: %s", raw)
	}
	if !strings.Contains(raw, SB11Path) {
		t.Errorf("modify step missing sb11 path: %s", raw)
	}
}

func TestBuildApplyPlan_ModifyStepInjectsPrivateKey(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	raw := plan.Steps[1].Metadata["raw_command"]
	if !strings.Contains(raw, validAccount().PrivateKey) {
		t.Errorf("modify step missing private key in raw command: %s", raw)
	}
	if !strings.Contains(raw, validAccount().IPv6) {
		t.Errorf("modify step missing ipv6 in raw command: %s", raw)
	}
	if !strings.Contains(raw, "33, 217, 129") {
		t.Errorf("modify step missing reserved tuple in raw command: %s", raw)
	}
}

func TestBuildApplyPlan_RejectsInvalidAccount(t *testing.T) {
	bad := map[string]warp.Account{
		"empty private key": {IPv6: "::1", Reserved: [3]int{1, 2, 3}},
		"empty ipv6":        {PrivateKey: "g9I2sgUH6OCbIBTehkEfVEnuvInHYZvPOFhWchMLSc4=", Reserved: [3]int{1, 2, 3}},
		"bad reserved": {
			PrivateKey: "g9I2sgUH6OCbIBTehkEfVEnuvInHYZvPOFhWchMLSc4=",
			IPv6:       "::1",
			Reserved:   [3]int{300, 0, 0},
		},
		"injection in pk": {
			PrivateKey: "g9I2sgUH6OCbIBTehkEfVEnuvInHYZvP'rm -rfXY=",
			IPv6:       "::1",
			Reserved:   [3]int{1, 2, 3},
		},
		"injection in ipv6": {
			PrivateKey: "g9I2sgUH6OCbIBTehkEfVEnuvInHYZvPOFhWchMLSc4=",
			IPv6:       "::1; rm -rf /",
			Reserved:   [3]int{1, 2, 3},
		},
	}
	for label, acc := range bad {
		if _, err := BuildApplyPlan(ApplyRequest{Account: acc}); err == nil {
			t.Errorf("%s: BuildApplyPlan should reject %+v", label, acc)
		}
	}
}

func TestBuildApplyPlan_BackupRollbackRemovesBak(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	rollback := plan.Steps[0].RollbackMetadata["raw_command"]
	if !strings.HasPrefix(rollback, "rm -f") {
		t.Errorf("backup rollback should rm -f .bak, got %q", rollback)
	}
	for _, f := range []string{SBJSONPath, SB10Path, SB11Path} {
		if !strings.Contains(rollback, f+backupExt) {
			t.Errorf("backup rollback missing %s.bak: %s", f, rollback)
		}
	}
}

func TestBuildApplyPlan_ModifyRollbackRestoresBak(t *testing.T) {
	plan, _ := BuildApplyPlan(ApplyRequest{Account: validAccount()})
	rollback := plan.Steps[1].RollbackMetadata["raw_command"]
	for _, f := range []string{SBJSONPath, SB10Path, SB11Path} {
		if !strings.Contains(rollback, f+backupExt) || !strings.Contains(rollback, f) {
			t.Errorf("modify rollback missing restore of %s: %s", f, rollback)
		}
	}
}

func TestShellQuote_HandlesSingleQuotes(t *testing.T) {
	cases := map[string]string{
		"":              `''`,
		"plain":         `'plain'`,
		"a'b":           `'a'\''b'`,
		"/etc/foo.conf": `'/etc/foo.conf'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJSONValueLine_FormatsByField(t *testing.T) {
	cases := []struct {
		field   string
		payload string
		want    string
	}{
		{"private_key", "ABC=", `"private_key": "ABC=",`},
		{"ipv6", "::1", `"::1/128",`},
		{"reserved", "1, 2, 3", `"reserved": [1, 2, 3],`},
		{"unknown", "x", "x"},
	}
	for _, c := range cases {
		if got := jsonValueLine(c.field, c.payload); got != c.want {
			t.Errorf("jsonValueLine(%q, %q) = %q, want %q", c.field, c.payload, got, c.want)
		}
	}
}
