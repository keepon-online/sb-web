package routing

import (
	"strings"
	"testing"
)

func TestValidateDomain_AcceptsCommonNames(t *testing.T) {
	cases := []string{
		"netflix.com",
		"www.google.com",
		"openai",
		"geolocation-!cn",
		"a",
		"a.b.c.d.e.f.g.h.i",
		"A-Z0-9.test",
	}
	for _, c := range cases {
		if err := validateDomain(c); err != nil {
			t.Errorf("validateDomain(%q) unexpected error: %v", c, err)
		}
	}
}

func TestValidateDomain_RejectsInjectionVectors(t *testing.T) {
	rejects := map[string]string{
		"empty":              "",
		"single quote":       "a'b",
		"double quote":       `a"b`,
		"backslash":          `a\b`,
		"backtick":           "a`b",
		"dollar":             "a$b",
		"semicolon":          "a;b",
		"ampersand":          "a&b",
		"pipe":               "a|b",
		"redir-lt":           "a<b",
		"redir-gt":           "a>b",
		"newline":            "a\nb",
		"carriage return":    "a\rb",
		"tab":                "a\tb",
		"space":              "a b",
		"command substitute": "a$(whoami)",
		"reserved marker":    emptyMarker,
		"slash":              "a/b",
		"colon":              "a:b",
		"asterisk":           "a*b",
		"questionmark":       "a?b",
		"hash":               "a#b",
		"parens":             "a(b)",
		"non-ascii":          "你好.com",
	}
	for label, s := range rejects {
		if err := validateDomain(s); err == nil {
			t.Errorf("validateDomain(%q) [%s] should reject", s, label)
		}
	}
}

func TestValidateDomain_RejectsOverLength(t *testing.T) {
	long := strings.Repeat("a", 254)
	if err := validateDomain(long); err == nil {
		t.Error("expected reject for 254-byte domain")
	}
	exact := strings.Repeat("a", 253)
	if err := validateDomain(exact); err != nil {
		t.Errorf("253-byte domain should pass: %v", err)
	}
}

func TestBuildSedPayload(t *testing.T) {
	if got := buildSedPayload(nil); got != `"yg_kkk"` {
		t.Errorf("empty domains payload = %q, want %q", got, `"yg_kkk"`)
	}
	if got := buildSedPayload([]string{}); got != `"yg_kkk"` {
		t.Errorf("empty slice payload = %q, want %q", got, `"yg_kkk"`)
	}
	if got := buildSedPayload([]string{"netflix"}); got != `"netflix"` {
		t.Errorf("single payload = %q, want %q", got, `"netflix"`)
	}
	if got := buildSedPayload([]string{"netflix", "openai"}); got != `"netflix","openai"` {
		t.Errorf("multi payload = %q, want %q", got, `"netflix","openai"`)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":              `''`,
		"plain":         `'plain'`,
		"with space":    `'with space'`,
		"with 'quote'":  `'with '\''quote'\'''`,
		"/etc/foo.conf": `'/etc/foo.conf'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildUpdatePlan_RejectsInvalidChannel(t *testing.T) {
	for _, c := range []Channel{0, -1, 7, 99} {
		_, err := BuildUpdatePlan(UpdateRequest{Channel: c, Mode: ModeDomainSuffix})
		if err == nil {
			t.Errorf("channel %d should be rejected", c)
		}
	}
}

func TestBuildUpdatePlan_RejectsInvalidMode(t *testing.T) {
	for _, m := range []Mode{0, -1, 3, 99} {
		_, err := BuildUpdatePlan(UpdateRequest{Channel: ChannelWarpWireguardIPv4, Mode: m})
		if err == nil {
			t.Errorf("mode %d should be rejected", m)
		}
	}
}

func TestBuildUpdatePlan_RejectsInjectionInDomains(t *testing.T) {
	req := UpdateRequest{
		Channel: ChannelWarpWireguardIPv4,
		Mode:    ModeDomainSuffix,
		Domains: []string{"netflix", `evil"; rm -rf /`},
	}
	if _, err := BuildUpdatePlan(req); err == nil {
		t.Fatal("expected rejection of shell-meta injection vector")
	}
}

