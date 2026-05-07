package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	pragmaJournalMode = "PRAGMA journal_mode=WAL;"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

const (
	SubscriptionButtonQR     = "qr"
	SubscriptionButtonCopy   = "copy"
	SubscriptionButtonImport = "import"
)

// TrafficRecord represents an aggregated traffic snapshot for a specific date.
type TrafficRecord struct {
	Date           time.Time
	TotalLimit     int64
	TotalUsed      int64
	TotalRemaining int64
}

// RuleVersion represents an archived version of a YAML rule file.
type RuleVersion struct {
	Filename  string
	Version   int64
	Content   string
	CreatedBy string
	CreatedAt time.Time
}

// TrafficRepository manages persistence of traffic usage snapshots.
type TrafficRepository struct {
	db *sql.DB
}

// SubscriptionLink represents a configurable subscription entry exposed to clients.
type SubscriptionLink struct {
	ID           int64
	Name         string
	Type         string
	Description  string
	RuleFilename string
	Buttons      []string
	ShortURL     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func normalizeSubscriptionButtons(input []string) []string {
	if len(input) == 0 {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	seen := make(map[string]struct{}, len(input))
	for _, button := range input {
		key := strings.ToLower(strings.TrimSpace(button))
		if _, ok := allowedSubscriptionButtons[key]; ok {
			seen[key] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	order := []string{SubscriptionButtonQR, SubscriptionButtonCopy, SubscriptionButtonImport}
	normalized := make([]string, 0, len(seen))
	for _, button := range order {
		if _, ok := seen[button]; ok {
			normalized = append(normalized, button)
		}
	}

	return normalized
}

func encodeSubscriptionButtons(input []string) (string, error) {
	normalized := normalizeSubscriptionButtons(input)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSubscriptionButtons(encoded string) []string {
	if strings.TrimSpace(encoded) == "" {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	var raw []string
	if err := json.Unmarshal([]byte(encoded), &raw); err != nil {
		return append([]string(nil), defaultSubscriptionButtons...)
	}

	return normalizeSubscriptionButtons(raw)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubscriptionLink(scanner rowScanner) (SubscriptionLink, error) {
	var (
		link    SubscriptionLink
		buttons string
	)

	if err := scanner.Scan(&link.ID, &link.Name, &link.Type, &link.Description, &link.RuleFilename, &buttons, &link.ShortURL, &link.CreatedAt, &link.UpdatedAt); err != nil {
		return SubscriptionLink{}, err
	}

	link.Buttons = decodeSubscriptionButtons(buttons)

	return link, nil
}

var (
	ErrTokenNotFound                = errors.New("token not found")
	ErrUserNotFound                 = errors.New("user not found")
	ErrUserExists                   = errors.New("user already exists")
	ErrRuleVersionNotFound          = errors.New("rule version not found")
	ErrSubscriptionNotFound         = errors.New("subscription link not found")
	ErrSubscriptionExists           = errors.New("subscription link already exists")
	ErrNodeNotFound                 = errors.New("node not found")
	ErrSubscribeFileNotFound        = errors.New("subscribe file not found")
	ErrSubscribeFileExists          = errors.New("subscribe file already exists")
	ErrUserSettingsNotFound         = errors.New("user settings not found")
	ErrCustomRuleNotFound           = errors.New("custom rule not found")
	ErrExternalSubscriptionNotFound = errors.New("external subscription not found")
	ErrExternalSubscriptionExists   = errors.New("external subscription already exists")
)

var (
	allowedSubscriptionButtons = map[string]struct{}{
		SubscriptionButtonQR:     {},
		SubscriptionButtonCopy:   {},
		SubscriptionButtonImport: {},
	}
	defaultSubscriptionButtons = []string{
		SubscriptionButtonQR,
		SubscriptionButtonCopy,
		SubscriptionButtonImport,
	}
)

// Node represents a proxy node stored in the database.
type Node struct {
	ID              int64
	Username        string
	RawURL          string
	NodeName        string
	Protocol        string
	ParsedConfig    string
	ClashConfig     string
	Enabled         bool
	Tag             string   // 向后兼容，等于 Tags[0]
	Tags            []string // 多标签支持
	OriginalServer  string
	SourceType      string
	SourceRefID     string
	SourceRefName   string
	SourceUpdatedAt string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SubscribeFile represents a subscription file configuration.
type SubscribeFile struct {
	ID                  int64
	Name                string
	Description         string
	URL                 string
	Type                string
	Filename            string
	FileShortCode       string     // 3-character code for file identification in composite short links
	CustomShortCode     string     // User-defined short code (replaces FileShortCode when set)
	AutoSyncCustomRules bool       // Whether to automatically sync custom rules to this file
	TemplateFilename    string     // 绑定的 V3 模板文件名，为空表示未绑定模板
	SelectedTags        []string   // 选中的节点标签，为空表示使用所有节点
	RawOutput           bool       // 非Clash配置，直接输出原始内容
	SortOrder           int        // 排序权重，值越小越靠前
	TrafficLimit        *float64   // 手动设置的总流量上限(GB)
	ExpireAt            *time.Time // Optional expiration timestamp
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// UserSettings represents user-specific configuration.
type UserSettings struct {
	Username            string
	ForceSyncExternal   bool
	MatchRule           string     // "node_name", "server_port", or "type_server_port"
	SyncScope           string     // "saved_only" or "all"
	KeepNodeName        bool       // Keep current node name when syncing
	CacheExpireMinutes  int        // Cache expiration time in minutes
	SyncTraffic         bool       // Sync traffic info from external subscriptions
	CustomRulesEnabled  bool       // Enable custom rules feature
	TemplateVersion     string     // Template version: "v1" (file-based), "v2" (database/ACL), "v3" (mihomo-style)
	EnableProxyProvider bool       // Enable proxy provider feature
	NodeOrder           []int64    // Node display order (array of node IDs)
	NodeNameFilter      string     // Regex pattern to filter out nodes by name during sync
	DebugEnabled        bool       // Enable debug logging to file
	DebugLogPath        string     // Path to current debug log file
	DebugStartedAt      *time.Time // When debug logging was started
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// SystemConfig represents global system configuration shared across all users.
type SystemConfig struct {
	ProxyGroupsSourceURL    string // Remote URL for proxy groups configuration
	ClientCompatibilityMode bool   // Auto-filter incompatible nodes for clients
	SilentMode              bool   // Silent mode: return 404 for all requests except subscription
	SilentModeTimeout       int    // Minutes to allow access after subscription fetch (default 15)
	EnableSubInfoNodes      bool   // Enable subscription info nodes (expire time and remaining traffic)
	SubInfoExpirePrefix     string // Prefix for expire time node, default "📅过期时间"
	SubInfoTrafficPrefix    string // Prefix for remaining traffic node, default "⌛剩余流量"
	EnableShortLink         bool   // 启用短链接（全局设置）
	EnableSubTrafficHeader  bool   // 启用订阅响应头流量信息
}

// ExternalSubscription represents an external subscription URL imported by user.
type ExternalSubscription struct {
	ID          int64
	Username    string
	Name        string
	URL         string
	UserAgent   string // User-Agent 请求头
	NodeCount   int
	LastSyncAt  *time.Time
	Upload      int64      // 已上传流量（字节）
	Download    int64      // 已下载流量（字节）
	Total       int64      // 总流量（字节）
	Expire      *time.Time // 过期时间
	TrafficMode string     // 流量统计方式: "download", "upload", "both", "none"
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CustomRule represents a custom rule for DNS, rules, or rule-providers.
type CustomRule struct {
	ID        int64
	Name      string
	Type      string // "dns", "rules", "rule-providers"
	Mode      string // "replace", "prepend"
	Content   string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CustomRuleApplication tracks what content was applied by custom rules to subscribe files
type CustomRuleApplication struct {
	ID              int64
	SubscribeFileID int64
	CustomRuleID    int64
	RuleType        string // "dns", "rules", "rule-providers"
	RuleMode        string // "replace", "prepend"
	AppliedContent  string // JSON-serialized content that was applied
	ContentHash     string // SHA256 hash of the content for quick comparison
	AppliedAt       time.Time
}

// ProxyProviderConfig represents a proxy-provider configuration for external subscription
type ProxyProviderConfig struct {
	ID                        int64
	Username                  string
	ExternalSubscriptionID    int64
	Name                      string // 代理集合名称
	Type                      string // http/file
	Interval                  int    // 更新间隔(秒)
	Proxy                     string // 下载代理
	SizeLimit                 int    // 文件大小限制
	Header                    string // JSON: {"User-Agent": [...], "Authorization": [...]}
	HealthCheckEnabled        bool
	HealthCheckURL            string
	HealthCheckInterval       int
	HealthCheckTimeout        int
	HealthCheckLazy           bool
	HealthCheckExpectedStatus int
	Filter                    string // 正则: 保留匹配的节点
	ExcludeFilter             string // 正则: 排除匹配的节点
	ExcludeType               string // 排除的协议类型，逗号分隔
	GeoIPFilter               string // 地理位置过滤，国家代码如 "HK" 或 "HK,TW"（仅 MMW 模式生效）
	Override                  string // JSON: 覆写配置
	ProcessMode               string // 'client'=客户端处理, 'mmw'=妙妙屋处理
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// NewTrafficRepository initializes a new SQLite-backed repository stored at the given path or DSN.
func NewTrafficRepository(path string) (*TrafficRepository, error) {
	if path == "" {
		return nil, errors.New("traffic repository path is empty")
	}

	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create traffic data directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(pragmaJournalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}

	repo := &TrafficRepository{db: db}
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

// Close releases the underlying database resources.
func (r *TrafficRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Checkpoint forces a WAL checkpoint to ensure all data is written to the main database file.
// This is useful before creating backups.
func (r *TrafficRepository) Checkpoint() error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

func (r *TrafficRepository) RecordDaily(ctx context.Context, date time.Time, totalLimit, totalUsed, totalRemaining int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	normalized := date.UTC().Format("2006-01-02")
	_, err := r.db.ExecContext(ctx, `INSERT INTO traffic_records (date, total_limit, total_used, total_remaining) VALUES (?, ?, ?, ?) ON CONFLICT(date) DO UPDATE SET total_limit = excluded.total_limit, total_used = excluded.total_used, total_remaining = excluded.total_remaining, created_at = CURRENT_TIMESTAMP`, normalized, totalLimit, totalUsed, totalRemaining)
	if err != nil {
		return fmt.Errorf("upsert traffic record: %w", err)
	}
	return nil
}

func (r *TrafficRepository) ListRecent(ctx context.Context, limit int) ([]TrafficRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.db.QueryContext(ctx, `SELECT date, total_limit, total_used, total_remaining FROM traffic_records ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent traffic records: %w", err)
	}
	defer rows.Close()
	var records []TrafficRecord
	for rows.Next() {
		var record TrafficRecord
		var dateStr string
		if err := rows.Scan(&dateStr, &record.TotalLimit, &record.TotalUsed, &record.TotalRemaining); err != nil {
			return nil, fmt.Errorf("scan traffic record: %w", err)
		}
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("parse traffic record date: %w", err)
		}
		record.Date = parsed
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *TrafficRepository) IsSyncTrafficEnabled(ctx context.Context) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("traffic repository not initialized")
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM user_settings WHERE COALESCE(sync_traffic, 0) != 0`).Scan(&count); err != nil {
		return false, fmt.Errorf("query sync traffic setting: %w", err)
	}
	return count > 0, nil
}

func (r *TrafficRepository) migrate() error {
	const trafficSchema = `
CREATE TABLE IF NOT EXISTS traffic_records (
    date TEXT PRIMARY KEY,
    total_limit INTEGER NOT NULL,
    total_used INTEGER NOT NULL,
    total_remaining INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(trafficSchema); err != nil {
		return fmt.Errorf("migrate traffic_records: %w", err)
	}

	const userTokenSchema = `
CREATE TABLE IF NOT EXISTS user_tokens (
    username TEXT PRIMARY KEY,
    token TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(userTokenSchema); err != nil {
		return fmt.Errorf("migrate user_tokens: %w", err)
	}

	// Add user_short_code column to user_tokens table if it doesn't exist (3-character code)
	if err := r.ensureUserTokenColumn("user_short_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Create unique index for user_short_code (only for non-empty values)
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tokens_user_short_code ON user_tokens(user_short_code) WHERE user_short_code != '';`); err != nil {
		return fmt.Errorf("create user_short_code index: %w", err)
	}

	// Generate user short codes for existing users that don't have one
	if err := r.generateMissingUserShortCodes(); err != nil {
		return fmt.Errorf("generate missing user short codes: %w", err)
	}

	// Add custom_user_short_code column to user_tokens table
	if err := r.ensureUserTokenColumn("custom_user_short_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_tokens_custom_user_short_code ON user_tokens(custom_user_short_code) WHERE custom_user_short_code != '';`); err != nil {
		return fmt.Errorf("create custom_user_short_code index: %w", err)
	}

	const sessionSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sessions_username ON sessions(username);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`

	if _, err := r.db.Exec(sessionSchema); err != nil {
		return fmt.Errorf("migrate sessions: %w", err)
	}

	const userSchema = `
CREATE TABLE IF NOT EXISTS users (
    username TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    email TEXT,
    nickname TEXT,
    avatar_url TEXT,
    role TEXT NOT NULL DEFAULT 'user',
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

	if _, err := r.db.Exec(userSchema); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}

	if err := r.ensureUserColumn("email", "TEXT"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("nickname", "TEXT"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("avatar_url", "TEXT"); err != nil {
		return err
	}

	if err := r.syncNicknames(); err != nil {
		return err
	}

	if err := r.ensureUserColumn("role", "TEXT NOT NULL DEFAULT 'user'"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("is_active", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	if err := r.ensureUserColumn("remark", "TEXT"); err != nil {
		return err
	}

	const historySchema = `
CREATE TABLE IF NOT EXISTS rule_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filename TEXT NOT NULL,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(filename, version)
);
`

	if _, err := r.db.Exec(historySchema); err != nil {
		return fmt.Errorf("migrate rule_versions: %w", err)
	}

	const subscriptionSchema = `
CREATE TABLE IF NOT EXISTS subscription_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT '',
    description TEXT,
    rule_filename TEXT NOT NULL,
    buttons TEXT NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);
`

	if _, err := r.db.Exec(subscriptionSchema); err != nil {
		return fmt.Errorf("migrate subscription_links: %w", err)
	}

	// Add short_url column to subscription_links table if it doesn't exist
	if err := r.ensureSubscriptionLinkColumn("short_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Create unique index for short_url (only for non-empty values)
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_links_short_url ON subscription_links(short_url) WHERE short_url != '';`); err != nil {
		return fmt.Errorf("create short_url index: %w", err)
	}

	const nodesSchema = `
CREATE TABLE IF NOT EXISTS nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    raw_url TEXT NOT NULL,
    node_name TEXT NOT NULL,
    protocol TEXT NOT NULL,
    parsed_config TEXT NOT NULL,
    clash_config TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    tag TEXT NOT NULL DEFAULT '手动输入',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_nodes_username ON nodes(username);
CREATE INDEX IF NOT EXISTS idx_nodes_protocol ON nodes(protocol);
CREATE INDEX IF NOT EXISTS idx_nodes_enabled ON nodes(enabled);
`

	if _, err := r.db.Exec(nodesSchema); err != nil {
		return fmt.Errorf("migrate nodes: %w", err)
	}

	// Add tag column to existing nodes table if it doesn't exist
	if err := r.ensureNodeColumn("tag", "TEXT NOT NULL DEFAULT '手动输入'"); err != nil {
		return err
	}

	// Add original_server column to existing nodes table if it doesn't exist
	if err := r.ensureNodeColumn("original_server", "TEXT"); err != nil {
		return err
	}

	// Add tags column (JSON array) for multi-tag support
	if err := r.ensureNodeColumn("tags", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	// Migrate existing tag data to tags column
	r.db.Exec(`UPDATE nodes SET tags = '["' || REPLACE(tag, '"', '\"') || '"]' WHERE (tags = '[]' OR tags = '') AND tag != '' AND tag IS NOT NULL`)

	// Add source tracking columns for Sing-box published nodes
	if err := r.ensureNodeColumn("source_type", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureNodeColumn("source_ref_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureNodeColumn("source_ref_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureNodeColumn("source_updated_at", "TIMESTAMP"); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_source ON nodes(source_type, source_ref_id, protocol);`); err != nil {
		return fmt.Errorf("create source index: %w", err)
	}

	// Create tag index after ensuring column exists
	if _, err := r.db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_tag ON nodes(tag);`); err != nil {
		return fmt.Errorf("create tag index: %w", err)
	}

	const subscribeFilesSchema = `
CREATE TABLE IF NOT EXISTS subscribe_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    url TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('create','import','upload')),
    filename TEXT NOT NULL,
    expire_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_subscribe_files_type ON subscribe_files(type);
`

	if _, err := r.db.Exec(subscribeFilesSchema); err != nil {
		return fmt.Errorf("migrate subscribe_files: %w", err)
	}

	// 用户-订阅关联表（多对多关系）
	// 关联到 subscribe_files 表
	const userSubscriptionsSchema = `
CREATE TABLE IF NOT EXISTS user_subscriptions (
    username TEXT NOT NULL,
    subscription_id INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (username, subscription_id),
    FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE,
    FOREIGN KEY(subscription_id) REFERENCES subscribe_files(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_username ON user_subscriptions(username);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_subscription_id ON user_subscriptions(subscription_id);
`

	if _, err := r.db.Exec(userSubscriptionsSchema); err != nil {
		return fmt.Errorf("migrate user_subscriptions: %w", err)
	}

	const userSettingsSchema = `
CREATE TABLE IF NOT EXISTS user_settings (
    username TEXT PRIMARY KEY,
    force_sync_external INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE
);
`

	if _, err := r.db.Exec(userSettingsSchema); err != nil {
		return fmt.Errorf("migrate user_settings: %w", err)
	}

	// Add match_rule column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("match_rule", "TEXT NOT NULL DEFAULT 'node_name'"); err != nil {
		return err
	}

	// Add cache_expire_minutes column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("cache_expire_minutes", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add sync_traffic column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("sync_traffic", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add sync_scope column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("sync_scope", "TEXT NOT NULL DEFAULT 'saved_only'"); err != nil {
		return err
	}

	// Add keep_node_name column to user_settings table if it doesn't exist
	if err := r.ensureUserSettingsColumn("keep_node_name", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	const externalSubscriptionsSchema = `
CREATE TABLE IF NOT EXISTS external_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    node_count INTEGER NOT NULL DEFAULT 0,
    last_sync_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(username) REFERENCES users(username) ON DELETE CASCADE,
    UNIQUE(username, url)
);
CREATE INDEX IF NOT EXISTS idx_external_subscriptions_username ON external_subscriptions(username);
CREATE INDEX IF NOT EXISTS idx_external_subscriptions_url ON external_subscriptions(url);
`

	if _, err := r.db.Exec(externalSubscriptionsSchema); err != nil {
		return fmt.Errorf("migrate external_subscriptions: %w", err)
	}

	// Add traffic fields to external_subscriptions table
	if err := r.ensureExternalSubscriptionColumn("upload", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("download", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("total", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("expire", "TIMESTAMP"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("user_agent", "TEXT NOT NULL DEFAULT 'clash-meta/2.4.0'"); err != nil {
		return err
	}
	if err := r.ensureExternalSubscriptionColumn("traffic_mode", "TEXT NOT NULL DEFAULT 'both'"); err != nil {
		return err
	}

	// Add custom_rules_enabled to user_settings table
	if err := r.ensureUserSettingsColumn("custom_rules_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add enable_short_link to user_settings table
	if err := r.ensureUserSettingsColumn("enable_short_link", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add template_version to user_settings table (default "v2")
	// This replaces the old use_new_template_system boolean field
	if err := r.ensureUserSettingsColumn("template_version", "TEXT NOT NULL DEFAULT 'v2'"); err != nil {
		return err
	}

	// Migrate old use_new_template_system to template_version
	// If use_new_template_system column exists, migrate its values
	if err := r.migrateTemplateVersionFromBool(); err != nil {
		return err
	}

	// Add enable_proxy_provider to user_settings table
	if err := r.ensureUserSettingsColumn("enable_proxy_provider", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add node_order to user_settings table (JSON array of node IDs for display order)
	if err := r.ensureUserSettingsColumn("node_order", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}

	// Add debug logging fields to user_settings table
	if err := r.ensureUserSettingsColumn("debug_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureUserSettingsColumn("debug_log_path", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureUserSettingsColumn("debug_started_at", "TIMESTAMP"); err != nil {
		return err
	}

	// Add node_name_filter to user_settings table (regex pattern to filter nodes)
	if err := r.ensureUserSettingsColumn("node_name_filter", "TEXT NOT NULL DEFAULT '剩余|流量|到期|订阅|时间|重置'"); err != nil {
		return err
	}

	// Add file_short_code column to subscribe_files table (3-character code)
	if err := r.ensureSubscribeFileColumn("file_short_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// Add expire_at column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("expire_at", "TIMESTAMP"); err != nil {
		return err
	}

	// Create unique index for file_short_code in subscribe_files (only for non-empty values)
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscribe_files_file_short_code ON subscribe_files(file_short_code) WHERE file_short_code != '';`); err != nil {
		return fmt.Errorf("create subscribe_files file_short_code index: %w", err)
	}

	// Generate file short codes for existing subscribe_files that don't have one
	if err := r.generateMissingFileShortCodes(); err != nil {
		return fmt.Errorf("generate missing file short codes: %w", err)
	}

	// Add custom_short_code column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("custom_short_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscribe_files_custom_short_code ON subscribe_files(custom_short_code) WHERE custom_short_code != '';`); err != nil {
		return fmt.Errorf("create subscribe_files custom_short_code index: %w", err)
	}

	// Add raw_output column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("raw_output", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add traffic_limit column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("traffic_limit", "REAL"); err != nil {
		return err
	}

	// Create system_config table for global settings
	const systemConfigSchema = `
CREATE TABLE IF NOT EXISTS system_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    proxy_groups_source_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := r.db.Exec(systemConfigSchema); err != nil {
		return fmt.Errorf("migrate system_config: %w", err)
	}

	// Ensure system_config has exactly one row (singleton pattern)
	const ensureSystemConfigRow = `
INSERT INTO system_config (id, proxy_groups_source_url)
SELECT 1, ''
WHERE NOT EXISTS (SELECT 1 FROM system_config WHERE id = 1);
`
	if _, err := r.db.Exec(ensureSystemConfigRow); err != nil {
		return fmt.Errorf("seed system_config: %w", err)
	}

	// Add client_compatibility_mode column to system_config table
	if err := r.ensureSystemConfigColumn("client_compatibility_mode", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add silent_mode column to system_config table
	if err := r.ensureSystemConfigColumn("silent_mode", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add silent_mode_timeout column to system_config table (default 15 minutes)
	if err := r.ensureSystemConfigColumn("silent_mode_timeout", "INTEGER NOT NULL DEFAULT 15"); err != nil {
		return err
	}

	// Add enable_sub_info_nodes column to system_config table
	if err := r.ensureSystemConfigColumn("enable_sub_info_nodes", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Add sub_info_expire_prefix column to system_config table
	if err := r.ensureSystemConfigColumn("sub_info_expire_prefix", "TEXT NOT NULL DEFAULT '📅过期时间'"); err != nil {
		return err
	}

	// Add sub_info_traffic_prefix column to system_config table
	if err := r.ensureSystemConfigColumn("sub_info_traffic_prefix", "TEXT NOT NULL DEFAULT '⌛剩余流量'"); err != nil {
		return err
	}

	// Add enable_short_link column to system_config table (default enabled)
	if err := r.ensureSystemConfigColumn("enable_short_link", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	// Add enable_sub_traffic_header column to system_config table (default enabled)
	if err := r.ensureSystemConfigColumn("enable_sub_traffic_header", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}

	const customRulesSchema = `
CREATE TABLE IF NOT EXISTS custom_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('dns','rules','rule-providers')),
    mode TEXT NOT NULL CHECK (mode IN ('replace','prepend','append')),
    content TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, type)
);
CREATE INDEX IF NOT EXISTS idx_custom_rules_type ON custom_rules(type);
CREATE INDEX IF NOT EXISTS idx_custom_rules_enabled ON custom_rules(enabled);
`

	if _, err := r.db.Exec(customRulesSchema); err != nil {
		return fmt.Errorf("migrate custom_rules: %w", err)
	}

	// Migrate existing custom_rules table to support 'append' mode
	if err := r.migrateCustomRulesAppendMode(); err != nil {
		return fmt.Errorf("migrate custom_rules append mode: %w", err)
	}

	// Add auto_sync_custom_rules column to subscribe_files table
	if err := r.ensureSubscribeFileColumn("auto_sync_custom_rules", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// 添加 template_filename 字段，用于绑定 V3 模板
	if err := r.ensureSubscribeFileColumn("template_filename", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	// 添加 selected_tags 字段，用于存储选中的节点标签（JSON 数组）
	if err := r.ensureSubscribeFileColumn("selected_tags", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}

	// 添加 sort_order 字段，用于自定义排序
	if err := r.ensureSubscribeFileColumn("sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// Create custom_rule_applications table for tracking applied content
	const customRuleApplicationsSchema = `
CREATE TABLE IF NOT EXISTS custom_rule_applications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscribe_file_id INTEGER NOT NULL,
    custom_rule_id INTEGER NOT NULL,
    rule_type TEXT NOT NULL,
    rule_mode TEXT NOT NULL,
    applied_content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (subscribe_file_id) REFERENCES subscribe_files(id) ON DELETE CASCADE,
    FOREIGN KEY (custom_rule_id) REFERENCES custom_rules(id) ON DELETE CASCADE,
    UNIQUE(subscribe_file_id, custom_rule_id, rule_type)
);
CREATE INDEX IF NOT EXISTS idx_custom_rule_applications_file ON custom_rule_applications(subscribe_file_id);
CREATE INDEX IF NOT EXISTS idx_custom_rule_applications_rule ON custom_rule_applications(custom_rule_id);
`

	if _, err := r.db.Exec(customRuleApplicationsSchema); err != nil {
		return fmt.Errorf("migrate custom_rule_applications: %w", err)
	}

	// Templates table for ACL4SSR rule configuration
	const templatesSchema = `
CREATE TABLE IF NOT EXISTS templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'clash' CHECK (category IN ('clash','surge')),
    template_url TEXT NOT NULL DEFAULT '',
    rule_source TEXT NOT NULL DEFAULT '',
    use_proxy INTEGER NOT NULL DEFAULT 0,
    enable_include_all INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);
CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(category);
`

	if _, err := r.db.Exec(templatesSchema); err != nil {
		return fmt.Errorf("migrate templates: %w", err)
	}

	// Proxy provider configs table
	const proxyProviderConfigsSchema = `
CREATE TABLE IF NOT EXISTS proxy_provider_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    external_subscription_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'http',
    interval INTEGER DEFAULT 3600,
    proxy TEXT DEFAULT 'DIRECT',
    size_limit INTEGER DEFAULT 0,
    header TEXT,
    health_check_enabled INTEGER DEFAULT 1,
    health_check_url TEXT DEFAULT 'https://www.gstatic.com/generate_204',
    health_check_interval INTEGER DEFAULT 300,
    health_check_timeout INTEGER DEFAULT 5000,
    health_check_lazy INTEGER DEFAULT 1,
    health_check_expected_status INTEGER DEFAULT 204,
    filter TEXT,
    exclude_filter TEXT,
    exclude_type TEXT,
    geo_ip_filter TEXT,
    override TEXT,
    process_mode TEXT DEFAULT 'client',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (external_subscription_id) REFERENCES external_subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_proxy_provider_configs_username ON proxy_provider_configs(username);
CREATE INDEX IF NOT EXISTS idx_proxy_provider_configs_external_subscription_id ON proxy_provider_configs(external_subscription_id);
`
	if _, err := r.db.Exec(proxyProviderConfigsSchema); err != nil {
		return fmt.Errorf("migrate proxy_provider_configs: %w", err)
	}

	// 添加 geo_ip_filter 列（为旧数据库迁移）
	if err := r.ensureProxyProviderConfigColumn("geo_ip_filter", "TEXT"); err != nil {
		return fmt.Errorf("ensure geo_ip_filter column: %w", err)
	}

	return nil
}

// ListSubscriptionLinks returns all configured subscription links ordered by creation.
func (r *TrafficRepository) ListSubscriptionLinks(ctx context.Context) ([]SubscriptionLink, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list subscription links: %w", err)
	}
	defer rows.Close()

	var links []SubscriptionLink
	for rows.Next() {
		link, err := scanSubscriptionLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan subscription link: %w", err)
		}
		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscription links: %w", err)
	}

	return links, nil
}

// GetSubscriptionByName retrieves a subscription link by its unique name.
func (r *TrafficRepository) GetSubscriptionByName(ctx context.Context, name string) (SubscriptionLink, error) {
	var link SubscriptionLink
	if r == nil || r.db == nil {
		return link, errors.New("traffic repository not initialized")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return link, errors.New("subscription name is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links WHERE name = ? LIMIT 1`, name)
	result, err := scanSubscriptionLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return link, ErrSubscriptionNotFound
		}
		return link, fmt.Errorf("get subscription by name: %w", err)
	}

	return result, nil
}

// GetSubscriptionByID retrieves a subscription link by its identifier.
func (r *TrafficRepository) GetSubscriptionByID(ctx context.Context, id int64) (SubscriptionLink, error) {
	var link SubscriptionLink
	if r == nil || r.db == nil {
		return link, errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return link, errors.New("subscription id is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links WHERE id = ? LIMIT 1`, id)
	result, err := scanSubscriptionLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return link, ErrSubscriptionNotFound
		}
		return link, fmt.Errorf("get subscription by id: %w", err)
	}

	return result, nil
}

// GetFirstSubscriptionLink returns the earliest created subscription link.
func (r *TrafficRepository) GetFirstSubscriptionLink(ctx context.Context) (SubscriptionLink, error) {
	var link SubscriptionLink
	if r == nil || r.db == nil {
		return link, errors.New("traffic repository not initialized")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, type, COALESCE(description, ''), rule_filename, buttons, COALESCE(short_url, ''), created_at, updated_at FROM subscription_links ORDER BY id ASC LIMIT 1`)
	result, err := scanSubscriptionLink(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return link, ErrSubscriptionNotFound
		}
		return link, fmt.Errorf("get first subscription: %w", err)
	}

	return result, nil
}

// CreateSubscriptionLink inserts a new subscription link definition.
func (r *TrafficRepository) CreateSubscriptionLink(ctx context.Context, link SubscriptionLink) (SubscriptionLink, error) {
	if r == nil || r.db == nil {
		return SubscriptionLink{}, errors.New("traffic repository not initialized")
	}

	link.Name = strings.TrimSpace(link.Name)
	link.Type = strings.TrimSpace(link.Type)
	link.Description = strings.TrimSpace(link.Description)
	link.RuleFilename = strings.TrimSpace(link.RuleFilename)

	if link.Name == "" {
		return SubscriptionLink{}, errors.New("subscription name is required")
	}
	if link.Type == "" {
		link.Type = link.Name
	}
	if link.RuleFilename == "" {
		return SubscriptionLink{}, errors.New("rule filename is required")
	}

	encodedButtons, err := encodeSubscriptionButtons(link.Buttons)
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("encode subscription buttons: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `INSERT INTO subscription_links (name, type, description, rule_filename, buttons) VALUES (?, ?, ?, ?, ?)`, link.Name, link.Type, link.Description, link.RuleFilename, encodedButtons)
	if err != nil {
		lowered := strings.ToLower(err.Error())
		if strings.Contains(lowered, "unique") {
			return SubscriptionLink{}, ErrSubscriptionExists
		}
		return SubscriptionLink{}, fmt.Errorf("create subscription link: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("fetch subscription id: %w", err)
	}

	// Note: subscription_links uses old short URL system, not the new composite short link system
	// The new composite system (file_short_code + user_short_code) is used by subscribe_files table

	return r.GetSubscriptionByID(ctx, id)
}

// UpdateSubscriptionLink updates an existing subscription link.
func (r *TrafficRepository) UpdateSubscriptionLink(ctx context.Context, link SubscriptionLink) (SubscriptionLink, error) {
	if r == nil || r.db == nil {
		return SubscriptionLink{}, errors.New("traffic repository not initialized")
	}

	if link.ID <= 0 {
		return SubscriptionLink{}, errors.New("subscription id is required")
	}

	link.Name = strings.TrimSpace(link.Name)
	link.Type = strings.TrimSpace(link.Type)
	link.Description = strings.TrimSpace(link.Description)
	link.RuleFilename = strings.TrimSpace(link.RuleFilename)

	if link.Name == "" {
		return SubscriptionLink{}, errors.New("subscription name is required")
	}
	if link.Type == "" {
		link.Type = link.Name
	}
	if link.RuleFilename == "" {
		return SubscriptionLink{}, errors.New("rule filename is required")
	}

	encodedButtons, err := encodeSubscriptionButtons(link.Buttons)
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("encode subscription buttons: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `UPDATE subscription_links SET name = ?, type = ?, description = ?, rule_filename = ?, buttons = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, link.Name, link.Type, link.Description, link.RuleFilename, encodedButtons, link.ID)
	if err != nil {
		lowered := strings.ToLower(err.Error())
		if strings.Contains(lowered, "unique") {
			return SubscriptionLink{}, ErrSubscriptionExists
		}
		return SubscriptionLink{}, fmt.Errorf("update subscription link: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return SubscriptionLink{}, fmt.Errorf("subscription update rows affected: %w", err)
	}
	if affected == 0 {
		return SubscriptionLink{}, ErrSubscriptionNotFound
	}

	return r.GetSubscriptionByID(ctx, link.ID)
}

// DeleteSubscriptionLink removes a subscription link definition.
func (r *TrafficRepository) DeleteSubscriptionLink(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	if id <= 0 {
		return errors.New("subscription id is required")
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM subscription_links WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscription link: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("subscription delete rows affected: %w", err)
	}
	if affected == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// CountSubscriptionsByFilename returns how many subscriptions reference the given rule filename.
func (r *TrafficRepository) CountSubscriptionsByFilename(ctx context.Context, filename string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return 0, errors.New("rule filename is required")
	}

	var count int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM subscription_links WHERE rule_filename = ?`, filename).Scan(&count); err != nil {
		return 0, fmt.Errorf("count subscription by filename: %w", err)
	}

	return count, nil
}

func (r *TrafficRepository) ensureUserColumn(name, definition string) error {
	return r.ensureColumn("users", name, definition)
}

func (r *TrafficRepository) ensureUserTokenColumn(name, definition string) error {
	return r.ensureColumn("user_tokens", name, definition)
}

func (r *TrafficRepository) ensureSubscriptionLinkColumn(name, definition string) error {
	return r.ensureColumn("subscription_links", name, definition)
}

func (r *TrafficRepository) ensureNodeColumn(name, definition string) error {
	return r.ensureColumn("nodes", name, definition)
}

func (r *TrafficRepository) ensureUserSettingsColumn(name, definition string) error {
	return r.ensureColumn("user_settings", name, definition)
}

func (r *TrafficRepository) ensureSubscribeFileColumn(name, definition string) error {
	return r.ensureColumn("subscribe_files", name, definition)
}

func (r *TrafficRepository) ensureExternalSubscriptionColumn(name, definition string) error {
	return r.ensureColumn("external_subscriptions", name, definition)
}

func (r *TrafficRepository) ensureProxyProviderConfigColumn(name, definition string) error {
	return r.ensureColumn("proxy_provider_configs", name, definition)
}

func (r *TrafficRepository) ensureSystemConfigColumn(name, definition string) error {
	return r.ensureColumn("system_config", name, definition)
}

func (r *TrafficRepository) syncNicknames() error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if _, err := r.db.Exec(`UPDATE users SET nickname = username WHERE nickname IS NULL OR nickname = ''`); err != nil {
		return fmt.Errorf("sync nicknames: %w", err)
	}

	return nil
}

func (r *TrafficRepository) migrateTemplateVersionFromBool() error {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('user_settings') WHERE name = 'use_new_template_system'`).Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	_, err = r.db.Exec(`UPDATE user_settings SET template_version = CASE WHEN use_new_template_system = 1 THEN 'v2' ELSE 'v1' END WHERE template_version = 'v2'`)
	if err != nil {
		return fmt.Errorf("migrate template_version values: %w", err)
	}

	return nil
}

func (r *TrafficRepository) migrateCustomRulesAppendMode() error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO custom_rules (name, type, mode, content) VALUES ('__test_append__', 'rules', 'append', 'test')`)
	if err == nil {
		_, _ = tx.Exec(`DELETE FROM custom_rules WHERE name = '__test_append__'`)
		return tx.Commit()
	}

	if _, err := tx.Exec(`ALTER TABLE custom_rules RENAME TO custom_rules_old`); err != nil {
		return fmt.Errorf("rename old table: %w", err)
	}

	const newTableSchema = `
CREATE TABLE custom_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('dns','rules','rule-providers')),
    mode TEXT NOT NULL CHECK (mode IN ('replace','prepend','append')),
    content TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, type)
);
CREATE INDEX IF NOT EXISTS idx_custom_rules_type ON custom_rules(type);
CREATE INDEX IF NOT EXISTS idx_custom_rules_enabled ON custom_rules(enabled);
`
	if _, err := tx.Exec(newTableSchema); err != nil {
		return fmt.Errorf("create new table: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO custom_rules (id, name, type, mode, content, enabled, created_at, updated_at)
		SELECT id, name, type, mode, content, enabled, created_at, updated_at
		FROM custom_rules_old
	`); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	if _, err := tx.Exec(`DROP TABLE custom_rules_old`); err != nil {
		return fmt.Errorf("drop old table: %w", err)
	}

	return tx.Commit()
}

func (r *TrafficRepository) ensureColumn(table, name, definition string) error {
	rows, err := r.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("%s table info: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			colName    string
			colType    string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scan table info: %w", err)
		}
		if strings.EqualFold(colName, name) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table info: %w", err)
	}

	alter := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, definition)
	if _, err := r.db.Exec(alter); err != nil {
		return fmt.Errorf("add column %s: %w", name, err)
	}

	return nil
}

func generateFileShortCode() (string, error) {
	return generateShortCode()
}

func generateUserShortCode() (string, error) {
	return generateShortCode()
}

func generateShortCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 3

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	for i := range bytes {
		bytes[i] = charset[int(bytes[i])%len(charset)]
	}

	return string(bytes), nil
}

func (r *TrafficRepository) generateMissingFileShortCodes() error {
	rows, err := r.db.Query(`SELECT id FROM subscribe_files WHERE file_short_code = '' OR file_short_code IS NULL`)
	if err != nil {
		return fmt.Errorf("query subscribe files without file short codes: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan file ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate file IDs: %w", err)
	}

	for _, id := range ids {
		if err := r.resetFileShortCode(context.Background(), id); err != nil {
			return fmt.Errorf("generate missing file short code for file %d: %w", id, err)
		}
	}

	return nil
}

func (r *TrafficRepository) generateMissingUserShortCodes() error {
	rows, err := r.db.Query(`SELECT username FROM user_tokens WHERE user_short_code = '' OR user_short_code IS NULL`)
	if err != nil {
		return fmt.Errorf("query users without user short codes: %w", err)
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return fmt.Errorf("scan username: %w", err)
		}
		usernames = append(usernames, username)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate usernames: %w", err)
	}

	for _, username := range usernames {
		const maxRetries = 10
		var updated bool
		for i := 0; i < maxRetries; i++ {
			code, err := generateUserShortCode()
			if err != nil {
				return err
			}
			if _, err = r.db.Exec(`UPDATE user_tokens SET user_short_code = ? WHERE username = ?`, code, username); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") {
					continue
				}
				return fmt.Errorf("update user short code for user %s: %w", username, err)
			}
			updated = true
			break
		}
		if !updated {
			return fmt.Errorf("generate unique user short code for user %s", username)
		}
	}

	return nil
}

