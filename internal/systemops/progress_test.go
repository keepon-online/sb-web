package systemops

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type collectingEmitter struct {
	mu     sync.Mutex
	events []ProgressEvent
}

func (c *collectingEmitter) Emit(event ProgressEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *collectingEmitter) Snapshot() []ProgressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ProgressEvent, len(c.events))
	copy(out, c.events)
	return out
}

func TestExecuteWithProgress_EmitsLifecycleEvents(t *testing.T) {
	plan := OperationPlan{
		Name:        "smoke",
		Description: "A test plan",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step 1", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "echo", Args: []string{"a"}},
			{ID: "s2", Title: "Step 2", Kind: StepKindSystem, Risk: RiskLevelMedium, Target: "t2", Command: "echo", Args: []string{"b"}},
		},
	}

	emitter := &collectingEmitter{}
	_, err := ExecuteWithProgress(context.Background(), plan, NewDefaultStepExecutor(), emitter)
	if err != nil {
		t.Fatalf("ExecuteWithProgress failed: %v", err)
	}

	events := emitter.Snapshot()
	// plan_started + (step_started + step_completed) * 2 + plan_completed = 6
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	wantTypes := []ProgressEventType{
		EventPlanStarted,
		EventStepStarted,
		EventStepCompleted,
		EventStepStarted,
		EventStepCompleted,
		EventPlanCompleted,
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Errorf("event[%d].Type = %q, want %q", i, events[i].Type, want)
		}
	}

	// Step events should carry stepID/title
	if events[1].StepID != "s1" || events[1].StepTitle != "Step 1" {
		t.Errorf("step started missing identity: %+v", events[1])
	}
	if events[2].Risk != RiskLevelLow {
		t.Errorf("expected risk low, got %s", events[2].Risk)
	}
}

func TestExecuteWithProgress_EmitsFailureEvents(t *testing.T) {
	plan := OperationPlan{
		Name: "fail-plan",
		Steps: []OperationStep{
			{ID: "s1", Title: "Will fail", Kind: StepKindSystem, Risk: RiskLevelHigh, Target: "t1"},
		},
	}

	emitter := &collectingEmitter{}
	executor := &failingExecutor{failOn: "s1"}

	_, err := ExecuteWithProgress(context.Background(), plan, executor, emitter)
	if err == nil {
		t.Fatal("expected execution error")
	}

	events := emitter.Snapshot()
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d: %+v", len(events), events)
	}

	if events[2].Type != EventStepFailed {
		t.Errorf("expected step_failed, got %s", events[2].Type)
	}
	if events[2].Error == "" {
		t.Error("step_failed event should carry error message")
	}
	if events[3].Type != EventPlanFailed {
		t.Errorf("expected plan_failed, got %s", events[3].Type)
	}
}

func TestExecuteWithProgress_NilEmitterDoesNotPanic(t *testing.T) {
	plan := OperationPlan{
		Name: "nil-emitter",
		Steps: []OperationStep{
			{ID: "s1", Title: "Step", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1", Command: "echo"},
		},
	}
	_, err := ExecuteWithProgress(context.Background(), plan, NewDefaultStepExecutor(), nil)
	if err != nil {
		t.Fatalf("nil emitter should not error: %v", err)
	}
}

func TestProgressEmitterFunc_AdaptsClosure(t *testing.T) {
	called := 0
	var emitter ProgressEmitter = ProgressEmitterFunc(func(event ProgressEvent) {
		called++
	})
	emitter.Emit(ProgressEvent{Type: EventPlanStarted})
	if called != 1 {
		t.Errorf("called = %d, want 1", called)
	}
}

func TestProgressStepExecutor_PropagatesError(t *testing.T) {
	plan := OperationPlan{
		Name: "test",
		Steps: []OperationStep{
			{ID: "fail", Title: "Fail", Kind: StepKindSystem, Risk: RiskLevelLow, Target: "t1"},
		},
	}
	executor := NewProgressStepExecutor(stubExecutor{err: errors.New("boom")}, nil, plan.Name)
	_, err := plan.Execute(context.Background(), executor)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

type stubExecutor struct {
	err error
}

func (s stubExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	return s.err
}
