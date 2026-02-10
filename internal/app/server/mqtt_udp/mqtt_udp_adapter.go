package mqtt_udp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"xiaozhi-esp32-server-golang/internal/app/server/types"
	"xiaozhi-esp32-server-golang/internal/data/client"
	. "xiaozhi-esp32-server-golang/internal/data/client"
	. "xiaozhi-esp32-server-golang/logger"
	log "xiaozhi-esp32-server-golang/logger"
)

type MqttConfig struct {
	Broker   string
	Type     string
	Port     int
	ClientID string
	Username string
	Password string
}

// MqttUdpAdapter MQTT-UDP适配器结构
type MqttUdpAdapter struct {
	client          mqtt.Client
	udpServer       *UdpServer
	mqttConfig      *MqttConfig
	deviceId2Conn   *sync.Map
	msgChan         chan mqtt.Message
	onNewConnection types.OnNewConnection
	stopCtx         context.Context
	stopCancel      context.CancelFunc
	sync.RWMutex
}

// MqttUdpAdapterOption 用于可选参数
type MqttUdpAdapterOption func(*MqttUdpAdapter)

// WithUdpServer 设置 udpServer
func WithUdpServer(udpServer *UdpServer) MqttUdpAdapterOption {
	return func(s *MqttUdpAdapter) {
		s.udpServer = udpServer
	}
}

func WithOnNewConnection(onNewConnection types.OnNewConnection) MqttUdpAdapterOption {
	return func(s *MqttUdpAdapter) {
		s.onNewConnection = onNewConnection
	}
}

// NewMqttUdpAdapter 创建新的MQTT-UDP适配器，config为必传，其它参数用Option
func NewMqttUdpAdapter(config *MqttConfig, opts ...MqttUdpAdapterOption) *MqttUdpAdapter {
	ctx, cancel := context.WithCancel(context.Background())
	s := &MqttUdpAdapter{
		mqttConfig:    config,
		deviceId2Conn: &sync.Map{},
		msgChan:       make(chan mqtt.Message, 10000),
		stopCtx:       ctx,
		stopCancel:    cancel,
	}
	for _, opt := range opts {
		opt(s)
	}

	go s.processMessage()
	return s
}

func (s *MqttUdpAdapter) getClient() mqtt.Client {
	s.RLock()
	client := s.client
	s.RUnlock()
	return client
}

func (s *MqttUdpAdapter) setClient(client mqtt.Client) {
	s.Lock()
	s.client = client
	s.Unlock()
	s.updateSessionsClient(client)
}

func (s *MqttUdpAdapter) getUdpServer() *UdpServer {
	s.RLock()
	udpServer := s.udpServer
	s.RUnlock()
	return udpServer
}

func (s *MqttUdpAdapter) setUdpServer(udpServer *UdpServer) {
	s.Lock()
	s.udpServer = udpServer
	s.Unlock()
}

func (s *MqttUdpAdapter) updateSessionsClient(client mqtt.Client) {
	s.deviceId2Conn.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*MqttUdpConn); ok {
			conn.SetMqttClient(client)
		}
		return true
	})
}

func (s *MqttUdpAdapter) clearDeviceSessions() {
	s.deviceId2Conn.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*MqttUdpConn); ok {
			conn.Destroy()
		}
		s.deviceId2Conn.Delete(key)
		return true
	})
}

// Start 启动 MQTT 客户端（非阻塞）：在后台 goroutine 中连接并重试，不阻塞程序运行
func (s *MqttUdpAdapter) Start() error {
	Infof("MqttUdpAdapter开始启动，后台连接MQTT服务器 Broker=%s:%d ClientID=%s", s.mqttConfig.Broker, s.mqttConfig.Port, s.mqttConfig.ClientID)
	go s.connectAndRetry()
	return nil
}

