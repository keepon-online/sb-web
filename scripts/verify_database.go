package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const (
	dbPath = "./data/traffic.db"
)

type TableCheck struct {
	name        string
	required    bool
	columnCount int
}

var requiredTables = []TableCheck{
	// 核心表
	{name: "traffic_records", required: true, columnCount: 8},
	{name: "user_tokens", required: true, columnCount: 6},
	{name: "sessions", required: true, columnCount: 5},
	{name: "users", required: true, columnCount: 7},

	// Clash 订阅管理表
	{name: "rule_versions", required: true, columnCount: 5},
	{name: "subscription_links", required: true, columnCount: 8},
	{name: "nodes", required: true, columnCount: 12},
	{name: "subscribe_files", required: true, columnCount: 8},
	{name: "user_subscriptions", required: true, columnCount: 5},
	{name: "user_settings", required: true, columnCount: 4},
	{name: "external_subscriptions", required: true, columnCount: 8},
	{name: "system_config", required: true, columnCount: 4},
	{name: "custom_rules", required: true, columnCount: 6},
	{name: "custom_rule_applications", required: true, columnCount: 5},
	{name: "templates", required: true, columnCount: 11},
	{name: "proxy_provider_configs", required: true, columnCount: 6},

	// Sing-box 管理表
	{name: "singbox_configs", required: true, columnCount: 8},
	{name: "certificates", required: true, columnCount: 8},
	{name: "argo_tunnels", required: true, columnCount: 8},
	{name: "warp_configs", required: true, columnCount: 7},
	{name: "system_operation_logs", required: true, columnCount: 6},
	{name: "singbox_subscriptions", required: true, columnCount: 6},
	{name: "system_environment", required: true, columnCount: 5},
}

func main() {
	// 检查数据库文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Printf("❌ 数据库文件不存在: %s", dbPath)
		log.Println("请先启动服务器以创建数据库")
		os.Exit(1)
	}

	// 连接数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("❌ 无法打开数据库: %v", err)
	}
	defer db.Close()

	// 检查连接
	if err := db.Ping(); err != nil {
		log.Fatalf("❌ 无法连接到数据库: %v", err)
	}

	fmt.Println("=== MiaomiaoWu 数据库结构验证 ===\n")

	// 获取所有表名
	tables, err := getTableList(db)
	if err != nil {
		log.Fatalf("❌ 无法获取表列表: %v", err)
	}

	fmt.Printf("数据库中共有 %d 个表\n\n", len(tables))

	// 检查必需的表
	missingTables := []TableCheck{}
	existingTables := 0
	for _, required := range requiredTables {
		if tableExists(tables, required.name) {
			columns, err := getColumnCount(db, required.name)
			if err != nil {
				fmt.Printf("⚠️  表 %s 存在但无法获取列信息: %v\n", required.name, err)
				continue
			}

			existingTables++
			if columns >= required.columnCount {
				fmt.Printf("✅ %-25s (%d 列)\n", required.name, columns)
			} else {
				fmt.Printf("⚠️  %-25s (%d 列，预期至少 %d 列)\n", required.name, columns, required.columnCount)
			}
		} else if required.required {
			missingTables = append(missingTables, required)
		}
	}

	// 检查缺失的必需表
	if len(missingTables) > 0 {
		fmt.Printf("\n❌ 缺失 %d 个必需的表:\n", len(missingTables))
		for _, table := range missingTables {
			fmt.Printf("   - %s\n", table.name)
		}
		fmt.Println("\n请运行服务器以自动创建缺失的表")
		os.Exit(1)
	}

	// 检查额外的不需要的表
	fmt.Printf("\n额外的表检查:\n")
	for _, table := range tables {
		if !isRequiredTable(table) {
			fmt.Printf("ℹ️  %-25s (非标准表)\n", table)
		}
	}

	// 总结
	fmt.Printf("\n=== 验证结果 ===\n")
	fmt.Printf("✅ 所有必需的表都已存在 (%d/%d)\n", existingTables, len(requiredTables))
	fmt.Printf("✅ 数据库结构验证通过\n")
}

func getTableList(db *sql.DB) ([]string, error) {
	query := `
		SELECT name FROM sqlite_master
		WHERE type='table'
		AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	return tables, rows.Err()
}

func tableExists(tables []string, tableName string) bool {
	for _, table := range tables {
		if table == tableName {
			return true
		}
	}
	return false
}

func isRequiredTable(tableName string) bool {
	for _, required := range requiredTables {
		if required.name == tableName {
			return true
		}
	}
	return false
}

func getColumnCount(db *sql.DB, tableName string) (int, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := db.Query(query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}

	return count, rows.Err()
}
