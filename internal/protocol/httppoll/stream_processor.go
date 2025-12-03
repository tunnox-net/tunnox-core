package httppoll

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/google/uuid"
)

const (
	defaultPollTimeout = 30 * time.Second
	maxRetries         = 3
	retryInterval      = 1 * time.Second
	maxBufferSize      = 1024 * 1024      // 1MB
	responseCacheTTL   = 60 * time.Second // 响应缓存过期时间
)

// StreamProcessor HTTP 长轮询流处理器
// 实现 stream.PackageStreamer 接口，内部使用 PacketConverter 进行转换
type StreamProcessor struct {
	*dispose.ManagerBase

	converter  *PacketConverter
	httpClient *http.Client
	pushURL    string
	pollURL    string

	// 连接信息
	connectionID string
	clientID     int64
	mappingID    string
	tunnelType   string

	// 数据流缓冲
	dataBuffer  *bytes.Buffer
	dataBufMu   sync.Mutex
	packetQueue chan *packet.TransferPacket

	// 分片重组器（用于处理服务器端发送的分片数据）
	fragmentReassembler *FragmentReassembler

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// 用于客户端：token 和 instanceID
	token      string
	instanceID string

	// Poll 响应缓存（RequestID -> 响应）
	responseCache   map[string]*cachedResponse
	responseCacheMu sync.RWMutex
	pollRequestChan chan string // 用于通知 pollLoop 发送新的 Poll 请求

	// 待使用的 Poll 请求 ID（由 TriggerImmediatePoll 设置，供 ReadPacket 使用）
	pendingPollRequestID string
	pendingPollRequestMu sync.Mutex
}

// cachedResponse 缓存的响应
type cachedResponse struct {
	pkt       *packet.TransferPacket
	expiresAt time.Time
}

// NewStreamProcessor 创建 HTTP 长轮询流处理器
func NewStreamProcessor(ctx context.Context, baseURL, pushURL, pollURL string, clientID int64, token, instanceID, mappingID string) *StreamProcessor {
	connType := "control"
	if mappingID != "" {
		connType = "data"
	}

	sp := &StreamProcessor{
		ManagerBase: dispose.NewManager("HTTPStreamProcessor", ctx),
		converter:   NewPacketConverter(),
		httpClient: &http.Client{
			Timeout: defaultPollTimeout + 5*time.Second,
		},
		pushURL:             pushURL,
		pollURL:             pollURL,
		clientID:            clientID,
		token:               token,
		instanceID:          instanceID,
		mappingID:           mappingID,
		tunnelType:          connType,
		dataBuffer:          bytes.NewBuffer(nil),
		packetQueue:         make(chan *packet.TransferPacket, 100),
		responseCache:       make(map[string]*cachedResponse),
		pollRequestChan:     make(chan string, 10),    // 缓冲 10 个请求
		fragmentReassembler: NewFragmentReassembler(), // 创建分片重组器
	}

	sp.converter.SetConnectionInfo("", clientID, mappingID, connType)

	sp.AddCleanHandler(sp.onClose)

	// 启动 Poll 循环
	go sp.pollLoop()

	return sp
}

// onClose 资源清理
func (sp *StreamProcessor) onClose() error {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()

	if sp.closed {
		return nil
	}
	sp.closed = true

	close(sp.packetQueue)
	close(sp.pollRequestChan)
	sp.dataBuffer.Reset()

	// 清理响应缓存
	sp.responseCacheMu.Lock()
	sp.responseCache = make(map[string]*cachedResponse)
	sp.responseCacheMu.Unlock()

	return nil
}

// pollLoop 持续发送 Poll 请求并缓存响应
func (sp *StreamProcessor) pollLoop() {
	for {
		select {
		case <-sp.Ctx().Done():
			return
		case requestID, ok := <-sp.pollRequestChan:
			if !ok {
				return
			}
			// 发送 Poll 请求
			sp.sendPollRequest(requestID)
		}
	}
}

// TriggerImmediatePoll 立即触发一个 Poll 请求（用于发送命令后快速获取响应）
// 返回的 RequestID 应该被 ReadPacket 使用
func (sp *StreamProcessor) TriggerImmediatePoll() string {
	requestID := uuid.New().String()
	// 设置待使用的 RequestID
	sp.pendingPollRequestMu.Lock()
	sp.pendingPollRequestID = requestID
	sp.pendingPollRequestMu.Unlock()

	select {
	case sp.pollRequestChan <- requestID:
		utils.Infof("[CMD_TRACE] [CLIENT] [TRIGGER_POLL_IMMEDIATE] RequestID=%s, ConnID=%s, Time=%s",
			requestID, sp.connectionID, time.Now().Format("15:04:05.000"))
		return requestID
	case <-sp.Ctx().Done():
		sp.pendingPollRequestMu.Lock()
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		return ""
	default:
		// 通道满，清除待使用的 RequestID
		sp.pendingPollRequestMu.Lock()
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		utils.Warnf("[CMD_TRACE] [CLIENT] [TRIGGER_POLL_IMMEDIATE_WARN] RequestID=%s, Reason=pollRequestChan_full, Time=%s",
			requestID, time.Now().Format("15:04:05.000"))
		return ""
	}
}

