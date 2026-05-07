package singbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"miaomiaowu/internal/logger"
)

// RuleType 规则类型
type RuleType string

const (
	RuleTypeDomain    RuleType = "domain"
	RuleTypeIP        RuleType = "ip"
	RuleTypeGeoIP     RuleType = "geoip"
	RuleTypeGeoSite   RuleType = "geosite"
	RuleTypeProcess   RuleType = "process"
	RuleTypeUserAgent RuleType = "useragent"
)

// RuleAction 规则动作
type RuleAction string

const (
	ActionRoute  RuleAction = "route"
	ActionBlock  RuleAction = "block"
	ActionDirect RuleAction = "direct"
	ActionProxy  RuleAction = "proxy"
)

// RoutingRule 路由规则
type RoutingRule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        RuleType   `json:"type"`
	Action      RuleAction `json:"action"`
	Raw         string     `json:"raw"`
	Value       string     `json:"value"`
	Outbound    string     `json:"outbound"`
	Enabled     bool       `json:"enabled"`
	Priority    int        `json:"priority"`
	Description string     `json:"description,omitempty"`
}

// RuleSet 规则集
type RuleSet struct {
	Name           string        `json:"name"`
	Type           string        `json:"type"`
	Format         string        `json:"format"`
	URL            string        `json:"url,omitempty"`
	UpdateInterval int           `json:"update_interval,omitempty"`
	Rules          []RoutingRule `json:"rules"`
}

// RoutingManager 路由管理器
type RoutingManager struct {
	paths ConfigPaths
}

// NewRoutingManager 创建路由管理器
func NewRoutingManager() *RoutingManager {
	env := DetectEnvironment()
	paths := GetConfigPaths(env)

	return &RoutingManager{
		paths: paths,
	}
}

// GenerateDefaultRules 生成默认规则
func (rm *RoutingManager) GenerateDefaultRules() []RoutingRule {
	return []RoutingRule{
		{
			ID:       "rule-1",
			Name:     "DNS查询",
			Type:     RuleTypeProcess,
			Action:   ActionDirect,
			Raw:      "process_name:chrome,edge,firefox",
			Value:    "chrome,edge,firefox",
			Outbound: "direct",
			Enabled:  true,
			Priority: 1,
		},
		{
			ID:       "rule-2",
			Name:     "私有地址",
			Type:     RuleTypeIP,
			Action:   ActionDirect,
			Raw:      "geoip:private",
			Value:    "private",
			Outbound: "direct",
			Enabled:  true,
			Priority: 2,
		},
		{
			ID:       "rule-3",
			Name:     "CN直连",
			Type:     RuleTypeGeoIP,
			Action:   ActionDirect,
			Raw:      "geoip:cn",
			Value:    "cn",
			Outbound: "direct",
			Enabled:  true,
			Priority: 3,
		},
		{
			ID:       "rule-4",
			Name:     "广告拦截",
			Type:     RuleTypeDomain,
			Action:   ActionBlock,
			Raw:      "geosite:category-ads-all",
			Value:    "category-ads-all",
			Outbound: "block",
			Enabled:  true,
			Priority: 4,
		},
	}
}

// GenerateRoutingConfig 生成路由配置
func (rm *RoutingManager) GenerateRoutingConfig(rules []RoutingRule) (*RouteConfig, error) {
	config := &RouteConfig{
		Rules:    make([]RouteRule, 0),
		Rule_set: []string{},
		Final:    "proxy",
		Auto:     true,
	}

	// 转换规则
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		routeRule := rm.convertRule(rule)
		config.Rules = append(config.Rules, routeRule)
	}

	// 添加默认规则集
	config.Rule_set = []string{
		"geosite-category-ads-all",
		"geosite-cn",
		"geoip-cn",
		"geoip-private",
	}

	return config, nil
}

