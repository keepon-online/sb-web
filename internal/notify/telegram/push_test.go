package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"miaomiaowu/internal/systemops"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestBuildPushPlan_RejectsInvalidConfig(t *testing.T) {
	if _, err := BuildPushPlan(Config{}, t.TempDir()); err == nil {
		t.Error("expected validation rejection")
	}
}

func TestBuildPushPlan_NoFilesYieldsError(t *testing.T) {
	dir := t.TempDir()
	_, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if err == nil {
		t.Error("expected error when no subscription files exist")
	}
	if !strings.Contains(err.Error(), "no subscription files") {
		t.Errorf("error should mention missing files: %v", err)
	}
}

func TestBuildPushPlan_SkipsZeroByteFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vl_reality.txt", "")     // zero-byte → skipped
	writeFile(t, dir, "vm_ws.txt", "non-empty") // counts
	plan, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if err != nil {
		t.Fatalf("BuildPushPlan: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("step count = %d, want 1", len(plan.Steps))
	}
	if !strings.Contains(plan.Steps[0].ID, "vm_ws") {
		t.Errorf("step ID should mention vm_ws: %s", plan.Steps[0].ID)
	}
}

func TestBuildPushPlan_SbBoxJSONSplitsInto4(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sbox.json", strings.Repeat("line\n", 40))
	plan, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if err != nil {
		t.Fatalf("BuildPushPlan: %v", err)
	}
	if len(plan.Steps) != 4 {
		t.Fatalf("step count = %d, want 4 (split)", len(plan.Steps))
	}
	for i, s := range plan.Steps {
		want := i + 1
		if got := s.Metadata[metaKeySplitIdx]; got != intStr(want) {
			t.Errorf("step %d split_idx = %q, want %q", i, got, intStr(want))
		}
		if got := s.Metadata[metaKeySplitN]; got != "4" {
			t.Errorf("step %d split_n = %q, want 4", i, got)
		}
		if !strings.Contains(s.Title, "sbox.json") {
			t.Errorf("step %d title missing sbox.json: %s", i, s.Title)
		}
	}
}

func TestBuildPushPlan_ClmiYAMLSplitsInto2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clmi.yaml", strings.Repeat("line\n", 20))
	plan, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if err != nil {
		t.Fatalf("BuildPushPlan: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("step count = %d, want 2 (split)", len(plan.Steps))
	}
}

func TestBuildPushPlan_OrderPreservedAsSbSh(t *testing.T) {
	dir := t.TempDir()
	// Write a subset in scrambled OS-creation order, plan order must follow orderedFiles.
	for _, name := range []string{"jhsub.txt", "vl_reality.txt", "hy2.txt"} {
		writeFile(t, dir, name, "x")
	}
	plan, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if err != nil {
		t.Fatalf("BuildPushPlan: %v", err)
	}
	wantSeq := []string{"vl_reality.txt", "hy2.txt", "jhsub.txt"}
	for i, s := range plan.Steps {
		if !strings.Contains(s.Target, wantSeq[i]) {
			t.Errorf("step %d target = %q, want to contain %q", i, s.Target, wantSeq[i])
		}
	}
}

func TestBuildPushPlan_RollbackOnFailureIsFalse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vl_reality.txt", "x")
	plan, _ := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if plan.RollbackOnFailure {
		t.Error("RollbackOnFailure must be false (best-effort push)")
	}
}

func TestBuildPushPlan_DefaultDirFallback(t *testing.T) {
	// Empty dir falls back to DefaultSBoxDir which won't exist in tests; expect
	// "no subscription files" error, not panic.
	_, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, "   ")
	if err == nil {
		t.Skip("default sbox dir populated on this host")
	}
}

func TestSanitizeID(t *testing.T) {
	cases := map[string]string{
		"vl_reality.txt":        "vl_reality_txt",
		"sing_box_gitlab.txt":   "sing_box_gitlab_txt",
		"weird/file:name?.txt":  "weird_file_name__txt",
		"___leading_trailing__": "leading_trailing",
		"":                      "",
	}
	for in, want := range cases {
		if got := sanitizeID(in); got != want {
			t.Errorf("sanitizeID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSliceLines_EquallySplits(t *testing.T) {
	text := strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, "\n") // 8 lines
	// 8 / 4 = 2 per segment, last segment gets the remainder.
	wantSeg := map[int]string{
		1: "a\nb",
		2: "c\nd",
		3: "e\nf",
		4: "g\nh",
	}
	for idx, want := range wantSeg {
		if got := sliceLines(text, idx, 4); got != want {
			t.Errorf("segment %d = %q, want %q", idx, got, want)
		}
	}
}

func TestSliceLines_LastSegmentTakesRemainder(t *testing.T) {
	// 9 lines / 4 = 2 per segment; last segment must absorb the trailing line.
	text := strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}, "\n")
	got := sliceLines(text, 4, 4)
	if got != "g\nh\ni" {
		t.Errorf("segment 4 = %q, want %q", got, "g\nh\ni")
	}
}

func TestSliceLines_EdgeCases(t *testing.T) {
	if got := sliceLines("", 1, 1); got != "" {
		t.Errorf("empty text = %q, want empty", got)
	}
	if got := sliceLines("x", 1, 1); got != "x" {
		t.Errorf("n<=1 should return entire text, got %q", got)
	}
}

