// Plan construction for sing-box kernel upgrades. Mirrors sb.sh:3846-3894
// upsbcroe() flow with one structural difference: every shell-meta-sensitive
// value (version, arch, paths) is validated or whitelisted before being
// composed into a step body. The systemops core handles rollback, RunIf
// gating, and audit/progress emission.
package upgrade

import (
	"fmt"
	"strings"

	"miaomiaowu/internal/systemops"
)

// File paths and service name managed by sb.sh.
const (
	SBoxDir     = "/etc/s-box"
	SingBoxBin  = SBoxDir + "/sing-box"
	SBJSONPath  = SBoxDir + "/sb.json"
	SB10Path    = SBoxDir + "/sb10.json"
	SB11Path    = SBoxDir + "/sb11.json"
	TarballPath = SBoxDir + "/sing-box.tar.gz"
	ServiceName = "sing-box"
	backupExt   = ".bak"

	// PlanNameUpgrade is the canonical plan name persisted in audit records.
	PlanNameUpgrade = "singbox.upgrade"

	// DownloadURLTemplate is the GitHub release artifact URL.
	// %s ordering: version, archive-stem (sing-box-<version>-linux-<arch>).
	DownloadURLTemplate = "https://github.com/SagerNet/sing-box/releases/download/v%s/%s.tar.gz"
)

// Channel enumerates the three upsbcroe selectors (sb.sh:3855-3865).
type Channel int

const (
	ChannelStable Channel = 1 // latcore — latest stable release
	ChannelPre    Channel = 2 // precore — latest alpha/rc/beta
	ChannelPinned Channel = 3 // user-supplied PinnedVersion
)

// UpgradeRequest is the canonical input to BuildUpgradePlan. It is consumed
// by both the synchronous and the SSE-streamed handler.
type UpgradeRequest struct {
	Channel       Channel
	PinnedVersion string // mandatory when Channel == ChannelPinned
	Arch          string // amd64 / arm64 / 386 / armv7
}

// allowedArches mirrors the cpu= dispatch table in sb.sh:47-52 plus a 386
// branch the upstream script never registers but GitHub releases ship.
//
// Any value outside this set is rejected before reaching the URL template or
// the shell — this is a security boundary, not a UX nicety.
var allowedArches = map[string]struct{}{
	"amd64": {},
	"arm64": {},
	"386":   {},
	"armv7": {},
}

