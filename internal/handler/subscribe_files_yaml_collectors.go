package handler

import "gopkg.in/yaml.v3"

func collectExistingProxyNodes(rootNode *yaml.Node) []string {
	nodeNames := make([]string, 0)

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return nodeNames
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return nodeNames
	}

	// 查找 proxies 节点
	var proxiesNode *yaml.Node
	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxies" {
			proxiesNode = valueNode
			break
		}
	}

	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return nodeNames
	}

	// 遍历 proxies，收集 name 字段
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i < len(proxyNode.Content)-1; i += 2 {
			keyNode := proxyNode.Content[i]
			valueNode := proxyNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" && valueNode.Kind == yaml.ScalarNode {
				nodeNames = append(nodeNames, valueNode.Value)
				break
			}
		}
	}

	return nodeNames
}

// collectUsedProviderNames 从 YAML 中收集所有 proxy-groups 的代理集合引用
// 支持两种模式：
// 1. 客户端模式：从 use 字段收集 provider 名称
// 2. 妙妙屋模式：从 proxy-group 的 name 字段收集（MMW模式下 proxy-group 名称与代理集合名称相同）
func collectUsedProviderNames(rootNode *yaml.Node) []string {
	providerNames := make([]string, 0)
	seen := make(map[string]bool)

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return providerNames
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return providerNames
	}

	// 查找 proxy-groups 节点
	var proxyGroupsNode *yaml.Node
	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxy-groups" {
			proxyGroupsNode = valueNode
			break
		}
	}

	if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
		return providerNames
	}

	// 遍历 proxy-groups
	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}

		var groupName string
		var hasUse bool

		for i := 0; i < len(groupNode.Content)-1; i += 2 {
			keyNode := groupNode.Content[i]
			valueNode := groupNode.Content[i+1]

			if keyNode.Kind == yaml.ScalarNode {
				switch keyNode.Value {
				case "name":
					if valueNode.Kind == yaml.ScalarNode {
						groupName = valueNode.Value
					}
				case "use":
					hasUse = true
					// 客户端模式：收集 use 字段的值
					if valueNode.Kind == yaml.SequenceNode {
						for _, useItem := range valueNode.Content {
							if useItem.Kind == yaml.ScalarNode && useItem.Value != "" {
								if !seen[useItem.Value] {
									seen[useItem.Value] = true
									providerNames = append(providerNames, useItem.Value)
								}
							}
						}
					}
				}
			}
		}

		// 妙妙屋模式：如果没有 use 字段，使用 proxy-group 的 name
		if !hasUse && groupName != "" && !seen[groupName] {
			seen[groupName] = true
			providerNames = append(providerNames, groupName)
		}
	}

	return providerNames
}

// copyMap 深拷贝 map
func copyMap(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]any:
			result[k] = copyMap(vv)
		case []any:
			newSlice := make([]any, len(vv))
			copy(newSlice, vv)
			result[k] = newSlice
		default:
			result[k] = v
		}
	}
	return result
}
