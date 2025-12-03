package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

func (c *HTTPLongPollingConn) pollLoop() {
	utils.Debugf("HTTP long polling: pollLoop started, clientID=%d, pollURL=%s", c.clientID, c.pollURL)
	defer utils.Debugf("HTTP long polling: pollLoop exiting, clientID=%d", c.clientID)

	// 检查 context 是否已取消
	if c.Ctx().Err() != nil {
		utils.Debugf("HTTP long polling: pollLoop context already cancelled: %v", c.Ctx().Err())
		return
	}

	for {
		select {
		case <-c.Ctx().Done():
			utils.Debugf("HTTP long polling: pollLoop exiting due to context cancellation: %v", c.Ctx().Err())
			return
		default:
		}

		// 构造 GET 请求
		u, err := url.Parse(c.pollURL)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to parse poll URL: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}

		q := u.Query()
		q.Set("timeout", strconv.Itoa(int(httppollDefaultPollTimeout.Seconds())))
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(c.Ctx(), "GET", u.String(), nil)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to create poll request: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}

		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		// 构造 TunnelPackage（包含连接信息和 requestID）
		// 生成 requestID（用于匹配请求和响应）
		// 注意：这种 Poll 请求主要用于维持连接（keepalive），而不是实际的数据传输
		requestID := generateRequestID()
		tunnelPkg := &httppoll.TunnelPackage{
			ConnectionID: c.connectionID,
			RequestID:    requestID,
			ClientID:     c.clientID,
			MappingID:    c.mappingID,
			TunnelType:   "keepalive", // 标记为 keepalive 类型，用于维持连接
		}

		// 编码并设置 X-Tunnel-Package header
		encodedPkg, err := httppoll.EncodeTunnelPackage(tunnelPkg)
		if err != nil {
			utils.Errorf("HTTP long polling: failed to encode tunnel package: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}
		req.Header.Set("X-Tunnel-Package", encodedPkg)

		utils.Debugf("HTTP long polling: sending poll request, clientID=%d, mappingID=%s, url=%s", c.clientID, c.mappingID, u.String())
		// 发送长轮询请求
		resp, err := c.pollClient.Do(req)
		if err != nil {
			// 如果是 context 取消，直接退出
			if err == context.Canceled || c.Ctx().Err() != nil {
				utils.Debugf("HTTP long polling: poll request cancelled, exiting")
				return
			}
			utils.Debugf("HTTP long polling: poll request failed: %v, retrying...", err)
			time.Sleep(httppollRetryInterval)
			continue
		}

		utils.Debugf("HTTP long polling: poll request succeeded, status=%d", resp.StatusCode)

		// 注意：keepalive 请求仅用于维持连接，不应该包含指令
		// 指令应该通过 HTTPStreamProcessor 的 Poll 请求（TunnelType="control" 或 "data"）接收
		// 如果服务端在 keepalive 响应中返回了控制包，这可能是设计问题，应该忽略
		xTunnelPackage := resp.Header.Get("X-Tunnel-Package")
		if xTunnelPackage != "" {
			utils.Warnf("HTTP long polling: received X-Tunnel-Package in keepalive response (unexpected), len=%d, requestID=%s. Keepalive requests should not contain control packets.", len(xTunnelPackage), requestID)
			// 不处理，因为 keepalive 请求不应该包含指令
		}

		// 解析响应（分片格式）
		var pollResp httppoll.FragmentResponse
		if err := json.NewDecoder(resp.Body).Decode(&pollResp); err != nil {
			resp.Body.Close()
			utils.Errorf("HTTP long polling: failed to decode poll response: %v", err)
			time.Sleep(httppollRetryInterval)
			continue
		}
		resp.Body.Close()

		// 处理数据：如果是分片，需要重组；如果是完整数据，直接处理
		if pollResp.Data != "" {
			// 判断是否为分片：total_fragments > 1
			isFragment := pollResp.TotalFragments > 1
			utils.Infof("HTTP long polling: received fragment response, groupID=%s, index=%d/%d, size=%d, isFragment=%v, mappingID=%s",
				pollResp.FragmentGroupID, pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentSize, isFragment, c.mappingID)

			// 解码Base64数据
			previewLen := 50
			if len(pollResp.Data) < previewLen {
				previewLen = len(pollResp.Data)
			}
			utils.Debugf("HTTP long polling: decoding fragment data, Data field len=%d, mappingID=%s",
				len(pollResp.Data), c.mappingID)
			fragmentData, err := base64.StdEncoding.DecodeString(pollResp.Data)
			if err != nil {
				previewLen2 := 100
				if len(pollResp.Data) < previewLen2 {
					previewLen2 = len(pollResp.Data)
				}
				utils.Errorf("HTTP long polling: failed to decode fragment data: %v, Data preview=%s", err, pollResp.Data[:previewLen2])
				time.Sleep(httppollRetryInterval)
				continue
			}
			utils.Debugf("HTTP long polling: decoded fragment data, len=%d, mappingID=%s",
				len(fragmentData), c.mappingID)

			// 如果是分片，需要重组
			if isFragment {
				utils.Infof("HTTP long polling: adding fragment to reassembler, groupID=%s, index=%d/%d, size=%d, originalSize=%d, mappingID=%s",
					pollResp.FragmentGroupID, pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentSize, pollResp.OriginalSize, c.mappingID)
				// 添加到分片重组器
				group, err := c.fragmentReassembler.AddFragment(
					pollResp.FragmentGroupID,
					pollResp.OriginalSize,
					pollResp.FragmentSize,
					pollResp.FragmentIndex,
					pollResp.TotalFragments,
					pollResp.SequenceNumber,
					fragmentData,
				)
				if err != nil {
					utils.Errorf("HTTP long polling: failed to add fragment: %v, groupID=%s, index=%d, mappingID=%s",
						err, pollResp.FragmentGroupID, pollResp.FragmentIndex, c.mappingID)
					time.Sleep(httppollRetryInterval)
					continue
				}

				// 使用原子操作检查是否完整（避免竞态条件）
				// 注意：不在这里重组，而是通过 GetNextCompleteGroup 按序列号顺序重组
				utils.Debugf("HTTP long polling: checking if fragment group complete, groupID=%s, receivedCount=%d/%d, mappingID=%s",
					pollResp.FragmentGroupID, group.ReceivedCount, pollResp.TotalFragments, c.mappingID)
				isComplete := group.IsComplete()
				if !isComplete {
					// 分片组不完整，继续等待更多分片
					utils.Infof("HTTP long polling: fragment %d/%d received, waiting for more fragments, groupID=%s, receivedCount=%d, mappingID=%s",
						pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentGroupID, group.ReceivedCount, c.mappingID)
					continue
				}

				// 分片组完整，检查是否可以按序列号顺序发送
				// 使用 GetNextCompleteGroup 确保按序列号顺序发送
				utils.Infof("HTTP long polling: fragment group complete, checking sequence order, groupID=%s, sequenceNumber=%d, mappingID=%s",
					pollResp.FragmentGroupID, pollResp.SequenceNumber, c.mappingID)
				
				// 尝试获取下一个按序列号顺序的完整分片组
				nextGroup, found, err := c.fragmentReassembler.GetNextCompleteGroup()
				if err != nil {
					utils.Errorf("HTTP long polling: failed to get next complete group: %v, groupID=%s, mappingID=%s",
						err, pollResp.FragmentGroupID, c.mappingID)
					c.fragmentReassembler.RemoveGroup(pollResp.FragmentGroupID)
					time.Sleep(httppollRetryInterval)
					continue
				}

				if found {
					utils.Infof("HTTP long polling: GetNextCompleteGroup found next group, groupID=%s, sequenceNumber=%d, mappingID=%s",
						nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
					// 这是下一个应该发送的分片组，重组并发送
					reassembledData, err := nextGroup.Reassemble()
					if err != nil {
						utils.Errorf("HTTP long polling: failed to reassemble: %v, groupID=%s, mappingID=%s",
							err, nextGroup.GroupID, c.mappingID)
						c.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
						time.Sleep(httppollRetryInterval)
						continue
					}

						// Base64编码重组后的数据
						base64Data := base64.StdEncoding.EncodeToString(reassembledData)
						utils.Infof("HTTP long polling: reassembled %d bytes from %d fragments, groupID=%s, sequenceNumber=%d, originalSize=%d, base64Len=%d, mappingID=%s",
							len(reassembledData), nextGroup.TotalFragments, nextGroup.GroupID, nextGroup.SequenceNumber, nextGroup.OriginalSize, len(base64Data), c.mappingID)

						// 验证重组后的数据大小
						if len(reassembledData) != nextGroup.OriginalSize {
							utils.Errorf("HTTP long polling: reassembled size mismatch: expected %d, got %d, groupID=%s, mappingID=%s",
								nextGroup.OriginalSize, len(reassembledData), nextGroup.GroupID, c.mappingID)
							c.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
							time.Sleep(httppollRetryInterval)
							continue
						}

						// 发送到 base64DataChan（确保只有完整的数据才会被发送）
						utils.Debugf("HTTP long polling: sending reassembled data to base64DataChan, size=%d, groupID=%s, sequenceNumber=%d, mappingID=%s",
							len(base64Data), nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
						select {
						case <-c.Ctx().Done():
							return
						case c.base64DataChan <- base64Data:
							utils.Infof("HTTP long polling: sent reassembled data to base64DataChan successfully, size=%d, groupID=%s, sequenceNumber=%d, mappingID=%s",
								len(base64Data), nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
							// 成功发送重组后的完整数据
						default:
							utils.Warnf("HTTP long polling: base64DataChan full, dropping reassembled data, size=%d, groupID=%s, sequenceNumber=%d, mappingID=%s",
								len(base64Data), nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
						}

						// 移除分片组
						c.fragmentReassembler.RemoveGroup(nextGroup.GroupID)

						// 继续检查是否有更多按序列号顺序的完整分片组
						for {
							nextGroup2, found2, err2 := c.fragmentReassembler.GetNextCompleteGroup()
							if err2 != nil || !found2 {
								break
							}
							reassembledData2, err2 := nextGroup2.Reassemble()
							if err2 != nil {
								utils.Errorf("HTTP long polling: failed to reassemble next group: %v, groupID=%s, mappingID=%s",
									err2, nextGroup2.GroupID, c.mappingID)
								c.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
								break
							}
							base64Data2 := base64.StdEncoding.EncodeToString(reassembledData2)
							utils.Infof("HTTP long polling: sending next complete group, groupID=%s, sequenceNumber=%d, size=%d, mappingID=%s",
								nextGroup2.GroupID, nextGroup2.SequenceNumber, len(reassembledData2), c.mappingID)
							select {
							case <-c.Ctx().Done():
								return
							case c.base64DataChan <- base64Data2:
								utils.Infof("HTTP long polling: sent next group to base64DataChan, size=%d, sequenceNumber=%d, mappingID=%s",
									len(base64Data2), nextGroup2.SequenceNumber, c.mappingID)
							default:
								utils.Warnf("HTTP long polling: base64DataChan full, dropping next group, size=%d, sequenceNumber=%d, mappingID=%s",
									len(base64Data2), nextGroup2.SequenceNumber, c.mappingID)
								break
							}
							c.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
						}
				} else {
					// 这不是下一个应该发送的分片组，等待序列号更小的分片组完成
					utils.Infof("HTTP long polling: fragment group complete but GetNextCompleteGroup returned not found, groupID=%s, sequenceNumber=%d, waiting for expected sequence, mappingID=%s",
						pollResp.FragmentGroupID, pollResp.SequenceNumber, c.mappingID)
				}
			} else {
				// 单分片数据（TotalFragments=1），也需要按序列号顺序发送
				// 添加到分片重组器，以便按序列号顺序处理
				utils.Infof("HTTP long polling: received single fragment (TotalFragments=1), adding to reassembler for sequence ordering, groupID=%s, sequenceNumber=%d, mappingID=%s",
					pollResp.FragmentGroupID, pollResp.SequenceNumber, c.mappingID)
				_, err := c.fragmentReassembler.AddFragment(
					pollResp.FragmentGroupID,
					pollResp.OriginalSize,
					pollResp.FragmentSize,
					pollResp.FragmentIndex,
					pollResp.TotalFragments,
					pollResp.SequenceNumber,
					fragmentData,
				)
				if err != nil {
					utils.Errorf("HTTP long polling: failed to add single fragment: %v, groupID=%s, mappingID=%s",
						err, pollResp.FragmentGroupID, c.mappingID)
					time.Sleep(httppollRetryInterval)
					continue
				}

				// 单分片数据应该立即完整，检查是否可以按序列号顺序发送
				utils.Infof("HTTP long polling: single fragment complete, checking sequence order, groupID=%s, sequenceNumber=%d, mappingID=%s",
					pollResp.FragmentGroupID, pollResp.SequenceNumber, c.mappingID)
				
				// 尝试获取下一个按序列号顺序的完整分片组
				nextGroup, found, err := c.fragmentReassembler.GetNextCompleteGroup()
				if err != nil {
					utils.Errorf("HTTP long polling: failed to get next complete group for single fragment: %v, groupID=%s, mappingID=%s",
						err, pollResp.FragmentGroupID, c.mappingID)
					c.fragmentReassembler.RemoveGroup(pollResp.FragmentGroupID)
					time.Sleep(httppollRetryInterval)
					continue
				}

				if found {
					utils.Infof("HTTP long polling: GetNextCompleteGroup found next group for single fragment, groupID=%s, sequenceNumber=%d, mappingID=%s",
						nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
					// 这是下一个应该发送的分片组，重组并发送
					reassembledData, err := nextGroup.Reassemble()
					if err != nil {
						utils.Errorf("HTTP long polling: failed to reassemble single fragment: %v, groupID=%s, mappingID=%s",
							err, nextGroup.GroupID, c.mappingID)
						c.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
						time.Sleep(httppollRetryInterval)
						continue
					}

					// Base64编码重组后的数据
					base64Data := base64.StdEncoding.EncodeToString(reassembledData)
					utils.Infof("HTTP long polling: reassembled single fragment, groupID=%s, sequenceNumber=%d, originalSize=%d, base64Len=%d, mappingID=%s",
						nextGroup.GroupID, nextGroup.SequenceNumber, nextGroup.OriginalSize, len(base64Data), c.mappingID)

					// 发送到 base64DataChan
					select {
					case <-c.Ctx().Done():
						return
					case c.base64DataChan <- base64Data:
						utils.Infof("HTTP long polling: sent single fragment to base64DataChan successfully, size=%d, groupID=%s, sequenceNumber=%d, mappingID=%s",
							len(base64Data), nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
					default:
						utils.Warnf("HTTP long polling: base64DataChan full, dropping single fragment, size=%d, groupID=%s, sequenceNumber=%d, mappingID=%s",
							len(base64Data), nextGroup.GroupID, nextGroup.SequenceNumber, c.mappingID)
					}

					// 移除分片组
					c.fragmentReassembler.RemoveGroup(nextGroup.GroupID)

					// 继续检查是否有更多按序列号顺序的完整分片组
					for {
						nextGroup2, found2, err2 := c.fragmentReassembler.GetNextCompleteGroup()
						if err2 != nil || !found2 {
							break
						}
						reassembledData2, err2 := nextGroup2.Reassemble()
						if err2 != nil {
							utils.Errorf("HTTP long polling: failed to reassemble next group: %v, groupID=%s, mappingID=%s",
								err2, nextGroup2.GroupID, c.mappingID)
							c.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
							break
						}
						base64Data2 := base64.StdEncoding.EncodeToString(reassembledData2)
						utils.Infof("HTTP long polling: sending next complete group, groupID=%s, sequenceNumber=%d, size=%d, mappingID=%s",
							nextGroup2.GroupID, nextGroup2.SequenceNumber, len(reassembledData2), c.mappingID)
				select {
				case <-c.Ctx().Done():
					return
						case c.base64DataChan <- base64Data2:
							utils.Infof("HTTP long polling: sent next group to base64DataChan, size=%d, sequenceNumber=%d, mappingID=%s",
								len(base64Data2), nextGroup2.SequenceNumber, c.mappingID)
				default:
							utils.Warnf("HTTP long polling: base64DataChan full, dropping next group, size=%d, sequenceNumber=%d, mappingID=%s",
								len(base64Data2), nextGroup2.SequenceNumber, c.mappingID)
							break
						}
						c.fragmentReassembler.RemoveGroup(nextGroup2.GroupID)
					}
				} else {
					// 这不是下一个应该发送的分片组，等待序列号更小的分片组完成
					utils.Infof("HTTP long polling: single fragment complete but GetNextCompleteGroup returned not found, groupID=%s, sequenceNumber=%d, waiting for expected sequence, mappingID=%s",
						pollResp.FragmentGroupID, pollResp.SequenceNumber, c.mappingID)
				}
			}
		} else if pollResp.Timeout {
			utils.Debugf("HTTP long polling: poll request timeout, retrying...")
		}

		// 继续循环，立即发起下一个请求（无论是否超时）
		continue
	}
}
