// Package warpapply wires the WARP account produced by warp.Register (or
// supplied directly by the operator) into sb.json / sb10.json / sb11.json
// and restarts sing-box.
//
// Business reference: sb.sh:3367-3415 changewg() — sed-replaces the three
// account fields (private_key on line 163/132, ipv6 on line 161/130, reserved
// on line 165/142 of sb10/sb11.json respectively) then copies the active file
// over sb.json and restarts the service.
//
// The package reuses the Sprint 2.2 routing.go template (backup → sed → copy
// → restart) with rollback so failed apply leaves the prior wireguard
// account intact.
package warpapply

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"miaomiaowu/internal/systemops"
	"miaomiaowu/internal/warp"
)

// File paths managed by sb.sh.
const (
	SBJSONPath  = "/etc/s-box/sb.json"
	SB10Path    = "/etc/s-box/sb10.json"
	SB11Path    = "/etc/s-box/sb11.json"
	SingBoxBin  = "/etc/s-box/sing-box"
	serviceName = "sing-box"
	backupExt   = ".bak"

	// PlanNameApply is the canonical plan name persisted in audit records.
	PlanNameApply = "warp.apply"
)

// fileEdit captures one sed line-replacement. The line numbers come straight
// from sb.sh:3398-3408 — sb10/sb11 differ because the JSON layout changed
// across sing-box 1.10 → 1.30+.
type fileEdit struct {
	File  string
	Line  int
	Field string // "private_key" | "ipv6" | "reserved"
}

// allEdits enumerates every sed target. Three fields × two files = 6 edits.
var allEdits = []fileEdit{
	{SB10Path, 163, "private_key"},
	{SB11Path, 132, "private_key"},
	{SB10Path, 161, "ipv6"},
	{SB11Path, 130, "ipv6"},
	{SB10Path, 165, "reserved"},
	{SB11Path, 142, "reserved"},
}

// privateKeyPattern is a strict base64 allow-list. WARP wireguard private
// keys decode to 32 bytes, so the base64 representation is exactly 44 bytes
// including the trailing "=" pad.
var privateKeyPattern = regexp.MustCompile(`^[A-Za-z0-9+/]{43}=$`)

// validatePrivateKey rejects anything that could break out of a single-
// quoted shell context, including the "=" character at any position other
// than the canonical pad slot.
func validatePrivateKey(k string) error {
	if k == "" {
		return fmt.Errorf("private_key is required")
	}
	if !privateKeyPattern.MatchString(k) {
		return fmt.Errorf("private_key %q does not match expected WARP wireguard format (44 chars, base64, single = pad)", k)
	}
	return nil
}

// validateIPv6 ensures the address parses as a real IPv6 literal and
// contains no shell metacharacters. The regex pre-check rejects obvious
// injection vectors before net.ParseIP even runs.
var ipv6CharPattern = regexp.MustCompile(`^[0-9a-fA-F:.]+$`)

func validateIPv6(v string) error {
	if v == "" {
		return fmt.Errorf("ipv6 is required")
	}
	if len(v) > 45 {
		return fmt.Errorf("ipv6 too long: %d bytes (max 45)", len(v))
	}
	if !ipv6CharPattern.MatchString(v) {
		return fmt.Errorf("ipv6 %q contains forbidden characters", v)
	}
	parsed := net.ParseIP(v)
	if parsed == nil {
		return fmt.Errorf("ipv6 %q is not a valid IP literal", v)
	}
	if parsed.To4() != nil {
		return fmt.Errorf("ipv6 %q is an IPv4 address, expected IPv6", v)
	}
	return nil
}

// validateReserved enforces each tuple element ∈ [0,255]. The array form is
// already type-safe but cloudflare can in theory return signed 32-bit ints.
func validateReserved(r [3]int) error {
	for i, v := range r {
		if v < 0 || v > 255 {
			return fmt.Errorf("reserved[%d] = %d out of range [0,255]", i, v)
		}
	}
	return nil
}