// ResetAllSubscriptionShortURLs resets file short codes for all subscribe_files.
func (r *TrafficRepository) ResetAllSubscriptionShortURLs(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id FROM subscribe_files`)
	if err != nil {
		return fmt.Errorf("query subscribe_files IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan file ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate file IDs: %w", err)
	}

	for _, id := range ids {
		if err := r.resetFileShortCode(ctx, id); err != nil {
			return fmt.Errorf("reset file short code for file %d: %w", id, err)
		}
	}

	return nil
}

func (r *TrafficRepository) resetFileShortCode(ctx context.Context, fileID int64) error {
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		code, err := generateFileShortCode()
		if err != nil {
			return err
		}

		_, err = r.db.ExecContext(ctx, `UPDATE subscribe_files SET file_short_code = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, code, fileID)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				continue
			}
			return fmt.Errorf("update file short code: %w", err)
		}

		return nil
	}

	return errors.New("failed to generate unique short URL after retries")
}

func (r *TrafficRepository) GetSubscriptionByShortURL(ctx context.Context, shortcode string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	shortcode = strings.TrimSpace(shortcode)
	if shortcode == "" {
		return "", errors.New("shortcode is required")
	}

	var filename string
	if err := r.db.QueryRowContext(ctx, `SELECT rule_filename FROM subscription_links WHERE short_url = ? LIMIT 1`, shortcode).Scan(&filename); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSubscriptionNotFound
		}
		return "", fmt.Errorf("query subscription by short URL: %w", err)
	}

	return filename, nil
}

