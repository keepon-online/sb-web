package warp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDecodeReserved_HappyPath(t *testing.T) {
	clientID := base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4, 5, 6})
	got, err := DecodeReserved(clientID)
	if err != nil {
		t.Fatalf("DecodeReserved: %v", err)
	}
	want := [3]int{1, 2, 3}
	if got != want {
		t.Errorf("Reserved = %v, want %v", got, want)
	}
}

func TestDecodeReserved_RejectsShortClientID(t *testing.T) {
	clientID := base64.StdEncoding.EncodeToString([]byte{1, 2})
	if _, err := DecodeReserved(clientID); err == nil {
		t.Error("expected error for client_id with <3 bytes after decode")
	}
}

func TestDecodeReserved_RejectsInvalidBase64(t *testing.T) {
	if _, err := DecodeReserved("not!base64$"); err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecodeReserved_MatchesSbShFallback(t *testing.T) {
	// sb.sh:3356 hardcodes res=[33,217,129]. Make sure DecodeReserved reads
	// 0x21,0xd9,0x81 the same way so any future refactor can't drift.
	clientID := base64.StdEncoding.EncodeToString([]byte{0x21, 0xd9, 0x81, 0xff})
	got, err := DecodeReserved(clientID)
	if err != nil {
		t.Fatalf("DecodeReserved: %v", err)
	}
	if got != [3]int{33, 217, 129} {
		t.Errorf("Reserved = %v, want [33 217 129]", got)
	}
}

func TestFallback_MatchesSbShValues(t *testing.T) {
	got := Fallback()
	if got.PrivateKey != FallbackPrivateKey {
		t.Errorf("PrivateKey = %q, want %q", got.PrivateKey, FallbackPrivateKey)
	}
	if got.IPv6 != FallbackV6 {
		t.Errorf("IPv6 = %q, want %q", got.IPv6, FallbackV6)
	}
	if got.Reserved != [3]int{33, 217, 129} {
		t.Errorf("Reserved = %v, want [33 217 129]", got.Reserved)
	}
}

func fakeCloudflareServer(t *testing.T, statusCode int, body string, captureBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("CF-Client-Version") != DefaultClientHeader {
			http.Error(w, "bad client header", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content-type", http.StatusBadRequest)
			return
		}
		if captureBody != nil {
			b, _ := io.ReadAll(r.Body)
			*captureBody = string(b)
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestRegister_HappyPath(t *testing.T) {
	clientID := base64.StdEncoding.EncodeToString([]byte{10, 20, 30})
	resp := map[string]any{
		"config": map[string]any{
			"interface": map[string]any{
				"addresses": map[string]any{
					"v4": "172.16.0.2/32",
					"v6": "2606:4700::abcd/128",
				},
			},
			"client_id": clientID,
		},
	}
	payload, _ := json.Marshal(resp)

	var capturedBody string
	srv := fakeCloudflareServer(t, http.StatusOK, string(payload), &capturedBody)
	defer srv.Close()

	got, err := registerWithURL(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("registerWithURL: %v", err)
	}
	if got.PrivateKey == "" || got.PublicKey == "" {
		t.Error("PrivateKey/PublicKey must be populated")
	}
	if got.IPv4 != "172.16.0.2" {
		t.Errorf("IPv4 = %q, want 172.16.0.2 (suffix stripped)", got.IPv4)
	}
	if got.IPv6 != "2606:4700::abcd" {
		t.Errorf("IPv6 = %q, want 2606:4700::abcd (suffix stripped)", got.IPv6)
	}
	if got.Reserved != [3]int{10, 20, 30} {
		t.Errorf("Reserved = %v, want [10 20 30]", got.Reserved)
	}
	if got.ClientID != clientID {
		t.Errorf("ClientID = %q, want %q", got.ClientID, clientID)
	}
	// Sanity: the body POSTed to Cloudflare must include the public key and
	// a parseable RFC 3339 timestamp.
	if !strings.Contains(capturedBody, `"key"`) {
		t.Errorf("captured body missing key field: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, `"tos"`) {
		t.Errorf("captured body missing tos timestamp: %s", capturedBody)
	}
}

func TestRegister_RejectsNon2xx(t *testing.T) {
	srv := fakeCloudflareServer(t, http.StatusForbidden, `{"error":"banned"}`, nil)
	defer srv.Close()
	_, err := registerWithURL(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected error for non-2xx")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should include status: %v", err)
	}
}

func TestRegister_RejectsEmptyClientID(t *testing.T) {
	payload := `{"config":{"interface":{"addresses":{"v6":"::1/128"}}, "client_id": ""}}`
	srv := fakeCloudflareServer(t, http.StatusOK, payload, nil)
	defer srv.Close()
	_, err := registerWithURL(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Error("expected error when client_id is empty")
	}
}

func TestRegister_RejectsMalformedResponse(t *testing.T) {
	srv := fakeCloudflareServer(t, http.StatusOK, `{not json`, nil)
	defer srv.Close()
	_, err := registerWithURL(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Error("expected error on malformed json")
	}
}

func TestRegister_RejectsShortClientID(t *testing.T) {
	bad := base64.StdEncoding.EncodeToString([]byte{1, 2}) // <3 bytes
	payload := `{"config":{"interface":{"addresses":{"v6":"::1/128"}}, "client_id": "` + bad + `"}}`
	srv := fakeCloudflareServer(t, http.StatusOK, payload, nil)
	defer srv.Close()
	_, err := registerWithURL(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Error("expected error when client_id has <3 bytes")
	}
}

func TestRegister_TransportFailurePropagates(t *testing.T) {
	srv := fakeCloudflareServer(t, http.StatusOK, "{}", nil)
	srv.Close() // close immediately so dial fails
	_, err := registerWithURL(context.Background(), &http.Client{Timeout: 1 * time.Second}, srv.URL)
	if err == nil {
		t.Error("expected transport error")
	}
}

func TestRegister_RespectsContextCancellation(t *testing.T) {
	// Server holds the request open long enough for the context to cancel.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := registerWithURL(ctx, srv.Client(), srv.URL)
	if err == nil {
		t.Error("expected context deadline error")
	}
}

func TestRegister_BodyTimestampIsRFC3339(t *testing.T) {
	var capturedBody string
	srv := fakeCloudflareServer(t, http.StatusOK, `{"config":{"interface":{"addresses":{"v6":"::1/128"}}, "client_id":"`+base64.StdEncoding.EncodeToString([]byte{1, 2, 3})+`"}}`, &capturedBody)
	defer srv.Close()

	if _, err := registerWithURL(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Fatalf("registerWithURL: %v", err)
	}
	var body struct {
		Tos string `json:"tos"`
	}
	if err := json.Unmarshal([]byte(capturedBody), &body); err != nil {
		t.Fatalf("captured body not JSON: %v / %s", err, capturedBody)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", body.Tos); err != nil {
		t.Errorf("tos %q not RFC 3339 with ms: %v", body.Tos, err)
	}
}

func TestRegister_HandlesAddressWithoutSuffix(t *testing.T) {
	payload := `{"config":{"interface":{"addresses":{"v6":"::cafe"}},"client_id":"` + base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) + `"}}`
	srv := fakeCloudflareServer(t, http.StatusOK, payload, nil)
	defer srv.Close()
	got, err := registerWithURL(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("registerWithURL: %v", err)
	}
	if got.IPv6 != "::cafe" {
		t.Errorf("IPv6 = %q, want ::cafe", got.IPv6)
	}
}

func TestRegister_NilClientUsesDefault(t *testing.T) {
	clientID := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	srv := fakeCloudflareServer(t, http.StatusOK, `{"config":{"interface":{"addresses":{"v6":"::1/128"}},"client_id":"`+clientID+`"}}`, nil)
	defer srv.Close()
	if _, err := registerWithURL(context.Background(), nil, srv.URL); err != nil {
		t.Errorf("nil client should fall back to default, got %v", err)
	}
}

// sanity guards
var _ = errors.New
