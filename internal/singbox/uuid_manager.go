package singbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"

	"miaomiaowu/internal/logger"
)

// UUIDManager UUID管理器
type UUIDManager struct {
	uuidMap map[string]*UUIDInfo // UUID -> UUID信息
	mutex   sync.RWMutex
}

// UUIDInfo UUID信息
type UUIDInfo struct {
	UUID      string `json:"uuid"`
	Protocol string `json:"protocol"`
	Port      int    `json:"port"`
	CreatedAt string `json:"created_at"`
	LastUsed  string `json:"last_used,omitempty"`
	Enabled   bool   `json:"enabled"`
	Notes     string `json:"notes,omitempty"`
}

// IPPriority IP优先级配置
type IPPriority struct {
	IP         string `json:"ip"`
	Priority   int    `json:"priority"`   // 优先级 1-5，5最高
	Type       string `json:"type"`       // ipv4, ipv6, prefer_ipv4, prefer_ipv6
	Enabled    bool   `json:"enabled"`
	GeoIP      string `json:"geo_ip,omitempty"` // 地理IP信息
	Desc       string `json:"desc,omitempty"`  // 描述
}

// NewUUIDManager 创建UUID管理器
func NewUUIDManager() *UUIDManager {
	return &UUIDManager{
		uuidMap: make(map[string]*UUIDInfo),
	}
}

// GenerateUUID 生成新UUID
func (um *UUIDManager) GenerateUUID() (string, error) {
	uuid, err := generateUUID()
	if err != nil {
		return "", fmt.Errorf("generate UUID failed: %w", err)
	}

	um.mutex.Lock()
	defer um.mutex.Unlock()

	// 检查UUID是否已存在
	if _, exists := um.uuidMap[uuid]; exists {
		// 如果UUID重复，重新生成
		return um.GenerateUUID()
	}

	// 创建UUID信息
	info := &UUIDInfo{
		UUID:      uuid,
		CreatedAt: currentTime(),
		Enabled:   true,
	}

	um.uuidMap[uuid] = info
	logger.Info("[UUID管理] 生成新UUID", "uuid", uuid)

	return uuid, nil
}

// AddUUID 添加UUID
func (um *UUIDManager) AddUUID(uuid, protocol string, port int) error {
	if !isValidUUID(uuid) {
		return fmt.Errorf("invalid UUID format")
	}

	um.mutex.Lock()
	defer um.mutex.Unlock()

	// 检查是否已存在
	if _, exists := um.uuidMap[uuid]; exists {
		return fmt.Errorf("UUID already exists")
	}

	info := &UUIDInfo{
		UUID:      uuid,
		Protocol: protocol,
		Port:      port,
		CreatedAt: currentTime(),
		Enabled:   true,
	}

	um.uuidMap[uuid] = info
	logger.Info("[UUID管理] 添加UUID", "uuid", uuid, "protocol", protocol)

	return nil
}

// RemoveUUID 移除UUID
func (um *UUIDManager) RemoveUUID(uuid string) error {
	um.mutex.Lock()
	defer um.mutex.Unlock()

	if _, exists := um.uuidMap[uuid]; !exists {
		return fmt.Errorf("UUID not found")
	}

	delete(um.uuidMap, uuid)
	logger.Info("[UUID管理] 移除UUID", "uuid", uuid)

	return nil
}

// UpdateUUID 更新UUID信息
func (um *UUIDManager) UpdateUUID(uuid string, updates map[string]interface{}) error {
	um.mutex.Lock()
	defer um.mutex.Unlock()

	info, exists := um.uuidMap[uuid]
	if !exists {
		return fmt.Errorf("UUID not found")
	}

	// 应用更新
	if protocol, ok := updates["protocol"].(string); ok {
		info.Protocol = protocol
	}
	if port, ok := updates["port"].(int); ok {
		info.Port = port
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		info.Enabled = enabled
	}
	if notes, ok := updates["notes"].(string); ok {
		info.Notes = notes
	}

	info.LastUsed = currentTime()
	logger.Info("[UUID管理] 更新UUID", "uuid", uuid)

	return nil
}

// GetUUIDInfo 获取UUID信息
func (um *UUIDManager) GetUUIDInfo(uuid string) (*UUIDInfo, error) {
	um.mutex.RLock()
	defer um.mutex.RUnlock()

	info, exists := um.uuidMap[uuid]
	if !exists {
		return nil, fmt.Errorf("UUID not found")
	}

	return info, nil
}

// ListUUIDs 列出所有UUID
func (um *UUIDManager) ListUUIDs() []*UUIDInfo {
	um.mutex.RLock()
	defer um.mutex.RUnlock()

	uuids := make([]*UUIDInfo, 0, len(um.uuidMap))
	for _, info := range um.uuidMap {
		uuids = append(uuids, info)
	}

	return uuids
}

// GetUUIDsByProtocol 根据协议获取UUID
func (um *UUIDManager) GetUUIDsByProtocol(protocol string) []*UUIDInfo {
	um.mutex.RLock()
	defer um.mutex.RUnlock()

	uuids := make([]*UUIDInfo, 0)
	for _, info := range um.uuidMap {
		if info.Protocol == protocol && info.Enabled {
			uuids = append(uuids, info)
		}
	}

	return uuids
}

// ValidateUUID 验证UUID
func (um *UUIDManager) ValidateUUID(uuid string) bool {
	return isValidUUID(uuid)
}

// GetAvailableUUID 获取可用的UUID
func (um *UUIDManager) GetAvailableUUID(protocol string) (string, error) {
	uuids := um.GetUUIDsByProtocol(protocol)
	if len(uuids) > 0 {
		// 返回第一个可用的UUID
		return uuids[0].UUID, nil
	}

	// 如果没有可用的UUID，生成新的
	return um.GenerateUUID()
}

