package systemops

import (
	"context"
	"errors"
	"testing"
)

type behaviorRecorder struct {
	executed []string
	failOn   map[string]error
}

func (r *behaviorRecorder) ExecuteStep(ctx context.Context, step OperationStep) error {
	r.executed = append(r.executed, step.ID)
	if err, ok := r.failOn[step.ID]; ok {
		return err
	}
	return nil
}

func TestContinueOnError_RecordsErrorAndProceeds(t *testing.T) {
	plan := OperationPlan{
		Name: "best-effort",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do"},
			{ID: "s2", Title: "Step 2 fails (continue)", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do", ContinueOnError: true},
			{ID: "s3", Title: "Step 3", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t3", Command: "do"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{"s2": errors.New("boom")}}

	result, err := plan.Execute(context.Background(), rec)
	if err != nil {
		t.Fatalf("expected nil err with ContinueOnError, got %v", err)
	}
	if got := []string{"s1", "s2", "s3"}; !equalSlices(rec.executed, got) {
		t.Errorf("executed = %v, want %v", rec.executed, got)
	}
	if result.Steps[1].Error != "boom" {
		t.Errorf("step s2 error = %q, want %q", result.Steps[1].Error, "boom")
	}
	if !result.Steps[1].Executed {
		t.Error("step s2 should be marked executed even on error")
	}
	if result.Steps[2].Error != "" {
		t.Errorf("step s3 should have no error, got %q", result.Steps[2].Error)
	}
}

func TestContinueOnError_DoesNotTriggerRollback(t *testing.T) {
	plan := OperationPlan{
		Name:              "no-rollback-on-continue",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do", RollbackCommand: "undo"},
			{ID: "s2", Title: "Step 2 fails (continue)", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do", ContinueOnError: true},
			{ID: "s3", Title: "Step 3", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t3", Command: "do", RollbackCommand: "undo"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{"s2": errors.New("boom")}}

	result, _ := plan.Execute(context.Background(), rec)
	for _, sr := range result.Steps {
		if sr.RolledBack {
			t.Errorf("step %s should not be rolled back when ContinueOnError fired", sr.ID)
		}
	}
}

func TestRunIfCommand_SkipsStepWhenProbeFails(t *testing.T) {
	plan := OperationPlan{
		Name: "run-if-skip",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do",
				RunIfCommand: "probe", RunIfArgs: []string{"check"}},
			{ID: "s2", Title: "Step 2", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{"s1-runif": errors.New("probe failed")}}

	result, err := plan.Execute(context.Background(), rec)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if got := []string{"s1-runif", "s2"}; !equalSlices(rec.executed, got) {
		t.Errorf("executed = %v, want %v (s1 main should NOT run after probe failed)", rec.executed, got)
	}
	if result.Steps[0].Executed {
		t.Error("step s1 should not be marked executed (probe failed)")
	}
	if result.Steps[0].SkippedReason != "run-if check failed" {
		t.Errorf("step s1 SkippedReason = %q, want %q", result.Steps[0].SkippedReason, "run-if check failed")
	}
}

func TestRunIfCommand_ExecutesStepWhenProbePasses(t *testing.T) {
	plan := OperationPlan{
		Name: "run-if-execute",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do",
				RunIfCommand: "probe"},
		},
	}
	rec := &behaviorRecorder{}

	result, err := plan.Execute(context.Background(), rec)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if got := []string{"s1-runif", "s1"}; !equalSlices(rec.executed, got) {
		t.Errorf("executed = %v, want %v (probe then main)", rec.executed, got)
	}
	if !result.Steps[0].Executed {
		t.Error("step s1 should be marked executed")
	}
}

func TestRunIfCommand_DoesNotRunInDryRun(t *testing.T) {
	plan := OperationPlan{
		Name:   "run-if-dryrun",
		DryRun: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do",
				RunIfCommand: "probe"},
		},
	}
	rec := &behaviorRecorder{}

	result, err := plan.Execute(context.Background(), rec)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(rec.executed) != 0 {
		t.Errorf("dry-run should not execute anything (including probe), executed = %v", rec.executed)
	}
	if result.Steps[0].SkippedReason != "dry-run" {
		t.Errorf("step s1 SkippedReason = %q, want %q", result.Steps[0].SkippedReason, "dry-run")
	}
}

func TestRollbackRunIf_SkipsRollbackWhenProbeFails(t *testing.T) {
	plan := OperationPlan{
		Name:              "rollback-runif-skip",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do",
				RollbackCommand:      "undo",
				RollbackRunIfCommand: "probe"},
			{ID: "s2", Title: "Step 2 fails", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{
		"s2":                errors.New("boom"),
		"s1-rollback-runif": errors.New("probe failed"),
	}}

	result, _ := plan.Execute(context.Background(), rec)
	// Probe ran but rollback main did not.
	wantExec := []string{"s1", "s2", "s1-rollback-runif"}
	if !equalSlices(rec.executed, wantExec) {
		t.Errorf("executed = %v, want %v", rec.executed, wantExec)
	}
	if result.Steps[0].RolledBack {
		t.Error("step s1 RolledBack should stay false when probe blocks rollback")
	}
	if result.Steps[0].RollbackError != "" {
		t.Errorf("RollbackError should be empty when probe blocks rollback, got %q", result.Steps[0].RollbackError)
	}
}

func TestRollbackRunIf_ExecutesRollbackWhenProbePasses(t *testing.T) {
	plan := OperationPlan{
		Name:              "rollback-runif-execute",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do",
				RollbackCommand:      "undo",
				RollbackRunIfCommand: "probe"},
			{ID: "s2", Title: "Step 2 fails", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{"s2": errors.New("boom")}}

	result, _ := plan.Execute(context.Background(), rec)
	wantExec := []string{"s1", "s2", "s1-rollback-runif", "s1-rollback"}
	if !equalSlices(rec.executed, wantExec) {
		t.Errorf("executed = %v, want %v", rec.executed, wantExec)
	}
	if !result.Steps[0].RolledBack {
		t.Error("step s1 should be marked rolled back")
	}
}

func TestRollbackRunIf_DoesNotAffectStepsWithoutProbe(t *testing.T) {
	plan := OperationPlan{
		Name:              "rollback-runif-mixed",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do",
				RollbackCommand: "undo"},
			{ID: "s2", Title: "Step 2 fails", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{"s2": errors.New("boom")}}

	result, _ := plan.Execute(context.Background(), rec)
	wantExec := []string{"s1", "s2", "s1-rollback"}
	if !equalSlices(rec.executed, wantExec) {
		t.Errorf("executed = %v, want %v (no probe should run)", rec.executed, wantExec)
	}
	if !result.Steps[0].RolledBack {
		t.Error("step s1 should be marked rolled back (no probe)")
	}
}

func TestContinueOnError_BackwardsCompatible(t *testing.T) {
	// Without ContinueOnError, behavior must match Sprint 1: plan aborts on first error.
	plan := OperationPlan{
		Name: "abort-on-error",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do"},
			{ID: "s2", Title: "Step 2 fails", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
			{ID: "s3", Title: "Step 3 (should NOT run)", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t3", Command: "do"},
		},
	}
	rec := &behaviorRecorder{failOn: map[string]error{"s2": errors.New("boom")}}

	_, err := plan.Execute(context.Background(), rec)
	if err == nil {
		t.Fatal("expected error to abort plan")
	}
	if got := []string{"s1", "s2"}; !equalSlices(rec.executed, got) {
		t.Errorf("executed = %v, want %v (s3 must not run)", rec.executed, got)
	}
}
