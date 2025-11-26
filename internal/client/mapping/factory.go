package mapping

import (
	"context"
	"fmt"

	"tunnox-core/internal/config"
)

// CreateAdapter 工厂方法创建协议适配器
func CreateAdapter(protocol string, config config.MappingConfig) (MappingAdapter, error) {
	switch protocol {
	case "tcp":
		return NewTCPMappingAdapter(), nil

	case "udp":
		return NewUDPMappingAdapter(context.Background()), nil

	case "socks5":
		// SOCKS5凭据从配置读取（如果需要）
		credentials := make(map[string]string)
		// 后续可在此从 config 中读取 SOCKS5 认证信息
		return NewSOCKS5MappingAdapter(credentials), nil

	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}
