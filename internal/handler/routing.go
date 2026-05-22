package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/routing"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

// routingHandler is the shared base for the three routing endpoints
// (status / update / update-sse). The auth.RequireAdmin wrapper is applied in
// main.go before this handler is reached.
type routingHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
	jsonPath string
}

// routingUpdateRequest matches the JSON body for POST /api/admin/routing/update
// and /api/admin/routing/update-sse.
type routingUpdateRequest struct {
	Channel int      `json:"channel"`
	Mode    int      `json:"mode"`
	Domains []string `json:"domains"`
}

// NewRoutingStatusHandler returns the GET /api/admin/routing/status handler.
func NewRoutingStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return &routingStatusHandler{
		repo:     repo,
		jsonPath: routing.SBJSONPath,
	}
}

// NewRoutingUpdateHandler returns the synchronous POST /api/admin/routing/update
// handler. It runs the plan through ExecuteWithAudit so every operation lands
// in the audit log.
func NewRoutingUpdateHandler(repo *storage.TrafficRepository) http.Handler {
	return &routingHandler{
		repo:     repo,
		executor: systemops.NewDefaultStepExecutor(),
		jsonPath: routing.SBJSONPath,
	}
}

// NewRoutingUpdateSSEHandler returns the streaming POST /api/admin/routing/update-sse
// handler. It emits ProgressEvent messages as the plan executes.
func NewRoutingUpdateSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &routingSSEHandler{
		repo:     repo,
		executor: systemops.NewDefaultStepExecutor(),
	}
}

// --- status ---

type routingStatusHandler struct {
	repo     *storage.TrafficRepository
	jsonPath string
}

func (h *routingStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	state, err := routing.ReadCurrent(h.jsonPath)
	if err != nil {
		logger.Warn("[Routing API] read current state failed", "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"channels": stateToWire(state),
	})
}

// stateToWire flattens the channel-keyed map into a JSON-friendly slice keyed
// by integer channel id, so the wire format stays stable.
func stateToWire(state map[routing.Channel]routing.ChannelState) []map[string]any {
	out := make([]map[string]any, 0, len(state))
	for ch := routing.ChannelWarpWireguardIPv4; ch <= routing.ChannelVPSLocalIPv6; ch++ {
		s := state[ch]
		out = append(out, map[string]any{
			"channel":       int(ch),
			"domain_suffix": s.DomainSuffix,
			"geosite":       s.GeoSite,
		})
	}
	return out
}

// --- synchronous update ---

func (h *routingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	req, err := decodeUpdateRequest(r)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	plan, err := routing.BuildUpdatePlan(req)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[Routing API] update plan invoked",
		"username", username,
		"plan", plan.Name,
		"channel", req.Channel,
		"mode", req.Mode,
		"entries", len(req.Domains),
	)

	result, execErr := systemops.ExecuteWithAudit(r.Context(), plan, h.executor, h.repo, username)

	payload := map[string]any{
		"plan":    plan.Name,
		"dry_run": result.DryRun,
		"steps":   result.Steps,
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		logger.Warn("[Routing API] update plan finished with error",
			"username", username,
			"error", execErr,
		)
		respondJSON(w, http.StatusInternalServerError, payload)
		return
	}
	respondJSON(w, http.StatusOK, payload)
}

// --- SSE update ---

type routingSSEHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
}

func (h *routingSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	req, err := decodeUpdateRequest(r)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	plan, err := routing.BuildUpdatePlan(req)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	emitter, err := systemops.NewSSEEmitter(w)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer emitter.Close()

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[Routing API] update plan streaming invoked",
		"username", username,
		"plan", plan.Name,
		"channel", req.Channel,
		"mode", req.Mode,
		"entries", len(req.Domains),
	)

	if _, execErr := systemops.ExecuteWithProgress(r.Context(), plan, h.executor, emitter); execErr != nil {
		logger.Warn("[Routing API] update plan (sse) finished with error",
			"username", username,
			"error", execErr,
		)
	}
}

// --- helpers ---

func decodeUpdateRequest(r *http.Request) (routing.UpdateRequest, error) {
	var raw routingUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return routing.UpdateRequest{}, fmt.Errorf("decode body: %w", err)
	}
	if raw.Channel < 1 || raw.Channel > 6 {
		return routing.UpdateRequest{}, errors.New("channel must be in 1..6")
	}
	if raw.Mode != 1 && raw.Mode != 2 {
		return routing.UpdateRequest{}, errors.New("mode must be 1 (domain_suffix) or 2 (geosite)")
	}
	if raw.Domains == nil {
		raw.Domains = []string{}
	}
	return routing.UpdateRequest{
		Channel: routing.Channel(raw.Channel),
		Mode:    routing.Mode(raw.Mode),
		Domains: raw.Domains,
	}, nil
}
