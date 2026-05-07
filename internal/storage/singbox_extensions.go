package storage

import (
	"fmt"
)

// migrateSingBoxTables 扩展数据库迁移，添加 Sing-box 相关表
func (r *TrafficRepository) migrateSingBoxTables() error {
	if r == nil || r.db == nil {
		return fmt.Errorf("traffic repository not initialized")
	}

	// Sing-box 配置表
	const singboxConfigsSchema = `
	CREATE TABLE IF NOT EXISTS singbox_configs (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    name TEXT NOT NULL,
	    protocol TEXT NOT NULL,
	    port INTEGER NOT NULL,
	    config_json TEXT NOT NULL,
	    enabled INTEGER NOT NULL DEFAULT 1,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_singbox_configs_protocol ON singbox_configs(protocol);
	CREATE INDEX IF NOT EXISTS idx_singbox_configs_enabled ON singbox_configs(enabled);
	`
	if _, err := r.db.Exec(singboxConfigsSchema); err != nil {
		return fmt.Errorf("migrate singbox_configs: %w", err)
	}

	// 证书管理表
	const certificatesSchema = `
	CREATE TABLE IF NOT EXISTS certificates (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    domain TEXT NOT NULL,
	    cert_type TEXT NOT NULL,
	    cert_path TEXT,
	    key_path TEXT,
	    expires_at TIMESTAMP,
	    auto_renew INTEGER NOT NULL DEFAULT 0,
	    acme_email TEXT,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_certificates_domain ON certificates(domain);
	CREATE INDEX IF NOT EXISTS idx_certificates_expires_at ON certificates(expires_at);
	`
	if _, err := r.db.Exec(certificatesSchema); err != nil {
		return fmt.Errorf("migrate certificates: %w", err)
	}

	// Argo 隧道配置表
	const argoTunnelsSchema = `
	CREATE TABLE IF NOT EXISTS argo_tunnels (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    name TEXT NOT NULL,
	    tunnel_type TEXT NOT NULL,
	    domain TEXT,
	    token TEXT,
	    credentials_json TEXT,
	    enabled INTEGER NOT NULL DEFAULT 0,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_argo_tunnels_enabled ON argo_tunnels(enabled);
	`
	if _, err := r.db.Exec(argoTunnelsSchema); err != nil {
		return fmt.Errorf("migrate argo_tunnels: %w", err)
	}

	// WARP 配置表
	const warpConfigsSchema = `
	CREATE TABLE IF NOT EXISTS warp_configs (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    warp_type TEXT NOT NULL,
	    license_key TEXT,
	    account_id TEXT,
	    access_token TEXT,
	    enabled INTEGER NOT NULL DEFAULT 0,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_warp_configs_enabled ON warp_configs(enabled);
	`
	if _, err := r.db.Exec(warpConfigsSchema); err != nil {
		return fmt.Errorf("migrate warp_configs: %w", err)
	}

	// 系统操作日志表
	const systemOperationLogsSchema = `
	CREATE TABLE IF NOT EXISTS system_operation_logs (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    username TEXT NOT NULL,
	    operation TEXT NOT NULL,
	    details TEXT,
	    status TEXT NOT NULL,
	    error_message TEXT,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_system_operation_logs_username ON system_operation_logs(username);
	CREATE INDEX IF NOT EXISTS idx_system_operation_logs_operation ON system_operation_logs(operation);
	CREATE INDEX IF NOT EXISTS idx_system_operation_logs_created_at ON system_operation_logs(created_at);
	`
	if _, err := r.db.Exec(systemOperationLogsSchema); err != nil {
		return fmt.Errorf("migrate system_operation_logs: %w", err)
	}

	// 订阅配置表
	const singboxSubscriptionsSchema = `
	CREATE TABLE IF NOT EXISTS singbox_subscriptions (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    name TEXT NOT NULL,
	    subscription_url TEXT,
	    config_json TEXT NOT NULL,
	    protocols TEXT NOT NULL,
	    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_singbox_subscriptions_name ON singbox_subscriptions(name);
	`
	if _, err := r.db.Exec(singboxSubscriptionsSchema); err != nil {
		return fmt.Errorf("migrate singbox_subscriptions: %w", err)
	}

	// 系统环境信息表
	const systemEnvironmentSchema = `
	CREATE TABLE IF NOT EXISTS system_environment (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    environment_type TEXT NOT NULL,
	    system_info TEXT NOT NULL,
	    network_info TEXT,
	    capabilities TEXT,
	    last_check TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := r.db.Exec(systemEnvironmentSchema); err != nil {
		return fmt.Errorf("migrate system_environment: %w", err)
	}

	return nil
}

// SingboxConfig 表示 Sing-box 配置
type SingboxConfig struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int    `json:"port"`
	ConfigJSON string `json:"config_json"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// Certificate 表示证书信息
type Certificate struct {
	ID        int64  `json:"id"`
	Domain    string `json:"domain"`
	CertType  string `json:"cert_type"`
	CertPath  string `json:"cert_path"`
	KeyPath   string `json:"key_path"`
	ExpiresAt string `json:"expires_at"`
	AutoRenew bool   `json:"auto_renew"`
	ACMEEmail string `json:"acme_email"`
	CreatedAt string `json:"created_at"`
}

// ArgoTunnel 表示 Argo 隧道配置
type ArgoTunnel struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	TunnelType      string `json:"tunnel_type"`
	Domain          string `json:"domain"`
	Token           string `json:"token"`
	CredentialsJSON string `json:"credentials_json"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"created_at"`
}

