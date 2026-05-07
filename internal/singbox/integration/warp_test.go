package integration

import "testing"

func TestGetWARPConfigsReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	manager := NewWARPManager()
	manager.configDir = t.TempDir() + "/missing"

	configs, err := manager.GetWARPConfigs()
	if err != nil {
		t.Fatalf("GetWARPConfigs returned error: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("configs length = %d, want 0", len(configs))
	}
}

func TestGetWARPStatusReturnsDisabledWhenDirectoryMissing(t *testing.T) {
	manager := NewWARPManager()
	manager.configDir = t.TempDir() + "/missing"

	status, err := manager.GetWARPStatus()
	if err != nil {
		t.Fatalf("GetWARPStatus returned error: %v", err)
	}
	if status.Enabled || status.Connected {
		t.Fatalf("status = %+v, want disabled and disconnected", status)
	}
}
