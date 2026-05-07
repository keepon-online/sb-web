package singbox

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
)

// NodeInfo 节点信息
type NodeInfo struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // vless, vmess, hysteria2, tuic, etc.
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	UUID        string            `json:"uuid"`
	Password    string            `json:"password,omitempty"`
	Cipher      string            `json:"cipher,omitempty"`
	Network     string            `json:"network,omitempty"`     // tcp, ws, h2, grpc
	Security    string            `json:"security,omitempty"`    // tls, reality
	Path        string            `json:"path,omitempty"`        // ws path
	HostHeader  string            `json:"host_header,omitempty"` // ws host
	SNI         string            `json:"sni,omitempty"`         // tls sni
	Fingerprint string            `json:"fingerprint,omitempty"` // reality fingerprint
	PublicKey   string            `json:"public_key,omitempty"`  // reality public key
	ShortID     string            `json:"short_id,omitempty"`    // reality short id
	Obfs        string            `json:"obfs,omitempty"`        // hysteria2 obfs
	Up          int               `json:"up,omitempty"`          // hysteria2 up
	Down        int               `json:"down,omitempty"`        // hysteria2 down
	Extra       map[string]string `json:"extra,omitempty"`       // extra parameters
	CreatedAt   time.Time         `json:"created_at"`
}

// SubscriptionConfig 订阅配置
type SubscriptionConfig struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Nodes           []NodeInfo `json:"nodes"`
	Format          string     `json:"format"` // clash, v2ray, singbox, etc.
	UpdateTime      time.Time  `json:"update_time"`
	Enabled         bool       `json:"enabled"`
	AutoUpdate      bool       `json:"auto_update"`
	UpdateInterval  int        `json:"update_interval"`  // minutes
	ShareCode       string     `json:"share_code"`       // 短链接代码
	UserCode        string     `json:"user_code"`        // 用户代码
	SubscriptionURL string     `json:"subscription_url"` // 完整订阅URL
}

// SubscriptionManager 订阅管理器
type SubscriptionManager struct {
	paths     ConfigPaths
	subDir    string
	configDir string
	baseURL   string // 订阅基础URL
}

// NewSubscriptionManager 创建订阅管理器
func NewSubscriptionManager() *SubscriptionManager {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	subDir := filepath.Join(paths.ConfigDir, "subscriptions")
	configDir := filepath.Join(paths.ConfigDir, "subscription_configs")

	return &SubscriptionManager{
		paths:     paths,
		subDir:    subDir,
		configDir: configDir,
		baseURL:   "", // 将从系统配置中获取
	}
}