func (r *TrafficRepository) GetFilenameByFileShortCode(ctx context.Context, fileShortCode string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	fileShortCode = strings.TrimSpace(fileShortCode)
	if fileShortCode == "" {
		return "", errors.New("file short code is required")
	}

	var filename string
	if err := r.db.QueryRowContext(ctx, `SELECT filename FROM subscribe_files WHERE file_short_code = ? LIMIT 1`, fileShortCode).Scan(&filename); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSubscribeFileNotFound
		}
		return "", fmt.Errorf("query subscribe file by file short code: %w", err)
	}

	return filename, nil
}

func (r *TrafficRepository) GetFilenameByCustomShortCode(ctx context.Context, code string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return "", ErrSubscribeFileNotFound
	}

	var filename string
	if err := r.db.QueryRowContext(ctx, `SELECT filename FROM subscribe_files WHERE custom_short_code = ? LIMIT 1`, code).Scan(&filename); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSubscribeFileNotFound
		}
		return "", fmt.Errorf("query subscribe file by custom short code: %w", err)
	}

	return filename, nil
}

func (r *TrafficRepository) GetUsernameByUserShortCode(ctx context.Context, userShortCode string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	userShortCode = strings.TrimSpace(userShortCode)
	if userShortCode == "" {
		return "", errors.New("user short code is required")
	}

	var username string
	if err := r.db.QueryRowContext(ctx, `SELECT username FROM user_tokens WHERE user_short_code = ? LIMIT 1`, userShortCode).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("user not found")
		}
		return "", fmt.Errorf("query user by user short code: %w", err)
	}

	return username, nil
}

