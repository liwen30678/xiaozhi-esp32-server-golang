package mqtt_udp

import (
	"testing"
	"time"
)

func newTestUdpSession() *UdpSession {
	return &UdpSession{
		RecvChannel: make(chan []byte, 1),
		SendChannel: make(chan []byte, 1),
		Status:      UdpSessionStatusActive,
	}
}

func TestMqttUdpConnBrokerLifecycleAffectsIsActive(t *testing.T) {
	conn := NewMqttUdpConn("device-1", "/p2p/device_sub/device_1", nil, nil, newTestUdpSession())
	defer conn.Destroy()

	if !conn.IsActive() {
		t.Fatalf("expected new mqtt udp conn to be active")
	}

	conn.MarkBrokerOffline(20 * time.Millisecond)
	if !conn.IsActive() {
		t.Fatalf("expected conn to remain active during offline grace period")
	}

	time.Sleep(35 * time.Millisecond)
	if conn.IsActive() {
		t.Fatalf("expected conn to become inactive after offline grace period")
	}

	conn.MarkBrokerOnline()
	if !conn.IsActive() {
		t.Fatalf("expected conn to become active again after broker reconnect")
	}
}

func TestLifecycleReconnectCancelsOfflineCleanup(t *testing.T) {
	adapter := NewMqttUdpAdapter(&MqttConfig{})
	adapter.offlineGracePeriod = 20 * time.Millisecond
	defer adapter.Stop()

	if notify := adapter.markDeviceOnline("device-1", 100); !notify {
		t.Fatalf("expected first online event to notify")
	}

	notifyOffline, version := adapter.markDeviceOffline("device-1", 200)
	if !notifyOffline {
		t.Fatalf("expected offline transition to notify")
	}
	if version == 0 {
		t.Fatalf("expected offline cleanup version to be assigned")
	}

	time.Sleep(5 * time.Millisecond)

	if notify := adapter.markDeviceOnline("device-1", 300); !notify {
		t.Fatalf("expected reconnect to notify online again")
	}

	time.Sleep(30 * time.Millisecond)

	state := adapter.getLifecycleState("device-1")
	if state == nil {
		t.Fatalf("expected lifecycle state to remain after reconnect")
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.brokerOnline {
		t.Fatalf("expected device to stay online after reconnect")
	}
	if state.cleanupTimer != nil {
		t.Fatalf("expected offline cleanup timer to be cancelled after reconnect")
	}
}

func TestHandleDisconnectIgnoresStaleConn(t *testing.T) {
	adapter := NewMqttUdpAdapter(&MqttConfig{})
	defer adapter.Stop()

	udpServer := NewUDPServer(0, "", 0)
	adapter.setUdpServer(udpServer)

	staleSession := udpServer.CreateSession("device-1", "")
	if staleSession == nil {
		t.Fatal("expected stale session to be created")
	}
	freshSession := udpServer.CreateSession("device-1", "")
	if freshSession == nil {
		t.Fatal("expected fresh session to be created")
	}
	t.Cleanup(func() {
		udpServer.CloseSessionByRef(staleSession)
		udpServer.CloseSessionByRef(freshSession)
	})

	staleConn := NewMqttUdpConn("device-1", "topic/device-1", nil, udpServer, staleSession)
	freshConn := NewMqttUdpConn("device-1", "topic/device-1", nil, udpServer, freshSession)

	adapter.SetDeviceSession("device-1", freshConn)
	adapter.handleDisconnect("device-1", staleConn)

	if adapter.getDeviceSession("device-1") != freshConn {
		t.Fatal("expected fresh conn to stay registered after stale disconnect")
	}
	if freshConn.GetUdpSession() != freshSession {
		t.Fatal("expected fresh udp session to remain attached")
	}
	if staleConn.GetUdpSession() != staleSession {
		t.Fatal("expected stale conn to remain untouched by ignored disconnect")
	}
}
