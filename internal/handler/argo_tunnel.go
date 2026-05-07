package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox/integration"
	"miaomiaowu/internal/storage"
)

// ArgoTunnelCreateRequest 创建Argo隧道请求
type ArgoTunnelCreateRequest struct {
	Name      string `json:"name"`
	Type      string `json:"type"`   // fixed, temp, argogo
	Domain    string `json:"domain"` // 仅固定域名隧道需要
	Token     string `json:"token"`  // 仅固定域名隧道需要
	LocalPort int    `json:"local_port"`
}

// ArgoTunnelActionRequest 隧道操作请求
type ArgoTunnelActionRequest struct {
	TunnelID string `json:"tunnel_id"`
	Action   string `json:"action"` // start, stop, delete
}

// NewArgoTunnelListHandler 创建Argo隧道列表处理器
func NewArgoTunnelListHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		am := integration.NewArgoManager()
		tunnels, err := am.ListTunnels()
		if err != nil {
			logger.Error("[Argo API] 获取隧道列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list tunnels failed: %w", err))
			return
		}

		// 获取每个隧道的状态
		result := make([]map[string]interface{}, 0)
		for _, tunnel := range tunnels {
			status, _ := am.GetTunnelStatus(tunnel.ID)

			tunnelData := map[string]interface{}{
				"id":            tunnel.ID,
				"name":          tunnel.Name,
				"type":          tunnel.Type,
				"domain":        tunnel.Domain,
				"enabled":       tunnel.Enabled,
				"port":          tunnel.Port,
				"local_service": tunnel.LocalService,
				"created_at":    tunnel.CreatedAt,
				"last_used":     tunnel.LastUsed,
				"status":        status,
			}
			result = append(result, tunnelData)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tunnels": result,
			"count":   len(result),
		})
	})
}

// NewArgoTunnelCreateHandler 创建Argo隧道创建处理器
func NewArgoTunnelCreateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ArgoTunnelCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		if req.LocalPort <= 0 || req.LocalPort > 65535 {
			writeError(w, http.StatusBadRequest, errors.New("invalid port number"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "argo_create", fmt.Sprintf("创建Argo隧道: %s", req.Name))

		am := integration.NewArgoManager()
		var tunnel *integration.ArgoTunnel
		var err error
		var quickTunnelURL string

		switch req.Type {
		case "fixed":
			// 固定域名隧道
			if req.Domain == "" || req.Token == "" {
				writeError(w, http.StatusBadRequest, errors.New("domain and token are required for fixed tunnels"))
				return
			}
			tunnel, err = am.CreateFixedTunnel(req.Name, req.Domain, req.Token, req.LocalPort)

		case "temp":
			// 临时隧道
			tunnel, err = am.CreateTempTunnel(req.Name, req.LocalPort)

		case "argogo":
			// argo-go隧道
			tunnel, err = am.CreateArgoGoTunnel(req.Name, req.LocalPort)

		case "quick":
			// 快速隧道
			tunnel, quickTunnelURL, err = am.CreateQuickTunnel(req.Name, req.LocalPort)
			if err == nil && quickTunnelURL != "" {
				// 返回快速隧道URL
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"status":  "success",
					"message": "快速隧道创建成功",
					"tunnel":  tunnel,
					"url":     quickTunnelURL,
				})
				return
			}

		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported tunnel type: %s", req.Type))
			return
		}

		if err != nil {
			logger.Error("[Argo API] 创建隧道失败", "error", err)
			logOperationWithError(repo, username, "argo_create", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create tunnel failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "隧道创建成功",
			"tunnel":  tunnel,
		})

		logger.Info("[Argo API] 隧道创建成功", "name", req.Name, "type", req.Type)
	})
}

// NewArgoTunnelActionHandler 创建Argo隧道操作处理器
func NewArgoTunnelActionHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ArgoTunnelActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.TunnelID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tunnel_id is required"))
			return
		}
		if req.Action == "" {
			writeError(w, http.StatusBadRequest, errors.New("action is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "argo_action", fmt.Sprintf("Argo隧道操作: %s %s", req.Action, req.TunnelID))

		am := integration.NewArgoManager()
		var err error

		switch req.Action {
		case "start":
			err = am.StartTunnel(req.TunnelID)
		case "stop":
			err = am.StopTunnel(req.TunnelID)
		case "delete":
			err = am.DeleteTunnel(req.TunnelID)
		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported action: %s", req.Action))
			return
		}

		if err != nil {
			logger.Error("[Argo API] 隧道操作失败", "action", req.Action, "error", err)
			logOperationWithError(repo, username, "argo_action", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("tunnel action failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("隧道%s成功", req.Action),
		})

		logger.Info("[Argo API] 隧道操作成功", "action", req.Action, "tunnel_id", req.TunnelID)
	})
}

// NewArgoTunnelStatusHandler 创建Argo隧道状态处理器
func NewArgoTunnelStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取隧道ID
		tunnelID := r.URL.Query().Get("tunnel_id")
		if tunnelID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tunnel_id parameter is required"))
			return
		}

		am := integration.NewArgoManager()
		status, err := am.GetTunnelStatus(tunnelID)
		if err != nil {
			logger.Error("[Argo API] 获取隧道状态失败", "error", err)
			writeError(w, http.StatusNotFound, fmt.Errorf("get tunnel status failed: %w", err))
			return
		}

		// 获取隧道指标
		metrics, _ := am.GetTunnelMetrics(tunnelID)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  status,
			"metrics": metrics,
		})
	})
}