func (r *TrafficRepository) GetUserShortCode(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	var code string
	if err := r.db.QueryRowContext(ctx, `SELECT user_short_code FROM user_tokens WHERE username = ? LIMIT 1`, username).Scan(&code); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("user short code not found")
		}
		return "", fmt.Errorf("query user short code: %w", err)
	}

	return code, nil
}

func (r *TrafficRepository) GetEffectiveUserShortCode(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	var userCode, customCode string
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(user_short_code, ''), COALESCE(custom_user_short_code, '') FROM user_tokens WHERE username = ? LIMIT 1`, username).Scan(&userCode, &customCode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("user short code not found")
		}
		return "", fmt.Errorf("query effective user short code: %w", err)
	}
	if customCode != "" {
		return customCode, nil
	}

	return userCode, nil
}

func (r *TrafficRepository) GetAllFileShortCodes(ctx context.Context) (map[string]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(file_short_code, ''), COALESCE(custom_short_code, ''), filename FROM subscribe_files`)
	if err != nil {
		return nil, fmt.Errorf("query all file short codes: %w", err)
	}
	defer rows.Close()

	codes := make(map[string]string)
	for rows.Next() {
		var fileCode, customCode, filename string
		if err := rows.Scan(&fileCode, &customCode, &filename); err != nil {
			return nil, fmt.Errorf("scan file short code: %w", err)
		}
		if customCode != "" {
			codes[customCode] = filename
		}
		if fileCode != "" {
			codes[fileCode] = filename
		}
	}

	return codes, rows.Err()
}

