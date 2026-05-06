package protocols

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"miaomiaowu/internal/singbox"
)

// VlessConfig Vless 协议配置
type VlessConfig struct {
	Server       string `json:"server"`
	ServerPort   int    `json:"server_port"`
	UUID         string `json:"uuid"`
	Flow         string `json:"flow"`
	Domain       string `json:"domain"`
	Path         string `json:"path"`
	Host         string `json:"host"`
	PublicKey    string `json:"public_key"`
	ShortID      string `json:"short_id"`
	Fingerprint  string `json:"fingerprint"`
	Reality      bool   `json:"reality"`
	WebSocket    bool   `json:"websocket"`
	TLS          bool   `json:"tls"`
}

// VlessBuilder Vless 配置构建器
type VlessBuilder struct {
	config *VlessConfig
}

// NewVlessBuilder 创建 Vless 构建器
func NewVlessBuilder() *VlessBuilder {
	return &VlessBuilder{
		config: &VlessConfig{
			ServerPort: 443,
			Path:       "/",
			Flow:       "xtls-rprx-vision",
			Fingerprint: "chrome",
			TLS:        true,
			WebSocket:  true,
		},
	}
}

// SetServer 设置服务器
func (b *VlessBuilder) SetServer(server string) *VlessBuilder {
	b.config.Server = server
	return b
}

// SetServerPort 设置服务器端口
func (b *VlessBuilder) SetServerPort(port int) *VlessBuilder {
	b.config.ServerPort = port
	return b
}

// SetUUID 设置 UUID
func (b *VlessBuilder) SetUUID(uuid string) *VlessBuilder {
	b.config.UUID = uuid
	return b
}

// GenerateUUID 生成随机 UUID
func (b *VlessBuilder) GenerateUUID() (*VlessBuilder, error) {
	uuid, err := generateUUID()
	if err != nil {
		return nil, err
	}
	b.config.UUID = uuid
	return b, nil
}

// SetFlow 设置流控
func (b *VlessBuilder) SetFlow(flow string) *VlessBuilder {
	b.config.Flow = flow
	return b
}

// SetDomain 设置域名
func (b *VlessBuilder) SetDomain(domain string) *VlessBuilder {
	b.config.Domain = domain
	return b
}

// SetPath 设置路径
func (b *VlessBuilder) SetPath(path string) *VlessBuilder {
	b.config.Path = path
	return b
}

// SetHost 设置主机头
func (b *VlessBuilder) SetHost(host string) *VlessBuilder {
	b.config.Host = host
	return b
}

// SetReality 设置 Reality
func (b *VlessBuilder) SetReality(publicKey, shortID string) *VlessBuilder {
	b.config.Reality = true
	b.config.PublicKey = publicKey
	b.config.ShortID = shortID
	return b
}

// EnableWebSocket 启用 WebSocket
func (b *VlessBuilder) EnableWebSocket(enabled bool) *VlessBuilder {
	b.config.WebSocket = enabled
	return b
}

// EnableTLS 启用 TLS
func (b *VlessBuilder) EnableTLS(enabled bool) *VlessBuilder {
	b.config.TLS = enabled
	return b
}

// Build 构建配置
func (b *VlessBuilder) Build() (*VlessConfig, error) {
	// 验证必要字段
	if b.config.Server == "" {
		return nil, fmt.Errorf("server is required")
	}

	if b.config.ServerPort <= 0 || b.config.ServerPort > 65535 {
		return nil, fmt.Errorf("invalid server port: %d", b.config.ServerPort)
	}

	if b.config.UUID == "" {
		return nil, fmt.Errorf("UUID is required")
	}

	if b.config.Reality && b.config.PublicKey == "" {
		return nil, fmt.Errorf("public key is required for Reality")
	}

	// 如果使用 WebSocket，确保有路径和主机
	if b.config.WebSocket && b.config.Path == "" {
		b.config.Path = "/"
	}

	return b.config, nil
}

// GenerateSingboxConfig 生成 Sing-box 配置
func (b *VlessBuilder) GenerateSingboxConfig() (*singbox.SingboxConfig, error) {
	vlessConfig, err := b.Build()
	if err != nil {
		return nil, err
	}

	generator := singbox.NewConfigGenerator()
	baseConfig, err := generator.GenerateDefaultConfig()
	if err != nil {
		return nil, err
	}

	// 构建 Sing-box 出站配置
	outbound := singbox.OutboundConfig{
		Type:       "vless",
		Tag:        "vless-out",
		Server:     vlessConfig.Server,
		ServerPort: vlessConfig.ServerPort,
		UUID:       vlessConfig.UUID,
	}

	// 设置 TLS
	tlsOptions := &singbox.TLSOptions{
		Enabled:    vlessConfig.TLS,
		ServerName: vlessConfig.Domain,
		Insecure:   false,
	}

	if vlessConfig.Reality {
		tlsOptions.Reality = &singbox.RealityOptions{
			Enabled:   true,
			PublicKey: vlessConfig.PublicKey,
			ShortID:   vlessConfig.ShortID,
		}
	}

	outbound.TLS = tlsOptions

	// 设置传输
	if vlessConfig.WebSocket {
		outbound.Transport = &singbox.TransportOptions{
			Type: "ws",
			Path: vlessConfig.Path,
			Headers: map[string]string{
				"Host": vlessConfig.Host,
			},
		}
	}

	// 添加到出站列表
	baseConfig.Outbounds = append(baseConfig.Outbounds, outbound)

	// 设置默认路由
	baseConfig.Route.Final = "vless-out"

	return baseConfig, nil
}

// GenerateLink 生成 Vless 链接
func (b *VlessBuilder) GenerateLink() (string, error) {
	config, err := b.Build()
	if err != nil {
		return "", err
	}

	// vless://uuid@server:port?params#remarks
	link := fmt.Sprintf("vless://%s@%s:%d", config.UUID, config.Server, config.ServerPort)

	params := []string{}

	if config.Flow != "" {
		params = append(params, "flow="+config.Flow)
	}

	if config.TLS {
		params = append(params, "security=tls")
		if config.Reality {
			params = append(params, "sni="+config.Domain)
			params = append(params, "fp="+config.Fingerprint)
			params = append(params, "pbk="+config.PublicKey)
			params = append(params, "sid="+config.ShortID)
			params = append(params, "type=tcp")
			params = append(params, "security=reality")
		} else if config.WebSocket {
			params = append(params, "sni="+config.Domain)
			params = append(params, "type=ws")
			params = append(params, "path="+config.Path)
			params = append(params, "host="+config.Host)
		}
	}

	if len(params) > 0 {
		link += "?" + params[0]
		for i := 1; i < len(params); i++ {
			link += "&" + params[i]
		}
	}

	link += "#Vless-" + config.Server

	return link, nil
}

// 工具函数

func generateUUID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	// 设置版本和变体
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// GenerateShortID 生成 Reality ShortID
func GenerateShortID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

// ParseVlessLink 解析 Vless 链接
func ParseVlessLink(link string) (*VlessConfig, error) {
	// 简化的 Vless 链接解析
	// 实际实现需要更完整的解析逻辑

	if len(link) < 8 || link[:8] != "vless://" {
		return nil, fmt.Errorf("invalid vless link format")
	}

	config := &VlessConfig{
		ServerPort: 443,
		Path:       "/",
		Flow:       "xtls-rprx-vision",
		Fingerprint: "chrome",
		TLS:        true,
		WebSocket:  true,
	}

	return config, nil
}