// connectAndRetry 在后台循环连接 MQTT，连接失败时按间隔重试，与 mqtt_server 解耦不阻塞主流程
func (s *MqttUdpAdapter) connectAndRetry() {
	const retryInterval = 5 * time.Second

	s.RLock()
	cfg := s.mqttConfig
	s.RUnlock()
	if cfg == nil {
		return
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("%s://%s:%d", cfg.Type, cfg.Broker, cfg.Port))
	opts.SetClientID(cfg.ClientID)
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		Errorf("MQTT连接丢失: %v", err)
	})

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		Info("MQTT已连接")
		topic := ServerSubTopicPrefix
		if token := client.Subscribe(topic, 0, s.handleMessage); token.Wait() && token.Error() != nil {
			Errorf("订阅主题失败: %v", token.Error())
		}
	})

	var retryCount int
	for {
		select {
		case <-s.stopCtx.Done():
			return
		default:
		}
		client := mqtt.NewClient(opts)
		s.setClient(client)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			retryCount++
			Errorf("连接MQTT服务器失败(第%d次): %v，%d秒后重试", retryCount, token.Error(), int(retryInterval.Seconds()))
			select {
			case <-s.stopCtx.Done():
				return
			case <-time.After(retryInterval):
				continue
			}
		}
		break
	}

	_ = s.checkClientActive()
}

func (s *MqttUdpAdapter) checkClientActive() error {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCtx.Done():
				return
			case <-ticker.C:
				s.deviceId2Conn.Range(func(key, value interface{}) bool {
					conn := value.(*MqttUdpConn)
					if !conn.IsActive() {
						conn.Destroy()
					}
					return true
				})
			}
		}
	}()
	return nil
}

func (s *MqttUdpAdapter) SetDeviceSession(deviceId string, conn *MqttUdpConn) {
	Debugf("SetDeviceSession, deviceId: %s", deviceId)
	s.deviceId2Conn.Store(deviceId, conn)
}

func (s *MqttUdpAdapter) getDeviceSession(deviceId string) *MqttUdpConn {
	Debugf("getDeviceSession, deviceId: %s", deviceId)
	if conn, ok := s.deviceId2Conn.Load(deviceId); ok {
		return conn.(*MqttUdpConn)
	}
	return nil
}

// handleMessage 将消息丢进队列
func (s *MqttUdpAdapter) handleMessage(client mqtt.Client, msg mqtt.Message) {
	select {
	case s.msgChan <- msg:
		return
	default:
		Debugf("handleMessage msg chan is full, topic: %s, payload: %s", msg.Topic(), string(msg.Payload()))
	}
}

// 断开连接，超时或goodbye主动断开
func (s *MqttUdpAdapter) handleDisconnect(deviceId string) {
	Debugf("handleDisconnect, deviceId: %s", deviceId)

	conn := s.getDeviceSession(deviceId)
	if conn == nil {
		Debugf("handleDisconnect, deviceId: %s not found", deviceId)
		return
	}
	udpServer := s.getUdpServer()
	if udpServer != nil {
		udpServer.CloseSession(conn.UdpSession.ConnId)
	}
	s.deviceId2Conn.Delete(deviceId)
}

// Stop 停止适配器：取消 context、断开 MQTT、关闭 UDP、清理会话（供热更前调用）
func (s *MqttUdpAdapter) Stop() {
	Debugf("enter MqttUdpAdapter Stop ")
	defer Debugf("exit MqttUdpAdapter Stop ")
	s.stopCancel()
	client := s.getClient()
	if client != nil && client.IsConnected() {
		Debugf("MqttUdpAdapter Stop, disconnect mqtt client")
		client.Disconnect(250)
	}
	udpServer := s.getUdpServer()
	Debugf("MqttUdpAdapter Stop, udpServer: %v", udpServer)
	if udpServer != nil {
		Debugf("MqttUdpAdapter Stop, close udpServer")
		_ = udpServer.Close()
	}
	s.clearDeviceSessions()
}

