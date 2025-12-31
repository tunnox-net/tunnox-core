package command

import (
	"testing"

	"tunnox-core/internal/packet"
)

func TestRequest_Fields(t *testing.T) {
	req := Request{
		CommandType: packet.Connect,
		RequestBody: map[string]string{"key": "value"},
		EnableTrace: true,
	}

	if req.CommandType != packet.Connect {
		t.Errorf("CommandType = %v, want %v", req.CommandType, packet.Connect)
	}

	if req.EnableTrace != true {
		t.Error("EnableTrace should be true")
	}

	body, ok := req.RequestBody.(map[string]string)
	if !ok {
		t.Error("RequestBody should be map[string]string")
	}
	if body["key"] != "value" {
		t.Errorf("RequestBody[key] = %s, want value", body["key"])
	}
}

func TestResponseData_Fields(t *testing.T) {
	tests := []struct {
		name    string
		resp    ResponseData
		success bool
		hasData bool
		hasErr  bool
	}{
		{
			name: "success response with data",
			resp: ResponseData{
				Success: true,
				Data:    "some data",
				Error:   "",
			},
			success: true,
			hasData: true,
			hasErr:  false,
		},
		{
			name: "error response",
			resp: ResponseData{
				Success: false,
				Data:    "",
				Error:   "error message",
			},
			success: false,
			hasData: false,
			hasErr:  true,
		},
		{
			name: "empty response",
			resp: ResponseData{
				Success: false,
				Data:    "",
				Error:   "",
			},
			success: false,
			hasData: false,
			hasErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.resp.Success != tt.success {
				t.Errorf("Success = %v, want %v", tt.resp.Success, tt.success)
			}
			if (tt.resp.Data != "") != tt.hasData {
				t.Errorf("HasData = %v, want %v", tt.resp.Data != "", tt.hasData)
			}
			if (tt.resp.Error != "") != tt.hasErr {
				t.Errorf("HasError = %v, want %v", tt.resp.Error != "", tt.hasErr)
			}
		})
	}
}

func TestGenerateConnectionCodeRequest(t *testing.T) {
	req := GenerateConnectionCodeRequest{
		TargetAddress: "tcp://192.168.1.10:8080",
		ActivationTTL: 300,
		MappingTTL:    3600,
		Description:   "Test connection",
	}

	if req.TargetAddress != "tcp://192.168.1.10:8080" {
		t.Errorf("TargetAddress = %s, want tcp://192.168.1.10:8080", req.TargetAddress)
	}
	if req.ActivationTTL != 300 {
		t.Errorf("ActivationTTL = %d, want 300", req.ActivationTTL)
	}
	if req.MappingTTL != 3600 {
		t.Errorf("MappingTTL = %d, want 3600", req.MappingTTL)
	}
	if req.Description != "Test connection" {
		t.Errorf("Description = %s, want 'Test connection'", req.Description)
	}
}

func TestGenerateConnectionCodeResponse(t *testing.T) {
	resp := GenerateConnectionCodeResponse{
		Code:          "ABC123",
		TargetAddress: "tcp://192.168.1.10:8080",
		ExpiresAt:     "2024-01-01T00:00:00Z",
		Description:   "Test",
	}

	if resp.Code != "ABC123" {
		t.Errorf("Code = %s, want ABC123", resp.Code)
	}
	if resp.TargetAddress != "tcp://192.168.1.10:8080" {
		t.Errorf("TargetAddress = %s, want tcp://192.168.1.10:8080", resp.TargetAddress)
	}
	if resp.ExpiresAt != "2024-01-01T00:00:00Z" {
		t.Errorf("ExpiresAt = %s, want 2024-01-01T00:00:00Z", resp.ExpiresAt)
	}
}

func TestListConnectionCodesResponse(t *testing.T) {
	resp := ListConnectionCodesResponse{
		Codes: []ConnectionCodeInfo{
			{
				Code:          "ABC123",
				TargetAddress: "tcp://192.168.1.10:8080",
				Status:        "active",
				CreatedAt:     "2024-01-01T00:00:00Z",
				ExpiresAt:     "2024-01-02T00:00:00Z",
				Activated:     false,
			},
		},
		Total: 1,
	}

	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if len(resp.Codes) != 1 {
		t.Errorf("Codes length = %d, want 1", len(resp.Codes))
	}
	if resp.Codes[0].Code != "ABC123" {
		t.Errorf("Codes[0].Code = %s, want ABC123", resp.Codes[0].Code)
	}
}

