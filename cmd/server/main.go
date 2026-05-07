package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/handler"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/proxygroups"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/version"
	"miaomiaowu/internal/web"
	ruletemplates "miaomiaowu/rule_templates"
	"miaomiaowu/subscribes"
)

func main() {
	// 初始化logger
	logger.Init()
	logger.Info("喵喵屋服务器启动中", "version", version.Version)

	// 启动日志清理任务（每天凌晨3点清理7天前的日志）
	go startLogCleanup()

	addr := getAddr()

	repo, err := storage.NewTrafficRepository(filepath.Join("data", "traffic.db"))
	if err != nil {
		logger.Error("流量数据库初始化失败", "error", err)
		os.Exit(1)
	}
	defer repo.Close()

	// 初始化 Sing-box 相关数据表
	if err := repo.InitSingBoxTables(); err != nil {
		logger.Warn("Sing-box 数据表初始化失败（非致命错误）", "error", err)
	}

	authManager, err := auth.NewManager(repo)
	if err != nil {
		logger.Error("认证管理器加载失败", "error", err)
		os.Exit(1)
	}

	tokenStore := auth.NewTokenStore(24 * time.Hour)

	// Load persisted sessions from database
	ctx := context.Background()
	sessions, err := repo.LoadSessions(ctx)
	if err != nil {
		logger.Warn("从数据库加载会话失败", "error", err)
	} else {
		for _, session := range sessions {
			tokenStore.LoadSession(session.Token, session.Username, session.ExpiresAt)
		}
		logger.Info("会话加载完成", "count", len(sessions))
	}

	// Cleanup expired sessions from database
	if err := repo.CleanupExpiredSessions(ctx); err != nil {
		logger.Warn("清理过期会话失败", "error", err)
	}

	subscribeDir := filepath.Join("subscribes")
	if err := subscribes.Ensure(subscribeDir); err != nil {
		logger.Error("订阅文件准备失败", "error", err)
		os.Exit(1)
	}

	ruleTemplatesDir := filepath.Join("rule_templates")
	if err := ruletemplates.Ensure(ruleTemplatesDir); err != nil {
		logger.Error("规则模板文件准备失败", "error", err)
		os.Exit(1)
	}

	// 初始化代理组配置 Store（纯内存存储）
	// 优先从系统配置的远程地址拉取，失败时使用空配置
	var proxyGroupsStore *proxygroups.Store

	// 获取系统配置中的远程地址
	systemConfig, err := repo.GetSystemConfig(ctx)
	if err != nil {
		logger.Warn("加载系统配置失败", "error", err)
	}

	// 从远程拉取配置
	data, resolvedURL, fetchErr := proxygroups.FetchConfig(systemConfig.ProxyGroupsSourceURL)
	if fetchErr != nil {
		logger.Warn("拉取代理组配置失败", "error", fetchErr)
		// 远程拉取失败时使用空配置初始化
		proxyGroupsStore, err = proxygroups.NewStore([]byte("[]"), "empty-fallback")
		if err != nil {
			logger.Error("创建代理组存储失败", "error", err)
			os.Exit(1)
		}
		logger.Info("代理组存储已使用空配置初始化", "reason", "远程拉取失败")
	} else {
		// 远程拉取成功
		proxyGroupsStore, err = proxygroups.NewStore(data, resolvedURL)
		if err != nil {
			logger.Error("代理组配置无效", "source", resolvedURL, "error", err)
			os.Exit(1)
		}
		logger.Info("代理组配置加载成功", "source", resolvedURL)
	}

	syncSubscribeFilesToDatabase(repo, subscribeDir)

	// 启动时初始化代理集合缓存
	go handler.InitProxyProviderCacheOnStartup(repo)

	// 启动代理集合定时同步器
	proxySyncCtx, stopProxySync := context.WithCancel(context.Background())
	go handler.StartProxyProviderCacheSync(proxySyncCtx, repo)

	trafficHandler := handler.NewTrafficSummaryHandler(repo)
	userRepo := auth.NewRepositoryAdapter(repo)
	loginRateLimiter := handler.NewLoginRateLimiter()

	mux := http.NewServeMux()
	mux.Handle("/api/setup/status", handler.NewSetupStatusHandler(repo))
	mux.Handle("/api/setup/init", handler.NewInitialSetupHandler(repo))
	mux.Handle("/api/setup/restore-backup", handler.NewSetupRestoreBackupHandler(repo))
	mux.Handle("/api/login", handler.NewLoginHandler(authManager, tokenStore, repo, loginRateLimiter))

	// Admin-only endpoints
	mux.Handle("/api/admin/credentials", auth.RequireAdmin(tokenStore, userRepo, handler.NewCredentialsHandler(authManager, tokenStore)))
	mux.Handle("/api/admin/users", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserListHandler(repo)))
	mux.Handle("/api/admin/users/create", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserCreateHandler(repo)))
	mux.Handle("/api/admin/users/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserDeleteHandler(repo)))
	mux.Handle("/api/admin/users/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserStatusHandler(repo)))
	mux.Handle("/api/admin/users/reset-password", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserResetPasswordHandler(repo)))
	mux.Handle("/api/admin/users/remark", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserRemarkHandler(repo)))
	mux.Handle("/api/admin/users/custom-short-code", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserCustomShortCodeHandler(repo)))
	mux.Handle("/api/admin/users/", auth.RequireAdmin(tokenStore, userRepo, handler.NewUserSubscriptionsHandler(repo)))
	mux.Handle("/api/admin/subscriptions", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionAdminHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/subscriptions/", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionAdminHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/subscribe-files", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscribeFilesHandler(repo)))
	mux.Handle("/api/admin/subscribe-files/", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscribeFilesHandler(repo)))
	mux.Handle("/api/admin/rules/", auth.RequireAdmin(tokenStore, userRepo, http.StripPrefix("/api/admin/rules/", handler.NewRuleEditorHandler(subscribeDir, repo))))
	mux.Handle("/api/admin/rule-templates", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleTemplatesHandler(repo)))
	mux.Handle("/api/admin/rule-templates/", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleTemplatesHandler(repo)))
	mux.Handle("/api/admin/template-v3/", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateV3Handler(repo)))
	mux.Handle("/api/admin/nodes", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodesHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/nodes/", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodesHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscriptions", auth.RequireAdmin(tokenStore, userRepo, handler.NewSyncExternalSubscriptionsHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/sync-external-subscription", auth.RequireAdmin(tokenStore, userRepo, handler.NewSyncSingleExternalSubscriptionHandler(repo, subscribeDir)))
	mux.Handle("/api/admin/rules/latest", auth.RequireAdmin(tokenStore, userRepo, handler.NewRuleMetadataHandler(subscribeDir, repo)))
	mux.Handle("/api/admin/custom-rules", auth.RequireAdmin(tokenStore, userRepo, handler.NewCustomRulesHandler(repo)))
	mux.Handle("/api/admin/custom-rules/", auth.RequireAdmin(tokenStore, userRepo, handler.NewCustomRuleHandler(repo)))
	mux.Handle("/api/admin/apply-custom-rules", auth.RequireAdmin(tokenStore, userRepo, handler.NewApplyCustomRulesHandler(repo)))
	mux.Handle("/api/admin/templates", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplatesHandler(repo)))
	mux.Handle("/api/admin/templates/", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateHandler(repo)))
	mux.Handle("/api/admin/templates/convert", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateConvertHandler()))
	mux.Handle("/api/admin/templates/fetch-source", auth.RequireAdmin(tokenStore, userRepo, handler.NewTemplateFetchSourceHandler()))
	mux.Handle("/api/admin/backup/download", auth.RequireAdmin(tokenStore, userRepo, handler.NewBackupDownloadHandler(repo)))
	mux.Handle("/api/admin/backup/restore", auth.RequireAdmin(tokenStore, userRepo, handler.NewBackupRestoreHandler(repo)))
	mux.Handle("/api/admin/update/check", auth.RequireAdmin(tokenStore, userRepo, handler.NewUpdateCheckHandler()))
	mux.Handle("/api/admin/update/apply", auth.RequireAdmin(tokenStore, userRepo, handler.NewUpdateApplyHandler()))
	mux.Handle("/api/admin/update/apply-sse", auth.RequireAdmin(tokenStore, userRepo, handler.NewUpdateApplySSEHandler()))
	mux.Handle("/api/admin/proxy-groups/sync", auth.RequireAdmin(tokenStore, userRepo, handler.NewProxyGroupsSyncHandler(repo, proxyGroupsStore)))

	// Sing-box management endpoints (admin only)
	mux.Handle("/api/admin/singbox/install", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxInstallHandler(repo)))
	mux.Handle("/api/admin/singbox/install-sse", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxInstallSSEHandler(repo)))
	mux.Handle("/api/admin/singbox/uninstall", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxUninstallHandler(repo)))
	mux.Handle("/api/admin/singbox/install-status", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxInstallStatusHandler(repo)))
	mux.Handle("/api/admin/singbox/service/start", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceStartHandler(repo)))
	mux.Handle("/api/admin/singbox/service/stop", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceStopHandler(repo)))
	mux.Handle("/api/admin/singbox/service/restart", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceRestartHandler(repo)))
	mux.Handle("/api/admin/singbox/service/enable", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceEnableHandler(repo)))
	mux.Handle("/api/admin/singbox/service/disable", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceDisableHandler(repo)))
	mux.Handle("/api/admin/singbox/service/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceStatusHandler(repo)))
	mux.Handle("/api/admin/singbox/service/logs", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceLogsHandler(repo)))
	mux.Handle("/api/admin/singbox/service/logs/stream", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxServiceLogsStreamHandler(repo)))
	mux.Handle("/api/admin/singbox/system/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxSystemStatusHandler(repo)))
	mux.Handle("/api/admin/singbox/environment", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxEnvironmentHandler(repo)))

	// Sing-box 配置管理 endpoints (admin only)
	mux.Handle("/api/admin/singbox/config/generate", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxConfigGenerateHandler(repo)))
	mux.Handle("/api/admin/singbox/deploy", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxDeployHandler(repo)))
	mux.Handle("/api/admin/singbox/config/save", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxConfigSaveHandler(repo)))
	mux.Handle("/api/admin/singbox/config/list", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxConfigListHandler(repo)))

	// Sing-box 端口管理 endpoints (admin only)
	mux.Handle("/api/admin/singbox/port/allocate", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxPortAllocateHandler(repo)))
	mux.Handle("/api/admin/singbox/port/check", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxPortCheckHandler(repo)))
	mux.Handle("/api/admin/singbox/port/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingboxPortStatusHandler(repo)))

	// TCPing endpoint (admin only)
	mux.Handle("/api/admin/tcping", auth.RequireAdmin(tokenStore, userRepo, handler.NewTCPingHandler()))
	mux.Handle("/api/admin/tcping/batch", auth.RequireAdmin(tokenStore, userRepo, handler.NewTCPingBatchHandler()))

	// Certificate management endpoints (admin only)
	mux.Handle("/api/admin/cert/generate", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateGenerateHandler(repo)))
	mux.Handle("/api/admin/cert/renew", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateRenewHandler(repo)))
	mux.Handle("/api/admin/cert/list", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateListHandler(repo)))
	mux.Handle("/api/admin/cert/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateDeleteHandler(repo)))
	mux.Handle("/api/admin/cert/check", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateCheckHandler(repo)))
	mux.Handle("/api/admin/cert/auto-renew", auth.RequireAdmin(tokenStore, userRepo, handler.NewAutoRenewCertificatesHandler(repo)))

	// Argo tunnel management endpoints (admin only)
	mux.Handle("/api/admin/argo/list", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelListHandler(repo)))
	mux.Handle("/api/admin/argo/create", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelCreateHandler(repo)))
	mux.Handle("/api/admin/argo/action", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelActionHandler(repo)))
	mux.Handle("/api/admin/argo/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelStatusHandler(repo)))
	mux.Handle("/api/admin/argo/logs", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelLogsHandler(repo)))
	mux.Handle("/api/admin/argo/install", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelInstallHandler(repo)))
	mux.Handle("/api/admin/argo/validate-token", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTokenValidatorHandler(repo)))
	mux.Handle("/api/admin/argo/quick", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoQuickTunnelHandler(repo)))
	mux.Handle("/api/admin/argo/metrics", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoMetricsHandler(repo)))
	mux.Handle("/api/admin/argo/logs/stream", auth.RequireAdmin(tokenStore, userRepo, handler.NewArgoTunnelStreamLogsHandler(repo)))

	// WARP management endpoints (admin only)
	mux.Handle("/api/admin/warp/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPStatusHandler(repo)))
	mux.Handle("/api/admin/warp/enable", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPEnableHandler(repo)))
	mux.Handle("/api/admin/warp/disable", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPDisableHandler(repo)))
	mux.Handle("/api/admin/warp/configs", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPConfigsHandler(repo)))
	mux.Handle("/api/admin/warp/update", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPUpdateHandler(repo)))
	mux.Handle("/api/admin/warp/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPDeleteHandler(repo)))
	mux.Handle("/api/admin/warp/check-connection", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPConnectionCheckHandler(repo)))
	mux.Handle("/api/admin/warp/generate-config", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPConfigGenerateHandler(repo)))
	mux.Handle("/api/admin/warp/install", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPInstallHandler(repo)))
	mux.Handle("/api/admin/warp/validate-license", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPLicenseValidateHandler(repo)))
	mux.Handle("/api/admin/warp/check-port", auth.RequireAdmin(tokenStore, userRepo, handler.NewWARPPortCheckHandler(repo)))

	// System optimization endpoints (admin only)
	mux.Handle("/api/admin/system/optimize", auth.RequireAdmin(tokenStore, userRepo, handler.NewSystemOptimizationHandler(repo)))
	mux.Handle("/api/admin/system/bbr-status", auth.RequireAdmin(tokenStore, userRepo, handler.NewBBRStatusHandler(repo)))
	mux.Handle("/api/admin/system/network-performance", auth.RequireAdmin(tokenStore, userRepo, handler.NewNetworkPerformanceHandler(repo)))
	mux.Handle("/api/admin/system/speed-test", auth.RequireAdmin(tokenStore, userRepo, handler.NewNetworkSpeedTestHandler(repo)))
	mux.Handle("/api/admin/system/resource-usage", auth.RequireAdmin(tokenStore, userRepo, handler.NewSystemResourceUsageHandler(repo)))
	mux.Handle("/api/admin/system/report", auth.RequireAdmin(tokenStore, userRepo, handler.NewSystemReportHandler(repo)))
	mux.Handle("/api/admin/system/kernel-param", auth.RequireAdmin(tokenStore, userRepo, handler.NewKernelParameterHandler(repo)))
	mux.Handle("/api/admin/system/interfaces", auth.RequireAdmin(tokenStore, userRepo, handler.NewNetworkInterfacesHandler(repo)))
	mux.Handle("/api/admin/system/routing", auth.RequireAdmin(tokenStore, userRepo, handler.NewRoutingTableHandler(repo)))
	mux.Handle("/api/admin/system/dns", auth.RequireAdmin(tokenStore, userRepo, handler.NewDNSConfigurationHandler(repo)))
	mux.Handle("/api/admin/system/connections", auth.RequireAdmin(tokenStore, userRepo, handler.NewActiveConnectionsHandler(repo)))

	// Subscription management endpoints (admin only)
	mux.Handle("/api/admin/subscription/generate", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionGenerateHandler(repo)))
	mux.Handle("/api/admin/subscription/list", auth.RequireAdmin(tokenStore, userRepo, handler.NewSingBoxSubscriptionListHandler(repo)))
	mux.Handle("/api/admin/subscription/detail", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionDetailHandler(repo)))
	mux.Handle("/api/admin/subscription/export", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionExportHandler(repo)))
	mux.Handle("/api/admin/subscription/update", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionUpdateHandler(repo)))
	mux.Handle("/api/admin/subscription/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionDeleteHandler(repo)))
	mux.Handle("/api/admin/subscription/url", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionURLHandler(repo)))
	mux.Handle("/api/admin/subscription/node-link", auth.RequireAdmin(tokenStore, userRepo, handler.NewNodeLinkGenerateHandler(repo)))
	mux.Handle("/api/admin/subscription/qrcode", auth.RequireAdmin(tokenStore, userRepo, handler.NewQRCodeGenerateHandler(repo)))
	mux.Handle("/api/admin/subscription/encrypt", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionEncryptHandler(repo)))
	mux.Handle("/api/admin/subscription/decrypt", auth.RequireAdmin(tokenStore, userRepo, handler.NewSubscriptionDecryptHandler(repo)))

	// Share management endpoints (admin only)
	mux.Handle("/api/admin/share/create", auth.RequireAdmin(tokenStore, userRepo, handler.NewShareCreateHandler(repo)))
	mux.Handle("/api/admin/share/node", auth.RequireAdmin(tokenStore, userRepo, handler.NewShareNodeHandler(repo)))
	mux.Handle("/api/admin/share/list", auth.RequireAdmin(tokenStore, userRepo, handler.NewShareListHandler(repo)))
	mux.Handle("/api/admin/share/detail", auth.RequireAdmin(tokenStore, userRepo, handler.NewShareDetailHandler(repo)))
	mux.Handle("/api/admin/share/update", auth.RequireAdmin(tokenStore, userRepo, handler.NewShareUpdateHandler(repo)))
	mux.Handle("/api/admin/share/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewShareDeleteHandler(repo)))
	mux.Handle("/api/admin/gitlab/sync", auth.RequireAdmin(tokenStore, userRepo, handler.NewGitLabSyncHandler(repo)))
	mux.Handle("/api/admin/github/sync", auth.RequireAdmin(tokenStore, userRepo, handler.NewGitHubSyncHandler(repo)))
	mux.Handle("/api/admin/pastebin/share", auth.RequireAdmin(tokenStore, userRepo, handler.NewPastebinShareHandler(repo)))

	// Certificate management endpoints (admin only)
	mux.Handle("/api/admin/certificate/generate", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateGenerateHandler(repo)))
	mux.Handle("/api/admin/certificate/renew", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateRenewHandler(repo)))
	mux.Handle("/api/admin/certificate/auto-renew", auth.RequireAdmin(tokenStore, userRepo, handler.NewAutoRenewCertificatesHandler(repo)))
	mux.Handle("/api/admin/certificate/list", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateListHandler(repo)))
	mux.Handle("/api/admin/certificate/delete", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateDeleteHandler(repo)))
	mux.Handle("/api/admin/certificate/check", auth.RequireAdmin(tokenStore, userRepo, handler.NewCertificateCheckHandler(repo)))

	// Monitoring endpoints (admin only)
	mux.Handle("/api/admin/monitoring/status", auth.RequireAdmin(tokenStore, userRepo, handler.NewMonitoringStatusHandler(repo)))

	// User endpoints (all authenticated users)
	mux.Handle("/api/proxy-groups", auth.RequireToken(tokenStore, handler.NewProxyGroupsHandler(proxyGroupsStore)))
	mux.Handle("/api/user/password", auth.RequireToken(tokenStore, handler.NewPasswordHandler(authManager)))
	mux.Handle("/api/user/profile", auth.RequireToken(tokenStore, handler.NewProfileHandler(repo)))
	mux.Handle("/api/user/settings", auth.RequireToken(tokenStore, handler.NewUserSettingsHandler(repo, tokenStore)))
	mux.Handle("/api/user/config", auth.RequireToken(tokenStore, handler.NewUserConfigHandler(repo)))
	mux.Handle("/api/user/token", auth.RequireToken(tokenStore, handler.NewUserTokenHandler(repo)))
	mux.Handle("/api/user/external-subscriptions", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionsHandler(repo)))
	mux.Handle("/api/user/external-subscriptions/nodes", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionNodesHandler(repo)))
	mux.Handle("/api/user/external-subscriptions/check-filter", auth.RequireToken(tokenStore, handler.NewExternalSubscriptionCheckFilterHandler(repo)))
	mux.Handle("/api/user/proxy-provider-configs", auth.RequireToken(tokenStore, handler.NewProxyProviderConfigsHandler(repo)))
	mux.Handle("/api/user/proxy-provider-cache/refresh", auth.RequireToken(tokenStore, handler.NewProxyProviderCacheRefreshHandler(repo)))
	mux.Handle("/api/user/proxy-provider-cache/status", auth.RequireToken(tokenStore, handler.NewProxyProviderCacheStatusHandler(repo)))
	mux.Handle("/api/user/proxy-provider-nodes", auth.RequireToken(tokenStore, handler.NewProxyProviderNodesHandler(repo)))
	mux.Handle("/api/proxy-provider/", handler.NewProxyProviderServeHandler(repo))

	// Debug日志相关endpoint
	mux.Handle("/api/user/debug/", auth.RequireToken(tokenStore, handler.NewDebugHandler(repo)))

	mux.Handle("/api/traffic/summary", auth.RequireToken(tokenStore, trafficHandler))
	mux.Handle("/api/subscriptions", auth.RequireToken(tokenStore, handler.NewSubscriptionListHandler(repo)))
	mux.Handle("/api/dns/resolve", auth.RequireToken(tokenStore, handler.NewDNSHandler()))
	mux.Handle("/api/subscribe-files", auth.RequireToken(tokenStore, handler.NewSubscribeFilesListHandler(repo)))

	// Create subscription handler (shared between endpoint and short links)
	subscriptionHandler := handler.NewSubscriptionHandlerConcrete(repo, subscribeDir)
	mux.Handle("/api/clash/subscribe", handler.NewSubscriptionEndpoint(tokenStore, repo, subscribeDir))

	// Short link reset endpoint (authenticated)
	mux.Handle("/api/user/short-link", auth.RequireToken(tokenStore, handler.NewShortLinkResetHandler(repo)))
	mux.Handle("/api/user/custom-short-code", auth.RequireToken(tokenStore, handler.NewUserCustomShortCodeSelfHandler(repo)))

	// Temporary subscription endpoints
	mux.Handle("/api/admin/temp-subscription", auth.RequireAdmin(tokenStore, userRepo, handler.NewTempSubscriptionHandler()))
	tempSubAccessHandler := handler.NewTempSubscriptionAccessHandler()

	// Combined handler for short links and web app
	// 短链接默认为 3 + 3, 订阅code+用户code, 自定义最小为1+1, 不限制长度
	// /t/{id} paths route to temporary subscription handler
	// All other paths go to the web handler
	shortLinkHandler := handler.NewShortLinkHandler(repo, subscriptionHandler)
	bruteForceProtector := handler.NewBruteForceProtector()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.Trim(r.URL.Path, "/")
		clientIP := handler.GetClientIP(r)

		// 暴力探测封禁检查
		if bruteForceProtector.IsBlocked(clientIP, r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		// Check if this is a temporary subscription access (starts with "t/" followed by 8 hex chars)
		if strings.HasPrefix(path, "t/") && len(path) == 10 {
			rec := &handler.StatusRecorder{ResponseWriter: w, StatusCode: 200}
			tempSubAccessHandler.ServeHTTP(rec, r)
			if rec.StatusCode == http.StatusNotFound || rec.StatusCode == http.StatusForbidden {
				bruteForceProtector.RecordFailure(clientIP, r.URL.Path)
			}
			return
		}
		// 自定义短链接后, 订阅+用户最小为2个字符
		// TryServe does DB lookup; returns false if no match, allowing fallthrough to web
		if len(path) >= 2 && isAlphanumeric(path) {
			if shortLinkHandler.TryServe(w, r) {
				return
			}
			bruteForceProtector.RecordFailure(clientIP, r.URL.Path)
		}
		// Otherwise, pass to web handler
		web.Handler().ServeHTTP(w, r)
	})

	allowedOrigins := getAllowedOrigins()

	// 静默模式中间件
	silentModeManager := handler.NewSilentModeManager(repo, tokenStore)
	handlerWithSilentMode := silentModeManager.Middleware(mux)
	handlerWithCORS := withCORS(handlerWithSilentMode, allowedOrigins)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handlerWithCORS,
		ReadHeaderTimeout: 5 * time.Second,
	}

	collectorCtx, stopCollector := context.WithCancel(context.Background())
	go startTrafficCollector(collectorCtx, trafficHandler)

	go func() {
		logger.Info("HTTP服务器启动", "version", version.Version, "address", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP服务器运行失败", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(srv, stopCollector, stopProxySync)
}

func getAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8150"
	}
	return ":" + port
}