// WarpConfig 表示 WARP 配置
type WarpConfig struct {
	ID          int64  `json:"id"`
	WarpType    string `json:"warp_type"`
	LicenseKey  string `json:"license_key"`
	AccountID   string `json:"account_id"`
	AccessToken string `json:"access_token"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
}

// SystemOperationLog 表示系统操作日志
type SystemOperationLog struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Operation    string `json:"operation"`
	Details      string `json:"details"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
}

// SingboxSubscription 表示 Sing-box 订阅配置
type SingboxSubscription struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	SubscriptionURL string `json:"subscription_url"`
	ConfigJSON      string `json:"config_json"`
	Protocols       string `json:"protocols"`
	CreatedAt       string `json:"created_at"`
}

// CreateSingboxConfig 创建 Sing-box 配置
func (r *TrafficRepository) CreateSingboxConfig(config *SingboxConfig) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("traffic repository not initialized")
	}

	result, err := r.db.Exec(`
		INSERT INTO singbox_configs (name, protocol, port, config_json, enabled)
		VALUES (?, ?, ?, ?, ?)
	`, config.Name, config.Protocol, config.Port, config.ConfigJSON, config.Enabled)
	if err != nil {
		return fmt.Errorf("create singbox config: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	config.ID = id
	return nil
}

// GetSingboxConfigs 获取所有 Sing-box 配置
func (r *TrafficRepository) GetSingboxConfigs() ([]SingboxConfig, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("traffic repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, name, protocol, port, config_json, enabled, created_at, updated_at
		FROM singbox_configs
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("get singbox configs: %w", err)
	}
	defer rows.Close()

	var configs []SingboxConfig
	for rows.Next() {
		var config SingboxConfig
		var enabled int
		err := rows.Scan(&config.ID, &config.Name, &config.Protocol, &config.Port,
			&config.ConfigJSON, &enabled, &config.CreatedAt, &config.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan singbox config: %w", err)
		}
		config.Enabled = enabled == 1
		configs = append(configs, config)
	}

	return configs, nil
}

// CreateCertificate 创建证书记录
func (r *TrafficRepository) CreateCertificate(cert *Certificate) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("traffic repository not initialized")
	}

	result, err := r.db.Exec(`
		INSERT INTO certificates (domain, cert_type, cert_path, key_path, expires_at, auto_renew, acme_email)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, cert.Domain, cert.CertType, cert.CertPath, cert.KeyPath, cert.ExpiresAt, cert.AutoRenew, cert.ACMEEmail)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	cert.ID = id
	return nil
}

// LogSystemOperation 记录系统操作日志
func (r *TrafficRepository) LogSystemOperation(log *SystemOperationLog) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("traffic repository not initialized")
	}

	_, err := r.db.Exec(`
		INSERT INTO system_operation_logs (username, operation, details, status, error_message)
		VALUES (?, ?, ?, ?, ?)
	`, log.Username, log.Operation, log.Details, log.Status, log.ErrorMessage)
	if err != nil {
		return fmt.Errorf("log system operation: %w", err)
	}

	return nil
}

// GetSystemOperationLogs 获取系统操作日志
func (r *TrafficRepository) GetSystemOperationLogs(limit int, offset int) ([]SystemOperationLog, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("traffic repository not initialized")
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.db.Query(`
		SELECT id, username, operation, details, status, error_message, created_at
		FROM system_operation_logs
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get system operation logs: %w", err)
	}
	defer rows.Close()

	var logs []SystemOperationLog
	for rows.Next() {
		var log SystemOperationLog
		err := rows.Scan(&log.ID, &log.Username, &log.Operation, &log.Details,
			&log.Status, &log.ErrorMessage, &log.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan system operation log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// EnsureTableExists 确保 Sing-box 相关表存在（用于兼容性检查）
func (r *TrafficRepository) EnsureTableExists(tableName string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("traffic repository not initialized")
	}

	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?
	`, tableName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check table exists: %w", err)
	}

	return count > 0, nil
}

// InitSingBoxTables 初始化 Sing-box 表（如果不存在）
func (r *TrafficRepository) InitSingBoxTables() error {
	tables := []string{
		"singbox_configs",
		"certificates",
		"argo_tunnels",
		"warp_configs",
		"system_operation_logs",
		"singbox_subscriptions",
		"system_environment",
	}

	// 检查是否所有表都存在
	allExist := true
	for _, table := range tables {
		exists, err := r.EnsureTableExists(table)
		if err != nil {
			return fmt.Errorf("check table %s exists: %w", table, err)
		}
		if !exists {
			allExist = false
			break
		}
	}

	// 如果所有表都存在，跳过迁移
	if allExist {
		return nil
	}

	// 否则执行迁移
	return r.migrateSingBoxTables()
}