// sendPollRequest 发送单个 Poll 请求并缓存响应
func (sp *StreamProcessor) sendPollRequest(requestID string) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		utils.Debugf("HTTPStreamProcessor: sendPollRequest - connection closed, requestID=%s", requestID)
		return
	}

	pollStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [CLIENT] [POLL_START] RequestID=%s, ConnID=%s, Time=%s",
		requestID, connID, pollStartTime.Format("15:04:05.000"))

	// 构建 Poll 请求的 TunnelPackage
	pollPkg := &TunnelPackage{
		ConnectionID: connID,
		RequestID:    requestID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType,
	}
	encoded, err := EncodeTunnelPackage(pollPkg)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to encode poll package: %v, requestID=%s", err, requestID)
		return
	}

	// 发送 Poll 请求
	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to create poll request: %v, requestID=%s", err, requestID)
		return
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	utils.Infof("HTTPStreamProcessor: sendPollRequest - Poll request sent, requestID=%s, encodedLen=%d", requestID, len(encoded))
	resp, err := sp.httpClient.Do(req)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - Poll request failed: %v, requestID=%s", err, requestID)
		return
	}
	defer resp.Body.Close()

	utils.Infof("HTTPStreamProcessor: sendPollRequest - Poll response received, status=%d, requestID=%s", resp.StatusCode, requestID)

	// 检查是否有控制包（X-Tunnel-Package 中）
	xTunnelPackage := resp.Header.Get("X-Tunnel-Package")
	utils.Infof("HTTPStreamProcessor: sendPollRequest - checking X-Tunnel-Package header, present=%v, len=%d, requestID=%s",
		xTunnelPackage != "", len(xTunnelPackage), requestID)
	if xTunnelPackage != "" {
		// 解码 TunnelPackage 以检查 RequestId
		pkg, err := DecodeTunnelPackage(xTunnelPackage)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to decode tunnel package: %v, requestID=%s", err, requestID)
			return
		}

		utils.Infof("HTTPStreamProcessor: sendPollRequest - decoded tunnel package, requestID in response=%s, expected=%s",
			pkg.RequestID, requestID)

		// 检查 RequestId 是否匹配
		if pkg.RequestID != requestID {
			utils.Warnf("HTTPStreamProcessor: sendPollRequest - RequestId mismatch, expected=%s, got=%s, ignoring response",
				requestID, pkg.RequestID)
			return
		}

		// 转换为 TransferPacket
		pkt, err := sp.converter.ReadPacket(resp)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to read packet: %v, requestID=%s", err, requestID)
			return
		}

		// 更新连接信息
		if pkg.ConnectionID != "" {
			sp.SetConnectionID(pkg.ConnectionID)
		}

		// 缓存响应
		sp.responseCacheMu.Lock()
		sp.responseCache[requestID] = &cachedResponse{
			pkt:       pkt,
			expiresAt: time.Now().Add(responseCacheTTL),
		}
		sp.responseCacheMu.Unlock()

		utils.Infof("HTTPStreamProcessor: sendPollRequest - cached response, requestID=%s, type=0x%02x",
			requestID, byte(pkt.PacketType)&0x3F)
	}

	// 处理数据流（如果有）- 支持分片数据
	var pollResp FragmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		// 判断是否为分片：total_fragments > 1
		isFragment := pollResp.TotalFragments > 1
		utils.Infof("HTTPStreamProcessor[%s]: sendPollRequest - received data, groupID=%s, index=%d/%d, size=%d, originalSize=%d, isFragment=%v, requestID=%s, connID=%s",
			sp.connectionID, pollResp.FragmentGroupID, pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentSize, pollResp.OriginalSize, isFragment, requestID, sp.connectionID)

		// 解码Base64数据
		fragmentData, err := base64.StdEncoding.DecodeString(pollResp.Data)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to decode fragment data: %v, requestID=%s", err, requestID)
			return
		}

		// 验证解码后的数据长度是否与 FragmentSize 匹配
		if len(fragmentData) != pollResp.FragmentSize {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - fragment size mismatch: expected %d, got %d, groupID=%s, index=%d, requestID=%s",
				pollResp.FragmentSize, len(fragmentData), pollResp.FragmentGroupID, pollResp.FragmentIndex, requestID)
			return
		}

		// 如果是分片，需要重组
		if isFragment {
			// 添加到分片重组器
			// 注意：使用 FragmentSize 字段（这是实际数据长度，CreateFragmentResponse 中设置的）
			utils.Debugf("HTTPStreamProcessor: sendPollRequest - adding fragment, groupID=%s, index=%d/%d, size=%d, originalSize=%d, requestID=%s",
				pollResp.FragmentGroupID, pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentSize, pollResp.OriginalSize, requestID)
			group, err := sp.fragmentReassembler.AddFragment(
				pollResp.FragmentGroupID,
				pollResp.OriginalSize,
				pollResp.FragmentSize,
				pollResp.FragmentIndex,
				pollResp.TotalFragments,
				pollResp.SequenceNumber,
				fragmentData,
			)
			if err != nil {
				utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to add fragment: %v, groupID=%s, index=%d, requestID=%s", err, pollResp.FragmentGroupID, pollResp.FragmentIndex, requestID)
				return
			}

			// 使用原子操作检查是否完整（避免竞态条件）
			// 注意：不在这里重组，而是通过 GetNextCompleteGroup 按序列号顺序重组
			isComplete := group.IsComplete()
			if !isComplete {
				// 分片组不完整，继续等待更多分片
				utils.Debugf("HTTPStreamProcessor: sendPollRequest - fragment %d/%d received, waiting for more, groupID=%s, receivedCount=%d, requestID=%s",
					pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentGroupID, group.ReceivedCount, requestID)
				return
			}

			// 分片组完整，检查是否可以按序列号顺序发送
			// 使用 GetNextCompleteGroup 确保按序列号顺序发送
			utils.Infof("HTTPStreamProcessor: sendPollRequest - fragment group complete, checking sequence order, groupID=%s, sequenceNumber=%d, requestID=%s",
				pollResp.FragmentGroupID, pollResp.SequenceNumber, requestID)

			// 尝试获取下一个按序列号顺序的完整分片组
			nextGroup, found, err := sp.fragmentReassembler.GetNextCompleteGroup()
			if err != nil {
				utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to get next complete group: %v, requestID=%s", err, requestID)
				return
			}

			if found {
				// 这是下一个应该发送的分片组，重组并发送
				reassembledData, err := nextGroup.Reassemble()
				if err != nil {
					utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to reassemble: %v, groupID=%s, requestID=%s", err, pollResp.FragmentGroupID, requestID)
					sp.fragmentReassembler.RemoveGroup(pollResp.FragmentGroupID)
					return
				}

				utils.Infof("HTTPStreamProcessor: sendPollRequest - reassembled %d bytes from %d fragments, groupID=%s, sequenceNumber=%d, originalSize=%d, requestID=%s",
					len(reassembledData), nextGroup.TotalFragments, nextGroup.GroupID, nextGroup.SequenceNumber, nextGroup.OriginalSize, requestID)
				// 验证重组后的数据大小
				if len(reassembledData) != nextGroup.OriginalSize {
					utils.Errorf("HTTPStreamProcessor: sendPollRequest - reassembled size mismatch: expected %d, got %d, groupID=%s, requestID=%s",
						nextGroup.OriginalSize, len(reassembledData), nextGroup.GroupID, requestID)
					sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
					return
				}
				sp.dataBufMu.Lock()
				oldBufferLen := sp.dataBuffer.Len()
				if sp.dataBuffer.Len()+len(reassembledData) <= maxBufferSize {
					n, err := sp.dataBuffer.Write(reassembledData)
					if err != nil {
						utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to write to data buffer: %v, requestID=%s", err, requestID)
					} else {
						utils.Infof("HTTPStreamProcessor: sendPollRequest - wrote %d bytes to data buffer, buffer size: %d -> %d, sequenceNumber=%d, requestID=%s",
							n, oldBufferLen, sp.dataBuffer.Len(), pollResp.SequenceNumber, requestID)
					}
				} else {
					utils.Errorf("HTTPStreamProcessor: sendPollRequest - data buffer full, dropping %d bytes, buffer size=%d, requestID=%s", len(reassembledData), sp.dataBuffer.Len(), requestID)
				}
				sp.dataBufMu.Unlock()

				// 移除分片组
				sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)

				// 继续检查是否有更多按序列号顺序的完整分片组
				for {
					nextGroup2, found2, err2 := sp.fragmentReassembler.GetNextCompleteGroup()
					if err2 != nil || !found2 {
						break
					}
					reassembledData2, err2 := nextGroup2.Reassemble()
					if err2 != nil {
						utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to reassemble next group: %v, groupID=%s, requestID=%s", err2, nextGroup2.GroupID, requestID)
						sp.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
						break
					}
					utils.Infof("HTTPStreamProcessor: sendPollRequest - sending next complete group, groupID=%s, sequenceNumber=%d, size=%d, requestID=%s",
						nextGroup2.GroupID, nextGroup2.SequenceNumber, len(reassembledData2), requestID)
					sp.dataBufMu.Lock()
					if sp.dataBuffer.Len()+len(reassembledData2) <= maxBufferSize {
						n, err := sp.dataBuffer.Write(reassembledData2)
						if err != nil {
							utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to write next group: %v, requestID=%s", err, requestID)
						} else {
							utils.Infof("HTTPStreamProcessor: sendPollRequest - wrote next group %d bytes, sequenceNumber=%d, requestID=%s",
								n, nextGroup2.SequenceNumber, requestID)
						}
					} else {
						utils.Errorf("HTTPStreamProcessor: sendPollRequest - data buffer full, dropping next group %d bytes, requestID=%s", len(reassembledData2), requestID)
					}
					sp.dataBufMu.Unlock()
					sp.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
				}
			} else {
				// 这不是下一个应该发送的分片组，等待序列号更小的分片组完成
				utils.Debugf("HTTPStreamProcessor: sendPollRequest - fragment group complete but not next in sequence, groupID=%s, sequenceNumber=%d, waiting for earlier groups, requestID=%s",
					pollResp.FragmentGroupID, pollResp.SequenceNumber, requestID)
			}
		} else {
			// 单分片数据（TotalFragments=1），也需要按序列号顺序发送
			// 添加到分片重组器，以便按序列号顺序处理
			utils.Infof("HTTPStreamProcessor[%s]: sendPollRequest - received single fragment (TotalFragments=1), adding to reassembler for sequence ordering, groupID=%s, sequenceNumber=%d, requestID=%s, connID=%s",
				sp.connectionID, pollResp.FragmentGroupID, pollResp.SequenceNumber, requestID, sp.connectionID)
			_, err := sp.fragmentReassembler.AddFragment(
				pollResp.FragmentGroupID,
				pollResp.OriginalSize,
				pollResp.FragmentSize,
				pollResp.FragmentIndex,
				pollResp.TotalFragments,
				pollResp.SequenceNumber,
				fragmentData,
			)
			if err != nil {
				utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to add single fragment: %v, groupID=%s, requestID=%s", err, pollResp.FragmentGroupID, requestID)
				return
			}

			// 单分片数据应该立即完整，检查是否可以按序列号顺序发送
			utils.Infof("HTTPStreamProcessor[%s]: sendPollRequest - single fragment complete, checking sequence order, groupID=%s, sequenceNumber=%d, requestID=%s, connID=%s",
				sp.connectionID, pollResp.FragmentGroupID, pollResp.SequenceNumber, requestID, sp.connectionID)

			// 尝试获取下一个按序列号顺序的完整分片组
			nextGroup, found, err := sp.fragmentReassembler.GetNextCompleteGroup()
			if err != nil {
				utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to get next complete group for single fragment: %v, requestID=%s", err, requestID)
				return
			}

			if found {
				utils.Infof("HTTPStreamProcessor[%s]: sendPollRequest - GetNextCompleteGroup found next group for single fragment, groupID=%s, sequenceNumber=%d, requestID=%s, connID=%s",
					sp.connectionID, nextGroup.GroupID, nextGroup.SequenceNumber, requestID, sp.connectionID)
				// 这是下一个应该发送的分片组，重组并发送
				reassembledData, err := nextGroup.Reassemble()
				if err != nil {
					utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to reassemble single fragment: %v, groupID=%s, requestID=%s", err, nextGroup.GroupID, requestID)
					sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
					return
				}

				utils.Infof("HTTPStreamProcessor[%s]: sendPollRequest - reassembled single fragment, groupID=%s, sequenceNumber=%d, originalSize=%d, requestID=%s, connID=%s",
					sp.connectionID, nextGroup.GroupID, nextGroup.SequenceNumber, nextGroup.OriginalSize, requestID, sp.connectionID)
				// 验证重组后的数据大小
				if len(reassembledData) != nextGroup.OriginalSize {
					utils.Errorf("HTTPStreamProcessor: sendPollRequest - reassembled size mismatch: expected %d, got %d, groupID=%s, requestID=%s",
						nextGroup.OriginalSize, len(reassembledData), nextGroup.GroupID, requestID)
					sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
					return
				}
				sp.dataBufMu.Lock()
				oldBufferLen := sp.dataBuffer.Len()
				if sp.dataBuffer.Len()+len(reassembledData) <= maxBufferSize {
					n, err := sp.dataBuffer.Write(reassembledData)
					if err != nil {
						utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to write to data buffer: %v, requestID=%s", err, requestID)
					} else {
						utils.Infof("HTTPStreamProcessor: sendPollRequest - wrote %d bytes to data buffer, buffer size: %d -> %d, sequenceNumber=%d, requestID=%s",
							n, oldBufferLen, sp.dataBuffer.Len(), nextGroup.SequenceNumber, requestID)
					}
				} else {
					utils.Errorf("HTTPStreamProcessor: sendPollRequest - data buffer full, dropping %d bytes, buffer size=%d, requestID=%s", len(reassembledData), sp.dataBuffer.Len(), requestID)
				}
				sp.dataBufMu.Unlock()

				// 移除分片组
				sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)

				// 继续检查是否有更多按序列号顺序的完整分片组
				for {
					nextGroup2, found2, err2 := sp.fragmentReassembler.GetNextCompleteGroup()
					if err2 != nil || !found2 {
						break
					}
					reassembledData2, err2 := nextGroup2.Reassemble()
					if err2 != nil {
						utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to reassemble next group: %v, groupID=%s, requestID=%s", err2, nextGroup2.GroupID, requestID)
						sp.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
						break
					}
					utils.Infof("HTTPStreamProcessor: sendPollRequest - sending next complete group, groupID=%s, sequenceNumber=%d, size=%d, requestID=%s",
						nextGroup2.GroupID, nextGroup2.SequenceNumber, len(reassembledData2), requestID)
					sp.dataBufMu.Lock()
					if sp.dataBuffer.Len()+len(reassembledData2) <= maxBufferSize {
						n, err := sp.dataBuffer.Write(reassembledData2)
						if err != nil {
							utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to write next group: %v, requestID=%s", err, requestID)
						} else {
							utils.Infof("HTTPStreamProcessor: sendPollRequest - wrote next group %d bytes, sequenceNumber=%d, requestID=%s",
								n, nextGroup2.SequenceNumber, requestID)
						}
					} else {
						utils.Errorf("HTTPStreamProcessor: sendPollRequest - data buffer full, dropping next group %d bytes, requestID=%s", len(reassembledData2), requestID)
					}
					sp.dataBufMu.Unlock()
					sp.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
				}
			} else {
				// 这不是下一个应该发送的分片组，等待序列号更小的分片组完成
				utils.Infof("HTTPStreamProcessor[%s]: sendPollRequest - single fragment complete but GetNextCompleteGroup returned not found, groupID=%s, sequenceNumber=%d, waiting for expected sequence, requestID=%s, connID=%s",
					sp.connectionID, pollResp.FragmentGroupID, pollResp.SequenceNumber, requestID, sp.connectionID)
			}
		}
	} else if pollResp.Timeout {
		utils.Debugf("HTTPStreamProcessor: sendPollRequest - poll request timeout, requestID=%s", requestID)
	}
}

