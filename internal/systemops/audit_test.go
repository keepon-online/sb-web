package systemops

import (
	"context"
	"errors"
	"testing"
)

type fakeAuditWriter struct {
	records []AuditRecord
	err     error
}

func (f *fakeAuditWriter) WriteAudit(ctx context.Context, record AuditRecord) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.records = append(f.records, record)
	return int64(len(f.records)), nil
}

func TestExecuteWithAudit_SuccessRecordsAllSteps(t *testing.T) {
	plan := OperationPlan{
		Name: "test-plan",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "echo", Args: []string{"a"}},
			{ID: "s2", Title: "Step 2", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "echo", Args: []string{"b"}},
		},
	}
	writer := &fakeAuditWriter{}
	executor := NewDefaultStepExecutor()

	result, err := ExecuteWithAudit(context.Background(), plan, executor, writer, "alice")
	if err != nil {
		t.Fatalf("ExecuteWithAudit failed: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 result steps, got %d", len(result.Steps))
	}
	if len(writer.records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(writer.records))
	}

	rec := writer.records[0]
	if rec.PlanName != "test-plan" {
		t.Errorf("plan name = %q, want test-plan", rec.PlanName)
	}
	if rec.Status != AuditStatusSuccess {
		t.Errorf("status = %q, want success", rec.Status)
	}
	if rec.Username != "alice" {
		t.Errorf("username = %q, want alice", rec.Username)
	}
	if len(rec.Steps) != 2 {
		t.Fatalf("expected 2 step audits, got %d", len(rec.Steps))
	}
	for _, s := range rec.Steps {
		if s.StartedAt.IsZero() || s.FinishedAt.IsZero() {
			t.Errorf("step %s missing timing", s.ID)
		}
		if s.Error != "" {
			t.Errorf("step %s should not have error: %s", s.ID, s.Error)
		}
	}
}

func TestExecuteWithAudit_DryRunMarksAllStepsSkipped(t *testing.T) {
	plan := OperationPlan{
		Name:   "dry-run-plan",
		DryRun: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "echo"},
		},
	}
	writer := &fakeAuditWriter{}

	_, err := ExecuteWithAudit(context.Background(), plan, NewDefaultStepExecutor(), writer, "")
	if err != nil {
		t.Fatalf("ExecuteWithAudit dry-run failed: %v", err)
	}
	if len(writer.records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(writer.records))
	}
	rec := writer.records[0]
	if !rec.DryRun {
		t.Error("audit should be marked as dry-run")
	}
	if rec.Status != AuditStatusSuccess {
		t.Errorf("dry-run status = %q, want success", rec.Status)
	}
	if len(rec.Steps) != 1 {
		t.Fatalf("expected 1 step audit, got %d", len(rec.Steps))
	}
	if rec.Steps[0].SkippedReason != "dry-run" {
		t.Errorf("skipped_reason = %q, want dry-run", rec.Steps[0].SkippedReason)
	}
}

type failingExecutor struct {
	failOn string
}

func (f *failingExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	if step.ID == f.failOn {
		return errors.New("step failed")
	}
	return nil
}

