package singbox

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/util"
)

const (
	// Sing-box GitHub 仓库
	singboxRepo = "SagerNet/sing-box"
	// Sing-box 版本 API
	singboxAPI = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
	// Sing-box 下载地址模板
	singboxDownloadURL = "https://github.com/SagerNet/sing-box/releases/download/v%s/%s"
)

// InstallProgress 表示安装进度
type InstallProgress struct {
	Step     string `json:"step"`     // 当前步骤
	Progress int    `json:"progress"` // 进度百分比 0-100
	Message  string `json:"message"`  // 进度消息
}

// Installer Sing-box 安装器
type Installer struct {
	env        Environment
	paths      ConfigPaths
	cmdExec    *util.SysCommand
	onProgress func(InstallProgress)
}

// NewInstaller 创建安装器，并确保安装所需目录存在。
func NewInstaller() (*Installer, error) {
	return newInstaller(true)
}

// NewReadOnlyInstaller 创建只读安装器，用于状态查询等不应改动系统目录的场景。
func NewReadOnlyInstaller() *Installer {
	installer, _ := newInstaller(false)
	return installer
}

func newInstaller(ensureDirectories bool) (*Installer, error) {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	if ensureDirectories {
		if err := EnsureDirectories(paths); err != nil {
			return nil, fmt.Errorf("ensure directories: %w", err)
		}
	}

	return &Installer{
		env:     env,
		paths:   paths,
		cmdExec: util.NewSysCommand(),
	}, nil
}

// SetProgressCallback 设置进度回调
func (i *Installer) SetProgressCallback(callback func(InstallProgress)) {
	i.onProgress = callback
}

// reportProgress 报告进度
func (i *Installer) reportProgress(step string, progress int, message string) {
	if i.onProgress != nil {
		i.onProgress(InstallProgress{
			Step:     step,
			Progress: progress,
			Message:  message,
		})
	}
	logger.Info("[Sing-box 安装]", step, progress, message)
}

// Install 安装 Sing-box
func (i *Installer) Install(version string) error {
	logger.Info("[Sing-box 安装] 开始安装", "version", version, "environment", i.env.String())

	// 1. 检查环境
	i.reportProgress("checking", 0, "检查运行环境...")
	if err := i.checkEnvironment(); err != nil {
		return fmt.Errorf("environment check failed: %w", err)
	}

	// 2. 下载 Sing-box
	i.reportProgress("downloading", 10, "下载 Sing-box...")
	downloadPath, err := i.downloadSingbox(version)
	if err != nil {
		return fmt.Errorf("download singbox: %w", err)
	}
	defer os.Remove(downloadPath)

	// 3. 安装二进制文件
	i.reportProgress("installing", 60, "安装二进制文件...")
	if err := i.installBinary(downloadPath); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	// 4. 配置服务
	i.reportProgress("configuring", 80, "配置系统服务...")
	if err := i.configureService(); err != nil {
		return fmt.Errorf("configure service: %w", err)
	}

	// 5. 验证安装
	i.reportProgress("verifying", 90, "验证安装...")
	if err := i.verifyInstallation(); err != nil {
		return fmt.Errorf("verify installation: %w", err)
	}

	i.reportProgress("completed", 100, "安装完成")
	logger.Info("[Sing-box 安装] 安装成功")
	return nil
}

// Uninstall 卸载 Sing-box
func (i *Installer) Uninstall() error {
	logger.Info("[Sing-box 卸载] 开始卸载")

	// 1. 停止服务
	i.reportProgress("stopping", 10, "停止服务...")
	if err := i.stopService(); err != nil {
		logger.Warn("[Sing-box 卸载] 停止服务失败", "error", err)
	}

	// 2. 禁用服务
	i.reportProgress("disabling", 30, "禁用服务...")
	if err := i.disableService(); err != nil {
		logger.Warn("[Sing-box 卸载] 禁用服务失败", "error", err)
	}

	// 3. 删除二进制文件
	i.reportProgress("removing", 50, "删除二进制文件...")
	if err := i.removeBinary(); err != nil {
		return fmt.Errorf("remove binary: %w", err)
	}

	// 4. 清理配置文件（可选）
	i.reportProgress("cleaning", 70, "清理配置文件...")
	if err := i.cleanupConfig(); err != nil {
		logger.Warn("[Sing-box 卸载] 清理配置文件失败", "error", err)
	}

	i.reportProgress("completed", 100, "卸载完成")
	logger.Info("[Sing-box 卸载] 卸载成功")
	return nil
}