// GenerateSubscriptionFromConfig 从Sing-box配置生成订阅
func (sm *SubscriptionManager) GenerateSubscriptionFromConfig(configName string, format string) (*SubscriptionConfig, error) {
	logger.Info("[订阅管理] 从配置生成订阅", "config", configName, "format", format)

	// 读取Sing-box配置文件
	configPath := filepath.Join(sm.paths.ConfigDir, configName+".json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析配置
	var singboxConfig map[string]interface{}
	if err := json.Unmarshal(configData, &singboxConfig); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 提取节点信息
	nodes, err := sm.extractNodesFromConfig(singboxConfig)
	if err != nil {
		return nil, fmt.Errorf("提取节点信息失败: %w", err)
	}

	// 创建订阅配置
	subscription := &SubscriptionConfig{
		ID:             generateSubscriptionID(),
		Name:           configName,
		Nodes:          nodes,
		Format:         format,
		UpdateTime:     time.Now(),
		Enabled:        true,
		AutoUpdate:     false,
		UpdateInterval: 60,
		ShareCode:      generateShareCode(),
		UserCode:       generateUserCode(),
	}

	// 保存订阅配置
	if err := sm.saveSubscriptionConfig(subscription); err != nil {
		return nil, fmt.Errorf("保存订阅配置失败: %w", err)
	}

	logger.Info("[订阅管理] 订阅生成成功", "name", configName, "nodes", len(nodes))
	return subscription, nil
}

// GenerateSubscriptionURL 生成订阅URL
func (sm *SubscriptionManager) GenerateSubscriptionURL(subscription *SubscriptionConfig) string {
	if sm.baseURL == "" {
		sm.baseURL = "http://localhost:8080" // 默认值
	}

	// 生成短链接格式: base_url/s/share_code+user_code
	shortLink := fmt.Sprintf("%s/s/%s%s", sm.baseURL, subscription.ShareCode, subscription.UserCode)

	// 根据格式生成不同的URL
	switch subscription.Format {
	case "clash":
		return shortLink + "?target=clash"
	case "v2ray":
		return shortLink + "?target=v2ray"
	case "singbox":
		return shortLink + "?target=singbox"
	default:
		return shortLink
	}
}

// ExportSubscription 导出订阅内容
func (sm *SubscriptionManager) ExportSubscription(subscriptionID string, format string) (string, error) {
	// 加载订阅配置
	subscription, err := sm.loadSubscriptionConfig(subscriptionID)
	if err != nil {
		return "", fmt.Errorf("加载订阅配置失败: %w", err)
	}

	// 根据格式导出
	switch format {
	case "clash":
		return sm.exportClashFormat(subscription)
	case "v2ray":
		return sm.exportV2RayFormat(subscription)
	case "singbox":
		return sm.exportSingboxFormat(subscription)
	case "base64":
		return sm.exportBase64Format(subscription)
	case "json":
		return sm.exportJSONFormat(subscription)
	default:
		return "", fmt.Errorf("不支持的格式: %s", format)
	}
}

// GenerateNodeLink 生成节点链接
func (sm *SubscriptionManager) GenerateNodeLink(node NodeInfo) string {
	switch node.Type {
	case "vless":
		return sm.generateVLESSLink(node)
	case "vmess":
		return sm.generateVMessLink(node)
	case "hysteria2":
		return sm.generateHysteria2Link(node)
	case "tuic":
		return sm.generateTUICLink(node)
	case "trojan":
		return sm.generateTrojanLink(node)
	case "ss":
		return sm.generateShadowsocksLink(node)
	default:
		return ""
	}
}

// GenerateQRCodeData 生成二维码数据
func (sm *SubscriptionManager) GenerateQRCodeData(subscriptionID string) (string, error) {
	subscription, err := sm.loadSubscriptionConfig(subscriptionID)
	if err != nil {
		return "", fmt.Errorf("加载订阅配置失败: %w", err)
	}

	// 生成订阅URL
	subURL := sm.GenerateSubscriptionURL(subscription)

	// 简单的二维码数据（实际应该使用二维码生成库）
	return fmt.Sprintf("SUBSCRIPTION:%s", subURL), nil
}

// UpdateSubscription 更新订阅
func (sm *SubscriptionManager) UpdateSubscription(subscriptionID string) error {
	logger.Info("[订阅管理] 更新订阅", "subscription_id", subscriptionID)

	subscription, err := sm.loadSubscriptionConfig(subscriptionID)
	if err != nil {
		return fmt.Errorf("加载订阅配置失败: %w", err)
	}

	// 从原始配置重新提取节点
	configPath := filepath.Join(sm.paths.ConfigDir, subscription.Name+".json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var singboxConfig map[string]interface{}
	if err := json.Unmarshal(configData, &singboxConfig); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	nodes, err := sm.extractNodesFromConfig(singboxConfig)
	if err != nil {
		return fmt.Errorf("提取节点信息失败: %w", err)
	}

	subscription.Nodes = nodes
	subscription.UpdateTime = time.Now()

	// 保存更新后的配置
	if err := sm.saveSubscriptionConfig(subscription); err != nil {
		return fmt.Errorf("保存订阅配置失败: %w", err)
	}

	logger.Info("[订阅管理] 订阅更新成功", "subscription_id", subscriptionID)
	return nil
}

// ListSubscriptions 列出所有订阅
func (sm *SubscriptionManager) ListSubscriptions() ([]*SubscriptionConfig, error) {
	files, err := os.ReadDir(sm.configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SubscriptionConfig{}, nil
		}
		return nil, fmt.Errorf("读取配置目录失败: %w", err)
	}

	subscriptions := make([]*SubscriptionConfig, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		subID := strings.TrimSuffix(file.Name(), ".json")
		subscription, err := sm.loadSubscriptionConfig(subID)
		if err != nil {
			logger.Warn("[订阅管理] 加载订阅配置失败", "sub_id", subID, "error", err)
			continue
		}

		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

// DeleteSubscription 删除订阅
func (sm *SubscriptionManager) DeleteSubscription(subscriptionID string) error {
	logger.Info("[订阅管理] 删除订阅", "subscription_id", subscriptionID)

	configPath := filepath.Join(sm.configDir, subscriptionID+".json")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除订阅配置失败: %w", err)
	}

	logger.Info("[订阅管理] 订阅删除成功")
	return nil
}

// 内部方法

func (sm *SubscriptionManager) extractNodesFromConfig(config map[string]interface{}) ([]NodeInfo, error) {
	nodes := make([]NodeInfo, 0)

	// 从inbounds中提取节点信息
	if inbounds, ok := config["inbounds"].([]interface{}); ok {
		for _, inbound := range inbounds {
			if inboundMap, ok := inbound.(map[string]interface{}); ok {
				node := sm.parseInboundToNode(inboundMap)
				if node != nil {
					nodes = append(nodes, *node)
				}
			}
		}
	}

	// 从outbounds中提取节点信息（如果有代理配置）
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for _, outbound := range outbounds {
			if outboundMap, ok := outbound.(map[string]interface{}); ok {
				// 跳过非代理类型的outbound
				if outboundMap["type"] == nil {
					continue
				}

				outboundType := outboundMap["type"].(string)
				if outboundType == "direct" || outboundType == "block" || outboundType == "dns" {
					continue
				}

				node := sm.parseOutboundToNode(outboundMap)
				if node != nil {
					nodes = append(nodes, *node)
				}
			}
		}
	}

	return nodes, nil
}

func (sm *SubscriptionManager) parseInboundToNode(inbound map[string]interface{}) *NodeInfo {
	// 解析inbound配置为节点信息
	nodeType, _ := inbound["type"].(string)
	tag, _ := inbound["tag"].(string)

	if nodeType == "" || tag == "" {
		return nil
	}

	node := &NodeInfo{
		Name:      tag,
		Type:      nodeType,
		CreatedAt: time.Now(),
		Extra:     make(map[string]string),
	}

	// 根据不同协议类型解析配置
	switch nodeType {
	case "vless", "vmess":
		return sm.parseVLESSInbound(inbound, node)
	case "hysteria2":
		return sm.parseHysteria2Inbound(inbound, node)
	case "tuic":
		return sm.parseTUICInbound(inbound, node)
	case "trojan":
		return sm.parseTrojanInbound(inbound, node)
	default:
		return nil
	}
}

func (sm *SubscriptionManager) parseOutboundToNode(outbound map[string]interface{}) *NodeInfo {
	// 解析outbound配置为节点信息（用于客户端模式）
	nodeType, _ := outbound["type"].(string)
	tag, _ := outbound["tag"].(string)

	if nodeType == "" {
		return nil
	}

	name := tag
	if name == "" {
		name = nodeType + "-" + time.Now().Format("20060102150405")
	}

	node := &NodeInfo{
		Name:      name,
		Type:      nodeType,
		CreatedAt: time.Now(),
		Extra:     make(map[string]string),
	}

	// 根据不同协议类型解析配置
	switch nodeType {
	case "vless", "vmess":
		return sm.parseVLESSOutbound(outbound, node)
	case "hysteria2":
		return sm.parseHysteria2Outbound(outbound, node)
	case "tuic":
		return sm.parseTUICOutbound(outbound, node)
	default:
		return nil
	}
}

func (sm *SubscriptionManager) parseVLESSInbound(inbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	// 解析VLESS inbound配置
	if listen, ok := inbound["listen"].(string); ok {
		node.Host = listen
		if node.Host == "" || node.Host == "0.0.0.0" {
			node.Host = getLocalIP()
		}
	}

	if port, ok := inbound["listen_port"].(float64); ok {
		node.Port = int(port)
	}

	if users, ok := inbound["users"].([]interface{}); ok && len(users) > 0 {
		if userMap, ok := users[0].(map[string]interface{}); ok {
			if uuid, ok := userMap["uuid"].(string); ok {
				node.UUID = uuid
			}
			if flow, ok := userMap["flow"].(string); ok {
				node.Extra["flow"] = flow
			}
		}
	}

	if tls, ok := inbound["tls"].(map[string]interface{}); ok {
		if enabled, ok := tls["enabled"].(bool); ok && enabled {
			node.Security = "tls"
			if serverName, ok := tls["server_name"].(string); ok {
				node.SNI = serverName
			}
		}
	}

	if transport, ok := inbound["transport"].(map[string]interface{}); ok {
		if transportType, ok := transport["type"].(string); ok {
			node.Network = transportType
			if transportType == "ws" {
				if path, ok := transport["path"].(string); ok {
					node.Path = path
				}
				if headers, ok := transport["headers"].(map[string]interface{}); ok {
					if host, ok := headers["Host"].(string); ok {
						node.HostHeader = host
					}
				}
			}
		}
	}

	return node
}

func (sm *SubscriptionManager) parseVLESSOutbound(outbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	// 解析VLESS outbound配置（客户端模式）
	if server, ok := outbound["server"].(string); ok {
		node.Host = server
	}

	if port, ok := outbound["server_port"].(float64); ok {
		node.Port = int(port)
	}

	if uuid, ok := outbound["uuid"].(string); ok {
		node.UUID = uuid
	}

	if flow, ok := outbound["flow"].(string); ok {
		node.Extra["flow"] = flow
	}

	if tls, ok := outbound["tls"].(map[string]interface{}); ok {
		if enabled, ok := tls["enabled"].(bool); ok && enabled {
			node.Security = "tls"
			if serverName, ok := tls["server_name"].(string); ok {
				node.SNI = serverName
			}
		}
	}

	if transport, ok := outbound["transport"].(map[string]interface{}); ok {
		if transportType, ok := transport["type"].(string); ok {
			node.Network = transportType
		}
	}

	return node
}

func (sm *SubscriptionManager) parseHysteria2Inbound(inbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	if listen, ok := inbound["listen"].(string); ok {
		node.Host = listen
		if node.Host == "" || node.Host == "0.0.0.0" {
			node.Host = getLocalIP()
		}
	}

	if port, ok := inbound["listen_port"].(float64); ok {
		node.Port = int(port)
	}

	if password, ok := inbound["password"].(string); ok {
		node.Password = password
	}

	if up, ok := inbound["up"].(string); ok {
		node.Up = parseBandwidth(up)
	}

	if down, ok := inbound["down"].(string); ok {
		node.Down = parseBandwidth(down)
	}

	if obfs, ok := inbound["obfs"].(map[string]interface{}); ok {
		if enabled, ok := obfs["enabled"].(bool); ok && enabled {
			if password, ok := obfs["password"].(string); ok {
				node.Obfs = password
			}
		}
	}

	return node
}

func (sm *SubscriptionManager) parseHysteria2Outbound(outbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	if server, ok := outbound["server"].(string); ok {
		node.Host = server
	}

	if port, ok := outbound["server_port"].(float64); ok {
		node.Port = int(port)
	}

	if password, ok := outbound["password"].(string); ok {
		node.Password = password
	}

	if up, ok := outbound["up"].(string); ok {
		node.Up = parseBandwidth(up)
	}

	if down, ok := outbound["down"].(string); ok {
		node.Down = parseBandwidth(down)
	}

	return node
}

func (sm *SubscriptionManager) parseTUICInbound(inbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	if listen, ok := inbound["listen"].(string); ok {
		node.Host = listen
		if node.Host == "" || node.Host == "0.0.0.0" {
			node.Host = getLocalIP()
		}
	}

	if port, ok := inbound["listen_port"].(float64); ok {
		node.Port = int(port)
	}

	if users, ok := inbound["users"].([]interface{}); ok && len(users) > 0 {
		if userMap, ok := users[0].(map[string]interface{}); ok {
			if uuid, ok := userMap["uuid"].(string); ok {
				node.UUID = uuid
			}
			if password, ok := userMap["password"].(string); ok {
				node.Password = password
			}
		}
	}

	return node
}

func (sm *SubscriptionManager) parseTUICOutbound(outbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	if server, ok := outbound["server"].(string); ok {
		node.Host = server
	}

	if port, ok := outbound["server_port"].(float64); ok {
		node.Port = int(port)
	}

	if uuid, ok := outbound["uuid"].(string); ok {
		node.UUID = uuid
	}

	if password, ok := outbound["password"].(string); ok {
		node.Password = password
	}

	return node
}

func (sm *SubscriptionManager) parseTrojanInbound(inbound map[string]interface{}, node *NodeInfo) *NodeInfo {
	if listen, ok := inbound["listen"].(string); ok {
		node.Host = listen
		if node.Host == "" || node.Host == "0.0.0.0" {
			node.Host = getLocalIP()
		}
	}

	if port, ok := inbound["listen_port"].(float64); ok {
		node.Port = int(port)
	}

	if users, ok := inbound["users"].([]interface{}); ok && len(users) > 0 {
		if userMap, ok := users[0].(map[string]interface{}); ok {
			if password, ok := userMap["password"].(string); ok {
				node.Password = password
			}
		}
	}

	return node
}

// 导出格式

func (sm *SubscriptionManager) exportClashFormat(subscription *SubscriptionConfig) (string, error) {
	// 生成Clash格式的配置
	clashConfig := map[string]interface{}{
		"proxies": make([]map[string]interface{}, 0),
		"proxy-groups": []map[string]interface{}{
			{
				"name": "Proxy",
				"type": "select",
				"proxies": func() []string {
					names := make([]string, 0)
					for _, node := range subscription.Nodes {
						names = append(names, node.Name)
					}
					return names
				}(),
			},
		},
		"rules": []string{"MATCH,Proxy"},
	}

	proxies := clashConfig["proxies"].([]map[string]interface{})
	for _, node := range subscription.Nodes {
		proxy := sm.convertNodeToClashProxy(node)
		if proxy != nil {
			proxies = append(proxies, proxy)
		}
	}
	clashConfig["proxies"] = proxies

	configData, err := json.MarshalIndent(clashConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("生成Clash配置失败: %w", err)
	}

	return string(configData), nil
}

func (sm *SubscriptionManager) exportV2RayFormat(subscription *SubscriptionConfig) (string, error) {
	// 生成V2Ray格式的订阅链接（Base64编码）
	links := make([]string, 0)
	for _, node := range subscription.Nodes {
		link := sm.GenerateNodeLink(node)
		if link != "" {
			links = append(links, link)
		}
	}

	combinedLinks := strings.Join(links, "\n")
	encodedLinks := base64.StdEncoding.EncodeToString([]byte(combinedLinks))

	return encodedLinks, nil
}

func (sm *SubscriptionManager) exportSingboxFormat(subscription *SubscriptionConfig) (string, error) {
	// 生成Sing-box格式的配置
	singboxConfig := map[string]interface{}{
		"log": map[string]interface{}{
			"level": "info",
		},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				{"tag": "dns-remote", "address": "https://1.1.1.1/dns-query"},
				{"tag": "dns-local", "address": "https://223.5.5.5/dns-query", "detour": "dns-remote"},
			},
		},
		"inbounds": []map[string]interface{}{
			{
				"type":                "tun",
				"tag":                 "tun-in",
				"interface_name":      "tun0",
				"inet4_route_address": []string{"0.0.0.0/1", "128.0.0.0/1"},
			},
		},
		"outbounds": make([]map[string]interface{}, 0),
		"route": map[string]interface{}{
			"rules": []map[string]interface{}{
				{"protocol": "dns", "outbound": "dns-out"},
				{" Clash ": "all", "outbound": "proxy"},
			},
			"final": "proxy",
		},
	}

	outbounds := singboxConfig["outbounds"].([]map[string]interface{})
	for _, node := range subscription.Nodes {
		outbound := sm.convertNodeToSingboxOutbound(node)
		if outbound != nil {
			outbounds = append(outbounds, outbound)
		}
	}

	// 添加selector
	selectorNames := make([]string, 0)
	for _, node := range subscription.Nodes {
		selectorNames = append(selectorNames, node.Name)
	}

	outbounds = append(outbounds, map[string]interface{}{
		"type":      "selector",
		"tag":       "proxy",
		"outbounds": append([]string{"direct"}, selectorNames...),
		"default":   subscription.Nodes[0].Name,
	})

	singboxConfig["outbounds"] = outbounds

	configData, err := json.MarshalIndent(singboxConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("生成Sing-box配置失败: %w", err)
	}

	return string(configData), nil
}

