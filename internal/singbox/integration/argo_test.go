package integration

import "testing"

func TestListTunnelsReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	manager := NewArgoManager()
	manager.tunnelDir = t.TempDir() + "/missing"

	tunnels, err := manager.ListTunnels()
	if err != nil {
		t.Fatalf("ListTunnels returned error: %v", err)
	}
	if len(tunnels) != 0 {
		t.Fatalf("tunnels length = %d, want 0", len(tunnels))
	}
}
