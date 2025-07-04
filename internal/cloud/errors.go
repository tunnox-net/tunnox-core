package cloud

import "errors"

// 云控相关错误
var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrInvalidToken         = errors.New("invalid token")
	ErrClientNotFound       = errors.New("client not found")
	ErrNodeNotFound         = errors.New("node not found")
	ErrPortMappingNotFound  = errors.New("port mapping not found")
	ErrNetworkUnavailable   = errors.New("network unavailable")
	ErrInvalidConfig        = errors.New("invalid configuration")
)
