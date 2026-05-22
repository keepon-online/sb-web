package systemops

import (
	"context"
	"fmt"
	"strings"
)

type StepKind string

const (
	StepKindService   StepKind = "service"
	StepKindFirewall  StepKind = "firewall"
	StepKindSystem    StepKind = "system"
	StepKindScheduler StepKind = "scheduler"
	StepKindBinary    StepKind = "binary"
	StepKindFile      StepKind = "file"
)

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type OperationPlan struct {
	Name              string
	Description       string
	DryRun            bool
	RollbackOnFailure bool
	Steps             []OperationStep
}

type OperationStep struct {
	ID          string
	Title       string
	Description string
	Kind        StepKind
	Risk        RiskLevel
	Target      string
	Command     string
	Args        []string
	Metadata    map[string]string

	// ContinueOnError, when true, records the step's error to StepResult and
	// proceeds to the next step instead of aborting the plan. Used for best-effort
	// operations where partial failure is acceptable (e.g. firewall teardown).
	ContinueOnError bool

	// RunIfCommand, when non-empty, is executed via the StepExecutor before
	// the main Command. If it returns nil (exit 0), the main step executes.
	// If it returns an error (non-zero exit), the step is skipped with
	// SkippedReason = "run-if check failed". RunIfCommand failures themselves
	// are never recorded as step errors and never trigger rollback.
	RunIfCommand string
	RunIfArgs    []string

	RollbackCommand  string
	RollbackArgs     []string
	RollbackMetadata map[string]string

	// RollbackRunIfCommand, when non-empty, is executed via the StepExecutor
	// before the rollback fires. Exit 0 → rollback executes. Non-zero exit →
	// rollback is silently skipped (the step's RolledBack flag stays false but
	// no RollbackError is recorded). This lets a step express "only roll back
	// if the side effect actually landed" semantics — e.g. restoring a binary
	// backup only when the .bak file exists, eliminating the need for shell
	// wrappers like `test -f && cp || true`. RunIf probes themselves are not
	// audited and never trigger further rollback.
	RollbackRunIfCommand string
	RollbackRunIfArgs    []string
}

type StepExecutor interface {
	ExecuteStep(ctx context.Context, step OperationStep) error
}

type OperationResult struct {
	Name   string
	DryRun bool
	Steps  []StepResult
}

type StepResult struct {
	ID            string
	Title         string
	Kind          StepKind
	Risk          RiskLevel
	Target        string
	Executed      bool
	SkippedReason string
	Error         string
	RolledBack    bool
	RollbackError string
}

func (p OperationPlan) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("operation plan name is required")
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("operation plan must include at least one step")
	}

	seenStepIDs := make(map[string]struct{}, len(p.Steps))
	for index, step := range p.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", index, err)
		}
		if _, ok := seenStepIDs[step.ID]; ok {
			return fmt.Errorf("step %d: duplicate step id %q", index, step.ID)
		}
		seenStepIDs[step.ID] = struct{}{}
	}

	return nil
}

func (p OperationPlan) Execute(ctx context.Context, executor StepExecutor) (OperationResult, error) {
	if err := p.Validate(); err != nil {
		return OperationResult{}, err
	}
	if !p.DryRun && executor == nil {
		return OperationResult{}, fmt.Errorf("step executor is required")
	}

	result := OperationResult{Name: p.Name, DryRun: p.DryRun}
	for index, step := range p.Steps {
		stepResult := StepResult{
			ID:     step.ID,
			Title:  step.Title,
			Kind:   step.Kind,
			Risk:   step.Risk,
			Target: step.Target,
		}

		if p.DryRun {
			stepResult.SkippedReason = "dry-run"
			result.Steps = append(result.Steps, stepResult)
			continue
		}

		if step.RunIfCommand != "" {
			probe := OperationStep{
				ID:      step.ID + "-runif",
				Title:   "RunIf: " + step.Title,
				Kind:    step.Kind,
				Risk:    step.Risk,
				Target:  step.Target,
				Command: step.RunIfCommand,
				Args:    step.RunIfArgs,
			}
			if err := executor.ExecuteStep(ctx, probe); err != nil {
				stepResult.SkippedReason = "run-if check failed"
				result.Steps = append(result.Steps, stepResult)
				continue
			}
		}

		if err := executor.ExecuteStep(ctx, step); err != nil {
			stepResult.Executed = true
			stepResult.Error = err.Error()
			result.Steps = append(result.Steps, stepResult)

			if step.ContinueOnError {
				continue
			}

			if p.RollbackOnFailure {
				p.rollbackCompleted(ctx, executor, &result, index)
			}
			return result, fmt.Errorf("execute step %q: %w", step.ID, err)
		}

		stepResult.Executed = true
		result.Steps = append(result.Steps, stepResult)
	}

	return result, nil
}

// rollbackCompleted iterates already-completed steps in reverse order and runs
// their rollback commands. Rollback errors are recorded but never abort the loop.
//
// When a step declares RollbackRunIfCommand the probe runs first; a non-zero
// probe exit silently skips the rollback (no RolledBack flag, no RollbackError).
func (p OperationPlan) rollbackCompleted(ctx context.Context, executor StepExecutor, result *OperationResult, failedIndex int) {
	for i := failedIndex - 1; i >= 0; i-- {
		step := p.Steps[i]
		if step.RollbackCommand == "" {
			continue
		}

		if step.RollbackRunIfCommand != "" {
			probe := OperationStep{
				ID:      step.ID + "-rollback-runif",
				Title:   "RollbackRunIf: " + step.Title,
				Kind:    step.Kind,
				Risk:    step.Risk,
				Target:  step.Target,
				Command: step.RollbackRunIfCommand,
				Args:    step.RollbackRunIfArgs,
			}
			if err := executor.ExecuteStep(ctx, probe); err != nil {
				continue
			}
		}

		rollbackStep := OperationStep{
			ID:       step.ID + "-rollback",
			Title:    "Rollback: " + step.Title,
			Kind:     step.Kind,
			Risk:     step.Risk,
			Target:   step.Target,
			Command:  step.RollbackCommand,
			Args:     step.RollbackArgs,
			Metadata: step.RollbackMetadata,
		}

		if i < len(result.Steps) {
			if err := executor.ExecuteStep(ctx, rollbackStep); err != nil {
				result.Steps[i].RollbackError = err.Error()
			} else {
				result.Steps[i].RolledBack = true
			}
		}
	}
}

func (s OperationStep) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if !s.Kind.Valid() {
		return fmt.Errorf("invalid step kind %q", s.Kind)
	}
	if !s.Risk.Valid() {
		return fmt.Errorf("invalid risk level %q", s.Risk)
	}
	if strings.TrimSpace(s.Target) == "" {
		return fmt.Errorf("target is required")
	}
	return nil
}

func (k StepKind) Valid() bool {
	switch k {
	case StepKindService, StepKindFirewall, StepKindSystem, StepKindScheduler, StepKindBinary, StepKindFile:
		return true
	default:
		return false
	}
}

func (r RiskLevel) Valid() bool {
	switch r {
	case RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical:
		return true
	default:
		return false
	}
}