// cleanupExpiredResponses 清理过期的响应缓存
func (sp *StreamProcessor) cleanupExpiredResponses() {
	now := time.Now()
	sp.responseCacheMu.Lock()
	defer sp.responseCacheMu.Unlock()

	for requestID, cached := range sp.responseCache {
		if now.After(cached.expiresAt) {
			delete(sp.responseCache, requestID)
		}
	}
}

// SetConnectionID 设置连接 ID（服务端分配）
func (sp *StreamProcessor) SetConnectionID(connID string) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.connectionID = connID
	sp.converter.SetConnectionInfo(connID, sp.clientID, sp.mappingID, sp.tunnelType)
}

// UpdateClientID 更新客户端 ID
func (sp *StreamProcessor) UpdateClientID(newClientID int64) {
	sp.closeMu.Lock()
	defer sp.closeMu.Unlock()
	sp.clientID = newClientID
	sp.converter.SetConnectionInfo(sp.connectionID, newClientID, sp.mappingID, sp.tunnelType)
}

// ReadPacket 从响应缓存中读取包
func (sp *StreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return nil, 0, io.EOF
	}

	// 如果 connectionID 为空，说明还没有从服务端获取到 ConnectionID
	// 在握手阶段，客户端会先发送 Push（握手请求），然后立即发送 Poll（等待握手响应）
	// 此时 connectionID 可能还是空的，需要等待服务端在握手响应中分配
	if connID == "" {
		// 先检查 packetQueue 中是否有响应包（Push 请求的响应可能已经在队列中）
		select {
		case pkt := <-sp.packetQueue:
			return pkt, 0, nil
		default:
		}
	}

	// 检查是否有待使用的 Poll 请求 ID（由 TriggerImmediatePoll 设置）
	sp.pendingPollRequestMu.Lock()
	requestID := sp.pendingPollRequestID
	if requestID != "" {
		// 使用已触发的 Poll 请求 ID，并清除
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		utils.Infof("[CMD_TRACE] [CLIENT] [READ_START] RequestID=%s (from TriggerImmediatePoll), ConnID=%s, Time=%s",
			requestID, connID, time.Now().Format("15:04:05.000"))
	} else {
		// 生成新的 RequestId
		requestID = uuid.New().String()
		sp.pendingPollRequestMu.Unlock()

		readStartTime := time.Now()
		utils.Infof("[CMD_TRACE] [CLIENT] [READ_START] RequestID=%s (new), ConnID=%s, Time=%s",
			requestID, connID, readStartTime.Format("15:04:05.000"))

		// 通知 pollLoop 发送 Poll 请求
		select {
		case sp.pollRequestChan <- requestID:
			utils.Infof("[CMD_TRACE] [CLIENT] [POLL_TRIGGER] RequestID=%s, ConnID=%s, Time=%s",
				requestID, connID, time.Now().Format("15:04:05.000"))
		case <-sp.Ctx().Done():
			return nil, 0, sp.Ctx().Err()
		default:
			// 通道满，直接返回（pollLoop 会继续处理）
			utils.Warnf("[CMD_TRACE] [CLIENT] [POLL_TRIGGER_FAILED] RequestID=%s, ConnID=%s, Reason=channel_full, Time=%s",
				requestID, connID, time.Now().Format("15:04:05.000"))
		}
	}

	readStartTime := time.Now()

	// 从缓存中查找响应（带超时）
	timeout := time.NewTimer(35 * time.Second) // 比 Poll 超时稍长
	defer timeout.Stop()

	// 优化：先立即检查一次缓存（可能响应已经到达）
	sp.responseCacheMu.RLock()
	cached, exists := sp.responseCache[requestID]
	sp.responseCacheMu.RUnlock()
	if exists {
		// 找到响应，从缓存中删除
		sp.responseCacheMu.Lock()
		delete(sp.responseCache, requestID)
		sp.responseCacheMu.Unlock()

		baseType := byte(cached.pkt.PacketType) & 0x3F
		var commandID string
		if cached.pkt.CommandPacket != nil {
			commandID = cached.pkt.CommandPacket.CommandId
		}
		utils.Infof("[CMD_TRACE] [CLIENT] [READ_IMMEDIATE] RequestID=%s, CommandID=%s, PacketType=0x%02x, Time=%s",
			requestID, commandID, baseType, time.Now().Format("15:04:05.000"))
		return cached.pkt, 0, nil
	}

	// 定期清理过期响应
	sp.cleanupExpiredResponses()

	// 使用更短的检查间隔（10ms），提高响应速度
	ticker := time.NewTicker(10 * time.Millisecond) // 每 10ms 检查一次
	defer ticker.Stop()

	// 用于定期清理过期响应（每 1 秒清理一次）
	lastCleanup := time.Now()

	for {
		select {
		case <-sp.Ctx().Done():
			return nil, 0, sp.Ctx().Err()
		case <-timeout.C:
			utils.Debugf("HTTPStreamProcessor: ReadPacket - timeout waiting for response, requestID=%s", requestID)
			return nil, 0, fmt.Errorf("timeout waiting for response")
		case <-ticker.C:
			// 检查缓存
			sp.responseCacheMu.RLock()
			cached, exists = sp.responseCache[requestID]
			sp.responseCacheMu.RUnlock()

			if exists {
				// 找到响应，从缓存中删除
				sp.responseCacheMu.Lock()
				delete(sp.responseCache, requestID)
				sp.responseCacheMu.Unlock()

				readDuration := time.Since(readStartTime)
				baseType := byte(cached.pkt.PacketType) & 0x3F
				var commandID string
				if cached.pkt.CommandPacket != nil {
					commandID = cached.pkt.CommandPacket.CommandId
				}
				utils.Infof("[CMD_TRACE] [CLIENT] [READ_COMPLETE] RequestID=%s, CommandID=%s, PacketType=0x%02x, ReadDuration=%v, Time=%s",
					requestID, commandID, baseType, readDuration, time.Now().Format("15:04:05.000"))
				return cached.pkt, 0, nil
			}

			// 定期清理过期响应（每 1 秒清理一次，避免频繁清理）
			if time.Since(lastCleanup) >= time.Second {
				sp.cleanupExpiredResponses()
				lastCleanup = time.Now()
			}
		}
	}
}

