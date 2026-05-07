package singbox

import (
	"strings"
	"testing"
)

func TestGenerateShareLinksForServerConfig(t *testing.T) {
	opts := ServerConfigOptions{
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
	}

	links, err := GenerateShareLinks(opts)
	if err != nil {
		t.Fatalf("GenerateShareLinks returned error: %v", err)
	}

	wantKeys := []string{"vless", "vmess", "hysteria2", "tuic", "anytls"}
	for _, key := range wantKeys {
		if links[key] == "" {
			t.Fatalf("missing %s link", key)
		}
	}

	assertContains := func(key string, parts ...string) {
		t.Helper()
		for _, part := range parts {
			if !strings.Contains(links[key], part) {
				t.Fatalf("%s link %q does not contain %q", key, links[key], part)
			}
		}
	}

	assertContains("vless",
		"vless://11111111-1111-4111-8111-111111111111@203.0.113.10:10001",
		"security=reality",
		"flow=xtls-rprx-vision",
		"sni=apple.com",
		"pbk=public-key",
		"sid=abcd1234",
	)
	assertContains("vmess", "vmess://")
	assertContains("hysteria2",
		"hysteria2://test-password@203.0.113.10:10003",
		"security=tls",
		"alpn=h3",
		"sni=apple.com",
	)
	assertContains("tuic",
		"tuic://11111111-1111-4111-8111-111111111111:test-password@203.0.113.10:10004",
		"congestion_control=bbr",
		"udp_relay_mode=native",
		"alpn=h3",
	)
	assertContains("anytls",
		"anytls://test-password@203.0.113.10:10005",
		"sni=apple.com",
	)
}
