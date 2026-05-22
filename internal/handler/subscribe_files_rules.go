package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"strings"

	"gopkg.in/yaml.v3"
)

func (h *subscribeFilesHandler) initializeCustomRuleApplications(ctx context.Context, fileID int64) {
	// Get all enabled custom rules to record their current state
	rules, err := h.repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		logger.Info("[Subscribe] 获取自定义规则失败", "error", err)
		return
	}

	if len(rules) == 0 {
		return
	}

	// Record each rule's current state without modifying the file
	for _, rule := range rules {
		// Calculate content hash for tracking future changes
		hash := sha256.Sum256([]byte(rule.Content))
		contentHash := hex.EncodeToString(hash[:])

		// Parse the rule content to extract the actual rules/providers that were applied
		// This must match the format used in applyRulesRule and applyRuleProvidersRule
		var appliedContent string
		if rule.Type == "rules" {
			// Parse rule content to get the array of rules
			var newRules []interface{}

			// Try to parse as map first (with "rules:" key)
			var parsedAsMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedAsMap); err == nil {
				if rulesValue, hasRulesKey := parsedAsMap["rules"]; hasRulesKey {
					if rulesArray, ok := rulesValue.([]interface{}); ok {
						newRules = rulesArray
					}
				}
			}

			// Try to parse as YAML array
			if len(newRules) == 0 {
				if err := yaml.Unmarshal([]byte(rule.Content), &newRules); err != nil {
					// Parse as plain text
					lines := strings.Split(rule.Content, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line != "" && !strings.HasPrefix(line, "#") {
							newRules = append(newRules, line)
						}
					}
				}
			}

			// Serialize to JSON format (same as applyRulesRule does)
			if len(newRules) > 0 {
				appliedJSON, _ := json.Marshal(newRules)
				appliedContent = string(appliedJSON)
			}
		} else if rule.Type == "rule-providers" {
			// Parse rule-providers content
			var parsedContent map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err == nil {
				var providersMap map[string]interface{}
				if providersValue, hasProvidersKey := parsedContent["rule-providers"]; hasProvidersKey {
					if pm, ok := providersValue.(map[string]interface{}); ok {
						providersMap = pm
					}
				} else {
					providersMap = parsedContent
				}

				// Serialize to JSON format
				if len(providersMap) > 0 {
					appliedJSON, _ := json.Marshal(providersMap)
					appliedContent = string(appliedJSON)
				}
			}
		} else if rule.Type == "dns" {
			// For DNS rules, we don't track applied content
			appliedContent = ""
		}

		app := &storage.CustomRuleApplication{
			SubscribeFileID: fileID,
			CustomRuleID:    rule.ID,
			RuleType:        rule.Type,
			RuleMode:        rule.Mode,
			AppliedContent:  appliedContent,
			ContentHash:     contentHash,
		}

		if err := h.repo.UpsertCustomRuleApplication(ctx, app); err != nil {
			logger.Info("[Subscribe] 记录自定义规则应用失败", "rule_id", rule.ID, "error", err)
		}
	}

	logger.Info("[Subscribe] 记录自定义规则应用状态完成", "rule_count", len(rules), "file_id", fileID)
}

// syncMMWProxyProvidersToFile 同步 MMW 模式代理集合的节点到指定文件
// 保存配置文件后调用，将 proxy-groups 中 use 引用的 MMW 模式代理集合节点直接写入配置
