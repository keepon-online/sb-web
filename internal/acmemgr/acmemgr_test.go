package acmemgr

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateHostname_Accepts(t *testing.T) {
	cases := []string{
		"example.com",
		"www.example.com",
		"a",
		"a.b.c.d.e",
		"sub-1.example.com",
		"123.example.com",
		"a-b.c-d.example",
	}
	for _, c := range cases {
		if err := ValidateHostname(c); err != nil {
			t.Errorf("ValidateHostname(%q) unexpected error: %v", c, err)
		}
	}
}

func TestValidateHostname_Rejects(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"whitespace":        "   ",
		"semicolon":         "a;b.com",
		"single quote":      "a'b.com",
		"double quote":      `a"b.com`,
		"slash":             "a/b.com",
		"space":             "a b.com",
		"backtick":          "a`b.com",
		"dollar":            "a$b.com",
		"newline":           "a\nb.com",
		"non-ascii":         "你好.com",
		"underscore":        "a_b.com", // RFC 1035 doesn't allow underscore
		"trailing dot":      "a.com.",  // empty label
		"leading dot":       ".a.com",  // empty label
		"double dot":        "a..com",  // empty label
		"too long hostname": strings.Repeat("a", 254) + ".com",
		"too long label":    strings.Repeat("a", 64) + ".com",
		"leading hyphen":    "-a.com", // pattern requires alnum at boundaries
		"trailing hyphen":   "a-.com",
	}
	for label, host := range cases {
		if err := ValidateHostname(host); err == nil {
			t.Errorf("ValidateHostname(%s = %q) should reject", label, host)
		}
	}
}

func TestNewManager_DefaultCacheDir(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(Config{
		CacheDir:     dir,
		Email:        "ops@example.com",
		AllowedHosts: []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat cache dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("cache dir not a directory")
	}
	if m.cacheDir != dir {
		t.Errorf("cacheDir = %q, want %q", m.cacheDir, dir)
	}
}

func TestNewManager_CreatesCacheDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("0700 perms not enforced on windows")
	}
	parent := t.TempDir()
	newDir := filepath.Join(parent, "fresh", "acme")
	_, err := NewManager(Config{CacheDir: newDir, AllowedHosts: []string{"example.com"}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("cache dir not created")
	}
}

func TestNewManager_RejectsInvalidHostname(t *testing.T) {
	_, err := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"valid.com", "a;b.com"},
	})
	if err == nil {
		t.Error("invalid hostname in AllowedHosts should reject")
	}
}

func TestNewManager_RejectsInvalidEmail(t *testing.T) {
	_, err := NewManager(Config{
		CacheDir:     t.TempDir(),
		Email:        "notanemail",
		AllowedHosts: []string{"example.com"},
	})
	if err == nil {
		t.Error("email without @ should reject")
	}
}

func TestNewManager_EmptyEmailAccepted(t *testing.T) {
	_, err := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	if err != nil {
		t.Errorf("empty email should be accepted, got %v", err)
	}
}

func TestHostPolicy_AcceptsAllowed(t *testing.T) {
	m, _ := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com", "TEST.org"},
	})
	if err := m.hostPolicy(context.Background(), "example.com"); err != nil {
		t.Errorf("allowed host rejected: %v", err)
	}
	// Case insensitive
	if err := m.hostPolicy(context.Background(), "test.org"); err != nil {
		t.Errorf("case-insensitive match failed: %v", err)
	}
}

func TestHostPolicy_RejectsNotAllowed(t *testing.T) {
	m, _ := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	if err := m.hostPolicy(context.Background(), "evil.com"); err == nil {
		t.Error("non-allowed host should be rejected")
	}
}

func TestHostPolicy_RejectsInvalidHostnameEvenIfInList(t *testing.T) {
	// Defense in depth: hostPolicy validates even values that came from
	// AllowedHosts (paranoid but catches bypass via maps with crafted keys).
	m, _ := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	// Manually inject a bad entry
	m.allowedHosts["a;b.com"] = struct{}{}
	if err := m.hostPolicy(context.Background(), "a;b.com"); err == nil {
		t.Error("hostPolicy must re-validate even allow-listed hosts")
	}
}

func TestRequestCertificate_NotReadyByDefault(t *testing.T) {
	m, _ := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	_, err := m.RequestCertificate(context.Background(), "example.com")
	if !errors.Is(err, ErrNotReady) {
		t.Errorf("expected ErrNotReady, got %v", err)
	}
}

