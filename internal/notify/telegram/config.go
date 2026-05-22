package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultConfigPath is the on-disk location for the persisted Telegram config.
//
// Layout matches the legacy sb.sh convention but stored as JSON (no shell).
const DefaultConfigPath = "/etc/s-box/sbtg.conf"

// File permissions for the persisted config — must remain 0600 because it
// contains the bot token.
const configFileMode os.FileMode = 0o600

var (
	tokenPattern  = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)
	chatIDPattern = regexp.MustCompile(`^-?[0-9]+$`)
)

// Validate enforces format constraints on the Telegram bot configuration.
//
// Token must match `<digits>:<alphanumeric_-+>`; ChatID must be a (possibly
// negative) integer to allow group chats.
func (c Config) Validate() error {
	token := strings.TrimSpace(c.Token)
	if token == "" {
		return fmt.Errorf("telegram: token is required")
	}
	if !tokenPattern.MatchString(token) {
		return fmt.Errorf("telegram: token format invalid")
	}

	chatID := strings.TrimSpace(c.ChatID)
	if chatID == "" {
		return fmt.Errorf("telegram: chat_id is required")
	}
	if !chatIDPattern.MatchString(chatID) {
		return fmt.Errorf("telegram: chat_id must be an integer")
	}

	return nil
}

// LoadConfig reads a Telegram config JSON file from disk.
//
// Returns os.ErrNotExist (wrappable via errors.Is) when the file is absent so
// callers can distinguish "no config" from "broken config".
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("telegram: parse config %s: %w", path, err)
	}

	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.ChatID = strings.TrimSpace(cfg.ChatID)
	return cfg, nil
}

// SaveConfig validates and atomically writes the Telegram config to disk with
// 0600 permissions. Parent directory is created with 0700 if missing.
func SaveConfig(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("telegram: ensure config dir %s: %w", dir, err)
		}
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("telegram: marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, configFileMode); err != nil {
		return fmt.Errorf("telegram: write temp config %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, configFileMode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("telegram: chmod temp config %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("telegram: rename temp config to %s: %w", path, err)
	}
	return nil
}

// MaskToken returns a redacted token suitable for API responses / logs.
//
// Rule: keep the first 6 and last 4 chars, replace the middle with "***".
// If the token is shorter than 10 chars, returns "***".
func MaskToken(token string) string {
	t := strings.TrimSpace(token)
	if len(t) < 10 {
		if t == "" {
			return ""
		}
		return "***"
	}
	return t[:6] + "***" + t[len(t)-4:]
}
