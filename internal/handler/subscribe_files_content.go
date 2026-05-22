package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/validator"

	"gopkg.in/yaml.v3"
)

func (h *subscribeFilesHandler) handleGetContent(w http.ResponseWriter, r *http.Request, filename string) {
	if filename == "" {
		writeBadRequest(w, "文件名不能为空")
		return
	}

	// 验证文件名
	filename, err := url.QueryUnescape(filename)
	if err != nil {
		writeBadRequest(w, "无效的文件名")
		return
	}

	// 检查文件是否存在于数据库
	_, err = h.repo.GetSubscribeFileByFilename(r.Context(), filename)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, errors.New("订阅文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 读取文件内容
	filePath := filepath.Join("subscribes", filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, errors.New("文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, errors.New("读取文件失败"))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"content": string(content),
	})
}

// handleUpdateContent 更新订阅文件内容
func (h *subscribeFilesHandler) handleUpdateContent(w http.ResponseWriter, r *http.Request, filename string) {
	if filename == "" {
		writeBadRequest(w, "文件名不能为空")
		return
	}

	// 验证文件名
	filename, err := url.QueryUnescape(filename)
	if err != nil {
		writeBadRequest(w, "无效的文件名")
		return
	}

	// 检查文件是否存在于数据库
	subscribeFile, err := h.repo.GetSubscribeFileByFilename(r.Context(), filename)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, errors.New("订阅文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 解析请求体
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.Content == "" {
		writeBadRequest(w, "内容不能为空")
		return
	}

	// 验证YAML格式，使用 Node API 保持顺序
	var rootNode yaml.Node
	if err := yaml.Unmarshal([]byte(req.Content), &rootNode); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("内容不是有效的YAML格式: "+err.Error()))
		return
	}

	// 转换为 map 进行基本校验（只检查错误，不做修复）
	var yamlCheck map[string]any
	if err := yaml.Unmarshal([]byte(req.Content), &yamlCheck); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("内容不是有效的YAML格式: "+err.Error()))
		return
	}

	// 校验配置内容
	validationResult := validator.ValidateClashConfig(yamlCheck)
	if !validationResult.Valid {
		logger.Info("[更新订阅文件] [配置校验] 校验失败", "filename", filename)
		var errorMessages []string
		for _, issue := range validationResult.Issues {
			if issue.Level == validator.ErrorLevel {
				errorMsg := issue.Message
				if issue.Location != "" {
					errorMsg = fmt.Sprintf("%s (位��: %s)", errorMsg, issue.Location)
				}
				errorMessages = append(errorMessages, errorMsg)
				logger.Info("[更新订阅文件] [配置校验] 错误", "message", errorMsg)
			}
		}
		writeError(w, http.StatusBadRequest, errors.New("配置校验失败: "+strings.Join(errorMessages, "; ")))
		return
	}

	// 直接保存前端发送的内容（已经过前端修复，保持字段顺序）
	contentToSave := RemoveUnicodeEscapeQuotes(req.Content)

	// 记录警告信息（如果有）
	for _, issue := range validationResult.Issues {
		if issue.Level == validator.WarningLevel {
			logger.Info("[更新订阅文件] [配置校验] 警告(前端已修复)", "message", issue.Message, "location", issue.Location)
		}
	}

	// 保存文件
	filePath := filepath.Join("subscribes", filename)
	if err := os.WriteFile(filePath, []byte(contentToSave), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("保存文件失败"))
		return
	}

	// 保存版本记录
	version, err := h.repo.SaveRuleVersion(r.Context(), filename, contentToSave, "admin")
	if err != nil {
		// 版本保存失败不影响文件保存，只记录错误
		writeError(w, http.StatusInternalServerError, errors.New("保存版本记录失败"))
		return
	}

	// 更新数据库中的updated_at字段
	subscribeFile.UpdatedAt = time.Now()
	_, err = h.repo.UpdateSubscribeFile(r.Context(), subscribeFile)
	if err != nil {
		// 更新时间戳失败不影响文件保存，只记录错误
		writeError(w, http.StatusInternalServerError, errors.New("更新订阅信息失败"))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "updated",
		"version": version,
	})
}

// initializeCustomRuleApplications records the initial custom rule application state for a newly created subscribe file.
// This is called when a file is created from the generator page where custom rules are already included in the content.
