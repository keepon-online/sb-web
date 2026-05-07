package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// ConfigGenerateRequest 配置生成请求
type ConfigGenerateRequest struct {
	Protocol string                 `json:"protocol"` // vless, vmess, hysteria2, tuic, anytls
	Options  map[string]interface{} `json:"options"`
}

// ConfigGenerateResponse 配置生成响应
type ConfigGenerateResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Config  *singbox.SingboxConfig `json:"config,omitempty"`
	Link    string                 `json:"link,omitempty"`
	Links   map[string]string      `json:"links,omitempty"`
	Port    int                    `json:"port,omitempty"`
}

// ConfigSaveRequest 配置保存请求
type ConfigSaveRequest struct {
	Name   string                 `json:"name"`
	Config *singbox.SingboxConfig `json:"config"`
}

// ConfigPublishRequest 配置发布请求
type ConfigPublishRequest struct {
	ConfigID         int64    `json:"config_id"`
	Protocols        []string `json:"protocols"`
	Tags             []string `json:"tags"`
	Enabled          bool     `json:"enabled"`
	ExternalHost     string   `json:"external_host"`
	RealityPublicKey string   `json:"reality_public_key"`
}

// ConfigPublishResponse 配置发布响应
type ConfigPublishResponse struct {
	Success      bool           `json:"success"`
	Message      string         `json:"message"`
	CreatedCount int            `json:"created_count"`
	UpdatedCount int            `json:"updated_count"`
	Created      []storage.Node `json:"created"`
	Updated      []storage.Node `json:"updated"`
}

// PublishedNodesResponse 已发布节点响应
type PublishedNodesResponse struct {
	Nodes []singbox.PublishedNode `json:"nodes"`
}

// PortAllocateRequest 端口分配请求
type PortAllocateRequest struct {
	Count   int `json:"count"`
	MinPort int `json:"min_port,omitempty"`
	MaxPort int `json:"max_port,omitempty"`
}

// NewSingboxConfigGenerateHandler 创建配置生成处理器
func NewSingboxConfigGenerateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ConfigGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_config_generate", fmt.Sprintf("生成配置: %s", req.Protocol))

		// 生成配置
		config, links, port, err := generateConfigWithLinks(req.Protocol, req.Options)
		if err != nil {
			logger.Error("[Singbox API] 配置生成失败", "error", err)
			logOperationWithError(repo, username, "singbox_config_generate", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("generate config failed: %w", err))
			return
		}
		link := ""
		if len(links) == 1 {
			for _, value := range links {
				link = value
			}
		}

		writeJSON(w, http.StatusOK, ConfigGenerateResponse{
			Success: true,
			Message: "配置生成成功",
			Config:  config,
			Link:    link,
			Links:   links,
			Port:    port,
		})

		logger.Info("[Singbox API] 配置生成成功", "protocol", req.Protocol, "port", port)
	})
}

// NewSingboxConfigSaveHandler 创建配置保存处理器
func NewSingboxConfigSaveHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req ConfigSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证配置
		generator := singbox.NewConfigGenerator()
		if err := generator.ValidateConfig(req.Config); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid config: %w", err))
			return
		}

		// 保存配置
		filename := req.Name + ".json"
		if err := generator.SaveConfig(req.Config, filename); err != nil {
			logger.Error("[Singbox API] 保存配置失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("save config failed: %w", err))
			return
		}

		// 记录到数据库
		configJSON, _ := json.Marshal(req.Config)
		singboxConfig := &storage.SingboxConfig{
			Name:       req.Name,
			Protocol:   extractProtocol(req.Config),
			Port:       extractPort(req.Config),
			ConfigJSON: string(configJSON),
			Enabled:    true,
		}

		if err := repo.CreateSingboxConfig(singboxConfig); err != nil {
			logger.Warn("[Singbox API] 数据库记录创建失败", "error", err)
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "配置保存成功",
			"file":    filename,
		})

		logger.Info("[Singbox API] 配置保存成功", "name", req.Name)
	})
}

// NewSingboxConfigListHandler 创建配置列表处理器
func NewSingboxConfigListHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 获取配置列表
		configs, err := repo.GetSingboxConfigs()
		if err != nil {
			logger.Error("[Singbox API] 获取配置列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get configs failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"configs": configs,
		})
	})
}

