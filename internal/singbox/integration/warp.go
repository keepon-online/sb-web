package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/util"
)

// WARPType WARP类型
type WARPType string

const (
	WARPTypeWARP   WARPType = "warp"  // 官方WARP
	WARPTypeWARPGo WARPType = "warpo" // WARP-GO
)

// WARPStatus WARP状态
type WARPStatus struct {
	Enabled         bool      `json:"enabled"`
	Type            WARPType  `json:"type"`
	Connected       bool      `json:"connected"`
	AccountID       string    `json:"account_id,omitempty"`
	LicenseKey      string    `json:"license_key,omitempty"`
	AccessToken     string    `json:"access_token,omitempty"`
	IPAddress       string    `json:"ip_address,omitempty"`
	IPAddressType   string    `json:"ip_address_type,omitempty"` // CF, reserved
	LastUpdated     time.Time `json:"last_updated"`
	LastError       string    `json:"last_error,omitempty"`
	PreferredServer bool      `json:"preferred_server"`
}

// WARPConfig WARP配置
type WARPConfig struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Type            WARPType   `json:"type"`
	AccountID       string     `json:"account_id,omitempty"`
	LicenseKey      string     `json:"license_key,omitempty"`
	AccessToken     string     `json:"access_token,omitempty"`
	Enabled         bool       `json:"enabled"`
	Endpoint        string     `json:"endpoint,omitempty"`
	Port            int        `json:"port,omitempty"`
	PreferredServer bool       `json:"preferred_server"`
	CreatedAt       time.Time  `json:"created_at"`
	Status          WARPStatus `json:"status"`
}

// WARPManager WARP管理器
type WARPManager struct {
	paths        singbox.ConfigPaths
	configDir    string
	binaryDir    string
	warpBinary   string
	warpGoBinary string
}

// NewWARPManager 创建WARP管理器
func NewWARPManager() *WARPManager {
	env := singbox.DetectEnvironment()
	paths := singbox.GetConfigPaths(env)

	configDir := filepath.Join(paths.ConfigDir, "warp")
	binaryDir := paths.BinDir

	return &WARPManager{
		paths:        paths,
		configDir:    configDir,
		binaryDir:    binaryDir,
		warpBinary:   filepath.Join(binaryDir, "warp-svc"),
		warpGoBinary: filepath.Join(binaryDir, "warp-go"),
	}
}

// DownloadWARP 下载官方WARP客户端
func (wm *WARPManager) DownloadWARP(progress chan int) error {
	logger.Info("[WARP] 开始下载WARP客户端")

	// 创建配置目录
	if err := os.MkdirAll(wm.configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 下载WARP客户端（示例URL）
	arch, osType := getSystemArchitecture()
	downloadURL := fmt.Sprintf("https://github.com/cloudflare/warp/releases/latest/download/warp-%s-%s", osType, arch)
	outputPath := wm.warpBinary + ".tmp"

	// 使用系统命令下载
	cmd := exec.Command("curl", "-L", "-o", outputPath, downloadURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("下载WARP失败: %w", err)
	}

	// 设置执行权限
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("设置WARP权限失败: %w", err)
	}

	// 移动到最终位置
	if err := os.Rename(outputPath, wm.warpBinary); err != nil {
		return fmt.Errorf("移动WARP失败: %w", err)
	}

	logger.Info("[WARP] WARP客户端下载成功", "path", wm.warpBinary)

	if progress != nil {
		progress <- 100
	}

	return nil
}

// DownloadWARPGo 下载WARP-GO客户端
func (wm *WARPManager) DownloadWARPGo(progress chan int) error {
	logger.Info("[WARP] 开始下载WARP-GO客户端")

	// 创建配置目录
	if err := os.MkdirAll(wm.configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 下载WARP-GO客户端（示例URL）
	downloadURL := "https://github.com/badafans/warp-go/releases/latest/download/warp-go_linux_amd64"
	outputPath := wm.warpGoBinary + ".tmp"

	// 使用系统命令下载
	cmd := exec.Command("curl", "-L", "-o", outputPath, downloadURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("下载WARP-GO失败: %w", err)
	}

	// 设置执行权限
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("设置WARP-GO权限失败: %w", err)
	}

	// 移动到最终位置
	if err := os.Rename(outputPath, wm.warpGoBinary); err != nil {
		return fmt.Errorf("移动WARP-GO失败: %w", err)
	}

	logger.Info("[WARP] WARP-GO客户端下载成功", "path", wm.warpGoBinary)

	if progress != nil {
		progress <- 100
	}

	return nil
}

