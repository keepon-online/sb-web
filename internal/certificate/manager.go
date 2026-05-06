package certificate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
)

// CertType 证书类型
type CertType string

const (
	CertTypeSelfSigned CertType = "selfsigned"
	CertTypeACME       CertType = "acme"
	CertTypeCustom     CertType = "custom"
)

// CertificateManager 证书管理器
type CertificateManager struct {
	paths singbox.ConfigPaths
}

// NewCertificateManager 创建证书管理器
func NewCertificateManager() *CertificateManager {
	env := singbox.DetectEnvironment()
	paths := singbox.GetConfigPaths(env)

	return &CertificateManager{
		paths: paths,
	}
}

// GenerateSelfSignedCert 生成自签名证书
func (cm *CertificateManager) GenerateSelfSignedCert(domain string, validityDays int) (*CertInfo, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	if validityDays <= 0 {
		validityDays = 365 // 默认1年
	}

	logger.Info("[证书管理] 生成自签名证书", "domain", domain, "validity", validityDays)

	// 生成RSA私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"Sing-box"},
			Country:      []string{"US"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 0, validityDays),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		DNSNames: []string{domain},
	}

	// 自签名证书
	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// 保存证书和私钥
	certDir := filepath.Join(cm.paths.ConfigDir, "certs")
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, fmt.Errorf("create cert directory: %w", err)
	}

	certFile := filepath.Join(certDir, domain+".crt")
	keyFile := filepath.Join(certDir, domain+".key")

	// 写入证书文件
	certOut, err := os.Create(certFile)
	if err != nil {
		return nil, fmt.Errorf("create cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certRaw}); err != nil {
		return nil, fmt.Errorf("encode certificate: %w", err)
	}

	// 写入私钥文件
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return nil, fmt.Errorf("create key file: %w", err)
	}
	defer keyOut.Close()

	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return nil, fmt.Errorf("encode private key: %w", err)
	}

	// 设置文件权限
	if err := os.Chmod(keyFile, 0600); err != nil {
		return nil, fmt.Errorf("chmod key file: %w", err)
	}

	certInfo := &CertInfo{
		Domain:       domain,
		CertType:    CertTypeSelfSigned,
		CertPath:    certFile,
		KeyPath:     keyFile,
		ExpiresAt:   time.Now().AddDate(0, 0, validityDays),
		AutoRenew:   false,
		Fingerprint: generateFingerprint(certRaw),
	}

	logger.Info("[证书管理] 自签名证书生成成功", "domain", domain, "cert_file", certFile)
	return certInfo, nil
}

// LoadCert 加载证书
func (cm *CertificateManager) LoadCert(domain string) (*CertInfo, error) {
	certDir := filepath.Join(cm.paths.ConfigDir, "certs")
	certFile := filepath.Join(certDir, domain+".crt")
	keyFile := filepath.Join(certDir, domain+".key")

	// 检查证书文件是否存在
	if _, err := os.Stat(certFile); err != nil {
		return nil, fmt.Errorf("certificate file not found: %w", err)
	}

	if _, err := os.Stat(keyFile); err != nil {
		return nil, fmt.Errorf("private key file not found: %w", err)
	}

	// 读取证书文件
	certData, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}

	// 解析证书
	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	certInfo := &CertInfo{
		Domain:       domain,
		CertType:    CertTypeCustom, // 或从证书内容推断
		CertPath:    certFile,
		KeyPath:     keyFile,
		ExpiresAt:   cert.NotAfter,
		Issuer:      cert.Issuer.CommonName,
		Subject:     cert.Subject.CommonName,
		Fingerprint: generateFingerprint(cert.Raw),
	}

	return certInfo, nil
}

// ValidateCert 验证证书
func (cm *CertificateManager) ValidateCert(certPath string) (bool, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return false, fmt.Errorf("read certificate: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return false, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("parse certificate: %w", err)
	}

	// 检查证书是否过期
	if time.Now().After(cert.NotAfter) {
		return false, nil
	}

	// 检查证书是否有效
	if time.Now().Before(cert.NotBefore) {
		return false, nil
	}

	return true, nil
}

