package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAPIBaseURL is the production Telegram Bot API endpoint.
const DefaultAPIBaseURL = "https://api.telegram.org"

// DefaultTimeout matches the legacy sb.sh `timeout 20s curl ...` semantics.
const DefaultTimeout = 20 * time.Second

// Config holds the credentials required to authenticate with the Telegram Bot API.
//
// Token must never be logged or surfaced via API responses.
type Config struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

// Client wraps an *http.Client and a Telegram Bot configuration.
//
// All public methods enforce a per-call deadline that is the minimum of the
// caller's context deadline and DefaultTimeout (20s).
type Client struct {
	cfg        Config
	httpClient *http.Client
	baseURL    string
}

// NewClient returns a Client backed by an *http.Client with a 20s timeout.
//
// Constructed Clients are safe for concurrent use.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		baseURL: DefaultAPIBaseURL,
	}
}

// WithBaseURL overrides the Telegram API base (test seam, never used in prod).
func (c *Client) WithBaseURL(base string) *Client {
	c.baseURL = strings.TrimRight(base, "/")
	return c
}

// SendMessage POSTs `text` to Telegram's sendMessage endpoint with parse_mode=HTML.
//
// Errors returned by this function never include the bot token. On any failure
// callers may safely log the error.
func (c *Client) SendMessage(ctx context.Context, text string) error {
	if strings.TrimSpace(c.cfg.Token) == "" {
		return fmt.Errorf("telegram: token is empty")
	}
	if strings.TrimSpace(c.cfg.ChatID) == "" {
		return fmt.Errorf("telegram: chat_id is empty")
	}
	if text == "" {
		return fmt.Errorf("telegram: message text is empty")
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.cfg.Token)

	body := url.Values{}
	body.Set("chat_id", c.cfg.ChatID)
	body.Set("parse_mode", "HTML")
	body.Set("text", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body.Encode()))
	if err != nil {
		// Sanitize: replace token in any returned error.
		return fmt.Errorf("telegram: build request: %w", redactErr(err, c.cfg.Token))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send request: %w", redactErr(err, c.cfg.Token))
	}
	defer resp.Body.Close()

	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		return fmt.Errorf("telegram: read response (status=%d): %w", resp.StatusCode, redactErr(readErr, c.cfg.Token))
	}

	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
	}
	if jsonErr := json.Unmarshal(payload, &parsed); jsonErr != nil {
		return fmt.Errorf("telegram: parse response (status=%d): %w", resp.StatusCode, redactErr(jsonErr, c.cfg.Token))
	}

	if !parsed.OK {
		// `Description` is from Telegram and never contains our token.
		return fmt.Errorf("telegram: api error (status=%d, code=%d): %s", resp.StatusCode, parsed.ErrorCode, parsed.Description)
	}

	return nil
}

// redactErr replaces any occurrence of `token` in err.Error() with "<redacted>".
// Returns a plain error so the secret never leaks into logs or audit records.
func redactErr(err error, token string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if token != "" && strings.Contains(msg, token) {
		msg = strings.ReplaceAll(msg, token, "<redacted>")
		return fmt.Errorf("%s", msg)
	}
	return err
}