func TestExecuteWithAudit_PartialFailureRecordsStatus(t *testing.T) {
	plan := OperationPlan{
		Name: "fail-plan",
		Steps: []OperationStep{
			{ID: "s1", Title: "OK", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1"},
			{ID: "s2", Title: "Fail", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2"},
		},
	}
	writer := &fakeAuditWriter{}
	executor := &failingExecutor{failOn: "s2"}

	_, err := ExecuteWithAudit(context.Background(), plan, executor, writer, "")
	if err == nil {
		t.Fatal("expected execution error")
	}
	if len(writer.records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(writer.records))
	}
	rec := writer.records[0]
	if rec.Status != AuditStatusPartial {
		t.Errorf("status = %q, want partial", rec.Status)
	}
	if rec.Error == "" {
		t.Error("audit error should be populated")
	}
}

func TestExecuteWithAudit_FirstStepFailureMarkedFailed(t *testing.T) {
	plan := OperationPlan{
		Name: "fail-first",
		Steps: []OperationStep{
			{ID: "s1", Title: "Fail first", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1"},
		},
	}
	writer := &fakeAuditWriter{}
	executor := &failingExecutor{failOn: "s1"}

	_, err := ExecuteWithAudit(context.Background(), plan, executor, writer, "")
	if err == nil {
		t.Fatal("expected execution error")
	}
	if writer.records[0].Status != AuditStatusFailed {
		t.Errorf("status = %q, want failed", writer.records[0].Status)
	}
}

func TestExecuteWithAudit_NilWriterSkipsAudit(t *testing.T) {
	plan := OperationPlan{
		Name: "no-audit",
		Steps: []OperationStep{
			{ID: "s1", Title: "OK", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "echo"},
		},
	}
	_, err := ExecuteWithAudit(context.Background(), plan, NewDefaultStepExecutor(), nil, "")
	if err != nil {
		t.Fatalf("nil writer should not error: %v", err)
	}
}

func TestExecuteWithAudit_PreservesDescriptionAndMetadata_Sprint12(t *testing.T) {
	plan := OperationPlan{
		Name: "test-meta",
		Steps: []OperationStep{
			{
				ID:          "s1",
				Title:       "echo a",
				Description: "echo a >/dev/null",
				Kind:        StepKindSystem,
				Risk:        RiskLevelLow,
				Target:      "tgt",
				Command:     "echo",
				Args:        []string{"a"},
				Metadata:    map[string]string{"raw_command": "echo a", "intent": "ensure greeting"},
			},
		},
	}
	writer := &fakeAuditWriter{}
	executor := NewDefaultStepExecutor()

	_, err := ExecuteWithAudit(context.Background(), plan, executor, writer, "alice")
	if err != nil {
		t.Fatalf("ExecuteWithAudit: %v", err)
	}
	if len(writer.records) != 1 || len(writer.records[0].Steps) != 1 {
		t.Fatalf("unexpected audit shape: %+v", writer.records)
	}
	got := writer.records[0].Steps[0]
	if got.Description != "echo a >/dev/null" {
		t.Errorf("Description = %q, want %q", got.Description, "echo a >/dev/null")
	}
	if got.Metadata["raw_command"] != "echo a" || got.Metadata["intent"] != "ensure greeting" {
		t.Errorf("Metadata not preserved: %v", got.Metadata)
	}
}

func TestExecuteWithAudit_DryRunIncludesMetadataFromPlanSteps(t *testing.T) {
	plan := OperationPlan{
		Name:   "test-dry",
		DryRun: true,
		Steps: []OperationStep{
			{
				ID:          "s1",
				Title:       "T",
				Description: "dry-run check",
				Kind:        StepKindSystem,
				Risk:        RiskLevelLow,
				Target:      "t1",
				Command:     "echo",
				Metadata:    map[string]string{"k": "v"},
			},
		},
	}
	writer := &fakeAuditWriter{}
	_, err := ExecuteWithAudit(context.Background(), plan, NewDefaultStepExecutor(), writer, "alice")
	if err != nil {
		t.Fatalf("ExecuteWithAudit dry-run: %v", err)
	}
	if writer.records[0].Steps[0].Description != "dry-run check" {
		t.Error("dry-run audit lost Description")
	}
	if writer.records[0].Steps[0].Metadata["k"] != "v" {
		t.Errorf("dry-run audit lost Metadata: %v", writer.records[0].Steps[0].Metadata)
	}
}

func TestCloneMetadata_NilAndEmpty(t *testing.T) {
	if cloneMetadata(nil) != nil {
		t.Error("nil input should return nil")
	}
	if cloneMetadata(map[string]string{}) != nil {
		t.Error("empty input should return nil (len 0 short-circuit)")
	}
}

func TestCloneMetadata_Independence(t *testing.T) {
	src := map[string]string{"a": "1", "b": "2"}
	dst := cloneMetadata(src)
	dst["a"] = "changed"
	if src["a"] != "1" {
		t.Error("clone is not independent")
	}
}

func TestAuditRecord_MarshalSteps(t *testing.T) {
	rec := AuditRecord{
		Steps: []StepAudit{{ID: "s1", Title: "T", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1"}},
	}
	out, err := rec.MarshalSteps()
	if err != nil {
		t.Fatalf("MarshalSteps failed: %v", err)
	}
	if out == "" || out == "[]" {
		t.Errorf("expected non-empty JSON, got %q", out)
	}

	empty := AuditRecord{}
	out, err = empty.MarshalSteps()
	if err != nil {
		t.Fatalf("MarshalSteps empty failed: %v", err)
	}
	if out != "[]" {
		t.Errorf("empty steps = %q, want []", out)
	}
}
