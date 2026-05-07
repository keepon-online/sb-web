package singbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareServerDeploymentGeneratesCredentialsConfigAndLinks(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("SINGBOX_BASE_DIR", baseDir)

	deployer := NewServerDeployer()
	result, err := deployer.Prepare(ServerDeployOptions{
		ExternalHost:       "203.0.113.10",
		Hostname:           "local-test",
		RealitySNI:         "apple.com",
		WebSocketPath:      "/vmessws",
		VlessRealityPort:   10001,
		VmessWebSocketPort: 10002,
		Hysteria2Port:      10003,
		TUICPort:           10004,
		AnyTLSPort:         10005,
		ConfigName:         "sb-server.json",
	})
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}

	if result.UUID == "" {
		t.Fatal("UUID was not generated")
	}
	if result.Password == "" {
		t.Fatal("password was not generated")
	}
	if result.RealityPrivateKey == "" || result.RealityPublicKey == "" {
		t.Fatalf("reality keypair was not generated: %#v", result)
	}
	if result.RealityShortID == "" {
		t.Fatal("short id was not generated")
	}
	if len(result.Config.Inbounds) != 5 {
		t.Fatalf("inbound count = %d, want 5", len(result.Config.Inbounds))
	}
	for _, key := range []string{"vless", "vmess", "hysteria2", "tuic", "anytls"} {
		if result.Links[key] == "" {
			t.Fatalf("missing %s link", key)
		}
	}
	if result.ConfigPath != filepath.Join(baseDir, "sb-server.json") {
		t.Fatalf("ConfigPath = %q", result.ConfigPath)
	}
	if _, err := os.Stat(result.ConfigPath); err != nil {
		t.Fatalf("saved config not found: %v", err)
	}
	if result.CertificatePath != filepath.Join(baseDir, "cert.pem") {
		t.Fatalf("CertificatePath = %q", result.CertificatePath)
	}
	if result.PrivateKeyPath != filepath.Join(baseDir, "private.key") {
		t.Fatalf("PrivateKeyPath = %q", result.PrivateKeyPath)
	}
	if _, err := os.Stat(result.CertificatePath); err != nil {
		t.Fatalf("certificate was not created: %v", err)
	}
	if _, err := os.Stat(result.PrivateKeyPath); err != nil {
		t.Fatalf("private key was not created: %v", err)
	}
}

func TestGenerateRealityKeyPairReturnsURLSafeKeys(t *testing.T) {
	privateKey, publicKey, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatalf("GenerateRealityKeyPair returned error: %v", err)
	}
	for label, value := range map[string]string{
		"private": privateKey,
		"public":  publicKey,
	} {
		if len(value) < 40 {
			t.Fatalf("%s key too short: %q", label, value)
		}
		for _, ch := range value {
			if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '-' && ch != '_' {
				t.Fatalf("%s key contains non URL-safe char %q in %q", label, ch, value)
			}
		}
	}
}
