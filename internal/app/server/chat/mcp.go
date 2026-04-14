package chat

import (
	"context"
	"encoding/json"

	. "xiaozhi-esp32-server-golang/internal/data/client"
	"xiaozhi-esp32-server-golang/internal/domain/mcp"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/spf13/viper"
)

type McpTransport struct {
	Client          *ClientState
	ServerTransport *ServerTransport
}

func (c *McpTransport) SendMcpMsg(payload []byte) error {
	//如果是initialize请求，则注入vision
	var request transport.JSONRPCRequest
	err := json.Unmarshal(payload, &request)
	if err == nil {
		if request.Method == "initialize" {
			if origInitParams, ok := request.Params.(map[string]interface{}); ok {
				b, err := json.Marshal(origInitParams)
				if err != nil {
					return err
				}

				var initParams mcp.InitializeParams
				err = json.Unmarshal(b, &initParams)
				if err != nil {
					return err
				}
				initParams.Capabilities["vision"] = mcp.Vision{
					Url:   viper.GetString("vision.vision_url"),
					Token: "1234567890",
				}
				request.Params = initParams
			}
			payload, _ = json.Marshal(request)
		}
	}

	return c.ServerTransport.SendMcpMsg(payload)
}

func (c *McpTransport) RecvMcpMsg(ctx context.Context, timeOut int) ([]byte, error) {
	return c.ServerTransport.RecvMcpMsg(ctx, timeOut)
}

func (c *McpTransport) GetMcpTransportType() string {
	if c == nil || c.ServerTransport == nil {
		return ""
	}
	return c.ServerTransport.GetTransportType()
}

func initMcp(deviceID string, mcpTransport *McpTransport) {
	if err := mcp.EnsureDeviceIotOverMcp(deviceID, mcpTransport); err != nil {
		log.Errorf("确保IotOverMcp客户端失败: %v", err)
		if mcpTransport != nil && mcpTransport.ServerTransport != nil {
			mcpTransport.ServerTransport.transport.Close()
		}
		return
	}
}