// EnableWARP 启用WARP
func (wm *WARPManager) EnableWARP(licenseKey string) error {
	logger.Info("[WARP] 启用WARP")

	// 检查WARP是否已安装
	if _, err := os.Stat(wm.warpBinary); os.IsNotExist(err) {
		return fmt.Errorf("WARP客户端未安装，请先安装")
	}

	// 注册WARP账户
	if licenseKey != "" {
		if err := wm.registerWarpAccount(licenseKey); err != nil {
			return fmt.Errorf("注册WARP账户失败: %w", err)
		}
	}

	// 启动WARP服务
	if err := wm.startWarpService(); err != nil {
		return fmt.Errorf("启动WARP服务失败: %w", err)
	}

	// 创建WARP配置
	config := &WARPConfig{
		ID:              generateWarpID(),
		Name:            "WARP",
		Type:            WARPTypeWARP,
		LicenseKey:      licenseKey,
		Enabled:         true,
		PreferredServer: false,
		CreatedAt:       time.Now(),
		Status: WARPStatus{
			Enabled:     true,
			Type:        WARPTypeWARP,
			Connected:   false,
			LastUpdated: time.Now(),
		},
	}

	// 保存配置
	if err := wm.saveConfig(config); err != nil {
		return fmt.Errorf("保存WARP配置失败: %w", err)
	}

	logger.Info("[WARP] WARP启用成功")
	return nil
}

// EnableWARPGo 启用WARP-GO
func (wm *WARPManager) EnableWARPGo(licenseKey string, port int, preferredServer bool) error {
	logger.Info("[WARP] 启用WARP-GO")

	// 检查WARP-GO是否已安装
	if _, err := os.Stat(wm.warpGoBinary); os.IsNotExist(err) {
		return fmt.Errorf("WARP-GO客户端未安装，请先安装")
	}

	// 创建WARP-GO配置
	config := &WARPConfig{
		ID:              generateWarpID(),
		Name:            "WARP-GO",
		Type:            WARPTypeWARPGo,
		LicenseKey:      licenseKey,
		Enabled:         true,
		Port:            port,
		PreferredServer: preferredServer,
		CreatedAt:       time.Now(),
		Status: WARPStatus{
			Enabled:         true,
			Type:            WARPTypeWARPGo,
			Connected:       false,
			PreferredServer: preferredServer,
			LastUpdated:     time.Now(),
		},
	}

	// 保存配置
	if err := wm.saveConfig(config); err != nil {
		return fmt.Errorf("保存WARP-GO配置失败: %w", err)
	}

	// 启动WARP-GO服务
	if err := wm.startWarpGoService(config); err != nil {
		return fmt.Errorf("启动WARP-GO服务失败: %w", err)
	}

	logger.Info("[WARP] WARP-GO启用成功")
	return nil
}

// DisableWARP 禁用WARP
func (wm *WARPManager) DisableWARP() error {
	logger.Info("[WARP] 禁用WARP")

	// 停止WARP服务
	wm.stopWarpService()

	// 停止WARP-GO服务
	wm.stopWarpGoService()

	// 禁用所有配置
	configs, err := wm.listConfigs()
	if err != nil {
		return fmt.Errorf("获取WARP配置失败: %w", err)
	}

	for _, config := range configs {
		config.Enabled = false
		config.Status.Enabled = false
		config.Status.Connected = false
		wm.saveConfig(config)
	}

	logger.Info("[WARP] WARP禁用成功")
	return nil
}

// GetWARPStatus 获取WARP状态
func (wm *WARPManager) GetWARPStatus() (*WARPStatus, error) {
	configs, err := wm.listConfigs()
	if err != nil {
		return nil, err
	}

	if len(configs) == 0 {
		return &WARPStatus{
			Enabled:   false,
			Type:      WARPTypeWARP,
			Connected: false,
		}, nil
	}

	// 获取启用的配置
	var activeConfig *WARPConfig
	for _, config := range configs {
		if config.Enabled {
			activeConfig = config
			break
		}
	}

	if activeConfig == nil {
		return &WARPStatus{
			Enabled: false,
		}, nil
	}

	// 更新状态
	switch activeConfig.Type {
	case WARPTypeWARP:
		return wm.getWarpStatus(activeConfig)
	case WARPTypeWARPGo:
		return wm.getWarpGoStatus(activeConfig)
	default:
		return &activeConfig.Status, nil
	}
}

// GetWARPConfigs 获取WARP配置列表
func (wm *WARPManager) GetWARPConfigs() ([]*WARPConfig, error) {
	return wm.listConfigs()
}