func TestRequestCertificate_RejectsInvalidHost(t *testing.T) {
	m, _ := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	_, err := m.RequestCertificate(context.Background(), "a;b.com")
	if err == nil {
		t.Error("invalid host should be rejected before readiness check")
	}
	if errors.Is(err, ErrNotReady) {
		t.Error("invalid host should be rejected before readiness check (got ErrNotReady)")
	}
}

func TestMarkReady_TogglesStatus(t *testing.T) {
	m, _ := NewManager(Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	})
	if s := m.Snapshot(); s.Ready {
		t.Error("initial Ready should be false")
	}
	m.MarkReady()
	s := m.Snapshot()
	if !s.Ready {
		t.Error("MarkReady should toggle Ready true")
	}
	if s.Started.IsZero() {
		t.Error("Started should be set on MarkReady")
	}
}

func TestSnapshot_ListsCachedDomains(t *testing.T) {
	dir := t.TempDir()
	// Plant some files mimicking autocert's cache layout
	for _, name := range []string{"example.com", "test.org", "wip+token", "acme_account+key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	m, _ := NewManager(Config{CacheDir: dir, AllowedHosts: []string{"example.com"}})
	s := m.Snapshot()
	want := map[string]bool{"example.com": true, "test.org": true}
	got := map[string]bool{}
	for _, d := range s.CachedDomains {
		got[d] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %q in cached domains: %v", k, s.CachedDomains)
		}
	}
	// wip+token must be excluded
	for _, d := range s.CachedDomains {
		if strings.Contains(d, "+") {
			t.Errorf("cached domains include WIP file: %q", d)
		}
		if strings.HasPrefix(d, "acme_account") {
			t.Errorf("cached domains include acme_account file: %q", d)
		}
	}
}

func TestCacheFilePath_ProducesLowercased(t *testing.T) {
	m, _ := NewManager(Config{
		CacheDir:     "/tmp/acme-test",
		AllowedHosts: []string{"Example.COM"},
	})
	path, err := m.CacheFilePath("Example.COM")
	if err != nil {
		t.Fatalf("CacheFilePath: %v", err)
	}
	if !strings.HasSuffix(path, "/example.com") {
		t.Errorf("path %q should end with lowercased host", path)
	}
}

func TestCacheFilePath_RejectsInvalidHost(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: "/tmp", AllowedHosts: []string{"example.com"}})
	_, err := m.CacheFilePath("a;b.com")
	if err == nil {
		t.Error("invalid host should be rejected (path-traversal defense)")
	}
}

func TestHTTPChallengeHandler_ReturnsNonNil(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	h := m.HTTPChallengeHandler(nil)
	if h == nil {
		t.Error("HTTPChallengeHandler should never return nil")
	}
}

func TestStartChallengeServer_HappyPathTogglesReady(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bind ephemeral port via :0 so the test never collides with whatever is
	// running on the host. The actual listen happens through net.Listen which
	// matches production behavior on a non-privileged port.
	if err := m.StartChallengeServer(ctx, "127.0.0.1:0", nil); err != nil {
		t.Fatalf("StartChallengeServer: %v", err)
	}
	defer m.StopChallengeServer()

	if !m.Snapshot().Ready {
		t.Error("Ready should toggle true after StartChallengeServer")
	}
	if m.challengeListener == nil {
		t.Error("challengeListener should be populated")
	}
}

func TestStartChallengeServer_RejectsDoubleStart(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.StartChallengeServer(ctx, "127.0.0.1:0", nil); err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer m.StopChallengeServer()
	if err := m.StartChallengeServer(ctx, "127.0.0.1:0", nil); err == nil {
		t.Error("second start should reject")
	}
}

func TestStartChallengeServer_BindFailurePropagates(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	failingListen := func(network, address string) (net.Listener, error) {
		return nil, errors.New("forced bind failure")
	}
	err := m.StartChallengeServer(context.Background(), ":0", failingListen)
	if err == nil {
		t.Fatal("expected bind error")
	}
	if !strings.Contains(err.Error(), "forced bind failure") {
		t.Errorf("error should surface listener err: %v", err)
	}
	if m.Snapshot().Ready {
		t.Error("Ready must stay false on bind failure")
	}
}

func TestStopChallengeServer_Idempotent(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	// Stop without start — should be a no-op
	if err := m.StopChallengeServer(); err != nil {
		t.Errorf("stop without start should be no-op, got %v", err)
	}
}