func (sm *SubscriptionManager) exportBase64Format(subscription *SubscriptionConfig) (string, error) {
	// 导出Base64格式的节点链接
	links := make([]string, 0)
	for _, node := range subscription.Nodes {
		link := sm.GenerateNodeLink(node)
		if link != "" {
			links = append(links, link)
		}
	}

	combinedLinks := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(combinedLinks)), nil
}

func (sm *SubscriptionManager) exportJSONFormat(subscription *SubscriptionConfig) (string, error) {
	configData, err := json.MarshalIndent(subscription, "", "  ")
	if err != nil {
		return "", fmt.Errorf("导出JSON失败: %w", err)
	}

	return string(configData), nil
}

// 节点链接生成

func (sm *SubscriptionManager) generateVLESSLink(node NodeInfo) string {
	if node.UUID == "" {
		return ""
	}

	// VLESS链接格式: vless://uuid@host:port?params#name
	params := url.Values{}
	params.Set("type", node.Network)
	params.Set("security", node.Security)

	if node.SNI != "" {
		params.Set("sni", node.SNI)
	}
	if node.Path != "" {
		params.Set("path", node.Path)
	}
	if node.HostHeader != "" {
		params.Set("host", node.HostHeader)
	}
	if node.Fingerprint != "" {
		params.Set("fp", node.Fingerprint)
	}
	if node.PublicKey != "" {
		params.Set("pbk", node.PublicKey)
	}
	if node.ShortID != "" {
		params.Set("sid", node.ShortID)
	}
	if flow, ok := node.Extra["flow"]; ok {
		params.Set("flow", flow)
	}

	link := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		node.UUID, node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	return link
}

