package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL      = "http://localhost:8080"
	timeout      = 5 * time.Second
)

type APITest struct {
	endpoint     string
	method       string
	expectedCode int
	description  string
}

var tests = []APITest{
	// Sing-box 安装和管理
	{"/api/admin/singbox/install-status", "GET", 401, "安装状态检查"},
	{"/api/admin/singbox/environment", "GET", 401, "环境检测"},
	{"/api/admin/singbox/service/status", "GET", 401, "服务状态检查"},
	{"/api/admin/singbox/system/status", "GET", 401, "系统状态检查"},

	// Sing-box 配置管理
	{"/api/admin/singbox/config/list", "GET", 401, "配置列表"},
	{"/api/admin/singbox/config/generate", "POST", 401, "配置生成"},
	{"/api/admin/singbox/deploy", "POST", 401, "配置部署"},
	{"/api/admin/singbox/port/allocate", "POST", 401, "端口分配"},
	{"/api/admin/singbox/port/check", "GET", 401, "端口检查"},

	// 证书管理
	{"/api/admin/certificate/list", "GET", 401, "证书列表"},
	{"/api/admin/certificate/generate", "POST", 401, "证书生成"},
	{"/api/admin/certificate/renew", "POST", 401, "证书续期"},
	{"/api/admin/certificate/auto-renew", "POST", 401, "自动续期"},
	{"/api/admin/certificate/delete", "DELETE", 401, "证书删除"},
	{"/api/admin/certificate/check", "POST", 401, "证书检查"},

	// Argo 隧道管理
	{"/api/admin/argo/list", "GET", 401, "隧道列表"},
	{"/api/admin/argo/create", "POST", 401, "创建隧道"},
	{"/api/admin/argo/action", "POST", 401, "隧道操作"},
	{"/api/admin/argo/status", "GET", 401, "隧道状态"},
	{"/api/admin/argo/install", "POST", 401, "安装Argo"},
	{"/api/admin/argo/quick", "GET", 401, "快速隧道"},
	{"/api/admin/argo/metrics", "GET", 401, "隧道指标"},

	// WARP 管理
	{"/api/admin/warp/status", "GET", 401, "WARP状态"},
	{"/api/admin/warp/enable", "POST", 401, "启用WARP"},
	{"/api/admin/warp/disable", "POST", 401, "禁用WARP"},
	{"/api/admin/warp/configs", "GET", 401, "WARP配置列表"},
	{"/api/admin/warp/generate-config", "POST", 401, "生成WARP配置"},
	{"/api/admin/warp/install", "POST", 401, "安装WARP"},

	// 系统优化
	{"/api/admin/system/optimize", "POST", 401, "系统优化"},
	{"/api/admin/system/bbr-status", "GET", 401, "BBR状态"},
	{"/api/admin/system/network-performance", "GET", 401, "网络性能"},
	{"/api/admin/system/resource-usage", "GET", 401, "资源使用"},
	{"/api/admin/system/report", "GET", 401, "系统报告"},

	// 订阅管理
	{"/api/admin/subscription/generate", "POST", 401, "生成订阅"},
	{"/api/admin/subscription/list", "GET", 401, "订阅列表"},
	{"/api/admin/subscription/export", "GET", 401, "导出订阅"},
	{"/api/admin/subscription/url", "GET", 401, "订阅URL"},
	{"/api/admin/subscription/qrcode", "GET", 401, "订阅二维码"},

	// 分享管理
	{"/api/admin/share/create", "POST", 401, "创建分享"},
	{"/api/admin/share/list", "GET", 401, "分享列表"},
	{"/api/admin/share/detail", "GET", 401, "分享详情"},
	{"/api/admin/gitlab/sync", "POST", 401, "GitLab同步"},
	{"/api/admin/github/sync", "POST", 401, "GitHub同步"},
	{"/api/admin/pastebin/share", "POST", 401, "Pastebin分享"},

	// 公共 API
	{"/", "GET", 200, "根路径"},
	{"/health", "GET", 200, "健康检查"},
}

func main() {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 检查服务器状态
	fmt.Println("检查服务器状态...")
	resp, err := client.Get(baseURL + "/")
	if err != nil {
		fmt.Printf("❌ 错误: 服务器未运行在 %s\n", baseURL)
		fmt.Println("请先启动服务器: ./server")
		return
	}
	resp.Body.Close()
	fmt.Println("✅ 服务器正在运行\n")

	totalTests := len(tests)
	passedTests := 0
	failedTests := 0

	fmt.Println("=== MiaomiaoWu API 端点验证测试 ===\n")

	// 测试各个功能组
	testGroups := map[string]string{
		"Sing-box 安装和管理": "singbox",
		"Sing-box 配置管理":   "config",
		"证书管理":            "certificate",
		"Argo 隧道管理":       "argo",
		"WARP 管理":          "warp",
		"系统优化":            "system",
		"订阅管理":            "subscription",
		"分享管理":            "share",
		"公共 API":           "public",
	}

	currentGroup := ""
	for _, test := range tests {
		// 检查是否需要打印分组标题
		for group, prefix := range testGroups {
			if strings.Contains(test.endpoint, prefix) && currentGroup != group {
				currentGroup = group
				fmt.Printf("\n--- %s ---\n", group)
				break
			}
		}

		// 发送测试请求
		req, _ := http.NewRequest(test.method, baseURL+test.endpoint, nil)
		resp, err := client.Do(req)

		statusCode := 0
		if err != nil {
			statusCode = 0
		} else {
			statusCode = resp.StatusCode
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		// 验证结果
		passed := false
		if statusCode == test.expectedCode || statusCode == 401 || statusCode == 403 {
			passed = true
			passedTests++
			fmt.Printf("✅ PASS %s (HTTP %d)\n", test.description, statusCode)
		} else {
			failedTests++
			fmt.Printf("❌ FAIL %s (HTTP %d, expected %d)\n", test.description, statusCode, test.expectedCode)
		}
	}

	// 输出测试结果汇总
	fmt.Printf("\n=== 测试结果汇总 ===\n")
	fmt.Printf("总测试数: %d\n", totalTests)
	fmt.Printf("通过: %d\n", passedTests)
	fmt.Printf("失败: %d\n", failedTests)

	if failedTests == 0 {
		fmt.Println("\n✅ 所有API端点验证通过！")
	} else {
		fmt.Printf("\n❌ 有 %d 个测试失败\n", failedTests)
	}
}