// checkEnvironment 检查运行环境
func (i *Installer) checkEnvironment() error {
	// 检查是否为 root
	if !IsRoot() {
		return fmt.Errorf("需要 root 权限进行安装")
	}

	// 检查系统兼容性
	if runtime.GOOS != "linux" {
		return fmt.Errorf("仅支持 Linux 系统")
	}

	// 检查架构支持
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" && arch != "arm" {
		return fmt.Errorf("不支持的架构: %s", arch)
	}

	return nil
}

// downloadSingbox 下载 Sing-box
func (i *Installer) downloadSingbox(version string) (string, error) {
	// 如果未指定版本，获取最新版本
	if version == "" || version == "latest" {
		var err error
		version, err = i.getLatestVersion()
		if err != nil {
			return "", fmt.Errorf("get latest version: %w", err)
		}
	}

	// 构建下载 URL
	arch := i.getArchName()
	osName := runtime.GOOS
	downloadURL := buildSingboxDownloadURL(version, osName, arch)

	logger.Info("[Sing-box 安装] 下载地址", "url", downloadURL)

	// 创建临时文件
	tempFile, err := os.CreateTemp("", "singbox-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tempFile.Close()

	// 下载文件
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// 写入文件
	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	i.reportProgress("downloading", 50, fmt.Sprintf("下载完成: sing-box-%s", version))
	return tempFile.Name(), nil
}

// installBinary 安装二进制文件
func (i *Installer) installBinary(downloadPath string) error {
	// 获取目标路径
	targetPath := filepath.Join(i.paths.BinDir, "sing-box")
	if err := extractSingboxBinaryFromTarGz(downloadPath, targetPath); err != nil {
		return fmt.Errorf("extract sing-box binary: %w", err)
	}

	logger.Info("[Sing-box 安装] 二进制文件已安装", "path", targetPath)
	return nil
}

func buildSingboxReleaseAsset(version, osName, arch string) string {
	return fmt.Sprintf("sing-box-%s-%s-%s.tar.gz", version, osName, arch)
}

func buildSingboxDownloadURL(version, osName, arch string) string {
	return fmt.Sprintf(singboxDownloadURL, version, buildSingboxReleaseAsset(version, osName, arch))
}

func extractSingboxBinaryFromTarGz(archivePath, targetPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if header.FileInfo().IsDir() || filepath.Base(header.Name) != "sing-box" {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("create target dir: %w", err)
		}

		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("create target: %w", err)
		}
		_, copyErr := io.Copy(dst, tr)
		closeErr := dst.Close()
		if copyErr != nil {
			return fmt.Errorf("copy binary: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close target: %w", closeErr)
		}
		return os.Chmod(targetPath, 0755)
	}

	return fmt.Errorf("sing-box binary not found in archive")
}

