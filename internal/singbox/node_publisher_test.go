package singbox

import (
	"encoding/json"
	"testing"

	"miaomiaowu/internal/storage"
)

func TestBuildPublishedNodesWritesClashCompatibleVLESSConfig(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		Protocols:        []string{"vless"},
		ExternalHost:     "203.0.113.10",
		RealityPublicKey: "public-key",
		Enabled:          true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}

	var clash map[string]any
	if err := json.Unmarshal([]byte(nodes[0].ClashConfig), &clash); err != nil {
		t.Fatalf("unmarshal clash config: %v", err)
	}
	if clash["type"] != "vless" {
		t.Fatalf("type = %v, want vless", clash["type"])
	}
	if clash["uuid"] != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("uuid = %v", clash["uuid"])
	}
	if _, ok := clash["user"]; ok {
		t.Fatalf("clash config should not use share-link user field: %#v", clash)
	}
	if _, ok := clash["pbk"]; ok {
		t.Fatalf("clash config should not keep top-level pbk field: %#v", clash)
	}
	realityOpts, ok := clash["reality-opts"].(map[string]any)
	if !ok {
		t.Fatalf("missing reality-opts: %#v", clash)
	}
	if realityOpts["public-key"] != "public-key" {
		t.Fatalf("public-key = %v", realityOpts["public-key"])
	}
	if realityOpts["short-id"] != "abcd1234" {
		t.Fatalf("short-id = %v", realityOpts["short-id"])
	}
}

func TestBuildPublishedNodesDoesNotRequireRealityPublicKeyWhenVLESSNotSelected(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		Protocols:    []string{"hysteria2"},
		ExternalHost: "203.0.113.10",
		Enabled:      true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}
	if nodes[0].Protocol != "hysteria2" {
		t.Fatalf("protocol = %q", nodes[0].Protocol)
	}

	var clash map[string]any
	if err := json.Unmarshal([]byte(nodes[0].ClashConfig), &clash); err != nil {
		t.Fatalf("unmarshal clash config: %v", err)
	}
	if clash["password"] != "test-password" {
		t.Fatalf("password = %v", clash["password"])
	}
}

func TestBuildPublishedNodesVMessClashConfigShape(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		Protocols:        []string{"vmess"},
		ExternalHost:     "203.0.113.10",
		RealityPublicKey: "public-key",
		Enabled:          true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}

	var clash map[string]any
	if err := json.Unmarshal([]byte(nodes[0].ClashConfig), &clash); err != nil {
		t.Fatalf("unmarshal clash config: %v", err)
	}
	if clash["network"] != "ws" {
		t.Fatalf("network = %v, want ws", clash["network"])
	}
	wsOpts, ok := clash["ws-opts"].(map[string]any)
	if !ok {
		t.Fatalf("missing ws-opts: %#v", clash)
	}
	if wsOpts["path"] != "/vmessws" {
		t.Fatalf("ws-opts.path = %v", wsOpts["path"])
	}
	headers, ok := wsOpts["headers"].(map[string]any)
	if !ok {
		t.Fatalf("missing ws-opts.headers: %#v", wsOpts)
	}
	if headers["Host"] != "203.0.113.10" {
		t.Fatalf("ws-opts.headers.Host = %v", headers["Host"])
	}
}

func TestBuildPublishedNodesAcceptsHy2Alias(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		Protocols:    []string{"hy2"},
		ExternalHost: "203.0.113.10",
		Enabled:      true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Protocol != "hysteria2" {
		t.Fatalf("expected single hysteria2 node, got %+v", nodes)
	}
}

