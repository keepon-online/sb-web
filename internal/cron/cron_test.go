package cron

import (
	"context"
	"errors"
	"strings"
	"testing"

	"miaomiaowu/internal/systemops"
)

// fakeExecutor tracks List/Apply invocations for unit tests.
type fakeExecutor struct {
	current   string
	listErr   error
	applyErr  error
	applied   []string
	listCalls int
}

func (f *fakeExecutor) executor() *Executor {
	return &Executor{
		List: func(ctx context.Context) (string, error) {
			f.listCalls++
			return f.current, f.listErr
		},
		Apply: func(ctx context.Context, content string) error {
			if f.applyErr != nil {
				return f.applyErr
			}
			f.applied = append(f.applied, content)
			f.current = content
			return nil
		},
	}
}

func TestBuildEnablePlan_Shape(t *testing.T) {
	plan := BuildEnablePlan()
	if plan.Name != PlanNameEnable {
		t.Errorf("Name = %q, want %q", plan.Name, PlanNameEnable)
	}
	// Single-step atomic operation: RollbackOnFailure is intentionally false
	// because the systemops rollback loop only revisits prior completed
	// steps, and the executor's Apply call is itself atomic.
	if plan.RollbackOnFailure {
		t.Error("Enable should not RollbackOnFailure (single-step, executor-atomic)")
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("step count = %d, want 1", len(plan.Steps))
	}
	step := plan.Steps[0]
	if step.RollbackCommand != "" {
		t.Errorf("step should not declare a no-op rollback, got %q", step.RollbackCommand)
	}
}

func TestBuildDisablePlan_Shape(t *testing.T) {
	plan := BuildDisablePlan()
	if plan.Name != PlanNameDisable {
		t.Errorf("Name = %q, want %q", plan.Name, PlanNameDisable)
	}
	if plan.RollbackOnFailure {
		t.Error("Disable should NOT RollbackOnFailure (no inverse for a partial purge)")
	}
}

func TestPurge_RemovesAllFourPatterns(t *testing.T) {
	input := strings.Join([]string{
		"# user comment",
		"0 0 * * * echo websbox-touched",
		"0 1 * * * systemctl restart sing-box",
		"* * * * * url http://example.com/ping",
		"0 4 * * * /usr/local/bin/sbwpph",
		"@daily echo unrelated",
		"",
	}, "\n")
	got := purge(input)
	wantLines := []string{"# user comment", "@daily echo unrelated"}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Errorf("purged crontab missing benign line %q: %s", want, got)
		}
	}
	for _, bad := range []string{"sing-box", "sbwpph", "url http", "websbox"} {
		if strings.Contains(got, bad) {
			t.Errorf("purged crontab still contains forbidden marker %q: %s", bad, got)
		}
	}
}

func TestPurge_EmptyInput(t *testing.T) {
	if got := purge(""); got != "" {
		t.Errorf("purge(empty) = %q, want empty", got)
	}
}

func TestPurge_AllRemovedYieldsEmpty(t *testing.T) {
	input := "0 1 * * * systemctl restart sing-box\n0 4 * * * /usr/local/bin/sbwpph\n"
	if got := purge(input); got != "" {
		t.Errorf("purge(all-bad) = %q, want empty", got)
	}
}

func TestAppendEntry(t *testing.T) {
	cases := []struct {
		content string
		entry   string
		want    string
	}{
		{"", "0 1 * * * x", "0 1 * * * x\n"},
		{"# header\n", "0 1 * * * x", "# header\n0 1 * * * x\n"},
		{"# no newline", "0 1 * * * x", "# no newline\n0 1 * * * x\n"},
		{"# header\n", "0 1 * * * x\n", "# header\n0 1 * * * x\n"},
	}
	for _, c := range cases {
		if got := appendEntry(c.content, c.entry); got != c.want {
			t.Errorf("appendEntry(%q, %q) = %q, want %q", c.content, c.entry, got, c.want)
		}
	}
}

