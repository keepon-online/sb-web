package systemops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestIntegration_FileAndSystemSteps 集成测试：文件写入 + 系统命令
func TestIntegration_FileAndSystemSteps(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.conf")

	plan := OperationPlan{
		Name: "Write config and verify",
		Steps: []OperationStep{
			{
				ID:     "write-config",
				Title:  "Write configuration file",
				Kind:   StepKindFile,
				Risk:   RiskLevelLow,
				Target: testFile,
				Metadata: map[string]string{
					"content": "test=value\n",
					"mode":    "0644",
				},
			},
			{
				ID:      "verify-file",
				Title:   "Verify file exists",
				Kind:    StepKindSystem,
				Risk:    RiskLevelLow,
				Target:  "filesystem",
				Command: "test",
				Args:    []string{"-f", testFile},
			},
		},
	}

	executor := NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		t.Fatalf("plan execution failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps in result, got %d", len(result.Steps))
	}

	for _, step := range result.Steps {
		if !step.Executed {
			t.Errorf("step %s was not executed", step.ID)
		}
		if step.Error != "" {
			t.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	// 验证文件确实被创建
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	if string(content) != "test=value\n" {
		t.Errorf("file content = %q, want %q", string(content), "test=value\\n")
	}
}

// TestIntegration_DryRunDoesNotModifyFilesystem 验证 dry-run 不修改文件系统
func TestIntegration_DryRunDoesNotModifyFilesystem(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "should-not-exist.txt")

	plan := OperationPlan{
		Name:   "Dry run test",
		DryRun: true,
		Steps: []OperationStep{
			{
				ID:     "write-file",
				Title:  "Write file",
				Kind:   StepKindFile,
				Risk:   RiskLevelLow,
				Target: testFile,
				Metadata: map[string]string{
					"content": "should not be written",
				},
			},
		},
	}

	executor := NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		t.Fatalf("dry-run execution failed: %v", err)
	}

	if !result.DryRun {
		t.Error("result should be marked as dry-run")
	}

	// 验证文件不存在
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("dry-run should not create files")
	}
}
