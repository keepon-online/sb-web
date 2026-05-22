package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"miaomiaowu/internal/acmemgr"
	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
)

// sharedAcmeManager holds the process-wide acmemgr.Manager. Sprint 13
// switched ownership from this package to main.go: SetSharedAcmeManager is
// called once during boot with a Bootstrap()-built Manager. The lazy
// fallback survives for backwards compatibility with tests / earlier wiring
// — it constructs a permissive (no allow-list) Manager so the status
// endpoint never panics, but request-cert will reject every host.
var (
	sharedAcmeManagerOnce sync.Once
	sharedAcmeManager     *acmemgr.Manager
	sharedAcmeManagerErr  error
	sharedAcmeManagerMu   sync.RWMutex
)

// SetSharedAcmeManager wires the process-wide Manager. Called once from
// main.go after acmemgr.Bootstrap returns. Subsequent calls replace the
// pointer (useful for tests resetting between cases).
func SetSharedAcmeManager(m *acmemgr.Manager) {
	sharedAcmeManagerMu.Lock()
	defer sharedAcmeManagerMu.Unlock()
	sharedAcmeManager = m
	sharedAcmeManagerErr = nil
}

func sharedAcmeMgr() (*acmemgr.Manager, error) {
	sharedAcmeManagerMu.RLock()
	if sharedAcmeManager != nil {
		defer sharedAcmeManagerMu.RUnlock()
		return sharedAcmeManager, nil
	}
	sharedAcmeManagerMu.RUnlock()

	// Lazy fallback for callers that haven't run SetSharedAcmeManager (legacy
	// tests, code paths not yet integrated with the bootstrap flow).
	sharedAcmeManagerOnce.Do(func() {
		mgr, err := acmemgr.NewManager(acmemgr.Config{
			CacheDir:     acmemgr.DefaultCacheDir,
			AllowedHosts: []string{},
		})
		sharedAcmeManagerMu.Lock()
		defer sharedAcmeManagerMu.Unlock()
		if sharedAcmeManager == nil {
			sharedAcmeManager = mgr
			sharedAcmeManagerErr = err
		}
	})
	sharedAcmeManagerMu.RLock()
	defer sharedAcmeManagerMu.RUnlock()
	return sharedAcmeManager, sharedAcmeManagerErr
}

// acmemgrStatusHandler implements GET /api/admin/acmemgr/status.
//
// Read-only Sprint 11 surface: reports whether the challenge listener is up,
// what cache directory is in use, and which domains already have certs. The
// actual request-cert / start-listener endpoints land in Sprint 12.
type acmemgrStatusHandler struct{}

// NewAcmemgrStatusHandler returns the read-only status handler.
func NewAcmemgrStatusHandler() http.Handler {
	return &acmemgrStatusHandler{}
}

func (h *acmemgrStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	mgr, err := sharedAcmeMgr()
	if err != nil {
		logger.Warn("[acmemgr] init failed", "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[acmemgr] status read", "username", username)
	respondJSON(w, http.StatusOK, mgr.Snapshot())
}

// acmemgrRequestCertRequest is the wire body for POST /api/admin/acmemgr/request-cert.
type acmemgrRequestCertRequest struct {
	Host string `json:"host"`
}

// acmemgrRequestCertHandler triggers an ACME certificate request for the
// supplied host. Returns 503 with ErrNotReady when the challenge server has
// not been started — the operator should call /start-listener (Sprint 12+)
// or wire StartChallengeServer through deployment config.
type acmemgrRequestCertHandler struct{}

// NewAcmemgrRequestCertHandler returns the request-cert handler.
func NewAcmemgrRequestCertHandler() http.Handler {
	return &acmemgrRequestCertHandler{}
}

func (h *acmemgrRequestCertHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var body acmemgrRequestCertRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeBadRequest(w, "decode body: "+err.Error())
			return
		}
	}

	mgr, err := sharedAcmeMgr()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[acmemgr] request-cert invoked", "username", username, "host", body.Host)

	ctx, cancel := context.WithTimeout(r.Context(), 2*60_000_000_000) // 2 minutes
	defer cancel()

	cert, err := mgr.RequestCertificate(ctx, body.Host)
	if err != nil {
		if errors.Is(err, acmemgr.ErrNotReady) {
			respondJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": err.Error(),
				"hint":  "challenge listener not started; configure ACME_LISTEN_ADDR and restart, or call /api/admin/acmemgr/start (Sprint 12+)",
			})
			return
		}
		logger.Warn("[acmemgr] request-cert failed", "username", username, "host", body.Host, "error", err)
		respondJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	cachePath, _ := mgr.CacheFilePath(body.Host)
	respondJSON(w, http.StatusOK, map[string]any{
		"host":               body.Host,
		"cache_path":         cachePath,
		"leaf_serial":        cert.Leaf.SerialNumber.String(),
		"leaf_subject_cn":    cert.Leaf.Subject.CommonName,
		"leaf_not_after":     cert.Leaf.NotAfter,
		"intermediate_chain": len(cert.Certificate) - 1,
	})
}