func (r *TrafficRepository) GetAllUserShortCodes(ctx context.Context) (map[string]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(user_short_code, ''), COALESCE(custom_user_short_code, ''), username FROM user_tokens`)
	if err != nil {
		return nil, fmt.Errorf("query all user short codes: %w", err)
	}
	defer rows.Close()

	codes := make(map[string]string)
	for rows.Next() {
		var userCode, customCode, username string
		if err := rows.Scan(&userCode, &customCode, &username); err != nil {
			return nil, fmt.Errorf("scan user short code: %w", err)
		}
		if customCode != "" {
			codes[customCode] = username
		}
		if userCode != "" {
			codes[userCode] = username
		}
	}

	return codes, rows.Err()
}

// User represents an authenticated account stored in the repository.
type User struct {
	Username     string
	PasswordHash string
	Email        string
	Nickname     string
	AvatarURL    string
	Role         string
	IsActive     bool
	Remark       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserProfileUpdate captures editable profile fields for a user.
type UserProfileUpdate struct {
	Email     string
	Nickname  string
	AvatarURL string
}

func (r *TrafficRepository) EnsureUser(ctx context.Context, username, passwordHash string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}

	_, err := r.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, nickname, role) VALUES (?, ?, ?, ?) ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash, updated_at = CURRENT_TIMESTAMP`, username, passwordHash, username, RoleUser)
	if err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}

	return nil
}

func (r *TrafficRepository) CreateUser(ctx context.Context, username, email, nickname, passwordHash, role, remark string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	nickname = strings.TrimSpace(nickname)
	role = strings.ToLower(strings.TrimSpace(role))
	remark = strings.TrimSpace(remark)

	if username == "" {
		return errors.New("username is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}
	if nickname == "" {
		nickname = username
	}
	if role != RoleAdmin {
		role = RoleUser
	}

	_, err := r.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, email, nickname, role, is_active, remark) VALUES (?, ?, ?, ?, ?, 1, ?)`, username, passwordHash, email, nickname, role, remark)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

func (r *TrafficRepository) GetUser(ctx context.Context, username string) (User, error) {
	var user User
	if r == nil || r.db == nil {
		return user, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return user, errors.New("username is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT username, password_hash, COALESCE(email, ''), COALESCE(nickname, ''), COALESCE(avatar_url, ''), COALESCE(role, ''), COALESCE(is_active, 1), COALESCE(remark, ''), created_at, updated_at FROM users WHERE username = ? LIMIT 1`, username)
	var active int
	if err := row.Scan(&user.Username, &user.PasswordHash, &user.Email, &user.Nickname, &user.AvatarURL, &user.Role, &active, &user.Remark, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, ErrUserNotFound
		}
		return user, fmt.Errorf("get user: %w", err)
	}
	if user.Nickname == "" {
		user.Nickname = user.Username
	}
	if user.Role == "" {
		user.Role = RoleUser
	}
	user.IsActive = active != 0

	return user, nil
}

func (r *TrafficRepository) GetAdminUsername(ctx context.Context) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	var username string
	if err := r.db.QueryRowContext(ctx, `SELECT username FROM users WHERE role = ? ORDER BY created_at ASC LIMIT 1`, RoleAdmin).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", fmt.Errorf("get admin username: %w", err)
	}

	return username, nil
}

func (r *TrafficRepository) ListUsers(ctx context.Context, limit int) ([]User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	query := `SELECT username, password_hash, COALESCE(email, ''), COALESCE(nickname, ''), COALESCE(avatar_url, ''), COALESCE(role, ''), COALESCE(is_active, 1), COALESCE(remark, ''), created_at, updated_at FROM users ORDER BY created_at ASC`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var active int
		if err := rows.Scan(&user.Username, &user.PasswordHash, &user.Email, &user.Nickname, &user.AvatarURL, &user.Role, &active, &user.Remark, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if user.Nickname == "" {
			user.Nickname = user.Username
		}
		if user.Role == "" {
			user.Role = RoleUser
		}
		user.IsActive = active != 0
		users = append(users, user)
	}

	return users, rows.Err()
}

func (r *TrafficRepository) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}

	return r.execUserUpdate(ctx, `UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, passwordHash, username)
}

func (r *TrafficRepository) UpdateUserRole(ctx context.Context, username, role string) error {
	role = strings.ToLower(strings.TrimSpace(role))
	if role != RoleAdmin {
		role = RoleUser
	}
	return r.execUserUpdate(ctx, `UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, role, strings.TrimSpace(username))
}

func (r *TrafficRepository) UpdateUserStatus(ctx context.Context, username string, active bool) error {
	value := 0
	if active {
		value = 1
	}
	return r.execUserUpdate(ctx, `UPDATE users SET is_active = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, value, strings.TrimSpace(username))
}

func (r *TrafficRepository) UpdateUserRemark(ctx context.Context, username, remark string) error {
	return r.execUserUpdate(ctx, `UPDATE users SET remark = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, remark, strings.TrimSpace(username))
}

