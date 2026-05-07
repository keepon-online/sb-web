package singbox

import "testing"

func TestListShareConfigsReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	manager := NewShareManager()
	manager.configDir = t.TempDir() + "/missing"

	configs, err := manager.ListShareConfigs()
	if err != nil {
		t.Fatalf("ListShareConfigs returned error: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("configs length = %d, want 0", len(configs))
	}
}
