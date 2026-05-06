package singbox

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/util"
)

// BBRState BBR状态
type BBRState struct {
	Enabled     bool   `json:"enabled"`
	Version     string `json:"version"`     // bbr, bbr2, bbr3
	CurrentMode string `json:"current_mode"` // 当前拥塞控制算法
	BBRAvailable bool  `json:"bbr_available"` // 系统是否支持BBR
}

// SystemOptimizer 系统优化器
type SystemOptimizer struct {
	paths ConfigPaths
}

// NewSystemOptimizer 创建系统优化器
func NewSystemOptimizer() *SystemOptimizer {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	return &SystemOptimizer{
		paths: paths,
	}
}

// EnableBBR 启用BBR加速
func (so *SystemOptimizer) EnableBBR(version string) error {
	logger.Info("[系统优化] 启用BBR加速", "version", version)

	// 检查系统是否支持BBR
	bbrState, err := so.GetBBRState()
	if err != nil {
		return fmt.Errorf("检查BBR状态失败: %w", err)
	}

	if !bbrState.BBRAvailable {
		return fmt.Errorf("系统不支持BBR")
	}

	// 设置BBR版本
	bbrVersion := version
	if bbrVersion == "" {
		bbrVersion = "bbr" // 默认使用BBR
	}

	// 修改内核参数
	kernelParams := map[string]string{
		"net.core.default_qdisc": "fq",
		"net.ipv4.tcp_congestion_control": bbrVersion,
	}

	for param, value := range kernelParams {
		if err := so.setKernelParameter(param, value); err != nil {
			return fmt.Errorf("设置内核参数 %s 失败: %w", param, err)
		}
	}

	// 保存到系统配置文件以持久化
	if err := so.persistBBRSettings(bbrVersion); err != nil {
		logger.Warn("[系统优化] 持久化BBR设置失败", "error", err)
	}

	logger.Info("[系统优化] BBR加速启用成功", "version", bbrVersion)
	return nil
}

// DisableBBR 禁用BBR加速
func (so *SystemOptimizer) DisableBBR() error {
	logger.Info("[系统优化] 禁用BBR加速")

	// 恢复默认拥塞控制算法
	if err := so.setKernelParameter("net.ipv4.tcp_congestion_control", "cubic"); err != nil {
		return fmt.Errorf("恢复默认拥塞控制算法失败: %w", err)
	}

	// 移除持久化配置
	so.removeBBRPersistence()

	logger.Info("[系统优化] BBR加速已禁用")
	return nil
}

// GetBBRState 获取BBR状态
func (so *SystemOptimizer) GetBBRState() (*BBRState, error) {
	state := &BBRState{
		Enabled:      false,
		Version:      "",
		CurrentMode:  "",
		BBRAvailable: false,
	}

	// 检查可用的拥塞控制算法
	availableAlgos, err := so.getAvailableCongestionControlAlgorithms()
	if err != nil {
		logger.Warn("[系统优化] 获取可用算法失败", "error", err)
		return state, nil
	}

	// 检查是否支持BBR
	for _, algo := range availableAlgos {
		if strings.Contains(strings.ToLower(algo), "bbr") {
			state.BBRAvailable = true
			break
		}
	}

	// 获取当前拥塞控制算法
	currentAlgo, err := so.getCurrentCongestionControlAlgorithm()
	if err != nil {
		logger.Warn("[系统优化] 获取当前算法失败", "error", err)
		return state, nil
	}

	state.CurrentMode = currentAlgo

	// 检查是否启用了BBR
	if strings.Contains(strings.ToLower(currentAlgo), "bbr") {
		state.Enabled = true
		state.Version = currentAlgo
	}

	return state, nil
}