// ApplyRequest captures the account to inject. Use a warp.Account or build
// one manually from operator input.
type ApplyRequest struct {
	Account warp.Account
}

// BuildApplyPlan returns a 4-step plan that mirrors sb.sh:3367-3415 changewg().
//
// Step layout:
//
//	1 backup-config       cp -p sb.json/sb10.json/sb11.json → .bak
//	2 modify-config       sed -i N s/.*/<payload>/ across all six edits
//	3 swap-active-config  sh -c "SBNH=$(sing-box version | cut)...; cp sb${10|11}.json sb.json"
//	4 restart-singbox     systemctl restart sing-box (no rollback — terminal step)
func BuildApplyPlan(req ApplyRequest) (systemops.OperationPlan, error) {
	if err := validatePrivateKey(req.Account.PrivateKey); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("account.private_key: %w", err)
	}
	if err := validateIPv6(req.Account.IPv6); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("account.ipv6: %w", err)
	}
	if err := validateReserved(req.Account.Reserved); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("account.reserved: %w", err)
	}

	// Backup step: every file we touch (sb.json, sb10.json, sb11.json).
	backupFiles := []string{SBJSONPath, SB10Path, SB11Path}
	backupCmds := make([]string, 0, len(backupFiles))
	rmBackupCmds := make([]string, 0, len(backupFiles))
	restoreCmds := make([]string, 0, len(backupFiles))
	for _, f := range backupFiles {
		backupCmds = append(backupCmds, fmt.Sprintf("cp -p %s %s", shellQuote(f), shellQuote(f+backupExt)))
		rmBackupCmds = append(rmBackupCmds, shellQuote(f+backupExt))
		restoreCmds = append(restoreCmds, fmt.Sprintf("cp -p %s %s", shellQuote(f+backupExt), shellQuote(f)))
	}
	backupCmd := strings.Join(backupCmds, " && ")
	rmBackupCmd := "rm -f " + strings.Join(rmBackupCmds, " ")
	restoreCmd := strings.Join(restoreCmds, " && ")

	// Modify step: build one sed invocation per (file, line, field) tuple.
	payloads := map[string]string{
		"private_key": req.Account.PrivateKey,
		"ipv6":        req.Account.IPv6,
		"reserved":    fmt.Sprintf("%d, %d, %d", req.Account.Reserved[0], req.Account.Reserved[1], req.Account.Reserved[2]),
	}
	sedCmds := make([]string, 0, len(allEdits))
	for _, e := range allEdits {
		payload := payloads[e.Field]
		// Each line in sb1x.json has a unique structural prefix (the JSON
		// key + colon) so we replace the whole line content. The payload
		// has been validated to contain no single quotes / shell meta.
		sedCmds = append(sedCmds, fmt.Sprintf(
			"sed -i %s %s",
			shellQuote(fmt.Sprintf("%ds#.*#%s#", e.Line, jsonValueLine(e.Field, payload))),
			shellQuote(e.File),
		))
	}
	modifyCmd := strings.Join(sedCmds, " && ")

	// Swap step: pick sb10/sb11 by detected sing-box major version, copy onto
	// the active sb.json. Mirrors sb.sh:3367 `[[ "$sbnh" == "1.10" ]] && num=10`.
	swapCmd := fmt.Sprintf(
		`SBNH=$(%s version 2>/dev/null | awk '/version/{print $NF}' | cut -d '.' -f 1,2); `+
			`if [ "$SBNH" = "1.10" ]; then SRC=%s; else SRC=%s; fi; `+
			`cp -p "$SRC" %s`,
		shellQuote(SingBoxBin),
		shellQuote(SB10Path),
		shellQuote(SB11Path),
		shellQuote(SBJSONPath),
	)

	plan := systemops.OperationPlan{
		Name:              PlanNameApply,
		Description:       "应用 WARP 账户到 sb.json/sb10/sb11 并重启 sing-box",
		RollbackOnFailure: true,
		Steps: []systemops.OperationStep{
			{
				ID:              "backup-config",
				Title:           "Backup sb.json/sb10.json/sb11.json",
				Kind:            systemops.StepKindFile,
				Risk:            systemops.RiskLevelLow,
				Target:          strings.Join(backupFiles, ","),
				Command:         "sh",
				Args:            []string{"-c", backupCmd},
				Metadata:        map[string]string{"raw_command": backupCmd},
				RollbackCommand: "sh",
				RollbackArgs:    []string{"-c", rmBackupCmd},
				RollbackMetadata: map[string]string{
					"raw_command": rmBackupCmd,
					"intent":      "remove backups created by this step",
				},
			},
			{
				ID:              "modify-config",
				Title:           "Inject WARP account fields into sb10/sb11.json",
				Kind:            systemops.StepKindFile,
				Risk:            systemops.RiskLevelHigh,
				Target:          SB10Path + "," + SB11Path,
				Command:         "sh",
				Args:            []string{"-c", modifyCmd},
				Metadata:        map[string]string{"raw_command": modifyCmd},
				RollbackCommand: "sh",
				RollbackArgs:    []string{"-c", restoreCmd},
				RollbackMetadata: map[string]string{
					"raw_command": restoreCmd,
					"intent":      "restore originals from .bak",
				},
			},
			{
				ID:              "swap-active-config",
				Title:           "Pick sb10 or sb11 by detected major and overwrite sb.json",
				Kind:            systemops.StepKindFile,
				Risk:            systemops.RiskLevelHigh,
				Target:          SBJSONPath,
				Command:         "sh",
				Args:            []string{"-c", swapCmd},
				Metadata:        map[string]string{"raw_command": swapCmd},
				RollbackCommand: "sh",
				RollbackArgs: []string{
					"-c",
					fmt.Sprintf("cp -p %s %s", shellQuote(SBJSONPath+backupExt), shellQuote(SBJSONPath)),
				},
				RollbackRunIfCommand: "test",
				RollbackRunIfArgs:    []string{"-f", SBJSONPath + backupExt},
				RollbackMetadata: map[string]string{
					"intent": "restore prior sb.json from .bak if present",
				},
			},
			{
				ID:       "restart-singbox",
				Title:    "systemctl restart sing-box",
				Kind:     systemops.StepKindService,
				Risk:     systemops.RiskLevelHigh,
				Target:   serviceName,
				Command:  "systemctl",
				Args:     []string{"restart", serviceName},
				Metadata: map[string]string{"raw_command": "systemctl restart " + serviceName},
				// Final step: no RollbackCommand (Sprint 3 routing.go convention).
			},
		},
	}

	if err := plan.Validate(); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("validate plan: %w", err)
	}
	return plan, nil
}

// jsonValueLine renders the post-sed JSON value line for a given field. The
// callers have already validated the payload; this function only chooses the
// right serializer per field.
//
// Field formats match sb10/sb11.json layouts:
//
//	private_key  →  "private_key": "<base64>"     (trailing comma added if needed by sed-line semantics)
//	ipv6         →  "<ipv6>/128",                 (kept as inline literal — sb.sh:3400 only replaces the address portion via sed)
//	reserved     →  "reserved": [a, b, c]
//
// Note: sb.sh uses positional sed replacements that overwrite the WHOLE line,
// so we re-emit the JSON key + value verbatim to keep the file parseable.
func jsonValueLine(field, payload string) string {
	switch field {
	case "private_key":
		return fmt.Sprintf(`"private_key": "%s",`, payload)
	case "ipv6":
		return fmt.Sprintf(`"%s/128",`, payload)
	case "reserved":
		return fmt.Sprintf(`"reserved": [%s],`, payload)
	default:
		// Unreachable: caller is internal and uses the closed set above.
		return payload
	}
}

// shellQuote wraps s in single quotes for safe sh -c interpolation.
// Identical to routing.shellQuote; duplicated here to keep packages decoupled.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
