package routing

import (
	"encoding/json"
	"fmt"
	"os"
)

// ChannelState reflects the routing rule entries currently configured for one
// channel (the rule index in sb.json's .route.rules[]).
type ChannelState struct {
	DomainSuffix []string `json:"domain_suffix"`
	GeoSite      []string `json:"geosite"`
}

// sbJSONShape captures the subset of sb.json we need to read.
type sbJSONShape struct {
	Route struct {
		Rules []struct {
			DomainSuffix []string `json:"domain_suffix"`
			GeoSite      []string `json:"geosite"`
		} `json:"rules"`
	} `json:"route"`
}

// ReadCurrent parses /etc/s-box/sb.json (or the supplied path) and returns the
// current state of all six channels. Mirrors sb.sh:3417-3524 sbymfl(), which
// reads .route.rules[1..6] but uses jq + sed for comment stripping. We use
// encoding/json directly, so the input file must be valid JSON (sb.sh applies
// `sed 's://.*::g'` to strip line comments; we therefore tolerate that).
//
// Per spec: rules[1..6] map to ChannelWarpWireguardIPv4..ChannelVPSLocalIPv6.
func ReadCurrent(sbJSONPath string) (map[Channel]ChannelState, error) {
	if sbJSONPath == "" {
		sbJSONPath = SBJSONPath
	}

	data, err := os.ReadFile(sbJSONPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sbJSONPath, err)
	}

	var shape sbJSONShape
	if err := json.Unmarshal(data, &shape); err != nil {
		return nil, fmt.Errorf("parse %s: %w", sbJSONPath, err)
	}

	result := make(map[Channel]ChannelState, 6)
	for ch := ChannelWarpWireguardIPv4; ch <= ChannelVPSLocalIPv6; ch++ {
		idx := int(ch) // rules[1] → channel 1, rules[2] → channel 2, ...
		state := ChannelState{
			DomainSuffix: []string{},
			GeoSite:      []string{},
		}
		if idx < len(shape.Route.Rules) {
			rule := shape.Route.Rules[idx]
			state.DomainSuffix = filterPlaceholder(rule.DomainSuffix)
			state.GeoSite = filterPlaceholder(rule.GeoSite)
		}
		result[ch] = state
	}
	return result, nil
}

// filterPlaceholder strips the "yg_kkk" marker (sb.sh's "未分流" sentinel) so
// the API surface returns a clean list to callers.
func filterPlaceholder(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == emptyMarker {
			continue
		}
		out = append(out, v)
	}
	return out
}
