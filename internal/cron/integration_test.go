package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installFakeCrontab writes a shell stub onto PATH that emulates the subset of
// crontab(1) this package depends on:
//
//	crontab -l       → reads $FAKE_CRONTAB_FILE, exits 1 with "no crontab" if absent
//	crontab -r       → deletes $FAKE_CRONTAB_FILE, exits 0; exits 1 "no crontab" if absent
//	crontab -        → writes stdin to $FAKE_CRONTAB_FILE
//
// Failure modes are toggled via $FAKE_CRONTAB_FAIL = list|apply|remove.
func installFakeCrontab(t *testing.T) (cronFile string) {
	t.Helper()
	dir := t.TempDir()
	cronFile = filepath.Join(dir, "cron.txt")

	script := `#!/bin/sh
case "$1" in
  -l)
    if [ "$FAKE_CRONTAB_FAIL" = "list" ]; then
      echo "fake list failure" >&2
      exit 2
    fi
    if [ ! -f "$FAKE_CRONTAB_FILE" ]; then
      echo "no crontab for $(whoami)" >&2
      exit 1
    fi
    cat "$FAKE_CRONTAB_FILE"
    ;;
  -r)
    if [ "$FAKE_CRONTAB_FAIL" = "remove" ]; then
      echo "fake remove failure" >&2
      exit 2
    fi
    if [ ! -f "$FAKE_CRONTAB_FILE" ]; then
      echo "no crontab for $(whoami)" >&2
      exit 1
    fi
    rm -f "$FAKE_CRONTAB_FILE"
    ;;
  -)
    if [ "$FAKE_CRONTAB_FAIL" = "apply" ]; then
      echo "fake apply failure" >&2
      exit 2
    fi
    cat > "$FAKE_CRONTAB_FILE"
    ;;
  *)
    echo "unsupported flag: $1" >&2
    exit 99
    ;;
esac
`
	binPath := filepath.Join(dir, "crontab")
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake crontab: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CRONTAB_FILE", cronFile)
	return cronFile
}

func TestLiveList_NoCrontabIsTreatedAsEmpty(t *testing.T) {
	installFakeCrontab(t)
	got, err := liveList(context.Background())
	if err != nil {
		t.Fatalf("liveList: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty when no crontab, got %q", got)
	}
}

func TestLiveList_ReturnsContent(t *testing.T) {
	cronFile := installFakeCrontab(t)
	want := "0 0 * * * /opt/work.sh\n"
	if err := os.WriteFile(cronFile, []byte(want), 0o600); err != nil {
		t.Fatalf("seed crontab: %v", err)
	}
	got, err := liveList(context.Background())
	if err != nil {
		t.Fatalf("liveList: %v", err)
	}
	if got != want {
		t.Errorf("liveList = %q, want %q", got, want)
	}
}

func TestLiveList_FailurePropagates(t *testing.T) {
	installFakeCrontab(t)
	t.Setenv("FAKE_CRONTAB_FAIL", "list")
	if _, err := liveList(context.Background()); err == nil {
		t.Error("expected error when fake crontab -l fails")
	}
}

func TestLiveApply_WritesContent(t *testing.T) {
	cronFile := installFakeCrontab(t)
	want := "0 1 * * * /usr/local/bin/job\n"
	if err := liveApply(context.Background(), want); err != nil {
		t.Fatalf("liveApply: %v", err)
	}
	got, err := os.ReadFile(cronFile)
	if err != nil {
		t.Fatalf("read seeded crontab: %v", err)
	}
	if string(got) != want {
		t.Errorf("written crontab = %q, want %q", got, want)
	}
}

func TestLiveApply_EmptyContentTriggersRemove(t *testing.T) {
	cronFile := installFakeCrontab(t)
	if err := os.WriteFile(cronFile, []byte("0 0 * * * x\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := liveApply(context.Background(), ""); err != nil {
		t.Fatalf("liveApply empty: %v", err)
	}
	if _, err := os.Stat(cronFile); !os.IsNotExist(err) {
		t.Errorf("cronFile should be removed, stat err = %v", err)
	}
}

func TestLiveApply_EmptyContentIsIdempotent(t *testing.T) {
	installFakeCrontab(t)
	// No prior crontab; empty Apply should not error.
	if err := liveApply(context.Background(), ""); err != nil {
		t.Errorf("liveApply on absent crontab should be no-op, got %v", err)
	}
}

func TestLiveApply_FailurePropagates(t *testing.T) {
	installFakeCrontab(t)
	t.Setenv("FAKE_CRONTAB_FAIL", "apply")
	err := liveApply(context.Background(), "0 0 * * * x\n")
	if err == nil {
		t.Error("expected error when fake crontab - fails")
	}
	if !strings.Contains(err.Error(), "fake apply failure") {
		t.Errorf("error should surface stderr: %v", err)
	}
}

func TestLiveApply_RemoveFailureSurfaced(t *testing.T) {
	cronFile := installFakeCrontab(t)
	if err := os.WriteFile(cronFile, []byte("0 0 * * * x\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Setenv("FAKE_CRONTAB_FAIL", "remove")
	err := liveApply(context.Background(), "")
	if err == nil {
		t.Error("expected error when crontab -r fails")
	}
}

func TestNewLiveExecutor_BindsLiveSeams(t *testing.T) {
	exec := NewLiveExecutor()
	if exec.List == nil {
		t.Error("List seam is nil")
	}
	if exec.Apply == nil {
		t.Error("Apply seam is nil")
	}
}