func (r *TrafficRepository) UpdateUserProfile(ctx context.Context, username string, profile UserProfileUpdate) error {
	email := strings.TrimSpace(profile.Email)
	nickname := strings.TrimSpace(profile.Nickname)
	avatar := strings.TrimSpace(profile.AvatarURL)
	username = strings.TrimSpace(username)
	if nickname == "" {
		nickname = username
	}
	return r.execUserUpdate(ctx, `UPDATE users SET email = ?, nickname = ?, avatar_url = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, email, nickname, avatar, username)
}

func (r *TrafficRepository) execUserUpdate(ctx context.Context, stmt string, args ...any) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	res, err := r.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("user update rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *TrafficRepository) RenameUser(ctx context.Context, oldUsername, newUsername string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	oldUsername = strings.TrimSpace(oldUsername)
	newUsername = strings.TrimSpace(newUsername)
	if oldUsername == "" || newUsername == "" {
		return errors.New("usernames are required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rename user begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `UPDATE users SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, newUsername, oldUsername)
	if err != nil {
		return fmt.Errorf("rename user: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rename user rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	for _, stmt := range []string{
		`UPDATE user_tokens SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`,
		`UPDATE sessions SET username = ? WHERE username = ?`,
		`UPDATE user_subscriptions SET username = ? WHERE username = ?`,
		`UPDATE user_settings SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`,
		`UPDATE nodes SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`,
		`UPDATE external_subscriptions SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, newUsername, oldUsername); err != nil {
			return fmt.Errorf("rename related user data: %w", err)
		}
	}

	return tx.Commit()
}

func (r *TrafficRepository) DeleteUser(ctx context.Context, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`DELETE FROM user_subscriptions WHERE username = ?`,
		`DELETE FROM sessions WHERE username = ?`,
		`DELETE FROM nodes WHERE username = ?`,
		`DELETE FROM external_subscriptions WHERE username = ?`,
		`DELETE FROM user_settings WHERE username = ?`,
		`DELETE FROM user_tokens WHERE username = ?`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, username); err != nil {
			return fmt.Errorf("delete related user data: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return tx.Commit()
}

func (r *TrafficRepository) GetOrCreateUserToken(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	var token string
	err := r.db.QueryRowContext(ctx, `SELECT token FROM user_tokens WHERE username = ? LIMIT 1`, username).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("query user token: %w", err)
	}

	token, err = generateRepositoryToken()
	if err != nil {
		return "", err
	}

	userShortCode, err := generateUserShortCode()
	if err != nil {
		return "", err
	}

	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		_, err = r.db.ExecContext(ctx, `INSERT INTO user_tokens (username, token, user_short_code) VALUES (?, ?, ?)`, username, token, userShortCode)
		if err == nil {
			return token, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			userShortCode, err = generateUserShortCode()
			if err != nil {
				return "", err
			}
			continue
		}
		return "", fmt.Errorf("create user token: %w", err)
	}

	return "", errors.New("failed to generate unique user token short code")
}

func (r *TrafficRepository) ResetUserToken(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}

	token, err := generateRepositoryToken()
	if err != nil {
		return "", err
	}
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		userShortCode, err := generateUserShortCode()
		if err != nil {
			return "", err
		}
		_, err = r.db.ExecContext(ctx, `INSERT INTO user_tokens (username, token, user_short_code, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(username) DO UPDATE SET token = excluded.token, user_short_code = excluded.user_short_code, updated_at = CURRENT_TIMESTAMP`, username, token, userShortCode)
		if err == nil {
			return token, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			continue
		}
		return "", fmt.Errorf("reset user token: %w", err)
	}
	return "", errors.New("failed to generate unique user short code")
}

func (r *TrafficRepository) ValidateUserToken(ctx context.Context, token string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token is required")
	}
	var username string
	err := r.db.QueryRowContext(ctx, `SELECT username FROM user_tokens WHERE token = ? LIMIT 1`, token).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTokenNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query user token by value: %w", err)
	}
	return username, nil
}

func generateRepositoryToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return fmt.Sprintf("%x", buf), nil
}

func (r *TrafficRepository) SaveRuleVersion(ctx context.Context, filename, content, createdBy string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}
	filename = strings.TrimSpace(filename)
	createdBy = strings.TrimSpace(createdBy)
	if filename == "" || createdBy == "" {
		return 0, errors.New("filename and createdBy are required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var current sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(version) FROM rule_versions WHERE filename = ?`, filename).Scan(&current); err != nil {
		return 0, fmt.Errorf("query max version: %w", err)
	}
	version := int64(1)
	if current.Valid {
		version = current.Int64 + 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rule_versions (filename, version, content, created_by) VALUES (?, ?, ?, ?)`, filename, version, content, createdBy); err != nil {
		return 0, fmt.Errorf("insert rule version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit rule version: %w", err)
	}
	return version, nil
}

func (r *TrafficRepository) ListRuleVersions(ctx context.Context, filename string, limit int) ([]RuleVersion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, errors.New("filename is required")
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `SELECT version, content, created_by, created_at FROM rule_versions WHERE filename = ? ORDER BY version DESC LIMIT ?`, filename, limit)
	if err != nil {
		return nil, fmt.Errorf("query rule versions: %w", err)
	}
	defer rows.Close()
	var versions []RuleVersion
	for rows.Next() {
		var version RuleVersion
		version.Filename = filename
		if err := rows.Scan(&version.Version, &version.Content, &version.CreatedBy, &version.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan rule version: %w", err)
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (r *TrafficRepository) LatestRuleVersion(ctx context.Context, filename string) (RuleVersion, error) {
	versions, err := r.ListRuleVersions(ctx, filename, 1)
	if err != nil {
		return RuleVersion{}, err
	}
	if len(versions) == 0 {
		return RuleVersion{}, ErrRuleVersionNotFound
	}
	return versions[0], nil
}

func (r *TrafficRepository) UpdateUserCustomShortCode(ctx context.Context, username, code string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	code = strings.TrimSpace(code)
	if username == "" {
		return errors.New("username is required")
	}
	if _, err := r.GetOrCreateUserToken(ctx, username); err != nil {
		return fmt.Errorf("ensure user token exists: %w", err)
	}
	return r.execUserUpdate(ctx, `UPDATE user_tokens SET custom_user_short_code = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, code, username)
}

func (r *TrafficRepository) GetUserCustomShortCode(ctx context.Context, username string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}
	var code string
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(custom_user_short_code, '') FROM user_tokens WHERE username = ? LIMIT 1`, username).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query user custom short code: %w", err)
	}
	return code, nil
}

func (r *TrafficRepository) GetUserSubscriptionIDs(ctx context.Context, username string) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT us.subscription_id FROM user_subscriptions us INNER JOIN subscribe_files sf ON us.subscription_id = sf.id WHERE us.username = ? ORDER BY us.created_at ASC`, username)
	if err != nil {
		return nil, fmt.Errorf("get user subscription IDs: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan subscription ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *TrafficRepository) SetUserSubscriptions(ctx context.Context, username string, subscriptionIDs []int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_subscriptions WHERE username = ?`, username); err != nil {
		return fmt.Errorf("delete existing subscriptions: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO user_subscriptions (username, subscription_id) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer stmt.Close()
	for _, id := range subscriptionIDs {
		if id <= 0 {
			continue
		}
		if _, err := stmt.ExecContext(ctx, username, id); err != nil {
			return fmt.Errorf("insert subscription %d: %w", id, err)
		}
	}
	return tx.Commit()
}

func (r *TrafficRepository) GetUserSubscriptions(ctx context.Context, username string) ([]SubscribeFile, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT s.id, s.name, COALESCE(s.description, ''), COALESCE(s.url, ''), s.type, s.filename, COALESCE(s.file_short_code, ''), COALESCE(s.custom_short_code, ''), COALESCE(s.auto_sync_custom_rules, 0), COALESCE(s.template_filename, ''), COALESCE(s.selected_tags, '[]'), s.expire_at, COALESCE(s.raw_output, 0), COALESCE(s.sort_order, 0), s.created_at, s.updated_at, s.traffic_limit FROM subscribe_files s INNER JOIN user_subscriptions us ON s.id = us.subscription_id WHERE us.username = ? ORDER BY s.sort_order ASC, s.created_at DESC`, username)
	if err != nil {
		return nil, fmt.Errorf("get user subscriptions: %w", err)
	}
	defer rows.Close()
	var files []SubscribeFile
	for rows.Next() {
		var file SubscribeFile
		var autoSync, rawOutput int
		var expireAt sql.NullTime
		var selectedTagsJSON string
		var trafficLimit sql.NullFloat64
		if err := rows.Scan(&file.ID, &file.Name, &file.Description, &file.URL, &file.Type, &file.Filename, &file.FileShortCode, &file.CustomShortCode, &autoSync, &file.TemplateFilename, &selectedTagsJSON, &expireAt, &rawOutput, &file.SortOrder, &file.CreatedAt, &file.UpdatedAt, &trafficLimit); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		file.AutoSyncCustomRules = autoSync != 0
		file.RawOutput = rawOutput != 0
		if expireAt.Valid {
			file.ExpireAt = &expireAt.Time
		}
		if trafficLimit.Valid {
			v := trafficLimit.Float64
			file.TrafficLimit = &v
		}
		if selectedTagsJSON != "" && selectedTagsJSON != "[]" {
			_ = json.Unmarshal([]byte(selectedTagsJSON), &file.SelectedTags)
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

// Session represents an authenticated session stored in the database.
type Session struct {
	Token     string
	Username  string
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (r *TrafficRepository) CreateSession(ctx context.Context, token, username string, expiresAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	token = strings.TrimSpace(token)
	username = strings.TrimSpace(username)
	if token == "" || username == "" {
		return errors.New("token and username are required")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO sessions (token, username, expires_at) VALUES (?, ?, ?)`, token, username, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *TrafficRepository) LoadSessions(ctx context.Context) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT token, username, expires_at, created_at FROM sessions WHERE expires_at > CURRENT_TIMESTAMP`)
	if err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.Token, &session.Username, &session.ExpiresAt, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (r *TrafficRepository) CleanupExpiredSessions(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}
	return nil
}

func (r *TrafficRepository) DeleteSession(ctx context.Context, token string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is required")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *TrafficRepository) DeleteUserSessions(ctx context.Context, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (r *TrafficRepository) GetUserSettings(ctx context.Context, username string) (UserSettings, error) {
	var settings UserSettings
	if r == nil || r.db == nil {
		return settings, errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return settings, errors.New("username is required")
	}

	const stmt = `SELECT username, force_sync_external, COALESCE(match_rule, 'node_name'), COALESCE(sync_scope, 'saved_only'), COALESCE(keep_node_name, 1), COALESCE(cache_expire_minutes, 0), COALESCE(sync_traffic, 0), COALESCE(custom_rules_enabled, 0), COALESCE(template_version, 'v2'), COALESCE(enable_proxy_provider, 0), COALESCE(node_order, '[]'), COALESCE(node_name_filter, '剩余|流量|到期|订阅|时间|重置'), COALESCE(debug_enabled, 0), COALESCE(debug_log_path, ''), debug_started_at, created_at, updated_at FROM user_settings WHERE username = ? LIMIT 1`
	var forceSyncInt, keepNodeNameInt, syncTrafficInt, customRulesEnabledInt, enableProxyProviderInt, debugEnabledInt int
	var nodeOrderJSON string
	var debugStartedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, stmt, username).Scan(&settings.Username, &forceSyncInt, &settings.MatchRule, &settings.SyncScope, &keepNodeNameInt, &settings.CacheExpireMinutes, &syncTrafficInt, &customRulesEnabledInt, &settings.TemplateVersion, &enableProxyProviderInt, &nodeOrderJSON, &settings.NodeNameFilter, &debugEnabledInt, &settings.DebugLogPath, &debugStartedAt, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return settings, ErrUserSettingsNotFound
		}
		return settings, fmt.Errorf("get user settings: %w", err)
	}

	settings.ForceSyncExternal = forceSyncInt == 1
	settings.KeepNodeName = keepNodeNameInt == 1
	settings.SyncTraffic = syncTrafficInt == 1
	settings.CustomRulesEnabled = customRulesEnabledInt == 1
	settings.EnableProxyProvider = enableProxyProviderInt == 1
	settings.DebugEnabled = debugEnabledInt == 1
	if nodeOrderJSON != "" && nodeOrderJSON != "[]" {
		if err := json.Unmarshal([]byte(nodeOrderJSON), &settings.NodeOrder); err != nil {
			settings.NodeOrder = []int64{}
		}
	} else {
		settings.NodeOrder = []int64{}
	}
	if debugStartedAt.Valid {
		settings.DebugStartedAt = &debugStartedAt.Time
	}
	return settings, nil
}

func (r *TrafficRepository) UpsertUserSettings(ctx context.Context, settings UserSettings) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	username := strings.TrimSpace(settings.Username)
	if username == "" {
		return errors.New("username is required")
	}

	boolInt := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	matchRule := strings.TrimSpace(settings.MatchRule)
	if matchRule == "" {
		matchRule = "node_name"
	}
	syncScope := strings.TrimSpace(settings.SyncScope)
	if syncScope == "" {
		syncScope = "saved_only"
	}
	templateVersion := strings.TrimSpace(settings.TemplateVersion)
	if templateVersion == "" {
		templateVersion = "v2"
	}
	nodeNameFilter := strings.TrimSpace(settings.NodeNameFilter)
	if nodeNameFilter == "" {
		nodeNameFilter = "剩余|流量|到期|订阅|时间|重置"
	}
	nodeOrderJSON := "[]"
	if len(settings.NodeOrder) > 0 {
		if b, err := json.Marshal(settings.NodeOrder); err == nil {
			nodeOrderJSON = string(b)
		}
	}
	cacheExpireMinutes := settings.CacheExpireMinutes
	if cacheExpireMinutes < 0 {
		cacheExpireMinutes = 0
	}

	const stmt = `
		INSERT INTO user_settings (username, force_sync_external, match_rule, sync_scope, keep_node_name, cache_expire_minutes, sync_traffic, custom_rules_enabled, template_version, enable_proxy_provider, node_order, node_name_filter, debug_enabled, debug_log_path, debug_started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(username) DO UPDATE SET
			force_sync_external = excluded.force_sync_external,
			match_rule = excluded.match_rule,
			sync_scope = excluded.sync_scope,
			keep_node_name = excluded.keep_node_name,
			cache_expire_minutes = excluded.cache_expire_minutes,
			sync_traffic = excluded.sync_traffic,
			custom_rules_enabled = excluded.custom_rules_enabled,
			template_version = excluded.template_version,
			enable_proxy_provider = excluded.enable_proxy_provider,
			node_order = excluded.node_order,
			node_name_filter = excluded.node_name_filter,
			debug_enabled = excluded.debug_enabled,
			debug_log_path = excluded.debug_log_path,
			debug_started_at = excluded.debug_started_at,
			updated_at = CURRENT_TIMESTAMP`
	if _, err := r.db.ExecContext(ctx, stmt, username, boolInt(settings.ForceSyncExternal), matchRule, syncScope, boolInt(settings.KeepNodeName), cacheExpireMinutes, boolInt(settings.SyncTraffic), boolInt(settings.CustomRulesEnabled), templateVersion, boolInt(settings.EnableProxyProvider), nodeOrderJSON, nodeNameFilter, boolInt(settings.DebugEnabled), settings.DebugLogPath, settings.DebugStartedAt); err != nil {
		return fmt.Errorf("upsert user settings: %w", err)
	}
	return nil
}

func (r *TrafficRepository) ListCustomRules(ctx context.Context, ruleType string) ([]CustomRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	query := `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules`
	args := []any{}
	if strings.TrimSpace(ruleType) != "" {
		query += ` WHERE type = ?`
		args = append(args, strings.TrimSpace(ruleType))
	}
	query += ` ORDER BY created_at DESC`
	return r.queryCustomRules(ctx, query, args...)
}

func (r *TrafficRepository) ListEnabledCustomRules(ctx context.Context, ruleType string) ([]CustomRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	query := `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules WHERE enabled = 1`
	args := []any{}
	if strings.TrimSpace(ruleType) != "" {
		query += ` AND type = ?`
		args = append(args, strings.TrimSpace(ruleType))
	}
	query += ` ORDER BY created_at DESC`
	return r.queryCustomRules(ctx, query, args...)
}

func (r *TrafficRepository) queryCustomRules(ctx context.Context, query string, args ...any) ([]CustomRule, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query custom rules: %w", err)
	}
	defer rows.Close()

	var rules []CustomRule
	for rows.Next() {
		var rule CustomRule
		var enabled int
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Mode, &rule.Content, &enabled, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan custom rule: %w", err)
		}
		rule.Enabled = enabled != 0
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *TrafficRepository) GetCustomRule(ctx context.Context, id int64) (*CustomRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	var rule CustomRule
	var enabled int
	err := r.db.QueryRowContext(ctx, `SELECT id, name, type, mode, content, enabled, created_at, updated_at FROM custom_rules WHERE id = ?`, id).Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Mode, &rule.Content, &enabled, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCustomRuleNotFound
		}
		return nil, fmt.Errorf("get custom rule: %w", err)
	}
	rule.Enabled = enabled != 0
	return &rule, nil
}

func (r *TrafficRepository) CreateCustomRule(ctx context.Context, rule *CustomRule) error {
	if err := validateCustomRule(rule); err != nil {
		return err
	}
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO custom_rules (name, type, mode, content, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, rule.Name, rule.Type, rule.Mode, rule.Content, enabled)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return errors.New("custom rule with this name and type already exists")
		}
		return fmt.Errorf("create custom rule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	rule.ID = id
	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return nil
}

func (r *TrafficRepository) UpdateCustomRule(ctx context.Context, rule *CustomRule) error {
	if err := validateCustomRule(rule); err != nil {
		return err
	}
	if rule.ID <= 0 {
		return errors.New("custom rule id is required")
	}
	enabled := 0
	if rule.Enabled {
		enabled = 1
	}
	res, err := r.db.ExecContext(ctx, `UPDATE custom_rules SET name = ?, type = ?, mode = ?, content = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, rule.Name, rule.Type, rule.Mode, rule.Content, enabled, rule.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return errors.New("custom rule with this name and type already exists")
		}
		return fmt.Errorf("update custom rule: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("custom rule rows affected: %w", err)
	}
	if affected == 0 {
		return ErrCustomRuleNotFound
	}
	rule.UpdatedAt = time.Now()
	return nil
}

func (r *TrafficRepository) DeleteCustomRule(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM custom_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete custom rule: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("custom rule rows affected: %w", err)
	}
	if affected == 0 {
		return ErrCustomRuleNotFound
	}
	return nil
}

func validateCustomRule(rule *CustomRule) error {
	if rule == nil {
		return errors.New("custom rule is required")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Type = strings.TrimSpace(rule.Type)
	rule.Mode = strings.TrimSpace(rule.Mode)
	rule.Content = strings.TrimSpace(rule.Content)
	if rule.Name == "" {
		return errors.New("custom rule name is required")
	}
	if rule.Type != "dns" && rule.Type != "rules" && rule.Type != "rule-providers" {
		return errors.New("custom rule type must be 'dns', 'rules', or 'rule-providers'")
	}
	if rule.Type == "dns" {
		rule.Mode = "replace"
	} else if rule.Type == "rules" {
		if rule.Mode != "replace" && rule.Mode != "prepend" && rule.Mode != "append" {
			return errors.New("custom rule mode must be 'replace', 'prepend', or 'append' for rules type")
		}
	} else if rule.Mode != "replace" && rule.Mode != "prepend" {
		return errors.New("custom rule mode must be 'replace' or 'prepend'")
	}
	if rule.Content == "" {
		return errors.New("custom rule content is required")
	}
	return nil
}

func (r *TrafficRepository) GetCustomRuleApplications(ctx context.Context, fileID int64) ([]CustomRuleApplication, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, subscribe_file_id, custom_rule_id, rule_type, rule_mode, applied_content, content_hash, applied_at FROM custom_rule_applications WHERE subscribe_file_id = ? ORDER BY applied_at DESC`, fileID)
	if err != nil {
		return nil, fmt.Errorf("get custom rule applications: %w", err)
	}
	defer rows.Close()
	var apps []CustomRuleApplication
	for rows.Next() {
		var app CustomRuleApplication
		if err := rows.Scan(&app.ID, &app.SubscribeFileID, &app.CustomRuleID, &app.RuleType, &app.RuleMode, &app.AppliedContent, &app.ContentHash, &app.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan custom rule application: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (r *TrafficRepository) UpsertCustomRuleApplication(ctx context.Context, app *CustomRuleApplication) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	if app == nil {
		return errors.New("custom rule application is required")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO custom_rule_applications (subscribe_file_id, custom_rule_id, rule_type, rule_mode, applied_content, content_hash, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(subscribe_file_id, custom_rule_id, rule_type)
		DO UPDATE SET rule_mode = excluded.rule_mode, applied_content = excluded.applied_content, content_hash = excluded.content_hash, applied_at = CURRENT_TIMESTAMP`, app.SubscribeFileID, app.CustomRuleID, app.RuleType, app.RuleMode, app.AppliedContent, app.ContentHash)
	if err != nil {
		return fmt.Errorf("upsert custom rule application: %w", err)
	}
	return nil
}

func (r *TrafficRepository) DeleteCustomRuleApplication(ctx context.Context, fileID, ruleID int64, ruleType string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM custom_rule_applications WHERE subscribe_file_id = ? AND custom_rule_id = ? AND rule_type = ?`, fileID, ruleID, ruleType)
	if err != nil {
		return fmt.Errorf("delete custom rule application: %w", err)
	}
	return nil
}

func (r *TrafficRepository) ListExternalSubscriptions(ctx context.Context, username string) ([]ExternalSubscription, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}
	return r.queryExternalSubscriptions(ctx, `SELECT id, username, name, url, COALESCE(user_agent, 'clash-meta/2.4.0'), node_count, last_sync_at, COALESCE(upload, 0), COALESCE(download, 0), COALESCE(total, 0), expire, COALESCE(traffic_mode, 'both'), created_at, updated_at FROM external_subscriptions WHERE username = ? ORDER BY created_at DESC`, username)
}

func (r *TrafficRepository) ListAllExternalSubscriptions(ctx context.Context) ([]ExternalSubscription, error) {
	return r.queryExternalSubscriptions(ctx, `SELECT id, username, name, url, COALESCE(user_agent, 'clash-meta/2.4.0'), node_count, last_sync_at, COALESCE(upload, 0), COALESCE(download, 0), COALESCE(total, 0), expire, COALESCE(traffic_mode, 'both'), created_at, updated_at FROM external_subscriptions ORDER BY created_at DESC`)
}

func (r *TrafficRepository) queryExternalSubscriptions(ctx context.Context, query string, args ...any) ([]ExternalSubscription, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query external subscriptions: %w", err)
	}
	defer rows.Close()
	var subs []ExternalSubscription
	for rows.Next() {
		var sub ExternalSubscription
		var lastSyncAt, expire sql.NullTime
		if err := rows.Scan(&sub.ID, &sub.Username, &sub.Name, &sub.URL, &sub.UserAgent, &sub.NodeCount, &lastSyncAt, &sub.Upload, &sub.Download, &sub.Total, &expire, &sub.TrafficMode, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan external subscription: %w", err)
		}
		if lastSyncAt.Valid {
			sub.LastSyncAt = &lastSyncAt.Time
		}
		if expire.Valid {
			sub.Expire = &expire.Time
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (r *TrafficRepository) GetExternalSubscription(ctx context.Context, id int64, username string) (ExternalSubscription, error) {
	var sub ExternalSubscription
	username = strings.TrimSpace(username)
	if id <= 0 || username == "" {
		return sub, errors.New("subscription id and username are required")
	}
	subs, err := r.queryExternalSubscriptions(ctx, `SELECT id, username, name, url, COALESCE(user_agent, 'clash-meta/2.4.0'), node_count, last_sync_at, COALESCE(upload, 0), COALESCE(download, 0), COALESCE(total, 0), expire, COALESCE(traffic_mode, 'both'), created_at, updated_at FROM external_subscriptions WHERE id = ? AND username = ? LIMIT 1`, id, username)
	if err != nil {
		return sub, err
	}
	if len(subs) == 0 {
		return sub, ErrExternalSubscriptionNotFound
	}
	return subs[0], nil
}

func (r *TrafficRepository) CreateExternalSubscription(ctx context.Context, sub ExternalSubscription) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}
	username := strings.TrimSpace(sub.Username)
	name := strings.TrimSpace(sub.Name)
	url := strings.TrimSpace(sub.URL)
	if username == "" || name == "" || url == "" {
		return 0, errors.New("username, name and url are required")
	}
	userAgent := strings.TrimSpace(sub.UserAgent)
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	trafficMode := strings.TrimSpace(sub.TrafficMode)
	if trafficMode == "" {
		trafficMode = "both"
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO external_subscriptions (username, name, url, user_agent, node_count, last_sync_at, upload, download, total, expire, traffic_mode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, username, name, url, userAgent, sub.NodeCount, sub.LastSyncAt, sub.Upload, sub.Download, sub.Total, sub.Expire, trafficMode)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return 0, ErrExternalSubscriptionExists
		}
		return 0, fmt.Errorf("create external subscription: %w", err)
	}
	return res.LastInsertId()
}

func (r *TrafficRepository) UpdateExternalSubscription(ctx context.Context, sub ExternalSubscription) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	if sub.ID <= 0 {
		return errors.New("subscription id is required")
	}
	username := strings.TrimSpace(sub.Username)
	name := strings.TrimSpace(sub.Name)
	url := strings.TrimSpace(sub.URL)
	if username == "" || name == "" || url == "" {
		return errors.New("username, name and url are required")
	}
	userAgent := strings.TrimSpace(sub.UserAgent)
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	trafficMode := strings.TrimSpace(sub.TrafficMode)
	if trafficMode == "" {
		trafficMode = "both"
	}
	res, err := r.db.ExecContext(ctx, `UPDATE external_subscriptions SET name = ?, url = ?, user_agent = ?, node_count = ?, last_sync_at = ?, upload = ?, download = ?, total = ?, expire = ?, traffic_mode = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`, name, url, userAgent, sub.NodeCount, sub.LastSyncAt, sub.Upload, sub.Download, sub.Total, sub.Expire, trafficMode, sub.ID, username)
	if err != nil {
		return fmt.Errorf("update external subscription: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("external subscription rows affected: %w", err)
	}
	if affected == 0 {
		return ErrExternalSubscriptionNotFound
	}
	return nil
}

func (r *TrafficRepository) DeleteExternalSubscription(ctx context.Context, id int64, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	username = strings.TrimSpace(username)
	if id <= 0 || username == "" {
		return errors.New("subscription id and username are required")
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM proxy_provider_configs WHERE external_subscription_id = ?`, id); err != nil {
		return fmt.Errorf("delete related proxy provider configs: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM external_subscriptions WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return fmt.Errorf("delete external subscription: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("external subscription rows affected: %w", err)
	}
	if affected == 0 {
		return ErrExternalSubscriptionNotFound
	}
	return nil
}

func (r *TrafficRepository) CreateProxyProviderConfig(ctx context.Context, config *ProxyProviderConfig) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}
	if config == nil {
		return 0, errors.New("proxy provider config is required")
	}
	enabled, lazy := boolToInt(config.HealthCheckEnabled), boolToInt(config.HealthCheckLazy)
	res, err := r.db.ExecContext(ctx, `INSERT INTO proxy_provider_configs (username, external_subscription_id, name, type, interval, proxy, size_limit, header, health_check_enabled, health_check_url, health_check_interval, health_check_timeout, health_check_lazy, health_check_expected_status, filter, exclude_filter, exclude_type, geo_ip_filter, override, process_mode) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		config.Username, config.ExternalSubscriptionID, config.Name, config.Type, config.Interval, config.Proxy, config.SizeLimit, config.Header,
		enabled, config.HealthCheckURL, config.HealthCheckInterval, config.HealthCheckTimeout, lazy, config.HealthCheckExpectedStatus,
		config.Filter, config.ExcludeFilter, config.ExcludeType, config.GeoIPFilter, config.Override, config.ProcessMode)
	if err != nil {
		return 0, fmt.Errorf("create proxy provider config: %w", err)
	}
	return res.LastInsertId()
}

func (r *TrafficRepository) GetProxyProviderConfig(ctx context.Context, id int64) (*ProxyProviderConfig, error) {
	configs, err := r.queryProxyProviderConfigs(ctx, `WHERE id = ?`, id)
	if err != nil || len(configs) == 0 {
		return nil, err
	}
	return &configs[0], nil
}

func (r *TrafficRepository) GetProxyProviderConfigByName(ctx context.Context, name string) (*ProxyProviderConfig, error) {
	configs, err := r.queryProxyProviderConfigs(ctx, `WHERE name = ?`, strings.TrimSpace(name))
	if err != nil || len(configs) == 0 {
		return nil, err
	}
	return &configs[0], nil
}

func (r *TrafficRepository) ListProxyProviderConfigs(ctx context.Context, username string) ([]ProxyProviderConfig, error) {
	return r.queryProxyProviderConfigs(ctx, `WHERE username = ? ORDER BY id ASC`, strings.TrimSpace(username))
}

func (r *TrafficRepository) ListProxyProviderConfigsBySubscription(ctx context.Context, externalSubscriptionID int64) ([]ProxyProviderConfig, error) {
	return r.queryProxyProviderConfigs(ctx, `WHERE external_subscription_id = ? ORDER BY id ASC`, externalSubscriptionID)
}

func (r *TrafficRepository) ListMMWProxyProviderConfigs(ctx context.Context) ([]ProxyProviderConfig, error) {
	return r.queryProxyProviderConfigs(ctx, `WHERE process_mode = 'mmw' ORDER BY id ASC`)
}

func (r *TrafficRepository) queryProxyProviderConfigs(ctx context.Context, where string, args ...any) ([]ProxyProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	query := `SELECT id, username, external_subscription_id, name, type, interval, proxy, size_limit,
		COALESCE(header, ''), health_check_enabled, health_check_url, health_check_interval,
		health_check_timeout, health_check_lazy, health_check_expected_status,
		COALESCE(filter, ''), COALESCE(exclude_filter, ''), COALESCE(exclude_type, ''),
		COALESCE(geo_ip_filter, ''), COALESCE(override, ''), process_mode, created_at, updated_at
		FROM proxy_provider_configs ` + where
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query proxy provider configs: %w", err)
	}
	defer rows.Close()
	var configs []ProxyProviderConfig
	for rows.Next() {
		var config ProxyProviderConfig
		var enabled, lazy int
		if err := rows.Scan(&config.ID, &config.Username, &config.ExternalSubscriptionID, &config.Name, &config.Type, &config.Interval, &config.Proxy, &config.SizeLimit, &config.Header, &enabled, &config.HealthCheckURL, &config.HealthCheckInterval, &config.HealthCheckTimeout, &lazy, &config.HealthCheckExpectedStatus, &config.Filter, &config.ExcludeFilter, &config.ExcludeType, &config.GeoIPFilter, &config.Override, &config.ProcessMode, &config.CreatedAt, &config.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan proxy provider config: %w", err)
		}
		config.HealthCheckEnabled = enabled != 0
		config.HealthCheckLazy = lazy != 0
		configs = append(configs, config)
	}
	return configs, rows.Err()
}

func (r *TrafficRepository) UpdateProxyProviderConfig(ctx context.Context, config *ProxyProviderConfig) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	if config == nil {
		return errors.New("proxy provider config is required")
	}
	res, err := r.db.ExecContext(ctx, `UPDATE proxy_provider_configs SET name = ?, type = ?, interval = ?, proxy = ?, size_limit = ?, header = ?, health_check_enabled = ?, health_check_url = ?, health_check_interval = ?, health_check_timeout = ?, health_check_lazy = ?, health_check_expected_status = ?, filter = ?, exclude_filter = ?, exclude_type = ?, geo_ip_filter = ?, override = ?, process_mode = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`,
		config.Name, config.Type, config.Interval, config.Proxy, config.SizeLimit, config.Header, boolToInt(config.HealthCheckEnabled), config.HealthCheckURL, config.HealthCheckInterval, config.HealthCheckTimeout, boolToInt(config.HealthCheckLazy), config.HealthCheckExpectedStatus, config.Filter, config.ExcludeFilter, config.ExcludeType, config.GeoIPFilter, config.Override, config.ProcessMode, config.ID, config.Username)
	if err != nil {
		return fmt.Errorf("update proxy provider config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("proxy provider config rows affected: %w", err)
	}
	if affected == 0 {
		return errors.New("proxy provider config not found or not owned by user")
	}
	return nil
}

func (r *TrafficRepository) DeleteProxyProviderConfig(ctx context.Context, id int64, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM proxy_provider_configs WHERE id = ? AND username = ?`, id, strings.TrimSpace(username))
	if err != nil {
		return fmt.Errorf("delete proxy provider config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("proxy provider config rows affected: %w", err)
	}
	if affected == 0 {
		return errors.New("proxy provider config not found or not owned by user")
	}
	return nil
}

