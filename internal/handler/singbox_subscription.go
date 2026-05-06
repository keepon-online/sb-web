package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// SubscriptionCreateRequest 创建订阅请求
type SubscriptionCreateRequest struct {
	Name   string `json:"name"`   // Sing-box配置名称
	Format string `json:"format"` // 订阅格式: clash, v2ray, singbox, base64, json
}

// SubscriptionExportRequest 导出订阅请求
type SubscriptionExportRequest struct {
	SubscriptionID string `json:"subscription_id"`
	Format         string `json:"format"` // clash, v2ray, singbox, base64, json
}

// SubscriptionUpdateRequest 更新订阅请求
type SubscriptionUpdateRequest struct {
	SubscriptionID  string `json:"subscription_id"`
	AutoUpdate      bool   `json:"auto_update"`
	UpdateInterval  int    `json:"update_interval"`
}

// NewSubscriptionGenerateHandler 创建订阅生成处理器
func NewSubscriptionGenerateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req SubscriptionCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		if req.Format == "" {
			req.Format = "clash" // 默认格式
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "subscription_generate", fmt.Sprintf("生成订阅: %s", req.Name))

		sm := singbox.NewSubscriptionManager()
		subscription, err := sm.GenerateSubscriptionFromConfig(req.Name, req.Format)
		if err != nil {
			logger.Error("[订阅API] 生成订阅失败", "error", err)
			logOperationWithError(repo, username, "subscription_generate", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("generate subscription failed: %w", err))
			return
		}

		// 生成订阅URL
		subscriptionURL := sm.GenerateSubscriptionURL(subscription)
		subscription.SubscriptionURL = subscriptionURL

		// 保存更新后的配置
		sm.UpdateSubscription(subscription.ID)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":           "success",
			"message":          "订阅生成成功",
			"subscription":     subscription,
			"subscription_url": subscriptionURL,
		})

		logger.Info("[订阅API] 订阅生成成功", "name", req.Name, "format", req.Format)
	})
}

// NewSubscriptionListHandler 创建订阅列表处理器
func NewSubscriptionListHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		sm := singbox.NewSubscriptionManager()
		subscriptions, err := sm.ListSubscriptions()
		if err != nil {
			logger.Error("[订阅API] 获取订阅列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"subscriptions": subscriptions,
			"count":         len(subscriptions),
		})
	})
}

// NewSubscriptionDetailHandler 创建订阅详情处理器
func NewSubscriptionDetailHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取订阅ID
		subscriptionID := r.URL.Query().Get("subscription_id")
		if subscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id parameter is required"))
			return
		}

		sm := singbox.NewSubscriptionManager()
		subscriptions, err := sm.ListSubscriptions()
		if err != nil {
			logger.Error("[订阅API] 获取订阅详情失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get subscription detail failed: %w", err))
			return
		}

		// 查找指定的订阅
		for _, sub := range subscriptions {
			if sub.ID == subscriptionID {
				writeJSON(w, http.StatusOK, sub)
				return
			}
		}

		writeError(w, http.StatusNotFound, errors.New("subscription not found"))
	})
}

// NewSubscriptionExportHandler 创建订阅导出处理器
func NewSubscriptionExportHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST and GET are supported"))
			return
		}

		var subscriptionID, format string

		if r.Method == http.MethodPost {
			// 解析请求
			var req SubscriptionExportRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
				return
			}
			subscriptionID = req.SubscriptionID
			format = req.Format
		} else {
			// 从URL参数获取
			subscriptionID = r.URL.Query().Get("subscription_id")
			format = r.URL.Query().Get("format")
		}

		// 验证参数
		if subscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}
		if format == "" {
			format = "clash" // 默认格式
		}

		sm := singbox.NewSubscriptionManager()
		content, err := sm.ExportSubscription(subscriptionID, format)
		if err != nil {
			logger.Error("[订阅API] 导出订阅失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("export subscription failed: %w", err))
			return
		}

		// 设置响应内容类型
	switch format {
		case "clash":
			w.Header().Set("Content-Type", "text/yaml")
		case "json":
			w.Header().Set("Content-Type", "application/json")
		default:
			w.Header().Set("Content-Type", "text/plain")
		}

		// 返回订阅内容
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))

		logger.Info("[订阅API] 订阅导出成功", "subscription_id", subscriptionID, "format", format)
	})
}

// NewSubscriptionUpdateHandler 创建订阅更新处理器
func NewSubscriptionUpdateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req SubscriptionUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "subscription_update", fmt.Sprintf("更新订阅: %s", req.SubscriptionID))

		sm := singbox.NewSubscriptionManager()
		if err := sm.UpdateSubscription(req.SubscriptionID); err != nil {
			logger.Error("[订阅API] 更新订阅失败", "error", err)
			logOperationWithError(repo, username, "subscription_update", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("update subscription failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "订阅更新成功",
		})

		logger.Info("[订阅API] 订阅更新成功", "subscription_id", req.SubscriptionID)
	})
}

