package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// ShareCreateRequest 创建分享请求
type ShareCreateRequest struct {
	Name     string              `json:"name"`
	Target   string              `json:"target"`   // gitlab, github, gist, local, pastebin
	Token    string              `json:"token,omitempty"`
	RepoURL  string              `json:"repo_url,omitempty"`
	FilePath string              `json:"file_path,omitempty"`
	Branch   string              `json:"branch,omitempty"`
	Message  string              `json:"message,omitempty"`
}

// ShareNodeRequest 分享节点请求
type ShareNodeRequest struct {
	SubscriptionID string `json:"subscription_id"`
	Target         string `json:"target"`
}

// ShareUpdateRequest 更新分享请求
type ShareUpdateRequest struct {
	ShareID       string `json:"share_id"`
	Enabled       bool   `json:"enabled,omitempty"`
	AutoShare     bool   `json:"auto_share,omitempty"`
	ShareInterval int    `json:"share_interval,omitempty"`
	Token         string `json:"token,omitempty"`
	FilePath      string `json:"file_path,omitempty"`
	Message       string `json:"message,omitempty"`
}

// NewShareCreateHandler 创建分享配置处理器
func NewShareCreateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ShareCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		if req.Target == "" {
			writeError(w, http.StatusBadRequest, errors.New("target is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "share_create", fmt.Sprintf("创建分享配置: %s -> %s", req.Name, req.Target))

		sm := singbox.NewShareManager()

		// 构建配置参数
		config := make(map[string]string)
		if req.Token != "" {
			config["token"] = req.Token
		}
		if req.RepoURL != "" {
			config["repo_url"] = req.RepoURL
		}
		if req.FilePath != "" {
			config["file_path"] = req.FilePath
		}
		if req.Branch != "" {
			config["branch"] = req.Branch
		}

		shareConfig, err := sm.CreateShareConfig(req.Name, singbox.ShareTarget(req.Target), config)
		if err != nil {
			logger.Error("[分享API] 创建分享配置失败", "error", err)
			logOperationWithError(repo, username, "share_create", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create share config failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "success",
			"message":      "分享配置创建成功",
			"share_config": shareConfig,
		})

		logger.Info("[分享API] 分享配置创建成功", "name", req.Name, "target", req.Target)
	})
}

// NewShareNodeHandler 创建分享节点处理器
func NewShareNodeHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ShareNodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}
		if req.Target == "" {
			writeError(w, http.StatusBadRequest, errors.New("target is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "share_node", fmt.Sprintf("分享节点: %s -> %s", req.SubscriptionID, req.Target))

		sm := singbox.NewShareManager()
		shareConfig, err := sm.ShareNodeInfo(req.SubscriptionID, singbox.ShareTarget(req.Target))
		if err != nil {
			logger.Error("[分享API] 分享节点失败", "error", err)
			logOperationWithError(repo, username, "share_node", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("share node failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "success",
			"message":      "节点分享成功",
			"share_config": shareConfig,
			"share_url":    shareConfig.URL,
		})

		logger.Info("[分享API] 节点分享成功", "subscription_id", req.SubscriptionID, "target", req.Target, "url", shareConfig.URL)
	})
}

// NewShareListHandler 创建分享列表处理器
func NewShareListHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		sm := singbox.NewShareManager()
		shareConfigs, err := sm.ListShareConfigs()
		if err != nil {
			logger.Error("[分享API] 获取分享列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list share configs failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"share_configs": shareConfigs,
			"count":         len(shareConfigs),
		})
	})
}

