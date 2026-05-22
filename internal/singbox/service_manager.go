package singbox

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/systemops"
	"miaomiaowu/internal/util"
)

// ServiceStatus 服务状态
type ServiceStatus struct {
	Running     bool   `json:"running"`      // 是否运行中
	Enabled     bool   `json:"enabled"`      // 是否启用
	PID         int    `json:"pid"`          // 进程ID
	Memory      string `json:"memory"`       // 内存使用
	Uptime      string `json:"uptime"`       // 运行时间
	Version     string `json:"version"`      // 版本信息
	LastRestart string `json:"last_restart"` // 最后重启时间
}

// ServiceManager 服务管理器接口
type ServiceManager interface {
	Install() error
	Start() error
	Stop() error
	Restart() error
	Enable() error
	Disable() error
	Status() (*ServiceStatus, error)
	Logs(lines int) (string, error)
	FollowLogs(ctx context.Context, onLog func(string)) error
}

// SystemdServiceManager systemd 服务管理器
type SystemdServiceManager struct {
	serviceName string
	binPath     string
	configPath  string
	cmdExec     *util.SysCommand // 保留用于 Status 和 Logs 方法
}

// DockerServiceManager Docker 服务管理器
type DockerServiceManager struct {
	containerName string
	cmdExec       *util.SysCommand
}

// NewServiceManager 创建服务管理器
func NewServiceManager(env Environment, paths ConfigPaths) (ServiceManager, error) {
	if env == EnvDocker {
		return &DockerServiceManager{
			containerName: "sing-box",
			cmdExec:       util.NewSysCommand(),
		}, nil
	}

	return &SystemdServiceManager{
		serviceName: "sing-box",
		cmdExec:     util.NewSysCommand(),
		binPath:     filepath.Join(paths.BinDir, "sing-box"),
		configPath:  filepath.Join(paths.ConfigDir, "config.json"),
	}, nil
}

// Systemd 服务管理实现

func (m *SystemdServiceManager) ServiceActionPlan(action systemops.ServiceAction, dryRun bool) (systemops.OperationPlan, error) {
	actionText := string(action)
	switch action {
	case systemops.ServiceActionStart,
		systemops.ServiceActionStop,
		systemops.ServiceActionRestart,
		systemops.ServiceActionEnable,
		systemops.ServiceActionDisable:
	default:
		return systemops.OperationPlan{}, fmt.Errorf("unsupported service action %q", actionText)
	}

	titleAction := strings.ToUpper(actionText[:1]) + actionText[1:]
	title := fmt.Sprintf("%s %s service", titleAction, m.serviceName)

	return systemops.OperationPlan{
		Name:   title,
		DryRun: dryRun,
		Steps: []systemops.OperationStep{
			{
				ID:      fmt.Sprintf("%s-service", actionText),
				Title:   title,
				Kind:    systemops.StepKindService,
				Risk:    systemops.RiskLevelMedium,
				Target:  m.serviceName,
				Command: "systemctl",
				Args:    []string{actionText, m.serviceName},
				Metadata: map[string]string{
					"service_action": actionText,
				},
			},
		},
	}, nil
}

func (m *SystemdServiceManager) InstallPlan(dryRun bool) systemops.OperationPlan {
	servicePath := filepath.Join("/etc/systemd/system", "sing-box.service")

	return systemops.OperationPlan{
		Name:   "Install sing-box systemd service",
		DryRun: dryRun,
		Steps: []systemops.OperationStep{
			{
				ID:     "write-service-file",
				Title:  "Write sing-box systemd service file",
				Kind:   systemops.StepKindFile,
				Risk:   systemops.RiskLevelHigh,
				Target: servicePath,
				Metadata: map[string]string{
					"content": m.serviceFileContent(),
					"mode":    "0644",
				},
			},
			{
				ID:      "reload-systemd",
				Title:   "Reload systemd manager configuration",
				Kind:    systemops.StepKindSystem,
				Risk:    systemops.RiskLevelLow,
				Target:  "systemd",
				Command: "systemctl",
				Args:    []string{"daemon-reload"},
			},
		},
	}
}

func (m *SystemdServiceManager) serviceFileContent() string {
	return fmt.Sprintf(`[Unit]
Description=Sing-box Service
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target

[Service]
Type=simple
User=root
ExecStart=%s run -c %s
Restart=on-failure
RestartSec=5s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`, m.binPath, m.configPath)
}

