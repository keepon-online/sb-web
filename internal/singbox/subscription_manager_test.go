package singbox

import "testing"

func TestListSubscriptionsReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	manager := NewSubscriptionManager()
	manager.configDir = t.TempDir() + "/missing"

	subscriptions, err := manager.ListSubscriptions()
	if err != nil {
		t.Fatalf("ListSubscriptions returned error: %v", err)
	}
	if len(subscriptions) != 0 {
		t.Fatalf("subscriptions length = %d, want 0", len(subscriptions))
	}
}