// isAlphanumeric checks if a string contains only alphanumeric characters
func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func waitForShutdown(srv *http.Server, cancels ...context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("收到关闭信号，开始优雅关闭")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 停止所有后台任务
	for _, cancelFunc := range cancels {
		if cancelFunc != nil {
			cancelFunc()
		}
	}

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("优雅关闭失败", "error", err)
	} else {
		logger.Info("服务器已安全关闭")
	}
}

func startTrafficCollector(ctx context.Context, trafficHandler *handler.TrafficSummaryHandler) {
	if trafficHandler == nil {
		return
	}

	// 带重试的流量收集函数
	runWithRetry := func() {
		logger.Info("[流量收集器] 开始每日流量收集", "start_time", time.Now().Format("2006-01-02 15:04:05"))

		maxRetries := 3
		retryDelay := 30 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := trafficHandler.RecordDailyUsage(runCtx)
			cancel()

			if err == nil {
				logger.Info("[流量收集器] 每日流量收集成功")
				return
			}

			logger.Warn("[流量收集器] 每日流量收集失败", "attempt", attempt, "max_retries", maxRetries, "error", err)

			if attempt < maxRetries {
				logger.Info("[流量收集器] 准备重试", "delay", retryDelay)
				select {
				case <-ctx.Done():
					logger.Info("[流量收集器] 重试已取消（服务器关闭）")
					return
				case <-time.After(retryDelay):
					// 继续重试
				}
			}
		}

		logger.Error("[流量收集器] 达到最大重试次数后仍失败", "max_retries", maxRetries)
	}

	runWithRetry()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("[流量收集器] 定时调度器已启动", "interval", "24小时")

	for {
		select {
		case <-ctx.Done():
			logger.Info("[流量收集器] 定时调度器已停止")
			return
		case <-ticker.C:
			runWithRetry()
		}
	}
}