// NewShareDetailHandler 创建分享详情处理器
func NewShareDetailHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取分享ID
		shareID := r.URL.Query().Get("share_id")
		if shareID == "" {
			writeError(w, http.StatusBadRequest, errors.New("share_id parameter is required"))
			return
		}

		sm := singbox.NewShareManager()
		status, err := sm.GetShareStatus(shareID)
		if err != nil {
			logger.Error("[分享API] 获取分享状态失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get share status failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

// NewShareUpdateHandler 创建分享更新处理器
func NewShareUpdateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ShareUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.ShareID == "" {
			writeError(w, http.StatusBadRequest, errors.New("share_id is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "share_update", fmt.Sprintf("更新分享配置: %s", req.ShareID))

		sm := singbox.NewShareManager()

		// 构建更新参数
		updates := make(map[string]interface{})
		if req.Token != "" {
			updates["token"] = req.Token
		}
		if req.FilePath != "" {
			updates["file_path"] = req.FilePath
		}
		if req.Message != "" {
			updates["message"] = req.Message
		}
		updates["enabled"] = req.Enabled
		updates["auto_share"] = req.AutoShare
		if req.ShareInterval > 0 {
			updates["share_interval"] = req.ShareInterval
		}

		if err := sm.UpdateShareConfig(req.ShareID, updates); err != nil {
			logger.Error("[分享API] 更新分享配置失败", "error", err)
			logOperationWithError(repo, username, "share_update", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("update share config failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "分享配置更新成功",
		})

		logger.Info("[分享API] 分享配置更新成功", "share_id", req.ShareID)
	})
}

// NewShareDeleteHandler 创建分享删除处理器
func NewShareDeleteHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only DELETE or POST is supported"))
			return
		}

		// 从URL获取分享ID
		shareID := r.URL.Query().Get("share_id")
		if shareID == "" {
			writeError(w, http.StatusBadRequest, errors.New("share_id parameter is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "share_delete", fmt.Sprintf("删除分享配置: %s", shareID))

		sm := singbox.NewShareManager()
		if err := sm.DeleteShareConfig(shareID); err != nil {
			logger.Error("[分享API] 删除分享配置失败", "error", err)
			logOperationWithError(repo, username, "share_delete", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("delete share config failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "分享配置删除成功",
		})

		logger.Info("[分享API] 分享配置删除成功", "share_id", shareID)
	})
}

// NewGitLabSyncHandler 创建GitLab同步处理器
func NewGitLabSyncHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			SubscriptionID string `json:"subscription_id"`
			Token          string `json:"token"`
			RepoURL        string `json:"repo_url"`
			FilePath       string `json:"file_path,omitempty"`
			Branch         string `json:"branch,omitempty"`
			Message        string `json:"message,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}
		if req.Token == "" {
			writeError(w, http.StatusBadRequest, errors.New("token is required"))
			return
		}
		if req.RepoURL == "" {
			writeError(w, http.StatusBadRequest, errors.New("repo_url is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "gitlab_sync", fmt.Sprintf("GitLab同步: %s -> %s", req.SubscriptionID, req.RepoURL))

		sm := singbox.NewShareManager()

		// 创建GitLab分享配置
		config := make(map[string]string)
		config["token"] = req.Token
		config["repo_url"] = req.RepoURL
		if req.FilePath != "" {
			config["file_path"] = req.FilePath
		}
		if req.Branch != "" {
			config["branch"] = req.Branch
		}

		shareConfig, err := sm.CreateShareConfig(
			fmt.Sprintf("gitlab-sync-%s", req.SubscriptionID),
			singbox.ShareTargetGitLab,
			config,
		)
		if err != nil {
			logger.Error("[GitLab同步API] 创建分享配置失败", "error", err)
			logOperationWithError(repo, username, "gitlab_sync", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create share config failed: %w", err))
			return
		}

		// 执行分享
		shareConfig.Message = req.Message
		if shareConfig.Message == "" {
			shareConfig.Message = fmt.Sprintf("Sync subscription: %s", time.Now().Format("2006-01-02 15:04:05"))
		}

		_, err = sm.ShareToTarget(shareConfig, "")
		if err != nil {
			logger.Error("[GitLab同步API] 同步失败", "error", err)
			logOperationWithError(repo, username, "gitlab_sync", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("sync to GitLab failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "success",
			"message":      "GitLab同步成功",
			"share_url":    shareConfig.URL,
			"share_config": shareConfig,
		})

		logger.Info("[GitLab同步API] GitLab同步成功", "subscription_id", req.SubscriptionID, "url", shareConfig.URL)
	})
}

// NewGitHubSyncHandler 创建GitHub同步处理器
func NewGitHubSyncHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			SubscriptionID string `json:"subscription_id"`
			Token          string `json:"token"`
			RepoURL        string `json:"repo_url"`
			FilePath       string `json:"file_path,omitempty"`
			Branch         string `json:"branch,omitempty"`
			Message        string `json:"message,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}
		if req.Token == "" {
			writeError(w, http.StatusBadRequest, errors.New("token is required"))
			return
		}
		if req.RepoURL == "" {
			writeError(w, http.StatusBadRequest, errors.New("repo_url is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "github_sync", fmt.Sprintf("GitHub同步: %s -> %s", req.SubscriptionID, req.RepoURL))

		sm := singbox.NewShareManager()

		// 创建GitHub分享配置
		config := make(map[string]string)
		config["token"] = req.Token
		config["repo_url"] = req.RepoURL
		if req.FilePath != "" {
			config["file_path"] = req.FilePath
		}
		if req.Branch != "" {
			config["branch"] = req.Branch
		}

		shareConfig, err := sm.CreateShareConfig(
			fmt.Sprintf("github-sync-%s", req.SubscriptionID),
			singbox.ShareTargetGitHub,
			config,
		)
		if err != nil {
			logger.Error("[GitHub同步API] 创建分享配置失败", "error", err)
			logOperationWithError(repo, username, "github_sync", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create share config failed: %w", err))
			return
		}

		// 执行分享
		shareConfig.Message = req.Message
		if shareConfig.Message == "" {
			shareConfig.Message = fmt.Sprintf("Sync subscription: %s", time.Now().Format("2006-01-02 15:04:05"))
		}

		_, err = sm.ShareToTarget(shareConfig, "")
		if err != nil {
			logger.Error("[GitHub同步API] 同步失败", "error", err)
			logOperationWithError(repo, username, "github_sync", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("sync to GitHub failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "success",
			"message":      "GitHub同步成功",
			"share_url":    shareConfig.URL,
			"share_config": shareConfig,
		})

		logger.Info("[GitHub同步API] GitHub同步成功", "subscription_id", req.SubscriptionID, "url", shareConfig.URL)
	})
}

// NewPastebinShareHandler 创建Pastebin分享处理器
func NewPastebinShareHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			SubscriptionID string `json:"subscription_id"`
			DevKey         string `json:"dev_key"`
			Name           string `json:"name,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}
		if req.DevKey == "" {
			writeError(w, http.StatusBadRequest, errors.New("dev_key is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "pastebin_share", fmt.Sprintf("Pastebin分享: %s", req.SubscriptionID))

		sm := singbox.NewShareManager()

		// 创建Pastebin分享配置
		config := make(map[string]string)
		config["token"] = req.DevKey

		name := req.Name
		if name == "" {
			name = fmt.Sprintf("subscription-%s", req.SubscriptionID)
		}

		shareConfig, err := sm.CreateShareConfig(name, singbox.ShareTargetPastebin, config)
		if err != nil {
			logger.Error("[Pastebin分享API] 创建分享配置失败", "error", err)
			logOperationWithError(repo, username, "pastebin_share", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create share config failed: %w", err))
			return
		}

		// 执行分享
		_, err = sm.ShareToTarget(shareConfig, "")
		if err != nil {
			logger.Error("[Pastebin分享API] 分享失败", "error", err)
			logOperationWithError(repo, username, "pastebin_share", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("share to pastebin failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "success",
			"message":      "Pastebin分享成功",
			"share_url":    shareConfig.URL,
			"share_config": shareConfig,
		})

		logger.Info("[Pastebin分享API] Pastebin分享成功", "subscription_id", req.SubscriptionID, "url", shareConfig.URL)
	})
}