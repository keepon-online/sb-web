package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/notify/telegram"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/systemops"
)

// telegramHandler exposes the Telegram bot configuration + push endpoints.
type telegramHandler struct {
	repo       *storage.TrafficRepository
	configPath string
	sboxDir    string
}

// TelegramHandlerOptions allows tests / callers to override the on-disk paths.
type TelegramHandlerOptions struct {
	ConfigPath string
	SBoxDir    string
}

// NewTelegramConfigHandler returns the GET/POST /api/admin/telegram/config endpoint.
func NewTelegramConfigHandler(repo *storage.TrafficRepository) http.Handler {
	return newTelegramHandler(repo, telegramRouteConfig)
}

// NewTelegramPushHandler returns the synchronous POST /api/admin/telegram/push endpoint.
func NewTelegramPushHandler(repo *storage.TrafficRepository) http.Handler {
	return newTelegramHandler(repo, telegramRoutePush)
}

// NewTelegramPushSSEHandler returns the streaming POST /api/admin/telegram/push-sse endpoint.
func NewTelegramPushSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return newTelegramHandler(repo, telegramRoutePushSSE)
}

type telegramRoute int

const (
	telegramRouteConfig telegramRoute = iota
	telegramRoutePush
	telegramRoutePushSSE
)

func newTelegramHandler(repo *storage.TrafficRepository, route telegramRoute) http.Handler {
	h := &telegramHandler{
		repo:       repo,
		configPath: telegram.DefaultConfigPath,
		sboxDir:    telegram.DefaultSBoxDir,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route {
		case telegramRouteConfig:
			h.serveConfig(w, r)
		case telegramRoutePush:
			h.servePushSync(w, r)
		case telegramRoutePushSSE:
			h.servePushSSE(w, r)
		default:
			writeError(w, http.StatusNotFound, errors.New("unknown telegram route"))
		}
	})
}

// configRequest is the wire format for POST /api/admin/telegram/config.
type configRequest struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

// configResponse is the on-the-wire shape for GET /api/admin/telegram/config.
// Token is always masked.
type configResponse struct {
	Configured  bool   `json:"configured"`
	TokenMasked string `json:"token_masked,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
}

func (h *telegramHandler) serveConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetConfig(w, r)
	case http.MethodPost:
		h.handleSaveConfig(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and POST are supported"))
	}
}

func (h *telegramHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := telegram.LoadConfig(h.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, configResponse{Configured: false})
			return
		}
		logger.Warn("[Telegram] 读取配置失败", "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Errorf("load telegram config: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, configResponse{
		Configured:  true,
		TokenMasked: telegram.MaskToken(cfg.Token),
		ChatID:      cfg.ChatID,
	})
}

func (h *telegramHandler) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req configRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}

	cfg := telegram.Config{
		Token:  strings.TrimSpace(req.Token),
		ChatID: strings.TrimSpace(req.ChatID),
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	username := getUsernameFromContext(r.Context())

	// Verify the token works *before* persisting it.
	client := telegram.NewClient(cfg)
	if err := client.SendMessage(r.Context(), "✅ Telegram 推送配置成功"); err != nil {
		logger.Warn("[Telegram] 验证消息失败", "user", username, "error", err)
		writeError(w, http.StatusBadGateway, fmt.Errorf("verify telegram: %w", err))
		return
	}

	if err := telegram.SaveConfig(h.configPath, cfg); err != nil {
		logger.Error("[Telegram] 保存配置失败", "error", err)
		writeError(w, http.StatusInternalServerError, fmt.Errorf("save telegram config: %w", err))
		return
	}

	// Audit log carries masked token only.
	logOperation(h.repo, username, "telegram_config_save", fmt.Sprintf("token=%s chat_id=%s", telegram.MaskToken(cfg.Token), cfg.ChatID))

	writeJSON(w, http.StatusOK, configResponse{
		Configured:  true,
		TokenMasked: telegram.MaskToken(cfg.Token),
		ChatID:      cfg.ChatID,
	})
}

func (h *telegramHandler) loadConfigOr401(w http.ResponseWriter) (telegram.Config, bool) {
	cfg, err := telegram.LoadConfig(h.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusPreconditionRequired, errors.New("telegram is not configured"))
			return telegram.Config{}, false
		}
		writeError(w, http.StatusInternalServerError, fmt.Errorf("load telegram config: %w", err))
		return telegram.Config{}, false
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusPreconditionFailed, err)
		return telegram.Config{}, false
	}
	return cfg, true
}

func (h *telegramHandler) servePushSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
		return
	}

	cfg, ok := h.loadConfigOr401(w)
	if !ok {
		return
	}

	plan, err := telegram.BuildPushPlan(cfg, h.sboxDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	executor := telegram.NewPushExecutor(telegram.NewClient(cfg))
	username := getUsernameFromContext(r.Context())

	result, execErr := systemops.ExecuteWithAudit(r.Context(), plan, executor, h.repo, username)

	payload := map[string]any{
		"plan_name": plan.Name,
		"steps":     result.Steps,
	}
	if execErr != nil {
		payload["error"] = execErr.Error()
		writeJSON(w, http.StatusInternalServerError, payload)
		return
	}
	payload["success"] = true
	writeJSON(w, http.StatusOK, payload)
}

func (h *telegramHandler) servePushSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
		return
	}

	cfg, ok := h.loadConfigOr401(w)
	if !ok {
		return
	}

	plan, err := telegram.BuildPushPlan(cfg, h.sboxDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	emitter, err := systemops.NewSSEEmitter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer emitter.Close()

	executor := telegram.NewPushExecutor(telegram.NewClient(cfg))
	_, _ = systemops.ExecuteWithProgress(r.Context(), plan, executor, emitter)
}
