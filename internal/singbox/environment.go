package singbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment 表示运行环境类型
type Environment int

const (
	EnvDocker     Environment = iota // Docker 容器环境
	EnvStandalone                    // 独立服务器环境
	EnvUnknown                       // 未知环境
)

// String 返回环境的字符串表示
func (e Environment) String() string {
	switch e {
	case EnvDocker:
		return "docker"
	case EnvStandalone:
		return "standalone"
	case EnvUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ConfigPaths 表示不同环境的配置路径
type ConfigPaths struct {
	BaseDir    string
	ConfigDir  string
	BinDir     string
	ServiceDir string
	LogDir     string
	DataDir    string
}

// SystemInfo 系统信息
type SystemInfo struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Kernel       string `json:"kernel"`
	Hostname     string `json:"hostname"`
	Environment  string `json:"environment"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// DetectEnvironment 检测当前运行环境
func DetectEnvironment() Environment {
	// 检查 Docker 环境标记文件
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return EnvDocker
	}

	// 检查 Docker 环境变量
	if os.Getenv("DOCKER") == "1" {
		return EnvDocker
	}

	// 检查 cgroup 信息中的 docker 标记
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") ||
		   strings.Contains(string(data), "kubepods") ||
		   strings.Contains(string(data), "containerd") {
			return EnvDocker
		}
	}

	// 默认为独立服务器环境
	return EnvStandalone
}

// GetConfigPaths 根据环境类型获取配置路径
func GetConfigPaths(env Environment) ConfigPaths {
	if env == EnvDocker {
		return ConfigPaths{
			BaseDir:    "/app/singbox",
			ConfigDir:  "/app/singbox/config",
			BinDir:     "/app/singbox/bin",
			ServiceDir: "/app/singbox/service",
			LogDir:     "/app/singbox/logs",
			DataDir:    "/app/data/singbox",
		}
	}

	// 独立服务器环境
	return ConfigPaths{
		BaseDir:    "/etc/s-box",
		ConfigDir:  "/etc/s-box",
		BinDir:     "/usr/local/bin",
		ServiceDir: "/etc/systemd/system",
		LogDir:     "/var/log/sing-box",
		DataDir:    "/var/lib/singbox",
	}
}

// EnsureDirectories 确保所有必要的目录存在
func EnsureDirectories(paths ConfigPaths) error {
	dirs := []string{
	 paths.BaseDir,
	 paths.ConfigDir,
	 paths.BinDir,
	 paths.ServiceDir,
	 paths.LogDir,
	 paths.DataDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// GetSystemInfo 获取系统信息
func GetSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{
		Environment: DetectEnvironment().String(),
	}

	// 获取操作系统信息
	if uname, err := getUnameInfo(); err == nil {
		info.Kernel = uname
	}

	// 获取架构信息
	info.Arch = getArch()

	// 获取操作系统类型
	info.OS = getOS()

	// 获取主机名
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}

	// 获取系统能力
	info.Capabilities = getCapabilities()

	return info, nil
}

// getUnameInfo 获取内核信息
func getUnameInfo() (string, error) {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// getArch 获取系统架构
func getArch() string {
	// 读取 /proc/sys/kernel/arch 或者使用 runtime.GOARCH
	data, err := os.ReadFile("/proc/sys/kernel/arch")
	if err != nil {
		// 如果读取失败，返回一个默认值
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// getOS 获取操作系统类型
func getOS() string {
	// 尝试读取 /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		// 如果读取失败，尝试其他方法
		return detectOSFromProc()
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			// 提取引号内的内容
			value := strings.TrimPrefix(line, "PRETTY_NAME=")
			value = strings.Trim(value, "\"")
			return value
		}
	}

	return detectOSFromProc()
}

// detectOSFromProc 从 /proc 检测操作系统
func detectOSFromProc() string {
	// 检查是否有 /etc/redhat-release
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		data, _ := os.ReadFile("/etc/redhat-release")
		return strings.TrimSpace(string(data))
	}

	// 检查是否有 /etc/debian_version
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "Debian GNU/Linux"
	}

	// 检查是否有 /etc/alpine-release
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		data, _ := os.ReadFile("/etc/alpine-release")
		return "Alpine Linux " + strings.TrimSpace(string(data))
	}

	return "Linux"
}

// getCapabilities 获取系统能力
func getCapabilities() []string {
	caps := []string{}

	// 检查是否有 systemd
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		caps = append(caps, "systemd")
	}

	// 检查是否有 Docker
	if DetectEnvironment() == EnvDocker {
		caps = append(caps, "docker")
	}

	// 检查是否有 TUN/TAP 支持
	if _, err := os.Stat("/dev/net/tun"); err == nil {
		caps = append(caps, "tun")
	}

	// 检查是否有 iptables
	if _, err := os.Stat("/sbin/iptables"); err == nil {
		caps = append(caps, "iptables")
	}

	return caps
}

// IsRoot 检查是否以 root 权限运行
func IsRoot() bool {
	return os.Geteuid() == 0
}

// CheckTUNSupport 检查 TUN 支持
func CheckTUNSupport() (bool, error) {
	// 检查 /dev/net/tun 是否存在
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		if os.IsNotExist(err) {
			// 尝试创建 TUN 设备
			if err := os.MkdirAll("/dev/net", 0755); err != nil {
				return false, fmt.Errorf("failed to create /dev/net: %w", err)
			}
			// 这里需要更复杂的逻辑来创建 TUN 设备
			// 简化版本：返回 false
			return false, nil
		}
		return false, err
	}

	// 检查是否可读
	file, err := os.OpenFile("/dev/net/tun", os.O_RDONLY, 0)
	if err != nil {
		return false, err
	}
	file.Close()

	return true, nil
}

// GetExecutablePath 获取可执行文件路径
func GetExecutablePath(name string) (string, error) {
	// 检查 PATH 环境变量
	path := os.Getenv("PATH")
	if path == "" {
		return "", fmt.Errorf("PATH environment variable not set")
	}

	// 遍历 PATH 中的每个目录
	for _, dir := range strings.Split(path, ":") {
		fullPath := filepath.Join(dir, name)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("executable '%s' not found in PATH", name)
}

// ValidateEnvironment 验证环境是否满足要求
func ValidateEnvironment() error {
	env := DetectEnvironment()

	// 检查是否为 root
	if !IsRoot() {
		return fmt.Errorf("this application requires root privileges")
	}

	// 检查 TUN 支持
	if tunSupported, err := CheckTUNSupport(); err != nil {
		return fmt.Errorf("failed to check TUN support: %w", err)
	} else if !tunSupported {
		return fmt.Errorf("TUN device not available")
	}

	// Docker 环境的额外检查
	if env == EnvDocker {
		// 检查是否有必要的挂载点
		requiredMounts := []string{"/app", "/app/data"}
		for _, mount := range requiredMounts {
			if _, err := os.Stat(mount); err != nil {
				return fmt.Errorf("required Docker mount point not found: %s", mount)
			}
		}
	}

	return nil
}

// GetEnvironmentSummary 获取环境摘要信息
func GetEnvironmentSummary() map[string]interface{} {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)
	sysInfo, _ := GetSystemInfo()

	return map[string]interface{}{
		"environment": env.String(),
		"is_root":     IsRoot(),
		"paths":       paths,
		"system_info": sysInfo,
		"validation_error": func() interface{} {
			if err := ValidateEnvironment(); err != nil {
				return err.Error()
			}
			return nil
		}(),
	}
}