func TestBuildUpdatePlan_EmptyDomainsResetsToMarker(t *testing.T) {
	plan, err := BuildUpdatePlan(UpdateRequest{
		Channel: ChannelWarpWireguardIPv4,
		Mode:    ModeDomainSuffix,
		Domains: nil,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	payload := plan.Steps[1].Metadata["payload"]
	if payload != `"yg_kkk"` {
		t.Errorf("payload = %q, want %q", payload, `"yg_kkk"`)
	}
}

func TestBuildUpdatePlan_AllChannelModeCombinationsHaveEdits(t *testing.T) {
	channels := []Channel{
		ChannelWarpWireguardIPv4, ChannelWarpWireguardIPv6,
		ChannelWarpSocks5IPv4, ChannelWarpSocks5IPv6,
		ChannelVPSLocalIPv4, ChannelVPSLocalIPv6,
	}
	modes := []Mode{ModeDomainSuffix, ModeGeoSite}
	for _, c := range channels {
		for _, m := range modes {
			req := UpdateRequest{Channel: c, Mode: m, Domains: []string{"openai"}}
			plan, err := BuildUpdatePlan(req)
			if err != nil {
				t.Errorf("channel %d mode %d: unexpected err: %v", c, m, err)
				continue
			}
			if len(plan.Steps) != 3 {
				t.Errorf("channel %d mode %d: want 3 steps, got %d", c, m, len(plan.Steps))
			}
			if !plan.RollbackOnFailure {
				t.Errorf("channel %d mode %d: RollbackOnFailure must be true", c, m)
			}
			// step1 + step2 must have RollbackCommand; step3 must not (Sprint 3 refactor).
			if plan.Steps[0].RollbackCommand == "" {
				t.Errorf("channel %d mode %d: step1 missing rollback", c, m)
			}
			if plan.Steps[1].RollbackCommand == "" {
				t.Errorf("channel %d mode %d: step2 missing rollback", c, m)
			}
			if plan.Steps[2].RollbackCommand != "" {
				t.Errorf("channel %d mode %d: step3 should NOT have placeholder rollback (Sprint 3 cleanup)", c, m)
			}
		}
	}
}

func TestBuildUpdatePlan_StepCommandsAreShelled(t *testing.T) {
	plan, err := BuildUpdatePlan(UpdateRequest{
		Channel: ChannelWarpWireguardIPv4,
		Mode:    ModeDomainSuffix,
		Domains: []string{"netflix"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if plan.Steps[0].Command != "sh" {
		t.Errorf("step1 command = %q, want sh", plan.Steps[0].Command)
	}
	if plan.Steps[1].Command != "sh" {
		t.Errorf("step2 command = %q, want sh", plan.Steps[1].Command)
	}
	if plan.Steps[2].Command != "systemctl" {
		t.Errorf("step3 command = %q, want systemctl", plan.Steps[2].Command)
	}
	// sed payload must be reachable in step2 raw_command, single-quoted, and reference the right line.
	raw := plan.Steps[1].Metadata["raw_command"]
	if !strings.Contains(raw, "sed -i") || !strings.Contains(raw, "184s/") {
		t.Errorf("step2 raw_command missing expected sed/line ref: %s", raw)
	}
	if !strings.Contains(raw, `"netflix"`) {
		t.Errorf("step2 raw_command missing quoted domain: %s", raw)
	}
}

func TestBuildUpdatePlan_WireguardIPv6DomainSuffixTouchesAllThreeFiles(t *testing.T) {
	plan, err := BuildUpdatePlan(UpdateRequest{
		Channel: ChannelWarpWireguardIPv6,
		Mode:    ModeDomainSuffix,
		Domains: []string{"openai"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	target := plan.Steps[0].Target
	for _, want := range []string{SBJSONPath, SB10Path, SB11Path} {
		if !strings.Contains(target, want) {
			t.Errorf("backup target %q missing %s", target, want)
		}
	}
}
