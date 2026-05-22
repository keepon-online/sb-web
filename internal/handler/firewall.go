package handler

import (
	"net/http"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/firewall"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

// firewallHandler dispatches both the synchronous and SSE variants of the
// firewall disable endpoint. Both routes share the same OperationPlan builder
// so behavior stays in lockstep.
type firewallHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
}

// NewFirewallDisableHandler returns the synchronous handler that runs the
// disable plan with audit persistence.
func NewFirewallDisableHandler(repo *storage.TrafficRepository) http.Handler {
	return &firewallHandler{
		repo:     repo,
		executor: systemops.NewDefaultStepExecutor(),
	}
}

// NewFirewallDisableSSEHandler returns the SSE handler that streams progress
// events as the disable plan executes.
func NewFirewallDisableSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &firewallSSEHandler{
		repo:     repo,
		executor: systemops.NewDefaultStepExecutor(),
	}
}

// DisableFirewall handles POST /api/admin/firewall/disable.
//
// It executes BuildDisablePlan() through ExecuteWithAudit and returns the
// resulting audit JSON. The route is bound through RequireAdmin in main.go,
// so reaching this handler already implies an authenticated admin caller.
func (h *firewallHandler) DisableFirewall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	plan := firewall.BuildDisablePlan()
	username := auth.UsernameOrDefault(r.Context(), "unknown")

	logger.Info("[Firewall API] disable plan invoked",
		"username", username,
		"plan", plan.Name,
		"steps", len(plan.Steps),
	)

	result, execErr := systemops.ExecuteWithAudit(r.Context(), plan, h.executor, h.repo, username)

	payload := map[string]any{
		"plan":    plan.Name,
		"dry_run": result.DryRun,
		"steps":   result.Steps,
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		logger.Warn("[Firewall API] disable plan finished with error",
			"username", username,
			"error", execErr,
		)
		respondJSON(w, http.StatusInternalServerError, payload)
		return
	}

	respondJSON(w, http.StatusOK, payload)
}

// ServeHTTP dispatches to DisableFirewall.
func (h *firewallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.DisableFirewall(w, r)
}

// firewallSSEHandler runs the disable plan and streams progress events via SSE.
type firewallSSEHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
}

// DisableFirewallSSE handles POST /api/admin/firewall/disable-sse.
func (h *firewallSSEHandler) DisableFirewallSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	emitter, err := systemops.NewSSEEmitter(w)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer emitter.Close()

	plan := firewall.BuildDisablePlan()
	username := auth.UsernameOrDefault(r.Context(), "unknown")

	logger.Info("[Firewall API] disable plan streaming invoked",
		"username", username,
		"plan", plan.Name,
		"steps", len(plan.Steps),
	)

	if _, execErr := systemops.ExecuteWithProgress(r.Context(), plan, h.executor, emitter); execErr != nil {
		logger.Warn("[Firewall API] disable plan (sse) finished with error",
			"username", username,
			"error", execErr,
		)
	}
}

// ServeHTTP dispatches to DisableFirewallSSE.
func (h *firewallSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.DisableFirewallSSE(w, r)
}
