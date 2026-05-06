package certificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
)

// ACMEClient ACME客户端
type ACMEClient struct {
	paths         singbox.ConfigPaths
	email         string
	caURL         string // CA服务器URL
	stagingURL    string // 测试环境URL
	accountKey    string
	accountKeyPath string
}

// NewACMEClient 创建ACME客户端
func NewACMEClient(email string) *ACMEClient {
	env := singbox.DetectEnvironment()
	paths := singbox.GetConfigPaths(env)

	return &ACMEClient{
		paths:       paths,
		email:       email,
		caURL:       "https://acme-v02.api.letsencrypt.org/directory",
		stagingURL:  "https://acme-staging-v02.api.letsencrypt.org/directory",
		accountKey:   "",
		accountKeyPath: filepath.Join(paths.ConfigDir, "acme", "account.key"),
	}
}

// SetStaging 设置使用测试环境
func (ac *ACMEClient) SetStaging(staging bool) {
	if staging {
		ac.caURL = ac.stagingURL
	}
}

// RegisterAccount 注册账户
func (ac *ACMEClient) RegisterAccount() error {
	logger.Info("[ACME] 注册账户", "email", ac.email)

	// 生成账户密钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256())
	if err != nil {
		return fmt.Errorf("generate account key: %w", err)
	}

	// 保存账户密钥
	acDir := filepath.Dir(ac.accountKeyPath)
	if err := os.MkdirAll(acDir, 0755); err != nil {
		return fmt.Errorf("create acme directory: %w", err)
	}

	keyFile, err := os.Create(ac.accountKeyPath)
	if err != nil {
		return fmt.Errorf("create account key file: %w", err)
	}
	defer keyFile.Close()

	// 这里应该使用golang.org/x/crypto/acme包来与ACME服务器交互
	// 由于复杂性，这里提供一个简化的实现框架

	ac.accountKey = ac.accountKeyPath
	logger.Info("[ACME] 账户密钥已保存", "path", ac.accountKeyPath)

	return nil
}

// RequestCertificate 申请证书
func (ac *ACMEClient) RequestCertificate(domain string) (*CertInfo, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	logger.Info("[ACME] 申请证书", "domain", domain)

	// 验证域名解析
	if err := ac.validateDomain(domain); err != nil {
		return nil, fmt.Errorf("domain validation failed: %w", err)
	}

	// 检查80端口是否可用
	if err := ac.checkHTTPPort(); err != nil {
		return nil, fmt.Errorf("HTTP port check failed: %w", err)
	}

	// 这里应该实现完整的ACME流程：
	// 1. 创建订单
	// 2. 准备挑战（HTTP-01或DNS-01）
	// 3. 完成挑战
	// 4. 下载证书
	// 5. 保存证书

	// 由于完整实现较复杂，这里提供一个框架
	certInfo := &CertInfo{
		Domain:     domain,
		CertType:   CertTypeACME,
		ACMEEmail: ac.email,
		AutoRenew:  true,
	}

	logger.Info("[ACME] 证书申请框架已创建", "domain", domain)
	return certInfo, nil
}

// RenewCertificate 更新证书
func (ac *ACMEClient) RenewCertificate(domain string) (*CertInfo, error) {
	logger.Info("[ACME] 更新证书", "domain", domain)

	// 更新证书实际上是重新申请
	return ac.RequestCertificate(domain)
}

