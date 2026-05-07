package integration

import (
	"encoding/json"
	"fmt"
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

// ArgoTunnelType Argo隧道类型
type ArgoTunnelType string

const (
	ArgoTunnelTypeFixed  ArgoTunnelType = "fixed"  // 固定域名隧道
	ArgoTunnelTypeTemp   ArgoTunnelType = "temp"   // 临时隧道
	ArgoTunnelTypeArgoGo ArgoTunnelType = "argogo" // argo-go
)

// ArgoTunnel Argo隧道配置
type ArgoTunnel struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Type         ArgoTunnelType   `json:"type"`
	Domain       string           `json:"domain,omitempty"`
	Token        string           `json:"token,omitempty"`
	Credentials  string           `json:"credentials,omitempty"`
	Enabled      bool             `json:"enabled"`
	Port         int              `json:"port"`
	LocalService string           `json:"local_service"`
	CreatedAt    time.Time        `json:"created_at"`
	LastUsed     time.Time        `json:"last_used"`
	Status       ArgoTunnelStatus `json:"status"`
}

// ArgoTunnelStatus 隧道状态
type ArgoTunnelStatus struct {
	Running   bool      `json:"running"`
	URL       string    `json:"url,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`
	Connected bool      `json:"connected"`
}

// ArgoManager Argo隧道管理器
type ArgoManager struct {
	paths             singbox.ConfigPaths
	tunnelDir         string
	binaryDir         string
	argoBinary        string
	cloudflaredBinary string
}

// NewArgoManager 创建Argo隧道管理器
func NewArgoManager() *ArgoManager {
	env := singbox.DetectEnvironment()
	paths := singbox.GetConfigPaths(env)

	tunnelDir := filepath.Join(paths.ConfigDir, "argo")
	binaryDir := paths.BinDir

	return &ArgoManager{
		paths:             paths,
		tunnelDir:         tunnelDir,
		binaryDir:         binaryDir,
		argoBinary:        filepath.Join(binaryDir, "argo"),
		cloudflaredBinary: filepath.Join(binaryDir, "cloudflared"),
	}
}

// DownloadCloudflared 下载cloudflared二进制文件
func (am *ArgoManager) DownloadCloudflared(progress chan int) error {
	logger.Info("[Argo] 开始下载cloudflared")

	arch, osType := getSystemArchitecture()
	baseURL := "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-"
	binaryName := fmt.Sprintf("%s-%s", osType, arch)
	downloadURL := baseURL + binaryName

	outputPath := am.cloudflaredBinary + ".tmp"

	// 使用系统命令下载
	cmd := exec.Command("curl", "-L", "-o", outputPath, downloadURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("下载cloudflared失败: %w", err)
	}

	// 设置执行权限
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("设置cloudflared权限失败: %w", err)
	}

	// 移动到最终位置
	if err := os.Rename(outputPath, am.cloudflaredBinary); err != nil {
		return fmt.Errorf("移动cloudflared失败: %w", err)
	}

	// 验证二进制文件
	cmd = exec.Command(am.cloudflaredBinary, "--version")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("验证cloudflared失败: %w: %s", err, string(output))
	}

	logger.Info("[Argo] cloudflared下载成功", "path", am.cloudflaredBinary)

	if progress != nil {
		progress <- 100
	}

	return nil
}

// DownloadArgoGo 下载argo-go二进制文件
func (am *ArgoManager) DownloadArgoGo(progress chan int) error {
	logger.Info("[Argo] 开始下载argo-go")

	// argo-go的下载地址（示例，实际地址可能不同）
	downloadURL := "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
	outputPath := am.argoBinary + ".tmp"

	// 使用系统命令下载
	cmd := exec.Command("curl", "-L", "-o", outputPath, downloadURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("下载argo-go失败: %w", err)
	}

	// 设置执行权限
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("设置argo-go权限失败: %w", err)
	}

	// 移动到最终位置
	if err := os.Rename(outputPath, am.argoBinary); err != nil {
		return fmt.Errorf("移动argo-go失败: %w", err)
	}

	logger.Info("[Argo] argo-go下载成功", "path", am.argoBinary)

	if progress != nil {
		progress <- 100
	}

	return nil
}

