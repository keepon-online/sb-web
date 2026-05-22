package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"miaomiaowu/internal/acmemgr"
)

func TestSetSharedAcmeManager_Roundtrip(t *testing.T) {
	// Reset shared state before/after to avoid cross-test interference.
	t.Cleanup(func() {
		sharedAcmeManagerMu.Lock()
		sharedAcmeManager = nil
		sharedAcmeManagerErr = nil
		sharedAcmeManagerMu.Unlock()
	})

	m, err := acmemgr.NewManager(acmemgr.Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	SetSharedAcmeManager(m)

	got, err := sharedAcmeMgr()
	if err != nil {
		t.Fatalf("sharedAcmeMgr: %v", err)
	}
	if got != m {
		t.Error("sharedAcmeMgr should return the previously set manager")
	}
}

func TestAcmemgrStatusHandler_ReturnsSnapshot(t *testing.T) {
	t.Cleanup(func() {
		sharedAcmeManagerMu.Lock()
		sharedAcmeManager = nil
		sharedAcmeManagerErr = nil
		sharedAcmeManagerMu.Unlock()
	})

	dir := t.TempDir()
	m, err := acmemgr.NewManager(acmemgr.Config{
		CacheDir:     dir,
		AllowedHosts: []string{"example.com"},
		Email:        "ops@example.com",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	SetSharedAcmeManager(m)

	h := NewAcmemgrStatusHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/acmemgr/status", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body acmemgr.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.CacheDir != dir {
		t.Errorf("CacheDir = %q, want %q", body.CacheDir, dir)
	}
	if body.Email != "ops@example.com" {
		t.Errorf("Email = %q", body.Email)
	}
}

func TestAcmemgrStatusHandler_RejectsNonGET(t *testing.T) {
	h := NewAcmemgrStatusHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/acmemgr/status", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestAcmemgrRequestCertHandler_NotReadyReturns503(t *testing.T) {
	t.Cleanup(func() {
		sharedAcmeManagerMu.Lock()
		sharedAcmeManager = nil
		sharedAcmeManagerErr = nil
		sharedAcmeManagerMu.Unlock()
	})

	m, _ := acmemgr.NewManager(acmemgr.Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	SetSharedAcmeManager(m)

	body := strings.NewReader(`{"host":"example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/acmemgr/request-cert", body)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(body.Len())

	rec := httptest.NewRecorder()
	NewAcmemgrRequestCertHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (ErrNotReady)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hint") {
		t.Errorf("body missing hint: %s", rec.Body.String())
	}
}

func TestAcmemgrRequestCertHandler_RejectsNonPOST(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/acmemgr/request-cert", nil)
	NewAcmemgrRequestCertHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestAcmemgrRequestCertHandler_RejectsMalformedBody(t *testing.T) {
	body := strings.NewReader(`{not json`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/acmemgr/request-cert", body)
	req.ContentLength = int64(body.Len())
	rec := httptest.NewRecorder()
	NewAcmemgrRequestCertHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// NOTE: a lazy-fallback test was intentionally omitted because it constructs
// a Manager bound to DefaultCacheDir (/etc/s-box/acme-cache) which is not
// writable in the test sandbox. The fallback path is exercised in production
// only — SetSharedAcmeManager covers the deterministic configuration flow.

func TestSetSharedAcmeManager_Replacement(t *testing.T) {
	t.Cleanup(func() {
		sharedAcmeManagerMu.Lock()
		sharedAcmeManager = nil
		sharedAcmeManagerErr = nil
		sharedAcmeManagerOnce = sync.Once{}
		sharedAcmeManagerMu.Unlock()
	})
	m1, _ := acmemgr.NewManager(acmemgr.Config{CacheDir: t.TempDir(), AllowedHosts: []string{"a.com"}})
	m2, _ := acmemgr.NewManager(acmemgr.Config{CacheDir: t.TempDir(), AllowedHosts: []string{"b.com"}})
	SetSharedAcmeManager(m1)
	got, _ := sharedAcmeMgr()
	if got != m1 {
		t.Error("first set should win")
	}
	SetSharedAcmeManager(m2)
	got, _ = sharedAcmeMgr()
	if got != m2 {
		t.Error("replacement should take effect")
	}
	_ = context.Background()
}