// UpdateWARPConfig 更新WARP配置
func (wm *WARPManager) UpdateWARPConfig(configID string, updates map[string]interface{}) error {
	config, err := wm.loadConfig(configID)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 应用更新
	if licenseKey, ok := updates["license_key"].(string); ok {
		config.LicenseKey = licenseKey
	}
	if port, ok := updates["port"].(int); ok {
		config.Port = port
	}
	if preferredServer, ok := updates["preferred_server"].(bool); ok {
		config.PreferredServer = preferredServer
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		config.Enabled = enabled
	}

	// 保存配置
	if err := wm.saveConfig(config); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	// 如果配置已启用，重启服务
	if config.Enabled {
		switch config.Type {
		case WARPTypeWARP:
			wm.restartWarpService()
		case WARPTypeWARPGo:
			wm.restartWarpGoService(config)
		}
	}

	return nil
}

// DeleteWARPConfig 删除WARP配置
func (wm *WARPManager) DeleteWARPConfig(configID string) error {
	logger.Info("[WARP] 删除WARP配置", "config_id", configID)

	config, err := wm.loadConfig(configID)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 停止服务
	if config.Enabled {
		switch config.Type {
		case WARPTypeWARP:
			wm.stopWarpService()
		case WARPTypeWARPGo:
			wm.stopWarpGoService()
		}
	}

	// 删除配置文件
	configPath := filepath.Join(wm.configDir, configID+".json")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除配置文件失败: %w", err)
	}

	logger.Info("[WARP] WARP配置删除成功")
	return nil
}

// 内部方法

func (wm *WARPManager) registerWarpAccount(licenseKey string) error {
	// 使用WARP CLI注册账户
	args := []string{"--accept-tos"}

	if licenseKey != "" {
		args = append(args, "registration", "license", "--key", licenseKey)
	} else {
		args = append(args, "registration", "new")
	}

	cmd := exec.Command(wm.warpBinary, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("注册WARP账户失败: %w: %s", err, string(output))
	}

	return nil
}

func (wm *WARPManager) startWarpService() error {
	// 启动WARP服务
	cmd := exec.Command(wm.warpBinary, "service", "start")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("启动WARP服务失败: %w: %s", err, string(output))
	}

	return nil
}

func (wm *WARPManager) stopWarpService() error {
	// 停止WARP服务
	cmd := exec.Command(wm.warpBinary, "service", "stop")
	_ = cmd.Run() // 忽略错误，服务可能未启动

	// 杀死所有WARP进程
	_ = util.Execute("killall", "-9", "warp-svc")

	return nil
}

func (wm *WARPManager) restartWarpService() {
	wm.stopWarpService()
	time.Sleep(2 * time.Second)
	wm.startWarpService()
}

func (wm *WARPManager) startWarpGoService(config *WARPConfig) error {
	// 启动WARP-GO服务
	args := []string{}

	if config.LicenseKey != "" {
		args = append(args, "--key", config.LicenseKey)
	}

	if config.Port > 0 {
		args = append(args, "--port", fmt.Sprintf("%d", config.Port))
	}

	if config.PreferredServer {
		args = append(args, "--cfon")
	}

	cmd := exec.Command(wm.warpGoBinary, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动WARP-GO服务失败: %w", err)
	}

	// 保存PID
	pidPath := filepath.Join(wm.configDir, config.ID+".pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		logger.Warn("[WARP] 保存PID失败", "error", err)
	}

	return nil
}

func (wm *WARPManager) stopWarpGoService() error {
	// 杀死所有WARP-GO进程
	_ = util.Execute("killall", "-9", "warp-go")

	// 删除PID文件
	files, _ := filepath.Glob(filepath.Join(wm.configDir, "*.pid"))
	for _, file := range files {
		os.Remove(file)
	}

	return nil
}

func (wm *WARPManager) restartWarpGoService(config *WARPConfig) {
	wm.stopWarpGoService()
	time.Sleep(2 * time.Second)
	wm.startWarpGoService(config)
}

func (wm *WARPManager) getWarpStatus(config *WARPConfig) (*WARPStatus, error) {
	status := &WARPStatus{
		Enabled:     true,
		Type:        WARPTypeWARP,
		Connected:   false,
		LastUpdated: time.Now(),
	}

	// 获取WARP状态
	cmd := exec.Command(wm.warpBinary, "status", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		status.LastError = string(output)
		return status, nil
	}

	// 解析状态
	var warpStatus map[string]interface{}
	if err := json.Unmarshal(output, &warpStatus); err != nil {
		status.LastError = err.Error()
		return status, nil
	}

	// 提取状态信息
	if statusField, ok := warpStatus["status"].(map[string]interface{}); ok {
		status.Connected = statusField["connected"] == true
	}

	if account, ok := warpStatus["account"].(map[string]interface{}); ok {
		if accountID, ok := account["id"].(string); ok {
			status.AccountID = accountID
		}
		if license, ok := account["license"].(string); ok {
			status.LicenseKey = license
		}
	}

	if tunnel, ok := warpStatus["tunnel"].(map[string]interface{}); ok {
		if ip, ok := tunnel["ip_address"].(string); ok {
			status.IPAddress = ip
		}
		if ipType, ok := tunnel["ip_address_type"].(string); ok {
			status.IPAddressType = ipType
		}
	}

	return status, nil
}

