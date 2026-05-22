// Package routing encapsulates the sb.sh sbymfl/changefl domain routing flow
// as systemops OperationPlan instances.
//
// Business reference: sb.sh:3417-3767 (sbymfl, changefl, changef) — six channels
// × two modes (complete domain suffix / geosite). Modifications target lines
// 184/187/193/196/202/205/211/214/220/223/229/232 of /etc/s-box/sb.json (and
// sibling sb10.json / sb11.json) followed by `systemctl restart sing-box`.
package routing

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"miaomiaowu/internal/systemops"
)

// Channel enumerates the six routing channels exposed by changefl().
type Channel int

const (
	ChannelWarpWireguardIPv4 Channel = 1
	ChannelWarpWireguardIPv6 Channel = 2
	ChannelWarpSocks5IPv4    Channel = 3
	ChannelWarpSocks5IPv6    Channel = 4
	ChannelVPSLocalIPv4      Channel = 5
	ChannelVPSLocalIPv6      Channel = 6
)

// Mode picks between complete-domain-suffix routing and geosite tag routing.
type Mode int

const (
	ModeDomainSuffix Mode = 1
	ModeGeoSite      Mode = 2
)

// File paths managed by sb.sh.
const (
	SBJSONPath  = "/etc/s-box/sb.json"
	SB10Path    = "/etc/s-box/sb10.json"
	SB11Path    = "/etc/s-box/sb11.json"
	SingBoxBin  = "/etc/s-box/sing-box"
	backupExt   = ".bak"
	emptyMarker = "yg_kkk"
	serviceName = "sing-box"

	// PlanNameUpdate is the canonical plan name persisted in audit records.
	PlanNameUpdate = "routing.update"
)

// UpdateRequest captures one user-initiated routing change. Empty Domains means
// "reset this channel/mode to the placeholder marker (yg_kkk)".
type UpdateRequest struct {
	Channel Channel
	Mode    Mode
	Domains []string
}

// fileEdit captures one sed line-replacement.
type fileEdit struct {
	File string
	Line int
}

// editKey is the (channel, mode) lookup key for editMap.
type editKey struct {
	Channel Channel
	Mode    Mode
}

// editMap mirrors the sed targets in sb.sh:3559-3766. For each (channel, mode)
// we record every file:line that must be rewritten.
var editMap = map[editKey][]fileEdit{
	{ChannelWarpWireguardIPv4, ModeDomainSuffix}: {{SBJSONPath, 184}, {SB10Path, 184}},
	{ChannelWarpWireguardIPv4, ModeGeoSite}:      {{SBJSONPath, 187}, {SB10Path, 187}},
	{ChannelWarpWireguardIPv6, ModeDomainSuffix}: {{SB10Path, 193}, {SB11Path, 184}, {SB11Path, 196}},
	{ChannelWarpWireguardIPv6, ModeGeoSite}:      {{SBJSONPath, 196}, {SB10Path, 196}},
	{ChannelWarpSocks5IPv4, ModeDomainSuffix}:    {{SB10Path, 202}, {SB11Path, 177}, {SB11Path, 190}},
	{ChannelWarpSocks5IPv4, ModeGeoSite}:         {{SBJSONPath, 205}, {SB10Path, 205}},
	{ChannelWarpSocks5IPv6, ModeDomainSuffix}:    {{SBJSONPath, 211}, {SB10Path, 211}},
	{ChannelWarpSocks5IPv6, ModeGeoSite}:         {{SBJSONPath, 214}, {SB10Path, 214}},
	{ChannelVPSLocalIPv4, ModeDomainSuffix}:      {{SB10Path, 220}, {SBJSONPath, 220}},
	{ChannelVPSLocalIPv4, ModeGeoSite}:           {{SBJSONPath, 223}, {SB10Path, 223}},
	{ChannelVPSLocalIPv6, ModeDomainSuffix}:      {{SB10Path, 229}, {SBJSONPath, 229}},
	{ChannelVPSLocalIPv6, ModeGeoSite}:           {{SBJSONPath, 232}, {SB10Path, 232}},
}

// copyAfterSet records (channel, mode) combinations that require copying
// sb{10|11}.json over sb.json after the sed edits (sb.sh:3604/3639).
var copyAfterSet = map[editKey]bool{
	{ChannelWarpWireguardIPv6, ModeDomainSuffix}: true,
	{ChannelWarpSocks5IPv4, ModeDomainSuffix}:    true,
}

