package systemops

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestOperationPlanValidate(t *testing.T) {
	tests := []struct {
		name    string
		plan    OperationPlan
		wantErr bool
	}{
		{
			name: "valid plan",
			plan: OperationPlan{
				Name: "restart sing-box",
				Steps: []OperationStep{
					{
						ID:     "restart-service",
						Title:  "Restart sing-box service",
						Kind:   StepKindService,
						Risk:   RiskLevelMedium,
						Target: "sing-box",
					},
				},
			},
		},
		{
			name: "missing name",
			plan: OperationPlan{
				Steps: []OperationStep{
					{ID: "restart-service", Title: "Restart", Kind: StepKindService, Risk: RiskLevelLow, Target: "sing-box"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty steps",
			plan: OperationPlan{
				Name: "restart sing-box",
			},
			wantErr: true,
		},
		{
			name: "missing step id",
			plan: OperationPlan{
				Name: "restart sing-box",
				Steps: []OperationStep{
					{Title: "Restart", Kind: StepKindService, Risk: RiskLevelLow, Target: "sing-box"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid step kind",
			plan: OperationPlan{
				Name: "restart sing-box",
				Steps: []OperationStep{
					{ID: "restart-service", Title: "Restart", Kind: StepKind("bad"), Risk: RiskLevelLow, Target: "sing-box"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid risk level",
			plan: OperationPlan{
				Name: "restart sing-box",
				Steps: []OperationStep{
					{ID: "restart-service", Title: "Restart", Kind: StepKindService, Risk: RiskLevel("bad"), Target: "sing-box"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing target",
			plan: OperationPlan{
				Name: "restart sing-box",
				Steps: []OperationStep{
					{ID: "restart-service", Title: "Restart", Kind: StepKindService, Risk: RiskLevelLow},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate step id",
			plan: OperationPlan{
				Name: "restart sing-box",
				Steps: []OperationStep{
					{ID: "restart-service", Title: "Restart", Kind: StepKindService, Risk: RiskLevelLow, Target: "sing-box"},
					{ID: "restart-service", Title: "Enable", Kind: StepKindService, Risk: RiskLevelLow, Target: "sing-box"},
				},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.plan.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestOperationPlanDryRunDoesNotExecuteSteps(t *testing.T) {
	plan := OperationPlan{
		Name:   "restart sing-box",
		DryRun: true,
		Steps: []OperationStep{
			{ID: "restart-service", Title: "Restart", Kind: StepKindService, Risk: RiskLevelMedium, Target: "sing-box"},
			{ID: "reload-systemd", Title: "Reload systemd", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "systemd"},
		},
	}
	executor := &recordingStepExecutor{}

	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if len(executor.executed) != 0 {
		t.Fatalf("dry-run executed steps = %#v", executor.executed)
	}
	if !result.DryRun {
		t.Fatal("result should be marked as dry-run")
	}
	if len(result.Steps) != 2 {
		t.Fatalf("result steps = %d, want 2", len(result.Steps))
	}
	for _, step := range result.Steps {
		if step.Executed {
			t.Fatalf("dry-run step %s should not be executed", step.ID)
		}
		if step.SkippedReason != "dry-run" {
			t.Fatalf("dry-run step reason = %q, want dry-run", step.SkippedReason)
		}
	}
}

func TestOperationPlanExecuteRunsStepsInOrder(t *testing.T) {
	plan := OperationPlan{
		Name: "restart sing-box",
		Steps: []OperationStep{
			{ID: "stop-service", Title: "Stop", Kind: StepKindService, Risk: RiskLevelMedium, Target: "sing-box"},
			{ID: "start-service", Title: "Start", Kind: StepKindService, Risk: RiskLevelMedium, Target: "sing-box"},
		},
	}
	executor := &recordingStepExecutor{}

	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	want := []string{"stop-service", "start-service"}
	if !reflect.DeepEqual(executor.executed, want) {
		t.Fatalf("executed = %#v, want %#v", executor.executed, want)
	}
	if result.DryRun {
		t.Fatal("result should not be marked as dry-run")
	}
	for _, step := range result.Steps {
		if !step.Executed {
			t.Fatalf("step %s should be executed", step.ID)
		}
	}
}

func TestOperationPlanExecuteRequiresExecutor(t *testing.T) {
	plan := OperationPlan{
		Name: "restart sing-box",
		Steps: []OperationStep{
			{ID: "restart-service", Title: "Restart", Kind: StepKindService, Risk: RiskLevelMedium, Target: "sing-box"},
		},
	}

	_, err := plan.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected missing executor error")
	}
}

func TestOperationPlanExecuteStopsOnStepError(t *testing.T) {
	plan := OperationPlan{
		Name: "restart sing-box",
		Steps: []OperationStep{
			{ID: "stop-service", Title: "Stop", Kind: StepKindService, Risk: RiskLevelMedium, Target: "sing-box"},
			{ID: "start-service", Title: "Start", Kind: StepKindService, Risk: RiskLevelMedium, Target: "sing-box"},
		},
	}
	executor := &recordingStepExecutor{failOn: "stop-service"}

	result, err := plan.Execute(context.Background(), executor)
	if err == nil {
		t.Fatal("expected step execution error")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("result steps = %d, want 1", len(result.Steps))
	}
	if result.Steps[0].Error == "" {
		t.Fatal("failed step should include error text")
	}
}

type recordingStepExecutor struct {
	executed []string
	failOn   string
}

func (e *recordingStepExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	e.executed = append(e.executed, step.ID)
	if step.ID == e.failOn {
		return errors.New("step failed")
	}
	return nil
}