func (sm *SubscriptionManager) generateVMessLink(node NodeInfo) string {
	// VMess链接生成（简化版）
	return ""
}

func (sm *SubscriptionManager) generateHysteria2Link(node NodeInfo) string {
	if node.Password == "" {
		return ""
	}

	// Hysteria2链接格式: hysteria2://password@host:port?params#name
	params := url.Values{}

	if node.SNI != "" {
		params.Set("sni", node.SNI)
	}
	if node.Obfs != "" {
		params.Set("obfs", "salamander")
		params.Set("obfs-password", node.Obfs)
	}
	if node.Up > 0 {
		params.Set("up", fmt.Sprintf("%d mbps", node.Up))
	}
	if node.Down > 0 {
		params.Set("down", fmt.Sprintf("%d mbps", node.Down))
	}

	link := fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s",
		url.QueryEscape(node.Password), node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	return link
}

func (sm *SubscriptionManager) generateTUICLink(node NodeInfo) string {
	if node.UUID == "" {
		return ""
	}

	// TUIC链接格式: tuic://uuid:password@host:port?params#name
	params := url.Values{}
	params.Set("udp", "true")

	if node.SNI != "" {
		params.Set("sni", node.SNI)
	}

	link := fmt.Sprintf("tuic://%s:%s@%s:%d?%s#%s",
		node.UUID, node.Password, node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	return link
}

func (sm *SubscriptionManager) generateTrojanLink(node NodeInfo) string {
	if node.Password == "" {
		return ""
	}

	// Trojan链接格式: trojan://password@host:port?params#name
	params := url.Values{}
	params.Set("security", node.Security)

	if node.SNI != "" {
		params.Set("sni", node.SNI)
	}
	if node.Type == "ws" {
		params.Set("type", "ws")
		if node.Path != "" {
			params.Set("path", node.Path)
		}
		if node.HostHeader != "" {
			params.Set("host", node.HostHeader)
		}
	}

	link := fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		url.QueryEscape(node.Password), node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	return link
}

func (sm *SubscriptionManager) generateShadowsocksLink(node NodeInfo) string {
	// Shadowsocks链接生成
	return ""
}

// 转换方法

func (sm *SubscriptionManager) convertNodeToClashProxy(node NodeInfo) map[string]interface{} {
	proxy := map[string]interface{}{
		"name": node.Name,
	}

	switch node.Type {
	case "vless":
		proxy["type"] = "vless"
		proxy["server"] = node.Host
		proxy["port"] = node.Port
		proxy["uuid"] = node.UUID
		proxy["network"] = node.Network
		if node.Security == "tls" {
			proxy["tls"] = true
			proxy["servername"] = node.SNI
		}
		if node.Network == "ws" {
			proxy["ws-opts"] = map[string]interface{}{
				"path":    node.Path,
				"headers": map[string]string{"Host": node.HostHeader},
			}
		}

	case "hysteria2":
		proxy["type"] = "hysteria2"
		proxy["server"] = node.Host
		proxy["port"] = node.Port
		proxy["password"] = node.Password
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if node.Obfs != "" {
			proxy["obfs"] = "salamander"
			proxy["obfs-password"] = node.Obfs
		}

	case "tuic":
		proxy["type"] = "tuic"
		proxy["server"] = node.Host
		proxy["port"] = node.Port
		proxy["uuid"] = node.UUID
		proxy["password"] = node.Password
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}

	default:
		return nil
	}

	return proxy
}

