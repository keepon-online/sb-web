package storage

import (
	"context"
	"testing"
)

func TestCreateNodeRejectsDuplicateSourceIdentity(t *testing.T) {
	repo, err := NewTrafficRepository(":memory:")
	if err != nil {
		t.Fatalf("NewTrafficRepository() error = %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	node := Node{
		Username:      "admin",
		RawURL:        "vless://first@example.com:443#first",
		NodeName:      "first-node",
		Protocol:      "vless",
		ParsedConfig:  `{"server":"first.example.com"}`,
		ClashConfig:   `{"name":"first-node"}`,
		Enabled:       true,
		Tags:          []string{"singbox"},
		SourceType:    "singbox",
		SourceRefID:   "42",
		SourceRefName: "config",
	}
	if _, err := repo.CreateNode(ctx, node); err != nil {
		t.Fatalf("first CreateNode() error = %v", err)
	}
	node.NodeName = "duplicate-node"
	if _, err := repo.CreateNode(ctx, node); err == nil {
		t.Fatalf("second CreateNode() error = nil, want duplicate source identity error")
	}

	nodes, err := repo.ListNodesBySource(ctx, "singbox", "42")
	if err != nil {
		t.Fatalf("ListNodesBySource() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("ListNodesBySource() returned %d nodes, want 1", len(nodes))
	}
}

func TestUpsertNodeBySourceUpdatesExistingNode(t *testing.T) {
	repo, err := NewTrafficRepository(":memory:")
	if err != nil {
		t.Fatalf("NewTrafficRepository() error = %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	first, created, err := repo.UpsertNodeBySource(ctx, Node{
		Username:       "admin",
		RawURL:         "vless://first@example.com:443#first",
		NodeName:       "first-node",
		Protocol:       "VLESS",
		ParsedConfig:   `{"server":"first.example.com"}`,
		ClashConfig:    `{"name":"first-node"}`,
		Enabled:        true,
		Tags:           []string{"singbox", "protocol:vless"},
		OriginalServer: "first.example.com",
		SourceType:     "singbox",
		SourceRefID:    "42",
		SourceRefName:  "first-config",
	})
	if err != nil {
		t.Fatalf("first UpsertNodeBySource() error = %v", err)
	}
	if !created {
		t.Fatalf("first UpsertNodeBySource() created = false, want true")
	}

	second, created, err := repo.UpsertNodeBySource(ctx, Node{
		Username:       "admin",
		RawURL:         "vless://second@example.com:8443#second",
		NodeName:       "second-node",
		Protocol:       "vless",
		ParsedConfig:   `{"server":"second.example.com"}`,
		ClashConfig:    `{"name":"second-node"}`,
		Enabled:        false,
		Tags:           []string{"singbox", "updated"},
		OriginalServer: "second.example.com",
		SourceType:     "singbox",
		SourceRefID:    "42",
		SourceRefName:  "second-config",
	})
	if err != nil {
		t.Fatalf("second UpsertNodeBySource() error = %v", err)
	}
	if created {
		t.Fatalf("second UpsertNodeBySource() created = true, want false")
	}
	if second.ID != first.ID {
		t.Fatalf("second ID = %d, want original ID %d", second.ID, first.ID)
	}

	nodes, err := repo.ListNodesBySource(ctx, "singbox", "42")
	if err != nil {
		t.Fatalf("ListNodesBySource() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("ListNodesBySource() returned %d nodes, want 1", len(nodes))
	}
	stored := nodes[0]
	if stored.ID != first.ID {
		t.Fatalf("stored ID = %d, want %d", stored.ID, first.ID)
	}
	if stored.RawURL != "vless://second@example.com:8443#second" {
		t.Fatalf("stored RawURL = %q", stored.RawURL)
	}
	if stored.NodeName != "second-node" {
		t.Fatalf("stored NodeName = %q", stored.NodeName)
	}
	if stored.Enabled {
		t.Fatalf("stored Enabled = true, want false")
	}
	if stored.SourceRefName != "second-config" {
		t.Fatalf("stored SourceRefName = %q", stored.SourceRefName)
	}
	if len(stored.Tags) != 2 || stored.Tags[0] != "singbox" || stored.Tags[1] != "updated" {
		t.Fatalf("stored Tags = %#v", stored.Tags)
	}
}