// GetCertExpiry 获取证书过期时间
func (cm *CertificateManager) GetCertExpiry(domain string) (time.Time, error) {
	certDir := filepath.Join(cm.paths.ConfigDir, "certs")
	certFile := filepath.Join(certDir, domain+".crt")

	certData, err := os.ReadFile(certFile)
	if err != nil {
		return time.Time{}, fmt.Errorf("read certificate: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return time.Time{}, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse certificate: %w", err)
	}

	return cert.NotAfter, nil
}

// ListCerts 列出所有证书
func (cm *CertificateManager) ListCerts() ([]*CertInfo, error) {
	certDir := filepath.Join(cm.paths.ConfigDir, "certs")

	// 确保目录存在
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, fmt.Errorf("create cert directory: %w", err)
	}

	files, err := os.ReadDir(certDir)
	if err != nil {
		return nil, fmt.Errorf("read cert directory: %w", err)
	}

	certs := make([]*CertInfo, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".crt") {
			continue
		}

		domain := strings.TrimSuffix(file.Name(), ".crt")
		cert, err := cm.LoadCert(domain)
		if err != nil {
			logger.Warn("[证书管理] 加载证书失败", "domain", domain, "error", err)
			continue
		}

		certs = append(certs, cert)
	}

	return certs, nil
}

// DeleteCert 删除证书
func (cm *CertificateManager) DeleteCert(domain string) error {
	certDir := filepath.Join(cm.paths.ConfigDir, "certs")
	certFile := filepath.Join(certDir, domain+".crt")
	keyFile := filepath.Join(certDir, domain+".key")

	// 删除证书文件
	if err := os.Remove(certFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove certificate: %w", err)
	}

	// 删除私钥文件
	if err := os.Remove(keyFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove private key: %w", err)
	}

	logger.Info("[证书管理] 删除证书", "domain", domain)
	return nil
}

// CertInfo 证书信息
type CertInfo struct {
	Domain       string    `json:"domain"`
	CertType    CertType `json:"cert_type"`
	CertPath    string    `json:"cert_path"`
	KeyPath     string    `json:"key_path"`
	ExpiresAt   time.Time `json:"expires_at"`
	Issuer      string    `json:"issuer,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	AutoRenew   bool      `json:"auto_renew"`
	ACMEEmail   string    `json:"acme_email,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
}

// CertStatus 证书状态
type CertStatus struct {
	Domain      string    `json:"domain"`
	Valid       bool      `json:"valid"`
	ExpiresIn   int       `json:"expires_in_days"`
	ExpiresAt   string    `json:"expires_at"`
	Fingerprint string    `json:"fingerprint"`
}

// GetCertStatus 获取证书状态
func (cm *CertificateManager) GetCertStatus(domain string) (*CertStatus, error) {
	cert, err := cm.LoadCert(domain)
	if err != nil {
		return nil, err
	}

	valid, err := cm.ValidateCert(cert.CertPath)
	if err != nil {
		return nil, err
	}

	expiresIn := int(time.Until(cert.ExpiresAt).Hours() / 24)

	status := &CertStatus{
		Domain:      domain,
		Valid:       valid,
		ExpiresIn:   expiresIn,
		ExpiresAt:   cert.ExpiresAt.Format("2006-01-02 15:04:05"),
		Fingerprint: cert.Fingerprint,
	}

	return status, nil
}

// 工具函数

func generateFingerprint(certRaw []byte) string {
	return fmt.Sprintf("%x", certRaw)
}

// CheckCertExpiry 检查证书是否即将过期
func CheckCertExpiry(cert *CertInfo, warnDays int) bool {
	if cert.ExpiresAt.IsZero() {
		return false
	}

	daysUntilExpiry := int(time.Until(cert.ExpiresAt).Hours() / 24)
	return daysUntilExpiry <= warnDays
}

// GenerateCertFingerprint 生成证书指纹
func GenerateCertFingerprint(certPath string) (string, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("read certificate: %w", err)
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate PEM")
	}

	return generateFingerprint(block.Bytes), nil
}