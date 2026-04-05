package mqtt_udp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"xiaozhi-esp32-server-golang/internal/app/server/types"

	log "xiaozhi-esp32-server-golang/logger"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	// mqttSessionTTL MQTT 会话保留时长：超过后才销毁整个连接。
	mqttSessionTTL = 72 * time.Hour
	// udpIdleTTL UDP 资源空闲保留时长：超过后释放 UDP 资源。
	udpIdleTTL = 10 * time.Minute
	// sessionCleanupInterval 连接状态巡检间隔。
	sessionCleanupInterval = 30 * time.Second
)

// MqttUdpConn 实现 types.IConn 接口，适配 MQTT-UDP 连接
// 你可以根据实际需要扩展方法和字段

type MqttUdpConn struct {
	ctx    context.Context
	cancel context.CancelFunc

	DeviceId string

	PubTopic   string
	MqttClient mqtt.Client
	udpServer  *UdpServer

	UdpSession *UdpSession

	recvCmdChan chan []byte
	sync.RWMutex

	data sync.Map

	onCloseCbList []func(deviceId string)

	lastMqttActiveTs int64 // MQTT 信令上下行活跃时间
	lastUdpActiveTs  int64 // UDP 音频上下行活跃时间
}

// NewMqttUdpConn 创建一个新的 MqttUdpConn 实例
func NewMqttUdpConn(deviceID string, pubTopic string, mqttClient mqtt.Client, udpServer *UdpServer, udpSession *UdpSession) *MqttUdpConn {
	ctx, cancel := context.WithCancel(context.Background())
	nowUnix := time.Now().Unix()
	log.Log().Debugf("NewMqttUdpConn pubTopic: %s", pubTopic)
	return &MqttUdpConn{
		ctx:      ctx,
		cancel:   cancel,
		DeviceId: deviceID,

		PubTopic:   pubTopic,
		MqttClient: mqttClient,
		udpServer:  udpServer,
		UdpSession: udpSession,

		recvCmdChan: make(chan []byte, 100),

		data:             sync.Map{},
		lastMqttActiveTs: nowUnix,
		lastUdpActiveTs:  nowUnix,
	}
}

// SendCmd 通过 MQTT-UDP 发送命令（需对接实际发送逻辑）
func (c *MqttUdpConn) SendCmd(msg []byte) error {
	//log.Debugf("mqtt udp conn send cmd, topic: %s, msg: %s", c.PubTopic, string(msg))
	c.touchMqttActive()
	c.RLock()
	client := c.MqttClient
	c.RUnlock()
	if client == nil {
		return errors.New("mqtt client is nil")
	}
	token := client.Publish(c.PubTopic, 0, false, msg)
	token.Wait()
	if token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (c *MqttUdpConn) PushMsgToRecvCmd(msg []byte) error {
	select {
	case c.recvCmdChan <- msg:
		c.touchMqttActive()
		return nil
	default:
		return errors.New("recvCmdChan is full")
	}
}

// RecvCmd 接收命令/信令数据
func (c *MqttUdpConn) RecvCmd(ctx context.Context, timeout int) ([]byte, error) {
	select {
	case <-ctx.Done():
		log.Debugf("mqtt udp conn recv cmd context done")
		return nil, ctx.Err()
	case msg := <-c.recvCmdChan:
		return msg, nil
	case <-time.After(time.Duration(timeout) * time.Second):
		log.Debugf("mqtt udp conn recv cmd timeout")
		return nil, nil
	}
}

// SendAudio 通过 MQTT-UDP 发送音频（需对接实际发送逻辑）
func (c *MqttUdpConn) SendAudio(audio []byte) error {
	udpSession := c.GetUdpSession()
	if udpSession == nil {
		return nil
	}
	ok, err := udpSession.SendAudioData(audio)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("sendAudioChan is full")
	}
	c.touchUdpActive()
	return nil
}

