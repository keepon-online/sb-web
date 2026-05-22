package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox/upgrade"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

// singboxUpgradeRequest is the JSON body for /api/admin/singbox/upgrade
// and /api/admin/singbox/upgrade-sse. Fields mirror upgrade.UpgradeRequest
// but live in this package so the wire format stays stable independent of
// internal struct reshuffles.
type singboxUpgradeRequest struct {
	Channel       int    `json:"channel"`
	PinnedVersion string `json:"pinned_version,omitempty"`
	Arch          string `json:"arch"`
	LatestStable  string `json:"latest_stable,omitempty"`
	LatestPre     string `json:"latest_pre,omitempty"`
}

// versionFetcher is the seam the handlers use for upstream version lookup.
// In production this is upgrade.FetchLatest; tests can swap in a fake.
type versionFetcher func(ctx context.Context, client *http.Client) (string, string, error)

// localVersionProbe returns the currently installed sing-box version string
// by running the binary. Defaults to a zero-cost lookup so the preview
// endpoint never blocks on a missing binary.
type localVersionProbe func(ctx context.Context) string

// singboxUpgradePreviewHandler implements POST /api/admin/singbox/upgrade/preview.
type singboxUpgradePreviewHandler struct {
	fetch versionFetcher
	probe localVersionProbe
}

// NewSingboxUpgradePreviewHandler returns the preview handler. It only reads
// state (network + local probe) and never mutates the system.
func NewSingboxUpgradePreviewHandler() http.Handler {
	return &singboxUpgradePreviewHandler{
		fetch: upgrade.FetchLatest,
		probe: probeInstalledVersion,
	}
}

func (h *singboxUpgradePreviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodPost, http.MethodGet)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	stable, pre, _ := h.fetch(ctx, nil)
	current := h.probe(ctx)

	respondJSON(w, http.StatusOK, map[string]any{
		"stable":          stable,
		"pre":             pre,
		"current_version": current,
		"arch":            defaultArch(),
	})
}

// singboxUpgradeHandler runs the upgrade plan synchronously through
// ExecuteWithAudit so every step lands in the operation audit table.
type singboxUpgradeHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
	fetch    versionFetcher
}

// NewSingboxUpgradeHandler returns the synchronous upgrade handler.
func NewSingboxUpgradeHandler(repo *storage.TrafficRepository) http.Handler {
	return &singboxUpgradeHandler{
		repo:     repo,
		executor: systemops.NewDefaultStepExecutor(),
		fetch:    upgrade.FetchLatest,
	}
}

func (h *singboxUpgradeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	req, plan, err := h.buildPlan(r)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[Singbox Upgrade] plan invoked",
		"username", username,
		"plan", plan.Name,
		"channel", req.Channel,
		"arch", req.Arch,
	)

	result, execErr := systemops.ExecuteWithAudit(r.Context(), plan, h.executor, h.repo, username)

	payload := map[string]any{
		"plan":    plan.Name,
		"dry_run": result.DryRun,
		"steps":   result.Steps,
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		logger.Warn("[Singbox Upgrade] plan finished with error",
			"username", username, "error", execErr,
		)
		respondJSON(w, http.StatusInternalServerError, payload)
		return
	}
	respondJSON(w, http.StatusOK, payload)
}

// singboxUpgradeSSEHandler streams progress events for the upgrade plan.
type singboxUpgradeSSEHandler struct {
	repo     *storage.TrafficRepository
	executor systemops.StepExecutor
	fetch    versionFetcher
}

// NewSingboxUpgradeSSEHandler returns the streaming upgrade handler.
func NewSingboxUpgradeSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return &singboxUpgradeSSEHandler{
		repo:     repo,
		executor: systemops.NewDefaultStepExecutor(),
		fetch:    upgrade.FetchLatest,
	}
}

func (h *singboxUpgradeSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	req, plan, err := buildUpgradePlanFromRequest(r, h.fetch)
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
	logger.Info("[Singbox Upgrade SSE] plan invoked",
		"username", username,
		"plan", plan.Name,
		"channel", req.Channel,
		"arch", req.Arch,
	)

	if _, execErr := systemops.ExecuteWithProgress(r.Context(), plan, h.executor, emitter); execErr != nil {
		logger.Warn("[Singbox Upgrade SSE] plan finished with error",
			"username", username, "error", execErr,
		)
	}
}

// buildPlan wires the body → request → plan transformation for the sync
// handler. The SSE handler uses buildUpgradePlanFromRequest directly because
// it has no access to the synchronous handler's struct.
func (h *singboxUpgradeHandler) buildPlan(r *http.Request) (upgrade.UpgradeRequest, systemops.OperationPlan, error) {
	return buildUpgradePlanFromRequest(r, h.fetch)
}

// buildUpgradePlanFromRequest decodes the JSON body, resolves latest versions
// when the body did not pin them explicitly, and returns the validated plan.
// All validation lives upstream of systemops; rejection here happens before
// any audit record is written.
func buildUpgradePlanFromRequest(r *http.Request, fetch versionFetcher) (upgrade.UpgradeRequest, systemops.OperationPlan, error) {
	var raw singboxUpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return upgrade.UpgradeRequest{}, systemops.OperationPlan{}, fmt.Errorf("decode body: %w", err)
	}

	if raw.Channel < int(upgrade.ChannelStable) || raw.Channel > int(upgrade.ChannelPinned) {
		return upgrade.UpgradeRequest{}, systemops.OperationPlan{}, errors.New("channel must be 1 (stable), 2 (pre), or 3 (pinned)")
	}
	if strings.TrimSpace(raw.Arch) == "" {
		raw.Arch = defaultArch()
	}

	req := upgrade.UpgradeRequest{
		Channel:       upgrade.Channel(raw.Channel),
		PinnedVersion: raw.PinnedVersion,
		Arch:          raw.Arch,
	}

	// When the body did not pin both latest values, query upstream once. The
	// preview endpoint is the recommended flow, but allow direct upgrade with
	// channel=Pinned to skip the network call entirely.
	stable, pre := raw.LatestStable, raw.LatestPre
	if req.Channel != upgrade.ChannelPinned && (stable == "" || pre == "") {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		s, p, _ := fetch(ctx, nil)
		if stable == "" {
			stable = s
		}
		if pre == "" {
			pre = p
		}
	}

	plan, err := upgrade.BuildUpgradePlan(req, stable, pre)
	if err != nil {
		return upgrade.UpgradeRequest{}, systemops.OperationPlan{}, err
	}
	return req, plan, nil
}

// probeInstalledVersion runs `/etc/s-box/sing-box version` to surface the
// currently installed version. Returns an empty string when the binary is
// absent or the call errors out — the preview UI must tolerate that.
func probeInstalledVersion(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, upgrade.SingBoxBin, "version").CombinedOutput()
	if err != nil {
		return ""
	}
	return upgrade.DetectMajor(string(out))
}

// defaultArch returns the GOARCH-derived architecture identifier, mirroring
// sb.sh:47-52. Used as the fallback when the client did not pass arch.
func defaultArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "386"
	case "arm":
		return "armv7"
	default:
		return runtime.GOARCH
	}
}
