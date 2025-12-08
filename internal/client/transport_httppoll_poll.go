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

		// 处理数据：使用统一的分片处理接口
		if pollResp.Data != "" {
			utils.Debugf("HTTP long polling: received fragment response, groupID=%s, index=%d/%d, size=%d, mappingID=%s",
				pollResp.FragmentGroupID, pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentSize, c.mappingID)

			// 使用统一的分片处理器（按序列号顺序处理）
			isComplete, reassembledData, err := httppoll.ProcessFragmentFromResponse(c.fragmentProcessor, pollResp)
			if err != nil {
				utils.Errorf("HTTP long polling: failed to process fragment: %v, groupID=%s, mappingID=%s",
					err, pollResp.FragmentGroupID, c.mappingID)
				time.Sleep(httppollRetryInterval)
				continue
			}

			// 如果分片组完整且可以立即返回（单分片情况），直接发送
			if isComplete && reassembledData != nil {
				base64Data := base64.StdEncoding.EncodeToString(reassembledData)
				utils.Infof("HTTP long polling: processed fragment immediately, groupID=%s, size=%d, mappingID=%s",
					pollResp.FragmentGroupID, len(reassembledData), c.mappingID)
				select {
				case <-c.Ctx().Done():
					return
				case c.base64DataChan <- base64Data:
					utils.Debugf("HTTP long polling: sent processed fragment to base64DataChan, size=%d, mappingID=%s",
						len(base64Data), c.mappingID)
				default:
					utils.Warnf("HTTP long polling: base64DataChan full, dropping processed fragment, size=%d, mappingID=%s",
						len(base64Data), c.mappingID)
				}
			}
		}

		// 无论是否收到新分片，都尝试处理已完整的分片组（避免分片组积压）
		// 这很重要：即使没有收到新分片，也要处理之前已完整但未处理的分片组
		for {
			reassembledData, found, err := c.fragmentProcessor.GetNextReassembledData()
			if err != nil {
				// 连接断开错误，关闭连接
				utils.Errorf("HTTP long polling: connection broken: %v, mappingID=%s", err, c.mappingID)
				c.Close() // 关闭连接，向上层报告
				return
			}
			if !found {
				// 没有更多按序列号顺序的完整分片组
				break
			}

			// Base64编码重组后的数据
			base64Data := base64.StdEncoding.EncodeToString(reassembledData)
			utils.Infof("HTTP long polling: sending reassembled data (sequence ordered), size=%d, base64Len=%d, mappingID=%s",
				len(reassembledData), len(base64Data), c.mappingID)

			// 发送到 base64DataChan
			select {
			case <-c.Ctx().Done():
				return
			case c.base64DataChan <- base64Data:
				utils.Debugf("HTTP long polling: sent reassembled data to base64DataChan, size=%d, mappingID=%s",
					len(base64Data), c.mappingID)
			default:
				// 如果 channel 满了，使用带超时的发送，避免无限阻塞
				// 但继续处理下一个分片组，而不是直接返回（避免分片组积压）
				utils.Warnf("HTTP long polling: base64DataChan full, waiting to send, size=%d, mappingID=%s",
					len(base64Data), c.mappingID)
				select {
				case <-c.Ctx().Done():
					return
				case c.base64DataChan <- base64Data:
					utils.Debugf("HTTP long polling: sent reassembled data to base64DataChan after wait, size=%d, mappingID=%s",
						len(base64Data), c.mappingID)
				case <-time.After(5 * time.Second):
					// 超时，跳过这个数据包，继续处理下一个（避免死锁）
					utils.Errorf("HTTP long polling: timeout sending to base64DataChan, dropping data, size=%d, mappingID=%s",
						len(base64Data), c.mappingID)
					// 继续处理下一个分片组，不返回
				}
			}
		}

		if pollResp.Timeout {
			utils.Debugf("HTTP long polling: poll request timeout, retrying...")
		}

		// 继续循环，立即发起下一个请求（无论是否超时）
		continue
	}
}
