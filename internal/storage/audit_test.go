package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"miaomiaowu/internal/systemops"
)

func newTestRepo(t *testing.T) *TrafficRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	repo, err := NewTrafficRepository(dbPath)
	if err != nil {
		t.Fatalf("NewTrafficRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestWriteAuditAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rec := systemops.AuditRecord{
		PlanName:   "install-singbox",
		DryRun:     false,
		Status:     systemops.AuditStatusSuccess,
		StartedAt:  now,
		FinishedAt: now.Add(2 * time.Second),
		Username:   "alice",
		Steps: []systemops.StepAudit{
			{
				ID:         "write-service-file",
				Title:      "Write service file",
				Kind:       systemops.StepKindFile,
				Risk:       systemops.RiskLevelHigh,
				Target:     "/etc/systemd/system/sing-box.service",
				Executed:   true,
				StartedAt:  now,
				FinishedAt: now.Add(1 * time.Second),
			},
		},
	}

	id, err := repo.WriteAudit(ctx, rec)
	if err != nil {
		t.Fatalf("WriteAudit failed: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected id > 0, got %d", id)
	}

	got, err := repo.GetAudit(ctx, id)
	if err != nil {
		t.Fatalf("GetAudit failed: %v", err)
	}
	if got.PlanName != rec.PlanName {
		t.Errorf("plan_name = %q, want %q", got.PlanName, rec.PlanName)
	}
	if got.Username != rec.Username {
		t.Errorf("username = %q, want %q", got.Username, rec.Username)
	}
	if got.Status != rec.Status {
		t.Errorf("status = %q, want %q", got.Status, rec.Status)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got.Steps))
	}
	if got.Steps[0].ID != "write-service-file" {
		t.Errorf("step id = %q, want write-service-file", got.Steps[0].ID)
	}

	list, err := repo.ListAudits(ctx, AuditQueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudits failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 audit in list, got %d", len(list))
	}
}

func TestListAuditsFilters(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i, status := range []string{
		systemops.AuditStatusSuccess,
		systemops.AuditStatusFailed,
		systemops.AuditStatusSuccess,
	} {
		_, err := repo.WriteAudit(ctx, systemops.AuditRecord{
			PlanName:   "p" + string(rune('A'+i)),
			Status:     status,
			StartedAt:  now,
			FinishedAt: now,
		})
		if err != nil {
			t.Fatalf("WriteAudit %d: %v", i, err)
		}
	}

	successList, err := repo.ListAudits(ctx, AuditQueryOptions{Status: systemops.AuditStatusSuccess})
	if err != nil {
		t.Fatalf("ListAudits success: %v", err)
	}
	if len(successList) != 2 {
		t.Errorf("success count = %d, want 2", len(successList))
	}

	failedList, err := repo.ListAudits(ctx, AuditQueryOptions{Status: systemops.AuditStatusFailed})
	if err != nil {
		t.Fatalf("ListAudits failed: %v", err)
	}
	if len(failedList) != 1 {
		t.Errorf("failed count = %d, want 1", len(failedList))
	}
}

func TestGetAuditNotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.GetAudit(context.Background(), 99999)
	if err != ErrAuditNotFound {
		t.Errorf("expected ErrAuditNotFound, got %v", err)
	}
}

func TestExecuteWithAuditPersistsToRepo(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	plan := systemops.OperationPlan{
		Name: "smoke-plan",
		Steps: []systemops.OperationStep{
			{ID: "echo", Title: "Echo", Kind: systemops.StepKindSystem, Risk: systemops.RiskLevelLow, Target: "system", Command: "echo", Args: []string{"ok"}},
		},
	}
	executor := systemops.NewDefaultStepExecutor()

	_, err := systemops.ExecuteWithAudit(ctx, plan, executor, repo, "bob")
	if err != nil {
		t.Fatalf("ExecuteWithAudit failed: %v", err)
	}

	list, err := repo.ListAudits(ctx, AuditQueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListAudits: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(list))
	}
	if list[0].Username != "bob" {
		t.Errorf("username = %q, want bob", list[0].Username)
	}
}
