package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSingboxDeployHandlerPreparesAndSavesServerConfig(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("SINGBOX_BASE_DIR", baseDir)

	body := bytes.NewBufferString(`{
		"external_host": "203.0.113.10",
		"hostname": "local-test",
		"reality_sni": "apple.com",
		"config_name": "sb.json"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/singbox/deploy", body)
	rec := httptest.NewRecorder()

	NewSingboxDeployHandler(nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Success           bool              `json:"success"`
		ConfigPath        string            `json:"config_path"`
		Links             map[string]string `json:"links"`
		UUID              string            `json:"uuid"`
		Password          string            `json:"password"`
		RealityPrivateKey string            `json:"reality_private_key"`
		RealityPublicKey  string            `json:"reality_public_key"`
		RealityShortID    string            `json:"reality_short_id"`
		CertificatePath   string            `json:"certificate_path"`
		PrivateKeyPath    string            `json:"private_key_path"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatal("success = false")
	}
	if response.ConfigPath != filepath.Join(baseDir, "sb.json") {
		t.Fatalf("ConfigPath = %q", response.ConfigPath)
	}
	if response.UUID == "" || response.Password == "" || response.RealityPrivateKey == "" || response.RealityPublicKey == "" || response.RealityShortID == "" {
		t.Fatalf("missing generated credentials: %#v", response)
	}
	for _, key := range []string{"vless", "vmess", "hysteria2", "tuic", "anytls"} {
		if response.Links[key] == "" {
			t.Fatalf("missing %s link", key)
		}
	}
}
