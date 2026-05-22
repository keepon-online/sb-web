package routing

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadCurrent_MissingFileReturnsError(t *testing.T) {
	_, err := ReadCurrent(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Error("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error should mention read failure: %v", err)
	}
}

func TestReadCurrent_RejectsCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	if _, err := ReadCurrent(path); err == nil {
		t.Error("expected error on broken JSON")
	}
}

func TestReadCurrent_EmptyRulesYieldsEmptyChannels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"route":{"rules":[]}}`), 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	got, err := ReadCurrent(path)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	for ch := ChannelWarpWireguardIPv4; ch <= ChannelVPSLocalIPv6; ch++ {
		state, ok := got[ch]
		if !ok {
			t.Errorf("channel %d missing", ch)
			continue
		}
		if len(state.DomainSuffix) != 0 {
			t.Errorf("channel %d DomainSuffix should be empty, got %v", ch, state.DomainSuffix)
		}
		if len(state.GeoSite) != 0 {
			t.Errorf("channel %d GeoSite should be empty, got %v", ch, state.GeoSite)
		}
	}
}

func TestReadCurrent_FullSixChannels(t *testing.T) {
	// Index 0 is the "default" route; rules[1..6] map to ChannelWarpWireguardIPv4..ChannelVPSLocalIPv6.
	body := `{
		"route": {
			"rules": [
				{"domain_suffix":["default.com"],"geosite":["default"]},
				{"domain_suffix":["netflix.com","openai.com"],"geosite":["geolocation-!cn"]},
				{"domain_suffix":["yg_kkk"],"geosite":["yg_kkk"]},
				{"domain_suffix":[],"geosite":["disney"]},
				{"domain_suffix":["www.google.com"],"geosite":[]},
				{"domain_suffix":["yg_kkk"],"geosite":[]},
				{"domain_suffix":[],"geosite":["netflix","yg_kkk"]}
			]
		}
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ReadCurrent(path)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if len(got) != 6 {
		t.Errorf("channel count = %d, want 6", len(got))
	}

	wantDomain := map[Channel][]string{
		ChannelWarpWireguardIPv4: {"netflix.com", "openai.com"},
		ChannelWarpWireguardIPv6: {},
		ChannelWarpSocks5IPv4:    {},
		ChannelWarpSocks5IPv6:    {"www.google.com"},
		ChannelVPSLocalIPv4:      {},
		ChannelVPSLocalIPv6:      {},
	}
	wantGeo := map[Channel][]string{
		ChannelWarpWireguardIPv4: {"geolocation-!cn"},
		ChannelWarpWireguardIPv6: {},
		ChannelWarpSocks5IPv4:    {"disney"},
		ChannelWarpSocks5IPv6:    {},
		ChannelVPSLocalIPv4:      {},
		ChannelVPSLocalIPv6:      {"netflix"},
	}

	for ch := ChannelWarpWireguardIPv4; ch <= ChannelVPSLocalIPv6; ch++ {
		state := got[ch]
		if !reflect.DeepEqual(state.DomainSuffix, wantDomain[ch]) {
			t.Errorf("channel %d DomainSuffix = %v, want %v", ch, state.DomainSuffix, wantDomain[ch])
		}
		if !reflect.DeepEqual(state.GeoSite, wantGeo[ch]) {
			t.Errorf("channel %d GeoSite = %v, want %v", ch, state.GeoSite, wantGeo[ch])
		}
	}
}

func TestReadCurrent_FiltersPlaceholderMarker(t *testing.T) {
	body := `{"route":{"rules":[
		{"domain_suffix":["x"],"geosite":[]},
		{"domain_suffix":["yg_kkk","real.com","yg_kkk"],"geosite":[]}
	]}}`
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadCurrent(path)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	state := got[ChannelWarpWireguardIPv4]
	if !reflect.DeepEqual(state.DomainSuffix, []string{"real.com"}) {
		t.Errorf("DomainSuffix = %v, want [real.com] (yg_kkk filtered)", state.DomainSuffix)
	}
}

func TestReadCurrent_DefaultPathFallback(t *testing.T) {
	// Empty string path triggers SBJSONPath default; we can't write to /etc
	// in tests so we just verify the failure path returns a useful error
	// rather than a panic.
	_, err := ReadCurrent("")
	if err == nil {
		t.Skip("/etc/s-box/sb.json exists on this host; skipping default-path negative test")
	}
	if !strings.Contains(err.Error(), SBJSONPath) {
		t.Errorf("error should mention default path %q, got %v", SBJSONPath, err)
	}
}

func TestReadCurrent_PartialRulesArrayHandledGracefully(t *testing.T) {
	body := `{"route":{"rules":[
		{"domain_suffix":["default"]},
		{"domain_suffix":["only1"]}
	]}}`
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadCurrent(path)
	if err != nil {
		t.Fatalf("ReadCurrent: %v", err)
	}
	if !reflect.DeepEqual(got[ChannelWarpWireguardIPv4].DomainSuffix, []string{"only1"}) {
		t.Errorf("channel 1 DomainSuffix = %v, want [only1]", got[ChannelWarpWireguardIPv4].DomainSuffix)
	}
	// Channels 2..6 should report empty without panicking.
	for ch := ChannelWarpWireguardIPv6; ch <= ChannelVPSLocalIPv6; ch++ {
		if len(got[ch].DomainSuffix) != 0 {
			t.Errorf("channel %d should be empty, got %v", ch, got[ch].DomainSuffix)
		}
	}
}

func TestFilterPlaceholder_EdgeCases(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want []string
	}{
		"nil":            {nil, []string{}},
		"empty":          {[]string{}, []string{}},
		"only marker":    {[]string{"yg_kkk"}, []string{}},
		"only marker x3": {[]string{"yg_kkk", "yg_kkk", "yg_kkk"}, []string{}},
		"no marker":      {[]string{"a", "b"}, []string{"a", "b"}},
		"mixed":          {[]string{"yg_kkk", "a", "yg_kkk", "b"}, []string{"a", "b"}},
	}
	for name, c := range cases {
		got := filterPlaceholder(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: filterPlaceholder(%v) = %v, want %v", name, c.in, got, c.want)
		}
	}
}
