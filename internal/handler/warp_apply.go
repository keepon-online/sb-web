package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
	"miaomiaowu/internal/warp"
	"miaomiaowu/internal/warpapply"
)

// warpApplyRequest is the wire-layer body for POST /api/admin/warp/apply.
//
// Two modes:
//
//  1. {"register": true}  → call warp.Register, apply the freshly minted account.
//  2. {"account": {...}}  → apply the supplied account verbatim.
//
// Exactly one of the two modes must be set. The handler refuses ambiguous
// bodies (both or neither).
type warpApplyRequest struct {
	Register bool          `json:"register,omitempty"`
	Fallback bool          `json:"fallback,omitempty"`
	DryRun   bool          `json:"dry_run,omitempty"`
	Account  *warp.Account `json:"account,omitempty"`
}

// warpApplyHandler wires Sprint 7's warp.Register + Sprint 8's warpapply
// pipeline behind a single POST. The plan runs through ExecuteWithAudit so
// every step lands in the operation_audit table.
type warpApplyHandler struct {
	repo     *storage.TrafficRepository
	register func(ctx context.Context, client *http.Client) (warp.Account, error)
	executor systemops.StepExecutor
}

// NewWarpApplyHandler returns the warp-apply handler.
func NewWarpApplyHandler(repo *storage.TrafficRepository) http.Handler {
	return &warpApplyHandler{
		repo:     repo,
		register: warp.Register,
		executor: systemops.NewDefaultStepExecutor(),
	}
}

// NewWarpApplySSEHandler returns the SSE variant streaming the apply plan.
//
// Uses the same JSON body shape (register / account / fallback) as the sync
// handler so the front-end can swap between the two without re-encoding.
func NewWarpApplySSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &warpApplySSEHandler{
		register: warp.Register,
		executor: systemops.NewDefaultStepExecutor(),
	}
}

type warpApplySSEHandler struct {
	register func(ctx context.Context, client *http.Client) (warp.Account, error)
	executor systemops.StepExecutor
}

func (h *warpApplySSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var body warpApplyRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeBadRequest(w, "decode body: "+err.Error())
			return
		}
	}
	if body.Register && body.Account != nil {
		writeBadRequest(w, "set either register=true or account, not both")
		return
	}
	if !body.Register && body.Account == nil {
		writeBadRequest(w, "missing register=true or account body")
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")

	var account warp.Account
	if body.Register {
		got, err := h.register(r.Context(), nil)
		if err != nil {
			if !body.Fallback {
				logger.Warn("[WARP Apply SSE] register failed",
					"username", username, "error", err)
				respondJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
				return
			}
			logger.Warn("[WARP Apply SSE] register failed, using fallback",
				"username", username, "error", err)
			account = warp.Fallback()
		} else {
			account = got
		}
	} else {
		account = *body.Account
	}

	plan, err := warpapply.BuildApplyPlan(warpapply.ApplyRequest{Account: account})
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	plan.DryRun = body.DryRun
	emitter, sseErr := systemops.NewSSEEmitter(w)
	if sseErr != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": sseErr.Error()})
		return
	}
	defer emitter.Close()

	logger.Info("[WARP Apply SSE] plan invoked",
		"username", username,
		"plan", plan.Name,
		"ipv6", account.IPv6,
	)

	if _, execErr := systemops.ExecuteWithProgress(r.Context(), plan, h.executor, emitter); execErr != nil {
		logger.Warn("[WARP Apply SSE] plan finished with error",
			"username", username, "error", execErr,
		)
	}
}

func (h *warpApplyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var body warpApplyRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeBadRequest(w, "decode body: "+err.Error())
			return
		}
	}

	if body.Register && body.Account != nil {
		writeBadRequest(w, "set either register=true or account, not both")
		return
	}
	if !body.Register && body.Account == nil {
		writeBadRequest(w, "missing register=true or account body")
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")

	var account warp.Account
	if body.Register {
		got, err := h.register(r.Context(), nil)
		if err != nil {
			if !body.Fallback {
				logger.Warn("[WARP Apply] register failed",
					"username", username, "error", err)
				respondJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
				return
			}
			logger.Warn("[WARP Apply] register failed, using fallback",
				"username", username, "error", err)
			account = warp.Fallback()
		} else {
			account = got
		}
	} else {
		account = *body.Account
	}

	plan, err := warpapply.BuildApplyPlan(warpapply.ApplyRequest{Account: account})
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	plan.DryRun = body.DryRun
	logger.Info("[WARP Apply] plan invoked",
		"username", username,
		"plan", plan.Name,
		"ipv6", account.IPv6,
	)

	result, execErr := systemops.ExecuteWithAudit(r.Context(), plan, h.executor, h.repo, username)
	payload := map[string]any{
		"plan":    plan.Name,
		"dry_run": result.DryRun,
		"steps":   result.Steps,
		"account_summary": map[string]any{
			"ipv6":     account.IPv6,
			"reserved": account.Reserved,
		},
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		logger.Warn("[WARP Apply] plan finished with error",
			"username", username, "error", execErr)
		respondJSON(w, http.StatusInternalServerError, payload)
		return
	}
	respondJSON(w, http.StatusOK, payload)
}
