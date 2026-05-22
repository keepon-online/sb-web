package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/substore"
	"os"
	"path/filepath"
)

func (h *subscribeFilesHandler) regenerateFromTemplate(ctx context.Context, username string, subscribeFile storage.SubscribeFile) error {
	if subscribeFile.TemplateFilename == "" {
		return errors.New("订阅未绑定模板")
	}

	// 1. 读取模板文件
	templatePath := filepath.Join("rule_templates", subscribeFile.TemplateFilename)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}
	logger.Info("[模板生成] 读取模板文件", "template", subscribeFile.TemplateFilename, "bytes", len(templateContent))

	// 2. 从节点表获取用户的所有代理节点
	nodes, err := h.repo.ListNodes(ctx, username)
	if err != nil {
		return fmt.Errorf("获取节点列表失败: %w", err)
	}

	// 构建选中标签的 map 用于快速查找
	selectedTagsMap := make(map[string]bool)
	for _, tag := range subscribeFile.SelectedTags {
		selectedTagsMap[tag] = true
	}
	hasTagFilter := len(selectedTagsMap) > 0

	if hasTagFilter {
		logger.Info("[模板生成] 启用标签过滤", "selected_tags", subscribeFile.SelectedTags, "tag_count", len(subscribeFile.SelectedTags))
	}

	// 将节点转换为 proxies 格式（[]map[string]any）
	var proxies []map[string]any
	enabledCount := 0
	filteredByTagCount := 0
	for _, node := range nodes {
		if !node.Enabled {
			continue // 跳过禁用的节点
		}
		enabledCount++
		// 如果设置了标签过滤，只使用选中标签的节点
		if hasTagFilter && !node.HasAnyTag(selectedTagsMap) {
			filteredByTagCount++
			continue
		}
		// ClashConfig 是 JSON 格式的字符串，需要解析
		var proxyConfig map[string]any
		if err := json.Unmarshal([]byte(node.ClashConfig), &proxyConfig); err != nil {
			logger.Info("[模板生成] 解析节点配置失败，跳过", "node", node.NodeName, "error", err)
			continue
		}
		// 确保节点名称正确（使用数据库中的名称）
		proxyConfig["name"] = node.NodeName
		proxies = append(proxies, proxyConfig)
	}
	logger.Info("[模板生成] 从节点表获取代理节点", "total", len(nodes), "enabled", enabledCount, "filtered_by_tag", filteredByTagCount, "used", len(proxies))

	// 3. 从代理集合表获取用户的代理集合配置（用于 proxy-providers）
	providerConfigs, err := h.repo.ListProxyProviderConfigs(ctx, username)
	if err != nil {
		logger.Info("[模板生成] 获取代理集合配置失败", "error", err)
		// 不是致命错误，继续处理
	}

	// 构建 providers map：provider name -> proxy names
	providers := make(map[string][]string)
	providerTagSet := make(map[string]bool)
	for _, config := range providerConfigs {
		providerTagSet[config.Name] = true
	}
	if len(providerTagSet) > 0 {
		for _, node := range nodes {
			if !node.Enabled {
				continue
			}
			for _, t := range node.Tags {
				if providerTagSet[t] {
					providers[t] = append(providers[t], node.NodeName)
				}
			}
		}
	}
	logger.Info("[模板生成] 从代理集合表获取代理集合", "count", len(providerConfigs), "with_nodes", len(providers))

	// 4. 使用 TemplateV3Processor 处理模板
	processor := substore.NewTemplateV3Processor(nil, providers)
	result, err := processor.ProcessTemplate(string(templateContent), proxies)
	if err != nil {
		return fmt.Errorf("处理模板失败: %w", err)
	}

	// 5. 注入代理节点到proxies字段（与预览保持一致）
	result, err = injectProxiesIntoTemplate(result, proxies)
	if err != nil {
		return fmt.Errorf("注入代理节点失败: %w", err)
	}

	// 6. 写入订阅文件
	subscribePath := filepath.Join("subscribes", subscribeFile.Filename)
	if err := os.WriteFile(subscribePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("写入订阅文件失败: %w", err)
	}

	logger.Info("[模板生成] 模板处理完成", "subscribe", subscribeFile.Name, "template", subscribeFile.TemplateFilename, "result_bytes", len(result))
	return nil
}

// RefreshAllTemplateSubscriptions 刷新所有绑定了模板的订阅
// 当节点发生变化（新增、删除、修改）时调用此函数
func RefreshAllTemplateSubscriptions(repo *storage.TrafficRepository, username string) {
	ctx := context.Background()

	// 获取所有绑定了模板的订阅
	files, err := repo.GetSubscribeFilesWithTemplate(ctx)
	if err != nil {
		logger.Info("[模板刷新] 获取绑定模板的订阅失败", "error", err)
		return
	}

	if len(files) == 0 {
		logger.Info("[模板刷新] 没有绑定模板的订阅需要刷新")
		return
	}

	logger.Info("[模板刷新] 开始刷新绑定模板的订阅", "count", len(files))

	// 创建临时 handler 用于调用 regenerateFromTemplate
	h := &subscribeFilesHandler{repo: repo}

	successCount := 0
	for _, file := range files {
		if err := h.regenerateFromTemplate(ctx, username, file); err != nil {
			logger.Info("[模板刷新] 刷新订阅失败", "subscribe", file.Name, "template", file.TemplateFilename, "error", err)
		} else {
			logger.Info("[模板刷新] 刷新订阅成功", "subscribe", file.Name, "template", file.TemplateFilename)
			successCount++
		}
	}

	logger.Info("[模板刷新] 刷新完成", "total", len(files), "success", successCount)
}

// RefreshSubscriptionsByTemplate 刷新绑定了指定模板的订阅
func RefreshSubscriptionsByTemplate(repo *storage.TrafficRepository, username string, templateFilename string) {
	ctx := context.Background()

	files, err := repo.GetSubscribeFilesByTemplate(ctx, templateFilename)
	if err != nil {
		logger.Info("[模板刷新] 获取绑定模板的订阅失败", "template", templateFilename, "error", err)
		return
	}
	if len(files) == 0 {
		return
	}

	logger.Info("[模板刷新] 开始刷新绑定指定模板的订阅", "template", templateFilename, "count", len(files))

	h := &subscribeFilesHandler{repo: repo}
	successCount := 0
	for _, file := range files {
		if err := h.regenerateFromTemplate(ctx, username, file); err != nil {
			logger.Info("[模板刷新] 刷新订阅失败", "subscribe", file.Name, "error", err)
		} else {
			successCount++
		}
	}

	logger.Info("[模板刷新] 刷新完成", "template", templateFilename, "total", len(files), "success", successCount)
}
