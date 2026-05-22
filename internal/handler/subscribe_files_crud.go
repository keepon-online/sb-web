package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
)

func (h *subscribeFilesHandler) handleList(w http.ResponseWriter, r *http.Request) {
	files, err := h.repo.ListSubscribeFiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"files": h.convertSubscribeFilesWithVersions(r.Context(), files),
	})
}

func (h *subscribeFilesHandler) handleReorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}
	if len(req.IDs) == 0 {
		writeBadRequest(w, "排序列表不能为空")
		return
	}
	if err := h.repo.ReorderSubscribeFiles(r.Context(), req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *subscribeFilesHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req subscribeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.Name == "" {
		writeBadRequest(w, "订阅名称是必填项")
		return
	}
	if req.URL == "" {
		writeBadRequest(w, "链接地址是必填项")
		return
	}
	if req.Type == "" {
		writeBadRequest(w, "类型是必填项")
		return
	}
	if req.Filename == "" {
		writeBadRequest(w, "文件名是必填项")
		return
	}

	file := storage.SubscribeFile{
		Name:        req.Name,
		Description: req.Description,
		URL:         req.URL,
		Type:        req.Type,
		Filename:    req.Filename,
	}

	expireAt, err := parseExpireAt(req.ExpireAt)
	if err != nil {
		writeBadRequest(w, "过期时间格式不正确，需为 RFC3339")
		return
	}
	file.ExpireAt = expireAt

	created, err := h.repo.CreateSubscribeFile(r.Context(), file)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Don't auto-apply custom rules for URL-based subscriptions
	// They will be applied when the subscription is first fetched

	respondJSON(w, http.StatusCreated, map[string]any{
		"file": convertSubscribeFile(created),
	})
}

