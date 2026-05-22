package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/certmgr"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

// certmgrStatusHandler implements GET /api/admin/certmgr/acme-status.
//
// Read-only: surfaces whether the legacy Acme-yg flow already left a usable
// domain certificate under /root/ygkkkca.
type certmgrStatusHandler struct{}

// NewCertmgrStatusHandler returns the read-only Acme detection handler.
func NewCertmgrStatusHandler() http.Handler {
	return &certmgrStatusHandler{}
}

func (h *certmgrStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	status, err := certmgr.DetectAcme()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, status)
}

// selfSignRequestBody mirrors certmgr.SelfSignRequest at the wire layer.
type selfSignRequestBody struct {
	CommonName string `json:"common_name,omitempty"`
	Days       int    `json:"days,omitempty"`
	CertPath   string `json:"cert_path,omitempty"`
	KeyPath    string `json:"key_path,omitempty"`
}

// certmgrSelfSignHandler implements POST /api/admin/certmgr/self-sign.
type certmgrSelfSignHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
}

// NewCertmgrSelfSignHandler returns the self-sign handler.
func NewCertmgrSelfSignHandler(repo *storage.TrafficRepository) http.Handler {
	return &certmgrSelfSignHandler{
		repo:     repo,
		executor: certmgr.NewExecutor(),
	}
}

// NewCertmgrSelfSignSSEHandler returns the SSE variant of self-sign.
// Single-step plan still benefits from SSE: the front-end sees plan_started
// + step_completed (or step_failed) without polling.
func NewCertmgrSelfSignSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &certmgrSelfSignSSEHandler{
		repo:     repo,
		executor: certmgr.NewExecutor(),
	}
}

// certmgrSelfSignSSEHandler streams self-sign progress events via SSE.
type certmgrSelfSignSSEHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
}

func (h *certmgrSelfSignSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var body selfSignRequestBody
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeBadRequest(w, "decode body: "+err.Error())
			return
		}
	}

	req := certmgr.SelfSignRequest{
		CommonName: body.CommonName,
		Days:       body.Days,
		CertPath:   body.CertPath,
		KeyPath:    body.KeyPath,
	}
	plan, err := certmgr.BuildSelfSignPlan(req)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	emitter, sseErr := systemops.NewSSEEmitter(w)
	if sseErr != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": sseErr.Error()})
		return
	}
	defer emitter.Close()

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[Certmgr Self-Sign SSE] plan invoked",
		"username", username, "cn", req.CommonName,
	)

	if _, execErr := systemops.ExecuteWithProgress(r.Context(), plan, h.executor, emitter); execErr != nil {
		logger.Warn("[Certmgr Self-Sign SSE] plan finished with error",
			"username", username, "error", execErr,
		)
	}
}

func (h *certmgrSelfSignHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var body selfSignRequestBody
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeBadRequest(w, "decode body: "+err.Error())
			return
		}
	}

	req := certmgr.SelfSignRequest{
		CommonName: body.CommonName,
		Days:       body.Days,
		CertPath:   body.CertPath,
		KeyPath:    body.KeyPath,
	}
	plan, err := certmgr.BuildSelfSignPlan(req)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[Certmgr Self-Sign] plan invoked",
		"username", username, "cn", req.CommonName, "days", req.Days,
	)

	result, execErr := systemops.ExecuteWithAudit(r.Context(), plan, h.executor, h.repo, username)
	payload := map[string]any{
		"plan":    plan.Name,
		"dry_run": result.DryRun,
		"steps":   result.Steps,
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		logger.Warn("[Certmgr Self-Sign] plan finished with error",
			"username", username, "error", execErr,
		)
		respondJSON(w, http.StatusInternalServerError, payload)
		return
	}
	respondJSON(w, http.StatusOK, payload)
}

// compile-time guard that the executor type still satisfies the interface
// after future refactors.
var _ systemops.StepExecutor = (*certmgr.Executor)(nil)

// ensure errors import stays referenced if future stricter linting kicks in.
var _ = errors.New
