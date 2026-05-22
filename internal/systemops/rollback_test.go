package systemops

import (
	"context"
	"errors"
	"testing"
)

type rollbackRecorder struct {
	executed   []string
	rolledBack []string
	failOn     string
}

func (r *rollbackRecorder) ExecuteStep(ctx context.Context, step OperationStep) error {
	if step.ID == r.failOn {
		return errors.New("forced failure")
	}
	// Distinguish rollback steps by suffix
	if len(step.ID) > 9 && step.ID[len(step.ID)-9:] == "-rollback" {
		r.rolledBack = append(r.rolledBack, step.ID)
	} else {
		r.executed = append(r.executed, step.ID)
	}
	return nil
}

func TestRollback_OnFailureExecutesInReverseOrder(t *testing.T) {
	plan := OperationPlan{
		Name:              "rollback-test",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{
				ID:              "s1",
				Title:           "Step 1",
				Kind:            StepKindSystem,
				Risk:            RiskLevelLow,
				Target:          "t1",
				Command:         "do",
				RollbackCommand: "undo",
				RollbackArgs:    []string{"s1"},
			},
			{
				ID:              "s2",
				Title:           "Step 2",
				Kind:            StepKindSystem,
				Risk:            RiskLevelLow,
				Target:          "t2",
				Command:         "do",
				RollbackCommand: "undo",
				RollbackArgs:    []string{"s2"},
			},
			{
				ID:      "s3",
				Title:   "Step 3 fails",
				Kind:    StepKindSystem,
				Risk:    RiskLevelLow,
				Target:  "t3",
				Command: "do",
			},
		},
	}

	recorder := &rollbackRecorder{failOn: "s3"}
	result, err := plan.Execute(context.Background(), recorder)
	if err == nil {
		t.Fatal("expected execution error")
	}

	wantExecuted := []string{"s1", "s2"}
	if !equalSlices(recorder.executed, wantExecuted) {
		t.Errorf("executed = %v, want %v", recorder.executed, wantExecuted)
	}

	wantRolledBack := []string{"s2-rollback", "s1-rollback"}
	if !equalSlices(recorder.rolledBack, wantRolledBack) {
		t.Errorf("rolled back = %v, want %v", recorder.rolledBack, wantRolledBack)
	}

	if !result.Steps[0].RolledBack {
		t.Error("step s1 should be marked rolled back")
	}
	if !result.Steps[1].RolledBack {
		t.Error("step s2 should be marked rolled back")
	}
}

func TestRollback_DisabledByDefault(t *testing.T) {
	plan := OperationPlan{
		Name: "no-rollback",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do", RollbackCommand: "undo"},
			{ID: "s2", Title: "Fail", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
		},
	}

	recorder := &rollbackRecorder{failOn: "s2"}
	_, err := plan.Execute(context.Background(), recorder)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(recorder.rolledBack) != 0 {
		t.Errorf("rollback should not run by default, got %v", recorder.rolledBack)
	}
}

func TestRollback_SkipsStepsWithoutRollbackCommand(t *testing.T) {
	plan := OperationPlan{
		Name:              "partial-rollback",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "Has rollback", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do", RollbackCommand: "undo"},
			{ID: "s2", Title: "No rollback", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do"},
			{ID: "s3", Title: "Fails", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t3", Command: "do"},
		},
	}

	recorder := &rollbackRecorder{failOn: "s3"}
	_, err := plan.Execute(context.Background(), recorder)
	if err == nil {
		t.Fatal("expected error")
	}

	if len(recorder.rolledBack) != 1 {
		t.Errorf("expected 1 rollback (s1), got %v", recorder.rolledBack)
	}
	if len(recorder.rolledBack) > 0 && recorder.rolledBack[0] != "s1-rollback" {
		t.Errorf("expected s1-rollback, got %v", recorder.rolledBack)
	}
}

type partialFailExecutor struct {
	executed         []string
	rollbackFailures map[string]bool
}

func (p *partialFailExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	if len(step.ID) > 9 && step.ID[len(step.ID)-9:] == "-rollback" {
		baseID := step.ID[:len(step.ID)-9]
		if p.rollbackFailures[baseID] {
			return errors.New("rollback failed")
		}
		p.executed = append(p.executed, step.ID)
		return nil
	}
	if step.ID == "fail-step" {
		return errors.New("step failed")
	}
	p.executed = append(p.executed, step.ID)
	return nil
}

func TestRollback_RollbackFailureRecordedButContinues(t *testing.T) {
	plan := OperationPlan{
		Name:              "rollback-failure",
		RollbackOnFailure: true,
		Steps: []OperationStep{
			{ID: "s1", Title: "S1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "do", RollbackCommand: "undo"},
			{ID: "s2", Title: "S2", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t2", Command: "do", RollbackCommand: "undo"},
			{ID: "fail-step", Title: "Fail", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t3", Command: "do"},
		},
	}

	exec := &partialFailExecutor{rollbackFailures: map[string]bool{"s2": true}}
	result, err := plan.Execute(context.Background(), exec)
	if err == nil {
		t.Fatal("expected error")
	}

	// s2 rollback should fail (recorded), s1 rollback should succeed
	if result.Steps[1].RollbackError == "" {
		t.Error("s2 should record rollback error")
	}
	if !result.Steps[0].RolledBack {
		t.Error("s1 should still be rolled back even after s2 rollback failed")
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
