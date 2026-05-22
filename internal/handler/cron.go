package handler

import (
	"net/http"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/cron"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

// cronToggleHandler executes either the Enable or Disable cron plan.
//
// One handler per direction keeps the URL surface explicit and lets each
// endpoint be audited under its own plan name.
type cronToggleHandler struct {
	repo     *storage.TrafficRepository
	plan     systemops.OperationPlan
	executor systemops.StepExecutor
	logTag   string
}

// NewCronEnableHandler returns the handler for POST /api/admin/cron/enable.
func NewCronEnableHandler(repo *storage.TrafficRepository) http.Handler {
	return &cronToggleHandler{
		repo:     repo,
		plan:     cron.BuildEnablePlan(),
		executor: cron.NewLiveExecutor(),
		logTag:   "[Cron Enable]",
	}
}

// NewCronDisableHandler returns the handler for POST /api/admin/cron/disable.
func NewCronDisableHandler(repo *storage.TrafficRepository) http.Handler {
	return &cronToggleHandler{
		repo:     repo,
		plan:     cron.BuildDisablePlan(),
		executor: cron.NewLiveExecutor(),
		logTag:   "[Cron Disable]",
	}
}

// NewCronEnableSSEHandler returns the SSE variant for POST /api/admin/cron/enable-sse.
func NewCronEnableSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &cronSSEHandler{
		plan:     cron.BuildEnablePlan(),
		executor: cron.NewLiveExecutor(),
		logTag:   "[Cron Enable SSE]",
	}
}

// NewCronDisableSSEHandler returns the SSE variant for POST /api/admin/cron/disable-sse.
func NewCronDisableSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &cronSSEHandler{
		plan:     cron.BuildDisablePlan(),
		executor: cron.NewLiveExecutor(),
		logTag:   "[Cron Disable SSE]",
	}
}

// cronSSEHandler streams cron plan progress events via SSE.
type cronSSEHandler struct {
	plan     systemops.OperationPlan
	executor systemops.StepExecutor
	logTag   string
}

func (h *cronSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info(h.logTag+" plan invoked",
		"username", username,
		"plan", h.plan.Name,
	)

	if _, execErr := systemops.ExecuteWithProgress(r.Context(), h.plan, h.executor, emitter); execErr != nil {
		logger.Warn(h.logTag+" plan finished with error",
			"username", username, "error", execErr,
		)
	}
}

func (h *cronToggleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info(h.logTag+" plan invoked",
		"username", username,
		"plan", h.plan.Name,
	)

	result, execErr := systemops.ExecuteWithAudit(r.Context(), h.plan, h.executor, h.repo, username)
	payload := map[string]any{
		"plan":    h.plan.Name,
		"dry_run": result.DryRun,
		"steps":   result.Steps,
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		logger.Warn(h.logTag+" plan finished with error",
			"username", username, "error", execErr,
		)
		respondJSON(w, http.StatusInternalServerError, payload)
		return
	}
	respondJSON(w, http.StatusOK, payload)
}
