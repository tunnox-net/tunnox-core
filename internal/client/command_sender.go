package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

// CommandRequest 命令请求参数
type CommandRequest struct {
	CommandType packet.CommandType
	RequestBody interface{}
	EnableTrace bool
}

// CommandResponseData 命令响应数据（避免与command_response_manager中的CommandResponse冲突）
type CommandResponseData struct {
	Success bool
	Data    string
	Error   string
}

// sendCommandAndWaitResponse 发送命令并等待响应（统一处理所有命令发送逻辑）
func (c *TunnoxClient) sendCommandAndWaitResponse(req *CommandRequest) (*CommandResponseData, error) {
	return c.sendCommandAndWaitResponseWithContext(context.Background(), req)
}

// sendCommandAndWaitResponseWithContext 发送命令并等待响应（支持context取消）
func (c *TunnoxClient) sendCommandAndWaitResponseWithContext(ctx context.Context, req *CommandRequest) (*CommandResponseData, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection not established, please connect to server first")
	}

	// 序列化请求
	var reqBody []byte
	var err error
	if req.RequestBody != nil {
		reqBody, err = json.Marshal(req.RequestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
	} else {
		reqBody = []byte("{}")
	}

	// 创建命令包
	cmdID, err := utils.GenerateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate command ID: %w", err)
	}

	cmdPkt := &packet.CommandPacket{
		CommandType: req.CommandType,
		CommandId:   cmdID,
		CommandBody: string(reqBody),
	}

	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	// 注册请求
	responseChan := c.commandResponseManager.RegisterRequest(cmdPkt.CommandId)
	defer c.commandResponseManager.UnregisterRequest(cmdPkt.CommandId)

	// 发送命令前再次检查连接状态
	if !c.IsConnected() {
		return nil, fmt.Errorf("control connection is closed, please reconnect to server")
	}

	// 获取控制流
	c.mu.RLock()
	controlStream := c.controlStream
	c.mu.RUnlock()

	if controlStream == nil {
		return nil, fmt.Errorf("control stream is nil")
	}

	// 发送命令
	var cmdStartTime time.Time
	if req.EnableTrace {
		cmdStartTime = time.Now()
		corelog.Infof("[CMD_TRACE] [CLIENT] [SEND_START] CommandID=%s, CommandType=%d, Time=%s",
			cmdPkt.CommandId, cmdPkt.CommandType, cmdStartTime.Format("15:04:05.000"))
	}

	_, err = controlStream.WritePacket(transferPkt, true, 0)
	if err != nil {
		if req.EnableTrace {
			corelog.Errorf("[CMD_TRACE] [CLIENT] [SEND_FAILED] CommandID=%s, Error=%v, Time=%s",
				cmdPkt.CommandId, err, time.Now().Format("15:04:05.000"))
		}

		// 发送失败，清理连接状态
		c.cleanupControlConnection()

		// 检查是否是流已关闭的错误
		errMsg := err.Error()
		if strings.Contains(errMsg, "stream is closed") ||
			strings.Contains(errMsg, "stream closed") ||
			strings.Contains(errMsg, "ErrStreamClosed") {
			return nil, fmt.Errorf("control connection is closed, please reconnect to server")
		}
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	if req.EnableTrace {
		corelog.Infof("[CMD_TRACE] [CLIENT] [SEND_COMPLETE] CommandID=%s, SendDuration=%v, Time=%s",
			cmdPkt.CommandId, time.Since(cmdStartTime), time.Now().Format("15:04:05.000"))
	}

	// 优化：发送命令后立即触发 Poll 请求，以快速获取响应
	if httppollStream, ok := controlStream.(*httppoll.StreamProcessor); ok {
		triggerTime := time.Now()
		pollRequestID := httppollStream.TriggerImmediatePoll()
		if req.EnableTrace {
			corelog.Infof("[CMD_TRACE] [CLIENT] [TRIGGER_POLL] CommandID=%s, PollRequestID=%s, Time=%s",
				cmdPkt.CommandId, pollRequestID, triggerTime.Format("15:04:05.000"))
		}
	}

	// 等待响应（支持context取消）
	var waitStartTime time.Time
	if req.EnableTrace {
		waitStartTime = time.Now()
		corelog.Infof("[CMD_TRACE] [CLIENT] [WAIT_START] CommandID=%s, Time=%s",
			cmdPkt.CommandId, waitStartTime.Format("15:04:05.000"))
	}

	cmdResp, err := c.commandResponseManager.WaitForResponseWithContext(ctx, cmdPkt.CommandId, responseChan)
	if err != nil {
		if req.EnableTrace {
			corelog.Errorf("[CMD_TRACE] [CLIENT] [WAIT_FAILED] CommandID=%s, WaitDuration=%v, Error=%v, Time=%s",
				cmdPkt.CommandId, time.Since(waitStartTime), err, time.Now().Format("15:04:05.000"))
		}
		return nil, err
	}

	if req.EnableTrace {
		corelog.Infof("[CMD_TRACE] [CLIENT] [WAIT_COMPLETE] CommandID=%s, WaitDuration=%v, TotalDuration=%v, Time=%s",
			cmdPkt.CommandId, time.Since(waitStartTime), time.Since(cmdStartTime), time.Now().Format("15:04:05.000"))
	}

	if !cmdResp.Success {
		return nil, fmt.Errorf("command failed: %s", cmdResp.Error)
	}

	return &CommandResponseData{
		Success: cmdResp.Success,
		Data:    cmdResp.Data,
		Error:   cmdResp.Error,
	}, nil
}

// cleanupControlConnection 清理控制连接状态
func (c *TunnoxClient) cleanupControlConnection() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.controlStream != nil {
		c.controlStream.Close()
		c.controlStream = nil
	}
	if c.controlConn != nil {
		c.controlConn.Close()
		c.controlConn = nil
	}
}
