package singbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
)

// ShareTarget 分享目标
type ShareTarget string

const (
	ShareTargetGitLab    ShareTarget = "gitlab"
	ShareTargetGitHub    ShareTarget = "github"
	ShareTargetGist      ShareTarget = "gist"
	ShareTargetLocal     ShareTarget = "local"
	ShareTargetPastebin  ShareTarget = "pastebin"
)

// ShareConfig 分享配置
type ShareConfig struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Target       ShareTarget  `json:"target"`
	Enabled      bool         `json:"enabled"`
	AutoShare    bool         `json:"auto_share"`
	ShareInterval int         `json:"share_interval"` // minutes
	LastShared   time.Time    `json:"last_shared"`
	URL          string       `json:"url,omitempty"`        // 分享后的URL
	Token        string       `json:"token,omitempty"`      // 访问令牌
	RepoURL      string       `json:"repo_url,omitempty"`   // 仓库URL
	FilePath     string       `json:"file_path,omitempty"`  // 文件路径
	Branch       string       `json:"branch,omitempty"`     // 分支名
	Message      string       `json:"message,omitempty"`    // 提交消息
	CreatedAt    time.Time    `json:"created_at"`
}

// ShareManager 分享管理器
type ShareManager struct {
	paths      ConfigPaths
	configDir  string
	cacheDir   string
}

// NewShareManager 创建分享管理器
func NewShareManager() *ShareManager {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	configDir := filepath.Join(paths.ConfigDir, "share_configs")
	cacheDir := filepath.Join(paths.ConfigDir, "share_cache")

	return &ShareManager{
		paths:     paths,
		configDir: configDir,
		cacheDir:  cacheDir,
	}
}

// ShareToTarget 分享到目标平台
func (sm *ShareManager) ShareToTarget(config *ShareConfig, content string) (string, error) {
	logger.Info("[分享管理] 开始分享", "target", config.Target, "name", config.Name)

	var url string
	var err error

	switch config.Target {
	case ShareTargetGitLab:
		url, err = sm.shareToGitLab(config, content)
	case ShareTargetGitHub:
		url, err = sm.shareToGitHub(config, content)
	case ShareTargetGist:
		url, err = sm.shareToGist(config, content)
	case ShareTargetLocal:
		url, err = sm.shareToLocal(config, content)
	case ShareTargetPastebin:
		url, err = sm.shareToPastebin(config, content)
	default:
		return "", fmt.Errorf("不支持的分享目标: %s", config.Target)
	}

	if err != nil {
		return "", fmt.Errorf("分享失败: %w", err)
	}

	// 更新配置
	config.URL = url
	config.LastShared = time.Now()
	sm.saveShareConfig(config)

	logger.Info("[分享管理] 分享成功", "target", config.Target, "url", url)
	return url, nil
}

// ShareNodeInfo 分享节点信息
func (sm *ShareManager) ShareNodeInfo(subscriptionID string, target ShareTarget) (*ShareConfig, error) {
	// 加载订阅配置
	subManager := NewSubscriptionManager()
	subscriptions, err := subManager.ListSubscriptions()
	if err != nil {
		return nil, fmt.Errorf("获取订阅列表失败: %w", err)
	}

	var targetSubscription *SubscriptionConfig
	for _, sub := range subscriptions {
		if sub.ID == subscriptionID {
			targetSubscription = sub
			break
		}
	}

	if targetSubscription == nil {
		return nil, fmt.Errorf("订阅不存在: %s", subscriptionID)
	}

	// 生成分享内容
	content, err := sm.generateShareContent(targetSubscription)
	if err != nil {
		return nil, fmt.Errorf("生成分享内容失败: %w", err)
	}

	// 创建分享配置
	shareConfig := &ShareConfig{
		ID:        generateShareID(),
		Name:      fmt.Sprintf("%s-share", targetSubscription.Name),
		Target:    target,
		Enabled:   true,
		AutoShare: false,
		Branch:    "main",
		Message:   fmt.Sprintf("Update subscription: %s", targetSubscription.Name),
		CreatedAt: time.Now(),
	}

	// 执行分享
	url, err := sm.ShareToTarget(shareConfig, content)
	if err != nil {
		return nil, err
	}

	shareConfig.URL = url
	return shareConfig, nil
}