func (sm *SubscriptionManager) convertNodeToSingboxOutbound(node NodeInfo) map[string]interface{} {
	outbound := map[string]interface{}{
		"type": node.Type,
		"tag":  node.Name,
	}

	switch node.Type {
	case "vless", "vmess":
		outbound["server"] = node.Host
		outbound["server_port"] = node.Port
		if node.UUID != "" {
			outbound["uuid"] = node.UUID
		}
		if node.Network != "" {
			outbound["network"] = node.Network
		}
		if node.Security == "tls" {
			outbound["tls"] = map[string]interface{}{
				"enabled":     true,
				"server_name": node.SNI,
			}
		}
		if node.Network == "ws" {
			outbound["transport"] = map[string]interface{}{
				"type":    "ws",
				"path":    node.Path,
				"headers": map[string]string{"Host": node.HostHeader},
			}
		}

	case "hysteria2":
		outbound["server"] = node.Host
		outbound["server_port"] = node.Port
		outbound["password"] = node.Password
		if node.SNI != "" {
			outbound["tls"] = map[string]interface{}{
				"enabled":     true,
				"server_name": node.SNI,
			}
		}
		if node.Obfs != "" {
			outbound["obfs"] = map[string]interface{}{
				"type":     "salamander",
				"password": node.Obfs,
			}
		}

	case "tuic":
		outbound["server"] = node.Host
		outbound["server_port"] = node.Port
		outbound["uuid"] = node.UUID
		outbound["password"] = node.Password
		if node.SNI != "" {
			outbound["tls"] = map[string]interface{}{
				"enabled":     true,
				"server_name": node.SNI,
			}
		}

	default:
		return nil
	}

	return outbound
}

