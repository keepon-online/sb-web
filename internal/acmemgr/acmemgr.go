// Package acmemgr is the Sprint 11.P1 platform abstraction for ACME (Let's
// Encrypt) certificate management. It wraps golang.org/x/crypto/acme/autocert
// so the rest of the codebase can request domain certificates without each
// caller needing to know about HTTP-01 challenges, cache directories, or
// renewal cron.
//
// Sprint 11 deliverable: skeleton + platform abstraction. The actual port-80
// challenge listener is wired up in Sprint 12 (separated to keep the change
// boundary auditable). Until then Manager.RequestCertificate returns the
// stale-cache or absent error so the upgrade path stays explicit.
//
// Business reference: replaces the `bash <(curl ... acme.sh)` shell-script
// fallback in sb.sh:306 with a Go-native pipeline that lives inside the
// process boundary.
package acmemgr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// Defaults mirror sb.sh's ygkkkca naming so a host that previously ran the
// upstream script can be reused without moving files.
const (
	DefaultCacheDir = "/etc/s-box/acme-cache"
	DefaultEmail    = ""

	// EnvCacheDir overrides Config.CacheDir at LoadFromEnv() time.
	EnvCacheDir = "ACME_CACHE_DIR"
	// EnvEmail overrides Config.Email.
	EnvEmail = "ACME_EMAIL"
	// EnvAllowedHosts is a comma-separated host whitelist.
	EnvAllowedHosts = "ACME_ALLOWED_HOSTS"
	// EnvListenAddr, when set, makes the bootstrap helper start the HTTP-01
	// challenge listener on that address (typical production value: ":80").
	EnvListenAddr = "ACME_LISTEN_ADDR"

	// HostPattern restricts the hostname accepted by the manager. Strict
	// RFC 1035 label syntax — rejected characters cannot leak into autocert's
	// HostPolicy check or the on-disk cache filename.
	hostnameMaxLength = 253
)

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.\-]*[a-zA-Z0-9])?$`)

// ErrNotReady is returned by RequestCertificate when the Sprint 12 challenge
// listener has not been started yet. Callers should fall back to the existing
// self-signed flow (certmgr.BuildSelfSignPlan).
var ErrNotReady = errors.New("acmemgr: challenge listener not started (Sprint 12 wires this up)")

// ValidateHostname enforces a strict allow-list before any hostname reaches
// the autocert layer. Rejection here is the final word.
func ValidateHostname(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("hostname is required")
	}
	if len(host) > hostnameMaxLength {
		return fmt.Errorf("hostname too long: %d bytes (max %d)", len(host), hostnameMaxLength)
	}
	if !hostnamePattern.MatchString(host) {
		return fmt.Errorf("hostname %q contains invalid characters", host)
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 {
			return fmt.Errorf("hostname has empty label")
		}
		if len(label) > 63 {
			return fmt.Errorf("hostname label %q exceeds 63 bytes", label)
		}
		// RFC 1035 §2.3.1: labels must start and end with alnum, not hyphen.
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("hostname label %q has leading or trailing hyphen", label)
		}
	}
	return nil
}

// Config controls Manager construction.
//
// CacheDir is created with 0700 if absent (private keys land there).
// Email is forwarded to the ACME provider for renewal notifications.
// AllowedHosts is the strict HostPolicy whitelist; empty rejects everything.
//
// ListenAddr is consumed only by BootstrapFromEnv / Bootstrap helpers; the
// raw NewManager constructor ignores it. Empty means "do not start the
// challenge listener" — the manager stays in not-ready state and request-cert
// returns ErrNotReady.
type Config struct {
	CacheDir     string
	Email        string
	AllowedHosts []string
	ListenAddr   string
}

// LoadConfigFromEnv reads the ACME_* environment variables and returns a
// Config. Unset variables fall back to defaults. Hostnames are split on comma
// and trimmed; validation happens inside NewManager so callers see a single
// chokepoint for rejection.
func LoadConfigFromEnv(getenv func(string) string) Config {
	if getenv == nil {
		getenv = osGetenv
	}

	cfg := Config{
		CacheDir:   strings.TrimSpace(getenv(EnvCacheDir)),
		Email:      strings.TrimSpace(getenv(EnvEmail)),
		ListenAddr: strings.TrimSpace(getenv(EnvListenAddr)),
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = DefaultCacheDir
	}
	if raw := strings.TrimSpace(getenv(EnvAllowedHosts)); raw != "" {
		for _, h := range strings.Split(raw, ",") {
			h = strings.TrimSpace(h)
			if h != "" {
				cfg.AllowedHosts = append(cfg.AllowedHosts, h)
			}
		}
	}
	return cfg
}

// Bootstrap constructs the Manager and — if cfg.ListenAddr is non-empty —
// starts the HTTP-01 challenge listener bound to ctx. The returned Manager
// is ready for use by handlers immediately; ctx cancellation closes the
// listener. listenFunc is the seam for tests; pass nil for production
// (net.Listen).
func Bootstrap(ctx context.Context, cfg Config, listenFunc func(network, address string) (net.Listener, error)) (*Manager, error) {
	m, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.ListenAddr == "" {
		return m, nil
	}
	if err := m.StartChallengeServer(ctx, cfg.ListenAddr, listenFunc); err != nil {
		return nil, fmt.Errorf("acmemgr: start challenge listener on %s: %w", cfg.ListenAddr, err)
	}
	return m, nil
}

// osGetenv is the indirection used by LoadConfigFromEnv so tests can inject a
// fake without touching real env vars.
var osGetenv = os.Getenv

// Manager wraps an autocert.Manager. The wrapper exists for two reasons:
//
//  1. Sprint 11 only needs to expose status + a graceful "not ready" error;
//     Sprint 12 will plug in the port-80 listener and challenge solver.
//  2. The hostname validation runs before any value reaches autocert, so an
//     injection attempt cannot influence the cache directory layout.
type Manager struct {
	cacheDir     string
	email        string
	allowedHosts map[string]struct{}
	autocert     *autocert.Manager

	mu                sync.RWMutex
	ready             bool // toggled true once StartChallengeServer succeeds
	started           time.Time
	challengeServer   *http.Server
	challengeListener net.Listener
}

// NewManager constructs a Manager and prepares the on-disk cache directory.
// Hostnames are validated; invalid entries cause construction to fail.
func NewManager(cfg Config) (*Manager, error) {
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = DefaultCacheDir
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("acmemgr: ensure cache dir %s: %w", cacheDir, err)
	}

	if cfg.Email != "" {
		if !strings.Contains(cfg.Email, "@") {
			return nil, fmt.Errorf("acmemgr: email %q missing @", cfg.Email)
		}
	}

	allowed := make(map[string]struct{}, len(cfg.AllowedHosts))
	for _, h := range cfg.AllowedHosts {
		if err := ValidateHostname(h); err != nil {
			return nil, fmt.Errorf("acmemgr: allowed_hosts: %w", err)
		}
		allowed[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	m := &Manager{
		cacheDir:     cacheDir,
		email:        cfg.Email,
		allowedHosts: allowed,
	}

	m.autocert = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cacheDir),
		Email:      cfg.Email,
		HostPolicy: m.hostPolicy,
	}
	return m, nil
}

// hostPolicy implements autocert.HostPolicy — only hosts in the whitelist
// can request certificates. Defense in depth: callers also pass the host
// through ValidateHostname before reaching this layer.
func (m *Manager) hostPolicy(ctx context.Context, host string) error {
	if err := ValidateHostname(host); err != nil {
		return err
	}
	_, ok := m.allowedHosts[strings.ToLower(host)]
	if !ok {
		return fmt.Errorf("host %q not in acmemgr allow-list", host)
	}
	return nil
}

// MarkReady is invoked by Sprint 12 once the port-80 challenge listener has
// been bound. Until that toggle flips, RequestCertificate refuses to start
// the ACME handshake.
func (m *Manager) MarkReady() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ready = true
	m.started = time.Now()
}

// Status describes the current readiness for the Web UI.
type Status struct {
	Ready         bool          `json:"ready"`
	Started       time.Time     `json:"started,omitempty"`
	CacheDir      string        `json:"cache_dir"`
	Email         string        `json:"email,omitempty"`
	AllowedHosts  []string      `json:"allowed_hosts"`
	CachedDomains []string      `json:"cached_domains,omitempty"`
	Certificates  []Certificate `json:"certificates,omitempty"`
}

// Certificate is the Sprint 15 expiry-monitoring projection. NotAfter is
// surfaced so Web UI can warn operators when renewal is overdue (autocert
// renews automatically at ~30 days, but visibility is still required).
type Certificate struct {
	Domain   string    `json:"domain"`
	NotAfter time.Time `json:"not_after"`
	DaysLeft int       `json:"days_left"`
	Issuer   string    `json:"issuer,omitempty"`
	Expired  bool      `json:"expired"`
	Expiring bool      `json:"expiring"` // < ExpiringSoonDays
	ParseErr string    `json:"parse_error,omitempty"`
}

// ExpiringSoonDays is the threshold below which a cert is flagged as
// "expiring". 30 days matches the autocert default renewal window.
const ExpiringSoonDays = 30

// Snapshot returns the current Status. Cached domain enumeration is best-
// effort — directory read errors are swallowed (an empty list is informative
// on its own).
func (m *Manager) Snapshot() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := Status{
		Ready:    m.ready,
		Started:  m.started,
		CacheDir: m.cacheDir,
		Email:    m.email,
	}
	for h := range m.allowedHosts {
		out.AllowedHosts = append(out.AllowedHosts, h)
	}

	entries, err := os.ReadDir(m.cacheDir)
	if err == nil {
		now := time.Now()
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), "acme_account") {
				continue
			}
			// autocert stores one cert per file named after the domain; skip
			// the +token / +http-01 suffixed work-in-progress files.
			name := e.Name()
			if strings.ContainsAny(name, "+ ") {
				continue
			}
			out.CachedDomains = append(out.CachedDomains, name)
			out.Certificates = append(out.Certificates, parseCachedCert(filepath.Join(m.cacheDir, name), name, now))
		}
	}
	return out
}

// parseCachedCert reads a single autocert cache entry and projects its leaf
// certificate into a Certificate summary. Parse errors do not abort the
// snapshot — the entry is still returned with ParseErr set so the operator
// can investigate via the UI.
func parseCachedCert(path, domain string, now time.Time) Certificate {
	c := Certificate{Domain: domain}
	data, err := os.ReadFile(path)
	if err != nil {
		c.ParseErr = err.Error()
		return c
	}
	// autocert writes "ec private key" PEM block followed by certificate PEM
	// block(s). Walk the chain and pick the first CERTIFICATE.
	var block *pem.Block
	rest := data
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			c.ParseErr = "no CERTIFICATE PEM block found"
			return c
		}
		if block.Type == "CERTIFICATE" {
			break
		}
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		c.ParseErr = err.Error()
		return c
	}
	c.NotAfter = leaf.NotAfter
	c.Issuer = leaf.Issuer.CommonName
	c.DaysLeft = int(leaf.NotAfter.Sub(now).Hours() / 24)
	c.Expired = !leaf.NotAfter.After(now)
	c.Expiring = !c.Expired && c.DaysLeft < ExpiringSoonDays
	return c
}

// HTTPChallengeHandler returns the http.Handler that solves HTTP-01
// challenges for the underlying autocert manager. Sprint 11 阶段 2 mounts
// this at the configured challenge address; Sprint 12 will optionally take
// it over for a co-tenant HTTP-01/HTTPS server.
func (m *Manager) HTTPChallengeHandler(fallback http.Handler) http.Handler {
	return m.autocert.HTTPHandler(fallback)
}

// StartChallengeServer binds an HTTP-01 challenge listener on addr (typical
// production value: ":80") and starts serving the autocert challenge handler.
// The function returns once the listener is up, leaving the serve loop on a
// background goroutine; cancel ctx to shut it down.
//
// listenFunc is the seam: production passes net.Listen; tests can pass a
// fake that returns a Pipe-backed listener so the manager can be exercised
// without privileged ports.
//
// On successful bind MarkReady() is invoked. Subsequent calls return an
// error — only one listener is supported per manager.
func (m *Manager) StartChallengeServer(ctx context.Context, addr string, listenFunc func(network, address string) (net.Listener, error)) error {
	m.mu.Lock()
	if m.ready {
		m.mu.Unlock()
		return fmt.Errorf("acmemgr: challenge server already started")
	}
	m.mu.Unlock()

	if listenFunc == nil {
		listenFunc = net.Listen
	}

	ln, err := listenFunc("tcp", addr)
	if err != nil {
		return fmt.Errorf("acmemgr: bind %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           m.HTTPChallengeHandler(http.NotFoundHandler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	m.mu.Lock()
	m.challengeServer = srv
	m.challengeListener = ln
	m.ready = true
	m.started = time.Now()
	m.mu.Unlock()

	go func() {
		_ = srv.Serve(ln)
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	return nil
}

// StopChallengeServer forcibly closes the listener and resets readiness. Used
// by tests to release a port between cases; production relies on ctx cancel.
func (m *Manager) StopChallengeServer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.challengeServer == nil {
		return nil
	}
	srv := m.challengeServer
	m.challengeServer = nil
	m.challengeListener = nil
	m.ready = false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// RequestCertificate is the public entry point for "fetch a TLS certificate
// for this hostname now". In Sprint 11 it returns ErrNotReady; Sprint 12
// replaces this with a real GetCertificate dance.
func (m *Manager) RequestCertificate(ctx context.Context, host string) (*tls.Certificate, error) {
	if err := ValidateHostname(host); err != nil {
		return nil, fmt.Errorf("acmemgr: %w", err)
	}
	m.mu.RLock()
	ready := m.ready
	m.mu.RUnlock()
	if !ready {
		return nil, ErrNotReady
	}
	hello := &tls.ClientHelloInfo{ServerName: host}
	return m.autocert.GetCertificate(hello)
}

// CacheFilePath returns the on-disk filename a host's certificate would land
// at — useful for tooling that wants to delete/inspect the artifact without
// importing autocert directly. The path is composed from validated input so
// callers cannot use it as a path-traversal vector.
func (m *Manager) CacheFilePath(host string) (string, error) {
	if err := ValidateHostname(host); err != nil {
		return "", err
	}
	return filepath.Join(m.cacheDir, strings.ToLower(host)), nil
}
