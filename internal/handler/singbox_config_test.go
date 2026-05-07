package handler

import "testing"

func TestGenerateConfigSupportsServerMultiProtocol(t *testing.T) {
	config, links, port, err := generateConfigWithLinks("server", map[string]interface{}{
		"external_host":        "203.0.113.10",
		"hostname":             "test-host",
		"uuid":                 "11111111-1111-4111-8111-111111111111",
		"password":             "test-password",
		"reality_sni":          "apple.com",
		"reality_private_key":  "private-key",
		"reality_public_key":   "public-key",
		"reality_short_id":     "abcd1234",
		"websocket_path":       "/vmessws",
		"certificate_path":     "/etc/s-box/cert.pem",
		"private_key_path":     "/etc/s-box/private.key",
		"vless_reality_port":   float64(10001),
		"vmess_websocket_port": float64(10002),
		"hysteria2_port":       float64(10003),
		"tuic_port":            float64(10004),
		"anytls_port":          float64(10005),
	})
	if err != nil {
		t.Fatalf("generateConfigWithLinks returned error: %v", err)
	}
	if len(config.Inbounds) != 5 {
		t.Fatalf("inbound count = %d, want 5", len(config.Inbounds))
	}
	if links["vless"] == "" || links["vmess"] == "" || links["hysteria2"] == "" || links["tuic"] == "" || links["anytls"] == "" {
		t.Fatalf("missing generated share links: %#v", links)
	}
	if port != 10001 {
		t.Fatalf("port = %d, want primary vless port 10001", port)
	}
}