func TestConnectionCodeInfo(t *testing.T) {
	activatedBy := int64(123)
	info := ConnectionCodeInfo{
		Code:          "ABC123",
		TargetAddress: "tcp://192.168.1.10:8080",
		Status:        "active",
		CreatedAt:     "2024-01-01T00:00:00Z",
		ExpiresAt:     "2024-01-02T00:00:00Z",
		Activated:     true,
		ActivatedBy:   &activatedBy,
		Description:   "Test",
	}

	if info.Code != "ABC123" {
		t.Errorf("Code = %s, want ABC123", info.Code)
	}
	if !info.Activated {
		t.Error("Activated should be true")
	}
	if info.ActivatedBy == nil || *info.ActivatedBy != 123 {
		t.Error("ActivatedBy should be 123")
	}
}

func TestActivateConnectionCodeRequest(t *testing.T) {
	req := ActivateConnectionCodeRequest{
		Code:          "ABC123",
		ListenAddress: "127.0.0.1:8888",
	}

	if req.Code != "ABC123" {
		t.Errorf("Code = %s, want ABC123", req.Code)
	}
	if req.ListenAddress != "127.0.0.1:8888" {
		t.Errorf("ListenAddress = %s, want 127.0.0.1:8888", req.ListenAddress)
	}
}

func TestActivateConnectionCodeResponse(t *testing.T) {
	resp := ActivateConnectionCodeResponse{
		MappingID:      "mapping-123",
		TargetAddress:  "tcp://192.168.1.10:8080",
		ListenAddress:  "127.0.0.1:8888",
		ExpiresAt:      "2024-01-02T00:00:00Z",
		TargetClientID: 456,
		SecretKey:      "secret123",
	}

	if resp.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", resp.MappingID)
	}
	if resp.TargetClientID != 456 {
		t.Errorf("TargetClientID = %d, want 456", resp.TargetClientID)
	}
	if resp.SecretKey != "secret123" {
		t.Errorf("SecretKey = %s, want secret123", resp.SecretKey)
	}
}

func TestListMappingsRequest(t *testing.T) {
	req := ListMappingsRequest{
		Direction: "outbound",
		Type:      "tcp",
		Status:    "active",
	}

	if req.Direction != "outbound" {
		t.Errorf("Direction = %s, want outbound", req.Direction)
	}
	if req.Type != "tcp" {
		t.Errorf("Type = %s, want tcp", req.Type)
	}
	if req.Status != "active" {
		t.Errorf("Status = %s, want active", req.Status)
	}
}

func TestListMappingsResponse(t *testing.T) {
	resp := ListMappingsResponse{
		Mappings: []MappingInfo{
			{
				MappingID:     "mapping-123",
				Type:          "outbound",
				TargetAddress: "tcp://192.168.1.10:8080",
				ListenAddress: "127.0.0.1:8888",
				Status:        "active",
				ExpiresAt:     "2024-01-02T00:00:00Z",
				CreatedAt:     "2024-01-01T00:00:00Z",
				BytesSent:     1000,
				BytesReceived: 2000,
			},
		},
		Total: 1,
	}

	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if len(resp.Mappings) != 1 {
		t.Errorf("Mappings length = %d, want 1", len(resp.Mappings))
	}
	if resp.Mappings[0].BytesSent != 1000 {
		t.Errorf("BytesSent = %d, want 1000", resp.Mappings[0].BytesSent)
	}
}

func TestMappingInfo(t *testing.T) {
	info := MappingInfo{
		MappingID:     "mapping-123",
		Type:          "outbound",
		TargetAddress: "tcp://192.168.1.10:8080",
		ListenAddress: "127.0.0.1:8888",
		Status:        "active",
		ExpiresAt:     "2024-01-02T00:00:00Z",
		CreatedAt:     "2024-01-01T00:00:00Z",
		BytesSent:     1000,
		BytesReceived: 2000,
	}

	if info.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", info.MappingID)
	}
	if info.Type != "outbound" {
		t.Errorf("Type = %s, want outbound", info.Type)
	}
	if info.BytesSent != 1000 {
		t.Errorf("BytesSent = %d, want 1000", info.BytesSent)
	}
	if info.BytesReceived != 2000 {
		t.Errorf("BytesReceived = %d, want 2000", info.BytesReceived)
	}
}

func TestGetMappingRequest(t *testing.T) {
	req := GetMappingRequest{
		MappingID: "mapping-123",
	}

	if req.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", req.MappingID)
	}
}

func TestGetMappingResponse(t *testing.T) {
	resp := GetMappingResponse{
		Mapping: MappingInfo{
			MappingID: "mapping-123",
			Status:    "active",
		},
	}

	if resp.Mapping.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", resp.Mapping.MappingID)
	}
}