// CreateFixedTunnel 创建固定域名隧道
func (am *ArgoManager) CreateFixedTunnel(name, domain, token string, localPort int) (*ArgoTunnel, error) {
	logger.Info("[Argo] 创建固定域名隧道", "name", name, "domain", domain)

	// 验证cloudflared是否已安装
	if _, err := os.Stat(am.cloudflaredBinary); os.IsNotExist(err) {
		return nil, fmt.Errorf("cloudflared未安装，请先安装")
	}

	// 创建隧道目录
	if err := os.MkdirAll(am.tunnelDir, 0755); err != nil {
		return nil, fmt.Errorf("创建隧道目录失败: %w", err)
	}

	// 生成凭据文件
	credentialsPath := filepath.Join(am.tunnelDir, name+".json")
	if err := am.generateCredentialsFile(token, credentialsPath); err != nil {
		return nil, fmt.Errorf("生成凭据文件失败: %w", err)
	}

	tunnel := &ArgoTunnel{
		ID:           generateTunnelID(),
		Name:         name,
		Type:         ArgoTunnelTypeFixed,
		Domain:       domain,
		Token:        token,
		Credentials:  credentialsPath,
		Enabled:      true,
		Port:         localPort,
		LocalService: fmt.Sprintf("http://localhost:%d", localPort),
		CreatedAt:    time.Now(),
		Status: ArgoTunnelStatus{
			Running: false,
		},
	}

	// 保存隧道配置
	if err := am.saveTunnelConfig(tunnel); err != nil {
		return nil, fmt.Errorf("保存隧道配置失败: %w", err)
	}

	logger.Info("[Argo] 固定域名隧道创建成功", "name", name, "domain", domain)
	return tunnel, nil
}

// CreateTempTunnel 创建临时隧道
func (am *ArgoManager) CreateTempTunnel(name string, localPort int) (*ArgoTunnel, error) {
	logger.Info("[Argo] 创建临时隧道", "name", name, "port", localPort)

	// 验证cloudflared是否已安装
	if _, err := os.Stat(am.cloudflaredBinary); os.IsNotExist(err) {
		return nil, fmt.Errorf("cloudflared未安装，请先安装")
	}

	tunnel := &ArgoTunnel{
		ID:           generateTunnelID(),
		Name:         name,
		Type:         ArgoTunnelTypeTemp,
		Enabled:      true,
		Port:         localPort,
		LocalService: fmt.Sprintf("http://localhost:%d", localPort),
		CreatedAt:    time.Now(),
		Status: ArgoTunnelStatus{
			Running: false,
		},
	}

	// 保存隧道配置
	if err := am.saveTunnelConfig(tunnel); err != nil {
		return nil, fmt.Errorf("保存隧道配置失败: %w", err)
	}

	logger.Info("[Argo] 临时隧道创建成功", "name", name)
	return tunnel, nil
}

// StartTunnel 启动隧道
func (am *ArgoManager) StartTunnel(tunnelID string) error {
	logger.Info("[Argo] 启动隧道", "tunnel_id", tunnelID)

	// 加载隧道配置
	tunnel, err := am.loadTunnelConfig(tunnelID)
	if err != nil {
		return fmt.Errorf("加载隧道配置失败: %w", err)
	}

	if tunnel.Status.Running {
		return fmt.Errorf("隧道已在运行中")
	}

	// 根据隧道类型启动
	switch tunnel.Type {
	case ArgoTunnelTypeFixed:
		if err := am.startFixedTunnel(tunnel); err != nil {
			return fmt.Errorf("启动固定隧道失败: %w", err)
		}
	case ArgoTunnelTypeTemp:
		if err := am.startTempTunnel(tunnel); err != nil {
			return fmt.Errorf("启动临时隧道失败: %w", err)
		}
	case ArgoTunnelTypeArgoGo:
		if err := am.startArgoGo(tunnel); err != nil {
			return fmt.Errorf("启动argo-go失败: %w", err)
		}
	default:
		return fmt.Errorf("不支持的隧道类型: %s", tunnel.Type)
	}

	tunnel.Status.Running = true
	tunnel.Status.StartTime = time.Now()
	tunnel.LastUsed = time.Now()

	// 更新配置
	if err := am.saveTunnelConfig(tunnel); err != nil {
		logger.Warn("[Argo] 更新隧道状态失败", "error", err)
	}

	logger.Info("[Argo] 隧道启动成功", "tunnel_id", tunnelID)
	return nil
}

