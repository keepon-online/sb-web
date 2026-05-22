package systemops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// DefaultStepExecutor 默认的步骤执行器
type DefaultStepExecutor struct{}

// NewDefaultStepExecutor 创建默认执行器
func NewDefaultStepExecutor() *DefaultStepExecutor {
	return &DefaultStepExecutor{}
}

// ExecuteStep 执行单个操作步骤
func (e *DefaultStepExecutor) ExecuteStep(ctx context.Context, step OperationStep) error {
	switch step.Kind {
	case StepKindService:
		return e.executeServiceStep(ctx, step)
	case StepKindFile:
		return e.executeFileStep(ctx, step)
	case StepKindSystem:
		return e.executeSystemStep(ctx, step)
	case StepKindFirewall:
		return e.executeFirewallStep(ctx, step)
	case StepKindBinary:
		return e.executeBinaryStep(ctx, step)
	case StepKindScheduler:
		return e.executeSchedulerStep(ctx, step)
	default:
		return fmt.Errorf("unsupported step kind: %s", step.Kind)
	}
}

func (e *DefaultStepExecutor) executeServiceStep(ctx context.Context, step OperationStep) error {
	if step.Command == "" {
		return fmt.Errorf("service step requires command")
	}
	return e.executeCommand(ctx, step.Command, step.Args...)
}

func (e *DefaultStepExecutor) executeFileStep(ctx context.Context, step OperationStep) error {
	content, ok := step.Metadata["content"]
	if !ok {
		return fmt.Errorf("file step requires 'content' in metadata")
	}

	modeStr := step.Metadata["mode"]
	if modeStr == "" {
		modeStr = "0644"
	}

	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid file mode %q: %w", modeStr, err)
	}

	if err := os.WriteFile(step.Target, []byte(content), os.FileMode(mode)); err != nil {
		return fmt.Errorf("write file %s: %w", step.Target, err)
	}

	return nil
}

func (e *DefaultStepExecutor) executeSystemStep(ctx context.Context, step OperationStep) error {
	if step.Command == "" {
		return fmt.Errorf("system step requires command")
	}
	return e.executeCommand(ctx, step.Command, step.Args...)
}

func (e *DefaultStepExecutor) executeFirewallStep(ctx context.Context, step OperationStep) error {
	if step.Command == "" {
		return fmt.Errorf("firewall step requires command")
	}
	return e.executeCommand(ctx, step.Command, step.Args...)
}

func (e *DefaultStepExecutor) executeBinaryStep(ctx context.Context, step OperationStep) error {
	if step.Command == "" {
		return fmt.Errorf("binary step requires command")
	}
	return e.executeCommand(ctx, step.Command, step.Args...)
}

func (e *DefaultStepExecutor) executeSchedulerStep(ctx context.Context, step OperationStep) error {
	if step.Command == "" {
		return fmt.Errorf("scheduler step requires command")
	}
	return e.executeCommand(ctx, step.Command, step.Args...)
}

func (e *DefaultStepExecutor) executeCommand(ctx context.Context, command string, args ...string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("execute %s: %w", command, err)
	}

	return nil
}
