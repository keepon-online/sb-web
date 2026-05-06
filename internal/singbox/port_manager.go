package singbox

import (
	"fmt"
	"net"
	"sync"

	"miaomiaowu/internal/logger"
)

// PortManager 端口管理器
type PortManager struct {
	usedPorts map[int]bool
	mutex     sync.RWMutex
}

// NewPortManager 创建端口管理器
func NewPortManager() *PortManager {
	pm := &PortManager{
		usedPorts: make(map[int]bool),
	}

	// 初始化时扫描已使用的端口
	pm.scanUsedPorts()

	return pm
}

// scanUsedPorts 扫描已使用的端口
func (pm *PortManager) scanUsedPorts() {
	// 常用系统端口
	systemPorts := []int{22, 80, 443, 8080, 3306, 5432, 6379, 27017}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for _, port := range systemPorts {
		pm.usedPorts[port] = true
	}

	// 检查网络端口
	if err := pm.checkNetworkPorts(); err != nil {
		logger.Warn("[端口管理] 扫描网络端口失败", "error", err)
	}
}

// checkNetworkPorts 检查网络端口使用情况
func (pm *PortManager) checkNetworkPorts() error {
	// 常见服务端口
	commonPorts := []int{
		21, 22, 23, 25, 53, 80, 110, 143, 443, 465, 587, 993, 995,
		1080, 3306, 3389, 5432, 5900, 6379, 8000, 8080, 8443, 8888,
		9000, 9001, 9090, 9200, 9300, 10000,
	}

	for _, port := range commonPorts {
		if pm.isPortInUse(port) {
			pm.usedPorts[port] = true
		}
	}

	return nil
}

// isPortInUse 检查端口是否在使用中
func (pm *PortManager) isPortInUse(port int) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return true // 端口被占用
	}
	listener.Close()
	return false
}

// AllocatePort 分配端口
func (pm *PortManager) AllocatePort() (int, error) {
	return pm.AllocatePortInRange(10000, 65535)
}

// AllocatePortInRange 在指定范围内分配端口
func (pm *PortManager) AllocatePortInRange(minPort, maxPort int) (int, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// 确保范围有效
	if minPort < 10000 {
		minPort = 10000
	}
	if maxPort > 65535 {
		maxPort = 65535
	}
	if minPort > maxPort {
		return 0, fmt.Errorf("invalid port range: min > max")
	}

	// 尝试随机端口
	for attempt := 0; attempt < 100; attempt++ {
		port, err := GenerateRandomPort(minPort, maxPort)
		if err != nil {
			continue
		}

		if !pm.usedPorts[port] {
			// 再次检查端口是否真的可用
			if pm.isPortInUse(port) {
				pm.usedPorts[port] = true
				continue
			}

			pm.usedPorts[port] = true
			logger.Info("[端口管理] 分配端口", "port", port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available port in range %d-%d", minPort, maxPort)
}

// AllocateSpecificPort 分配指定端口
func (pm *PortManager) AllocateSpecificPort(port int) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port number: %d", port)
	}

	if pm.usedPorts[port] {
		return fmt.Errorf("port %d is already in use", port)
	}

	// 检查端口是否真的可用
	if pm.isPortInUse(port) {
		pm.usedPorts[port] = true
		return fmt.Errorf("port %d is already in use", port)
	}

	pm.usedPorts[port] = true
	logger.Info("[端口管理] 分配指定端口", "port", port)
	return nil
}

// ReleasePort 释放端口
func (pm *PortManager) ReleasePort(port int) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	delete(pm.usedPorts, port)
	logger.Info("[端口管理] 释放端口", "port", port)
}

// IsPortAvailable 检查端口是否可用
func (pm *PortManager) IsPortAvailable(port int) bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	if pm.usedPorts[port] {
		return false
	}

	// 实际检查端口
	return !pm.isPortInUse(port)
}

// GetUsedPorts 获取已使用的端口列表
func (pm *PortManager) GetUsedPorts() []int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	ports := make([]int, 0, len(pm.usedPorts))
	for port := range pm.usedPorts {
		ports = append(ports, port)
	}

	return ports
}

// CheckPortConflict 检查端口冲突
func (pm *PortManager) CheckPortConflict(ports []int) []int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	conflicts := []int{}
	for _, port := range ports {
		if pm.usedPorts[port] {
			conflicts = append(conflicts, port)
		}
	}

	return conflicts
}

// BatchAllocatePorts 批量分配端口
func (pm *PortManager) BatchAllocatePorts(count int, minPort, maxPort int) ([]int, error) {
	if count <= 0 {
		return []int{}, nil
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	ports := make([]int, 0, count)
	attempts := 0
	maxAttempts := count * 10 // 最多尝试次数

	for len(ports) < count && attempts < maxAttempts {
		attempts++

		port, err := GenerateRandomPort(minPort, maxPort)
		if err != nil {
			continue
		}

		if !pm.usedPorts[port] {
			if !pm.isPortInUse(port) {
				pm.usedPorts[port] = true
				ports = append(ports, port)
			} else {
				pm.usedPorts[port] = true
			}
		}
	}

	if len(ports) < count {
		// 释放已分配的端口
		for _, port := range ports {
			delete(pm.usedPorts, port)
		}
		return nil, fmt.Errorf("could only allocate %d of %d requested ports", len(ports), count)
	}

	logger.Info("[端口管理] 批量分配端口", "count", count, "ports", ports)
	return ports, nil
}

// ValidatePortRange 验证端口范围
func (pm *PortManager) ValidatePortRange(minPort, maxPort int) error {
	if minPort < 1 || maxPort > 65535 {
		return fmt.Errorf("port range must be between 1 and 65535")
	}

	if minPort > maxPort {
		return fmt.Errorf("min port cannot be greater than max port")
	}

	if minPort < 10000 {
		logger.Warn("[端口管理] 端口范围包含系统端口", "min_port", minPort)
	}

	return nil
}

// GetAvailablePortCount 获取可用端口数量
func (pm *PortManager) GetAvailablePortCount(minPort, maxPort int) int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	if minPort < 10000 {
		minPort = 10000
	}
	if maxPort > 65535 {
		maxPort = 65535
	}

	totalPorts := maxPort - minPort + 1
	usedCount := 0

	for port := range pm.usedPorts {
		if port >= minPort && port <= maxPort {
			usedCount++
		}
	}

	return totalPorts - usedCount
}