// WritePacket 通过 HTTP Push 发送包
func (sp *StreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return 0, io.ErrClosedPipe
	}

	// 1. 生成 RequestId（用于匹配请求和响应）
	requestID := uuid.New().String()

	// [CMD_TRACE] 记录 Push 请求开始
	writeStartTime := time.Now()
	baseType := byte(pkt.PacketType) & 0x3F
	var commandID string
	if pkt.CommandPacket != nil {
		commandID = pkt.CommandPacket.CommandId
	}
	utils.Infof("[CMD_TRACE] [CLIENT] [PUSH_START] RequestID=%s, CommandID=%s, PacketType=0x%02x, ConnID=%s, Time=%s",
		requestID, commandID, baseType, connID, writeStartTime.Format("15:04:05.000"))

	// 2. 更新转换器的连接状态
	sp.converter.SetConnectionInfo(sp.connectionID, sp.clientID, sp.mappingID, sp.tunnelType)

	// 3. 转换为 HTTP Request（携带 RequestId）
	req, err := sp.converter.WritePacket(pkt, requestID)
	if err != nil {
		return 0, fmt.Errorf("failed to convert packet: %w", err)
	}

	// 3. 设置请求 URL 和认证
	reqURL, err := url.Parse(sp.pushURL)
	if err != nil {
		return 0, fmt.Errorf("invalid push URL: %w", err)
	}
	req.URL = reqURL

	// 检查 context 是否已取消
	select {
	case <-sp.Ctx().Done():
		utils.Errorf("HTTPStreamProcessor: WritePacket - context canceled before sending Push request, requestID=%s, connID=%s, err=%v", requestID, sp.connectionID, sp.Ctx().Err())
		return 0, fmt.Errorf("push request failed: context canceled: %w", sp.Ctx().Err())
	default:
	}

	utils.Infof("HTTPStreamProcessor: WritePacket - sending Push request, requestID=%s, connID=%s, type=0x%02x", requestID, sp.connectionID, byte(pkt.PacketType)&0x3F)

	// 4. 发送请求（带重试）
	var resp *http.Response
	for retry := 0; retry < maxRetries; retry++ {
		// 使用独立的 context，避免被主 context 取消影响
		reqCtx, reqCancel := context.WithTimeout(context.Background(), 10*time.Second)
		reqWithCtx := req.WithContext(reqCtx)

		resp, err = sp.httpClient.Do(reqWithCtx)
		reqCancel() // 立即取消 context，释放资源

		if err == nil {
			utils.Infof("HTTPStreamProcessor: WritePacket - Push request sent successfully, requestID=%s, connID=%s", requestID, sp.connectionID)
			break
		}
		utils.Warnf("HTTPStreamProcessor: WritePacket - Push request failed (retry %d/%d), requestID=%s, connID=%s, err=%v", retry+1, maxRetries, requestID, sp.connectionID, err)
		if retry < maxRetries-1 {
			time.Sleep(retryInterval * time.Duration(retry+1))
			// 重新创建请求（使用相同的 RequestId）
			req, _ = sp.converter.WritePacket(pkt, requestID)
			reqURL, _ := url.Parse(sp.pushURL)
			req.URL = reqURL
			if sp.token != "" {
				req.Header.Set("Authorization", "Bearer "+sp.token)
			}
		}
	}

	if err != nil {
		return 0, fmt.Errorf("push request failed: %w", err)
	}
	defer resp.Body.Close()

	// 5. 处理响应（如果有控制包响应，在 X-Tunnel-Package 中）
	if resp.Header.Get("X-Tunnel-Package") != "" {
		// 解码 TunnelPackage 以检查 RequestId
		pkg, err := DecodeTunnelPackage(resp.Header.Get("X-Tunnel-Package"))
		if err == nil {
			// 检查 RequestId 是否匹配
			if pkg.RequestID == requestID {
				// RequestId 匹配，处理响应
				respPkt, _ := sp.converter.ReadPacket(resp)
				// 将响应包放入队列，供后续读取
				if respPkt != nil {
					// 安全地向 packetQueue 写入，使用 recover 捕获可能的 panic（channel 已关闭）
					func() {
						defer func() {
							if r := recover(); r != nil {
								utils.Warnf("HTTPStreamProcessor: WritePacket - panic when writing to packetQueue (likely closed), requestID=%s, connID=%s, error=%v", requestID, sp.connectionID, r)
							}
						}()
						select {
						case sp.packetQueue <- respPkt:
						default:
							// 队列满，丢弃
							utils.Warnf("HTTPStreamProcessor: WritePacket - packetQueue full, dropping response packet, requestID=%s, connID=%s", requestID, sp.connectionID)
						}
					}()
				}
				// 更新连接信息
				if pkg.ConnectionID != "" {
					sp.SetConnectionID(pkg.ConnectionID)
				}
			} else {
				utils.Debugf("HTTPStreamProcessor: WritePacket - RequestId mismatch, expected=%s, got=%s, ignoring response",
					requestID, pkg.RequestID)
			}
		}
	}

	// 6. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("push request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// 读取并丢弃响应 body（确保连接正确关闭）
	// 注意：即使 body 为空，也要读取，否则连接可能不会正确关闭
	if resp.Body != nil {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil && readErr != io.EOF {
			utils.Warnf("HTTPStreamProcessor: WritePacket - failed to read response body: %v, requestID=%s, connID=%s", readErr, requestID, sp.connectionID)
		} else {
			utils.Infof("HTTPStreamProcessor: WritePacket - Push request completed successfully, requestID=%s, connID=%s, bodyLen=%d", requestID, sp.connectionID, len(body))
		}
	}

	// [CMD_TRACE] 记录 Push 请求完成
	writeDuration := time.Since(writeStartTime)
	utils.Infof("[CMD_TRACE] [CLIENT] [PUSH_COMPLETE] RequestID=%s, CommandID=%s, Duration=%v, Time=%s",
		requestID, commandID, writeDuration, time.Now().Format("15:04:05.000"))

	return 0, nil
}