// NewSingboxConfigPublishHandler 创建配置发布处理器
func NewSingboxConfigPublishHandler(repo *storage.TrafficRepository) http.Handler {
	publisher := singbox.NewNodePublisher(repo)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var req ConfigPublishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}
		if req.ConfigID <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("config_id is required"))
			return
		}

		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_config_publish", fmt.Sprintf("发布配置到订阅: %d", req.ConfigID))

		result, err := publisher.PublishConfig(r.Context(), singbox.PublishRequest{
			ConfigID:         req.ConfigID,
			Protocols:        req.Protocols,
			Tags:             req.Tags,
			Enabled:          req.Enabled,
			ExternalHost:     req.ExternalHost,
			RealityPublicKey: req.RealityPublicKey,
		})
		if err != nil {
			logger.Error("[Singbox API] 配置发布失败", "error", err)
			logOperationWithError(repo, username, "singbox_config_publish", err.Error())
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusOK, ConfigPublishResponse{
			Success:      true,
			Message:      "发布成功",
			CreatedCount: len(result.Created),
			UpdatedCount: len(result.Updated),
			Created:      result.Created,
			Updated:      result.Updated,
		})
	})
}

// NewSingboxPublishedNodesHandler 创建已发布节点列表处理器
func NewSingboxPublishedNodesHandler(repo *storage.TrafficRepository) http.Handler {
	publisher := singbox.NewNodePublisher(repo)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		configID, err := strconv.ParseInt(r.URL.Query().Get("config_id"), 10, 64)
		if err != nil || configID <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("valid config_id is required"))
			return
		}

		nodes, err := publisher.ListPublishedNodes(r.Context(), configID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list published nodes: %w", err))
			return
		}
		writeJSON(w, http.StatusOK, PublishedNodesResponse{Nodes: nodes})
	})
}

// NewSingboxPublishedNodeDeleteHandler 创建已发布节点删除处理器
func NewSingboxPublishedNodeDeleteHandler(repo *storage.TrafficRepository) http.Handler {
	publisher := singbox.NewNodePublisher(repo)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only DELETE is supported"))
			return
		}

		nodeID, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || nodeID <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("valid id is required"))
			return
		}

		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_published_node_delete", fmt.Sprintf("取消发布节点: %d", nodeID))

		if err := publisher.DeletePublishedNode(r.Context(), nodeID); err != nil {
			logger.Error("[Singbox API] 取消发布失败", "error", err)
			logOperationWithError(repo, username, "singbox_published_node_delete", err.Error())
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "取消发布成功",
		})
	})
}

// NewSingboxPortAllocateHandler 创建端口分配处理器
func NewSingboxPortAllocateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req PortAllocateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 设置默认值
		if req.Count <= 0 {
			req.Count = 1
		}
		if req.MinPort <= 0 {
			req.MinPort = 10000
		}
		if req.MaxPort <= 0 {
			req.MaxPort = 65535
		}

		// 验证端口范围
		pm := singbox.NewPortManager()
		if err := pm.ValidatePortRange(req.MinPort, req.MaxPort); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		// 分配端口
		var ports []int
		var err error

		if req.Count == 1 {
			port, err := pm.AllocatePortInRange(req.MinPort, req.MaxPort)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			ports = []int{port}
		} else {
			ports, err = pm.BatchAllocatePorts(req.Count, req.MinPort, req.MaxPort)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_port_allocate", fmt.Sprintf("分配端口: %v", ports))

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "success",
			"ports":  ports,
		})

		logger.Info("[Singbox API] 端口分配成功", "ports", ports)
	})
}