// NewSubscriptionDeleteHandler 创建订阅删除处理器
func NewSubscriptionDeleteHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only DELETE or POST is supported"))
			return
		}

		// 从URL获取订阅ID
		subscriptionID := r.URL.Query().Get("subscription_id")
		if subscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id parameter is required"))
			return
		}

		// 记录操作日志
		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "subscription_delete", fmt.Sprintf("删除订阅: %s", subscriptionID))

		sm := singbox.NewSubscriptionManager()
		if err := sm.DeleteSubscription(subscriptionID); err != nil {
			logger.Error("[订阅API] 删除订阅失败", "error", err)
			logOperationWithError(repo, username, "subscription_delete", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("delete subscription failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "订阅删除成功",
		})

		logger.Info("[订阅API] 订阅删除成功", "subscription_id", subscriptionID)
	})
}

// NewSubscriptionURLHandler 创建订阅URL生成处理器
func NewSubscriptionURLHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取订阅ID
		subscriptionID := r.URL.Query().Get("subscription_id")
		if subscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id parameter is required"))
			return
		}

		sm := singbox.NewSubscriptionManager()
		subscriptions, err := sm.ListSubscriptions()
		if err != nil {
			logger.Error("[订阅API] 获取订阅列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get subscriptions failed: %w", err))
			return
		}

		// 查找指定的订阅
		for _, sub := range subscriptions {
			if sub.ID == subscriptionID {
				subscriptionURL := sm.GenerateSubscriptionURL(sub)
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"subscription_id":  subscriptionID,
					"subscription_url": subscriptionURL,
					"share_code":      sub.ShareCode,
					"user_code":       sub.UserCode,
				})
				return
			}
		}

		writeError(w, http.StatusNotFound, errors.New("subscription not found"))
	})
}

// NewNodeLinkGenerateHandler 创建节点链接生成处理器
func NewNodeLinkGenerateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			SubscriptionID string `json:"subscription_id"`
			NodeName        string `json:"node_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}

		sm := singbox.NewSubscriptionManager()
		subscriptions, err := sm.ListSubscriptions()
		if err != nil {
			logger.Error("[订阅API] 获取订阅列表失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get subscriptions failed: %w", err))
			return
		}

		// 查找指定的订阅和节点
		for _, sub := range subscriptions {
			if sub.ID == req.SubscriptionID {
				for _, node := range sub.Nodes {
					if req.NodeName == "" || node.Name == req.NodeName {
						link := sm.GenerateNodeLink(node)
						if link != "" {
							writeJSON(w, http.StatusOK, map[string]interface{}{
								"node_name": node.Name,
								"node_type": node.Type,
								"link":      link,
							})
							return
						}
					}
				}
				writeError(w, http.StatusNotFound, errors.New("node not found"))
				return
			}
		}

		writeError(w, http.StatusNotFound, errors.New("subscription not found"))
	})
}

// NewQRCodeGenerateHandler 创建二维码生成处理器
func NewQRCodeGenerateHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// 从URL获取订阅ID
		subscriptionID := r.URL.Query().Get("subscription_id")
		if subscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id parameter is required"))
			return
		}

		sm := singbox.NewSubscriptionManager()
		qrData, err := sm.GenerateQRCodeData(subscriptionID)
		if err != nil {
			logger.Error("[订阅API] 生成二维码数据失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("generate QR code data failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"subscription_id": subscriptionID,
			"qr_data":        qrData,
		})
	})
}

// NewSubscriptionEncryptHandler 创建订阅加密处理器
func NewSubscriptionEncryptHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			SubscriptionID string `json:"subscription_id"`
			Format         string `json:"format"`
			EncryptionKey  string `json:"encryption_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.SubscriptionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("subscription_id is required"))
			return
		}
		if req.EncryptionKey == "" {
			writeError(w, http.StatusBadRequest, errors.New("encryption_key is required"))
			return
		}
		if req.Format == "" {
			req.Format = "base64" // 默认格式
		}

		sm := singbox.NewSubscriptionManager()
		content, err := sm.ExportSubscription(req.SubscriptionID, req.Format)
		if err != nil {
			logger.Error("[订阅API] 导出订阅失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("export subscription failed: %w", err))
			return
		}

		// 加密内容
		encryptedContent, err := sm.EncryptContent(content, req.EncryptionKey)
		if err != nil {
			logger.Error("[订阅API] 加密内容失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("encrypt content failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"subscription_id":    req.SubscriptionID,
			"format":             req.Format,
			"encrypted_content":  encryptedContent,
		})

		logger.Info("[订阅API] 订阅加密成功", "subscription_id", req.SubscriptionID)
	})
}

// NewSubscriptionDecryptHandler 创建订阅解密处理器
func NewSubscriptionDecryptHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// 解析请求
		var req struct {
			EncryptedContent string `json:"encrypted_content"`
			EncryptionKey    string `json:"encryption_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}

		// 验证参数
		if req.EncryptedContent == "" {
			writeError(w, http.StatusBadRequest, errors.New("encrypted_content is required"))
			return
		}
		if req.EncryptionKey == "" {
			writeError(w, http.StatusBadRequest, errors.New("encryption_key is required"))
			return
		}

		sm := singbox.NewSubscriptionManager()
		decryptedContent, err := sm.DecryptContent(req.EncryptedContent, req.EncryptionKey)
		if err != nil {
			logger.Error("[订阅API] 解密内容失败", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("decrypt content failed: %w", err))
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"decrypted_content": decryptedContent,
		})

		logger.Info("[订阅API] 订阅解密成功")
	})
}