// StopTunnel 停止隧道
func (am *ArgoManager) StopTunnel(tunnelID string) error {
	logger.Info("[Argo] 停止隧道", "tunnel_id", tunnelID)

	// 加载隧道配置
	tunnel, err := am.loadTunnelConfig(tunnelID)
	if err != nil {
		return fmt.Errorf("加载隧道配置失败: %w", err)
	}

	if !tunnel.Status.Running {
		return fmt.Errorf("隧道未运行")
	}

	// 查找并停止进程
	if err := am.killTunnelProcess(tunnelID); err != nil {
		return fmt.Errorf("停止隧道进程失败: %w", err)
	}

	tunnel.Status.Running = false
	tunnel.Status.StartTime = time.Time{}

	// 更新配置
	if err := am.saveTunnelConfig(tunnel); err != nil {
		logger.Warn("[Argo] 更新隧道状态失败", "error", err)
	}

	logger.Info("[Argo] 隧道停止成功", "tunnel_id", tunnelID)
	return nil
}

// GetTunnelStatus 获取隧道状态
func (am *ArgoManager) GetTunnelStatus(tunnelID string) (*ArgoTunnelStatus, error) {
	tunnel, err := am.loadTunnelConfig(tunnelID)
	if err != nil {
		return nil, err
	}

	// 检查进程是否还在运行
	running := am.isTunnelRunning(tunnelID)
	if !running && tunnel.Status.Running {
		// 进程已停止，更新状态
		tunnel.Status.Running = false
		am.saveTunnelConfig(tunnel)
	}

	return &tunnel.Status, nil
}

// ListTunnels 列出所有隧道
func (am *ArgoManager) ListTunnels() ([]*ArgoTunnel, error) {
	files, err := os.ReadDir(am.tunnelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ArgoTunnel{}, nil
		}
		return nil, fmt.Errorf("读取隧道目录失败: %w", err)
	}

	tunnels := make([]*ArgoTunnel, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		tunnelID := strings.TrimSuffix(file.Name(), ".json")
		tunnel, err := am.loadTunnelConfig(tunnelID)
		if err != nil {
			logger.Warn("[Argo] 加载隧道配置失败", "tunnel_id", tunnelID, "error", err)
			continue
		}

		tunnels = append(tunnels, tunnel)
	}

	return tunnels, nil
}

// DeleteTunnel 删除隧道
func (am *ArgoManager) DeleteTunnel(tunnelID string) error {
	logger.Info("[Argo] 删除隧道", "tunnel_id", tunnelID)

	// 停止隧道
	am.StopTunnel(tunnelID)

	// 删除配置文件
	configPath := filepath.Join(am.tunnelDir, tunnelID+".json")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除配置文件失败: %w", err)
	}

	logger.Info("[Argo] 隧道删除成功", "tunnel_id", tunnelID)
	return nil
}

// 内部方法

func (am *ArgoManager) startFixedTunnel(tunnel *ArgoTunnel) error {
	// 启动固定域名隧道
	args := []string{
		"tunnel",
		"--config", tunnel.Credentials,
		"run",
	}

	cmd := exec.Command(am.cloudflaredBinary, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动隧道失败: %w", err)
	}

	// 保存PID
	pidPath := filepath.Join(am.tunnelDir, tunnel.ID+".pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		logger.Warn("[Argo] 保存PID失败", "error", err)
	}

	return nil
}