// NewSingboxPortCheckHandler 创建端口检查处理器
func NewSingboxPortCheckHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			Ports []int `json:"ports"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 检查端口冲突
		pm := singbox.NewPortManager()
		conflicts := pm.CheckPortConflict(req.Ports)

		// 构建结果
		results := make(map[int]bool)
		for _, port := range req.Ports {
			results[port] = true
			for _, conflict := range conflicts {
				if port == conflict {
					results[port] = false
					break
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": results,
			"conflicts": conflicts,
		})
	})
}

// NewSingboxPortStatusHandler 创建端口状态处理器
func NewSingboxPortStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		pm := singbox.NewPortManager()

		// 获取查询参数
		minPort := 10000
		maxPort := 65535

		if minStr := r.URL.Query().Get("min"); minStr != "" {
			if _, err := fmt.Sscanf(minStr, "%d", &minPort); err != nil {
				minPort = 10000
			}
		}

		if maxStr := r.URL.Query().Get("max"); maxStr != "" {
			if _, err := fmt.Sscanf(maxStr, "%d", &maxPort); err != nil {
				maxPort = 65535
			}
		}

		// 获取端口状态
		usedPorts := pm.GetUsedPorts()
		availableCount := pm.GetAvailablePortCount(minPort, maxPort)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"used_ports":      usedPorts,
			"available_count": availableCount,
			"range": map[string]int{
				"min": minPort,
				"max": maxPort,
			},
		})
	})
}

// 辅助函数

func generateConfig(protocol string, options map[string]interface{}) (*singbox.SingboxConfig, string, int, error) {
	config, links, port, err := generateConfigWithLinks(protocol, options)
	if err != nil {
		return nil, "", 0, err
	}
	link := ""
	if len(links) == 1 {
		for _, value := range links {
			link = value
		}
	}
	return config, link, port, nil
}

func generateConfigWithLinks(protocol string, options map[string]interface{}) (*singbox.SingboxConfig, map[string]string, int, error) {
	if protocol == "server" || protocol == "multi" || protocol == "sb-server" {
		opts := singbox.ServerConfigOptions{
			ExternalHost:       getStringOption(options, "external_host", ""),
			Hostname:           getStringOption(options, "hostname", ""),
			UUID:               getStringOption(options, "uuid", ""),
			Password:           getStringOption(options, "password", ""),
			RealitySNI:         getStringOption(options, "reality_sni", "apple.com"),
			RealityPrivateKey:  getStringOption(options, "reality_private_key", ""),
			RealityPublicKey:   getStringOption(options, "reality_public_key", ""),
			RealityShortID:     getStringOption(options, "reality_short_id", ""),
			WebSocketPath:      getStringOption(options, "websocket_path", "/vmessws"),
			CertificatePath:    getStringOption(options, "certificate_path", "/etc/s-box/cert.pem"),
			PrivateKeyPath:     getStringOption(options, "private_key_path", "/etc/s-box/private.key"),
			VlessRealityPort:   getIntOption(options, "vless_reality_port", 10000),
			VmessWebSocketPort: getIntOption(options, "vmess_websocket_port", 10001),
			Hysteria2Port:      getIntOption(options, "hysteria2_port", 10002),
			TUICPort:           getIntOption(options, "tuic_port", 10003),
			AnyTLSPort:         getIntOption(options, "anytls_port", 10004),
		}
		config, err := singbox.BuildServerConfig(opts)
		if err != nil {
			return nil, nil, 0, err
		}
		links, err := singbox.GenerateShareLinks(opts)
		if err != nil {
			return nil, nil, 0, err
		}
		return config, links, opts.VlessRealityPort, nil
	}

	generator := singbox.NewConfigGenerator()

	// 构建协议选项
	protocolOpts := singbox.ProtocolOptions{
		Server:     getStringOption(options, "server", ""),
		ServerPort: getIntOption(options, "server_port", 443),
		UUID:       getStringOption(options, "uuid", ""),
		Password:   getStringOption(options, "password", ""),
		Domain:     getStringOption(options, "domain", ""),
		Path:       getStringOption(options, "path", "/"),
		Host:       getStringOption(options, "host", ""),
		PublicKey:  getStringOption(options, "public_key", ""),
		ShortID:    getStringOption(options, "short_id", ""),
		UpMbps:     getIntOption(options, "up_mbps", 100),
		DownMbps:   getIntOption(options, "down_mbps", 100),
	}

	// 如果没有指定端口，自动分配
	if protocolOpts.ServerPort == 0 || protocolOpts.ServerPort == 443 {
		pm := singbox.NewPortManager()
		port, err := pm.AllocatePort()
		if err != nil {
			return nil, nil, 0, fmt.Errorf("allocate port: %w", err)
		}
		protocolOpts.ServerPort = port
	}

	// 生成配置
	config, err := generator.GenerateProtocolConfig(singbox.ProtocolType(protocol), protocolOpts)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("generate protocol config: %w", err)
	}

	// 生成链接 (暂时返回空字符串)
	links := map[string]string{}
	port := protocolOpts.ServerPort

	return config, links, port, nil
}

func extractProtocol(config *singbox.SingboxConfig) string {
	// 从配置中提取协议类型
	if len(config.Outbounds) > 0 {
		return string(config.Outbounds[0].Type)
	}
	return "unknown"
}

func extractPort(config *singbox.SingboxConfig) int {
	// 从配置中提取端口
	if len(config.Outbounds) > 0 {
		return config.Outbounds[0].ServerPort
	}
	return 0
}

func getStringOption(options map[string]interface{}, key, defaultValue string) string {
	if value, ok := options[key]; ok {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return defaultValue
}

func getIntOption(options map[string]interface{}, key string, defaultValue int) int {
	if value, ok := options[key]; ok {
		if intValue, ok := value.(float64); ok {
			return int(intValue)
		}
	}
	return defaultValue
}
