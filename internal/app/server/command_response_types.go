package server

// ConnectionCodeResponse 连接码响应
type ConnectionCodeResponse struct {
	Code                string `json:"code"`
	TargetAddress       string `json:"target_address"`
	ExpiresAt           string `json:"expires_at"`            // 连接码激活截止时间（必须在此之前激活）
	MappingExpiresAt    string `json:"mapping_expires_at"`    // 激活后映射的过期时间
	ActivationTTLMinutes int    `json:"activation_ttl_minutes"` // 激活有效期（分钟）
	MappingTTLDays      int    `json:"mapping_ttl_days"`       // 映射有效期（天）
	Description         string `json:"description,omitempty"`
}

// ConnectionCodeInfo 连接码信息
type ConnectionCodeInfo struct {
	Code          string `json:"code"`
	TargetAddress string `json:"target_address"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Activated     bool   `json:"activated"`
	ActivatedBy   string `json:"activated_by,omitempty"`
	Description   string `json:"description,omitempty"`
}

// ConnectionCodeListResponse 连接码列表响应
type ConnectionCodeListResponse struct {
	Codes []ConnectionCodeInfo `json:"codes"`
	Total int                  `json:"total"`
}

// MappingActivateResponse 映射激活响应
type MappingActivateResponse struct {
	MappingID     string `json:"mapping_id"`
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

// MappingItem 映射项
type MappingItem struct {
	MappingID     string `json:"mapping_id"`
	Type          string `json:"type"`
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	Status        string `json:"status"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	BytesSent     int64  `json:"bytes_sent,omitempty"`
	BytesReceived int64  `json:"bytes_received,omitempty"`
	Description   string `json:"description,omitempty"`
}

// MappingListResponse 映射列表响应
type MappingListResponse struct {
	Mappings []MappingItem `json:"mappings"`
	Total    int           `json:"total"`
}

// MappingDetailResponse 映射详情响应
type MappingDetailResponse struct {
	Mapping MappingItem `json:"mapping"`
}

