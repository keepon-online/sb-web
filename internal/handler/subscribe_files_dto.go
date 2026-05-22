package handler

import (
	"context"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
)

type subscribeFileRequest struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	URL                 string   `json:"url"`
	Type                string   `json:"type"`
	Filename            string   `json:"filename"`
	AutoSyncCustomRules *bool    `json:"auto_sync_custom_rules,omitempty"` // Pointer to distinguish between false and not provided
	TemplateFilename    *string  `json:"template_filename,omitempty"`      // 绑定的 V3 模板文件名
	SelectedTags        []string `json:"selected_tags,omitempty"`          // 选中的节点标签
	CustomShortCode     *string  `json:"custom_short_code,omitempty"`      // 自定义短链接码
	ExpireAt            *string  `json:"expire_at,omitempty"`
	RawOutput           *bool    `json:"raw_output,omitempty"` // 非Clash配置，直接输出原始内容
	TrafficLimit        *float64 `json:"traffic_limit,omitempty"`
}

type subscribeFileDTO struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	Type                string     `json:"type"`
	Filename            string     `json:"filename"`
	ExpireAt            *time.Time `json:"expire_at,omitempty"`
	AutoSyncCustomRules bool       `json:"auto_sync_custom_rules"`
	TemplateFilename    string     `json:"template_filename"`
	SelectedTags        []string   `json:"selected_tags"`
	CustomShortCode     string     `json:"custom_short_code"`
	RawOutput           bool       `json:"raw_output"`
	TrafficLimit        *float64   `json:"traffic_limit"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LatestVersion       int64      `json:"latest_version,omitempty"`
}

func convertSubscribeFile(file storage.SubscribeFile) subscribeFileDTO {
	selectedTags := file.SelectedTags
	if selectedTags == nil {
		selectedTags = []string{}
	}
	return subscribeFileDTO{
		ID:                  file.ID,
		Name:                file.Name,
		Description:         file.Description,
		Type:                file.Type,
		Filename:            file.Filename,
		ExpireAt:            file.ExpireAt,
		AutoSyncCustomRules: file.AutoSyncCustomRules,
		TemplateFilename:    file.TemplateFilename,
		SelectedTags:        selectedTags,
		CustomShortCode:     file.CustomShortCode,
		RawOutput:           file.RawOutput,
		TrafficLimit:        file.TrafficLimit,
		CreatedAt:           file.CreatedAt,
		UpdatedAt:           file.UpdatedAt,
	}
}

func convertSubscribeFiles(files []storage.SubscribeFile) []subscribeFileDTO {
	result := make([]subscribeFileDTO, 0, len(files))
	for _, file := range files {
		result = append(result, convertSubscribeFile(file))
	}
	return result
}

func (h *subscribeFilesHandler) convertSubscribeFilesWithVersions(ctx context.Context, files []storage.SubscribeFile) []subscribeFileDTO {
	result := make([]subscribeFileDTO, 0, len(files))
	for _, file := range files {
		dto := convertSubscribeFile(file)

		// 获取最新版本号
		if versions, err := h.repo.ListRuleVersions(ctx, file.Filename, 1); err == nil && len(versions) > 0 {
			dto.LatestVersion = versions[0].Version
		}

		result = append(result, dto)
	}
	return result
}

func parseExpireAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	// Try RFC3339 first (without milliseconds)
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Fallback to RFC3339Nano (with milliseconds/nanoseconds)
		parsed, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, err
		}
	}
	return &parsed, nil
}
