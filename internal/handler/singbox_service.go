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

// ServiceRequest 服务操作请求
type ServiceRequest struct {
	Action string `json:"action"` // start, stop, restart, enable, disable
}

// ServiceResponse 服务操作响应
type ServiceResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NewSingboxServiceStartHandler 创建启动服务处理器
func NewSingboxServiceStartHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_service_start", "启动 Sing-box 服务")

		// 启动服务
		if err := manager.Start(); err != nil {
			logger.Error("[Singbox API] 启动服务失败", "error", err)
			logOperationWithError(repo, username, "singbox_service_start", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("start service failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, ServiceResponse{
			Status:  "success",
			Message: "服务启动成功",
		})

		logger.Info("[Singbox API] 服务启动成功")
	})
}

// NewSingboxServiceStopHandler 创建停止服务处理器
func NewSingboxServiceStopHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_service_stop", "停止 Sing-box 服务")

		// 停止服务
		if err := manager.Stop(); err != nil {
			logger.Error("[Singbox API] 停止服务失败", "error", err)
			logOperationWithError(repo, username, "singbox_service_stop", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("stop service failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, ServiceResponse{
			Status:  "success",
			Message: "服务已停止",
		})

		logger.Info("[Singbox API] 服务停止成功")
	})
}

// NewSingboxServiceRestartHandler 创建重启服务处理器
func NewSingboxServiceRestartHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_service_restart", "重启 Sing-box 服务")

		// 重启服务
		if err := manager.Restart(); err != nil {
			logger.Error("[Singbox API] 重启服务失败", "error", err)
			logOperationWithError(repo, username, "singbox_service_restart", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("restart service failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, ServiceResponse{
			Status:  "success",
			Message: "服务已重启",
		})

		logger.Info("[Singbox API] 服务重启成功")
	})
}

// NewSingboxServiceEnableHandler 创建启用服务处理器
func NewSingboxServiceEnableHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_service_enable", "启用 Sing-box 服务开机自启")

		// 启用服务
		if err := manager.Enable(); err != nil {
			logger.Error("[Singbox API] 启用服务失败", "error", err)
			logOperationWithError(repo, username, "singbox_service_enable", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("enable service failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, ServiceResponse{
			Status:  "success",
			Message: "服务已启用开机自启",
		})

		logger.Info("[Singbox API] 服务启用成功")
	})
}

// NewSingboxServiceDisableHandler 创建禁用服务处理器
func NewSingboxServiceDisableHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_service_disable", "禁用 Sing-box 服务开机自启")

		// 禁用服务
		if err := manager.Disable(); err != nil {
			logger.Error("[Singbox API] 禁用服务失败", "error", err)
			logOperationWithError(repo, username, "singbox_service_disable", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("disable service failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, ServiceResponse{
			Status:  "success",
			Message: "服务已禁用开机自启",
		})

		logger.Info("[Singbox API] 服务禁用成功")
	})
}

// NewSingboxServiceStatusHandler 创建服务状态处理器
func NewSingboxServiceStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 获取服务状态
		status, err := manager.Status()
		if err != nil {
			logger.Error("[Singbox API] 获取服务状态失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get service status failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

// NewSingboxServiceLogsHandler 创建日志查看处理器
func NewSingboxServiceLogsHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 获取行数参数
		lines := 100 // 默认100行
		if linesStr := r.URL.Query().Get("lines"); linesStr != "" {
			if _, err := fmt.Sscanf(linesStr, "%d", &lines); err != nil {
				writeError(w, http.StatusBadRequest, errors.New("invalid lines parameter"))
				return
			}
			if lines <= 0 || lines > 10000 {
				writeError(w, http.StatusBadRequest, errors.New("lines must be between 1 and 10000"))
				return
			}
		}

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create service manager: %w", err))
			return
		}

		// 获取日志
		logs, err := manager.Logs(lines)
		if err != nil {
			logger.Error("[Singbox API] 获取日志失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get logs failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"logs": logs,
		})
	})
}

// NewSingboxServiceLogsStreamHandler 创建实时日志流处理器 (SSE)
func NewSingboxServiceLogsStreamHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
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

		// 创建服务管理器
		manager, err := createServiceManager()
		if err != nil {
			sendSSEError(w, flusher, fmt.Sprintf("创建服务管理器失败: %v", err))
			return
		}

		// 创建上下文，用于控制日志流
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// 发送连接成功消息
		sendSSEMessage(w, flusher, map[string]string{
			"type":    "connected",
			"message": "日志流已连接",
		})

		// 跟踪日志
		err = manager.FollowLogs(ctx, func(logLine string) {
			sendSSEMessage(w, flusher, map[string]string{
				"type": "log",
				"log": logLine,
			})
		})

		if err != nil && err != context.Canceled {
			sendSSEError(w, flusher, fmt.Sprintf("日志流错误: %v", err))
		}
	})
}

// NewSingboxSystemStatusHandler 创建系统状态处理器
func NewSingboxSystemStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 获取系统信息
		sysInfo, err := singbox.GetSystemInfo()
		if err != nil {
			logger.Error("[Singbox API] 获取系统信息失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get system info failed: %w", err))
			return
		}

		// 获取环境信息
		env := singbox.DetectEnvironment()
		paths := singbox.GetConfigPaths(env)

		// 检查 TUN 支持
		tunSupported, _ := singbox.CheckTUNSupport()

		// 检查 Sing-box 安装状态
		installer, _ := singbox.NewInstaller()
		installed := installer.IsInstalled()
		var version string
		if installed {
			version, _ = installer.GetVersion()
		}

		// 获取服务状态
		manager, _ := singbox.NewServiceManager(env, paths)
		serviceStatus, _ := manager.Status()

		// 构建响应
		status := map[string]interface{}{
			"environment":      env.String(),
			"is_root":          singbox.IsRoot(),
			"tun_supported":    tunSupported,
			"singbox_installed": installed,
			"singbox_version":  version,
			"service_status":   serviceStatus,
			"system_info":      sysInfo,
			"config_paths":     paths,
			"timestamp":        time.Now().Unix(),
		}

		writeJSON(w, http.StatusOK, status)
	})
}

// 辅助函数

func createServiceManager() (singbox.ServiceManager, error) {
	env := singbox.DetectEnvironment()
	paths := singbox.GetConfigPaths(env)
	return singbox.NewServiceManager(env, paths)
}

func sendSSEMessage(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

func sendSSEError(w http.ResponseWriter, flusher http.Flusher, message string) {
	sendSSEMessage(w, flusher, map[string]string{
		"type":  "error",
		"error": message,
	})
}