// Install 安装服务
func (m *SystemdServiceManager) Install() error {
	logger.Info("[服务管理] 安装 systemd 服务")

	plan := m.InstallPlan(false)
	executor := systemops.NewDefaultStepExecutor()

	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		return fmt.Errorf("execute install plan: %w", err)
	}

	for _, step := range result.Steps {
		if step.Error != "" {
			return fmt.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	logger.Info("[服务管理] 服务安装成功")
	return nil
}

// Start 启动服务
func (m *SystemdServiceManager) Start() error {
	logger.Info("[服务管理] 启动服务")

	plan, err := m.ServiceActionPlan(systemops.ServiceActionStart, false)
	if err != nil {
		return fmt.Errorf("create start plan: %w", err)
	}

	executor := systemops.NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		return fmt.Errorf("execute start plan: %w", err)
	}

	for _, step := range result.Steps {
		if step.Error != "" {
			return fmt.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	// 等待服务启动
	time.Sleep(2 * time.Second)

	// 检查服务状态
	status, err := m.Status()
	if err != nil {
		return fmt.Errorf("check status: %w", err)
	}

	if !status.Running {
		return fmt.Errorf("service failed to start")
	}

	logger.Info("[服务管理] 服务启动成功", "pid", status.PID)
	return nil
}

// Stop 停止服务
func (m *SystemdServiceManager) Stop() error {
	logger.Info("[服务管理] 停止服务")

	plan, err := m.ServiceActionPlan(systemops.ServiceActionStop, false)
	if err != nil {
		return fmt.Errorf("create stop plan: %w", err)
	}

	executor := systemops.NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		return fmt.Errorf("execute stop plan: %w", err)
	}

	for _, step := range result.Steps {
		if step.Error != "" {
			return fmt.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	logger.Info("[服务管理] 服务已停止")
	return nil
}

// Restart 重启服务
func (m *SystemdServiceManager) Restart() error {
	logger.Info("[服务管理] 重启服务")

	plan, err := m.ServiceActionPlan(systemops.ServiceActionRestart, false)
	if err != nil {
		return fmt.Errorf("create restart plan: %w", err)
	}

	executor := systemops.NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		return fmt.Errorf("execute restart plan: %w", err)
	}

	for _, step := range result.Steps {
		if step.Error != "" {
			return fmt.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	// 等待服务重启
	time.Sleep(2 * time.Second)

	logger.Info("[服务管理] 服务已重启")
	return nil
}

// Enable 启用服务（开机自启）
func (m *SystemdServiceManager) Enable() error {
	logger.Info("[服务管理] 启用服务开机自启")

	plan, err := m.ServiceActionPlan(systemops.ServiceActionEnable, false)
	if err != nil {
		return fmt.Errorf("create enable plan: %w", err)
	}

	executor := systemops.NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		return fmt.Errorf("execute enable plan: %w", err)
	}

	for _, step := range result.Steps {
		if step.Error != "" {
			return fmt.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	logger.Info("[服务管理] 服务已启用开机自启")
	return nil
}

// Disable 禁用服务
func (m *SystemdServiceManager) Disable() error {
	logger.Info("[服务管理] 禁用服务开机自启")

	plan, err := m.ServiceActionPlan(systemops.ServiceActionDisable, false)
	if err != nil {
		return fmt.Errorf("create disable plan: %w", err)
	}

	executor := systemops.NewDefaultStepExecutor()
	result, err := plan.Execute(context.Background(), executor)
	if err != nil {
		return fmt.Errorf("execute disable plan: %w", err)
	}

	for _, step := range result.Steps {
		if step.Error != "" {
			return fmt.Errorf("step %s failed: %s", step.ID, step.Error)
		}
	}

	logger.Info("[服务管理] 服务已禁用开机自启")
	return nil
}

// Status 获取服务状态
func (m *SystemdServiceManager) Status() (*ServiceStatus, error) {
	status := &ServiceStatus{}

	// 检查服务是否启用
	enabledResult, err := m.cmdExec.Execute("systemctl", "is-enabled", m.serviceName)
	if err == nil && strings.Contains(enabledResult.Output, "enabled") {
		status.Enabled = true
	}

	// 检查服务是否运行
	activeResult, err := m.cmdExec.Execute("systemctl", "is-active", m.serviceName)
	if err == nil && strings.Contains(activeResult.Output, "active") {
		status.Running = true
	}

	// 获取详细状态
	showResult, err := m.cmdExec.Execute("systemctl", "show", m.serviceName,
		"--property=MainPID,ExecStart,ActiveEnterTimestamp")
	if err == nil {
		lines := strings.Split(showResult.Output, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MainPID=") {
				fmt.Sscanf(line, "MainPID=%d", &status.PID)
			} else if strings.HasPrefix(line, "ActiveEnterTimestamp=") {
				timestamp := strings.TrimPrefix(line, "ActiveEnterTimestamp=")
				status.LastRestart = timestamp
			}
		}
	}

	// 获取版本信息
	if status.Running {
		versionResult, err := m.cmdExec.Execute(m.binPath, "version")
		if err == nil {
			status.Version = strings.TrimSpace(versionResult.Output)
		}
	}

	return status, nil
}

// Logs 获取日志
func (m *SystemdServiceManager) Logs(lines int) (string, error) {
	result, err := m.cmdExec.Execute("journalctl", "-u", m.serviceName,
		"-n", fmt.Sprintf("%d", lines), "--no-pager")
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}

	return result.Output, nil
}

// FollowLogs 跟踪日志
func (m *SystemdServiceManager) FollowLogs(ctx context.Context, onLog func(string)) error {
	// 使用 journalctl -f 跟踪日志
	cmd := util.NewSysCommand()
	cmd.SetTimeout(0) // 无超时

	resultChan := make(chan *util.CommandResult)
	errorChan := make(chan error)

	go func() {
		result, err := cmd.ExecuteWithProgress("journalctl",
			[]string{"-u", m.serviceName, "-f", "--no-pager", "-n", "0"},
			func(output string) {
				lines := strings.Split(output, "\n")
				for _, line := range lines {
					if line != "" {
						onLog(line)
					}
				}
			})

		if err != nil {
			errorChan <- err
		} else {
			resultChan <- result
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errorChan:
		return err
	case <-resultChan:
		return nil
	}
}

// Docker 服务管理实现

// Install Docker 环境不需要安装
func (m *DockerServiceManager) Install() error {
	logger.Info("[服务管理] Docker 环境，跳过服务安装")
	return nil
}

// Start 启动容器
func (m *DockerServiceManager) Start() error {
	logger.Info("[服务管理] 启动 Docker 容器")

	result, err := m.cmdExec.Execute("docker", "start", m.containerName)
	if err != nil {
		return fmt.Errorf("start container: %w, output: %s", err, result.Output)
	}

	logger.Info("[服务管理] 容器启动成功")
	return nil
}

// Stop 停止容器
func (m *DockerServiceManager) Stop() error {
	logger.Info("[服务管理] 停止 Docker 容器")

	result, err := m.cmdExec.Execute("docker", "stop", m.containerName)
	if err != nil {
		return fmt.Errorf("stop container: %w, output: %s", err, result.Output)
	}

	logger.Info("[服务管理] 容器已停止")
	return nil
}

// Restart 重启容器
func (m *DockerServiceManager) Restart() error {
	logger.Info("[服务管理] 重启 Docker 容器")

	result, err := m.cmdExec.Execute("docker", "restart", m.containerName)
	if err != nil {
		return fmt.Errorf("restart container: %w, output: %s", err, result.Output)
	}

	logger.Info("[服务管理] 容器已重启")
	return nil
}

// Enable Docker 环境不需要启用
func (m *DockerServiceManager) Enable() error {
	logger.Info("[服务管理] Docker 环境，跳过启用服务")
	return nil
}

// Disable Docker 环境不需要禁用
func (m *DockerServiceManager) Disable() error {
	logger.Info("[服务管理] Docker 环境，跳过禁用服务")
	return nil
}

// Status 获取容器状态
func (m *DockerServiceManager) Status() (*ServiceStatus, error) {
	status := &ServiceStatus{}

	// 检查容器状态
	result, err := m.cmdExec.Execute("docker", "inspect", "--format",
		"{{.State.Status}}", m.containerName)

	if err != nil {
		return status, fmt.Errorf("inspect container: %w", err)
	}

	containerStatus := strings.TrimSpace(result.Output)
	status.Running = containerStatus == "running"

	// 获取 PID
	pidResult, err := m.cmdExec.Execute("docker", "inspect", "--format",
		"{{.State.Pid}}", m.containerName)
	if err == nil {
		fmt.Sscanf(pidResult.Output, "%d", &status.PID)
	}

	return status, nil
}

// Logs 获取容器日志
func (m *DockerServiceManager) Logs(lines int) (string, error) {
	result, err := m.cmdExec.Execute("docker", "logs", "--tail",
		fmt.Sprintf("%d", lines), m.containerName)
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}

	return result.Output, nil
}

// FollowLogs 跟踪容器日志
func (m *DockerServiceManager) FollowLogs(ctx context.Context, onLog func(string)) error {
	cmd := util.NewSysCommand()
	cmd.SetTimeout(0)

	resultChan := make(chan *util.CommandResult)
	errorChan := make(chan error)

	go func() {
		result, err := cmd.ExecuteWithProgress("docker",
			[]string{"logs", "-f", m.containerName},
			func(output string) {
				lines := strings.Split(output, "\n")
				for _, line := range lines {
					if line != "" {
						onLog(line)
					}
				}
			})

		if err != nil {
			errorChan <- err
		} else {
			resultChan <- result
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errorChan:
		return err
	case <-resultChan:
		return nil
	}
}

// GetServiceStatus 获取服务状态（便捷函数）
func GetServiceStatus() (*ServiceStatus, error) {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	manager, err := NewServiceManager(env, paths)
	if err != nil {
		return nil, fmt.Errorf("create service manager: %w", err)
	}

	return manager.Status()
}