// 保存和加载

func (sm *SubscriptionManager) saveSubscriptionConfig(subscription *SubscriptionConfig) error {
	if err := os.MkdirAll(sm.configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configPath := filepath.Join(sm.configDir, subscription.ID+".json")

	configData, err := json.MarshalIndent(subscription, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化订阅配置失败: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return fmt.Errorf("写入订阅配置失败: %w", err)
	}

	return nil
}

func (sm *SubscriptionManager) loadSubscriptionConfig(subscriptionID string) (*SubscriptionConfig, error) {
	configPath := filepath.Join(sm.configDir, subscriptionID+".json")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取订阅配置失败: %w", err)
	}

	var subscription SubscriptionConfig
	if err := json.Unmarshal(configData, &subscription); err != nil {
		return nil, fmt.Errorf("解析订阅配置失败: %w", err)
	}

	return &subscription, nil
}

// 工具函数

func generateSubscriptionID() string {
	return fmt.Sprintf("sub-%d", time.Now().UnixNano())
}

func generateShareCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 3)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func generateUserCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 3)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func parseBandwidth(bw string) int {
	// 解析带宽字符串，转换为Mbps
	// 示例: "100 mbps" -> 100
	bw = strings.ToLower(bw)
	bw = strings.ReplaceAll(bw, " mbps", "")
	bw = strings.ReplaceAll(bw, " mb", "")
	bw = strings.ReplaceAll(bw, " gbps", "")
	bw = strings.ReplaceAll(bw, " gb", "")

	var value int
	if _, err := fmt.Sscanf(bw, "%d", &value); err == nil {
		return value
	}

	return 0
}

// EncryptContent 加密订阅内容
func (sm *SubscriptionManager) EncryptContent(content string, key string) (string, error) {
	// 简单的AES加密
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", fmt.Errorf("创建加密器失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建GCM失败: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	ciphertext := gcm.Seal(nonce, nonce, []byte(content), nil)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptContent 解密订阅内容
func (sm *SubscriptionManager) DecryptContent(encryptedContent string, key string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedContent)
	if err != nil {
		return "", fmt.Errorf("Base64解码失败: %w", err)
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", fmt.Errorf("创建解密器失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建GCM失败: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("密文长度不足")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}

	return string(plaintext), nil
}
