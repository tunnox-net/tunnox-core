package infrastructure

import (
	"net"
	"strconv"
	"time"
)

// NetworkManager 网络管理器
type NetworkManager interface {
	// 获取可用端口
	GetAvailablePort(startPort, endPort int) (int, error)

	// 检查端口是否可用
	IsPortAvailable(port int) bool

	// 获取本机IP地址
	GetLocalIP() (string, error)

	// 检查网络连接
	CheckConnectivity(host string, port int, timeout time.Duration) error

	// 获取网络接口信息
	GetNetworkInterfaces() ([]net.Interface, error)
}

// NetworkManagerImpl 网络管理器实现
type NetworkManagerImpl struct{}

// NewNetworkManager 创建新的网络管理器
func NewNetworkManager() *NetworkManagerImpl {
	return &NetworkManagerImpl{}
}

// GetAvailablePort 获取可用端口
func (nm *NetworkManagerImpl) GetAvailablePort(startPort, endPort int) (int, error) {
	for port := startPort; port <= endPort; port++ {
		if nm.IsPortAvailable(port) {
			return port, nil
		}
	}
	return 0, ErrNoAvailablePort
}

// IsPortAvailable 检查端口是否可用
func (nm *NetworkManagerImpl) IsPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// GetLocalIP 获取本机IP地址
func (nm *NetworkManagerImpl) GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", ErrNoLocalIP
}

// CheckConnectivity 检查网络连接
func (nm *NetworkManagerImpl) CheckConnectivity(host string, port int, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", host+":"+strconv.Itoa(port), timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// GetNetworkInterfaces 获取网络接口信息
func (nm *NetworkManagerImpl) GetNetworkInterfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// 错误定义
var (
	ErrNoAvailablePort = &NetworkError{Message: "no available port found"}
	ErrNoLocalIP       = &NetworkError{Message: "no local IP found"}
)

// NetworkError 网络错误
type NetworkError struct {
	Message string
}

func (e *NetworkError) Error() string {
	return e.Message
}