// domainPattern is a strict allow-list: ASCII letters, digits, dot, exclamation
// mark, hyphen. Sufficient for RFC 1035 host labels and geosite identifiers
// (e.g. "netflix", "geolocation-!cn"). Rejects every character that could
// terminate or hijack a shell context.
var domainPattern = regexp.MustCompile(`^[a-zA-Z0-9.!\-]+$`)

// validateDomain rejects any string that could break out of a shell single-quote
// context or sed substitution. Rejected characters include:
//
//	" ' \ \n \r \t ; $ ` & | * ? < > ( ) { } [ ] # / : + , = % space
//	plus any other non-ASCII / control character.
//
// Allowed characters: a-z A-Z 0-9 . ! -
// Max length: 253 bytes (RFC 1035 §2.3.4).
func validateDomain(s string) error {
	if s == "" {
		return fmt.Errorf("domain entry cannot be empty")
	}
	if len(s) > 253 {
		return fmt.Errorf("domain entry too long: %d bytes (max 253)", len(s))
	}
	if !domainPattern.MatchString(s) {
		return fmt.Errorf("domain entry %q contains invalid characters (allowed: a-zA-Z0-9.!-)", s)
	}
	if s == emptyMarker {
		return fmt.Errorf("domain entry %q is reserved", emptyMarker)
	}
	return nil
}