func TestExecutor_EnableOnEmptyCrontab(t *testing.T) {
	f := &fakeExecutor{}
	plan := BuildEnablePlan()
	if _, err := plan.Execute(context.Background(), f.executor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(f.applied) != 1 {
		t.Fatalf("Apply called %d times, want 1", len(f.applied))
	}
	if !strings.Contains(f.applied[0], CronEntry) {
		t.Errorf("applied crontab missing canonical entry: %s", f.applied[0])
	}
}

func TestExecutor_EnablePurgesStaleEntries(t *testing.T) {
	f := &fakeExecutor{current: strings.Join([]string{
		"# kept",
		"0 1 * * * old systemctl restart sing-box --legacy",
		"@reboot /opt/sbwpph --start",
		"",
	}, "\n")}
	plan := BuildEnablePlan()
	if _, err := plan.Execute(context.Background(), f.executor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	applied := f.applied[0]
	if !strings.Contains(applied, "# kept") {
		t.Errorf("Enable lost benign line: %s", applied)
	}
	for _, bad := range []string{"--legacy", "sbwpph"} {
		if strings.Contains(applied, bad) {
			t.Errorf("Enable did not strip stale %q: %s", bad, applied)
		}
	}
	if !strings.Contains(applied, CronEntry) {
		t.Errorf("Enable missing canonical entry: %s", applied)
	}
}

func TestExecutor_DisableClearsEverythingSbRelated(t *testing.T) {
	f := &fakeExecutor{current: strings.Join([]string{
		"# unrelated",
		"0 1 * * * systemctl restart sing-box",
		"@reboot /opt/sbwpph --start",
		"30 * * * * curl url http://example.com/ping",
		"",
	}, "\n")}
	plan := BuildDisablePlan()
	if _, err := plan.Execute(context.Background(), f.executor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	applied := f.applied[0]
	if !strings.Contains(applied, "# unrelated") {
		t.Errorf("Disable lost benign line: %s", applied)
	}
	for _, bad := range []string{"sing-box", "sbwpph", "url http"} {
		if strings.Contains(applied, bad) {
			t.Errorf("Disable left forbidden marker %q: %s", bad, applied)
		}
	}
}

func TestExecutor_DisableIdempotentOnEmpty(t *testing.T) {
	f := &fakeExecutor{}
	plan := BuildDisablePlan()
	if _, err := plan.Execute(context.Background(), f.executor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(f.applied) != 1 || f.applied[0] != "" {
		t.Errorf("Disable on empty applied = %q, want empty string", f.applied)
	}
}

func TestExecutor_RejectsUnknownMode(t *testing.T) {
	f := &fakeExecutor{}
	step := systemops.OperationStep{
		ID:       "bogus",
		Metadata: map[string]string{metaModeKey: "explode"},
	}
	err := f.executor().ExecuteStep(context.Background(), step)
	if err == nil {
		t.Error("expected error for unknown mode")
	}
	if !strings.Contains(err.Error(), "explode") {
		t.Errorf("error should mention bad mode: %v", err)
	}
}

func TestExecutor_RejectsMissingSeams(t *testing.T) {
	exec := &Executor{}
	err := exec.ExecuteStep(context.Background(), systemops.OperationStep{Metadata: map[string]string{metaModeKey: metaModeEnable}})
	if err == nil {
		t.Error("expected error when List/Apply are unset")
	}
}

func TestExecutor_ListErrorPropagates(t *testing.T) {
	f := &fakeExecutor{listErr: errors.New("forced list failure")}
	plan := BuildEnablePlan()
	if _, err := plan.Execute(context.Background(), f.executor()); err == nil {
		t.Error("expected error when list fails")
	}
}

func TestExecutor_ApplyErrorPropagatesAndCrontabUntouched(t *testing.T) {
	// Atomicity contract: when Apply fails the previous crontab content must
	// remain unchanged. This is delegated to crontab(1)'s own atomic write —
	// the in-memory fake never mutates `current` on error so the assertion
	// here is "no successful apply landed".
	f := &fakeExecutor{
		current:  "0 0 * * * /opt/keep.sh\n",
		applyErr: errors.New("forced apply failure"),
	}
	plan := BuildEnablePlan()
	if _, err := plan.Execute(context.Background(), f.executor()); err == nil {
		t.Error("expected enable to fail when Apply fails")
	}
	if len(f.applied) != 0 {
		t.Errorf("Apply should not have recorded a successful write, got %d", len(f.applied))
	}
	if f.current != "0 0 * * * /opt/keep.sh\n" {
		t.Errorf("crontab content was mutated despite Apply failure: %q", f.current)
	}
}

func TestCronEntry_MatchesSbShReference(t *testing.T) {
	// sb.sh:3804 — preserve the exact entry string so the crontab installed by
	// the legacy script and by this package are byte-identical, simplifying
	// migrations between the two control planes.
	want := "0 1 * * * systemctl restart sing-box;rc-service sing-box restart"
	if CronEntry != want {
		t.Errorf("CronEntry drifted from sb.sh:3804\n got:  %q\n want: %q", CronEntry, want)
	}
}