func (r *TrafficRepository) GetSystemConfig(ctx context.Context) (SystemConfig, error) {
	if r == nil || r.db == nil {
		return SystemConfig{}, errors.New("traffic repository not initialized")
	}
	const query = `SELECT proxy_groups_source_url, client_compatibility_mode, silent_mode, silent_mode_timeout, enable_sub_info_nodes, sub_info_expire_prefix, sub_info_traffic_prefix, COALESCE(enable_short_link, 1), COALESCE(enable_sub_traffic_header, 1) FROM system_config WHERE id = 1`
	var cfg SystemConfig
	var compatibilityMode, silentMode, timeout, enableInfoNodes, enableShortLink, enableTrafficHeader int
	err := r.db.QueryRowContext(ctx, query).Scan(&cfg.ProxyGroupsSourceURL, &compatibilityMode, &silentMode, &timeout, &enableInfoNodes, &cfg.SubInfoExpirePrefix, &cfg.SubInfoTrafficPrefix, &enableShortLink, &enableTrafficHeader)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return defaultSystemConfig(), nil
		}
		return SystemConfig{}, fmt.Errorf("query system config: %w", err)
	}
	cfg.ClientCompatibilityMode = compatibilityMode != 0
	cfg.SilentMode = silentMode != 0
	cfg.SilentModeTimeout = timeout
	cfg.EnableSubInfoNodes = enableInfoNodes != 0
	cfg.EnableShortLink = enableShortLink != 0
	cfg.EnableSubTrafficHeader = enableTrafficHeader != 0
	applySystemConfigDefaults(&cfg)
	return cfg, nil
}

