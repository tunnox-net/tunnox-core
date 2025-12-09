package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

// HandleHTTPPoll 处理 HTTP Poll Poll 请求（公开方法，供协议层调用）
// GET /tunnox/v1/poll?timeout=30
func (s *ManagementAPIServer) HandleHTTPPoll(w http.ResponseWriter, r *http.Request) {
	// 1. 获取并解码 X-Tunnel-Package（必须）
	packageHeader := r.Header.Get("X-Tunnel-Package")
	if packageHeader == "" {
		s.respondError(w, http.StatusBadRequest, "missing X-Tunnel-Package header")
		return
	}
	utils.Debugf("HTTP long polling: [HANDLE_POLL] received Poll request, X-Tunnel-Package len=%d", len(packageHeader))

	// 2. 解码控制包
	pkg, err := httppoll.DecodeTunnelPackage(packageHeader)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode tunnel package: %v", err))
		return
	}

	// 3. 获取 ConnectionID（必须）
	connID := pkg.ConnectionID
	if connID == "" {
		s.respondError(w, http.StatusBadRequest, "missing connection_id in tunnel package")
		return
	}

	// 4. 验证 ConnectionID 格式
	if !httppoll.ValidateConnectionID(connID) {
		s.respondError(w, http.StatusBadRequest, "invalid connection_id format")
		return
	}

	// 5. 获取或创建连接
	// 注意：Poll 请求可能先于 Push 请求到达（例如握手时，客户端先发送 Push，然后立即发送 Poll）
	// 因此，如果连接不存在，也应该创建连接
	if s.httppollRegistry == nil {
		s.httppollRegistry = httppoll.NewConnectionRegistry()
	}

	// 使用 GetOrCreate 确保原子性（避免并发创建）
	streamProcessor := s.httppollRegistry.GetOrCreate(connID, func() *httppoll.ServerStreamProcessor {
		utils.Debugf("HTTP long polling: [HANDLE_POLL] connection not found, creating new connection, connID=%s", connID)
		return s.createHTTPLongPollingConnection(connID, pkg, r.Context())
	})
	if streamProcessor == nil {
		utils.Errorf("HTTP long polling: [HANDLE_POLL] failed to create connection, connID=%s", connID)
		s.respondError(w, http.StatusServiceUnavailable, "Failed to create connection")
		return
	}

	// 6. 检查是否是 keepalive 类型的请求
	// keepalive 请求可以接收数据流，但不接收控制包
	if pkg.TunnelType == "keepalive" {
		utils.Debugf("HTTP long polling: [HANDLE_POLL] received keepalive Poll request, connID=%s, requestID=%s", connID, pkg.RequestID)

		// 解析超时参数
		timeout := httppollDefaultTimeout
		if t := r.URL.Query().Get("timeout"); t != "" {
			if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 && parsed <= httppollMaxTimeout {
				timeout = parsed
			}
		}

		// 创建带超时的 context
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
		defer cancel()

		// 调用 HandlePollRequest 处理 keepalive 请求（可以接收数据流）
		base64Data, _, err := streamProcessor.HandlePollRequest(ctx, pkg.RequestID, "keepalive")
		if err != nil {
			if err == context.DeadlineExceeded || err == context.Canceled {
				// 超时或取消，返回超时响应
				resp := HTTPPollResponse{
					Success:   true,
					Timeout:   true,
					Timestamp: time.Now().Unix(),
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
				return
			}
			utils.Errorf("HTTP long polling: [HANDLE_POLL] HandlePollRequest failed for keepalive: %v, connID=%s", err, connID)
			s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Poll request failed: %v", err))
			return
		}

		// 如果收到数据流，返回数据（分片格式）
		if base64Data != "" {
			utils.Debugf("HTTP long polling: [HANDLE_POLL] keepalive request received data, len=%d, connID=%s", len(base64Data), connID)
			// base64Data 现在是分片响应的JSON字符串，直接解析并返回
			var fragmentResp HTTPPollResponse
			if err := json.Unmarshal([]byte(base64Data), &fragmentResp); err != nil {
				utils.Errorf("HTTP long polling: [HANDLE_POLL] failed to unmarshal fragment response: %v, connID=%s", err, connID)
				s.respondError(w, http.StatusInternalServerError, "Failed to parse fragment response")
				return
			}
			fragmentResp.Success = true
			fragmentResp.Timestamp = time.Now().Unix()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(fragmentResp)
			return
		}

		// 如果没有数据，返回超时响应
		resp := HTTPPollResponse{
			Success:   true,
			Timeout:   true,
			Timestamp: time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// 7. 更新 clientID 和 mappingID（如果需要，与 Push 请求保持一致）
	if pkg.ClientID > 0 {
		streamProcessor.UpdateClientID(pkg.ClientID)
	}
	if pkg.MappingID != "" {
		streamProcessor.SetMappingID(pkg.MappingID)
	}

	// 8. 解析超时参数
	timeout := httppollDefaultTimeout
	if t := r.URL.Query().Get("timeout"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 && parsed <= httppollMaxTimeout {
			timeout = parsed
		}
	}

	// 9. 长轮询：等待数据
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	// 调试：确认使用的 ServerStreamProcessor 实例
	requestID := pkg.RequestID
	tunnelType := pkg.TunnelType
	if tunnelType == "" {
		tunnelType = "control" // 默认为 control
	}
	utils.Debugf("HTTP long polling: [HANDLE_POLL] calling HandlePollRequest, connID=%s, pointer=%p, requestID=%s, tunnelType=%s", connID, streamProcessor, requestID, tunnelType)
	base64Data, responsePkg, err := streamProcessor.HandlePollRequest(ctx, requestID, tunnelType)

	// 处理错误（使用辅助函数，保持主函数简洁）
	if err != nil {
		if s.handlePollError(w, err, streamProcessor, connID, requestID, tunnelType) {
			return
		}
	}

	// 成功情况：记录日志
	utils.Debugf("HTTP long polling: [HANDLE_POLL] HandlePollRequest returned successfully, hasControlPacket=%v, hasData=%v, connID=%s",
		responsePkg != nil, base64Data != "", connID)

	// 9. 检查是否有控制包响应（如 TunnelOpenAck）
	if responsePkg != nil {
		encodedPkg, err := httppoll.EncodeTunnelPackage(responsePkg)
		if err == nil {
			w.Header().Set("X-Tunnel-Package", encodedPkg)
			utils.Debugf("HTTP long polling: [HANDLE_POLL] returning control packet in X-Tunnel-Package header, type=%s, connID=%s, encodedLen=%d",
				responsePkg.Type, connID, len(encodedPkg))
		} else {
			utils.Errorf("HTTP long polling: [HANDLE_POLL] failed to encode tunnel package: %v, connID=%s", err, connID)
		}
	} else {
		utils.Debugf("HTTP long polling: [HANDLE_POLL] no control packet to return, connID=%s", connID)
	}

	// 10. 返回响应（分片格式）
	// base64Data 是 HandlePollRequest 返回的分片响应的JSON字符串，直接作为响应 body 返回
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if base64Data != "" {
		// 直接返回 JSON 字符串，不需要解析和重新序列化
		utils.Debugf("HTTP long polling: [HANDLE_POLL] writing HTTP response with data, status=200, hasControlPacket=%v, dataLen=%d, connID=%s",
			responsePkg != nil, len(base64Data), connID)
		if _, err := w.Write([]byte(base64Data)); err != nil {
			utils.Errorf("HTTP long polling: [HANDLE_POLL] failed to write response body: %v, connID=%s", err, connID)
		} else {
			utils.Debugf("HTTP long polling: [HANDLE_POLL] HTTP response written successfully, connID=%s", connID)
		}
	} else {
		// 没有数据，返回超时响应
		resp := HTTPPollResponse{
			Success:   true,
			Timeout:   true,
			Timestamp: time.Now().Unix(),
		}
		utils.Debugf("HTTP long polling: [HANDLE_POLL] writing timeout response, status=200, hasControlPacket=%v, connID=%s",
			responsePkg != nil, connID)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			utils.Errorf("HTTP long polling: [HANDLE_POLL] failed to write timeout response: %v, connID=%s", err, connID)
		} else {
			utils.Debugf("HTTP long polling: [HANDLE_POLL] timeout response written successfully, connID=%s", connID)
		}
	}
}

// handlePollError 处理 Poll 请求的错误
// 根据错误类型返回适当的 HTTP 响应
func (s *ManagementAPIServer) handlePollError(
	w http.ResponseWriter,
	err error,
	streamProcessor *httppoll.ServerStreamProcessor,
	connID string,
	requestID string,
	tunnelType string,
) bool {
	// 1. 检查 StreamProcessor 状态 - 连接已失效
	if streamProcessor.IsClosed() || streamProcessor.IsContextDone() {
		utils.Warnf("HTTP long polling: [HANDLE_POLL] Connection closed or context done, connID=%s", connID)

		// 注意：不从 registry 删除连接，因为：
		// 1. 连接清理应该由 SessionManager 统一管理
		// 2. Registry 只是查找缓存，删除后再创建会导致冲突
		// 3. SessionManager 的清理机制会自动处理过期连接

		// 返回 410 Gone，告诉客户端需要重新握手（使用新的 connectionID）
		resp := HTTPPollResponse{
			Success:   false,
			Error:     errCodeConnectionClosed,
			Message:   errMsgConnectionClosed,
			Timestamp: time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone) // 410 Gone
		json.NewEncoder(w).Encode(resp)
		return true
	}

	// 2. 正常的超时（context deadline exceeded）
	if err == context.DeadlineExceeded {
		utils.Debugf("HTTP long polling: [HANDLE_POLL] Normal timeout, connID=%s", connID)
		s.writeTimeoutResponse(w, connID, requestID, tunnelType)
		return true
	}

	// 3. 正常的客户端取消（context canceled 或 EOF）
	if err == context.Canceled || err == io.EOF {
		utils.Debugf("HTTP long polling: [HANDLE_POLL] Client canceled or EOF (normal), connID=%s", connID)
		s.writeTimeoutResponse(w, connID, requestID, tunnelType)
		return true
	}

	// 4. 限流错误
	if coreErrors.GetErrorType(err) == coreErrors.ErrorTypeRateLimited {
		utils.Warnf("HTTP long polling: [HANDLE_POLL] Rate limited, connID=%s", connID)
		s.writeRateLimitResponse(w)
		return true
	}

	// 5. 其他真正的错误
	utils.Errorf("HTTP long polling: [HANDLE_POLL] Unexpected error: %v, connID=%s", err, connID)
	s.respondError(w, http.StatusInternalServerError, err.Error())
	return true
}

// writeTimeoutResponse 写入超时响应（200 OK + timeout=true）
func (s *ManagementAPIServer) writeTimeoutResponse(
	w http.ResponseWriter,
	connID string,
	requestID string,
	tunnelType string,
) {
	// 为控制连接返回心跳包，避免客户端 ReadPacket 超时导致重连
	if tunnelType == "control" {
		heartbeatPkg := &httppoll.TunnelPackage{
			Type:         "Heartbeat",
			ConnectionID: connID,
			RequestID:    requestID,
		}
		encodedPkg, err := httppoll.EncodeTunnelPackage(heartbeatPkg)
		if err == nil {
			w.Header().Set("X-Tunnel-Package", encodedPkg)
		} else {
			utils.Errorf("HTTP long polling: [HANDLE_POLL] Failed to encode heartbeat package: %v, connID=%s", err, connID)
		}
	}

	resp := HTTPPollResponse{
		Success:   true,
		Timeout:   true,
		Timestamp: time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// writeRateLimitResponse 写入限流响应（429 Too Many Requests）
func (s *ManagementAPIServer) writeRateLimitResponse(w http.ResponseWriter) {
	resp := HTTPPollResponse{
		Success:   false,
		Error:     errCodeRateLimited,
		Message:   errMsgRateLimited,
		Timestamp: time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(resp)
}

