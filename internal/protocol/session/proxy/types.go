package proxy

// SOCKS5TunnelRequest SOCKS5 隧道请求（从 ClientA 发送）
type SOCKS5TunnelRequest struct {
	TunnelID       string `json:"tunnel_id"`
	MappingID      string `json:"mapping_id"`
	TargetClientID int64  `json:"target_client_id"`
	TargetHost     string `json:"target_host"` // 动态目标地址
	TargetPort     int    `json:"target_port"` // 动态目标端口
	Protocol       string `json:"protocol"`
}
