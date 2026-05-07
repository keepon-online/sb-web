package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"miaomiaowu/internal/logger"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

const bytesPerGigabyte = 1073741824.0

type TrafficSummaryHandler struct {
	client *http.Client
	repo   *storage.TrafficRepository
}

type trafficSummaryResponse struct {
	Metrics trafficSummaryMetrics `json:"metrics"`
	History []trafficDailyUsage   `json:"history"`
}

type trafficSummaryMetrics struct {
	TotalLimitGB     float64 `json:"total_limit_gb"`
	TotalUsedGB      float64 `json:"total_used_gb"`
	TotalRemainingGB float64 `json:"total_remaining_gb"`
	UsagePercentage  float64 `json:"usage_percentage"`
}

type trafficDailyUsage struct {
	Date   string  `json:"date"`
	UsedGB float64 `json:"used_gb"`
}

func NewTrafficSummaryHandler(repo *storage.TrafficRepository) *TrafficSummaryHandler {
	if repo == nil {
		panic("traffic summary handler requires repository")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	return newTrafficSummaryHandler(client, repo)
}

func newTrafficSummaryHandler(client *http.Client, repo *storage.TrafficRepository) *TrafficSummaryHandler {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return &TrafficSummaryHandler{client: client, repo: repo}
}

func (h *TrafficSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
		return
	}

	ctx := r.Context()
	username := auth.UsernameFromContext(ctx)

	var totalLimit, totalUsed int64
	if username != "" {
		externalLimit, externalUsed := h.fetchExternalSubscriptionTraffic(ctx, username)
		totalLimit += externalLimit
		totalUsed += externalUsed
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	if err := h.recordSnapshot(ctx, totalLimit, totalUsed, totalRemaining); err != nil {
		logger.Info("[流量] 记录快照失败", "error", err)
	}

	history, err := h.loadHistory(ctx, 30)
	if err != nil {
		logger.Info("[流量] 加载历史记录失败", "error", err)
	}

	response := trafficSummaryResponse{
		Metrics: trafficSummaryMetrics{
			TotalLimitGB:     roundUpTwoDecimals(bytesToGigabytes(totalLimit)),
			TotalUsedGB:      roundUpTwoDecimals(bytesToGigabytes(totalUsed)),
			TotalRemainingGB: roundUpTwoDecimals(bytesToGigabytes(totalRemaining)),
			UsagePercentage:  roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit)),
		},
		History: history,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// RecordDailyUsage fetches the latest external subscription traffic and persists the snapshot.
func (h *TrafficSummaryHandler) RecordDailyUsage(ctx context.Context) error {
	externalLimit, externalUsed := h.syncAndFetchExternalSubscriptionTraffic(ctx)
	totalLimit := externalLimit
	totalUsed := externalUsed
	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	logger.Info("[流量记录] 总计流量",
		"limit_gb", roundUpTwoDecimals(bytesToGigabytes(totalLimit)),
		"used_gb", roundUpTwoDecimals(bytesToGigabytes(totalUsed)),
		"remaining_gb", roundUpTwoDecimals(bytesToGigabytes(totalRemaining)),
		"usage_percent", roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit)))

	if err := h.recordSnapshot(ctx, totalLimit, totalUsed, totalRemaining); err != nil {
		logger.Error("[流量记录] 保存快照到数据库失败", "error", err)
		return err
	}

	logger.Info("[流量记录] 快照已成功保存到数据库")
	return nil
}

// syncAndFetchExternalSubscriptionTraffic syncs traffic info from external subscriptions when sync_traffic is enabled.
func (h *TrafficSummaryHandler) syncAndFetchExternalSubscriptionTraffic(ctx context.Context) (int64, int64) {
	if h.repo == nil {
		return 0, 0
	}

	enabled, err := h.repo.IsSyncTrafficEnabled(ctx)
	if err != nil {
		logger.Warn("[流量记录] 检查sync_traffic设置失败", "error", err)
		return 0, 0
	}

	if !enabled {
		logger.Info("[流量记录] sync_traffic未启用，跳过外部订阅同步")
		return 0, 0
	}

	subs, err := h.repo.ListAllExternalSubscriptions(ctx)
	if err != nil {
		logger.Warn("[流量记录] 获取外部订阅失败", "error", err)
		return 0, 0
	}

	if len(subs) == 0 {
		logger.Info("[Traffic Record] No external subscriptions found")
		return 0, 0
	}

	logger.Info("[流量记录] 同步外部订阅", "count", len(subs))

	var totalLimit, totalUsed int64
	now := time.Now()

	for _, sub := range subs {
		updatedSub, err := h.fetchExternalSubscriptionTrafficInfo(ctx, sub)
		if err != nil {
			logger.Info("[流量记录] 获取订阅流量失败", "name", sub.Name, "error", err)
			updatedSub = sub
		} else if updateErr := h.repo.UpdateExternalSubscription(ctx, updatedSub); updateErr != nil {
			logger.Info("[流量记录] 更新订阅失败", "name", sub.Name, "error", updateErr)
		}

		if updatedSub.Expire != nil && updatedSub.Expire.Before(now) {
			logger.Info("[流量记录] 跳过已过期订阅", "name", updatedSub.Name, "expired_at", updatedSub.Expire.Format("2006-01-02 15:04:05"))
			continue
		}

		if strings.ToLower(strings.TrimSpace(updatedSub.TrafficMode)) == "none" {
			logger.Info("[流量记录] 跳过不统计订阅", "name", updatedSub.Name)
			continue
		}

		used := externalSubscriptionUsedTraffic(updatedSub)
		totalLimit += updatedSub.Total
		totalUsed += used

		logger.Info("[流量记录] 添加订阅流量",
			"name", updatedSub.Name,
			"limit_gb", bytesToGigabytes(updatedSub.Total),
			"used_gb", bytesToGigabytes(used))
	}

	logger.Info("[流量记录] 外部订阅流量总计",
		"limit_gb", bytesToGigabytes(totalLimit),
		"used_gb", bytesToGigabytes(totalUsed))

	return totalLimit, totalUsed
}