func TestParsePositive(t *testing.T) {
	if v, err := parsePositive("3"); err != nil || v != 3 {
		t.Errorf("parsePositive(3) = (%d, %v), want (3, nil)", v, err)
	}
	if _, err := parsePositive("0"); err == nil {
		t.Error("parsePositive(0) should error")
	}
	if _, err := parsePositive("-5"); err == nil {
		t.Error("parsePositive(-5) should error")
	}
	if _, err := parsePositive("abc"); err == nil {
		t.Error("parsePositive(abc) should error")
	}
}

// PushExecutor integration: drives the full step → Telegram POST flow.
func TestPushExecutor_HappyPath(t *testing.T) {
	var hits int32
	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := readAllStr(r)
		captured = append(captured, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeFile(t, dir, "vl_reality.txt", "vless://abc")
	plan, err := BuildPushPlan(Config{Token: "1:a", ChatID: "9"}, dir)
	if err != nil {
		t.Fatalf("BuildPushPlan: %v", err)
	}

	client := NewClient(Config{Token: "1:a", ChatID: "9"}).WithBaseURL(srv.URL)
	exec := NewPushExecutor(client)
	if _, err := plan.Execute(context.Background(), exec); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("API hits = %d, want 1", got)
	}
	if len(captured) != 1 || !strings.Contains(captured[0], "vless%3A%2F%2Fabc") {
		t.Errorf("body did not include url-encoded content: %v", captured)
	}
	if !strings.Contains(captured[0], "Vless-reality-vision") {
		t.Errorf("body should include header text: %v", captured)
	}
}

func TestPushExecutor_NilClientFails(t *testing.T) {
	exec := NewPushExecutor(nil)
	err := exec.ExecuteStep(context.Background(), systemops.OperationStep{
		ID:       "x",
		Metadata: map[string]string{metaKeyFile: "/nonexistent"},
	})
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestPushExecutor_MissingMetadata(t *testing.T) {
	c := NewClient(Config{Token: "1:a", ChatID: "1"})
	exec := NewPushExecutor(c)
	if err := exec.ExecuteStep(context.Background(), systemops.OperationStep{ID: "x"}); err == nil {
		t.Error("expected error for nil metadata")
	}
}

func TestPushExecutor_MissingFilePath(t *testing.T) {
	c := NewClient(Config{Token: "1:a", ChatID: "1"})
	exec := NewPushExecutor(c)
	step := systemops.OperationStep{ID: "x", Metadata: map[string]string{}}
	if err := exec.ExecuteStep(context.Background(), step); err == nil {
		t.Error("expected error for missing file path")
	}
}

func TestPushExecutor_FileReadFails(t *testing.T) {
	c := NewClient(Config{Token: "1:a", ChatID: "1"})
	exec := NewPushExecutor(c)
	step := systemops.OperationStep{
		ID:       "x",
		Metadata: map[string]string{metaKeyFile: "/nonexistent/path/file.txt"},
	}
	if err := exec.ExecuteStep(context.Background(), step); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestPushExecutor_InvalidSplitMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "content")
	c := NewClient(Config{Token: "1:a", ChatID: "1"}).WithBaseURL(srv.URL)
	exec := NewPushExecutor(c)
	step := systemops.OperationStep{
		ID: "bad-split",
		Metadata: map[string]string{
			metaKeyFile:     filepath.Join(dir, "file.txt"),
			metaKeySplitIdx: "5",
			metaKeySplitN:   "3", // idx > n
		},
	}
	if err := exec.ExecuteStep(context.Background(), step); err == nil {
		t.Error("expected error for idx > n")
	}
}

func TestPushExecutor_SplitSubsequentSegmentsAreHeaderless(t *testing.T) {
	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAllStr(r)
		captured = append(captured, body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeFile(t, dir, "sbox.json", strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, "\n"))
	plan, err := BuildPushPlan(Config{Token: "1:a", ChatID: "1"}, dir)
	if err != nil {
		t.Fatalf("BuildPushPlan: %v", err)
	}

	c := NewClient(Config{Token: "1:a", ChatID: "1"}).WithBaseURL(srv.URL)
	exec := NewPushExecutor(c)
	if _, err := plan.Execute(context.Background(), exec); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(captured) != 4 {
		t.Fatalf("captured = %d messages, want 4", len(captured))
	}
	// Segment 1 carries the header; 2/3/4 do not.
	if !strings.Contains(captured[0], "%E9%85%8D%E7%BD%AE") {
		// "配置" url-encoded — but the header text may contain other chars; just
		// verify the header keyword "Sing-box" survives encoding.
		if !strings.Contains(captured[0], "Sing-box") {
			t.Errorf("segment 1 should carry header text, got %q", captured[0])
		}
	}
	for i := 1; i < 4; i++ {
		if strings.Contains(captured[i], "Sing-box") && strings.Contains(captured[i], "%E9%85%8D%E7%BD%AE") {
			t.Errorf("segment %d should NOT carry header, got %q", i+1, captured[i])
		}
	}
}

func intStr(n int) string {
	// Local mini-helper to avoid pulling strconv just for two callers.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