// BuildUpgradePlan composes the eight-step upgrade plan. The function is pure
// (no network, no disk) so it is trivial to unit-test; the dynamic version
// lookup is performed by FetchLatest in release.go and the two values are
// passed in here.
//
// latestStable / latestPre may be empty strings when FetchLatest could not
// reach upstream. In that case Channel must be ChannelPinned with a valid
// PinnedVersion — otherwise BuildUpgradePlan returns an error and the
// operator is required to supply a version explicitly.
func BuildUpgradePlan(req UpgradeRequest, latestStable, latestPre string) (systemops.OperationPlan, error) {
	version, err := resolveVersion(req, latestStable, latestPre)
	if err != nil {
		return systemops.OperationPlan{}, err
	}

	if _, ok := allowedArches[req.Arch]; !ok {
		return systemops.OperationPlan{}, fmt.Errorf("arch %q not in whitelist (amd64/arm64/386/armv7)", req.Arch)
	}

	if err := ValidateVersion(version); err != nil {
		// Defense in depth: even values that come from FetchLatest pass through
		// the same gate as user input.
		return systemops.OperationPlan{}, fmt.Errorf("validate version: %w", err)
	}

	// The archive stem is composed from values that have all just been
	// validated/whitelisted — safe to interpolate.
	archiveStem := fmt.Sprintf("sing-box-%s-linux-%s", version, req.Arch)
	downloadURL := fmt.Sprintf(DownloadURLTemplate, version, archiveStem)
	extractedDir := SBoxDir + "/" + archiveStem

	// install-binary, cleanup, switch-config are composed of multiple shell
	// commands that must succeed atomically — so they use sh -c with errexit
	// implicit per && chaining.

	installSnippet := strings.Join([]string{
		fmt.Sprintf("mv %s %s", shellQuote(extractedDir+"/sing-box"), shellQuote(SingBoxBin)),
		fmt.Sprintf("chown root:root %s", shellQuote(SingBoxBin)),
		fmt.Sprintf("chmod +x %s", shellQuote(SingBoxBin)),
	}, " && ")

	cleanupSnippet := fmt.Sprintf("rm -rf %s %s", shellQuote(TarballPath), shellQuote(extractedDir))

	// switch-config: detect new binary's major version and copy the matching
	// sb1x.json over sb.json (sb.sh:3879-3882). Falls back to sb11 when
	// version probe fails, matching the script's default branch.
	switchConfigSnippet := fmt.Sprintf(
		`SBNH=$(%s version 2>/dev/null | awk '/version/{print $NF}' | cut -d '.' -f 1,2); `+
			`if [ "$SBNH" = "1.10" ]; then SRC=%s; else SRC=%s; fi; `+
			`cp -p "$SRC" %s`,
		shellQuote(SingBoxBin),
		shellQuote(SB10Path),
		shellQuote(SB11Path),
		shellQuote(SBJSONPath),
	)

	// restart-singbox: sb.sh branches on `command -v apk`. systemops has no
	// "skip if probe succeeds" primitive, so the non-apk path uses sh -c with
	// an explicit negation. This is the documented sb.sh:3769-3777 flow,
	// reduced to one step per branch with RunIfCommand short-circuiting.
	restartSystemdSnippet := "if ! command -v apk >/dev/null 2>&1; then " +
		fmt.Sprintf("systemctl enable %s && systemctl start %s && systemctl restart %s; ",
			shellQuote(ServiceName), shellQuote(ServiceName), shellQuote(ServiceName)) +
		"fi"

	steps := []systemops.OperationStep{
		{
			ID:              "backup-binary",
			Title:           "备份现有 sing-box 二进制",
			Description:     "cp -p " + SingBoxBin + " " + SingBoxBin + backupExt,
			Kind:            systemops.StepKindFile,
			Risk:            systemops.RiskLevelLow,
			Target:          SingBoxBin,
			Command:         "cp",
			Args:            []string{"-p", SingBoxBin, SingBoxBin + backupExt},
			Metadata:        map[string]string{"raw_command": "cp -p " + SingBoxBin + " " + SingBoxBin + backupExt},
			RunIfCommand:    "test",
			RunIfArgs:       []string{"-f", SingBoxBin},
			RollbackCommand: "rm",
			RollbackArgs:    []string{"-f", SingBoxBin + backupExt},
			RollbackMetadata: map[string]string{
				"raw_command": "rm -f " + SingBoxBin + backupExt,
				"intent":      "remove the binary backup created by this step",
			},
		},
		{
			ID:              "backup-config",
			Title:           "备份现有 sb.json 配置",
			Description:     "cp -p " + SBJSONPath + " " + SBJSONPath + backupExt,
			Kind:            systemops.StepKindFile,
			Risk:            systemops.RiskLevelLow,
			Target:          SBJSONPath,
			Command:         "cp",
			Args:            []string{"-p", SBJSONPath, SBJSONPath + backupExt},
			Metadata:        map[string]string{"raw_command": "cp -p " + SBJSONPath + " " + SBJSONPath + backupExt},
			RunIfCommand:    "test",
			RunIfArgs:       []string{"-f", SBJSONPath},
			RollbackCommand: "rm",
			RollbackArgs:    []string{"-f", SBJSONPath + backupExt},
			RollbackMetadata: map[string]string{
				"raw_command": "rm -f " + SBJSONPath + backupExt,
				"intent":      "remove the config backup created by this step",
			},
		},
		{
			ID:          "download-tarball",
			Title:       fmt.Sprintf("下载 sing-box v%s (%s) 压缩包", version, req.Arch),
			Description: "curl -L --retry 2 -o " + TarballPath + " " + downloadURL,
			Kind:        systemops.StepKindBinary,
			Risk:        systemops.RiskLevelMedium,
			Target:      TarballPath,
			Command:     "curl",
			Args:        []string{"-L", "--retry", "2", "-fSs", "-o", TarballPath, downloadURL},
			Metadata: map[string]string{
				"raw_command": "curl -L --retry 2 -fSs -o " + TarballPath + " " + downloadURL,
				"version":     version,
				"arch":        req.Arch,
				"url":         downloadURL,
			},
			RollbackCommand: "rm",
			RollbackArgs:    []string{"-f", TarballPath},
			RollbackMetadata: map[string]string{
				"raw_command": "rm -f " + TarballPath,
				"intent":      "remove partially downloaded tarball",
			},
		},
		{
			ID:              "extract-tarball",
			Title:           "解压 sing-box 压缩包到 /etc/s-box",
			Description:     "tar xzf " + TarballPath + " -C " + SBoxDir,
			Kind:            systemops.StepKindBinary,
			Risk:            systemops.RiskLevelMedium,
			Target:          extractedDir,
			Command:         "tar",
			Args:            []string{"xzf", TarballPath, "-C", SBoxDir},
			Metadata:        map[string]string{"raw_command": "tar xzf " + TarballPath + " -C " + SBoxDir},
			RollbackCommand: "rm",
			RollbackArgs:    []string{"-rf", extractedDir},
			RollbackMetadata: map[string]string{
				"raw_command": "rm -rf " + extractedDir,
				"intent":      "remove the extracted release directory",
			},
		},
		{
			ID:          "install-binary",
			Title:       "安装新二进制并设置 root:root 700",
			Description: installSnippet,
			Kind:        systemops.StepKindBinary,
			Risk:        systemops.RiskLevelCritical,
			Target:      SingBoxBin,
			Command:     "sh",
			Args:        []string{"-c", installSnippet},
			Metadata:    map[string]string{"raw_command": installSnippet},
			// Restore the previous binary from .bak only when it exists. The
			// RollbackRunIfCommand probe (Sprint 5) replaces the prior
			// `sh -c "test -f && cp || true"` wrapper: on first install the
			// .bak is absent, the probe short-circuits, and the rollback step
			// is silently skipped — no shell, no ambiguous exit-code juggling.
			RollbackCommand: "cp",
			RollbackArgs:    []string{"-p", SingBoxBin + backupExt, SingBoxBin},
			RollbackMetadata: map[string]string{
				"raw_command": "cp -p " + SingBoxBin + backupExt + " " + SingBoxBin,
				"intent":      "restore the previous binary from .bak if present",
			},
			RollbackRunIfCommand: "test",
			RollbackRunIfArgs:    []string{"-f", SingBoxBin + backupExt},
		},
		{
			ID:              "cleanup-tarball",
			Title:           "清理压缩包和解压目录",
			Description:     cleanupSnippet,
			Kind:            systemops.StepKindFile,
			Risk:            systemops.RiskLevelLow,
			Target:          TarballPath,
			Command:         "sh",
			Args:            []string{"-c", cleanupSnippet},
			Metadata:        map[string]string{"raw_command": cleanupSnippet},
			ContinueOnError: true, // 清理失败不应阻断升级
		},
		{
			ID:              "switch-config",
			Title:           "根据新内核大版本切换 sb.json",
			Description:     switchConfigSnippet,
			Kind:            systemops.StepKindFile,
			Risk:            systemops.RiskLevelHigh,
			Target:          SBJSONPath,
			Command:         "sh",
			Args:            []string{"-c", switchConfigSnippet},
			Metadata:        map[string]string{"raw_command": switchConfigSnippet},
			RollbackCommand: "cp",
			RollbackArgs:    []string{"-p", SBJSONPath + backupExt, SBJSONPath},
			RollbackMetadata: map[string]string{
				"raw_command": "cp -p " + SBJSONPath + backupExt + " " + SBJSONPath,
				"intent":      "restore the previous sb.json from .bak if present",
			},
			RollbackRunIfCommand: "test",
			RollbackRunIfArgs:    []string{"-f", SBJSONPath + backupExt},
		},
		{
			ID:           "restart-singbox-apk",
			Title:        "重启 sing-box (apk / OpenRC 分支)",
			Description:  "rc-service sing-box restart",
			Kind:         systemops.StepKindService,
			Risk:         systemops.RiskLevelHigh,
			Target:       ServiceName,
			Command:      "rc-service",
			Args:         []string{ServiceName, "restart"},
			Metadata:     map[string]string{"raw_command": "rc-service " + ServiceName + " restart"},
			RunIfCommand: "which",
			RunIfArgs:    []string{"apk"},
			// Final step: no rollback (Sprint 3 routing.go convention).
			// Failure here triggers prior steps' rollback which restores binary
			// + config to the pre-upgrade state.
		},
		{
			ID:          "restart-singbox-systemd",
			Title:       "重启 sing-box (systemd 分支)",
			Description: restartSystemdSnippet,
			Kind:        systemops.StepKindService,
			Risk:        systemops.RiskLevelHigh,
			Target:      ServiceName,
			Command:     "sh",
			Args:        []string{"-c", restartSystemdSnippet},
			Metadata: map[string]string{
				"raw_command": restartSystemdSnippet,
				"intent":      "no-op when apk is present; the apk branch above handles that case",
			},
			// Final step: same reasoning as above. The sh -c body is itself a
			// guard ("if ! command -v apk") so this step is idempotent when
			// apk is present.
		},
	}

	plan := systemops.OperationPlan{
		Name: PlanNameUpgrade,
		Description: fmt.Sprintf("Upgrade sing-box kernel: channel=%d version=%s arch=%s",
			req.Channel, version, req.Arch),
		DryRun:            false,
		RollbackOnFailure: true,
		Steps:             steps,
	}

	if err := plan.Validate(); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("validate plan: %w", err)
	}
	return plan, nil
}