// RecvAudio 接收音频数据
func (c *MqttUdpConn) RecvAudio(ctx context.Context, timeout int) ([]byte, error) {
	udpSession := c.GetUdpSession()
	if udpSession == nil {
		wait := time.Second
		if timeout > 0 {
			timeoutDuration := time.Duration(timeout) * time.Second
			if timeoutDuration < wait {
				wait = timeoutDuration
			}
		}
		select {
		case <-ctx.Done():
			log.Debugf("mqtt udp conn recv audio context done")
			return nil, ctx.Err()
		case <-time.After(wait):
			return nil, nil
		}
	}
	select {
	case <-ctx.Done():
		log.Debugf("mqtt udp conn recv audio context done")
		return nil, ctx.Err()
	case audio, ok := <-udpSession.RecvChannel:
		if ok {
			c.touchUdpActive()
			return audio, nil
		}
		return nil, nil
	case <-time.After(time.Duration(timeout) * time.Second):
		log.Debugf("mqtt udp conn recv audio timeout")
		return nil, nil
	}
}

// GetDeviceID 获取设备ID
func (c *MqttUdpConn) GetDeviceID() string {
	return c.DeviceId
}

// Close 关闭连接
func (c *MqttUdpConn) Close() error {
	//c.cancel()
	c.Destroy()
	return nil
}

func (c *MqttUdpConn) OnClose(closeCb func(deviceId string)) {
	c.onCloseCbList = append(c.onCloseCbList, closeCb)
}

func (c *MqttUdpConn) SetMqttClient(client mqtt.Client) {
	c.Lock()
	c.MqttClient = client
	c.Unlock()
}

func (c *MqttUdpConn) GetUdpSession() *UdpSession {
	c.RLock()
	defer c.RUnlock()
	return c.UdpSession
}

func (c *MqttUdpConn) SetUdpSession(session *UdpSession) {
	c.Lock()
	c.UdpSession = session
	c.Unlock()
	if session != nil {
		c.touchUdpActive()
	}
}

func (c *MqttUdpConn) ReleaseUdpSession() {
	c.Lock()
	udpSession := c.UdpSession
	c.UdpSession = nil
	c.Unlock()
	if udpSession == nil {
		return
	}
	if c.udpServer != nil {
		c.udpServer.CloseSession(udpSession.ConnId)
	} else {
		udpSession.Destroy()
	}
}

func (c *MqttUdpConn) GetTransportType() string {
	return types.TransportTypeMqttUdp
}

func (c *MqttUdpConn) SetData(key string, value interface{}) {
	c.data.Store(key, value)
}

func (c *MqttUdpConn) GetData(key string) (interface{}, error) {
	value, ok := c.data.Load(key)
	if !ok {
		return nil, errors.New("key not found")
	}
	return value, nil
}

func (c *MqttUdpConn) IsActive() bool {
	return c.IsMqttActive(time.Now())
}

func (c *MqttUdpConn) IsMqttActive(now time.Time) bool {
	return isActiveWithin(atomic.LoadInt64(&c.lastMqttActiveTs), mqttSessionTTL, now)
}

func (c *MqttUdpConn) IsUdpActive(now time.Time) bool {
	if c.GetUdpSession() == nil {
		return true
	}
	return isActiveWithin(atomic.LoadInt64(&c.lastUdpActiveTs), udpIdleTTL, now)
}

func (c *MqttUdpConn) touchMqttActive() {
	atomic.StoreInt64(&c.lastMqttActiveTs, time.Now().Unix())
}

func (c *MqttUdpConn) touchUdpActive() {
	atomic.StoreInt64(&c.lastUdpActiveTs, time.Now().Unix())
}

func isActiveWithin(lastTs int64, ttl time.Duration, now time.Time) bool {
	if ttl <= 0 {
		return true
	}
	if lastTs <= 0 {
		return true
	}
	return now.Unix()-lastTs < int64(ttl.Seconds())
}

// 销毁
func (c *MqttUdpConn) Destroy() {
	c.cancel()
	for _, cb := range c.onCloseCbList {
		cb(c.DeviceId)
	}
}

func (c *MqttUdpConn) CloseAudioChannel() error {
	c.ReleaseUdpSession()
	return nil
}
