package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

// HandleHTTPPush 处理 HTTP Poll Push 请求（公开方法，供协议层调用）
// POST /tunnox/v1/push
func (s *ManagementAPIServer) HandleHTTPPush(w http.ResponseWriter, r *http.Request) {
	utils.Debugf("HTTP long polling: [HANDLE_PUSH] received Push request, method=%s, contentLength=%d", r.Method, r.ContentLength)

	// 1. 获取并解码 X-Tunnel-Package（必须）
	packageHeader := r.Header.Get("X-Tunnel-Package")
	if packageHeader == "" {
		utils.Errorf("HTTP long polling: [HANDLE_PUSH] missing X-Tunnel-Package header")
		s.respondError(w, http.StatusBadRequest, "missing X-Tunnel-Package header")
		return
	}
	utils.Debugf("HTTP long polling: [HANDLE_PUSH] X-Tunnel-Package len=%d", len(packageHeader))

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
	if s.httppollRegistry == nil {
		s.httppollRegistry = httppoll.NewConnectionRegistry()
	}

	// 使用 GetOrCreate 确保原子性（避免并发创建）
	streamProcessor := s.httppollRegistry.GetOrCreate(connID, func() *httppoll.ServerStreamProcessor {
		return s.createHTTPLongPollingConnection(connID, pkg, r.Context())
	})
	if streamProcessor == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Failed to create connection")
		return
	}

	// 更新 clientID 和 mappingID（如果需要）
	if pkg.ClientID > 0 {
		streamProcessor.UpdateClientID(pkg.ClientID)
	}
	if pkg.MappingID != "" {
		streamProcessor.SetMappingID(pkg.MappingID)
	}

	// 6. 处理 Push 请求（body 可能为空，用于控制包）
	var pushReq HTTPPushRequest
	if r.Body != nil {
		// 尝试读取请求体（不依赖ContentLength，因为可能未正确设置）
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			if err := json.Unmarshal(bodyBytes, &pushReq); err == nil {
				// 处理数据流（支持分片格式）
				if pushReq.Data != "" {
					// 判断是否为分片：total_fragments > 1
					isFragment := pushReq.TotalFragments > 1

					if isFragment {
						// 使用统一的分片处理器（立即处理，不需要序列号顺序）
						processor := httppoll.NewImmediateFragmentProcessor(streamProcessor.GetFragmentReassembler())
						isComplete, reassembledData, err := httppoll.ProcessFragmentFromResponse(processor, pushReq)
						if err != nil {
							utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to process fragment: %v, connID=%s", err, connID)
							s.respondError(w, http.StatusBadRequest, "Failed to process fragment")
							return
						}

						if isComplete {
							// 分片组已完整并重组
							// Base64编码重组后的数据
							base64Data := base64.StdEncoding.EncodeToString(reassembledData)
							utils.Debugf("HTTP long polling: [HANDLE_PUSH] reassembled %d bytes from %d fragments, groupID=%s, connID=%s",
								len(reassembledData), pushReq.TotalFragments, pushReq.FragmentGroupID, connID)

							// 推送到流处理器
							if err := streamProcessor.PushData(base64Data); err != nil {
								utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to push reassembled data: %v, connID=%s", err, connID)
								s.respondError(w, http.StatusServiceUnavailable, "Connection closed")
								return
							}
						} else {
							// 分片组不完整或已被其他 goroutine 重组，等待更多分片
							utils.Debugf("HTTP long polling: [HANDLE_PUSH] fragment %d/%d received, waiting for more fragments or already reassembled, groupID=%s, connID=%s",
								pushReq.FragmentIndex, pushReq.TotalFragments, pushReq.FragmentGroupID, connID)
						}
					} else {
						// 完整数据（TotalFragments=1 或非分片格式），直接推送
						if err := streamProcessor.PushData(pushReq.Data); err != nil {
							utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to push data: %v, connID=%s", err, connID)
							s.respondError(w, http.StatusServiceUnavailable, "Connection closed")
							return
						}
						utils.Debugf("HTTP long polling: [HANDLE_PUSH] pushed data successfully, dataLen=%d, connID=%s", len(pushReq.Data), connID)
					}
				} else {
					utils.Debugf("HTTP long polling: [HANDLE_PUSH] body parsed but data field is empty, connID=%s", connID)
				}
			} else {
				utils.Debugf("HTTP long polling: [HANDLE_PUSH] failed to parse JSON body: %v, bodyLen=%d, connID=%s", err, len(bodyBytes), connID)
			}
		} else if err != nil {
			utils.Debugf("HTTP long polling: [HANDLE_PUSH] failed to read body: %v, connID=%s", err, connID)
		}
	}

	// 7. 处理控制包（如果有 type 字段）
	var responsePkg *httppoll.TunnelPackage
	if pkg.Type != "" {
		utils.Debugf("HTTP long polling: [HANDLE_PUSH] processing control package, type=%s, connID=%s", pkg.Type, connID)
		responsePkg = s.handleControlPackage(streamProcessor, pkg)
	}

	// 8. 返回响应（如果有控制包响应，放在 X-Tunnel-Package 中）
	if responsePkg != nil {
		// 设置响应包的连接信息
		responsePkg.ConnectionID = connID
		responsePkg.ClientID = streamProcessor.GetClientID()
		responsePkg.MappingID = streamProcessor.GetMappingID()
		responsePkg.TunnelType = pkg.TunnelType
		// 携带请求的 RequestId（如果存在）
		if pkg.RequestID != "" {
			responsePkg.RequestID = pkg.RequestID
		}
		encodedPkg, err := httppoll.EncodeTunnelPackage(responsePkg)
		if err == nil {
			w.Header().Set("X-Tunnel-Package", encodedPkg)
			utils.Debugf("HTTP long polling: [HANDLE_PUSH] set X-Tunnel-Package header, len=%d, connID=%s", len(encodedPkg), connID)
		} else {
			utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to encode response package: %v, connID=%s", err, connID)
		}
	}

	// 9. 返回响应
	utils.Debugf("HTTP long polling: [HANDLE_PUSH] preparing response, connID=%s", connID)
	resp := HTTPPushResponse{
		Success:   true,
		Timestamp: time.Now().Unix(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	utils.Debugf("HTTP long polling: [HANDLE_PUSH] writing ACK response, connID=%s", connID)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		utils.Errorf("HTTP long polling: [HANDLE_PUSH] failed to write response: %v, connID=%s", err, connID)
		return
	}
	utils.Debugf("HTTP long polling: [HANDLE_PUSH] response written successfully, connID=%s", connID)
}

