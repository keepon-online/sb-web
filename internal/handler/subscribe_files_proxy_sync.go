package handler

import (
	"context"
	"fmt"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func (h *subscribeFilesHandler) syncMMWProxyProvidersToFile(subscribeDir, filename string) {
	SyncMMWProxyProvidersToFile(h.repo, subscribeDir, filename)
}

// SyncMMWProxyProvidersToFile 同步 MMW 模式代理集合的节点到指定文件（公共版本）
// 可由 subscription.go 调用，确保获取订阅时包含最新的代理集合节点
func SyncMMWProxyProvidersToFile(repo *storage.TrafficRepository, subscribeDir, filename string) {
	filePath := filepath.Join(subscribeDir, filename)

	// 1. 读取刚保存的 YAML 文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		logger.Info("[MMW同步] 读取文件失败", "error", err)
		return
	}

	// 2. 解析 YAML
	var rootNode yaml.Node
	if err := yaml.Unmarshal(content, &rootNode); err != nil {
		logger.Info("[MMW同步] 解析YAML失败", "error", err)
		return
	}

	// 3. 查找 proxy-groups，收集 use 引用的代理集合名称
	providerNames := collectUsedProviderNames(&rootNode)
	if len(providerNames) == 0 {
		return
	}

	// 获取现有节点数量用于比较
	existingNodes := collectExistingProxyNodes(&rootNode)
	logger.Info("[MMW同步] 文件使用代理集合", "filename", filename, "count", len(providerNames), "providers", providerNames, "existing_nodes", len(existingNodes))

	ctx := context.Background()
	syncedCount := 0

	// 4. 根据名称查找代理集合配置，筛选 MMW 模式
	for _, providerName := range providerNames {
		config, err := repo.GetProxyProviderConfigByName(ctx, providerName)
		if err != nil {
			logger.Info("[MMW同步] 查询代理集合配置失败", "provider_name", providerName, "error", err)
			continue
		}
		if config == nil {
			continue
		}
		if config.ProcessMode != "mmw" {
			continue
		}

		// 5. 从缓存获取节点数据
		cache := GetProxyProviderCache()
		entry, ok := cache.Get(config.ID)
		if !ok || cache.IsExpired(entry) {
			// 缓存不存在或过期，尝试刷新
			sub, err := repo.GetExternalSubscription(ctx, config.ExternalSubscriptionID, config.Username)
			if err != nil || sub.ID == 0 {
				logger.Info("[MMW同步] 获取代理集合的外部订阅失败", "provider_name", providerName, "error", err)
				continue
			}
			entry, err = RefreshProxyProviderCache(&sub, config)
			if err != nil {
				logger.Info("[MMW同步] 刷新代理集合缓存失败", "provider_name", providerName, "error", err)
				continue
			}
		}

		if len(entry.Nodes) == 0 {
			logger.Info("[MMW同步] 代理集合没有节点", "provider_name", providerName)
			continue
		}

		// 6. 为节点添加前缀（使用名称前缀，即第一个 - 之前的部分）
		namePrefix := config.Name
		if idx := strings.Index(config.Name, "-"); idx > 0 {
			namePrefix = config.Name[:idx]
		}
		prefix := fmt.Sprintf("〖%s〗", namePrefix)

		// 复制节点并添加前缀
		proxiesRaw := make([]any, len(entry.Nodes))
		nodeNames := make([]string, 0, len(entry.Nodes))
		for i, node := range entry.Nodes {
			nodeCopy := copyMap(node.(map[string]any))
			if name, ok := nodeCopy["name"].(string); ok {
				newName := prefix + name
				nodeCopy["name"] = newName
				nodeNames = append(nodeNames, newName)
			}
			proxiesRaw[i] = nodeCopy
		}

		// 7. 调用已有的同步函数写入节点
		if err := updateYAMLFileWithProxyProviderNodes(subscribeDir, filename, config.Name, prefix, proxiesRaw, nodeNames); err != nil {
			logger.Info("[MMW同步] 更新文件失败", "filename", filename, "error", err)
			continue
		}

		// 记录同步完成（详细的 old_count/new_count 日志已在 external_sync.go 中输出）
		logger.Info("[MMW同步] 代理集合同步完成", "provider_name", providerName, "node_count", len(nodeNames))

		syncedCount++
	}

	if syncedCount > 0 {
		logger.Info("[MMW同步] 文件同步完成", "filename", filename, "synced_count", syncedCount)
	}
}
