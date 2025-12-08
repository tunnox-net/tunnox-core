package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

func (c *HTTPLongPollingConn) sendData(data []byte) error {
	// 分片数据
	// 注意：客户端发送时，序列号使用0作为占位符，服务器端会重新分配序列号
	fragments, err := httppoll.SplitDataIntoFragments(data, 0)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to split data into fragments")
	}

	utils.Infof("HTTP long polling: sendData splitting %d bytes into %d fragments, connectionID=%s", len(data), len(fragments), c.connectionID)

	// 发送每个分片
	for _, fragment := range fragments {
		if err := c.sendFragment(fragment); err != nil {
			return coreErrors.Wrapf(err, coreErrors.ErrorTypeNetwork, "failed to send fragment %d/%d", fragment.FragmentIndex, fragment.TotalFragments)
		}
		utils.Infof("HTTP long polling: sendData sent fragment %d/%d (size=%d, groupID=%s), connectionID=%s",
			fragment.FragmentIndex, fragment.TotalFragments, fragment.FragmentSize, fragment.FragmentGroupID, c.connectionID)
	}

	return nil
}

// sendFragment 发送单个分片
func (c *HTTPLongPollingConn) sendFragment(fragment *httppoll.FragmentResponse) error {
	// 构造请求（使用分片格式，统一使用 FragmentResponse）
	reqBody := &httppoll.FragmentResponse{
		FragmentGroupID: fragment.FragmentGroupID,
		OriginalSize:    fragment.OriginalSize,
		FragmentSize:    fragment.FragmentSize,
		FragmentIndex:   fragment.FragmentIndex,
		TotalFragments:  fragment.TotalFragments,
		Data:            fragment.Data,
		Timestamp:       time.Now().Unix(),
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to marshal request")
	}

	// 发送 POST 请求
	req, err := http.NewRequestWithContext(c.Ctx(), "POST", c.pushURL, bytes.NewReader(reqJSON))
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to create request")
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// 构造 TunnelPackage（包含连接信息）
	tunnelPkg := &httppoll.TunnelPackage{
		ConnectionID: c.connectionID,
		ClientID:     c.clientID,
		MappingID:    c.mappingID,
		TunnelType:   c.connType,
	}

	// 编码并设置 X-Tunnel-Package header
	encodedPkg, err := httppoll.EncodeTunnelPackage(tunnelPkg)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to encode tunnel package")
	}
	req.Header.Set("X-Tunnel-Package", encodedPkg)

	utils.Infof("HTTP long polling: sending push request (fragment %d/%d), connectionID=%s, clientID=%d, mappingID=%s, fragmentSize=%d, url=%s",
		fragment.FragmentIndex, fragment.TotalFragments, c.connectionID, c.clientID, c.mappingID, fragment.FragmentSize, c.pushURL)

	var resp *http.Response
	var retryCount int
	for retryCount < httppollMaxRetries {
		resp, err = c.pushClient.Do(req)
		if err == nil {
			break
		}

		retryCount++
		if retryCount < httppollMaxRetries {
			time.Sleep(httppollRetryInterval * time.Duration(retryCount))
			// 重新创建请求（Body 已被读取）
			req, _ = http.NewRequestWithContext(c.Ctx(), "POST", c.pushURL, bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			if c.token != "" {
				req.Header.Set("Authorization", "Bearer "+c.token)
			}
			// 重新编码 TunnelPackage
			tunnelPkg := &httppoll.TunnelPackage{
				ConnectionID: c.connectionID,
				ClientID:     c.clientID,
				MappingID:    c.mappingID,
				TunnelType:   c.connType,
			}
			if encodedPkg, err := httppoll.EncodeTunnelPackage(tunnelPkg); err == nil {
				req.Header.Set("X-Tunnel-Package", encodedPkg)
			}
		}
	}

	if err != nil {
		utils.Errorf("HTTP long polling: push request failed after %d retries: %v", retryCount, err)
		return coreErrors.Wrapf(err, coreErrors.ErrorTypeNetwork, "push request failed after %d retries", retryCount)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		utils.Errorf("HTTP long polling: push request failed: status %d, body: %s", resp.StatusCode, string(body))
		return coreErrors.Newf(coreErrors.ErrorTypeNetwork, "push request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	utils.Infof("HTTP long polling: push request succeeded, status=%d", resp.StatusCode)

	return nil
}
