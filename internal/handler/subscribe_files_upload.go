package handler

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

func (h *subscribeFilesHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	// 解析multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB
		writeBadRequest(w, "解析表单失败")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeBadRequest(w, "文件上传失败")
		return
	}
	defer file.Close()

	// 解析覆盖和原始输出参数
	overwriteIDStr := r.FormValue("overwrite_id")
	rawOutputStr := r.FormValue("raw_output")
	rawOutput := rawOutputStr == "true" || rawOutputStr == "1"

	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("读取文件失败"))
		return
	}

	// 非原始输出模式需要验证YAML格式
	if !rawOutput {
		var yamlCheck map[string]any
		if err := yaml.Unmarshal(content, &yamlCheck); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("文件不是有效的YAML格式"))
			return
		}
	}

	subscribesDir := "subscribes"
	if err := os.MkdirAll(subscribesDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("创建订阅目录失败"))
		return
	}

	// 覆盖模式：替换已有订阅的文件内容
	if overwriteIDStr != "" && overwriteIDStr != "0" {
		overwriteID, parseErr := strconv.ParseInt(overwriteIDStr, 10, 64)
		if parseErr != nil || overwriteID <= 0 {
			writeBadRequest(w, "无效的覆盖订阅ID")
			return
		}

		existing, getErr := h.repo.GetSubscribeFileByID(r.Context(), overwriteID)
		if getErr != nil {
			if errors.Is(getErr, storage.ErrSubscribeFileNotFound) {
				writeError(w, http.StatusNotFound, errors.New("要覆盖的订阅不存在"))
				return
			}
			writeError(w, http.StatusInternalServerError, getErr)
			return
		}

		// 覆写物理文件
		filePath := filepath.Join(subscribesDir, existing.Filename)
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("保存订阅文件失败"))
			return
		}

		// 如果 raw_output 状态变化，更新数据库
		if existing.RawOutput != rawOutput {
			existing.RawOutput = rawOutput
			if _, updateErr := h.repo.UpdateSubscribeFile(r.Context(), existing); updateErr != nil {
				logger.Info("[上传覆盖] 更新 raw_output 失败", "id", overwriteID, "error", updateErr)
			}
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"file": convertSubscribeFile(existing),
		})
		return
	}

	// 新建模式
	name := r.FormValue("name")
	if name == "" {
		name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}

	description := r.FormValue("description")
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	// 非原始输出模式确保文件名有.yaml或.yml扩展名
	if !rawOutput {
		ext := filepath.Ext(filename)
		if ext != ".yaml" && ext != ".yml" {
			filename = filename + ".yaml"
		}
	}

	filePath := filepath.Join(subscribesDir, filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("保存订阅文件失败"))
		return
	}

	subscribeFile := storage.SubscribeFile{
		Name:        name,
		Description: description,
		URL:         "",
		Type:        storage.SubscribeTypeUpload,
		Filename:    filename,
		RawOutput:   rawOutput,
	}

	created, err := h.repo.CreateSubscribeFile(r.Context(), subscribeFile)
	if err != nil {
		_ = os.Remove(filePath)
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"file": convertSubscribeFile(created),
	})
}