// syncSubscribeFilesToDatabase scans the subscribes directory and ensures
// every YAML file has a corresponding record in the subscribe_files table.
// This helps with backward compatibility when upgrading from older versions.
func syncSubscribeFilesToDatabase(repo *storage.TrafficRepository, subscribeDir string) {
	if repo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Read all files from subscribes directory
	entries, err := os.ReadDir(subscribeDir)
	if err != nil {
		logger.Warn("读取订阅目录失败", "dir", subscribeDir, "error", err)
		return
	}

	synced := 0
	for _, entry := range entries {
		// Skip directories and non-YAML files
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if filepath.Ext(filename) != ".yaml" && filepath.Ext(filename) != ".yml" {
			continue
		}

		// Skip the .keep.yaml placeholder file
		if filename == ".keep.yaml" {
			continue
		}

		// Check if this file already has a database record
		if _, err := repo.GetSubscribeFileByFilename(ctx, filename); err == nil {
			// File already exists in database, skip
			continue
		} else if !errors.Is(err, storage.ErrSubscribeFileNotFound) {
			logger.Warn("检查订阅文件失败", "filename", filename, "error", err)
			continue
		}

		// File doesn't exist in database, create a new record
		// Use filename without extension as the name
		name := filename[:len(filename)-len(filepath.Ext(filename))]

		file := storage.SubscribeFile{
			Name:        name,
			Description: "自动同步的订阅文件",
			URL:         "",                          // No URL for legacy files
			Type:        storage.SubscribeTypeUpload, // Mark as upload type
			Filename:    filename,
		}

		if _, err := repo.CreateSubscribeFile(ctx, file); err != nil {
			logger.Warn("同步订阅文件到数据库失败", "filename", filename, "error", err)
			continue
		}

		synced++
	}

	if synced > 0 {
		logger.Info("订阅文件同步完成", "count", synced)
	}
}

// startLogCleanup 启动日志清理任务
func startLogCleanup() {
	logManager := logger.NewLogManager("data/logs")

	// 启动时立即清理一次
	if err := logManager.CleanupOldLogs(); err != nil {
		logger.Error("[日志清理] 启动时清理失败", "error", err)
	}

	// 每天凌晨3点清理
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("[日志清理] 定时清理任务已启动", "interval", "24小时", "max_age", "7天")

	for range ticker.C {
		if err := logManager.CleanupOldLogs(); err != nil {
			logger.Error("[日志清理] 定时清理失败", "error", err)
		}
	}
}