func (am *ArgoManager) startTempTunnel(tunnel *ArgoTunnel) error {
	// 启动临时隧道
	args := []string{
		"tunnel",
		"--url", tunnel.LocalService,
		"metrics",
		"localhost:0",
	}

	cmd := exec.Command(am.cloudflaredBinary, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动临时隧道失败: %w", err)
	}

	// 保存PID
	pidPath := filepath.Join(am.tunnelDir, tunnel.ID+".pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		logger.Warn("[Argo] 保存PID失败", "error", err)
	}

	// 等待一段时间获取临时URL
	time.Sleep(3 * time.Second)
	url := am.getTempTunnelURL(cmd)
	if url != "" {
		tunnel.Status.URL = url
	}

	return nil
}

func (am *ArgoManager) startArgoGo(tunnel *ArgoTunnel) error {
	// 启动argo-go
	args := []string{
		"tunnel",
		"--url", tunnel.LocalService,
		"--log", "info",
	}

	cmd := exec.Command(am.argoBinary, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动argo-go失败: %w", err)
	}

	// 保存PID
	pidPath := filepath.Join(am.tunnelDir, tunnel.ID+".pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		logger.Warn("[Argo] 保存PID失败", "error", err)
	}

	return nil
}

func (am *ArgoManager) generateCredentialsFile(token, outputPath string) error {
	// 这里应该调用Cloudflare API生成凭据文件
	// 简化实现，创建一个基本的凭据文件
	credentials := map[string]interface{}{
		"AccountTag":   "dummy",
		"TunnelID":     generateTunnelID(),
		"TunnelName":   "tunnel",
		"TunnelSecret": token,
	}

	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化凭据失败: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("写入凭据文件失败: %w", err)
	}

	return nil
}

func (am *ArgoManager) saveTunnelConfig(tunnel *ArgoTunnel) error {
	configPath := filepath.Join(am.tunnelDir, tunnel.ID+".json")

	data, err := json.MarshalIndent(tunnel, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化隧道配置失败: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("写入隧道配置失败: %w", err)
	}

	return nil
}

func (am *ArgoManager) loadTunnelConfig(tunnelID string) (*ArgoTunnel, error) {
	configPath := filepath.Join(am.tunnelDir, tunnelID+".json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取隧道配置失败: %w", err)
	}

	var tunnel ArgoTunnel
	if err := json.Unmarshal(data, &tunnel); err != nil {
		return nil, fmt.Errorf("解析隧道配置失败: %w", err)
	}

	return &tunnel, nil
}

func (am *ArgoManager) killTunnelProcess(tunnelID string) error {
	pidPath := filepath.Join(am.tunnelDir, tunnelID+".pid")

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("读取PID文件失败: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return fmt.Errorf("解析PID失败: %w", err)
	}

	// 查找并杀死进程
	if err := util.Execute("kill", "-9", fmt.Sprintf("%d", pid)); err != nil {
		return fmt.Errorf("杀死进程失败: %w", err)
	}

	// 删除PID文件
	os.Remove(pidPath)

	return nil
}

func (am *ArgoManager) isTunnelRunning(tunnelID string) bool {
	pidPath := filepath.Join(am.tunnelDir, tunnelID+".pid")

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
		return false
	}

	// 检查进程是否存在
	cmd := exec.Command("kill", "-0", fmt.Sprintf("%d", pid))
	return cmd.Run() == nil
}

func (am *ArgoManager) getTempTunnelURL(cmd *exec.Cmd) string {
	// 从进程输出中获取临时URL
	// 简化实现，返回空字符串
	return ""
}

// Cloudflare API客户端

// CloudflareClient Cloudflare API客户端
type CloudflareClient struct {
	apiToken string
	baseURL  string
	client   *http.Client
}

