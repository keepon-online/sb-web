package upgrade

import (
	"strings"
	"testing"
)

func TestValidateVersion_AcceptsStableAndPrerelease(t *testing.T) {
	cases := []string{
		"1.10.7",
		"1.10.0",
		"1.13.0-alpha.1",
		"1.13.0-rc.2",
		"1.13.0-beta.3",
		"1.30.0",
		"1.30.99-alpha.99",
		"0.0.0",
		"99.99.99-rc.0",
	}
	for _, c := range cases {
		if err := ValidateVersion(c); err != nil {
			t.Errorf("ValidateVersion(%q) unexpected error: %v", c, err)
		}
	}
}

func TestValidateVersion_RejectsInjectionVectors(t *testing.T) {
	rejects := map[string]string{
		"empty":                 "",
		"path traversal ..":     "../etc/passwd",
		"dotted path traversal": "1.10../7",
		"shell semicolon":       "1.10.7;rm -rf /",
		"shell pipe":            "1.10.7|ls",
		"shell ampersand":       "1.10.7 & rm",
		"shell command sub":     "1.10.7$(whoami)",
		"shell backtick":        "1.10.7`whoami`",
		"shell dollar":          "1.10.7$x",
		"shell single quote":    "1.10.7'x",
		"shell double quote":    "1.10.7\"x",
		"shell backslash":       "1.10.7\\x",
		"newline":               "1.10.7\n",
		"carriage return":       "1.10.7\r",
		"tab":                   "1.10.7\t",
		"space":                 "1.10.7 ",
		"leading space":         " 1.10.7",
		"non-ascii":             "1.10.7é",
		"emoji":                 "1.10.7🔥",
		"chinese":               "一.二.三",
		"slash":                 "1.10/7",
		"only two parts":        "1.10",
		"trailing dot":          "1.10.7.",
		"missing patch":         "1.10.",
		"leading v":             "v1.10.7",
		"invalid suffix":        "1.13.0-snapshot.1",
		"missing suffix digit":  "1.13.0-alpha",
		"suffix non-numeric":    "1.13.0-alpha.x",
		"negative":              "-1.10.7",
		"plus operator":         "1.10.7+build",
		"oversized 33 bytes":    strings.Repeat("9", 33),
		"oversized 64 bytes":    strings.Repeat("9", 64),
	}
	for label, v := range rejects {
		if err := ValidateVersion(v); err == nil {
			t.Errorf("ValidateVersion(%q) [%s] should reject", v, label)
		}
	}
}

func TestValidateVersion_BoundaryLength(t *testing.T) {
	// At maxVersionLen (32 bytes) — content still has to match the regex,
	// otherwise the regex rejects regardless. Use a version-ish string padded
	// with patch digits to hit exactly 32 bytes.
	thirtyTwo := "1.999.99999999999999999999999999" // 32 bytes
	if len(thirtyTwo) != maxVersionLen {
		t.Fatalf("test fixture length = %d, expected %d", len(thirtyTwo), maxVersionLen)
	}
	if err := ValidateVersion(thirtyTwo); err != nil {
		t.Errorf("32-byte well-formed version should pass length gate, got: %v", err)
	}
	thirtyThree := thirtyTwo + "9"
	if err := ValidateVersion(thirtyThree); err == nil {
		t.Errorf("33-byte version should be rejected by length gate")
	}
}

func TestDetectMajor_HappyPaths(t *testing.T) {
	cases := map[string]string{
		"sing-box version 1.10.7":                                "1.10",
		"sing-box version 1.13.0-alpha.1":                        "1.13",
		"sing-box version 1.30.0":                                "1.30",
		"multi-line\nsing-box version 1.13.0-rc.2\ngoarch amd64": "1.13",
	}
	for input, want := range cases {
		if got := DetectMajor(input); got != want {
			t.Errorf("DetectMajor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDetectMajor_EmptyOrUnrecognised(t *testing.T) {
	cases := []string{
		"",
		"no version here",
		"version", // contains keyword but no numeric tail
		"\n\n",
	}
	for _, c := range cases {
		if got := DetectMajor(c); got != "" {
			t.Errorf("DetectMajor(%q) = %q, want empty", c, got)
		}
	}
}

func TestDetectMajor_FallbackScansEntireBlob(t *testing.T) {
	// No 'version' keyword line, but a bare X.Y still appears: fall through.
	in := "build info goes here 1.30.99 something"
	if got := DetectMajor(in); got != "1.30" {
		t.Errorf("fallback major = %q, want 1.30", got)
	}
}
