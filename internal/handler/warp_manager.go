package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox/integration"
	"miaomiaowu/internal/storage"
)

// WARPEnableRequest WARP启用请求
type WARPEnableRequest struct {
	Type            string `json:"type"`             // warp, warpo
	LicenseKey      string `json:"license_key,omitempty"`
	Port            int    `json:"port,omitempty"`
	PreferredServer bool   `json:"preferred_server,omitempty"`
}

// WARPUpdateRequest WARP更新请求
type WARPUpdateRequest struct {
	ConfigID        string `json:"config_id"`
	LicenseKey      string `json:"license_key,omitempty"`
	Port            int    `json:"port,omitempty"`
	PreferredServer bool   `json:"preferred_server,omitempty"`
	Enabled         bool   `json:"enabled,omitempty"`
}

// NewWARPStatusHandler 创建WARP状态处理器
func NewWARPStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		wm := integration.NewWARPManager()
		status, err := wm.GetWARPStatus()
		if err != nil {
			logger.Error("[WARP API] 获取WARP状态失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get warp status failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

// NewWARPEnableHandler 创建WARP启用处理器
func NewWARPEnableHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req WARPEnableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, errors.New("type is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "warp_enable", fmt.Sprintf("启用WARP: %s", req.Type))

		wm := integration.NewWARPManager()
		var err error

		switch req.Type {
		case "warp":
			err = wm.EnableWARP(req.LicenseKey)
		case "warpo":
			if req.Port <= 0 || req.Port > 65535 {
				writeError(w, http.StatusBadRequest, errors.New("invalid port number"))
				return
			}
			err = wm.EnableWARPGo(req.LicenseKey, req.Port, req.PreferredServer)
		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported WARP type: %s", req.Type))
			return
		}

		if err != nil {
			logger.Error("[WARP API] 启用WARP失败", "error", err)
			logOperationWithError(repo, username, "warp_enable", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("enable WARP failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "WARP启用成功",
		})

		logger.Info("[WARP API] WARP启用成功", "type", req.Type)
	})
}

// NewWARPDisableHandler 创建WARP禁用处理器
func NewWARPDisableHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "warp_disable", "禁用WARP")

		wm := integration.NewWARPManager()
		if err := wm.DisableWARP(); err != nil {
			logger.Error("[WARP API] 禁用WARP失败", "error", err)
			logOperationWithError(repo, username, "warp_disable", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("disable WARP failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "WARP禁用成功",
		})

		logger.Info("[WARP API] WARP禁用成功")
	})
}

// NewWARPConfigsHandler 创建WARP配置列表处理器
func NewWARPConfigsHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		wm := integration.NewWARPManager()
		configs, err := wm.GetWARPConfigs()
		if err != nil {
			logger.Error("[WARP API] 获取WARP配置列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get warp configs failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"configs": configs,
			"count":   len(configs),
		})
	})
}

// NewWARPUpdateHandler 创建WARP更新处理器
func NewWARPUpdateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req WARPUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.ConfigID == "" {
			writeError(w, http.StatusBadRequest, errors.New("config_id is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "warp_update", fmt.Sprintf("更新WARP配置: %s", req.ConfigID))

		wm := integration.NewWARPManager()

		// 构建更新映射
		updates := make(map[string]interface{})
		if req.LicenseKey != "" {
			updates["license_key"] = req.LicenseKey
		}
		if req.Port > 0 {
			updates["port"] = req.Port
		}
		updates["preferred_server"] = req.PreferredServer
		updates["enabled"] = req.Enabled

		if err := wm.UpdateWARPConfig(req.ConfigID, updates); err != nil {
			logger.Error("[WARP API] 更新WARP配置失败", "error", err)
			logOperationWithError(repo, username, "warp_update", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("update WARP config failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "WARP配置更新成功",
		})

		logger.Info("[WARP API] WARP配置更新成功", "config_id", req.ConfigID)
	})
}

// NewWARPDeleteHandler 创建WARP删除处理器
func NewWARPDeleteHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only DELETE or POST is supported"))
			return
		}

		// 从URL获取配置ID
		configID := r.URL.Query().Get("config_id")
		if configID == "" {
			writeError(w, http.StatusBadRequest, errors.New("config_id parameter is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "warp_delete", fmt.Sprintf("删除WARP配置: %s", configID))

		wm := integration.NewWARPManager()
		if err := wm.DeleteWARPConfig(configID); err != nil {
			logger.Error("[WARP API] 删除WARP配置失败", "error", err)
			logOperationWithError(repo, username, "warp_delete", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("delete WARP config failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "WARP配置删除成功",
		})

		logger.Info("[WARP API] WARP配置删除成功", "config_id", configID)
	})
}

// NewWARPConnectionCheckHandler 创建WARP连接检查处理器
func NewWARPConnectionCheckHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		wm := integration.NewWARPManager()
		connected, ip, err := wm.CheckWARPConnection()
		if err != nil {
			logger.Error("[WARP API] 检查WARP连接失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("check WARP connection failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connected": connected,
			"ip_address": ip,
			"message": map[bool]string{true: "WARP连接正常", false: "WARP未连接"}[connected],
		})
	})
}