// WriteExact 将数据流写入 HTTP Request Body
func (sp *StreamProcessor) WriteExact(data []byte) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.ErrClosedPipe
	}

	// 获取序列号（客户端也需要序列号，但主要用于日志追踪）
	// 注意：客户端发送数据时，序列号由服务器端分配，这里使用0作为占位符
	// 实际上，客户端发送的分片会在服务器端重新分配序列号
	sequenceNumber := int64(0)

	// 对大数据包进行分片处理（类似服务器端的 WriteExact）
	utils.Infof("HTTPStreamProcessor[%s]: WriteExact - splitting %d bytes, connID=%s", sp.connectionID, len(data), sp.connectionID)
	fragments, err := SplitDataIntoFragments(data, sequenceNumber)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor[%s]: WriteExact - failed to split data into fragments: %v, connID=%s", sp.connectionID, err, sp.connectionID)
		return fmt.Errorf("failed to split data into fragments: %w", err)
	}

	utils.Infof("HTTPStreamProcessor[%s]: WriteExact - split into %d fragments, connID=%s", sp.connectionID, len(fragments), sp.connectionID)

	// 发送每个分片
	for i, fragment := range fragments {
		utils.Infof("HTTPStreamProcessor[%s]: WriteExact - sending fragment %d/%d, groupID=%s, size=%d, originalSize=%d, connID=%s",
			sp.connectionID, i+1, len(fragments), fragment.FragmentGroupID, fragment.FragmentSize, fragment.OriginalSize, sp.connectionID)
		// 序列化分片响应为 JSON
		fragmentJSON, err := MarshalFragmentResponse(fragment)
		if err != nil {
			return fmt.Errorf("failed to marshal fragment: %w", err)
		}

		// 生成 RequestId（用于匹配请求和响应）
		requestID := uuid.New().String()

		// 构建 HTTP Request
		// 数据流传输时，X-Tunnel-Package 只包含连接标识（用于路由）
		dataPkg := &TunnelPackage{
			ConnectionID: sp.connectionID,
			RequestID:    requestID,
			ClientID:     sp.clientID,
			MappingID:    sp.mappingID,
			TunnelType:   "data",
			// Type 为空，表示这是数据流传输
		}
		encodedPkg, err := EncodeTunnelPackage(dataPkg)
		if err != nil {
			return fmt.Errorf("failed to encode data package: %w", err)
		}

		// 将分片 JSON 作为 data 字段发送（服务器端会识别并处理分片）
		reqBody := map[string]interface{}{
			"data":      string(fragmentJSON), // 发送 JSON 字符串，而不是 Base64 编码的原始数据
			"timestamp": time.Now().Unix(),
		}
		reqJSON, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(sp.Ctx(), "POST", sp.pushURL, bytes.NewReader(reqJSON))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Tunnel-Package", encodedPkg)
		if sp.token != "" {
			req.Header.Set("Authorization", "Bearer "+sp.token)
		}

		// 发送请求
		utils.Debugf("HTTPStreamProcessor[%s]: WriteExact - sending fragment %d/%d push request, groupID=%s, requestID=%s, connID=%s",
			sp.connectionID, i+1, len(fragments), fragment.FragmentGroupID, requestID, sp.connectionID)
		resp, err := sp.httpClient.Do(req)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor[%s]: WriteExact - push request failed for fragment %d/%d: %v, groupID=%s, requestID=%s, connID=%s",
				sp.connectionID, i+1, len(fragments), err, fragment.FragmentGroupID, requestID, sp.connectionID)
			return fmt.Errorf("push data request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			utils.Errorf("HTTPStreamProcessor[%s]: WriteExact - push request failed for fragment %d/%d: status %d, body: %s, groupID=%s, requestID=%s, connID=%s",
				sp.connectionID, i+1, len(fragments), resp.StatusCode, string(body), fragment.FragmentGroupID, requestID, sp.connectionID)
			return fmt.Errorf("push data request failed: status %d, body: %s", resp.StatusCode, string(body))
		}
		utils.Debugf("HTTPStreamProcessor[%s]: WriteExact - fragment %d/%d sent successfully, groupID=%s, requestID=%s, connID=%s",
			sp.connectionID, i+1, len(fragments), fragment.FragmentGroupID, requestID, sp.connectionID)
	}

	utils.Infof("HTTPStreamProcessor[%s]: WriteExact - all %d fragments sent successfully, originalSize=%d, connID=%s",
		sp.connectionID, len(fragments), len(data), sp.connectionID)

	return nil
}

