package systemops

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AuditRecord captures a single OperationPlan execution.
type AuditRecord struct {
	ID         int64       `json:"id"`
	PlanName   string      `json:"plan_name"`
	DryRun     bool        `json:"dry_run"`
	Status     string      `json:"status"` // success, failed, partial
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt time.Time   `json:"finished_at"`
	Steps      []StepAudit `json:"steps"`
	Error      string      `json:"error,omitempty"`
	Username   string      `json:"username,omitempty"`
}

// StepAudit records a single step's outcome.
//
// Sprint 12 extension: `Description` + `Metadata` carry the human-readable
// command transcript and the step's arbitrary key/value metadata (e.g.
// raw_command, payload, intent). They are JSON-encoded into the existing
// steps_json TEXT column; older audit rows decode with zero values so the
// schema migration is implicit.
type StepAudit struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Description   string            `json:"description,omitempty"`
	Kind          StepKind          `json:"kind"`
	Risk          RiskLevel         `json:"risk"`
	Target        string            `json:"target"`
	Executed      bool              `json:"executed"`
	SkippedReason string            `json:"skipped_reason,omitempty"`
	Error         string            `json:"error,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// AuditStatus values.
const (
	AuditStatusSuccess = "success"
	AuditStatusFailed  = "failed"
	AuditStatusPartial = "partial"
)

// AuditWriter persists audit records. Storage layer implements this.
type AuditWriter interface {
	WriteAudit(ctx context.Context, record AuditRecord) (int64, error)
}

// AuditingStepExecutor wraps a StepExecutor and records timing for each step.
// The collected step audits can be retrieved via Drain after plan execution.
type AuditingStepExecutor struct {
	inner StepExecutor
	audits []StepAudit
}

// NewAuditingStepExecutor wraps the given executor.
func NewAuditingStepExecutor(inner StepExecutor) *AuditingStepExecutor {
	return &AuditingStepExecutor{inner: inner}
}

// ExecuteStep delegates to inner executor while recording timing.
func (a *AuditingStepExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	startedAt := time.Now().UTC()
	err := a.inner.ExecuteStep(ctx, step)
	finishedAt := time.Now().UTC()

	audit := StepAudit{
		ID:          step.ID,
		Title:       step.Title,
		Description: step.Description,
		Kind:        step.Kind,
		Risk:        step.Risk,
		Target:      step.Target,
		Executed:    true,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		Metadata:    cloneMetadata(step.Metadata),
	}
	if err != nil {
		audit.Error = err.Error()
	}
	a.audits = append(a.audits, audit)
	return err
}

// Drain returns and clears the recorded step audits.
func (a *AuditingStepExecutor) Drain() []StepAudit {
	out := a.audits
	a.audits = nil
	return out
}

// ExecuteWithAudit runs a plan, persists an audit record, and returns the result.
//
// Use this as the canonical entrypoint when audit is required.
func ExecuteWithAudit(
	ctx context.Context,
	plan OperationPlan,
	executor StepExecutor,
	writer AuditWriter,
	username string,
) (OperationResult, error) {
	if writer == nil {
		return plan.Execute(ctx, executor)
	}

	auditing := NewAuditingStepExecutor(executor)
	startedAt := time.Now().UTC()

	var (
		result OperationResult
		execErr error
	)
	if plan.DryRun {
		result, execErr = plan.Execute(ctx, executor)
	} else {
		result, execErr = plan.Execute(ctx, auditing)
	}

	finishedAt := time.Now().UTC()
	stepAudits := auditing.Drain()

	if plan.DryRun {
		stepAudits = stepAuditsFromResult(result, plan.Steps, startedAt, finishedAt)
	}

	record := AuditRecord{
		PlanName:   plan.Name,
		DryRun:     plan.DryRun,
		Status:     deriveAuditStatus(result, execErr, plan.DryRun),
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Steps:      stepAudits,
		Username:   username,
	}
	if execErr != nil {
		record.Error = execErr.Error()
	}

	if _, writeErr := writer.WriteAudit(ctx, record); writeErr != nil {
		return result, fmt.Errorf("write audit (plan=%s, exec_err=%v): %w", plan.Name, execErr, writeErr)
	}

	return result, execErr
}

func stepAuditsFromResult(result OperationResult, planSteps []OperationStep, startedAt, finishedAt time.Time) []StepAudit {
	// Index plan steps by ID so we can fish out Description/Metadata that
	// the runtime StepResult does not carry.
	originals := make(map[string]OperationStep, len(planSteps))
	for _, s := range planSteps {
		originals[s.ID] = s
	}

	audits := make([]StepAudit, 0, len(result.Steps))
	for _, step := range result.Steps {
		original := originals[step.ID]
		audits = append(audits, StepAudit{
			ID:            step.ID,
			Title:         step.Title,
			Description:   original.Description,
			Kind:          step.Kind,
			Risk:          step.Risk,
			Target:        step.Target,
			Executed:      step.Executed,
			SkippedReason: step.SkippedReason,
			Error:         step.Error,
			StartedAt:     startedAt,
			FinishedAt:    finishedAt,
			Metadata:      cloneMetadata(original.Metadata),
		})
	}
	return audits
}

// cloneMetadata returns a shallow copy of m. The audit layer must own a
// distinct map so later mutations of the source step (improbable but easy to
// guard against) cannot reach back into a persisted record.
func cloneMetadata(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func deriveAuditStatus(result OperationResult, execErr error, dryRun bool) string {
	if dryRun {
		return AuditStatusSuccess
	}
	if execErr == nil {
		return AuditStatusSuccess
	}
	for _, step := range result.Steps {
		if step.Executed && step.Error == "" {
			return AuditStatusPartial
		}
	}
	return AuditStatusFailed
}

// MarshalSteps returns the JSON serialization of step audits, used by storage layer.
func (r AuditRecord) MarshalSteps() (string, error) {
	if len(r.Steps) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(r.Steps)
	if err != nil {
		return "", fmt.Errorf("marshal steps: %w", err)
	}
	return string(data), nil
}
