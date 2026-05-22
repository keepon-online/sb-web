// Package warp encapsulates the sb.sh warpwg/warpcode/reg/reserved business
// (sb.sh:3316-3365) — registering a fresh Cloudflare Warp account and
// computing the wireguard parameters sing-box needs to enable Warp-IPV6
// outbound.
//
// Two responsibilities:
//
//  1. Generate an X25519 keypair via crypto/ecdh (replacing the legacy
//     `openssl genpkey -algorithm X25519` + xxd/base64 pipeline).
//
//  2. Register the public key with Cloudflare's consumer Warp endpoint and
//     surface the returned IPv6 address plus the three-byte "reserved"
//     tuple that wireguard configs require.
//
// The package never writes to disk; persistence is the caller's concern.
package warp

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Cloudflare consumer Warp endpoint and headers exactly as sb.sh:3322-3324.
const (
	DefaultRegURL       = "https://api.cloudflareclient.com/v0a2158/reg"
	DefaultClientHeader = "a-7.21-0721"
	DefaultTimeout      = 5 * time.Second

	// HardcodedFallback values from sb.sh:3354-3356, returned by Fallback()
	// for callers that need to mirror the legacy script's "API failed →
	// well-known WARP account" behavior.
	FallbackV6          = "2606:4700:110:860e:738f:b37:f15:d38d"
	FallbackPrivateKey  = "g9I2sgUH6OCbIBTehkEfVEnuvInHYZvPOFhWchMLSc4="
	fallbackReservedHex = "21d981" // hex-encoded [33, 217, 129]
)

// Account is the canonical wireguard parameter bundle returned by Register.
//
// Reserved is the 3-byte tuple sing-box expects as `reserved`: [a, b, c]
// where a/b/c are 0-255 integers derived from the Cloudflare client_id.
type Account struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	IPv6       string `json:"ipv6"`
	IPv4       string `json:"ipv4,omitempty"`
	Reserved   [3]int `json:"reserved"`
	ClientID   string `json:"client_id"`
}

// regResponse mirrors the subset of the Cloudflare /reg payload we consume.
type regResponse struct {
	Config struct {
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
		ClientID string `json:"client_id"`
	} `json:"config"`
}

// Register issues the registration POST against Cloudflare and returns the
// derived Account. Pass nil for client to use the package default.
//
// On any non-2xx response or transport failure the error is returned with no
// fallback applied — callers that want the legacy hardcoded triple can use
// Fallback() explicitly.
func Register(ctx context.Context, client *http.Client) (Account, error) {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	return registerWithURL(ctx, client, DefaultRegURL)
}

func registerWithURL(ctx context.Context, client *http.Client, regURL string) (Account, error) {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Account{}, fmt.Errorf("warp: generate X25519 key: %w", err)
	}

	pubKey := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())
	privKey := base64.StdEncoding.EncodeToString(priv.Bytes())

	body := map[string]string{
		"key": pubKey,
		"tos": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Account{}, fmt.Errorf("warp: marshal body: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, regURL, strings.NewReader(string(payload)))
	if err != nil {
		return Account{}, fmt.Errorf("warp: build request: %w", err)
	}
	req.Header.Set("CF-Client-Version", DefaultClientHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Account{}, fmt.Errorf("warp: register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Account{}, fmt.Errorf("warp: cloudflare returned status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return Account{}, fmt.Errorf("warp: read response: %w", err)
	}

	var parsed regResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Account{}, fmt.Errorf("warp: parse response: %w", err)
	}

	if parsed.Config.ClientID == "" {
		return Account{}, fmt.Errorf("warp: cloudflare response missing client_id")
	}

	reserved, err := DecodeReserved(parsed.Config.ClientID)
	if err != nil {
		return Account{}, fmt.Errorf("warp: derive reserved: %w", err)
	}

	return Account{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		IPv6:       strings.SplitN(parsed.Config.Interface.Addresses.V6, "/", 2)[0],
		IPv4:       strings.SplitN(parsed.Config.Interface.Addresses.V4, "/", 2)[0],
		Reserved:   reserved,
		ClientID:   parsed.Config.ClientID,
	}, nil
}

// DecodeReserved replays sb.sh:3336-3338 in Go: base64-decode the client_id,
// then return the first three bytes as [a, b, c]. Cloudflare client_ids are
// always at least 3 bytes after decoding.
func DecodeReserved(clientID string) ([3]int, error) {
	raw, err := base64.StdEncoding.DecodeString(clientID)
	if err != nil {
		return [3]int{}, fmt.Errorf("base64 decode client_id: %w", err)
	}
	if len(raw) < 3 {
		return [3]int{}, fmt.Errorf("client_id decodes to %d bytes (need ≥3)", len(raw))
	}
	return [3]int{int(raw[0]), int(raw[1]), int(raw[2])}, nil
}

// Fallback returns the hardcoded "well-known" Warp account from sb.sh:3354-
// 3356. Callers should only invoke this when Register fails AND business
// logic explicitly accepts the shared account.
func Fallback() Account {
	raw := []byte{0x21, 0xd9, 0x81}
	return Account{
		PrivateKey: FallbackPrivateKey,
		IPv6:       FallbackV6,
		Reserved:   [3]int{int(raw[0]), int(raw[1]), int(raw[2])},
	}
}

// Ensure fallbackReservedHex constant stays in sync with the Fallback()
// implementation — referenced from tests so it cannot silently drift.
var _ = fallbackReservedHex