func TestDeleteMappingRequest(t *testing.T) {
	req := DeleteMappingRequest{
		MappingID: "mapping-123",
	}

	if req.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", req.MappingID)
	}
}

func TestHTTPDomainBaseDomainInfo(t *testing.T) {
	info := HTTPDomainBaseDomainInfo{
		Domain:      "example.com",
		Description: "Example domain",
		IsDefault:   true,
	}

	if info.Domain != "example.com" {
		t.Errorf("Domain = %s, want example.com", info.Domain)
	}
	if !info.IsDefault {
		t.Error("IsDefault should be true")
	}
}

func TestGetBaseDomainsResponse(t *testing.T) {
	resp := GetBaseDomainsResponse{
		Success: true,
		BaseDomains: []HTTPDomainBaseDomainInfo{
			{Domain: "example.com", IsDefault: true},
		},
		Error: "",
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if len(resp.BaseDomains) != 1 {
		t.Errorf("BaseDomains length = %d, want 1", len(resp.BaseDomains))
	}
}

func TestCheckSubdomainRequest(t *testing.T) {
	req := CheckSubdomainRequest{
		Subdomain:  "myapp",
		BaseDomain: "example.com",
	}

	if req.Subdomain != "myapp" {
		t.Errorf("Subdomain = %s, want myapp", req.Subdomain)
	}
	if req.BaseDomain != "example.com" {
		t.Errorf("BaseDomain = %s, want example.com", req.BaseDomain)
	}
}

func TestCheckSubdomainResponse(t *testing.T) {
	resp := CheckSubdomainResponse{
		Success:    true,
		Available:  true,
		FullDomain: "myapp.example.com",
		Error:      "",
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if !resp.Available {
		t.Error("Available should be true")
	}
	if resp.FullDomain != "myapp.example.com" {
		t.Errorf("FullDomain = %s, want myapp.example.com", resp.FullDomain)
	}
}

func TestGenSubdomainRequest(t *testing.T) {
	req := GenSubdomainRequest{
		BaseDomain: "example.com",
	}

	if req.BaseDomain != "example.com" {
		t.Errorf("BaseDomain = %s, want example.com", req.BaseDomain)
	}
}

func TestGenSubdomainResponse(t *testing.T) {
	resp := GenSubdomainResponse{
		Success:    true,
		Subdomain:  "abc123",
		FullDomain: "abc123.example.com",
		Error:      "",
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if resp.Subdomain != "abc123" {
		t.Errorf("Subdomain = %s, want abc123", resp.Subdomain)
	}
}

func TestCreateHTTPDomainRequest(t *testing.T) {
	req := CreateHTTPDomainRequest{
		TargetURL:   "http://localhost:8080",
		Subdomain:   "myapp",
		BaseDomain:  "example.com",
		MappingTTL:  3600,
		Description: "My app",
	}

	if req.TargetURL != "http://localhost:8080" {
		t.Errorf("TargetURL = %s, want http://localhost:8080", req.TargetURL)
	}
	if req.MappingTTL != 3600 {
		t.Errorf("MappingTTL = %d, want 3600", req.MappingTTL)
	}
}

func TestCreateHTTPDomainResponse(t *testing.T) {
	resp := CreateHTTPDomainResponse{
		Success:    true,
		MappingID:  "mapping-123",
		FullDomain: "myapp.example.com",
		TargetURL:  "http://localhost:8080",
		ExpiresAt:  "2024-01-02T00:00:00Z",
		Error:      "",
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if resp.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", resp.MappingID)
	}
}

func TestHTTPDomainMappingInfo(t *testing.T) {
	info := HTTPDomainMappingInfo{
		MappingID:  "mapping-123",
		FullDomain: "myapp.example.com",
		TargetURL:  "http://localhost:8080",
		Status:     "active",
		CreatedAt:  "2024-01-01T00:00:00Z",
		ExpiresAt:  "2024-01-02T00:00:00Z",
	}

	if info.MappingID != "mapping-123" {
		t.Errorf("MappingID = %s, want mapping-123", info.MappingID)
	}
	if info.FullDomain != "myapp.example.com" {
		t.Errorf("FullDomain = %s, want myapp.example.com", info.FullDomain)
	}
}

func TestListHTTPDomainsResponse(t *testing.T) {
	resp := ListHTTPDomainsResponse{
		Success: true,
		Mappings: []HTTPDomainMappingInfo{
			{MappingID: "mapping-123", FullDomain: "myapp.example.com"},
		},
		Total: 1,
		Error: "",
	}

	if !resp.Success {
		t.Error("Success should be true")
	}
	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if len(resp.Mappings) != 1 {
		t.Errorf("Mappings length = %d, want 1", len(resp.Mappings))
	}
}