// ReloadMqttClient 仅重连 MQTT（保持 UDP 服务器实例）
func (s *MqttUdpAdapter) ReloadMqttClient(newConfig *MqttConfig) {
	if newConfig == nil {
		return
	}
	s.Lock()
	s.mqttConfig = newConfig
	oldClient := s.client
	s.Unlock()
	if oldClient != nil && oldClient.IsConnected() {
		oldClient.Disconnect(250)
	}
	s.clearDeviceSessions()
	go s.connectAndRetry()
}

// ReloadUdpServer 仅重启 UDP（保持 MQTT 连接）
func (s *MqttUdpAdapter) ReloadUdpServer(newUdpServer *UdpServer) {
	if newUdpServer == nil {
		return
	}
	oldUdp := s.getUdpServer()
	s.clearDeviceSessions()
	s.setUdpServer(newUdpServer)
	if oldUdp != nil {
		_ = oldUdp.Close()
	}
}

// 处理消息
func (s *MqttUdpAdapter) processMessage() {
	for {
		select {
		case <-s.stopCtx.Done():
			return
		case msg := <-s.msgChan:
			Debugf("mqtt handleMessage, topic: %s, payload: %s", msg.Topic(), string(msg.Payload()))
			var clientMsg ClientMessage
			if err := json.Unmarshal(msg.Payload(), &clientMsg); err != nil {
				Errorf("解析JSON失败: %v", err)
				continue
			}
			topicMacAddr, deviceId := s.getDeviceIdByTopic(msg.Topic())
			if deviceId == "" {
				Errorf("mac_addr解析失败: %v", msg.Topic())
				continue
			}

			deviceSession := s.getDeviceSession(deviceId)
			if deviceSession == nil {
				// 从UDP服务端获取会话信息
				udpServer := s.getUdpServer()
				if udpServer == nil {
					Errorf("udpServer is nil, deviceId: %s", deviceId)
					continue
				}
				udpSession := udpServer.CreateSession(deviceId, "")
				if udpSession == nil {
					Errorf("创建 udpSession 失败, deviceId: %s", deviceId)
					continue
				}

				publicTopic := fmt.Sprintf("%s%s", client.ServerPubTopicPrefix, topicMacAddr)

				mqttClient := s.getClient()
				if mqttClient == nil {
					Errorf("mqtt client is nil, deviceId: %s", deviceId)
					continue
				}
				deviceSession = NewMqttUdpConn(deviceId, publicTopic, mqttClient, udpServer, udpSession)

				strAesKey, strFullNonce := udpSession.GetAesKeyAndNonce()
				deviceSession.SetData("aes_key", strAesKey)
				deviceSession.SetData("full_nonce", strFullNonce)

				//保存至deviceId2UdpSession
				s.SetDeviceSession(deviceId, deviceSession)

				deviceSession.OnClose(s.handleDisconnect)

				s.onNewConnection(deviceSession)
			}

			err := deviceSession.PushMsgToRecvCmd(msg.Payload())
			if err != nil {
				Errorf("InternalRecvCmd失败: %v", err)
				continue
			}
		}
	}
}

func (s *MqttUdpAdapter) getDeviceIdByTopic(topic string) (string, string) {
	var topicMacAddr, deviceId string
	//根据topic(/p2p/device_public/mac_addr)解析出来mac_addr
	strList := strings.Split(topic, "/")
	if len(strList) == 4 {
		topicMacAddr = strList[3]

		// 检查是否为新格式: "GID_test@@@ba_8f_17_de_94_94@@@e4b0c442-98fc-4e1b-8c3d-6a5b6a5b6a6d"
		if strings.Contains(topicMacAddr, "@@@") {
			parts := strings.Split(topicMacAddr, "@@@")
			if len(parts) >= 2 {
				// 提取中间部分作为MAC地址
				macAddr := parts[1]
				deviceId = strings.ReplaceAll(macAddr, "_", ":")
			}
		} else {
			deviceId = strings.ReplaceAll(topicMacAddr, "_", ":")
		}
	}

	log.Log().Debugf("topicMacAddr: %s, deviceId: %s", topicMacAddr, deviceId)
	return topicMacAddr, deviceId
}