// configureService 配置系统服务
func (i *Installer) configureService() error {
	if i.env == EnvDocker {
		// Docker 环境不需要配置 systemd 服务
		logger.Info("[Sing-box 安装] Docker 环境跳过服务配置")
		return nil
	}

	// 创建 systemd 服务文件
	serviceContent := fmt.Sprintf(`[Unit]
Description=Sing-box Service
After=network.target

[Service]
Type=simple
User=root
ExecStart=%s run -c %s/config.json
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, filepath.Join(i.paths.BinDir, "sing-box"), i.paths.ConfigDir)

	servicePath := filepath.Join(i.paths.ServiceDir, "sing-box.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// 重新加载 systemd
	result, err := i.cmdExec.Execute("systemctl", "daemon-reload")
	if err != nil {
		logger.Warn("[Sing-box 安装] systemd daemon-reload 失败", "error", err, "output", result.Output)
	}

	logger.Info("[Sing-box 安装] 系统服务已配置", "path", servicePath)
	return nil
}

// verifyInstallation 验证安装
func (i *Installer) verifyInstallation() error {
	// 检查二进制文件是否存在
	binPath := filepath.Join(i.paths.BinDir, "sing-box")
	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found: %w", err)
	}

	// 检查版本
	result, err := i.cmdExec.Execute(binPath, "version")
	if err != nil {
		return fmt.Errorf("execute version command: %w", err)
	}

	logger.Info("[Sing-box 安装] 版本信息", "version", strings.TrimSpace(result.Output))
	return nil
}

// stopService 停止服务
func (i *Installer) stopService() error {
	if i.env == EnvDocker {
		return nil // Docker 环境由容器管理
	}

	_, err := i.cmdExec.Execute("systemctl", "stop", "sing-box")
	if err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	return nil
}

// disableService 禁用服务
func (i *Installer) disableService() error {
	if i.env == EnvDocker {
		return nil // Docker 环境由容器管理
	}

	_, err := i.cmdExec.Execute("systemctl", "disable", "sing-box")
	if err != nil {
		return fmt.Errorf("disable service: %w", err)
	}
	return nil
}

// removeBinary 删除二进制文件
func (i *Installer) removeBinary() error {
	binPath := filepath.Join(i.paths.BinDir, "sing-box")
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove binary: %w", err)
	}

	// 删除服务文件
	if i.env != EnvDocker {
		servicePath := filepath.Join(i.paths.ServiceDir, "sing-box.service")
		if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
			logger.Warn("[Sing-box 卸载] 删除服务文件失败", "error", err)
		}

		// 重新加载 systemd
		i.cmdExec.Execute("systemctl", "daemon-reload")
	}

	return nil
}

// cleanupConfig 清理配置文件
func (i *Installer) cleanupConfig() error {
	// 这里不删除配置文件，保留用户配置
	// 可以根据需要实现
	logger.Info("[Sing-box 卸载] 保留配置文件", "config_dir", i.paths.ConfigDir)
	return nil
}

// getLatestVersion 获取最新版本
func (i *Installer) getLatestVersion() (string, error) {
	resp, err := http.Get(singboxAPI)
	if err != nil {
		return "", fmt.Errorf("get latest version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	// 简单解析：从 API 响应中提取 tag_name
	// 这里简化处理，实际应该完整解析 JSON
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	bodyStr := string(body)
	if strings.Contains(bodyStr, `"tag_name"`) {
		start := strings.Index(bodyStr, `"tag_name":"`) + len(`"tag_name":"`)
		end := strings.Index(bodyStr[start:], `"`)
		if end > 0 {
			version := bodyStr[start : start+end]
			// 移除 'v' 前缀
			version = strings.TrimPrefix(version, "v")
			return version, nil
		}
	}

	return "", fmt.Errorf("parse version from response")
}

// getArchName 获取架构名称
func (i *Installer) getArchName() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armv7"
	default:
		return arch
	}
}

// IsInstalled 检查 Sing-box 是否已安装
func (i *Installer) IsInstalled() bool {
	binPath := filepath.Join(i.paths.BinDir, "sing-box")
	_, err := os.Stat(binPath)
	return err == nil
}

// GetVersion 获取已安装的 Sing-box 版本
func (i *Installer) GetVersion() (string, error) {
	if !i.IsInstalled() {
		return "", fmt.Errorf("sing-box not installed")
	}

	binPath := filepath.Join(i.paths.BinDir, "sing-box")
	result, err := i.cmdExec.Execute(binPath, "version")
	if err != nil {
		return "", fmt.Errorf("get version: %w", err)
	}

	return strings.TrimSpace(result.Output), nil
}
