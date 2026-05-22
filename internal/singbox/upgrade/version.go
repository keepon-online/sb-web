// Package upgrade encapsulates sb.sh's sing-box kernel upgrade flow
// (upsbcroe + restartsb at sb.sh:3833-3894 / 3769-3777) as systemops
// OperationPlan instances.
//
// Security posture: every user-supplied version string passes ValidateVersion
// before reaching the shell, the download URL template, or the archive
// directory name. The arch identifier is restricted to a whitelist before any
// shell interpolation happens (BuildUpgradePlan).
package upgrade

import (
	"fmt"
	"regexp"
	"strings"
)

// maxVersionLen caps user-supplied version strings before any character is
// inspected. The longest legitimate tag observed at the time of writing is
// "1.13.0-alpha.99" (15 bytes); 32 leaves headroom for future schemes without
// allowing pathological inputs.
const maxVersionLen = 32

// versionPattern enforces the strict format documented in sb.sh:3862-3863.
// Accepts:
//
//	1.10.7
//	1.13.0-alpha.1
//	1.13.0-rc.2
//	1.13.0-beta.3
//	1.30.0
//
// Rejects everything else (path traversal, shell metacharacters, control
// characters, non-ASCII, empty strings, oversized payloads).
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|rc|beta)\.[0-9]+)?$`)

// ValidateVersion is the single chokepoint every user-supplied version string
// must pass before it reaches the URL template or the shell.
//
// The function is intentionally strict: there is no "best effort" mode. A
// rejection here is the final word — callers must surface the error to the
// operator rather than fall back to a sanitized variant.
func ValidateVersion(v string) error {
	if v == "" {
		return fmt.Errorf("version is required")
	}
	if len(v) > maxVersionLen {
		return fmt.Errorf("version too long: %d bytes (max %d)", len(v), maxVersionLen)
	}
	// Pre-screen for non-printable / non-ASCII characters so the regex never
	// has to reason about them. This also makes the error message clearer for
	// the operator (e.g. trailing newline pasted from a webpage).
	for _, r := range v {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("version contains non-ASCII or control character at byte position")
		}
	}
	if strings.Contains(v, "..") {
		// Belt-and-braces against path traversal even though the regex would
		// already reject ".."; an explicit error is easier to read.
		return fmt.Errorf("version contains forbidden sequence %q", "..")
	}
	if !versionPattern.MatchString(v) {
		return fmt.Errorf("version %q does not match the required format", v)
	}
	return nil
}

// majorPattern extracts the leading "X.Y" prefix from a sing-box version output
// line. sb.sh:3879 uses `awk '/version/{print $NF}' | cut -d '.' -f 1,2`; this
// is the Go equivalent and tolerates the same upstream output variations.
var majorPattern = regexp.MustCompile(`([0-9]+\.[0-9]+)`)

// DetectMajor extracts the X.Y portion (e.g. "1.10", "1.30") from the output
// of `sing-box version`. Returns an empty string when no version line is
// present so callers can fall back to the sb11.json branch (the sb.sh
// `[[ "$sbnh" == "1.10" ]] && num=10 || num=11` default).
//
// Callers must not pass this result into shell interpolation without
// re-validation; it is consumed by Go logic to pick between sb10/sb11.
func DetectMajor(versionOutput string) string {
	for _, line := range strings.Split(versionOutput, "\n") {
		if !strings.Contains(line, "version") {
			continue
		}
		// Take the last whitespace-separated field, mirroring awk '$NF'.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		last := fields[len(fields)-1]
		if m := majorPattern.FindString(last); m != "" {
			return m
		}
	}
	// Fallback: scan the whole blob for the first X.Y token. Some builds emit
	// `sing-box version 1.10.7` without keyword separators.
	return majorPattern.FindString(versionOutput)
}