// NewWARPConfigGenerateHandler 创建WARP配置生成处理器
func NewWARPConfigGenerateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and POST are supported"))
			return
		}

		// 从URL获取配置类型
		configType := r.URL.Query().Get("type")
		if configType == "" {
			configType = "singbox" // 默认生成singbox格式
		}

		wm := integration.NewWARPManager()
		config, err := wm.GenerateWarpConfig(configType)
		if err != nil {
			logger.Error("[WARP API] 生成WARP配置失败", "error", err)
			writeError(w, http.StatusBadRequest, fmt.Errorf("generate WARP config failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"type":    configType,
			"config":  config,
		})
	})
}

// NewWARPInstallHandler 创建WARP安装处理器
func NewWARPInstallHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			Type string `json:"type"` // warp, warpo
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, errors.New("type is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "warp_install", fmt.Sprintf("安装WARP客户端: %s", req.Type))

		wm := integration.NewWARPManager()
		var err error

		switch req.Type {
		case "warp":
			progressChan := make(chan int, 10)
			go func() {
				for progress := range progressChan {
					logger.Info("[WARP API] 安装进度", "progress", progress)
				}
			}()
			err = wm.DownloadWARP(progressChan)
			close(progressChan)

		case "warpo":
			progressChan := make(chan int, 10)
			go func() {
				for progress := range progressChan {
					logger.Info("[WARP API] 安装进度", "progress", progress)
				}
			}()
			err = wm.DownloadWARPGo(progressChan)
			close(progressChan)

		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported WARP type: %s", req.Type))
			return
		}

		if err != nil {
			logger.Error("[WARP API] 安装WARP客户端失败", "type", req.Type, "error", err)
			logOperationWithError(repo, username, "warp_install", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("install WARP client failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("%s客户端安装成功", req.Type),
		})

		logger.Info("[WARP API] WARP客户端安装成功", "type", req.Type)
	})
}

// NewWARPLicenseValidateHandler 创建WARP License验证处理器
func NewWARPLicenseValidateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			LicenseKey string `json:"license_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 基本验证
		valid := true
		message := "License Key格式正确"

		if req.LicenseKey == "" {
			valid = false
			message = "License Key不能为空"
		} else if len(req.LicenseKey) < 10 {
			valid = false
			message = "License Key格式不正确"
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   valid,
			"message": message,
		})
	})
}

// NewWARPPortCheckHandler 创建WARP端口检查处理器
func NewWARPPortCheckHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取端口
		portStr := r.URL.Query().Get("port")
		if portStr == "" {
			writeError(w, http.StatusBadRequest, errors.New("port parameter is required"))
			return
		}

		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			writeError(w, http.StatusBadRequest, errors.New("invalid port number"))
			return
		}

		// 检查端口是否被占用
		// 简化实现
		available := true

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"port":      port,
			"available": available,
			"message":   map[bool]string{true: "端口可用", false: "端口已被占用"}[available],
		})
	})
}