package singbox

import (
	"path/filepath"
	"testing"
)

func TestGetConfigPathsUsesLocalBaseDirOverride(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("SINGBOX_BASE_DIR", baseDir)

	paths := GetConfigPaths(EnvStandalone)

	if paths.BaseDir != baseDir {
		t.Fatalf("BaseDir = %q, want %q", paths.BaseDir, baseDir)
	}
	if paths.ConfigDir != baseDir {
		t.Fatalf("ConfigDir = %q, want %q", paths.ConfigDir, baseDir)
	}
	if paths.BinDir != filepath.Join(baseDir, "bin") {
		t.Fatalf("BinDir = %q", paths.BinDir)
	}
	if paths.ServiceDir != filepath.Join(baseDir, "service") {
		t.Fatalf("ServiceDir = %q", paths.ServiceDir)
	}
	if paths.LogDir != filepath.Join(baseDir, "logs") {
		t.Fatalf("LogDir = %q", paths.LogDir)
	}
	if paths.DataDir != filepath.Join(baseDir, "data") {
		t.Fatalf("DataDir = %q", paths.DataDir)
	}
}

func TestSaveConfigUsesLocalBaseDirOverride(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("SINGBOX_BASE_DIR", baseDir)

	generator := NewConfigGenerator()
	config, err := BuildServerConfig(ServerConfigOptions{
		ExternalHost:       "203.0.113.10",
		UUID:               "11111111-1111-4111-8111-111111111111",
		Password:           "test-password",
		RealitySNI:         "apple.com",
		RealityPrivateKey:  "private-key",
		RealityPublicKey:   "public-key",
		RealityShortID:     "abcd1234",
		CertificatePath:    filepath.Join(baseDir, "cert.pem"),
		PrivateKeyPath:     filepath.Join(baseDir, "private.key"),
		VlessRealityPort:   10001,
		VmessWebSocketPort: 10002,
		Hysteria2Port:      10003,
		TUICPort:           10004,
		AnyTLSPort:         10005,
	})
	if err != nil {
		t.Fatalf("BuildServerConfig returned error: %v", err)
	}

	if err := generator.SaveConfig(config, "local-server.json"); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	if _, err := generator.LoadConfig("local-server.json"); err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
}
