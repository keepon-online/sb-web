package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

func newTestRepoForAudit(t *testing.T) *storage.TrafficRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	repo, err := storage.NewTrafficRepository(dbPath)
	if err != nil {
		t.Fatalf("NewTrafficRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func seedAudit(t *testing.T, repo *storage.TrafficRepository, planName, status string) int64 {
	t.Helper()
	now := time.Now().UTC()
	id, err := repo.WriteAudit(context.Background(), systemops.AuditRecord{
		PlanName:   planName,
		Status:     status,
		StartedAt:  now,
		FinishedAt: now,
	})
	if err != nil {
		t.Fatalf("WriteAudit: %v", err)
	}
	return id
}

func TestAuditHandler_List(t *testing.T) {
	repo := newTestRepoForAudit(t)
	seedAudit(t, repo, "plan-a", systemops.AuditStatusSuccess)
	seedAudit(t, repo, "plan-b", systemops.AuditStatusFailed)

	h := NewAuditHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit/operations", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Count int `json:"count"`
		Items []struct {
			PlanName string `json:"plan_name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
}

func TestAuditHandler_ListWithStatusFilter(t *testing.T) {
	repo := newTestRepoForAudit(t)
	seedAudit(t, repo, "plan-a", systemops.AuditStatusSuccess)
	seedAudit(t, repo, "plan-b", systemops.AuditStatusFailed)

	h := NewAuditHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit/operations?status=failed", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
}

func TestAuditHandler_Detail(t *testing.T) {
	repo := newTestRepoForAudit(t)
	id := seedAudit(t, repo, "plan-detail", systemops.AuditStatusSuccess)

	h := NewAuditHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit/operations/"+strconv.FormatInt(id, 10), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PlanName string `json:"plan_name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PlanName != "plan-detail" {
		t.Errorf("plan_name = %q, want plan-detail", resp.PlanName)
	}
}

func TestAuditHandler_DetailNotFound(t *testing.T) {
	repo := newTestRepoForAudit(t)
	h := NewAuditHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit/operations/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAuditHandler_DetailInvalidID(t *testing.T) {
	repo := newTestRepoForAudit(t)
	h := NewAuditHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit/operations/not-a-number", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAuditHandler_MethodNotAllowed(t *testing.T) {
	repo := newTestRepoForAudit(t)
	h := NewAuditHandler(repo)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/audit/operations", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
