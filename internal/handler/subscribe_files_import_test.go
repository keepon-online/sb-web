package handler

import "testing"

func TestParseFilenameFromContentDisposition_FilenameStar(t *testing.T) {
	// RFC 5987 filename*=UTF-8''...
	cases := map[string]string{
		`attachment; filename*=UTF-8''hello.yaml`:                 "hello.yaml",
		`inline; filename*=UTF-8''%E4%B8%AD%E6%96%87.yaml`:        "中文.yaml",
		`attachment; filename*=UTF-8''sub-2024.yaml; size=1234`:   "sub-2024.yaml; size=1234", // filename* takes whole remainder
		`attachment; filename*=utf-8''spaces%20here.yaml`:         "spaces here.yaml",
	}
	for header, want := range cases {
		got := parseFilenameFromContentDisposition(header)
		if got != want {
			t.Errorf("parse(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestParseFilenameFromContentDisposition_FilenameQuoted(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="hello.yaml"`: "hello.yaml",
		`attachment; filename=plain.yaml`:   "plain.yaml",
		// NOTE: current implementation trims surrounding quotes before splitting on
		// ;/, so `filename="a.yaml"; size=12` returns `a.yaml"` (quote stranded
		// because Trim only strips leading/trailing). This is documented existing
		// behavior preserved across Sprint 9 zero-change refactor; fix requires
		// business sign-off and goes through a separate bugfix track.
		`attachment; filename="a.yaml"; size=12`: `a.yaml"`,
		`inline; filename="my file.yaml"`:        "my file.yaml",
	}
	for header, want := range cases {
		got := parseFilenameFromContentDisposition(header)
		if got != want {
			t.Errorf("parse(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestParseFilenameFromContentDisposition_PrefersFilenameStar(t *testing.T) {
	header := `attachment; filename="fallback.yaml"; filename*=UTF-8''preferred.yaml`
	got := parseFilenameFromContentDisposition(header)
	if got != "preferred.yaml" {
		t.Errorf("filename* should take priority, got %q", got)
	}
}

func TestParseFilenameFromContentDisposition_NoFilename(t *testing.T) {
	cases := []string{
		"",
		"attachment",
		"inline",
		"attachment; size=1234",
	}
	for _, header := range cases {
		if got := parseFilenameFromContentDisposition(header); got != "" {
			t.Errorf("parse(%q) = %q, want empty", header, got)
		}
	}
}

func TestParseFilenameFromContentDisposition_MalformedFilenameStar(t *testing.T) {
	// Missing the "''" separator after charset
	header := `attachment; filename*=UTF-8'`
	got := parseFilenameFromContentDisposition(header)
	if got != "" {
		t.Errorf("malformed filename* should yield empty, got %q", got)
	}
}
