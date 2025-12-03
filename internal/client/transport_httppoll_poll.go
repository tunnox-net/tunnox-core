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
				// 添加到分片重组器
				group, err := c.fragmentReassembler.AddFragment(
					pollResp.FragmentGroupID,
					pollResp.OriginalSize,
					pollResp.FragmentSize,
					pollResp.FragmentIndex,
					pollResp.TotalFragments,
					fragmentData,
				)
				if err != nil {
					utils.Errorf("HTTP long polling: failed to add fragment: %v", err)
					time.Sleep(httppollRetryInterval)
					continue
				}

				// 检查是否完整
				if group.IsComplete() {
					// 重组数据
					reassembledData, err := group.Reassemble()
					if err != nil {
						utils.Errorf("HTTP long polling: failed to reassemble fragments: %v", err)
						c.fragmentReassembler.RemoveGroup(pollResp.FragmentGroupID)
						time.Sleep(httppollRetryInterval)
						continue
					}

					// Base64编码重组后的数据
					base64Data := base64.StdEncoding.EncodeToString(reassembledData)
					utils.Debugf("HTTP long polling: reassembled %d bytes from %d fragments, groupID=%s, mappingID=%s",
						len(reassembledData), pollResp.TotalFragments, pollResp.FragmentGroupID, c.mappingID)

					// 发送到 base64DataChan
					select {
					case <-c.Ctx().Done():
						return
					case c.base64DataChan <- base64Data:
					default:
						utils.Warnf("HTTP long polling: base64DataChan full, dropping reassembled data")
					}

					// 移除分片组
					c.fragmentReassembler.RemoveGroup(pollResp.FragmentGroupID)
				} else {
					utils.Debugf("HTTP long polling: fragment %d/%d received, waiting for more fragments, groupID=%s",
						pollResp.FragmentIndex, pollResp.TotalFragments, pollResp.FragmentGroupID)
				}
			} else {
				// 完整数据，直接发送到 base64DataChan
				base64Data := pollResp.Data // 已经是Base64编码
				select {
				case <-c.Ctx().Done():
					return
				case c.base64DataChan <- base64Data:
				default:
					utils.Warnf("HTTP long polling: base64DataChan full, dropping data")
				}
			}
		} else if pollResp.Timeout {
			utils.Debugf("HTTP long polling: poll request timeout, retrying...")
		}

		// 继续循环，立即发起下一个请求（无论是否超时）
		continue
	}
}
