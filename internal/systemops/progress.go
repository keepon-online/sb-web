package systemops

import (
	"context"
	"encoding/json"
	"time"
)

// ProgressEventType represents the lifecycle stage of a step or plan.
type ProgressEventType string

const (
	EventPlanStarted    ProgressEventType = "plan_started"
	EventStepStarted    ProgressEventType = "step_started"
	EventStepCompleted  ProgressEventType = "step_completed"
	EventStepFailed     ProgressEventType = "step_failed"
	EventStepSkipped    ProgressEventType = "step_skipped"
	EventPlanCompleted  ProgressEventType = "plan_completed"
	EventPlanFailed     ProgressEventType = "plan_failed"
	EventStepRolledBack ProgressEventType = "step_rolled_back"
)

// ProgressEvent is emitted by ProgressStepExecutor for every lifecycle change.
type ProgressEvent struct {
	Type      ProgressEventType `json:"type"`
	PlanName  string            `json:"plan_name,omitempty"`
	StepID    string            `json:"step_id,omitempty"`
	StepTitle string            `json:"step_title,omitempty"`
	Risk      RiskLevel         `json:"risk,omitempty"`
	Message   string            `json:"message,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// ProgressEmitter receives progress events. Implementations must be safe for
// concurrent use if used across goroutines.
type ProgressEmitter interface {
	Emit(event ProgressEvent)
}

// ProgressEmitterFunc adapts a plain function to ProgressEmitter.
type ProgressEmitterFunc func(event ProgressEvent)

// Emit implements ProgressEmitter.
func (f ProgressEmitterFunc) Emit(event ProgressEvent) {
	f(event)
}

// ProgressStepExecutor wraps a StepExecutor and emits ProgressEvent for each step.
type ProgressStepExecutor struct {
	inner    StepExecutor
	emitter  ProgressEmitter
	planName string
}

// NewProgressStepExecutor wraps the given executor with progress reporting.
func NewProgressStepExecutor(inner StepExecutor, emitter ProgressEmitter, planName string) *ProgressStepExecutor {
	return &ProgressStepExecutor{inner: inner, emitter: emitter, planName: planName}
}

// ExecuteStep emits step_started and step_completed/step_failed events.
func (p *ProgressStepExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	if p.emitter != nil {
		p.emitter.Emit(ProgressEvent{
			Type:      EventStepStarted,
			PlanName:  p.planName,
			StepID:    step.ID,
			StepTitle: step.Title,
			Risk:      step.Risk,
			Timestamp: time.Now().UTC(),
		})
	}

	err := p.inner.ExecuteStep(ctx, step)

	if p.emitter == nil {
		return err
	}

	event := ProgressEvent{
		PlanName:  p.planName,
		StepID:    step.ID,
		StepTitle: step.Title,
		Risk:      step.Risk,
		Timestamp: time.Now().UTC(),
	}
	if err != nil {
		event.Type = EventStepFailed
		event.Error = err.Error()
	} else {
		event.Type = EventStepCompleted
	}
	p.emitter.Emit(event)
	return err
}

// ExecuteWithProgress runs a plan and emits plan-level + step-level progress events.
func ExecuteWithProgress(
	ctx context.Context,
	plan OperationPlan,
	executor StepExecutor,
	emitter ProgressEmitter,
) (OperationResult, error) {
	emit := func(event ProgressEvent) {
		if emitter != nil {
			emitter.Emit(event)
		}
	}

	emit(ProgressEvent{
		Type:      EventPlanStarted,
		PlanName:  plan.Name,
		Message:   plan.Description,
		Timestamp: time.Now().UTC(),
	})

	progressing := NewProgressStepExecutor(executor, emitter, plan.Name)
	result, err := plan.Execute(ctx, progressing)

	finalEvent := ProgressEvent{
		PlanName:  plan.Name,
		Timestamp: time.Now().UTC(),
	}
	if err != nil {
		finalEvent.Type = EventPlanFailed
		finalEvent.Error = err.Error()
	} else {
		finalEvent.Type = EventPlanCompleted
	}
	emit(finalEvent)

	return result, err
}

// MarshalJSON allows ProgressEvent to be sent through SSE handlers.
func (e ProgressEvent) MarshalJSON() ([]byte, error) {
	type alias ProgressEvent
	return json.Marshal(alias(e))
}
