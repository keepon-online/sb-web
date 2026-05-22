package systemops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultStepExecutor_ExecuteFileStep(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	executor := NewDefaultStepExecutor()
	step := OperationStep{
		ID:     "write-file",
		Title:  "Write test file",
		Kind:   StepKindFile,
		Risk:   RiskLevelLow,
		Target: testFile,
		Metadata: map[string]string{
			"content": "test content",
			"mode":    "0644",
		},
	}

	err := executor.ExecuteStep(context.Background(), step)
	if err != nil {
		t.Fatalf("ExecuteStep failed: %v", err)
	}

	// 验证文件已创建
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("file was not created")
	}

	// 验证文件内容
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("file content = %q, want %q", string(content), "test content")
	}
}

func TestDefaultStepExecutor_ExecuteFileStep_MissingContent(t *testing.T) {
	executor := NewDefaultStepExecutor()
	step := OperationStep{
		ID:       "write-file",
		Title:    "Write test file",
		Kind:     StepKindFile,
		Risk:     RiskLevelLow,
		Target:   "/tmp/test.txt",
		Metadata: map[string]string{},
	}

	err := executor.ExecuteStep(context.Background(), step)
	if err == nil {
		t.Fatal("expected error for missing content, got nil")
	}
}

func TestDefaultStepExecutor_ExecuteFileStep_InvalidMode(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	executor := NewDefaultStepExecutor()
	step := OperationStep{
		ID:     "write-file",
		Title:  "Write test file",
		Kind:   StepKindFile,
		Risk:   RiskLevelLow,
		Target: testFile,
		Metadata: map[string]string{
			"content": "test",
			"mode":    "invalid",
		},
	}

	err := executor.ExecuteStep(context.Background(), step)
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
}

func TestDefaultStepExecutor_ExecuteSystemStep(t *testing.T) {
	executor := NewDefaultStepExecutor()
	step := OperationStep{
		ID:      "echo-test",
		Title:   "Echo test",
		Kind:    StepKindSystem,
		Risk:    RiskLevelLow,
		Target:  "system",
		Command: "echo",
		Args:    []string{"test"},
	}

	err := executor.ExecuteStep(context.Background(), step)
	if err != nil {
		t.Fatalf("ExecuteStep failed: %v", err)
	}
}

func TestDefaultStepExecutor_ExecuteSystemStep_MissingCommand(t *testing.T) {
	executor := NewDefaultStepExecutor()
	step := OperationStep{
		ID:     "no-command",
		Title:  "No command",
		Kind:   StepKindSystem,
		Risk:   RiskLevelLow,
		Target: "system",
	}

	err := executor.ExecuteStep(context.Background(), step)
	if err == nil {
		t.Fatal("expected error for missing command, got nil")
	}
}

func TestDefaultStepExecutor_UnsupportedStepKind(t *testing.T) {
	executor := NewDefaultStepExecutor()
	step := OperationStep{
		ID:     "unsupported",
		Title:  "Unsupported",
		Kind:   StepKind("unsupported"),
		Risk:   RiskLevelLow,
		Target: "target",
	}

	err := executor.ExecuteStep(context.Background(), step)
	if err == nil {
		t.Fatal("expected error for unsupported step kind, got nil")
	}
}

func TestDefaultStepExecutor_ContextCancellation(t *testing.T) {
	executor := NewDefaultStepExecutor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	step := OperationStep{
		ID:      "sleep",
		Title:   "Sleep",
		Kind:    StepKindSystem,
		Risk:    RiskLevelLow,
		Target:  "system",
		Command: "sleep",
		Args:    []string{"10"},
	}

	err := executor.ExecuteStep(ctx, step)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