// CreateShareConfig 创建分享配置
func (sm *ShareManager) CreateShareConfig(name string, target ShareTarget, config map[string]string) (*ShareConfig, error) {
	logger.Info("[分享管理] 创建分享配置", "name", name, "target", target)

	shareConfig := &ShareConfig{
		ID:        generateShareID(),
		Name:      name,
		Target:    target,
		Enabled:   true,
		AutoShare: false,
		CreatedAt: time.Now(),
	}

	// 根据目标类型设置配置
	switch target {
	case ShareTargetGitLab, ShareTargetGitHub:
		if token, ok := config["token"]; ok {
			shareConfig.Token = token
		}
		if repoURL, ok := config["repo_url"]; ok {
			shareConfig.RepoURL = repoURL
		}
		if filePath, ok := config["file_path"]; ok {
			shareConfig.FilePath = filePath
		}
		if branch, ok := config["branch"]; ok {
			shareConfig.Branch = branch
		} else {
			shareConfig.Branch = "main"
		}
	case ShareTargetPastebin:
		if token, ok := config["token"]; ok {
			shareConfig.Token = token
		}
	case ShareTargetLocal:
		if filePath, ok := config["file_path"]; ok {
			shareConfig.FilePath = filePath
		}
	}

	// 保存配置
	if err := sm.saveShareConfig(shareConfig); err != nil {
		return nil, fmt.Errorf("保存分享配置失败: %w", err)
	}

	logger.Info("[分享管理] 分享配置创建成功", "name", name)
	return shareConfig, nil
}

// UpdateShareConfig 更新分享配置
func (sm *ShareManager) UpdateShareConfig(shareID string, updates map[string]interface{}) error {
	config, err := sm.loadShareConfig(shareID)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 应用更新
	if enabled, ok := updates["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if autoShare, ok := updates["auto_share"].(bool); ok {
		config.AutoShare = autoShare
	}
	if shareInterval, ok := updates["share_interval"].(int); ok {
		config.ShareInterval = shareInterval
	}
	if token, ok := updates["token"].(string); ok {
		config.Token = token
	}
	if filePath, ok := updates["file_path"].(string); ok {
		config.FilePath = filePath
	}
	if message, ok := updates["message"].(string); ok {
		config.Message = message
	}

	// 保存更新后的配置
	if err := sm.saveShareConfig(config); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("[分享管理] 分享配置更新成功", "share_id", shareID)
	return nil
}

// DeleteShareConfig 删除分享配置
func (sm *ShareManager) DeleteShareConfig(shareID string) error {
	logger.Info("[分享管理] 删除分享配置", "share_id", shareID)

	configPath := filepath.Join(sm.configDir, shareID+".json")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除配置文件失败: %w", err)
	}

	logger.Info("[分享管理] 分享配置删除成功")
	return nil
}

// ListShareConfigs 列出分享配置
func (sm *ShareManager) ListShareConfigs() ([]*ShareConfig, error) {
	if err := os.MkdirAll(sm.configDir, 0755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}

	files, err := os.ReadDir(sm.configDir)
	if err != nil {
		return nil, fmt.Errorf("读取配置目录失败: %w", err)
	}

	configs := make([]*ShareConfig, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		shareID := strings.TrimSuffix(file.Name(), ".json")
		config, err := sm.loadShareConfig(shareID)
		if err != nil {
			logger.Warn("[分享管理] 加载配置失败", "share_id", shareID, "error", err)
			continue
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// 内部方法

func (sm *ShareManager) shareToGitLab(config *ShareConfig, content string) (string, error) {
	if config.Token == "" {
		return "", fmt.Errorf("GitLab token is required")
	}
	if config.RepoURL == "" {
		return "", fmt.Errorf("GitLab repository URL is required")
	}

	// 解析仓库URL
	// 格式: https://gitlab.com/owner/repo
	parts := strings.Split(strings.TrimPrefix(config.RepoURL, "https://gitlab.com/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid GitLab repository URL")
	}

	owner := parts[0]
	repo := parts[1]

	// 设置文件路径
	filePath := config.FilePath
	if filePath == "" {
		filePath = "subscriptions/config.json"
	}

	// 构建API URL
	apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s%%2F%s/repository/files/%s", owner, repo, filePath)

	// 准备请求体
	requestBody := map[string]interface{}{
		"branch":        config.Branch,
		"content":       content,
		"commit_message": config.Message,
	}

	if config.Message == "" {
		requestBody["commit_message"] = fmt.Sprintf("Update subscription: %s", time.Now().Format("2006-01-02 15:04:05"))
	}

	// 检查文件是否存在
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", config.Token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("检查文件失败: %w", err)
	}
	defer resp.Body.Close()

	// 根据文件是否存在决定创建或更新
	method := "POST"
	if resp.StatusCode == http.StatusOK {
		method = "PUT"
	}

	// 发送请求
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err = http.NewRequest(method, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", config.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab API错误: %s, %s", resp.Status, string(body))
	}

	// 返回文件URL
	return fmt.Sprintf("%s/-/raw/%s/%s", config.RepoURL, config.Branch, filePath), nil
}

func (sm *ShareManager) shareToGitHub(config *ShareConfig, content string) (string, error) {
	if config.Token == "" {
		return "", fmt.Errorf("GitHub token is required")
	}
	if config.RepoURL == "" {
		return "", fmt.Errorf("GitHub repository URL is required")
	}

	// 解析仓库URL
	// 格式: https://github.com/owner/repo
	parts := strings.Split(strings.TrimPrefix(config.RepoURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid GitHub repository URL")
	}

	owner := parts[0]
	repo := parts[1]

	// 设置文件路径
	filePath := config.FilePath
	if filePath == "" {
		filePath = "subscriptions/config.json"
	}

	// 构建API URL
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, filePath)

	// 准备请求体
	requestBody := map[string]interface{}{
		"message": config.Message,
		"content": content,
	}

	if config.Message == "" {
		requestBody["message"] = fmt.Sprintf("Update subscription: %s", time.Now().Format("2006-01-02 15:04:05"))
	}

	// 检查文件是否存在
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s"))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("检查文件失败: %w", err)
	}
	defer resp.Body.Close()

	// 如果文件存在，需要提供SHA
	if resp.StatusCode == http.StatusOK {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			if sha, ok := result["sha"].(string); ok {
				requestBody["sha"] = sha
			}
		}
	}

	// 发送请求
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err = http.NewRequest("PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", config.Token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API错误: %s, %s", resp.Status, string(body))
	}

	// 解析响应获取下载URL
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if content, ok := result["content"].(map[string]interface{}); ok {
			if downloadURL, ok := content["download_url"].(string); ok {
				return downloadURL, nil
			}
		}
	}

	return fmt.Sprintf("%s/blob/%s/%s", config.RepoURL, config.Branch, filePath), nil
}