// resolveVersion picks the version string for the requested channel.
//
// For ChannelStable / ChannelPre the corresponding latestStable / latestPre
// argument must be non-empty; an empty string means FetchLatest could not
// reach upstream and the operator must fall back to ChannelPinned with an
// explicit PinnedVersion (the "must Pinned mode" fallback documented in
// release.go).
func resolveVersion(req UpgradeRequest, latestStable, latestPre string) (string, error) {
	switch req.Channel {
	case ChannelStable:
		if latestStable == "" {
			return "", fmt.Errorf("stable version lookup failed; please use Pinned mode with an explicit version")
		}
		return latestStable, nil
	case ChannelPre:
		if latestPre == "" {
			return "", fmt.Errorf("prerelease version lookup failed; please use Pinned mode with an explicit version")
		}
		return latestPre, nil
	case ChannelPinned:
		if req.PinnedVersion == "" {
			return "", fmt.Errorf("pinned channel requires PinnedVersion")
		}
		return req.PinnedVersion, nil
	default:
		return "", fmt.Errorf("invalid channel %d (expected 1=stable, 2=pre, 3=pinned)", req.Channel)
	}
}

// shellQuote wraps s in single quotes for safe sh -c interpolation. Identical
// to the routing package helper; duplicated here to keep packages decoupled.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