// NewCloudflareClient 创建Cloudflare API客户端
func NewCloudflareClient(apiToken string) *CloudflareClient {
	return &CloudflareClient{
		apiToken: apiToken,
		baseURL:  "https://api.cloudflare.com/client/v4",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateTunnel 创建隧道
func (cc *CloudflareClient) CreateTunnel(name, accountID string) (*ArgoTunnel, error) {
	// 实现Cloudflare API调用创建隧道
	return nil, fmt.Errorf("未实现")
}

// GetTunnelInfo 获取隧道信息
func (cc *CloudflareClient) GetTunnelInfo(tunnelID string) (*ArgoTunnel, error) {
	// 实现Cloudflare API调用获取隧道信息
	return nil, fmt.Errorf("未实现")
}

// ListTunnels 列出账户下的所有隧道
func (cc *CloudflareClient) ListTunnels(accountID string) ([]*ArgoTunnel, error) {
	// 实现Cloudflare API调用列出隧道
	return nil, fmt.Errorf("未实现")
}

// 工具函数

func getSystemArchitecture() (string, string) {
	// 获取系统架构
	osType := "linux"
	arch := "amd64"

	// 简化实现
	return arch, osType
}

func generateTunnelID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ValidateToken 验证Cloudflare Token
func ValidateToken(token string) bool {
	if token == "" {
		return false
	}

	// 基本验证：Cloudflare Token通常是37个字符
	if len(token) != 37 {
		return false
	}

	// Token应该以特定格式开头
	if !strings.HasPrefix(token, "eyJ") && !strings.HasPrefix(token, "aB") {
		return false
	}

	return true
}

// GetTunnelMetrics 获取隧道指标
func (am *ArgoManager) GetTunnelMetrics(tunnelID string) (map[string]interface{}, error) {
	tunnel, err := am.loadTunnelConfig(tunnelID)
	if err != nil {
		return nil, err
	}

	metrics := map[string]interface{}{
		"tunnel_id":   tunnelID,
		"running":     tunnel.Status.Running,
		"uptime":      time.Since(tunnel.Status.StartTime).String(),
		"local_port":  tunnel.Port,
		"local_url":   tunnel.LocalService,
		"remote_url":  tunnel.Status.URL,
		"connections": 0,
		"bytes_sent":  0,
		"bytes_recv":  0,
	}

	// 如果隧道正在运行，获取实际指标
	if tunnel.Status.Running {
		// 这里应该调用cloudflared的metrics端点
		// 简化实现，返回默认值
	}

	return metrics, nil
}

// DownloadArgoTunnelQuickTunnels 下载argo隧道快速隧道工具
func (am *ArgoManager) DownloadArgoTunnelQuickTunnels(progress chan int) error {
	logger.Info("[Argo] 开始下载argo隧道快速隧道工具")

	// 下载argo隧道快速隧道工具
	downloadURL := "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
	outputPath := filepath.Join(am.binaryDir, "argo-tunnel")

	// 使用系统命令下载
	cmd := exec.Command("curl", "-L", "-o", outputPath, downloadURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("下载argo隧道快速隧道工具失败: %w", err)
	}

	// 设置执行权限
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("设置argo隧道快速隧道工具权限失败: %w", err)
	}

	logger.Info("[Argo] argo隧道快速隧道工具下载成功", "path", outputPath)

	if progress != nil {
		progress <- 100
	}

	return nil
}

// CreateQuickTunnel 创建快速隧道（不需要token）
func (am *ArgoManager) CreateQuickTunnel(name string, localPort int) (*ArgoTunnel, string, error) {
	logger.Info("[Argo] 创建快速隧道", "name", name, "port", localPort)

	// 验证cloudflared是否已安装
	if _, err := os.Stat(am.cloudflaredBinary); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("cloudflared未安装，请先安装")
	}

	// 创建隧道目录
	if err := os.MkdirAll(am.tunnelDir, 0755); err != nil {
		return nil, "", fmt.Errorf("创建隧道目录失败: %w", err)
	}

	// 启动快速隧道并获取URL
	localService := fmt.Sprintf("http://localhost:%d", localPort)
	args := []string{
		"tunnel",
		"--url", localService,
		"metrics",
		"localhost:0",
	}

	cmd := exec.Command(am.cloudflaredBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", fmt.Errorf("启动快速隧道失败: %w", err)
	}

	// 从输出中提取URL
	url := am.extractQuickTunnelURL(string(output))
	if url == "" {
		return nil, "", fmt.Errorf("无法获取快速隧道URL")
	}

	tunnel := &ArgoTunnel{
		ID:           generateTunnelID(),
		Name:         name,
		Type:         ArgoTunnelTypeTemp,
		Enabled:      true,
		Port:         localPort,
		LocalService: localService,
		CreatedAt:    time.Now(),
		Status: ArgoTunnelStatus{
			Running:   true,
			URL:       url,
			StartTime: time.Now(),
		},
	}

	// 保存隧道配置
	if err := am.saveTunnelConfig(tunnel); err != nil {
		return nil, "", fmt.Errorf("保存隧道配置失败: %w", err)
	}

	logger.Info("[Argo] 快速隧道创建成功", "name", name, "url", url)
	return tunnel, url, nil
}