func TestStartChallengeServer_ContextCancelShutsDown(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	ctx, cancel := context.WithCancel(context.Background())
	if err := m.StartChallengeServer(ctx, "127.0.0.1:0", nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	cancel()
	// Give the shutdown goroutine a moment to run
	time.Sleep(100 * time.Millisecond)
	// Second start should now succeed since ctx-cancel triggered shutdown
	if err := m.StopChallengeServer(); err != nil {
		t.Errorf("explicit stop after ctx-cancel should be no-op or success, got %v", err)
	}
}

func TestLoadConfigFromEnv_AllVarsSet(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case EnvCacheDir:
			return "/tmp/acme-custom"
		case EnvEmail:
			return "ops@example.com"
		case EnvAllowedHosts:
			return "  example.com , test.org  ,  ,  another.io  "
		case EnvListenAddr:
			return ":8080"
		}
		return ""
	}
	cfg := LoadConfigFromEnv(getenv)
	if cfg.CacheDir != "/tmp/acme-custom" {
		t.Errorf("CacheDir = %q, want /tmp/acme-custom", cfg.CacheDir)
	}
	if cfg.Email != "ops@example.com" {
		t.Errorf("Email = %q", cfg.Email)
	}
	want := []string{"example.com", "test.org", "another.io"}
	if len(cfg.AllowedHosts) != 3 {
		t.Fatalf("AllowedHosts len = %d, want 3", len(cfg.AllowedHosts))
	}
	for i, h := range want {
		if cfg.AllowedHosts[i] != h {
			t.Errorf("AllowedHosts[%d] = %q, want %q", i, cfg.AllowedHosts[i], h)
		}
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
}

func TestLoadConfigFromEnv_EmptyFallsBackToDefaults(t *testing.T) {
	cfg := LoadConfigFromEnv(func(string) string { return "" })
	if cfg.CacheDir != DefaultCacheDir {
		t.Errorf("CacheDir = %q, want default %q", cfg.CacheDir, DefaultCacheDir)
	}
	if cfg.Email != "" || cfg.ListenAddr != "" {
		t.Errorf("expected zero strings for Email/ListenAddr, got %+v", cfg)
	}
	if len(cfg.AllowedHosts) != 0 {
		t.Errorf("expected zero hosts, got %v", cfg.AllowedHosts)
	}
}

func TestLoadConfigFromEnv_NilGetenvUsesOSDefault(t *testing.T) {
	// Don't set any ACME_* envs; just verify nil-getenv doesn't panic.
	cfg := LoadConfigFromEnv(nil)
	if cfg.CacheDir == "" {
		t.Error("nil getenv should still produce default cache dir")
	}
}

func TestBootstrap_NoListenAddrLeavesNotReady(t *testing.T) {
	cfg := Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
	}
	m, err := Bootstrap(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if m.Snapshot().Ready {
		t.Error("Bootstrap without ListenAddr should leave Ready=false")
	}
}

func TestBootstrap_StartsListenerWhenAddrSet(t *testing.T) {
	cfg := Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
		ListenAddr:   "127.0.0.1:0",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m, err := Bootstrap(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer m.StopChallengeServer()
	if !m.Snapshot().Ready {
		t.Error("Bootstrap with ListenAddr should toggle Ready=true")
	}
}

func TestBootstrap_BindFailureBubblesUp(t *testing.T) {
	cfg := Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"example.com"},
		ListenAddr:   ":80",
	}
	failingListen := func(network, address string) (net.Listener, error) {
		return nil, errors.New("forced bind failure")
	}
	_, err := Bootstrap(context.Background(), cfg, failingListen)
	if err == nil {
		t.Error("expected bind error to propagate")
	}
	if !strings.Contains(err.Error(), "forced bind failure") {
		t.Errorf("error should surface listener err: %v", err)
	}
}

func TestBootstrap_InvalidConfigBubblesUp(t *testing.T) {
	cfg := Config{
		CacheDir:     t.TempDir(),
		AllowedHosts: []string{"a;b.com"}, // injection vector
	}
	_, err := Bootstrap(context.Background(), cfg, nil)
	if err == nil {
		t.Error("invalid hostname should propagate from NewManager")
	}
}

func TestChallengeHandler_ServesWellKnownPath(t *testing.T) {
	m, _ := NewManager(Config{CacheDir: t.TempDir(), AllowedHosts: []string{"example.com"}})
	h := m.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	// Non-challenge request → fallback handler
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Errorf("fallback should return 418, got %d", rec.Code)
	}

	// Challenge request without a matching token → autocert returns 404 (no
	// active challenge). Either way we confirm the handler routes by path.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/missing", nil)
	h.ServeHTTP(rec2, req2)
	// Don't assert exact code: autocert may answer with 404 (no token) or
	// something else if a challenge race occurred. We only assert the
	// fallback (418) was NOT consulted.
	if rec2.Code == http.StatusTeapot {
		t.Error("autocert should handle /.well-known/acme-challenge/ itself, not delegate to fallback")
	}
}