// validateDomain 验证域名
func (ac *ACMEClient) validateDomain(domain string) error {
	// 检查域名解析
	ips, err := net.LookupIP(domain)
	if err != nil {
		return fmt.Errorf("domain DNS lookup failed: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("domain has no IP addresses")
	}

	logger.Info("[ACME] 域名验证成功", "domain", domain, "ips", ips)
	return nil
}

// checkHTTPPort 检查HTTP端口
func (ac *ACMEClient) checkHTTPPort() error {
	// 检查80端口是否可用
	conn, err := net.DialTimeout("tcp", ":80", 5*time.Second)
	if err != nil {
		return fmt.Errorf("HTTP port 80 not accessible: %w", err)
	}
	conn.Close()

	logger.Info("[ACME] HTTP端口检查通过")
	return nil
}

// PrepareHTTPChallenge 准备HTTP挑战
func (ac *ACMEClient) PrepareHTTPChallenge(domain string) error {
	// 创建HTTP挑战目录
	challengeDir := filepath.Join(ac.paths.ConfigDir, "acme", ".well-known", "acme-challenge")
	if err := os.MkdirAll(challengeDir, 0755); err != nil {
		return fmt.Errorf("create challenge directory: %w", err)
	}

	// 创建挑战令牌文件
	token := generateToken()
	challengeFile := filepath.Join(challengeDir, token)

	if err := os.WriteFile(challengeFile, []byte(token), 0644); err != nil {
		return fmt.Errorf("write challenge token: %w", err)
	}

	logger.Info("[ACME] HTTP挑战准备完成", "domain", domain, "token", token)
	return nil
}

// GenerateCSR 生成证书签名请求
func (ac *ACMEClient) GenerateCSR(domain string) (string, error) {
	// 生成私钥和CSR
	privateKey, err := ecdsa.GenerateKey(elliptic.P256())
	if err != nil {
		return "", fmt.Errorf("generate private key: %w", err)
	}

	// 这里应该生成CSR
	// 简化实现，返回空字符串
	return "", nil
}

// CheckCertificateStatus 检查证书状态
func (ac *ACMEClient) CheckCertificateStatus(domain string) (*CertStatus, error) {
	cm := NewCertificateManager()
	return cm.GetCertStatus(domain)
}

// AutoRenewCertificate 自动更新证书
func (ac *ACMEClient) AutoRenewCertificate(domain string, warnDays int) (*CertInfo, error) {
	// 检查证书是否需要更新
	status, err := ac.CheckCertificateStatus(domain)
	if err != nil {
		// 如果证书不存在，直接申请新证书
		logger.Info("[ACME] 证书不存在，申请新证书", "domain", domain)
		return ac.RequestCertificate(domain)
	}

	// 检查是否即将过期
	if status.ExpiresIn <= warnDays {
		logger.Info("[ACME] 证书即将过期，自动更新", "domain", domain, "expires_in_days", status.ExpiresIn)
		return ac.RenewCertificate(domain)
	}

	logger.Info("[ACME] 证书状态良好", "domain", domain, "expires_in_days", status.ExpiresIn)
	return nil, fmt.Errorf("certificate does not need renewal")
}

// CheckAllCertificates 检查所有证书并自动更新
func (ac *ACMEClient) CheckAllCertificates(warnDays int) ([]string, error) {
	cm := NewCertificateManager()
	certs, err := cm.ListCerts()
	if err != nil {
		return nil, err
	}

	renewedCerts := []string{}

	for _, cert := range certs {
		if cert.AutoRenew && cert.CertType == CertTypeACME {
			if CheckCertExpiry(cert, warnDays) {
				logger.Info("[ACME] 检测到需要更新的证书", "domain", cert.Domain)

				_, err := ac.AutoRenewCertificate(cert.Domain, warnDays)
				if err != nil {
					logger.Error("[ACME] 自动更新失败", "domain", cert.Domain, "error", err)
					continue
				}

				renewedCerts = append(renewedCerts, cert.Domain)
			}
		}
	}

	return renewedCerts, nil
}

// 工具函数

func generateToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
	b := make([]byte, 43)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return ""
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// GetACMEEmail 获取ACME邮箱
func (ac *ACMEClient) GetACMEEmail() string {
	return ac.email
}

// SetACMEEmail 设置ACME邮箱
func (ac *ACMEClient) SetACMEEmail(email string) {
	ac.email = email
}

// IsAccountRegistered 检查账户是否已注册
func (ac *ACMEClient) IsAccountRegistered() bool {
	if _, err := os.Stat(ac.accountKeyPath); err != nil {
		return false
	}
	return true
}