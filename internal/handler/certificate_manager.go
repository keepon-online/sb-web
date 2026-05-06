package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"miaomiaowu/internal/certificate"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// CertGenerateRequest 证书生成请求
type CertGenerateRequest struct {
	Domain       string `json:"domain"`
	CertType     string `json:"cert_type"`
	ValidityDays int    `json:"validity_days"`
	Email        string `json:"email,omitempty"`
	AutoRenew    bool   `json:"auto_renew"`
}

// CertRenewRequest 证书更新请求
type CertRenewRequest struct {
	Domain    string `json:"domain"`
	Force     bool   `json:"force"`
	WarnDays  int    `json:"warn_days,omitempty"`
}

// NewCertificateGenerateHandler 创建证书生成处理器
func NewCertificateGenerateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req CertGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证域名
		if err := singbox.ValidateDomain(req.Domain); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid domain: %w", err))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "cert_generate", fmt.Sprintf("生成证书: %s", req.Domain))

		// 生成证书
		var certInfo *certificate.CertInfo
		var err error

		switch req.CertType {
		case "selfsigned", "":
			cm := certificate.NewCertificateManager()
			certInfo, err = cm.GenerateSelfSignedCert(req.Domain, req.ValidityDays)

		case "acme":
			if req.Email == "" {
				writeError(w, http.StatusBadRequest, errors.New("email is required for ACME certificates"))
				return
			}
			ac := certificate.NewACMEClient(req.Email)
			certInfo, err = ac.RequestCertificate(req.Domain)

		default:
			writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported certificate type: %s", req.CertType))
			return
		}

		if err != nil {
			logger.Error("[证书API] 证书生成失败", "error", err)
			logOperationWithError(repo, username, "cert_generate", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("generate certificate failed: %w", err))
			return
		}

		// 保存到数据库
		if err := saveCertToDatabase(repo, certInfo, username); err != nil {
			logger.Warn("[证书API] 数据库保存失败", "error", err)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "证书生成成功",
			"cert":    certInfo,
		})

		logger.Info("[证书API] 证书生成成功", "domain", req.Domain, "type", req.CertType)
	})
}

// NewCertificateRenewHandler 创建证书更新处理器
func NewCertificateRenewHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req CertRenewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 设置默认值
		if req.WarnDays <= 0 {
			req.WarnDays = 30 // 默认30天
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "cert_renew", fmt.Sprintf("更新证书: %s", req.Domain))

		// 更新证书
		cm := certificate.NewCertificateManager()
		certInfo, err := cm.LoadCert(req.Domain)
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("certificate not found: %w", err))
			return
		}

		// 检查是否需要更新
		if !req.Force && !certificate.CheckCertExpiry(certInfo, req.WarnDays) {
			writeJSON(w, http.StatusOK, map[string]string{
				"status":  "success",
				"message": "证书无需更新",
				"reason":  "not expired",
			})
			return
		}

		// 根据证书类型执行更新
		var renewedCert *certificate.CertInfo
		switch certInfo.CertType {
		case "acme":
			ac := certificate.NewACMEClient(certInfo.ACMEEmail)
			renewedCert, err = ac.RenewCertificate(req.Domain)

		default:
			// 对于自签名证书，重新生成
			renewedCert, err = cm.GenerateSelfSignedCert(req.Domain, 365)
		}

		if err != nil {
			logger.Error("[证书API] 证书更新失败", "error", err)
			logOperationWithError(repo, username, "cert_renew", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("renew certificate failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "证书更新成功",
			"cert":    renewedCert,
		})

		logger.Info("[证书API] 证书更新成功", "domain", req.Domain)
	})
}

// NewCertificateListHandler 创建证书列表处理器
func NewCertificateListHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 获取证书列表
		cm := certificate.NewCertificateManager()
		certs, err := cm.ListCerts()
		if err != nil {
			logger.Error("[证书API] 获取证书列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list certificates failed: %w", err))
			return
		}

		// 获取证书状态
		certStatuses := make(map[string]*certificate.CertStatus)
		for _, cert := range certs {
		status, err := cm.GetCertStatus(cert.Domain)
		if err == nil {
			certStatuses[cert.Domain] = status
		}
	}

		writeJSON(w, http.StatusOK, map[string]interface{}{
		"certs":       certs,
		"cert_status": certStatuses,
	})
}

}