func (wm *WARPManager) getWarpGoStatus(config *WARPConfig) (*WARPStatus, error) {
	status := &WARPStatus{
		Enabled:         true,
		Type:            WARPTypeWARPGo,
		Connected:       false,
		PreferredServer: config.PreferredServer,
		LastUpdated:     time.Now(),
	}

	// 检查进程是否运行
	pidPath := filepath.Join(wm.configDir, config.ID+".pid")
	if pidData, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err == nil {
			// 检查进程是否存在
			cmd := exec.Command("kill", "-0", fmt.Sprintf("%d", pid))
			status.Connected = cmd.Run() == nil
		}
	}

	// 获取IP地址（简化实现）
	if status.Connected {
		// 这里应该调用API获取实际的IP地址
		status.IPAddress = "Unknown"
		status.IPAddressType = "Unknown"
	}

	return status, nil
}

func (wm *WARPManager) saveConfig(config *WARPConfig) error {
	configPath := filepath.Join(wm.configDir, config.ID+".json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	return nil
}

func (wm *WARPManager) loadConfig(configID string) (*WARPConfig, error) {
	configPath := filepath.Join(wm.configDir, configID+".json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}

	var config WARPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	return &config, nil
}

func (wm *WARPManager) listConfigs() ([]*WARPConfig, error) {
	files, err := os.ReadDir(wm.configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*WARPConfig{}, nil
		}
		return nil, fmt.Errorf("读取配置目录失败: %w", err)
	}

	configs := make([]*WARPConfig, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		configID := strings.TrimSuffix(file.Name(), ".json")
		config, err := wm.loadConfig(configID)
		if err != nil {
			logger.Warn("[WARP] 加载配置失败", "config_id", configID, "error", err)
			continue
		}

		configs = append(configs, config)
	}

	return configs, nil
}

func generateWarpID() string {
	return fmt.Sprintf("warp-%d", time.Now().UnixNano())
}

// CheckWARPConnection 检查WARP连接
func (wm *WARPManager) CheckWARPConnection() (bool, string, error) {
	// 检查WARP连接状态
	status, err := wm.GetWARPStatus()
	if err != nil {
		return false, "", err
	}

	if !status.Connected {
		return false, "WARP未连接", nil
	}

	// 获取当前IP地址
	resp, err := http.Get("https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		return false, "", fmt.Errorf("获取IP地址失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", fmt.Errorf("读取响应失败: %w", err)
	}

	lines := strings.Split(string(body), "\n")
	ip := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "ip=") {
			ip = strings.TrimPrefix(line, "ip=")
			break
		}
	}

	return true, ip, nil
}

// GenerateWarpConfig 生成WARP配置文件
func (wm *WARPManager) GenerateWarpConfig(configType string) (string, error) {
	switch configType {
	case "singbox":
		return wm.generateSingboxWarpConfig(), nil
	case "wireguard":
		return wm.generateWireguardWarpConfig(), nil
	default:
		return "", fmt.Errorf("不支持的配置类型: %s", configType)
	}
}

func (wm *WARPManager) generateSingboxWarpConfig() string {
	// 生成Sing-box格式的WARP配置
	return `{
		"type": "wireguard",
		"tag": "warp-out",
		"server": "engage.cloudflareclient.com",
		"server_port": 2408,
		"local_address": ["172.16.0.2/32", "2606:4700:110:XXXXXX/128"],
		"private_key": "YOUR_PRIVATE_KEY",
		"peer_public_key": "YOUR_PEER_PUBLIC_KEY",
		"mtu": 1280,
		"domain_strategy": "prefer_ipv4"
	}`
}

func (wm *WARPManager) generateWireguardWarpConfig() string {
	// 生成WireGuard格式的WARP配置
	return `[Interface]
PrivateKey = YOUR_PRIVATE_KEY
Address = 172.16.0.2/32
Address = 2606:4700:110:XXXXXX/128
DNS = 1.1.1.1

[Peer]
PublicKey = YOUR_PEER_PUBLIC_KEY
Endpoint = engage.cloudflareclient.com:2408
AllowedIPs = 0.0.0.0/0
AllowedIPs = ::/0`
}
