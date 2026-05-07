package singbox

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"miaomiaowu/internal/logger"
)

// ProtocolType 协议类型
type ProtocolType string

const (
	ProtocolVless     ProtocolType = "vless"
	ProtocolVmess     ProtocolType = "vmess"
	ProtocolHysteria2 ProtocolType = "hysteria2"
	ProtocolTuic      ProtocolType = "tuic"
	ProtocolAnytls    ProtocolType = "anytls"
)

// SingboxConfig Sing-box 配置结构
type SingboxConfig struct {
	Log          LogConfig          `json:"log"`
	DNS          DNSConfig          `json:"dns"`
	Inbounds     []InboundConfig    `json:"inbounds"`
	Outbounds    []OutboundConfig   `json:"outbounds"`
	Route        RouteConfig        `json:"route"`
	Experimental ExperimentalConfig `json:"experimental,omitempty"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp,omitempty"`
}

// DNSConfig DNS 配置
type DNSConfig struct {
	Servers []DNSServer `json:"servers"`
	Rules   []DNSRule   `json:"rules,omitempty"`
}

// DNSServer DNS 服务器
type DNSServer struct {
	Tag             string `json:"tag"`
	Address         string `json:"address"`
	AddressResolver string `json:"address_resolver,omitempty"`
	Strategy        string `json:"strategy,omitempty"`
	Detour          string `json:"detour,omitempty"`
}

// DNSRule DNS 规则
type DNSRule struct {
	Inbound  []string `json:"inbound,omitempty"`
	Outbound string   `json:"outbound,omitempty"`
	Server   string   `json:"server"`
	Disabled bool     `json:"disabled,omitempty"`
}

// InboundConfig 入站配置
type InboundConfig struct {
	Type                     string                   `json:"type"`
	Tag                      string                   `json:"tag"`
	Listen                   string                   `json:"listen"`
	ListenPort               int                      `json:"listen_port"`
	Users                    []map[string]interface{} `json:"users,omitempty"`
	Password                 string                   `json:"password,omitempty"`
	CongestionControl        string                   `json:"congestion_control,omitempty"`
	Masquerade               string                   `json:"masquerade,omitempty"`
	Sniff                    bool                     `json:"sniff,omitempty"`
	SniffOverrideDestination bool                     `json:"sniff_override_destination,omitempty"`
	TLS                      *TLSOptions              `json:"tls,omitempty"`
	Transport                *TransportOptions        `json:"transport,omitempty"`
	Options                  map[string]interface{}   `json:"options,omitempty"`
}

// OutboundConfig 出站配置
type OutboundConfig struct {
	Type       string                 `json:"type"`
	Tag        string                 `json:"tag"`
	Server     string                 `json:"server,omitempty"`
	ServerPort int                    `json:"server_port,omitempty"`
	Username   string                 `json:"username,omitempty"`
	Password   string                 `json:"password,omitempty"`
	UUID       string                 `json:"uuid,omitempty"`
	TLS        *TLSOptions            `json:"tls,omitempty"`
	Transport  *TransportOptions      `json:"transport,omitempty"`
	Detour     string                 `json:"detour,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// TLSOptions TLS 选项
type TLSOptions struct {
	Enabled         bool            `json:"enabled"`
	ServerName      string          `json:"server_name,omitempty"`
	Insecure        bool            `json:"insecure,omitempty"`
	ALPN            []string        `json:"alpn,omitempty"`
	CertificatePath string          `json:"certificate_path,omitempty"`
	KeyPath         string          `json:"key_path,omitempty"`
	Reality         *RealityOptions `json:"reality,omitempty"`
}

// RealityOptions Reality 选项
type RealityOptions struct {
	Enabled    bool   `json:"enabled"`
	PublicKey  string `json:"public_key,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	ShortID    string `json:"short_id,omitempty"`
}