// CloneUUID 克隆UUID（用于多端口配置）
func (um *UUIDManager) CloneUUID(originalUUID string) (string, error) {
	um.mutex.RLock()
	originalInfo, exists := um.uuidMap[originalUUID]
	um.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("original UUID not found")
	}

	// 生成新UUID
	newUUID, err := um.GenerateUUID()
	if err != nil {
		return "", err
	}

	// 复制原UUID的信息
	um.mutex.Lock()
	defer um.mutex.Unlock()

	info := um.uuidMap[newUUID]
	info.Protocol = originalInfo.Protocol
	info.Notes = "Cloned from " + originalUUID

	return newUUID, nil
}

// IPManager IP管理器
type IPManager struct {
	ipPriorities map[string]*IPPriority
	mutex        sync.RWMutex
}

// NewIPManager 创建IP管理器
func NewIPManager() *IPManager {
	return &IPManager{
		ipPriorities: make(map[string]*IPPriority),
	}
}

// AddIPPriority 添加IP优先级
func (im *IPManager) AddIPPriority(priority *IPPriority) error {
	if priority.IP == "" {
		return fmt.Errorf("IP address is required")
	}

	if priority.Priority < 1 || priority.Priority > 5 {
		return fmt.Errorf("priority must be between 1 and 5")
	}

	im.mutex.Lock()
	defer im.mutex.Unlock()

	im.ipPriorities[priority.IP] = priority
	logger.Info("[IP管理] 添加IP优先级", "ip", priority.IP, "priority", priority.Priority)

	return nil
}

// RemoveIPPriority 移除IP优先级
func (im *IPManager) RemoveIPPriority(ip string) error {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	if _, exists := im.ipPriorities[ip]; !exists {
		return fmt.Errorf("IP priority not found")
	}

	delete(im.ipPriorities, ip)
	logger.Info("[IP管理] 移除IP优先级", "ip", ip)

	return nil
}

// GetIPPriorities 获取所有IP优先级
func (im *IPManager) GetIPPriorities() []*IPPriority {
	im.mutex.RLock()
	defer im.mutex.RUnlock()

	priorities := make([]*IPPriority, 0, len(im.ipPriorities))
	for _, p := range im.ipPriorities {
		priorities = append(priorities, p)
	}

	return priorities
}

// GetPreferredIP 获取首选IP
func (im *IPManager) GetPreferredIP(ipType string) string {
	im.mutex.RLock()
	defer im.mutex.RUnlock()

	// 按优先级排序查找
	maxPriority := 0
	var preferredIP string

	for _, p := range im.ipPriorities {
		if !p.Enabled {
			continue
		}

		// 检查IP类型匹配
		typeMatch := false
		if ipType == "ipv4" && isIPv4(p.IP) {
			typeMatch = true
		} else if ipType == "ipv6" && isIPv6(p.IP) {
			typeMatch = true
		} else if ipType == "" {
			typeMatch = true
		}

		if typeMatch && p.Priority > maxPriority {
			maxPriority = p.Priority
			preferredIP = p.IP
		}
	}

	return preferredIP
}

// UpdateIPPriority 更新IP优先级
func (im *IPManager) UpdateIPPriority(ip string, updates map[string]interface{}) error {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	priority, exists := im.ipPriorities[ip]
	if !exists {
		return fmt.Errorf("IP priority not found")
	}

	// 应用更新
	if prio, ok := updates["priority"].(int); ok {
		if prio < 1 || prio > 5 {
			return fmt.Errorf("priority must be between 1 and 5")
		}
		priority.Priority = prio
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		priority.Enabled = enabled
	}
	if desc, ok := updates["desc"].(string); ok {
		priority.Desc = desc
	}

	im.ipPriorities[ip] = priority
	logger.Info("[IP管理] 更新IP优先级", "ip", ip)

	return nil
}

// 工具函数

func isValidUUID(uuid string) bool {
	// 简单的UUID格式验证
	if len(uuid) != 36 {
		return false
	}

	parts := []int{8, 13, 18, 23}
	for i, pos := range parts {
		if i > 0 {
			if uuid[pos-1] != '-' {
				return false
			}
		}
		if i < len(parts)-1 && uuid[pos] != '-' {
			return false
		}
	}

	// 验证是否为有效的十六进制
	hexParts := []string{uuid[0:8], uuid[9:13], uuid[14:18], uuid[19:23], uuid[24:36]}
	for _, part := range hexParts {
		if _, err := hex.DecodeString(part); err != nil {
			return false
		}
	}

	return true
}

func isIPv4(ip string) bool {
	// 简单的IPv4验证
	parts := []string{}
	for _, part := range []string{ip} {
		parts = append(parts, part)
	}

	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}

	return true
}

func isIPv6(ip string) bool {
	// 简单的IPv6验证
	return !isIPv4(ip) && len(ip) > 0
}

func currentTime() string {
	// 返回当前时间字符串
	// 实际实现应该使用 time 包
	return ""
}

// GenerateBatchUUIDs 批量生成UUID
func GenerateBatchUUIDs(count int) ([]string, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive")
	}

	uuids := make([]string, 0, count)
	seen := make(map[string]bool)

	for len(uuids) < count {
		uuid, err := generateUUID()
		if err != nil {
			return nil, err
		}

		// 确保UUID唯一
		if !seen[uuid] {
			seen[uuid] = true
			uuids = append(uuids, uuid)
		}
	}

	logger.Info("[UUID管理] 批量生成UUID", "count", count)
	return uuids, nil
}

// GenerateShortID 生成ShortID（用于Reality）
func GenerateShortID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateKey 生成密钥
func GenerateKey(length int) (string, error) {
	if length <= 0 {
		length = 32
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}

	return string(b), nil
}