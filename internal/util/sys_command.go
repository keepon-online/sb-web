package util

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// 允许执行的命令白名单
var allowedCommands = map[string]bool{
	"systemctl":    true,
	"service":      true,
	"docker":       true,
	"curl":         true,
	"wget":         true,
	"mkdir":        true,
	"chmod":        true,
	"chown":        true,
	"cp":           true,
	"mv":           true,
	"rm":           true,
	"cat":          true,
	"grep":         true,
	"sed":          true,
	"awk":          true,
	"tail":         true,
	"head":         true,
	"ps":           true,
	"kill":         true,
	"systemctl":    true,
	"journalctl":   true,
	"iptables":     true,
	"ip":           true,
	"nslookup":     true,
	"ping":         true,
	"openssl":      true,
	"tar":          true,
	"unzip":        true,
	"apt-get":      true,
	"yum":          true,
	"dnf":          true,
	"uptime":       true,
	"free":         true,
	"df":           true,
	"uname":        true,
	"id":           true,
	"whoami":       true,
	"date":         true,
	"test":         true,
	"ls":           true,
	"find":         true,
	"stat":         true,
	"basename":     true,
	"dirname":      true,
	"readlink":     true,
	"realpath":     true,
}

// 危险字符模式
var dangerousPatterns = []string{
	";", "&", "|", "$", "`", "$(", "${", ">", "<", "\n", "\r",
}

// CommandResult 表示命令执行结果
type CommandResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
}

// SysCommand 系统命令执行器
type SysCommand struct {
	timeout time.Duration
}

// NewSysCommand 创建系统命令执行器
func NewSysCommand() *SysCommand {
	return &SysCommand{
		timeout: 30 * time.Second,
	}
}

// SetTimeout 设置命令超时时间
func (sc *SysCommand) SetTimeout(timeout time.Duration) {
	sc.timeout = timeout
}

// Execute 执行命令（安全版本）
func (sc *SysCommand) Execute(cmd string, args ...string) (*CommandResult, error) {
	// 验证命令是否在白名单中
	if !isAllowedCommand(cmd) {
		return nil, fmt.Errorf("command '%s' is not allowed", cmd)
	}

	// 验证参数是否包含危险字符
	for i, arg := range args {
		if containsDangerousPatterns(arg) {
			return nil, fmt.Errorf("argument %d contains dangerous patterns: %s", i, arg)
		}
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), sc.timeout)
	defer cancel()

	// 执行命令
	execCmd := exec.CommandContext(ctx, cmd, args...)

	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()

	result := &CommandResult{
		Output: stdout.String(),
		Error:  stderr.String(),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		}
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

// ExecuteWithProgress 执行命令并返回进度（用于长时间运行的操作）
func (sc *SysCommand) ExecuteWithProgress(cmd string, args []string, onProgress func(string)) (*CommandResult, error) {
	// 验证命令和参数
	if !isAllowedCommand(cmd) {
		return nil, fmt.Errorf("command '%s' is not allowed", cmd)
	}

	for i, arg := range args {
		if containsDangerousPatterns(arg) {
			return nil, fmt.Errorf("argument %d contains dangerous patterns: %s", i, arg)
		}
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), sc.timeout)
	defer cancel()

	// 执行命令
	execCmd := exec.CommandContext(ctx, cmd, args...)

	stdout, err := execCmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := execCmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := execCmd.Start(); err != nil {
		return nil, err
	}

	// 读取输出
	result := &CommandResult{}
	buf := make([]byte, 4096)

	// 读取标准输出
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			output := string(buf[:n])
			result.Output += output
			if onProgress != nil {
				onProgress(output)
			}
		}
		if err != nil {
			break
		}
	}

	// 读取标准错误
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			result.Error += string(buf[:n])
		}
		if err != nil {
			break
		}
	}

	// 等待命令完成
	err = execCmd.Wait()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		}
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

// ExecuteCombined 执行命令并合并输出和错误
func (sc *SysCommand) ExecuteCombined(cmd string, args ...string) (string, error) {
	result, err := sc.Execute(cmd, args...)
	if err != nil {
		return result.Output + "\n" + result.Error, err
	}
	return result.Output, nil
}

// isAllowedCommand 检查命令是否在白名单中
func isAllowedCommand(cmd string) bool {
	// 获取基本命令名（不带路径）
	baseCmd := cmd
	if strings.Contains(cmd, "/") {
		parts := strings.Split(cmd, "/")
		baseCmd = parts[len(parts)-1]
	}

	return allowedCommands[baseCmd]
}

// containsDangerousPatterns 检查字符串是否包含危险字符
func containsDangerousPatterns(s string) bool {
	for _, pattern := range dangerousPatterns {
		if strings.Contains(s, pattern) {
			// 允许某些特定情况
			if pattern == ">" && strings.Contains(s, ">>") {
				continue // 允许追加重定向
			}
			if pattern == "&" && strings.Contains(s, "&&") {
				continue // 允许逻辑与
			}
			if pattern == "$" && !strings.Contains(s, "$(") && !strings.Contains(s, "${") {
				continue // 允许单独的 $ 字符
			}
			return true
		}
	}
	return false
}

// ValidatePath 验证路径是否安全
func ValidatePath(path string) error {
	// 检查路径遍历攻击
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains parent directory reference")
	}

	// 检查绝对路径
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "./") {
		return fmt.Errorf("path must be absolute or relative")
	}

	return nil
}

// ValidatePort 验证端口号是否有效
func ValidatePort(port int) error {
	if port < 10000 || port > 65535 {
		return fmt.Errorf("port must be between 10000 and 65535")
	}
	return nil
}

// ValidateDomain 验证域名是否有效
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// 基本域名格式验证
	if strings.Contains(domain, " ") || strings.Contains(domain, "\t") {
		return fmt.Errorf("domain contains whitespace")
	}

	return nil
}

// SanitizeString 清理字符串输入
func SanitizeString(s string) string {
	// 移除危险字符
	result := s
	for _, pattern := range dangerousPatterns {
		result = strings.ReplaceAll(result, pattern, "")
	}
	return strings.TrimSpace(result)
}