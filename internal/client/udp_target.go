package client

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/stream/transform"
	"tunnox-core/internal/utils"
)

// HandleUDPTarget 处理 UDP 目标端隧道
func HandleUDPTarget(client *TunnoxClient, tunnelID, mappingID, secretKey, targetHost string, targetPort int, transformConfig *transform.TransformConfig) {
	utils.Infof("Client: handling UDP target tunnel, tunnel_id=%s, target=%s:%d", tunnelID, targetHost, targetPort)

	// 1. 解析目标 UDP 地址
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		utils.Errorf("Client: failed to resolve UDP address %s: %v", targetAddr, err)
		return
	}

	// 2. 创建 UDP 连接到目标
	targetConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		utils.Errorf("Client: failed to connect to UDP target %s: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	utils.Infof("Client: connected to UDP target %s for tunnel %s", targetAddr, tunnelID)

	// 3. 建立隧道连接
	tunnelConn, tunnelStream, err := client.DialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		utils.Errorf("Client: failed to dial tunnel: %v", err)
		return
	}
	defer tunnelConn.Close()

	utils.Infof("Client: UDP tunnel %s established successfully", tunnelID)

	// 4. 关闭 StreamProcessor，切换到裸连接模式
	tunnelStream.Close()

	// 5. 启动 UDP 双向转发
	bidirectionalCopyUDPTarget(tunnelConn, targetConn, tunnelID, transformConfig)
}

// bidirectionalCopyUDPTarget UDP 双向转发（作为目标端）
func bidirectionalCopyUDPTarget(tunnelConn net.Conn, targetConn *net.UDPConn, tunnelID string, transformConfig *transform.TransformConfig) {
	defer tunnelConn.Close()
	defer targetConn.Close()

	// 创建转换器
	transformer, err := transform.NewTransformer(transformConfig)
	if err != nil {
		utils.Errorf("Client: failed to create transformer: %v", err)
		return
	}

	// 包装读写器
	reader := io.Reader(tunnelConn)
	writer := io.Writer(tunnelConn)
	if transformer != nil {
		reader, _ = transformer.WrapReader(reader)
		writer, _ = transformer.WrapWriter(writer)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// 隧道 → 目标 UDP
	go func() {
		defer wg.Done()
		defer targetConn.Close()

		for {
			// 读取长度
			lengthBuf := make([]byte, 2)
			if _, err := io.ReadFull(reader, lengthBuf); err != nil {
				if err != io.EOF {
					utils.Debugf("Client: UDP tunnel read error: %v", err)
				}
				return
			}

			length := binary.BigEndian.Uint16(lengthBuf)
			if length == 0 || length > 65535 {
				utils.Errorf("Client: invalid UDP packet length: %d", length)
				return
			}

			// 读取数据
			data := make([]byte, length)
			if _, err := io.ReadFull(reader, data); err != nil {
				utils.Errorf("Client: failed to read UDP data: %v", err)
				return
			}

			// 发送到目标 UDP
			if _, err := targetConn.Write(data); err != nil {
				utils.Errorf("Client: failed to write to UDP target: %v", err)
				return
			}
		}
	}()

	// 目标 UDP → 隧道
	go func() {
		defer wg.Done()
		defer tunnelConn.Close()

		buffer := make([]byte, 65535)
		for {
			// 设置读取超时
			targetConn.SetReadDeadline(time.Now().Add(30 * time.Second))

			n, err := targetConn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err != io.EOF {
					utils.Debugf("Client: UDP target read error: %v", err)
				}
				return
			}

			if n == 0 {
				continue
			}

			// 写入长度
			lengthBuf := make([]byte, 2)
			binary.BigEndian.PutUint16(lengthBuf, uint16(n))
			if _, err := writer.Write(lengthBuf); err != nil {
				utils.Errorf("Client: failed to write length: %v", err)
				return
			}

			// 写入数据
			if _, err := writer.Write(buffer[:n]); err != nil {
				utils.Errorf("Client: failed to write data: %v", err)
				return
			}
		}
	}()

	wg.Wait()
	utils.Infof("Client: UDP tunnel %s closed", tunnelID)
}