func (h *subscribeFilesHandler) handleUpdate(w http.ResponseWriter, r *http.Request, idSegment string) {
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的订阅ID")
		return
	}

	existing, err := h.repo.GetSubscribeFileByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var req subscribeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	// 更新字段
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.URL != "" {
		existing.URL = req.URL
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	// Update auto_sync_custom_rules if provided
	wasAutoSyncEnabled := existing.AutoSyncCustomRules
	if req.AutoSyncCustomRules != nil {
		existing.AutoSyncCustomRules = *req.AutoSyncCustomRules
	}
	if req.RawOutput != nil {
		existing.RawOutput = *req.RawOutput
	}
	if req.TrafficLimit != nil {
		existing.TrafficLimit = req.TrafficLimit
	}
	// 更新模板绑定（绑定模板后禁用规则同步）
	templateJustBound := false
	tagsChanged := false
	if req.TemplateFilename != nil {
		existing.TemplateFilename = *req.TemplateFilename
		// 绑定模板后自动禁用规则同步，因为配置将由模板生成
		if *req.TemplateFilename != "" {
			existing.AutoSyncCustomRules = false
			templateJustBound = true
			logger.Info("[订阅更新] 绑定模板，已禁用规则同步", "subscribe_id", existing.ID, "template", *req.TemplateFilename)
		}
	}
	// 更新选中的节点标签
	if req.SelectedTags != nil {
		existing.SelectedTags = req.SelectedTags
		tagsChanged = true
	}
	// 更新自定义短链接码
	if req.CustomShortCode != nil {
		code := strings.TrimSpace(*req.CustomShortCode)
		if code != "" {
			for _, c := range code {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
					writeBadRequest(w, "自定义连接只能包含字母和数字")
					return
				}
			}
			// 同表唯一性：不能与其他订阅的 file_short_code 或 custom_short_code 冲突
			fileCodes, err := h.repo.GetAllFileShortCodes(r.Context())
			if err == nil {
				if fn, exists := fileCodes[code]; exists && fn != existing.Filename {
					writeBadRequest(w, "该自定义连接已被其他订阅使用")
					return
				}
			}
		}
		existing.CustomShortCode = code
		if m := GetSilentModeManager(); m != nil {
			m.InvalidateShortLinkCache()
		}
	}
	if req.ExpireAt != nil {
		if *req.ExpireAt == "" {
			existing.ExpireAt = nil
		} else {
			expireAt, parseErr := parseExpireAt(req.ExpireAt)
			if parseErr != nil {
				writeBadRequest(w, "过期时间格式不正确，需为 RFC3339")
				return
			}
			existing.ExpireAt = expireAt
		}
	}

	// 处理文件名更新
	oldFilename := existing.Filename
	needRenameFile := false
	if req.Filename != "" && req.Filename != existing.Filename {
		// 验证新文件名
		ext := filepath.Ext(req.Filename)
		if ext != ".yaml" && ext != ".yml" {
			writeError(w, http.StatusBadRequest, errors.New("文件名必须以 .yaml 或 .yml 结尾"))
			return
		}

		// 检查新文件名是否已被其他订阅使用
		if existingFile, err := h.repo.GetSubscribeFileByFilename(r.Context(), req.Filename); err == nil && existingFile.ID != id {
			writeError(w, http.StatusConflict, errors.New("文件名已被其他订阅使用"))
			return
		}

		existing.Filename = req.Filename
		needRenameFile = true
	}

	updated, err := h.repo.UpdateSubscribeFile(r.Context(), existing)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// 如果文件名发生变化，重命名物理文件
	if needRenameFile {
		oldPath := filepath.Join("subscribes", oldFilename)
		newPath := filepath.Join("subscribes", req.Filename)

		// 检查旧文件是否存在
		if _, err := os.Stat(oldPath); err == nil {
			// 重命名文件
			if err := os.Rename(oldPath, newPath); err != nil {
				// 重命名失败，回滚数据库更新
				existing.Filename = oldFilename
				_, _ = h.repo.UpdateSubscribeFile(r.Context(), existing)
				writeError(w, http.StatusInternalServerError, errors.New("重命名文件失败: "+err.Error()))
				return
			}
		}
		// 如果旧文件不存在，只更新数据库记录，不报错
	}

	// If auto_sync was just enabled (changed from false to true), trigger immediate sync
	if !wasAutoSyncEnabled && updated.AutoSyncCustomRules {
		go func() {
			addedGroups, err := syncCustomRulesToFile(context.Background(), h.repo, updated)
			if err != nil {
				logger.Info("[AutoSync] 同步自定义规则失败", "filename", updated.Filename, "id", updated.ID, "error", err)
			} else {
				logger.Info("[AutoSync] 同步自定义规则成功", "filename", updated.Filename, "id", updated.ID)
				if len(addedGroups) > 0 {
					logger.Info("[AutoSync] 添加的代理组", "groups", addedGroups)
				}
			}
		}()
	}

	// 如果绑定了V3模板或标签变化，从模板重新生成订阅文件
	if (templateJustBound || tagsChanged) && updated.TemplateFilename != "" {
		go func() {
			ctx := context.Background()
			username := auth.UsernameFromContext(r.Context())
			if username == "" {
				logger.Info("[模板生成] 无法获取用户名，跳过模板生成", "subscribe_id", updated.ID)
				return
			}
			if err := h.regenerateFromTemplate(ctx, username, updated); err != nil {
				logger.Info("[模板生成] 生成失败", "subscribe_id", updated.ID, "template", updated.TemplateFilename, "error", err)
			} else {
				logger.Info("[模板生成] 生成成功", "subscribe_id", updated.ID, "template", updated.TemplateFilename)
			}
		}()
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"file": convertSubscribeFile(updated),
	})
}

func (h *subscribeFilesHandler) handleDelete(w http.ResponseWriter, r *http.Request, idSegment string) {
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的订阅ID")
		return
	}

	// 获取文件信息以便删除物理文件
	file, err := h.repo.GetSubscribeFileByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 删除数据库记录
	if err := h.repo.DeleteSubscribeFile(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 删除物理文件
	filePath := filepath.Join("subscribes", file.Filename)
	_ = os.Remove(filePath) // 忽略错误，即使文件不存在也继续

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// parseFilenameFromContentDisposition 从Content-Disposition头解析文件名
