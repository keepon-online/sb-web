package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// SystemOptimizationRequest 系统优化请求
type SystemOptimizationRequest struct {
	Action  string `json:"action"`  // enable-bbr, disable-bbr, optimize-all, optimize-bbr
	Version string `json:"version"` // bbr, bbr2, bbr3
}

// NetworkSpeedTestRequest 网络速度测试请求
type NetworkSpeedTestRequest struct {
	Target string `json:"target"` // 测试目标主机
}

// NewSystemOptimizationHandler 创建系统优化处理器
func NewSystemOptimizationHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req SystemOptimizationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Action == "" {
			writeError(w, http.StatusBadRequest, errors.New("action is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "system_optimize", fmt.Sprintf("系统优化: %s", req.Action))

		optimizer := singbox.NewSystemOptimizer()
		var err error

		switch req.Action {
		case "enable-bbr":
			err = optimizer.EnableBBR(req.Version)

		case "disable-bbr":
			err = optimizer.DisableBBR()

		case "optimize-all":
			err = optimizer.OptimizeSystemSettings()

		case "optimize-bbr":
			if req.Version == "" {
				req.Version = "bbr"
			}
			err = optimizer.EnableBBR(req.Version)
			if err == nil {
				// 同时应用其他系统优化
				err = optimizer.OptimizeSystemSettings()
			}

		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported action: %s", req.Action))
			return
		}

		if err != nil {
			logger.Error("[系统优化API] 优化失败", "action", req.Action, "error", err)
			logOperationWithError(repo, username, "system_optimize", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("system optimization failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("系统优化操作 %s 执行成功", req.Action),
		})

		logger.Info("[系统优化API] 优化操作成功", "action", req.Action)
	})
}

// NewBBRStatusHandler 创建BBR状态处理器
func NewBBRStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		bbrState, err := optimizer.GetBBRState()
		if err != nil {
			logger.Error("[系统优化API] 获取BBR状态失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get BBR status failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, bbrState)
	})
}

// NewNetworkPerformanceHandler 创建网络性能检查处理器
func NewNetworkPerformanceHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		performance, err := optimizer.CheckNetworkPerformance()
		if err != nil {
			logger.Error("[系统优化API] 网络性能检查失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("network performance check failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, performance)
	})
}

// NewNetworkSpeedTestHandler 创建网络速度测试处理器
func NewNetworkSpeedTestHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST and GET are supported"))
			return
		}

		var target string

		if r.Method == http.MethodPost {
			// 解析请求
			var req NetworkSpeedTestRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
				return
			}
			target = req.Target
		} else {
			// 从URL参数获取
			target = r.URL.Query().Get("target")
		}

		if target == "" {
			target = "www.google.com" // 默认测试目标
		}

		optimizer := singbox.NewSystemOptimizer()
		results, err := optimizer.TestNetworkSpeed(target)
		if err != nil {
			logger.Error("[系统优化API] 网络速度测试失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("network speed test failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"target":       target,
			"test_time":    time.Now().Format("2006-01-02 15:04:05"),
			"results":      results,
		})
	})
}

// NewSystemResourceUsageHandler 创建系统资源使用处理器
func NewSystemResourceUsageHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		usage, err := optimizer.GetSystemResourceUsage()
		if err != nil {
			logger.Error("[系统优化API] 获取系统资源使用失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get system resource usage failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, usage)
	})
}

// NewSystemReportHandler 创建系统报告处理器
func NewSystemReportHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		report, err := optimizer.CreateSystemReport()
		if err != nil {
			logger.Error("[系统优化API] 生成系统报告失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("create system report failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, report)
	})
}

// NewKernelParameterHandler 创建内核参数处理器
func NewKernelParameterHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and POST are supported"))
			return
		}

		if r.Method == http.MethodGet {
			// 获取内核参数
			param := r.URL.Query().Get("param")
			if param == "" {
				writeError(w, http.StatusBadRequest, errors.New("param parameter is required"))
				return
			}

			optimizer := singbox.NewSystemOptimizer()
			value, err := optimizer.GetKernelParameter(param)
			if err != nil {
				logger.Error("[系统优化API] 获取内核参数失败", "param", param, "error", err)
				writeError(w, http.StatusInternalServerError, fmt.Errorf("get kernel parameter failed: %w", err))
				return
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"param": param,
				"value": value,
			})
		} else {
			// 设置内核参数
			var req struct {
				Param string `json:"param"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
				return
			}

			if req.Param == "" || req.Value == "" {
				writeError(w, http.StatusBadRequest, errors.New("param and value are required"))
				return
			}

			// 记录操作日志
			username := getUsernameFromContext(r.Context())
			logOperation(repo, username, "kernel_param_set", fmt.Sprintf("设置内核参数: %s=%s", req.Param, req.Value))

			optimizer := singbox.NewSystemOptimizer()
			if err := optimizer.SetKernelParameter(req.Param, req.Value); err != nil {
				logger.Error("[系统优化API] 设置内核参数失败", "param", req.Param, "error", err)
				logOperationWithError(repo, username, "kernel_param_set", err.Error())
				writeError(w, http.StatusInternalServerError, fmt.Errorf("set kernel parameter failed: %w", err))
				return
			}

			writeJSON(w, http.StatusOK, map[string]string{
				"status":  "success",
				"message": fmt.Sprintf("内核参数 %s 设置成功", req.Param),
			})
		}
	})
}

// NewNetworkInterfacesHandler 创建网络接口处理器
func NewNetworkInterfacesHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		interfaces, err := optimizer.GetNetworkInterfaces()
		if err != nil {
			logger.Error("[系统优化API] 获取网络接口失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get network interfaces failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"interfaces": interfaces,
			"count":      len(interfaces),
		})
	})
}

// NewRoutingTableHandler 创建路由表处理器
func NewRoutingTableHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		routes, err := optimizer.GetRoutingTable()
		if err != nil {
			logger.Error("[系统优化API] 获取路由表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get routing table failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"routes": routes,
			"count":  len(routes),
		})
	})
}

// NewDNSConfigurationHandler 创建DNS配置处理器
func NewDNSConfigurationHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		dns, err := optimizer.GetDNSConfiguration()
		if err != nil {
			logger.Error("[系统优化API] 获取DNS配置失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get DNS configuration failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dns_servers": dns,
			"count":       len(dns),
		})
	})
}

// NewActiveConnectionsHandler 创建活动连接处理器
func NewActiveConnectionsHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		optimizer := singbox.NewSystemOptimizer()
		connections, err := optimizer.GetActiveConnections()
		if err != nil {
			logger.Error("[系统优化API] 获取活动连接失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get active connections failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"active_connections": connections,
		})
	})
}