// convertRule 转换规则
func (rm *RoutingManager) convertRule(rule RoutingRule) RouteRule {
	routeRule := RouteRule{
		Outbound: rule.Outbound,
	}

	// 解析规则原始格式
	parts := strings.Split(rule.Raw, ":")

	switch len(parts) {
	case 2:
		// 格式: type:value (如 geoip:cn)
		routeRule.Inbound = []string{"any"}
		routeRule.Outbound = rule.Outbound

	case 3:
		// 格式: type:subtype:value (如 geosite:category-ads-all)
		routeRule.Inbound = []string{"any"}
		routeRule.Outbound = rule.Outbound

	default:
		// 默认处理
		routeRule.Inbound = []string{"any"}
		routeRule.Outbound = rule.Outbound
	}

	return routeRule
}

// AddCustomRule 添加自定义规则
func (rm *RoutingManager) AddCustomRule(rule RoutingRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	// 验证规则类型
	if err := validateRuleType(rule.Type); err != nil {
		return err
	}

	// 保存规则到文件
	if err := rm.saveRule(rule); err != nil {
		return err
	}

	logger.Info("[路由管理] 添加自定义规则", "id", rule.ID, "name", rule.Name)
	return nil
}

// RemoveRule 移除规则
func (rm *RoutingManager) RemoveRule(ruleID string) error {
	// 删除规则文件
	ruleFile := filepath.Join(rm.paths.ConfigDir, "rules", ruleID+".json")
	if err := os.Remove(ruleFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove rule file: %w", err)
	}

	logger.Info("[路由管理] 移除规则", "id", ruleID)
	return nil
}

// UpdateRule 更新规则
func (rm *RoutingManager) UpdateRule(ruleID string, updates map[string]interface{}) error {
	// 加载现有规则
	rule, err := rm.loadRule(ruleID)
	if err != nil {
		return err
	}

	// 应用更新
	if name, ok := updates["name"].(string); ok {
		rule.Name = name
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		rule.Enabled = enabled
	}
	if outbound, ok := updates["outbound"].(string); ok {
		rule.Outbound = outbound
	}
	if priority, ok := updates["priority"].(int); ok {
		rule.Priority = priority
	}

	// 保存更新后的规则
	if err := rm.saveRule(rule); err != nil {
		return err
	}

	logger.Info("[路由管理] 更新规则", "id", ruleID)
	return nil
}

