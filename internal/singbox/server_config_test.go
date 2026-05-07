package singbox

import (
	"encoding/json"
	"testing"
)

func TestBuildServerConfigIncludesFiveServerInbounds(t *testing.T) {
	cfg, err := BuildServerConfig(ServerConfigOptions{
		ExternalHost:       "203.0.113.10",
		Hostname:           "test-host",
		UUID:               "11111111-1111-4111-8111-111111111111",
		Password:           "test-password",
		RealitySNI:         "apple.com",
		RealityPrivateKey:  "private-key",
		RealityPublicKey:   "public-key",
		RealityShortID:     "abcd1234",
		WebSocketPath:      "/vmessws",
		CertificatePath:    "/etc/s-box/cert.pem",
		PrivateKeyPath:     "/etc/s-box/private.key",
		VlessRealityPort:   10001,
		VmessWebSocketPort: 10002,
		Hysteria2Port:      10003,
		TUICPort:           10004,
		AnyTLSPort:         10005,
	})
	if err != nil {
		t.Fatalf("BuildServerConfig returned error: %v", err)
	}

	if got := len(cfg.Inbounds); got != 5 {
		t.Fatalf("inbound count = %d, want 5", got)
	}

	byTag := map[string]InboundConfig{}
	for _, inbound := range cfg.Inbounds {
		byTag[inbound.Tag] = inbound
	}

	assertInbound := func(tag, typ string, port int) InboundConfig {
		t.Helper()
		inbound, ok := byTag[tag]
		if !ok {
			t.Fatalf("missing inbound tag %q", tag)
		}
		if inbound.Type != typ {
			t.Fatalf("%s type = %q, want %q", tag, inbound.Type, typ)
		}
		if inbound.ListenPort != port {
			t.Fatalf("%s listen_port = %d, want %d", tag, inbound.ListenPort, port)
		}
		return inbound
	}

	vless := assertInbound("vless-sb", "vless", 10001)
	vmess := assertInbound("vmess-ws-sb", "vmess", 10002)
	hy2 := assertInbound("hysteria2-sb", "hysteria2", 10003)
	tuic := assertInbound("tuic5-sb", "tuic", 10004)
	anytls := assertInbound("anytls-sb", "anytls", 10005)

	if vless.TLS == nil || vless.TLS.Reality == nil || !vless.TLS.Reality.Enabled {
		t.Fatalf("vless reality tls not configured: %#v", vless.TLS)
	}
	if vless.TLS.Reality.PublicKey != "public-key" {
		t.Fatalf("vless reality public key = %q", vless.TLS.Reality.PublicKey)
	}
	if vmess.Transport == nil || vmess.Transport.Type != "ws" || vmess.Transport.Path != "/vmessws" {
		t.Fatalf("vmess websocket transport not configured: %#v", vmess.Transport)
	}
	for _, inbound := range []InboundConfig{hy2, tuic, anytls} {
		if inbound.TLS == nil || !inbound.TLS.Enabled {
			t.Fatalf("%s tls not enabled", inbound.Tag)
		}
		if inbound.TLS.CertificatePath != "/etc/s-box/cert.pem" {
			t.Fatalf("%s cert path = %q", inbound.Tag, inbound.TLS.CertificatePath)
		}
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("generated config is not valid json")
	}
}

func TestBuildServerConfigRejectsDuplicatePorts(t *testing.T) {
	_, err := BuildServerConfig(ServerConfigOptions{
		ExternalHost:       "203.0.113.10",
		UUID:               "11111111-1111-4111-8111-111111111111",
		Password:           "test-password",
		RealitySNI:         "apple.com",
		RealityPrivateKey:  "private-key",
		RealityPublicKey:   "public-key",
		RealityShortID:     "abcd1234",
		CertificatePath:    "/etc/s-box/cert.pem",
		PrivateKeyPath:     "/etc/s-box/private.key",
		VlessRealityPort:   10001,
		VmessWebSocketPort: 10001,
		Hysteria2Port:      10003,
		TUICPort:           10004,
		AnyTLSPort:         10005,
	})
	if err == nil {
		t.Fatal("expected duplicate port error")
	}
}
