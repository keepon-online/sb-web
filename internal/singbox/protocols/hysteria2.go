package protocols

import (
	"fmt"
	"math/rand"
	"time"

	"miaomiaowu/internal/singbox"
)

// Hysteria2Config Hysteria2 协议配置
type Hysteria2Config struct {
	Server       string `json:"server"`
	ServerPort   int    `json:"server_port"`
	Password     string `json:"password"`
	Domain       string `json:"domain"`
	UpMbps       int    `json:"up_mbps"`
	DownMbps     int    `json:"down_mbps"`
	Obfs         string `json:"obfs,omitempty"`
	SNI          string `json:"sni,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
}

// Hysteria2Builder Hysteria2 配置构建器
type Hysteria2Builder struct {
	config *Hysteria2Config
}

// NewHysteria2Builder 创建 Hysteria2 构建器
func NewHysteria2Builder() *Hysteria2Builder {
	return &Hysteria2Builder{
		config: &Hysteria2Config{
			ServerPort: 443,
			UpMbps:     100,
			DownMbps:   100,
		},
	}
}

// SetServer 设置服务器
func (b *Hysteria2Builder) SetServer(server string) *Hysteria2Builder {
	b.config.Server = server
	return b
}

// SetServerPort 设置服务器端口
func (b *Hysteria2Builder) SetServerPort(port int) *Hysteria2Builder {
	b.config.ServerPort = port
	return b
}

// SetPassword 设置密码
func (b *Hysteria2Builder) SetPassword(password string) *Hysteria2Builder {
	b.config.Password = password
	return b
}

// GeneratePassword 生成随机密码
func (b *Hysteria2Builder) GeneratePassword(length int) *Hysteria2Builder {
	if length <= 0 {
		length = 16
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	b.config.Password = string(b)
	return b
}

// SetDomain 设置域名
func (b *Hysteria2Builder) SetDomain(domain string) *Hysteria2Builder {
	b.config.Domain = domain
	return b
}

// SetBandwidth 设置带宽
func (b *Hysteria2Builder) SetBandwidth(up, down int) *Hysteria2Builder {
	b.config.UpMbps = up
	b.config.DownMbps = down
	return b
}

// SetObfs 设置混淆
func (b *Hysteria2Builder) SetObfs(obfs string) *Hysteria2Builder {
	b.config.Obfs = obfs
	return b
}

// SetSNI 设置 SNI
func (b *Hysteria2Builder) SetSNI(sni string) *Hysteria2Builder {
	b.config.SNI = sni
	return b
}

// SetInsecure 设置不安全模式
func (b *Hysteria2Builder) SetInsecure(insecure bool) *Hysteria2Builder {
	b.config.Insecure = insecure
	return b
}

// Build 构建配置
func (b *Hysteria2Builder) Build() (*Hysteria2Config, error) {
	// 验证必要字段
	if b.config.Server == "" {
		return nil, fmt.Errorf("server is required")
	}

	if b.config.ServerPort <= 0 || b.config.ServerPort > 65535 {
		return nil, fmt.Errorf("invalid server port: %d", b.config.ServerPort)
	}

	if b.config.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	return b.config, nil
}

// GenerateSingboxConfig 生成 Sing-box 配置
func (b *Hysteria2Builder) GenerateSingboxConfig() (*singbox.SingboxConfig, error) {
	hy2Config, err := b.Build()
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
		Type:       "hysteria2",
		Tag:        "hysteria2-out",
		Server:     hy2Config.Server,
		ServerPort: hy2Config.ServerPort,
		Password:   hy2Config.Password,
		TLS: &singbox.TLSOptions{
			Enabled:    true,
			ServerName: hy2Config.Domain,
			Insecure:   hy2Config.Insecure,
		},
		Transport: &singbox.TransportOptions{
			Type:         "hysteria2",
			MaxUploadMbps:   hy2Config.UpMbps,
			MaxDownloadMbps: hy2Config.DownMbps,
		},
	}

	// 添加混淆
	if hy2Config.Obfs != "" {
		if outbound.Transport.Options == nil {
			outbound.Transport.Options = make(map[string]interface{})
		}
		outbound.Transport.Options["obfs"] = hy2Config.Obfs
	}

	// 添加到出站列表
	baseConfig.Outbounds = append(baseConfig.Outbounds, outbound)

	// 设置默认路由
	baseConfig.Route.Final = "hysteria2-out"

	return baseConfig, nil
}

// GenerateLink 生成 Hysteria2 链接
func (b *Hysteria2Builder) GenerateLink() (string, error) {
	config, err := b.Build()
	if err != nil {
		return "", err
	}

	// hysteria2://password@server:port?params#remarks
	link := fmt.Sprintf("hysteria2://%s@%s:%d", config.Password, config.Server, config.ServerPort)

	params := []string{}

	if config.SNI != "" {
		params = append(params, "sni="+config.SNI)
	}

	if config.Insecure {
		params = append(params, "insecure=1")
	}

	if config.Obfs != "" {
		params = append(params, "obfs="+config.Obfs)
	}

	if config.UpMbps > 0 {
		params = append(params, fmt.Sprintf("up=%dmbps", config.UpMbps))
	}

	if config.DownMbps > 0 {
		params = append(params, fmt.Sprintf("down=%dmbps", config.DownMbps))
	}

	if len(params) > 0 {
		link += "?" + params[0]
		for i := 1; i < len(params); i++ {
			link += "&" + params[i]
		}
	}

	link += "#Hysteria2-" + config.Server

	return link, nil
}

// ParseHysteria2Link 解析 Hysteria2 链接
func ParseHysteria2Link(link string) (*Hysteria2Config, error) {
	// 简化的 Hysteria2 链接解析
	if len(link) < 12 || link[:12] != "hysteria2://" {
		return nil, fmt.Errorf("invalid hysteria2 link format")
	}

	config := &Hysteria2Config{
		ServerPort: 443,
		UpMbps:     100,
		DownMbps:   100,
	}

	// 这里需要实现完整的链接解析逻辑
	// 暂时返回基础配置

	return config, nil
}

// GenerateObfsPassword 生成混淆密码
func GenerateObfsPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)

	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	return string(b)
}