// BuildUpdatePlan returns a 3-step OperationPlan implementing the changefl flow:
//  1. Backup the sb.json/sb10.json/sb11.json files that will be touched.
//  2. Apply sed line replacements (and optionally cp sb{10|11}.json → sb.json).
//  3. systemctl restart sing-box.
//
// Rollback semantics (per Sprint 2.2 spec, refined in Sprint 3):
//   - step1 rollback = remove backups (undoes the backup creation).
//   - step2 rollback = cp .bak files back over the originals.
//   - step3 rollback = none (final step; failure triggers prior steps'
//     rollback which restores .bak and the operator can re-issue restart).
func BuildUpdatePlan(req UpdateRequest) (systemops.OperationPlan, error) {
	if req.Channel < ChannelWarpWireguardIPv4 || req.Channel > ChannelVPSLocalIPv6 {
		return systemops.OperationPlan{}, fmt.Errorf("invalid channel %d (expected 1..6)", req.Channel)
	}
	if req.Mode != ModeDomainSuffix && req.Mode != ModeGeoSite {
		return systemops.OperationPlan{}, fmt.Errorf("invalid mode %d (expected 1 or 2)", req.Mode)
	}

	for i, d := range req.Domains {
		if err := validateDomain(d); err != nil {
			return systemops.OperationPlan{}, fmt.Errorf("domains[%d]: %w", i, err)
		}
	}

	key := editKey{req.Channel, req.Mode}
	edits, ok := editMap[key]
	if !ok {
		return systemops.OperationPlan{}, fmt.Errorf("no edits registered for channel %d mode %d", req.Channel, req.Mode)
	}

	// Build the modified-files set, then the deterministically-ordered list.
	modifiedSet := map[string]struct{}{}
	for _, e := range edits {
		modifiedSet[e.File] = struct{}{}
	}
	if copyAfterSet[key] {
		modifiedSet[SBJSONPath] = struct{}{}
	}
	files := make([]string, 0, len(modifiedSet))
	for f := range modifiedSet {
		files = append(files, f)
	}
	sort.Strings(files)

	payload := buildSedPayload(req.Domains)

	// Step 1: backup.
	backupCmds := make([]string, 0, len(files))
	for _, f := range files {
		backupCmds = append(backupCmds, fmt.Sprintf("cp -p %s %s", shellQuote(f), shellQuote(f+backupExt)))
	}
	backupCmd := strings.Join(backupCmds, " && ")

	rmBackupCmds := make([]string, 0, len(files))
	for _, f := range files {
		rmBackupCmds = append(rmBackupCmds, shellQuote(f+backupExt))
	}
	rmBackupCmd := "rm -f " + strings.Join(rmBackupCmds, " ")

	// Step 2: sed (and optional cp).
	sedCmds := make([]string, 0, len(edits))
	for _, e := range edits {
		// Format: sed -i '<line>s/.*/<payload>/' '<file>'
		// payload is wrapped in single quotes; validation guarantees no single
		// quotes inside the payload itself.
		sedCmds = append(sedCmds, fmt.Sprintf(
			"sed -i %s %s",
			shellQuote(fmt.Sprintf("%ds/.*/%s/", e.Line, payload)),
			shellQuote(e.File),
		))
	}
	modifyCmd := strings.Join(sedCmds, " && ")
	if copyAfterSet[key] {
		// Pick sb10/sb11 source at run time, mirroring sb.sh `sbnh=... && num=10 or 11`.
		// Fallback to sb11 when version detection fails (matches the script's
		// `[[ "$sbnh" == "1.10" ]] && num=10 || num=11` default branch).
		copySnippet := fmt.Sprintf(
			"SBNH=$(%s version 2>/dev/null | awk '/version/{print $NF}' | cut -d '.' -f 1,2); "+
				"if [ \"$SBNH\" = \"1.10\" ]; then SRC=%s; else SRC=%s; fi; "+
				"cp -p \"$SRC\" %s",
			shellQuote(SingBoxBin),
			shellQuote(SB10Path),
			shellQuote(SB11Path),
			shellQuote(SBJSONPath),
		)
		modifyCmd += " && " + copySnippet
	}

	restoreCmds := make([]string, 0, len(files))
	for _, f := range files {
		restoreCmds = append(restoreCmds, fmt.Sprintf("cp -p %s %s", shellQuote(f+backupExt), shellQuote(f)))
	}
	restoreCmd := strings.Join(restoreCmds, " && ")

	plan := systemops.OperationPlan{
		Name:              PlanNameUpdate,
		Description:       fmt.Sprintf("Update sing-box routing: channel=%d mode=%d entries=%d", req.Channel, req.Mode, len(req.Domains)),
		DryRun:            false,
		RollbackOnFailure: true,
		Steps: []systemops.OperationStep{
			{
				ID:              "backup-config",
				Title:           "Backup sb.json/sb10.json/sb11.json",
				Kind:            systemops.StepKindFile,
				Risk:            systemops.RiskLevelLow,
				Target:          strings.Join(files, ","),
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
				Title:           fmt.Sprintf("Apply routing edits (channel %d, mode %d)", req.Channel, req.Mode),
				Kind:            systemops.StepKindFile,
				Risk:            systemops.RiskLevelHigh,
				Target:          strings.Join(files, ","),
				Command:         "sh",
				Args:            []string{"-c", modifyCmd},
				Metadata:        map[string]string{"raw_command": modifyCmd, "payload": payload},
				RollbackCommand: "sh",
				RollbackArgs:    []string{"-c", restoreCmd},
				RollbackMetadata: map[string]string{
					"raw_command": restoreCmd,
					"intent":      "restore originals from .bak",
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
				// Final step: no RollbackCommand. The systemops core skips
				// rollback for steps with empty RollbackCommand (plan.go), so
				// failure here triggers rollback of the prior two steps which
				// restore .bak and the next-best-effort sing-box restart can
				// be invoked by the operator manually if needed.
			},
		},
	}

	if err := plan.Validate(); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("validate plan: %w", err)
	}
	return plan, nil
}

// buildSedPayload turns the user's domain list into the literal text that will
// replace the configured rules[].domain_suffix / geosite array element line.
//
// Empty list → "yg_kkk" placeholder (matches sb.sh: w4flym='"yg_kkk"').
// Non-empty list → "d1","d2","d3" (matches sb.sh: sed 's/ /","/g' wrapping).
//
// strconv.Quote is used so each domain is rendered as a Go/JSON-style double-
// quoted literal, which is also the desired JSON array element format. Any
// surprise character would have been rejected by validateDomain.
func buildSedPayload(domains []string) string {
	if len(domains) == 0 {
		return strconv.Quote(emptyMarker)
	}
	quoted := make([]string, len(domains))
	for i, d := range domains {
		quoted[i] = strconv.Quote(d)
	}
	return strings.Join(quoted, ",")
}

// shellQuote returns s wrapped in single quotes, escaping any embedded single
// quote via the standard `'\”` trick. Used to safely embed arbitrary paths /
// numeric strings into the `sh -c '...'` step body. Domain payloads do not
// reach this function — they are validated to a strict ASCII subset upstream.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
