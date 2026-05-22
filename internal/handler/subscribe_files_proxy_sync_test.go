package handler

import (
	"reflect"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseYAML(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatalf("parse YAML: %v", err)
	}
	return &root
}

func TestCollectExistingProxyNodes_HappyPath(t *testing.T) {
	src := `
proxies:
  - name: hk-01
    type: vmess
  - name: jp-02
    type: trojan
  - name: us-03
    type: ss
proxy-groups: []
`
	root := parseYAML(t, src)
	got := collectExistingProxyNodes(root)
	sort.Strings(got)
	want := []string{"hk-01", "jp-02", "us-03"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCollectExistingProxyNodes_NoProxiesKey(t *testing.T) {
	src := `proxy-groups: []`
	root := parseYAML(t, src)
	got := collectExistingProxyNodes(root)
	if len(got) != 0 {
		t.Errorf("missing proxies key should yield empty, got %v", got)
	}
}

func TestCollectExistingProxyNodes_ProxiesNotSequence(t *testing.T) {
	src := `proxies: notanarray`
	root := parseYAML(t, src)
	got := collectExistingProxyNodes(root)
	if len(got) != 0 {
		t.Errorf("scalar proxies should yield empty, got %v", got)
	}
}

func TestCollectExistingProxyNodes_NodeWithoutName(t *testing.T) {
	src := `
proxies:
  - type: vmess
    server: 1.2.3.4
  - name: only-this
    type: ss
`
	root := parseYAML(t, src)
	got := collectExistingProxyNodes(root)
	if !reflect.DeepEqual(got, []string{"only-this"}) {
		t.Errorf("got %v, want [only-this]", got)
	}
}

func TestCollectUsedProviderNames_ClientModeWithUse(t *testing.T) {
	src := `
proxy-groups:
  - name: select-1
    type: select
    use:
      - prov-a
      - prov-b
  - name: select-2
    type: select
    use:
      - prov-b
      - prov-c
`
	root := parseYAML(t, src)
	got := collectUsedProviderNames(root)
	sort.Strings(got)
	want := []string{"prov-a", "prov-b", "prov-c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (dedup expected)", got, want)
	}
}

func TestCollectUsedProviderNames_MMWModeWithoutUse(t *testing.T) {
	src := `
proxy-groups:
  - name: hk-group
    type: select
  - name: jp-group
    type: url-test
`
	root := parseYAML(t, src)
	got := collectUsedProviderNames(root)
	sort.Strings(got)
	want := []string{"hk-group", "jp-group"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v (MMW mode uses name)", got, want)
	}
}

func TestCollectUsedProviderNames_MixedMode(t *testing.T) {
	src := `
proxy-groups:
  - name: with-use
    type: select
    use: [provider-a]
  - name: mmw-mode
    type: select
`
	root := parseYAML(t, src)
	got := collectUsedProviderNames(root)
	sort.Strings(got)
	// with-use's name not collected (has use); mmw-mode's name collected
	want := []string{"mmw-mode", "provider-a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCollectUsedProviderNames_NoProxyGroups(t *testing.T) {
	src := `proxies: []`
	root := parseYAML(t, src)
	got := collectUsedProviderNames(root)
	if len(got) != 0 {
		t.Errorf("missing proxy-groups should yield empty, got %v", got)
	}
}

func TestCopyMap_Independence(t *testing.T) {
	src := map[string]any{
		"a": 1,
		"b": "string",
		"c": map[string]any{"nested": 42},
		"d": []any{1, 2, 3},
	}
	dst := copyMap(src)

	// Verify equality
	if !reflect.DeepEqual(src, dst) {
		t.Errorf("copyMap not equal:\nsrc=%v\ndst=%v", src, dst)
	}

	// Mutate nested map in dst, src must not change
	dst["c"].(map[string]any)["nested"] = 999
	if src["c"].(map[string]any)["nested"] != 42 {
		t.Error("nested map mutation leaked back to src — deep copy broken")
	}

	// Add a new key to dst, src must not change
	dst["new"] = "added"
	if _, exists := src["new"]; exists {
		t.Error("top-level mutation leaked back to src")
	}
}

func TestCopyMap_EmptyAndNil(t *testing.T) {
	if got := copyMap(nil); got == nil {
		t.Error("nil input should yield non-nil empty map")
	}
	if got := copyMap(nil); len(got) != 0 {
		t.Errorf("nil input should yield empty map, got %v", got)
	}
	if got := copyMap(map[string]any{}); len(got) != 0 {
		t.Errorf("empty input len = %d", len(got))
	}
}

func TestCopyMap_SliceShallowCopy(t *testing.T) {
	src := map[string]any{"s": []any{1, 2, 3}}
	dst := copyMap(src)
	dstSlice := dst["s"].([]any)
	dstSlice[0] = 999
	srcSlice := src["s"].([]any)
	if srcSlice[0] == 999 {
		t.Error("slice shallow copy contract broken: dst mutation leaked to src")
	}
}