// ReadExact 从数据流缓冲读取指定长度
func (sp *StreamProcessor) ReadExact(length int) ([]byte, error) {
	utils.Debugf("HTTPStreamProcessor[%s]: ReadExact - requested %d bytes, current buffer size=%d, connID=%s",
		sp.connectionID, length, sp.dataBuffer.Len(), sp.connectionID)

	sp.dataBufMu.Lock()
	defer sp.dataBufMu.Unlock()

	// 从缓冲读取，如果不够则触发 Poll 请求获取更多数据
	for sp.dataBuffer.Len() < length {
		currentBufferLen := sp.dataBuffer.Len()
		sp.dataBufMu.Unlock()
		utils.Debugf("HTTPStreamProcessor[%s]: ReadExact - buffer has %d bytes, need %d, triggering Poll request, connID=%s",
			sp.connectionID, currentBufferLen, length, sp.connectionID)
		// 触发 Poll 获取更多数据
		_, _, err := sp.ReadPacket()
		if err != nil {
			utils.Errorf("HTTPStreamProcessor[%s]: ReadExact - ReadPacket failed: %v, connID=%s", sp.connectionID, err, sp.connectionID)
			return nil, err
		}
		sp.dataBufMu.Lock()
		utils.Debugf("HTTPStreamProcessor[%s]: ReadExact - after Poll, buffer size=%d, need %d, connID=%s",
			sp.connectionID, sp.dataBuffer.Len(), length, sp.connectionID)
	}

	data := make([]byte, length)
	n, err := sp.dataBuffer.Read(data)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor[%s]: ReadExact - failed to read from buffer: %v, connID=%s", sp.connectionID, err, sp.connectionID)
		return nil, err
	}
	if n < length {
		utils.Errorf("HTTPStreamProcessor[%s]: ReadExact - read %d bytes, expected %d, connID=%s", sp.connectionID, n, length, sp.connectionID)
		return nil, io.ErrUnexpectedEOF
	}

	utils.Debugf("HTTPStreamProcessor[%s]: ReadExact - read %d bytes successfully, remaining buffer size=%d, connID=%s",
		sp.connectionID, n, sp.dataBuffer.Len(), sp.connectionID)
	return data[:n], nil
}

// GetReader 获取底层 Reader（HTTP 无状态，返回 nil）
func (sp *StreamProcessor) GetReader() io.Reader {
	// HTTP 是无状态的，没有底层的 io.Reader
	// 返回 nil，上层代码应该使用 ReadPacket() 和 ReadExact()
	return nil
}

// GetWriter 获取底层 Writer（HTTP 无状态，返回 nil）
func (sp *StreamProcessor) GetWriter() io.Writer {
	// HTTP 是无状态的，没有底层的 io.Writer
	// 返回 nil，上层代码应该使用 WritePacket() 和 WriteExact()
	return nil
}

// Close 关闭连接
func (sp *StreamProcessor) Close() {
	sp.Dispose.CloseWithError()
}

// 确保 StreamProcessor 实现 stream.PackageStreamer 接口
var _ stream.PackageStreamer = (*StreamProcessor)(nil)