func (h *TrafficSummaryHandler) fetchExternalSubscriptionTrafficInfo(ctx context.Context, sub storage.ExternalSubscription) (storage.ExternalSubscription, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
	if err != nil {
		return sub, fmt.Errorf("create request: %w", err)
	}

	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return sub, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return sub, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	userInfo := resp.Header.Get("subscription-userinfo")
	if userInfo == "" {
		return sub, nil
	}

	upload, download, total, expire := ParseTrafficInfoHeader(userInfo)
	sub.Upload = upload
	sub.Download = download
	sub.Total = total
	sub.Expire = expire

	logger.Info("[流量记录] 解析流量信息",
		"name", sub.Name,
		"upload_mb", float64(upload)/(1024*1024),
		"download_mb", float64(download)/(1024*1024),
		"total_gb", float64(total)/(1024*1024*1024))

	return sub, nil
}

func (h *TrafficSummaryHandler) recordSnapshot(ctx context.Context, totalLimit, totalUsed, totalRemaining int64) error {
	if h.repo == nil {
		return nil
	}

	return h.repo.RecordDaily(ctx, time.Now(), totalLimit, totalUsed, totalRemaining)
}

func (h *TrafficSummaryHandler) loadHistory(ctx context.Context, days int) ([]trafficDailyUsage, error) {
	if h.repo == nil {
		return nil, nil
	}

	records, err := h.repo.ListRecent(ctx, days)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Date.Before(records[j].Date)
	})

	usages := make([]trafficDailyUsage, 0, len(records))
	var prevUsed int64
	var hasPrev bool

	for _, record := range records {
		delta := record.TotalUsed
		if hasPrev {
			delta = record.TotalUsed - prevUsed
			if delta < 0 {
				delta = 0
			}
		}

		prevUsed = record.TotalUsed
		hasPrev = true

		usages = append(usages, trafficDailyUsage{
			Date:   record.Date.Format("2006-01-02"),
			UsedGB: roundUpTwoDecimals(bytesToGigabytes(delta)),
		})
	}

	return usages, nil
}

// fetchExternalSubscriptionTraffic fetches traffic from external subscriptions used in subscription files.
func (h *TrafficSummaryHandler) fetchExternalSubscriptionTraffic(ctx context.Context, username string) (int64, int64) {
	if h.repo == nil {
		return 0, 0
	}

	settings, err := h.repo.GetUserSettings(ctx, username)
	if err != nil || !settings.SyncTraffic {
		return 0, 0
	}

	subscribeFiles, err := h.repo.ListSubscribeFiles(ctx)
	if err != nil {
		logger.Info("[流量] 获取订阅文件列表失败", "error", err)
		return 0, 0
	}

	usedExternalURLs := make(map[string]bool)
	for _, file := range subscribeFiles {
		filePath := filepath.Join("subscribes", file.Filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Info("[流量] 读取订阅文件失败", "filename", file.Filename, "error", err)
			continue
		}

		fileURLs, err := GetExternalSubscriptionsFromFile(ctx, data, username, h.repo)
		if err != nil {
			logger.Info("[流量] 解析订阅文件失败", "filename", file.Filename, "error", err)
			continue
		}

		for url := range fileURLs {
			usedExternalURLs[url] = true
		}
	}

	if len(usedExternalURLs) == 0 {
		logger.Info("[流量] 未找到使用中的外部订阅")
		return 0, 0
	}

	subs, err := h.repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		logger.Info("[流量] 获取外部订阅失败", "error", err)
		return 0, 0
	}

	var totalLimit, totalUsed int64
	now := time.Now()

	for _, sub := range subs {
		if !usedExternalURLs[sub.URL] {
			continue
		}
		if sub.Expire != nil && sub.Expire.Before(now) {
			logger.Info("[流量] 跳过已过期订阅", "name", sub.Name, "expired_at", sub.Expire.Format("2006-01-02 15:04:05"))
			continue
		}
		if strings.ToLower(strings.TrimSpace(sub.TrafficMode)) == "none" {
			logger.Info("[流量] 跳过不统计订阅", "name", sub.Name)
			continue
		}

		used := externalSubscriptionUsedTraffic(sub)
		totalLimit += sub.Total
		totalUsed += used
	}

	logger.Info("[流量] 外部订阅流量总计", "limit", totalLimit, "used", totalUsed)
	return totalLimit, totalUsed
}

func externalSubscriptionUsedTraffic(sub storage.ExternalSubscription) int64 {
	switch strings.ToLower(strings.TrimSpace(sub.TrafficMode)) {
	case "download":
		return sub.Download
	case "upload":
		return sub.Upload
	default:
		return sub.Upload + sub.Download
	}
}

func roundUpTwoDecimals(value float64) float64 {
	return math.Ceil(value*100) / 100
}

func bytesToGigabytes(total int64) float64 {
	if total <= 0 {
		return 0
	}

	return float64(total) / bytesPerGigabyte
}

func usagePercentage(used, limit int64) float64 {
	if limit <= 0 {
		return 0
	}

	return (float64(used) / float64(limit)) * 100
}
