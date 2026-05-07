#!/bin/bash

# API端点验证脚本
# 用于验证所有主要的API端点是否正确注册和响应

BASE_URL="${API_BASE_URL:-http://localhost:8080}"
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 测试函数
test_endpoint() {
    local endpoint=$1
    local method=$2
    local expected_code=${3:-200}
    local description=$4

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    echo -n "Testing: ${description}... "

    # 发送请求
    if [ "$method" = "GET" ]; then
        response=$(curl -s -o /dev/null -w "%{http_code}" -X GET "${BASE_URL}${endpoint}")
    elif [ "$method" = "POST" ]; then
        response=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}${endpoint}")
    else
        response=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" "${BASE_URL}${endpoint}")
    fi

    # 检查响应
    if [ "$response" = "$expected_code" ] || [ "$response" = "401" ] || [ "$response" = "403" ]; then
        echo -e "${GREEN}✓ PASS${NC} (HTTP $response)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}✗ FAIL${NC} (HTTP $response, expected $expected_code)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

echo -e "${BLUE}=== MiaomiaoWu API 端点验证测试 ===${NC}\n"

# 检查服务器是否运行
echo "检查服务器状态..."
if ! curl -s -f "${BASE_URL}/" > /dev/null 2>&1; then
    echo -e "${RED}错误: 服务器未运行在 ${BASE_URL}${NC}"
    echo "请先启动服务器: ./server"
    exit 1
fi
echo -e "${GREEN}✓ 服务器正在运行${NC}\n"

# Sing-box 安装和管理 API
echo -e "${BLUE}--- Sing-box 安装和管理 ---${NC}"
test_endpoint "/api/admin/singbox/install-status" "GET" "401" "安装状态检查"
test_endpoint "/api/admin/singbox/environment" "GET" "401" "环境检测"
test_endpoint "/api/admin/singbox/service/status" "GET" "401" "服务状态检查"
test_endpoint "/api/admin/singbox/system/status" "GET" "401" "系统状态检查"

# Sing-box 配置管理 API
echo -e "\n${BLUE}--- Sing-box 配置管理 ---${NC}"
test_endpoint "/api/admin/singbox/config/list" "GET" "401" "配置列表"
test_endpoint "/api/admin/singbox/config/generate" "POST" "401" "配置生成"
test_endpoint "/api/admin/singbox/deploy" "POST" "401" "配置部署"
test_endpoint "/api/admin/singbox/port/allocate" "POST" "401" "端口分配"
test_endpoint "/api/admin/singbox/port/check" "GET" "401" "端口检查"

# 证书管理 API
echo -e "\n${BLUE}--- 证书管理 ---${NC}"
test_endpoint "/api/admin/certificate/list" "GET" "401" "证书列表"
test_endpoint "/api/admin/certificate/generate" "POST" "401" "证书生成"
test_endpoint "/api/admin/certificate/renew" "POST" "401" "证书续期"
test_endpoint "/api/admin/certificate/auto-renew" "POST" "401" "自动续期"
test_endpoint "/api/admin/certificate/delete" "DELETE" "401" "证书删除"
test_endpoint "/api/admin/certificate/check" "POST" "401" "证书检查"

# Argo 隧道管理 API
echo -e "\n${BLUE}--- Argo 隧道管理 ---${NC}"
test_endpoint "/api/admin/argo/list" "GET" "401" "隧道列表"
test_endpoint "/api/admin/argo/create" "POST" "401" "创建隧道"
test_endpoint "/api/admin/argo/action" "POST" "401" "隧道操作"
test_endpoint "/api/admin/argo/status" "GET" "401" "隧道状态"
test_endpoint "/api/admin/argo/install" "POST" "401" "安装Argo"
test_endpoint "/api/admin/argo/quick" "GET" "401" "快速隧道"
test_endpoint "/api/admin/argo/metrics" "GET" "401" "隧道指标"

# WARP 管理 API
echo -e "\n${BLUE}--- WARP 管理 ---${NC}"
test_endpoint "/api/admin/warp/status" "GET" "401" "WARP状态"
test_endpoint "/api/admin/warp/enable" "POST" "401" "启用WARP"
test_endpoint "/api/admin/warp/disable" "POST" "401" "禁用WARP"
test_endpoint "/api/admin/warp/configs" "GET" "401" "WARP配置列表"
test_endpoint "/api/admin/warp/generate-config" "POST" "401" "生成WARP配置"
test_endpoint "/api/admin/warp/install" "POST" "401" "安装WARP"

# 系统优化 API
echo -e "\n${BLUE}--- 系统优化 ---${NC}"
test_endpoint "/api/admin/system/optimize" "POST" "401" "系统优化"
test_endpoint "/api/admin/system/bbr-status" "GET" "401" "BBR状态"
test_endpoint "/api/admin/system/network-performance" "GET" "401" "网络性能"
test_endpoint "/api/admin/system/resource-usage" "GET" "401" "资源使用"
test_endpoint "/api/admin/system/report" "GET" "401" "系统报告"

# 订阅管理 API
echo -e "\n${BLUE}--- 订阅管理 ---${NC}"
test_endpoint "/api/admin/subscription/generate" "POST" "401" "生成订阅"
test_endpoint "/api/admin/subscription/list" "GET" "401" "订阅列表"
test_endpoint "/api/admin/subscription/export" "GET" "401" "导出订阅"
test_endpoint "/api/admin/subscription/url" "GET" "401" "订阅URL"
test_endpoint "/api/admin/subscription/qrcode" "GET" "401" "订阅二维码"

# 分享管理 API
echo -e "\n${BLUE}--- 分享管理 ---${NC}"
test_endpoint "/api/admin/share/create" "POST" "401" "创建分享"
test_endpoint "/api/admin/share/list" "GET" "401" "分享列表"
test_endpoint "/api/admin/share/detail" "GET" "401" "分享详情"
test_endpoint "/api/admin/gitlab/sync" "POST" "401" "GitLab同步"
test_endpoint "/api/admin/github/sync" "POST" "401" "GitHub同步"
test_endpoint "/api/admin/pastebin/share" "POST" "401" "Pastebin分享"

# 公共 API
echo -e "\n${BLUE}--- 公共 API ---${NC}"
test_endpoint "/" "GET" "200" "根路径"
test_endpoint "/health" "GET" "200" "健康检查"

# 输出测试结果
echo -e "\n${BLUE}=== 测试结果汇总 ===${NC}"
echo -e "总测试数: ${TOTAL_TESTS}"
echo -e "${GREEN}通过: ${PASSED_TESTS}${NC}"
echo -e "${RED}失败: ${FAILED_TESTS}${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "\n${GREEN}✓ 所有API端点验证通过！${NC}"
    exit 0
else
    echo -e "\n${RED}✗ 有 ${FAILED_TESTS} 个测试失败${NC}"
    exit 1
fi