// NewCertificateDeleteHandler 创建证书删除处理器
func NewCertificateDeleteHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only DELETE or POST is supported"))
			return
		}

		// 从URL获取域名
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			writeError(w, http.StatusBadRequest, errors.New("domain parameter is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "cert_delete", fmt.Sprintf("删除证书: %s", domain))

		// 删除证书
		cm := certificate.NewCertificateManager()
		if err := cm.DeleteCert(domain); err != nil {
			logger.Error("[证书API] 证书删除失败", "error", err)
			logOperationWithError(repo, username, "cert_delete", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("delete certificate failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
		"message": "证书删除成功",
	})

		logger.Info("[证书API] 证书删除成功", "domain", domain)
	})
}

// NewCertificateCheckHandler 创建证书检查处理器
func NewCertificateCheckHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			Domains   []string `json:"domains"`
			WarnDays  int      `json:"warn_days,omitempty"`
			AutoRenew bool     `json:"auto_renew,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 设置默认值
		if req.WarnDays <= 0 {
			req.WarnDays = 30
		}

		results := make(map[string]*certificate.CertStatus)

		// 检查每个证书
		cm := certificate.NewCertificateManager()
		for _, domain := range req.Domains {
			status, err := cm.GetCertStatus(domain)
			if err != nil {
				logger.Warn("[证书API] 证书检查失败", "domain", domain, "error", err)
				continue
			}
			results[domain] = status

			// 如果启用自动更新且证书即将过期
			if req.AutoRenew && !status.Valid && status.ExpiresIn <= req.WarnDays {
				logger.Info("[证书API] 触发自动更新", "domain", domain, "expires_in_days", status.ExpiresIn)
				// 这里可以触发异步更新任务
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": results,
		})
	})
}

// NewMonitoringStatusHandler 创建监控状态处理器
func NewMonitoringStatusHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 获取系统状态
		env := singbox.DetectEnvironment()
		sysInfo, _ := singbox.GetSystemInfo()

		// 获取证书状态
		cm := certificate.NewCertificateManager()
		certs, err := cm.ListCerts()
		if err != nil {
			logger.Error("[监控API] 获取证书状态失败", "error", err)
		}

		// 检查证书过期状态
		expiringCerts := []string{}
		for _, cert := range certs {
			if certificate.CheckCertExpiry(cert, 30) {
				expiringCerts = append(expiringCerts, cert.Domain)
			}
		}

		// 端口使用情况
		pm := singbox.NewPortManager()
		usedPorts := pm.GetUsedPorts()
		availableCount := pm.GetAvailablePortCount(10000, 65535)

		status := map[string]interface{}{
			"environment":     env.String(),
			"system_info":     sysInfo,
			"certificates":    map[string]interface{}{
				"total":         len(certs),
				"expiring":      expiringCerts,
			},
			"ports": map[string]interface{}{
				"used":         usedPorts,
				"available":    availableCount,
			},
			"timestamp":      getCurrentTimestamp(),
		}

		writeJSON(w, http.StatusOK, status)
	})
}

// 辅助函数

func saveCertToDatabase(repo *storage.TrafficRepository, certInfo *certificate.CertInfo, username string) error {
	cert := &storage.Certificate{
		Domain:     certInfo.Domain,
		CertType:   string(certInfo.CertType),
		CertPath:   certInfo.CertPath,
		KeyPath:    certInfo.KeyPath,
		ExpiresAt:  certInfo.ExpiresAt.Format("2006-01-02 15:04:05"),
		AutoRenew:  certInfo.AutoRenew,
		ACMEEmail:  certInfo.ACMEEmail,
	}

	return repo.CreateCertificate(cert)
}

func getCurrentTimestamp() string {
	return ""
}

// NewAutoRenewCertificatesHandler 创建自动更新证书处理器
func NewAutoRenewCertificatesHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			WarnDays int `json:"warn_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 设置默认值
		if req.WarnDays <= 0 {
			req.WarnDays = 30
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "cert_auto_renew", fmt.Sprintf("自动更新证书，提前天数: %d", req.WarnDays))

		// 执行自动更新
		// 这里应该使用存储的ACME邮箱
		renewedCerts, err := autoRenewAllCerts(req.WarnDays)
		if err != nil {
			logger.Error("[证书API] 自动更新失败", "error", err)
			logOperationWithError(repo, username, "cert_auto_renew", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("auto renew failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":            "success",
			"message":           "自动更新完成",
			"renewed_certificates": renewedCerts,
		})

		logger.Info("[证书API] 自动更新完成", "renewed_count", len(renewedCerts))
	})
}

func autoRenewAllCerts(warnDays int) ([]string, error) {
	// 这里应该从数据库获取ACME邮箱
	ac := certificate.NewACMEClient("admin@example.com")
	return ac.CheckAllCertificates(warnDays)
}