func TestBuildPublishedNodesPopulatesSourceMetadata(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		Protocols:        []string{"vless", "hysteria2"},
		ExternalHost:     "203.0.113.10",
		RealityPublicKey: "public-key",
		Tags:             []string{"custom"},
		Enabled:          true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("node count = %d, want 2", len(nodes))
	}
	for _, node := range nodes {
		if node.SourceType != publishedNodeSourceType {
			t.Fatalf("SourceType = %q, want %q", node.SourceType, publishedNodeSourceType)
		}
		if node.SourceRefID != "42" {
			t.Fatalf("SourceRefID = %q, want 42", node.SourceRefID)
		}
		if node.SourceRefName != "server" {
			t.Fatalf("SourceRefName = %q, want server", node.SourceRefName)
		}
		if node.OriginalServer != "203.0.113.10" {
			t.Fatalf("OriginalServer = %q, want 203.0.113.10", node.OriginalServer)
		}
		if node.NodeName != "server-"+node.Protocol {
			t.Fatalf("NodeName = %q, protocol %q", node.NodeName, node.Protocol)
		}
		seen := make(map[string]bool)
		for _, tag := range node.Tags {
			if seen[tag] {
				t.Fatalf("duplicate tag %q in %v", tag, node.Tags)
			}
			seen[tag] = true
		}
		if !seen["singbox"] || !seen["singbox:server"] || !seen["protocol:"+node.Protocol] || !seen["custom"] {
			t.Fatalf("missing required tag in %v", node.Tags)
		}
	}
}

func TestBuildPublishedNodesEmptyProtocolsPublishesAllAvailable(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		ExternalHost:     "203.0.113.10",
		RealityPublicKey: "public-key",
		Enabled:          true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}
	if len(nodes) != 5 {
		t.Fatalf("node count = %d, want 5 (vless+vmess+hysteria2+tuic+anytls)", len(nodes))
	}
	got := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		got[node.Protocol] = true
	}
	for _, want := range []string{"vless", "vmess", "hysteria2", "tuic", "anytls"} {
		if !got[want] {
			t.Fatalf("missing protocol %q in %v", want, got)
		}
	}
}

func TestBuildPublishedNodesClashConfigDoesNotShareSubMaps(t *testing.T) {
	config := testServerSingboxConfig(t)

	nodes, err := buildPublishedNodes(config, PublishRequest{
		Protocols:        []string{"vless"},
		ExternalHost:     "203.0.113.10",
		RealityPublicKey: "public-key",
		Enabled:          true,
	}, "admin")
	if err != nil {
		t.Fatalf("buildPublishedNodes returned error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(nodes[0].ParsedConfig), &parsed); err != nil {
		t.Fatalf("unmarshal parsed config: %v", err)
	}
	var clash map[string]any
	if err := json.Unmarshal([]byte(nodes[0].ClashConfig), &clash); err != nil {
		t.Fatalf("unmarshal clash config: %v", err)
	}
	if _, ok := clash["raw"]; ok {
		t.Fatalf("clash config should not include raw share link: %#v", clash)
	}
	if _, ok := parsed["raw"]; !ok {
		t.Fatalf("parsed config should include raw share link: %#v", parsed)
	}
}

func testServerSingboxConfig(t *testing.T) storage.SingboxConfig {
	t.Helper()
	cfg, err := BuildServerConfig(ServerConfigOptions{
		ExternalHost:       "203.0.113.10",
		Hostname:           "local-test",
		UUID:               "11111111-1111-4111-8111-111111111111",
		Password:           "test-password",
		RealitySNI:         "apple.com",
		RealityPrivateKey:  "private-key",
		RealityPublicKey:   "public-key",
		RealityShortID:     "abcd1234",
		WebSocketPath:      "/vmessws",
		CertificatePath:    "/tmp/cert.pem",
		PrivateKeyPath:     "/tmp/private.key",
		VlessRealityPort:   10001,
		VmessWebSocketPort: 10002,
		Hysteria2Port:      10003,
		TUICPort:           10004,
		AnyTLSPort:         10005,
	})
	if err != nil {
		t.Fatalf("BuildServerConfig returned error: %v", err)
	}
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return storage.SingboxConfig{
		ID:         42,
		Name:       "server",
		Protocol:   "server",
		Port:       10001,
		ConfigJSON: string(configJSON),
		Enabled:    true,
	}
}