// NewArgoTunnelLogsHandler 创建Argo隧道日志处理器
func NewArgoTunnelLogsHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and POST are supported"))
			return
		}

		// 从URL获取隧道ID
		tunnelID := r.URL.Query().Get("tunnel_id")
		if tunnelID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tunnel_id parameter is required"))
			return
		}

		// 获取日志行数
		linesStr := r.URL.Query().Get("lines")
		lines := 100
		if linesStr != "" {
			if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
				lines = n
			}
		}

		am := integration.NewArgoManager()
		logs, err := am.GetTunnelLogs(tunnelID, lines)
		if err != nil {
			logger.Error("[Argo API] 获取隧道日志失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get tunnel logs failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tunnel_id": tunnelID,
			"logs":      logs,
			"lines":     lines,
		})
	})
}

// NewArgoTunnelInstallHandler 创建Argo工具安装处理器
func NewArgoTunnelInstallHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			ToolType string `json:"tool_type"` // cloudflared, argogo, argotunnel
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "argo_install", fmt.Sprintf("安装Argo工具: %s", req.ToolType))

		am := integration.NewArgoManager()
		var err error

		switch req.ToolType {
		case "cloudflared":
			progressChan := make(chan int, 10)
			go func() {
				for progress := range progressChan {
					// 这里可以通过SSE推送进度
					logger.Info("[Argo API] 安装进度", "progress", progress)
				}
			}()
			err = am.DownloadCloudflared(progressChan)
			close(progressChan)

		case "argogo":
			progressChan := make(chan int, 10)
			go func() {
				for progress := range progressChan {
					logger.Info("[Argo API] 安装进度", "progress", progress)
				}
			}()
			err = am.DownloadArgoGo(progressChan)
			close(progressChan)

		case "argotunnel":
			progressChan := make(chan int, 10)
			go func() {
				for progress := range progressChan {
					logger.Info("[Argo API] 安装进度", "progress", progress)
				}
			}()
			err = am.DownloadArgoTunnelQuickTunnels(progressChan)
			close(progressChan)

		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported tool type: %s", req.ToolType))
			return
		}

		if err != nil {
			logger.Error("[Argo API] 安装工具失败", "tool_type", req.ToolType, "error", err)
			logOperationWithError(repo, username, "argo_install", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("install tool failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("%s安装成功", req.ToolType),
		})

		logger.Info("[Argo API] 工具安装成功", "tool_type", req.ToolType)
	})
}

// NewArgoTokenValidatorHandler 创建Argo Token验证处理器
func NewArgoTokenValidatorHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证Token
		valid := integration.ValidateToken(req.Token)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   valid,
			"message": map[bool]string{true: "Token格式正确", false: "Token格式错误"}[valid],
		})
	})
}

// NewArgoQuickTunnelHandler 创建Argo快速隧道处理器
func NewArgoQuickTunnelHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			Name      string `json:"name"`
			LocalPort int    `json:"local_port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		if req.LocalPort <= 0 || req.LocalPort > 65535 {
			writeError(w, http.StatusBadRequest, errors.New("invalid port number"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "argo_quick", fmt.Sprintf("创建快速隧道: %s", req.Name))

		am := integration.NewArgoManager()
		tunnel, url, err := am.CreateQuickTunnel(req.Name, req.LocalPort)
		if err != nil {
			logger.Error("[Argo API] 创建快速隧道失败", "error", err)
			logOperationWithError(repo, username, "argo_quick", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create quick tunnel failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "快速隧道创建成功",
			"tunnel":  tunnel,
			"url":     url,
		})

		logger.Info("[Argo API] 快速隧道创建成功", "name", req.Name, "url", url)
	})
}

// NewArgoMetricsHandler 创建Argo指标处理器
func NewArgoMetricsHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取隧道ID
		tunnelID := r.URL.Query().Get("tunnel_id")
		if tunnelID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tunnel_id parameter is required"))
			return
		}

		am := integration.NewArgoManager()
		metrics, err := am.GetTunnelMetrics(tunnelID)
		if err != nil {
			logger.Error("[Argo API] 获取隧道指标失败", "error", err)
			writeError(w, http.StatusNotFound, fmt.Errorf("get tunnel metrics failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, metrics)
	})
}

// NewArgoTunnelStreamLogsHandler 创建Argo隧道日志流处理器
func NewArgoTunnelStreamLogsHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取隧道ID
		tunnelID := r.URL.Query().Get("tunnel_id")
		if tunnelID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tunnel_id parameter is required"))
			return
		}

		// 设置SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// 获取日志流
		am := integration.NewArgoManager()
		logChan, err := am.FollowTunnelLogs(tunnelID)
		if err != nil {
			logger.Error("[Argo API] 获取日志流失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("follow logs failed: %w", err))
			return
		}

		// 设置SSE超时
		ctx := r.Context()
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
			return
		}

		// 发送日志
		timeout := time.NewTimer(5 * time.Minute)
		defer timeout.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timeout.C:
				return
			case log, ok := <-logChan:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", log)
				flusher.Flush()
				timeout.Reset(30 * time.Second)
			}
		}
	})
}
