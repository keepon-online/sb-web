package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// InstallRequest 安装请求
type InstallRequest struct {
	Version string `json:"version"` // Sing-box 版本，空字符串表示最新版本
}

// InstallResponse 安装响应
type InstallResponse struct {
	Status  string `json:"status"`  // 安装状态
	Message string `json:"message"` // 状态消息
	Version string `json:"version"` // 安装的版本
}

// UninstallResponse 卸载响应
type UninstallResponse struct {
	Status  string `json:"status"`  // 卸载状态
	Message string `json:"message"` // 状态消息
}

// NewSingboxInstallHandler 创建安装处理器
func NewSingboxInstallHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req InstallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 创建安装器
		installer, err := singbox.NewInstaller()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create installer: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_install", fmt.Sprintf("安装 Sing-box 版本: %s", req.Version))

		// 执行安装
		if err := installer.Install(req.Version); err != nil {
			logger.Error("[Singbox API] 安装失败", "error", err)
			logOperationWithError(repo, username, "singbox_install", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("installation failed: %w", err))
			return
		}

		// 获取安装的版本
		version, _ := installer.GetVersion()

		// 返回成功响应
		writeJSON(w, http.StatusOK, InstallResponse{
			Status:  "success",
			Message: "Sing-box 安装成功",
			Version: version,
		})

		logger.Info("[Singbox API] 安装成功", "version", version)
	})
}

// NewSingboxUninstallHandler 创建卸载处理器
func NewSingboxUninstallHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 创建安装器
		installer, err := singbox.NewInstaller()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create installer: %w", err))
			return
		}

		// 检查是否已安装
		if !installer.IsInstalled() {
			writeError(w, http.StatusBadRequest, errors.New("Sing-box 未安装"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_uninstall", "卸载 Sing-box")

		// 执行卸载
		if err := installer.Uninstall(); err != nil {
			logger.Error("[Singbox API] 卸载失败", "error", err)
			logOperationWithError(repo, username, "singbox_uninstall", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("uninstallation failed: %w", err))
			return
		}

		// 返回成功响应
		writeJSON(w, http.StatusOK, UninstallResponse{
			Status:  "success",
			Message: "Sing-box 卸载成功",
		})

		logger.Info("[Singbox API] 卸载成功")
	})
}

// NewSingboxInstallStatusHandler 创建安装状态处理器
func NewSingboxInstallStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 创建安装器
		installer, err := singbox.NewInstaller()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create installer: %w", err))
			return
		}

		// 检查安装状态
		installed := installer.IsInstalled()
		var version string
		if installed {
			version, _ = installer.GetVersion()
		}

		// 返回状态
		status := map[string]interface{}{
			"installed": installed,
			"version":   version,
		}

		// 添加环境信息
		env := singbox.DetectEnvironment()
		sysInfo, _ := singbox.GetSystemInfo()
		status["environment"] = env.String()
		status["system_info"] = sysInfo

		writeJSON(w, http.StatusOK, status)
	})
}

// NewSingboxInstallSSEHandler 创建带进度的安装处理器 (SSE)
func NewSingboxInstallSSEHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 设置 SSE 头
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// 发送进度函数
		sendProgress := func(step string, progress int, message string) {
			data := map[string]interface{}{
				"step":     step,
				"progress": progress,
				"message":  message,
			}
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
		}

		// 解析请求
		var req InstallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendProgress("error", 0, fmt.Sprintf("请求解析失败: %v", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())

		// 创建安装器并设置进度回调
		installer, err := singbox.NewInstaller()
		if err != nil {
			sendProgress("error", 0, fmt.Sprintf("创建安装器失败: %v", err))
			logOperationWithError(repo, username, "singbox_install", err.Error())
			return
		}

		installer.SetProgressCallback(func(progress singbox.InstallProgress) {
			sendProgress(progress.Step, progress.Progress, progress.Message)
		})

		// 执行安装
		if err := installer.Install(req.Version); err != nil {
			logger.Error("[Singbox API] 安装失败", "error", err)
			sendProgress("error", 0, fmt.Sprintf("安装失败: %v", err))
			logOperationWithError(repo, username, "singbox_install", err.Error())
			return
		}

		// 获取版本
		version, _ := installer.GetVersion()
		sendProgress("completed", 100, fmt.Sprintf("安装成功，版本: %s", version))

		logger.Info("[Singbox API] SSE 安装完成", "version", version)
	})
}

// NewSingboxEnvironmentHandler 创建环境信息处理器
func NewSingboxEnvironmentHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 获取环境摘要
		summary := singbox.GetEnvironmentSummary()

		writeJSON(w, http.StatusOK, summary)
	})
}

// 辅助函数

func getUsernameFromContext(ctx context.Context) string {
	if username, ok := ctx.Value("username").(string); ok {
		return username
	}
	return "unknown"
}

func logOperation(repo *storage.TrafficRepository, username, operation, details string) {
	if repo != nil {
		log := &storage.SystemOperationLog{
			Username:  username,
			Operation: operation,
			Details:   details,
			Status:    "started",
		}
		_ = repo.LogSystemOperation(log)
	}
}

func logOperationWithError(repo *storage.TrafficRepository, username, operation, errorMsg string) {
	if repo != nil {
		log := &storage.SystemOperationLog{
			Username:     username,
			Operation:    operation,
			ErrorMessage: errorMsg,
			Status:       "failed",
		}
		_ = repo.LogSystemOperation(log)
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// 确保导出类型
type InstallProgress = singbox.InstallProgress

// 为安装过程添加超时控制
func installWithTimeout(installer *singbox.Installer, version string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- installer.Install(version)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("installation timeout after %v", timeout)
	}
}
