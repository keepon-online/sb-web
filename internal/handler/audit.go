package handler

import (
	"net/http"
	"strconv"
	"strings"

	"miaomiaowu/internal/storage"
)

type auditHandler struct {
	repo *storage.TrafficRepository
}

// NewAuditHandler returns the operation audit listing/detail handler.
func NewAuditHandler(repo *storage.TrafficRepository) http.Handler {
	return &auditHandler{repo: repo}
}

func (h *auditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/audit")
	path = strings.Trim(path, "/")

	switch {
	case path == "operations" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case strings.HasPrefix(path, "operations/") && r.Method == http.MethodGet:
		h.handleDetail(w, r, strings.TrimPrefix(path, "operations/"))
	default:
		methodNotAllowed(w, http.MethodGet)
	}
}

func (h *auditHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "audit storage unavailable"})
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	opts := storage.AuditQueryOptions{
		Limit:    limit,
		Offset:   offset,
		Status:   q.Get("status"),
		PlanName: q.Get("plan_name"),
	}

	records, err := h.repo.ListAudits(r.Context(), opts)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"items": records,
		"count": len(records),
	})
}

func (h *auditHandler) handleDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	if h.repo == nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "audit storage unavailable"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeBadRequest(w, "invalid id")
		return
	}

	record, err := h.repo.GetAudit(r.Context(), id)
	if err == storage.ErrAuditNotFound {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, record)
}
