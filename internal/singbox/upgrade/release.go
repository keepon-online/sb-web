// HTTP version discovery for sing-box releases. Mirrors sb.sh:3833-3844
// lapre() with one structural difference: jsdelivr metadata API is dropped
// (it caches stale data and was already a fallback in upstream); the GitHub
// HTML "/releases/latest" and "/releases" pages are used as the canonical
// sources.
//
// The function never panics on parse failure: it returns empty strings and a
// nil error so the upgrade UI can degrade to mandatory Pinned mode.
package upgrade

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	// fetchTimeout caps each individual HTTP call.
	fetchTimeout = 10 * time.Second

	// maxResponseBody guards against pathological responses (default GitHub
	// pages are well under 1 MiB; 4 MiB is generous headroom).
	maxResponseBody = 4 * 1024 * 1024
)

// latestReleaseURL is the canonical "latest stable" GitHub page.
// Declared as a var (not const) so tests can swap to httptest endpoints.
//
// releasesListURL is the listing page used to surface the most recent
// pre-release (alpha/rc/beta).
var (
	latestReleaseURL = "https://github.com/SagerNet/sing-box/releases/latest"
	releasesListURL  = "https://github.com/SagerNet/sing-box/releases"
)

// stableTagPattern matches `/tag/vX.Y.Z` (stable releases never include a
// suffix). sb.sh:3856 uses `grep -oP 'tag/v\K[0-9.]+'`.
var stableTagPattern = regexp.MustCompile(`tag/v([0-9]+\.[0-9]+\.[0-9]+)\b`)

// preTagPattern matches `/tag/vX.Y.Z-alpha.N` (or rc/beta). sb.sh:3858 uses
// `grep -oP '/tag/v\K[0-9.]+-[^"]+'`. We use a tighter regex with a stricter
// suffix grammar so unrelated tag strings cannot poison the result.
var preTagPattern = regexp.MustCompile(`tag/v([0-9]+\.[0-9]+\.[0-9]+-(?:alpha|rc|beta)\.[0-9]+)\b`)

// FetchLatest queries GitHub for the latest stable and pre-release tags.
//
// On success returns two version strings that have already passed
// ValidateVersion. On failure of either query returns an empty string for
// that channel and never raises an error — callers degrade to mandatory
// Pinned mode.
//
// client may be nil; a default client with the package timeout is used.
// The provided context bounds total wall-clock; the per-call timeout still
// applies so a hung connection cannot block longer than fetchTimeout.
func FetchLatest(ctx context.Context, client *http.Client) (stable string, pre string, err error) {
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}

	if body, ok := fetchPage(ctx, client, latestReleaseURL); ok {
		if v := firstMatch(stableTagPattern, body); v != "" {
			if ValidateVersion(v) == nil {
				stable = v
			}
		}
	}

	if body, ok := fetchPage(ctx, client, releasesListURL); ok {
		if v := firstMatch(preTagPattern, body); v != "" {
			if ValidateVersion(v) == nil {
				pre = v
			}
		}
	}

	return stable, pre, nil
}

// fetchPage performs a single GET. Returns the response body and true on
// 2xx; on any other outcome returns an empty body and false. Errors are
// swallowed so FetchLatest can return the documented best-effort signature.
func fetchPage(ctx context.Context, client *http.Client, url string) (string, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", false
	}
	// GitHub returns the lightweight HTML listing for any unauthenticated
	// browser. A neutral UA avoids the "scraper" branch.
	req.Header.Set("User-Agent", "sb-web-upgrade/1.0")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return "", false
	}
	return string(body), true
}

// firstMatch returns the first capturing-group value matched by re in body,
// or empty if no match. Equivalent to `grep -oP ... | head -n1 | cut ...`.
func firstMatch(re *regexp.Regexp, body string) string {
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