func (sm *ShareManager) shareToGist(config *ShareConfig, content string) (string, error) {
	if config.Token == "" {
		return "", fmt.Errorf("GitHub token is required")
	}

	// 准备请求体
	requestBody := map[string]interface{}{
		"description": config.Message,
		"public":      false,
		"files": map[string]interface{}{
			"subscription.json": map[string]interface{}{
				"content": content,
			},
		},
	}

	if config.Message == "" {
		requestBody["description"] = fmt.Sprintf("Subscription: %s", time.Now().Format("2006-01-02 15:04:05"))
	}

	// 发送请求
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	apiURL := "https://api.github.com/gists"
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", config.Token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub Gist API错误: %s, %s", resp.Status, string(body))
	}

	// 解析响应获取Gist URL
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if htmlURL, ok := result["html_url"].(string); ok {
			return htmlURL, nil
		}
	}

	return "", fmt.Errorf("获取Gist URL失败")
}

func (sm *ShareManager) shareToLocal(config *ShareConfig, content string) (string, error) {
	if config.FilePath == "" {
		return "", fmt.Errorf("local file path is required")
	}

	// 确保目录存在
	dir := filepath.Dir(config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(config.FilePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return config.FilePath, nil
}

func (sm *ShareManager) shareToPastebin(config *ShareConfig, content string) (string, error) {
	// 简化实现，使用常见的pastebin API
	apiURL := "https://pastebin.com/api/api_post.php"

	// 准备表单数据
	formData := map[string]string{
		"api_dev_key":    config.Token,
		"api_option":     "paste",
		"api_paste_code": content,
		"api_paste_name": config.Name,
		"api_paste_expire": "1N", // 1天过期
	}

	// 构建请求体
	body := &bytes.Buffer{}
	for key, value := range formData {
		fmt.Fprintf(body, "%s=%s&", key, value)
	}

	// 发送请求
	resp, err := http.Post(apiURL, "application/x-www-form-urlencoded", body)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if strings.HasPrefix(string(result), "http") {
		return string(result), nil
	}

	return "", fmt.Errorf("Pastebin API错误: %s", string(result))
}

func (sm *ShareManager) generateShareContent(subscription *SubscriptionConfig) (string, error) {
	// 生成JSON格式的分享内容
	content, err := json.MarshalIndent(subscription, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化订阅失败: %w", err)
	}

	return string(content), nil
}

func (sm *ShareManager) saveShareConfig(config *ShareConfig) error {
	if err := os.MkdirAll(sm.configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configPath := filepath.Join(sm.configDir, config.ID+".json")

	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}

	return nil
}

func (sm *ShareManager) loadShareConfig(shareID string) (*ShareConfig, error) {
	configPath := filepath.Join(sm.configDir, shareID+".json")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}

	var config ShareConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	return &config, nil
}

func generateShareID() string {
	return fmt.Sprintf("share-%d", time.Now().UnixNano())
}

// GetShareStatus 获取分享状态
func (sm *ShareManager) GetShareStatus(shareID string) (map[string]interface{}, error) {
	config, err := sm.loadShareConfig(shareID)
	if err != nil {
		return nil, err
	}

	status := map[string]interface{}{
		"id":           config.ID,
		"name":         config.Name,
		"target":       config.Target,
		"enabled":      config.Enabled,
		"auto_share":   config.AutoShare,
		"last_shared":  config.LastShared,
		"url":          config.URL,
		"status":       "active",
	}

	// 检查分享是否过期
	if !config.LastShared.IsZero() && time.Since(config.LastShared) > 24*time.Hour {
		status["status"] = "expired"
	}

	return status, nil
}