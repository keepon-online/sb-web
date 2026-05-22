package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaskToken(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"abc":                      "***",
		"abcdefghi":                "***", // 9 chars
		"abcdefghij":               "abcdef***ghij",
		"123456789:AAAAAAAAAAwxyz": "123456***wxyz",
		"   123456789:abcdwxyz   ": "123456***wxyz", // TrimSpace
	}
	for in, want := range cases {
		if got := MaskToken(in); got != want {
			t.Errorf("MaskToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigValidate_Accepts(t *testing.T) {
	cases := []Config{
		{Token: "123456789:AAEBcdef-_GHi", ChatID: "123456"},
		{Token: "1:a", ChatID: "-100123456789"}, // group chat
		{Token: "999999:Z_-Z", ChatID: "0"},
	}
	for _, c := range cases {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%+v) unexpected err: %v", c, err)
		}
	}
}

func TestConfigValidate_Rejects(t *testing.T) {
	cases := map[string]Config{
		"empty token":            {Token: "", ChatID: "1"},
		"empty chat":             {Token: "1:a", ChatID: ""},
		"no colon in token":      {Token: "abc", ChatID: "1"},
		"chat has letter":        {Token: "1:a", ChatID: "abc"},
		"chat is float":          {Token: "1:a", ChatID: "1.5"},
		"token has space":        {Token: "1: a", ChatID: "1"},
		"token has special char": {Token: "1:a@b", ChatID: "1"},
		"chat double negative":   {Token: "1:a", ChatID: "--1"},
	}
	for label, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%s) should reject: %+v", label, c)
		}
	}
}

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sbtg.conf")
	original := Config{Token: "987654321:WXYZcdefghIJKLmnop", ChatID: "-1001234567890"}

	if err := SaveConfig(path, original); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded != original {
		t.Errorf("round trip mismatch: got %+v, want %+v", loaded, original)
	}
}

func TestSaveConfig_FilePermissionsAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file permissions not enforced on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sbtg.conf")
	if err := SaveConfig(path, Config{Token: "1:a", ChatID: "1"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode = %#o, want 0600", mode)
	}
}

func TestSaveConfig_RejectsInvalidConfigBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sbtg.conf")
	if err := SaveConfig(path, Config{Token: "", ChatID: ""}); err == nil {
		t.Fatal("expected validation rejection")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file should not exist after rejection, stat err = %v", err)
	}
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("tmp file should not exist after rejection, stat err = %v", err)
	}
}

func TestSaveConfig_AtomicityNoTempLingers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sbtg.conf")
	if err := SaveConfig(path, Config{Token: "111:abc", ChatID: "1"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("tmp file should be renamed away, stat err = %v", err)
	}
}

func TestLoadConfig_MissingFileReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadConfig(filepath.Join(dir, "missing.conf"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadConfig_RejectsCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.conf")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Error("expected error on broken JSON")
	}
}

func TestSendMessage_SuccessfulPost(t *testing.T) {
	var captured *http.Request
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r
		body, _ := readAllStr(r)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Token: "111:abc", ChatID: "42"}).WithBaseURL(srv.URL)
	if err := c.SendMessage(context.Background(), "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if captured == nil {
		t.Fatal("no request captured")
	}
	if !strings.Contains(captured.URL.Path, "/bot111:abc/sendMessage") {
		t.Errorf("path = %q, want /bot111:abc/sendMessage", captured.URL.Path)
	}
	if !strings.Contains(capturedBody, "chat_id=42") {
		t.Errorf("body missing chat_id: %q", capturedBody)
	}
	if !strings.Contains(capturedBody, "text=hello") {
		t.Errorf("body missing text: %q", capturedBody)
	}
	if !strings.Contains(capturedBody, "parse_mode=HTML") {
		t.Errorf("body missing parse_mode: %q", capturedBody)
	}
}

func TestSendMessage_APIErrorIncludesNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"chat not found"}`))
	}))
	defer srv.Close()

	token := "999:supersecrettoken-_X"
	c := NewClient(Config{Token: token, ChatID: "1"}).WithBaseURL(srv.URL)
	err := c.SendMessage(context.Background(), "msg")
	if err == nil {
		t.Fatal("expected error from non-ok response")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error message leaks token: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("error message missing telegram description: %s", err.Error())
	}
}

func TestSendMessage_TransportErrorIsTokenRedacted(t *testing.T) {
	token := "777:transportleak_-_X"
	// Point to a non-routable address so DialContext fails; the resulting
	// *url.Error contains the request URL (with token), and redactErr must
	// strip it.
	c := NewClient(Config{Token: token, ChatID: "1"}).WithBaseURL("http://127.0.0.1:1")
	err := c.SendMessage(context.Background(), "msg")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("transport error leaks token: %s", err.Error())
	}
}

func TestSendMessage_EmptyArguments(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		text string
	}{
		{"empty token", Config{Token: "", ChatID: "1"}, "msg"},
		{"empty chat", Config{Token: "1:a", ChatID: ""}, "msg"},
		{"empty text", Config{Token: "1:a", ChatID: "1"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := NewClient(c.cfg)
			if err := client.SendMessage(context.Background(), c.text); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func readAllStr(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	defer r.Body.Close()
	buf := make([]byte, 8192)
	n, err := r.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return "", err
	}
	return string(buf[:n]), nil
}
