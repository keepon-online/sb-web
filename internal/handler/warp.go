package handler

import (
	"context"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/warp"
)

// warpRegisterHandler implements POST /api/admin/warp/register.
//
// Wires the warp.Register flow into the admin API surface. Body is optional;
// when ?fallback=true is set and the Cloudflare call fails the handler
// returns the well-known account from sb.sh:3354-3356 alongside the original
// error so the operator can decide whether to accept the shared identity.
type warpRegisterHandler struct {
	register func(ctx context.Context, client *http.Client) (warp.Account, error)
}

// NewWarpRegisterHandler returns the WARP registration handler.
func NewWarpRegisterHandler() http.Handler {
	return &warpRegisterHandler{register: warp.Register}
}

func (h *warpRegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")
	logger.Info("[WARP Register] invoked", "username", username)

	account, err := h.register(r.Context(), nil)
	if err != nil {
		allowFallback := strings.EqualFold(r.URL.Query().Get("fallback"), "true")
		if allowFallback {
			fb := warp.Fallback()
			logger.Warn("[WARP Register] cloudflare failed, returning fallback",
				"username", username, "error", err)
			respondJSON(w, http.StatusOK, map[string]any{
				"account":        fb,
				"fallback_used":  true,
				"register_error": err.Error(),
			})
			return
		}
		logger.Warn("[WARP Register] cloudflare failed",
			"username", username, "error", err)
		respondJSON(w, http.StatusBadGateway, map[string]any{
			"error": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"account":       account,
		"fallback_used": false,
	})
}