// TransportOptions 传输选项
type TransportOptions struct {
	Type            string            `json:"type"`
	Path            string            `json:"path,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	MaxUploadMbps   int               `json:"max_upload_mbps,omitempty"`
	MaxDownloadMbps int               `json:"max_download_mbps,omitempty"`
}

// RouteConfig 路由配置
type RouteConfig struct {
	Rules    []RouteRule `json:"rules,omitempty"`
	Rule_set []string    `json:"rule_set,omitempty"`
	Final    string      `json:"final"`
	Auto     bool        `json:"auto,omitempty"`
}

// RouteRule 路由规则
type RouteRule struct {
	Inbound  []string `json:"inbound,omitempty"`
	Outbound string   `json:"outbound,omitempty"`
}

// ExperimentalConfig 实验性配置
type ExperimentalConfig struct {
	CacheFile *CacheFileConfig `json:"cache_file,omitempty"`
}

// CacheFileConfig 缓存文件配置
type CacheFileConfig struct {
	Enabled     bool   `json:"enabled"`
	Path        string `json:"path,omitempty"`
	StoreFakeIP bool   `json:"store_fakeip,omitempty"`
}

// ConfigGenerator 配置生成器
type ConfigGenerator struct {
	paths ConfigPaths
}

// NewConfigGenerator 创建配置生成器
func NewConfigGenerator() *ConfigGenerator {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	return &ConfigGenerator{
		paths: paths,
	}
}

// GenerateDefaultConfig 生成默认配置
func (cg *ConfigGenerator) GenerateDefaultConfig() (*SingboxConfig, error) {
	config := &SingboxConfig{
		Log: LogConfig{
			Level:     "info",
			Timestamp: true,
		},
		DNS: DNSConfig{
			Servers: []DNSServer{
				{
					Tag:      "local",
					Address:  "https://1.1.1.1/dns-query",
					Strategy: "prefer_ipv4",
				},
				{
					Tag:     "remote",
					Address: "https://8.8.8.8/dns-query",
					Detour:  "direct",
				},
			},
		},
		Inbounds: []InboundConfig{
			cg.generateTunInbound(),
			cg.generateMixedInbound(),
		},
		Outbounds: []OutboundConfig{
			cg.generateDirectOutbound(),
			cg.generateBlockOutbound(),
			cg.generateDNSOutbound(),
		},
		Route: RouteConfig{
			Rules: []RouteRule{
				{
					Inbound:  []string{"tun-in"},
					Outbound: "dns-out",
				},
			},
			Final: "direct",
			Auto:  true,
		},
		Experimental: ExperimentalConfig{
			CacheFile: &CacheFileConfig{
				Enabled:     true,
				Path:        filepath.Join(cg.paths.DataDir, "cache.db"),
				StoreFakeIP: true,
			},
		},
	}

	return config, nil
}

// generateTunInbound 生成 TUN 入站配置
func (cg *ConfigGenerator) generateTunInbound() InboundConfig {
	inbound := InboundConfig{
		Type:                     "tun",
		Tag:                      "tun-in",
		Sniff:                    true,
		SniffOverrideDestination: true,
	}

	// 设置选项
	inbound.Options = map[string]interface{}{
		"stack":        "system",
		"auto_route":   true,
		"strict_route": false,
	}

	return inbound
}

// generateMixedInbound 生成混合入站配置
func (cg *ConfigGenerator) generateMixedInbound() InboundConfig {
	return InboundConfig{
		Type:                     "mixed",
		Tag:                      "mixed-in",
		Listen:                   "0.0.0.0",
		ListenPort:               2080,
		Sniff:                    true,
		SniffOverrideDestination: true,
	}
}

// generateDirectOutbound 生成直连出站
func (cg *ConfigGenerator) generateDirectOutbound() OutboundConfig {
	return OutboundConfig{
		Type: "direct",
		Tag:  "direct",
	}
}

// generateBlockOutbound 生成阻断出站
func (cg *ConfigGenerator) generateBlockOutbound() OutboundConfig {
	return OutboundConfig{
		Type: "block",
		Tag:  "block",
	}
}

// generateDNSOutbound 生成 DNS 出站
func (cg *ConfigGenerator) generateDNSOutbound() OutboundConfig {
	return OutboundConfig{
		Type: "dns",
		Tag:  "dns-out",
	}
}

// GenerateProtocolConfig 生成协议配置
func (cg *ConfigGenerator) GenerateProtocolConfig(protocol ProtocolType, options ProtocolOptions) (*SingboxConfig, error) {
	// 生成基础配置
	config, err := cg.GenerateDefaultConfig()
	if err != nil {
		return nil, err
	}

	// 根据协议类型添加出站配置
	outbound, err := cg.generateProtocolOutbound(protocol, options)
	if err != nil {
		return nil, fmt.Errorf("generate protocol outbound: %w", err)
	}

	// 添加到出站列表
	config.Outbounds = append(config.Outbounds, *outbound)

	// 设置默认路由
	config.Route.Final = outbound.Tag

	return config, nil
}

// ProtocolOptions 协议选项
type ProtocolOptions struct {
	Server     string
	ServerPort int
	UUID       string
	Password   string
	Domain     string
	Path       string
	Host       string
	Port       int
	CertPath   string
	KeyPath    string
	PublicKey  string
	ShortID    string
	UpMbps     int
	DownMbps   int
}

// generateProtocolOutbound 生成协议出站配置
func (cg *ConfigGenerator) generateProtocolOutbound(protocol ProtocolType, opts ProtocolOptions) (*OutboundConfig, error) {
	switch protocol {
	case ProtocolVless:
		return cg.generateVlessOutbound(opts)
	case ProtocolVmess:
		return cg.generateVmessOutbound(opts)
	case ProtocolHysteria2:
		return cg.generateHysteria2Outbound(opts)
	case ProtocolTuic:
		return cg.generateTuicOutbound(opts)
	case ProtocolAnytls:
		return cg.generateAnytlsOutbound(opts)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

// generateVlessOutbound 生成 Vless 出站
func (cg *ConfigGenerator) generateVlessOutbound(opts ProtocolOptions) (*OutboundConfig, error) {
	if opts.UUID == "" {
		uuid, err := generateUUID()
		if err != nil {
			return nil, err
		}
		opts.UUID = uuid
	}

	outbound := &OutboundConfig{
		Type:       "vless",
		Tag:        "vless-out",
		Server:     opts.Server,
		ServerPort: opts.ServerPort,
		UUID:       opts.UUID,
		TLS: &TLSOptions{
			Enabled:    true,
			ServerName: opts.Domain,
			Insecure:   false,
		},
		Transport: &TransportOptions{
			Type: "ws",
			Path: opts.Path,
			Headers: map[string]string{
				"Host": opts.Host,
			},
		},
	}

	// 如果有 Reality 公钥
	if opts.PublicKey != "" {
		outbound.TLS.Reality = &RealityOptions{
			Enabled:   true,
			PublicKey: opts.PublicKey,
			ShortID:   opts.ShortID,
		}
		outbound.TLS.ServerName = opts.Domain
	}

	return outbound, nil
}

// generateVmessOutbound 生成 Vmess 出站
func (cg *ConfigGenerator) generateVmessOutbound(opts ProtocolOptions) (*OutboundConfig, error) {
	if opts.UUID == "" {
		uuid, err := generateUUID()
		if err != nil {
			return nil, err
		}
		opts.UUID = uuid
	}

	return &OutboundConfig{
		Type:       "vmess",
		Tag:        "vmess-out",
		Server:     opts.Server,
		ServerPort: opts.ServerPort,
		UUID:       opts.UUID,
		Password:   opts.Password,
		TLS: &TLSOptions{
			Enabled:    true,
			ServerName: opts.Domain,
			Insecure:   false,
		},
		Transport: &TransportOptions{
			Type: "ws",
			Path: opts.Path,
			Headers: map[string]string{
				"Host": opts.Host,
			},
		},
	}, nil
}

// generateHysteria2Outbound 生成 Hysteria2 出站
func (cg *ConfigGenerator) generateHysteria2Outbound(opts ProtocolOptions) (*OutboundConfig, error) {
	if opts.Password == "" {
		password, err := generatePassword(16)
		if err != nil {
			return nil, err
		}
		opts.Password = password
	}

	return &OutboundConfig{
		Type:       "hysteria2",
		Tag:        "hysteria2-out",
		Server:     opts.Server,
		ServerPort: opts.ServerPort,
		Password:   opts.Password,
		TLS: &TLSOptions{
			Enabled:    true,
			ServerName: opts.Domain,
			Insecure:   false,
		},
		Transport: &TransportOptions{
			Type:            "hysteria2",
			MaxUploadMbps:   opts.UpMbps,
			MaxDownloadMbps: opts.DownMbps,
		},
	}, nil
}

// generateTuicOutbound 生成 Tuic 出站
func (cg *ConfigGenerator) generateTuicOutbound(opts ProtocolOptions) (*OutboundConfig, error) {
	if opts.UUID == "" {
		uuid, err := generateUUID()
		if err != nil {
			return nil, err
		}
		opts.UUID = uuid
	}

	if opts.Password == "" {
		password, err := generatePassword(16)
		if err != nil {
			return nil, err
		}
		opts.Password = password
	}

	return &OutboundConfig{
		Type:       "tuic",
		Tag:        "tuic-out",
		Server:     opts.Server,
		ServerPort: opts.ServerPort,
		UUID:       opts.UUID,
		Password:   opts.Password,
		TLS: &TLSOptions{
			Enabled:    true,
			ServerName: opts.Domain,
			ALPN:       []string{"h3"},
		},
	}, nil
}

// generateAnytlsOutbound 生成 Anytls 出站
func (cg *ConfigGenerator) generateAnytlsOutbound(opts ProtocolOptions) (*OutboundConfig, error) {
	if opts.Password == "" {
		password, err := generatePassword(16)
		if err != nil {
			return nil, err
		}
		opts.Password = password
	}

	return &OutboundConfig{
		Type:       "shadowsocks",
		Tag:        "anytls-out",
		Server:     opts.Server,
		ServerPort: opts.ServerPort,
		Password:   opts.Password,
	}, nil
}

// SaveConfig 保存配置到文件
func (cg *ConfigGenerator) SaveConfig(config *SingboxConfig, filename string) error {
	configPath := filepath.Join(cg.paths.ConfigDir, filename)

	// 确保配置目录存在
	if err := os.MkdirAll(cg.paths.ConfigDir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// 序列化配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	logger.Info("[配置生成器] 配置已保存", "path", configPath)
	return nil
}

// LoadConfig 从文件加载配置
func (cg *ConfigGenerator) LoadConfig(filename string) (*SingboxConfig, error) {
	configPath := filepath.Join(cg.paths.ConfigDir, filename)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config SingboxConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &config, nil
}

// ValidateConfig 验证配置
func (cg *ConfigGenerator) ValidateConfig(config *SingboxConfig) error {
	// 基本验证
	if len(config.Inbounds) == 0 {
		return fmt.Errorf("至少需要一个入站配置")
	}

	if len(config.Outbounds) == 0 {
		return fmt.Errorf("至少需要一个出站配置")
	}

	// 验证端口
	for _, inbound := range config.Inbounds {
		if inbound.ListenPort <= 0 || inbound.ListenPort > 65535 {
			return fmt.Errorf("无效的入站端口: %d", inbound.ListenPort)
		}
	}

	// 验证路由规则
	if config.Route.Final == "" {
		return fmt.Errorf("缺少默认路由")
	}

	return nil
}

// 工具函数

// generateUUID 生成 UUID
func generateUUID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return uuid, nil
}

// generatePassword 生成随机密码
func generatePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}

	return string(b), nil
}

// GenerateRandomPort 生成随机端口
func GenerateRandomPort(minPort, maxPort int) (int, error) {
	if minPort < 10000 {
		minPort = 10000
	}
	if maxPort > 65535 {
		maxPort = 65535
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxPort-minPort+1)))
	if err != nil {
		return 0, err
	}

	return int(n.Int64()) + minPort, nil
}

// CheckPortAvailable 检查端口是否可用
func CheckPortAvailable(port int) bool {
	// 这里可以实现实际的端口检查逻辑
	// 暂时返回 true
	return true
}