// OptimizeSystemSettings 优化系统设置
func (so *SystemOptimizer) OptimizeSystemSettings() error {
	logger.Info("[系统优化] 开始优化系统设置")

	optimizations := []struct {
		name string
		param string
		value string
	}{
		{"增加文件描述符限制", "fs.file-max", "2097152"},
		{"优化TCP缓冲区", "net.core.rmem_max", "16777216"},
		{"优化TCP缓冲区", "net.core.wmem_max", "16777216"},
		{"优化TCP缓冲区", "net.ipv4.tcp_rmem", "4096 87380 16777216"},
		{"优化TCP缓冲区", "net.ipv4.tcp_wmem", "4096 65536 16777216"},
		{"启用TCP Fast Open", "net.ipv4.tcp_fastopen", "3"},
		{"优化TCP超时", "net.ipv4.tcp_fin_timeout", "30"},
		{"启用TCP时间戳", "net.ipv4.tcp_timestamps", "1"},
		{"启用TCP窗口缩放", "net.ipv4.tcp_window_scaling", "1"},
		{"优化TCP保活", "net.ipv4.tcp_keepalive_time", "1200"},
		{"优化TCP保活", "net.ipv4.tcp_keepalive_intvl", "30"},
		{"优化TCP保活", "net.ipv4.tcp_keepalive_probes", "3"},
		{"启用SYN cookies保护", "net.ipv4.tcp_syncookies", "1"},
		{"优化SYN队列", "net.ipv4.tcp_max_syn_backlog", "8192"},
		{"优化 backlog 队列", "net.core.somaxconn", "1024"},
		{"启用IP转发", "net.ipv4.ip_forward", "1"},
		{"优化连接跟踪", "net.netfilter.nf_conntrack_max", "262144"},
		{"优化连接跟踪超时", "net.netfilter.nf_conntrack_timeout_established", "7200"},
	}

	for _, opt := range optimizations {
		if err := so.setKernelParameter(opt.param, opt.value); err != nil {
			logger.Warn("[系统优化] 设置失败", "name", opt.name, "error", err)
		} else {
			logger.Info("[系统优化] 设置成功", "name", opt.name)
		}
	}

	// 持久化优化设置
	if err := so.persistSystemOptimizations(); err != nil {
		logger.Warn("[系统优化] 持久化优化设置失败", "error", err)
	}

	logger.Info("[系统优化] 系统设置优化完成")
	return nil
}

// CheckNetworkPerformance 检查网络性能
func (so *SystemOptimizer) CheckNetworkPerformance() (map[string]interface{}, error) {
	results := make(map[string]interface{})

	// 检查网络接口状态
	interfaces, err := so.getNetworkInterfaces()
	if err != nil {
		logger.Warn("[系统优化] 获取网络接口失败", "error", err)
	} else {
		results["interfaces"] = interfaces
	}

	// 检查路由表
	routes, err := so.getRoutingTable()
	if err != nil {
		logger.Warn("[系统优化] 获取路由表失败", "error", err)
	} else {
		results["routes"] = routes
	}

	// 检查DNS配置
	dns, err := so.getDNSConfiguration()
	if err != nil {
		logger.Warn("[系统优化] 获取DNS配置失败", "error", err)
	} else {
		results["dns"] = dns
	}

	// 检查当前连接数
	connections, err := so.getActiveConnections()
	if err != nil {
		logger.Warn("[系统优化] 获取活动连接失败", "error", err)
	} else {
		results["active_connections"] = connections
	}

	// 检查BBR状态
	bbrState, err := so.GetBBRState()
	if err != nil {
		logger.Warn("[系统优化] 获取BBR状态失败", "error", err)
	} else {
		results["bbr"] = bbrState
	}

	return results, nil
}

// 内部方法

func (so *SystemOptimizer) setKernelParameter(param, value string) error {
	// 使用sysctl命令设置内核参数
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", param, value))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("设置内核参数失败: %w, output: %s", err, string(output))
	}

	// 验证设置是否生效
	currentValue, err := so.getKernelParameter(param)
	if err != nil {
		return fmt.Errorf("验证参数设置失败: %w", err)
	}

	if currentValue != value {
		return fmt.Errorf("参数设置验证失败: 期望 %s, 实际 %s", value, currentValue)
	}

	return nil
}

func (so *SystemOptimizer) getKernelParameter(param string) (string, error) {
	cmd := exec.Command("sysctl", "-n", param)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("获取内核参数失败: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (so *SystemOptimizer) getAvailableCongestionControlAlgorithms() ([]string, error) {
	// 读取可用算法列表
	data, err := os.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control")
	if err != nil {
		return nil, fmt.Errorf("读取可用算法失败: %w", err)
	}

	// 解析算法列表
	content := strings.TrimSpace(string(data))
	algorithms := strings.Fields(content)

	return algorithms, nil
}

func (so *SystemOptimizer) getCurrentCongestionControlAlgorithm() (string, error) {
	return so.getKernelParameter("net.ipv4.tcp_congestion_control")
}

func (so *SystemOptimizer) persistBBRSettings(version string) error {
	// 持久化BBR设置到sysctl配置文件
	configPath := "/etc/sysctl.d/99-bbr.conf"

	content := fmt.Sprintf("# BBR congestion control\nnet.core.default_qdisc=fq\nnet.ipv4.tcp_congestion_control=%s\n", version)

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入BBR配置文件失败: %w", err)
	}

	// 重新加载sysctl配置
	cmd := exec.Command("sysctl", "--system")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("重新加载sysctl配置失败: %w", err)
	}

	return nil
}

func (so *SystemOptimizer) removeBBRPersistence() error {
	// 删除BBR配置文件
	configPath := "/etc/sysctl.d/99-bbr.conf"
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除BBR配置文件失败: %w", err)
	}

	return nil
}