func (am *ArgoManager) extractQuickTunnelURL(output string) string {
	// 从输出中提取快速隧道URL
	// 输出格式通常为: "https://xxx.trycloudflare.com"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "https://") && strings.Contains(line, "trycloudflare.com") {
			// 提取URL
			start := strings.Index(line, "https://")
			if start >= 0 {
				url := line[start:]
				end := strings.IndexAny(url, " \n\r\t")
				if end > 0 {
					return url[:end]
				}
				return url
			}
		}
	}
	return ""
}

// CreateArgoGoTunnel 创建argo-go隧道
func (am *ArgoManager) CreateArgoGoTunnel(name string, localPort int) (*ArgoTunnel, error) {
	logger.Info("[Argo] 创建argo-go隧道", "name", name, "port", localPort)

	// 验证argo-go是否已安装
	if _, err := os.Stat(am.argoBinary); os.IsNotExist(err) {
		return nil, fmt.Errorf("argo-go未安装，请先安装")
	}

	tunnel := &ArgoTunnel{
		ID:           generateTunnelID(),
		Name:         name,
		Type:         ArgoTunnelTypeArgoGo,
		Enabled:      true,
		Port:         localPort,
		LocalService: fmt.Sprintf("http://localhost:%d", localPort),
		CreatedAt:    time.Now(),
		Status: ArgoTunnelStatus{
			Running: false,
		},
	}

	// 保存隧道配置
	if err := am.saveTunnelConfig(tunnel); err != nil {
		return nil, fmt.Errorf("保存隧道配置失败: %w", err)
	}

	logger.Info("[Argo] argo-go隧道创建成功", "name", name)
	return tunnel, nil
}

// GetTunnelLogs 获取隧道日志
func (am *ArgoManager) GetTunnelLogs(tunnelID string, lines int) (string, error) {
	tunnel, err := am.loadTunnelConfig(tunnelID)
	if err != nil {
		return "", fmt.Errorf("加载隧道配置失败: %w", err)
	}

	if !tunnel.Status.Running {
		return "", fmt.Errorf("隧道未运行")
	}

	// 这里应该从cloudflared的日志文件中读取
	// 简化实现，返回默认日志
	return fmt.Sprintf("[Argo] 隧道 %s 日志 (最近 %d 行)\n", tunnel.Name, lines), nil
}

// FollowTunnelLogs 跟随隧道日志（SSE）
func (am *ArgoManager) FollowTunnelLogs(tunnelID string) (<-chan string, error) {
	logChan := make(chan string, 100)

	go func() {
		defer close(logChan)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tunnel, err := am.loadTunnelConfig(tunnelID)
				if err != nil || !tunnel.Status.Running {
					return
				}

				logs, err := am.GetTunnelLogs(tunnelID, 10)
				if err == nil && logs != "" {
					logChan <- logs
				}
			}
		}
	}()

	return logChan, nil
}