func (r *TrafficRepository) UpdateSystemConfig(ctx context.Context, cfg SystemConfig) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	applySystemConfigDefaults(&cfg)
	res, err := r.db.ExecContext(ctx, `UPDATE system_config SET proxy_groups_source_url = ?, client_compatibility_mode = ?, silent_mode = ?, silent_mode_timeout = ?, enable_sub_info_nodes = ?, sub_info_expire_prefix = ?, sub_info_traffic_prefix = ?, enable_short_link = ?, enable_sub_traffic_header = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1`,
		cfg.ProxyGroupsSourceURL, boolToInt(cfg.ClientCompatibilityMode), boolToInt(cfg.SilentMode), cfg.SilentModeTimeout, boolToInt(cfg.EnableSubInfoNodes), cfg.SubInfoExpirePrefix, cfg.SubInfoTrafficPrefix, boolToInt(cfg.EnableShortLink), boolToInt(cfg.EnableSubTrafficHeader))
	if err != nil {
		return fmt.Errorf("update system config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("system config rows affected: %w", err)
	}
	if affected > 0 {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO system_config (id, proxy_groups_source_url, client_compatibility_mode, silent_mode, silent_mode_timeout, enable_sub_info_nodes, sub_info_expire_prefix, sub_info_traffic_prefix, enable_short_link, enable_sub_traffic_header) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.ProxyGroupsSourceURL, boolToInt(cfg.ClientCompatibilityMode), boolToInt(cfg.SilentMode), cfg.SilentModeTimeout, boolToInt(cfg.EnableSubInfoNodes), cfg.SubInfoExpirePrefix, cfg.SubInfoTrafficPrefix, boolToInt(cfg.EnableShortLink), boolToInt(cfg.EnableSubTrafficHeader))
	if err != nil {
		return fmt.Errorf("insert system config: %w", err)
	}
	return nil
}

func defaultSystemConfig() SystemConfig {
	cfg := SystemConfig{}
	applySystemConfigDefaults(&cfg)
	return cfg
}

func applySystemConfigDefaults(cfg *SystemConfig) {
	if cfg.SilentModeTimeout <= 0 {
		cfg.SilentModeTimeout = 15
	}
	if cfg.SubInfoExpirePrefix == "" {
		cfg.SubInfoExpirePrefix = "📅过期时间"
	}
	if cfg.SubInfoTrafficPrefix == "" {
		cfg.SubInfoTrafficPrefix = "⌛剩余流量"
	}
	if !cfg.EnableShortLink {
		cfg.EnableShortLink = true
	}
	if !cfg.EnableSubTrafficHeader {
		cfg.EnableSubTrafficHeader = true
	}
}

func (r *TrafficRepository) GetSubscribeFilesWithAutoSync(ctx context.Context) ([]SubscribeFile, error) {
	files, err := r.GetSubscribeFilesWithTemplate(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SubscribeFile, 0, len(files))
	for _, file := range files {
		if file.AutoSyncCustomRules {
			result = append(result, file)
		}
	}
	return result, nil
}