func (so *SystemOptimizer) persistSystemOptimizations() error {
	// 持久化系统优化设置
	configPath := "/etc/sysctl.d/99-singbox-optimization.conf"

	content := `# Sing-box system optimizations
fs.file-max = 2097152
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_timestamps = 1
net.ipv4.tcp_window_scaling = 1
net.ipv4.tcp_keepalive_time = 1200
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 3
net.ipv4.tcp_syncookies = 1
net.ipv4.tcp_max_syn_backlog = 8192
net.core.somaxconn = 1024
net.ipv4.ip_forward = 1
net.netfilter.nf_conntrack_max = 262144
net.netfilter.nf_conntrack_timeout_established = 7200
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入优化配置文件失败: %w", err)
	}

	return nil
}

func (so *SystemOptimizer) getNetworkInterfaces() ([]map[string]interface{}, error) {
	cmd := exec.Command("ip", "link", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取网络接口失败: %w", err)
	}

	// 解析网络接口信息
	interfaces := make([]map[string]interface{}, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ":") && !strings.HasPrefix(line, " ") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
			 ifaceName := strings.TrimSpace(parts[1])
			 ifaceInfo := map[string]interface{}{
					"name": ifaceName,
					"state": "unknown",
				}

				// 获取接口状态
				if strings.Contains(line, "UP") {
				 ifaceInfo["state"] = "up"
				} else if strings.Contains(line, "DOWN") {
				 ifaceInfo["state"] = "down"
				}

				interfaces = append(interfaces, ifaceInfo)
			}
		}
	}

	return interfaces, nil
}

func (so *SystemOptimizer) getRoutingTable() ([]string, error) {
	cmd := exec.Command("ip", "route", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取路由表失败: %w", err)
	}

	routes := strings.Split(strings.TrimSpace(string(output)), "\n")
	return routes, nil
}

func (so *SystemOptimizer) getDNSConfiguration() ([]string, error) {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("读取DNS配置失败: %w", err)
	}

	dnsServers := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dnsServers = append(dnsServers, parts[1])
			}
		}
	}

	return dnsServers, nil
}

func (so *SystemOptimizer) getActiveConnections() (int, error) {
	cmd := exec.Command("ss", "-tan")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("获取活动连接失败: %w", err)
	}

	// 计算活动连接数
	lines := strings.Split(string(output), "\n")
	// 减去标题行
	connections := len(lines) - 1

	return connections, nil
}

// TestNetworkSpeed 测试网络速度
func (so *SystemOptimizer) TestNetworkSpeed(target string) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	if target == "" {
		target = "www.google.com" // 默认测试目标
	}

	// 测试延迟
	startTime := time.Now()
	cmd := exec.Command("ping", "-c", "4", "-W", "2", target)
	if output, err := cmd.CombinedOutput(); err != nil {
		results["ping_error"] = string(output)
	} else {
		results["ping_output"] = string(output)
		results["ping_duration_ms"] = time.Since(startTime).Milliseconds()
	}

	// 测试DNS解析
	dnsStart := time.Now()
	dnsCmd := exec.Command("nslookup", target)
	if dnsOutput, err := dnsCmd.CombinedOutput(); err != nil {
		results["dns_error"] = string(dnsOutput)
	} else {
		results["dns_output"] = string(dnsOutput)
		results["dns_duration_ms"] = time.Since(dnsStart).Milliseconds()
	}

	return results, nil
}

// GetSystemResourceUsage 获取系统资源使用情况
func (so *SystemOptimizer) GetSystemResourceUsage() (map[string]interface{}, error) {
	usage := make(map[string]interface{})

	// 获取CPU使用率
	cpuUsage, err := so.getCPUUsage()
	if err != nil {
		logger.Warn("[系统优化] 获取CPU使用率失败", "error", err)
	} else {
		usage["cpu"] = cpuUsage
	}

	// 获取内存使用率
	memUsage, err := so.getMemoryUsage()
	if err != nil {
		logger.Warn("[系统优化] 获取内存使用率失败", "error", err)
	} else {
		usage["memory"] = memUsage
	}

	// 获取磁盘使用率
	diskUsage, err := so.getDiskUsage()
	if err != nil {
		logger.Warn("[系统优化] 获取磁盘使用率失败", "error", err)
	} else {
		usage["disk"] = diskUsage
	}

	// 获取网络统计
	netStats, err := so.getNetworkStats()
	if err != nil {
		logger.Warn("[系统优化] 获取网络统计失败", "error", err)
	} else {
		usage["network"] = netStats
	}

	return usage, nil
}

func (so *SystemOptimizer) getCPUUsage() (map[string]interface{}, error) {
	cmd := exec.Command("top", "-bn1", "-d", "1")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取CPU使用率失败: %w", err)
	}

	// 解析top输出获取CPU使用率
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Cpu(s)") || strings.Contains(line, "%Cpu(s)") {
			// 提取CPU使用率信息
			return map[string]interface{}{
				"raw": line,
			}, nil
		}
	}

	return nil, fmt.Errorf("未找到CPU使用率信息")
}

func (so *SystemOptimizer) getMemoryUsage() (map[string]interface{}, error) {
	cmd := exec.Command("free", "-h")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取内存使用率失败: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) >= 2 {
		// 解析内存信息
		memLine := lines[1]
		fields := strings.Fields(memLine)

		if len(fields) >= 4 {
			return map[string]interface{}{
				"total":     fields[1],
				"used":      fields[2],
				"free":      fields[3],
				"raw":       memLine,
			}, nil
		}
	}

	return nil, fmt.Errorf("解析内存信息失败")
}

func (so *SystemOptimizer) getDiskUsage() (map[string]interface{}, error) {
	cmd := exec.Command("df", "-h", "/")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取磁盘使用率失败: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) >= 2 {
		diskLine := lines[1]
		fields := strings.Fields(diskLine)

		if len(fields) >= 5 {
			return map[string]interface{}{
				"total":     fields[1],
				"used":      fields[2],
				"available": fields[3],
				"usage_percent": fields[4],
				"mountpoint": fields[5],
				"raw":       diskLine,
			}, nil
		}
	}

	return nil, fmt.Errorf("解析磁盘信息失败")
}

func (so *SystemOptimizer) getNetworkStats() (map[string]interface{}, error) {
	// 读取网络统计信息
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, fmt.Errorf("读取网络统计失败: %w", err)
	}

	interfaces := make([]map[string]interface{}, 0)
	lines := strings.Split(string(data), "\n")

	for i, line := range lines {
		if i < 2 { // 跳过标题行
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 10 {
		 ifaceName := strings.TrimSuffix(fields[0], ":")
			stats := map[string]interface{}{
				"name":       ifaceName,
				"rx_bytes":   fields[1],
				"rx_packets": fields[2],
				"tx_bytes":   fields[9],
				"tx_packets": fields[10],
			}

			interfaces = append(interfaces, stats)
		}
	}

	return map[string]interface{}{
		"interfaces": interfaces,
	}, nil
}

// CreateSystemReport 生成系统报告
func (so *SystemOptimizer) CreateSystemReport() (map[string]interface{}, error) {
	report := make(map[string]interface{})

	// 系统信息
	report["timestamp"] = time.Now().Format("2006-01-02 15:04:05")
	report["hostname"], _ = os.Hostname()

	// 网络性能检查
	networkPerf, _ := so.CheckNetworkPerformance()
	report["network_performance"] = networkPerf

	// 系统资源使用
	resourceUsage, _ := so.GetSystemResourceUsage()
	report["resource_usage"] = resourceUsage

	// BBR状态
	bbrState, _ := so.GetBBRState()
	report["bbr_status"] = bbrState

	// 系统负载
	loadAvg, _ := so.getLoadAverage()
	report["load_average"] = loadAvg

	// 运行时间
	uptime, _ := so.getUptime()
	report["uptime"] = uptime

	return report, nil
}

func (so *SystemOptimizer) getLoadAverage() (map[string]interface{}, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil, fmt.Errorf("读取平均负载失败: %w", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		return map[string]interface{}{
			"1min":  fields[0],
			"5min":  fields[1],
			"15min": fields[2],
		}, nil
	}

	return nil, fmt.Errorf("解析平均负载失败")
}

func (so *SystemOptimizer) getUptime() (string, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "", fmt.Errorf("读取运行时间失败: %w", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		uptimeSeconds := fields[0]
		return fmt.Sprintf("%s秒", uptimeSeconds), nil
	}

	return "", fmt.Errorf("解析运行时间失败")
}

// GetKernelParameter 获取内核参数
func (so *SystemOptimizer) GetKernelParameter(param string) (string, error) {
	return so.getKernelParameter(param)
}

// SetKernelParameter 设置内核参数
func (so *SystemOptimizer) SetKernelParameter(param, value string) error {
	return so.setKernelParameter(param, value)
}

// GetNetworkInterfaces 获取网络接口
func (so *SystemOptimizer) GetNetworkInterfaces() ([]map[string]interface{}, error) {
	return so.getNetworkInterfaces()
}

// GetRoutingTable 获取路由表
func (so *SystemOptimizer) GetRoutingTable() ([]string, error) {
	return so.getRoutingTable()
}

// GetDNSConfiguration 获取DNS配置
func (so *SystemOptimizer) GetDNSConfiguration() ([]string, error) {
	return so.getDNSConfiguration()
}

// GetActiveConnections 获取活动连接数
func (so *SystemOptimizer) GetActiveConnections() (int, error) {
	return so.getActiveConnections()
}