// ListRules 列出规则
func (rm *RoutingManager) ListRules() ([]RoutingRule, error) {
	rulesDir := filepath.Join(rm.paths.ConfigDir, "rules")

	// 确保目录存在
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return nil, fmt.Errorf("create rules directory: %w", err)
	}

	// 读取规则文件
	files, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil, fmt.Errorf("read rules directory: %w", err)
	}

	rules := make([]RoutingRule, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		ruleFile := filepath.Join(rulesDir, file.Name())
		rule, err := rm.loadRuleFromFile(ruleFile)
		if err != nil {
			logger.Warn("[路由管理] 加载规则文件失败", "file", file.Name(), "error", err)
			continue
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// saveRule 保存规则
func (rm *RoutingManager) saveRule(rule RoutingRule) error {
	rulesDir := filepath.Join(rm.paths.ConfigDir, "rules")

	// 确保目录存在
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("create rules directory: %w", err)
	}

	// 序列化规则
	data, err := json.MarshalIndent(rule, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rule: %w", err)
	}

	// 写入文件
	ruleFile := filepath.Join(rulesDir, rule.ID+".json")
	if err := os.WriteFile(ruleFile, data, 0644); err != nil {
		return fmt.Errorf("write rule file: %w", err)
	}

	return nil
}

// loadRule 加载规则
func (rm *RoutingManager) loadRule(ruleID string) (RoutingRule, error) {
	ruleFile := filepath.Join(rm.paths.ConfigDir, "rules", ruleID+".json")
	return rm.loadRuleFromFile(ruleFile)
}

// loadRuleFromFile 从文件加载规则
func (rm *RoutingManager) loadRuleFromFile(filePath string) (RoutingRule, error) {
	var rule RoutingRule

	data, err := os.ReadFile(filePath)
	if err != nil {
		return rule, fmt.Errorf("read rule file: %w", err)
	}

	if err := json.Unmarshal(data, &rule); err != nil {
		return rule, fmt.Errorf("unmarshal rule: %w", err)
	}

	return rule, nil
}

// ValidateRule 验证规则
func (rm *RoutingManager) ValidateRule(rule RoutingRule) error {
	// 验证必要字段
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	if rule.Type == "" {
		return fmt.Errorf("rule type is required")
	}

	if rule.Action == "" {
		return fmt.Errorf("rule action is required")
	}

	if rule.Outbound == "" {
		return fmt.Errorf("rule outbound is required")
	}

	// 验证规则格式
	if rule.Raw == "" {
		return fmt.Errorf("rule raw format is required")
	}

	// 验证规则类型
	if err := validateRuleType(rule.Type); err != nil {
		return err
	}

	return nil
}

// validateRuleType 验证规则类型
func validateRuleType(ruleType RuleType) error {
	validTypes := map[RuleType]bool{
		RuleTypeDomain:    true,
		RuleTypeIP:        true,
		RuleTypeGeoIP:     true,
		RuleTypeGeoSite:   true,
		RuleTypeProcess:   true,
		RuleTypeUserAgent: true,
	}

	if !validTypes[ruleType] {
		return fmt.Errorf("invalid rule type: %s", ruleType)
	}

	return nil
}

// GenerateRuleSetConfig 生成规则集配置
func (rm *RoutingManager) GenerateRuleSetConfig(ruleSets []RuleSet) (map[string]interface{}, error) {
	config := make(map[string]interface{})

	for _, ruleSet := range ruleSets {
		setConfig := map[string]interface{}{
			"type":   ruleSet.Type,
			"format": ruleSet.Format,
		}

		if ruleSet.URL != "" {
			setConfig["url"] = ruleSet.URL
		}

		if ruleSet.UpdateInterval > 0 {
			setConfig["download_detour"] = "direct"
		}

		config[ruleSet.Name] = setConfig
	}

	return config, nil
}

// ImportRuleSet 导入规则集
func (rm *RoutingManager) ImportRuleSet(name, url string) error {
	_ = RuleSet{
		Name:   name,
		Type:   "remote",
		Format: "binary",
		URL:    url,
	}

	// 这里实现实际的规则集导入逻辑
	// 包括下载、验证、保存等步骤

	logger.Info("[路由管理] 导入规则集", "name", name, "url", url)
	return nil
}

// ParseDomainSuffixList 解析域名后缀列表
func ParseDomainSuffixList(content string) ([]string, error) {
	lines := strings.Split(content, "\n")
	domains := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 验证域名格式
		if !isValidDomain(line) {
			logger.Warn("[路由管理] 无效域名格式", "domain", line)
			continue
		}

		domains = append(domains, line)
	}

	return domains, nil
}

// ParseCIDRList 解析CIDR列表
func ParseCIDRList(content string) ([]string, error) {
	lines := strings.Split(content, "\n")
	cidrs := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 验证CIDR格式
		if !isValidCIDR(line) {
			logger.Warn("[路由管理] 无效CIDR格式", "cidr", line)
			continue
		}

		cidrs = append(cidrs, line)
	}

	return cidrs, nil
}

// 工具函数

func isValidDomain(domain string) bool {
	if domain == "" {
		return false
	}

	// 简单的域名验证
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}
	}

	return true
}

func isValidCIDR(cidr string) bool {
	// 简单的CIDR验证
	if strings.Contains(cidr, "/") {
		parts := strings.Split(cidr, "/")
		if len(parts) != 2 {
			return false
		}

		ip := parts[0]
		if !isValidIP(ip) {
			return false
		}

		return true
	}

	return isValidIP(cidr)
}

func isValidIP(ip string) bool {
	return isIPv4(ip) || isIPv6(ip)
}
