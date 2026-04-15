package mqtt_udp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRotateDeviceUdpSessionReplacesSessionAndCleansOldIndexes(t *testing.T) {
	udpServer := NewUDPServer(0, "", 0)
	adapter := &MqttUdpAdapter{udpServer: udpServer}

	oldSession := udpServer.CreateSession("device-1", "")
	require.NotNil(t, oldSession)

	oldAddr := testUDPAddr(10001)
	oldAliasAddr := testUDPAddr(10002)
	oldSession.SetRemoteAddr(oldAddr)
	udpServer.addUdpSession(oldAddr, oldSession)
	udpServer.addUdpSession(oldAliasAddr, oldSession)

	conn := NewMqttUdpConn("device-1", "topic/device-1", nil, udpServer, oldSession)
	t.Cleanup(func() {
		conn.ReleaseUdpSession()
	})

	_, oldNonce := oldSession.GetAesKeyAndNonce()
	adapter.bindUdpSessionData(conn, oldSession)

	newSession, err := adapter.rotateDeviceUdpSession(conn, "device-1")
	require.NoError(t, err)
	require.NotNil(t, newSession)

	assert.NotSame(t, oldSession, newSession)
	assert.Same(t, newSession, conn.GetUdpSession())
	assert.Same(t, newSession, udpServer.GetNonce(newSession.ConnId))
	assert.Nil(t, udpServer.GetNonce(oldSession.ConnId))
	assert.Nil(t, udpServer.getUdpSession(oldAddr))
	assert.Nil(t, udpServer.getUdpSession(oldAliasAddr))
	assert.True(t, oldSession.IsClosed())
	assert.NotEqual(t, oldSession.ConnId, newSession.ConnId)

	newKey, newNonce := newSession.GetAesKeyAndNonce()
	assert.NotEqual(t, oldNonce, newNonce)

	boundKey, err := conn.GetData("aes_key")
	require.NoError(t, err)
	assert.Equal(t, newKey, boundKey)

	boundNonce, err := conn.GetData("full_nonce")
	require.NoError(t, err)
	assert.Equal(t, newNonce, boundNonce)
}

func TestProcessPacketRebindsSessionByConnIDAndRemovesOldAlias(t *testing.T) {
	udpServer := NewUDPServer(0, "", 0)

	session := udpServer.CreateSession("device-2", "")
	require.NotNil(t, session)
	t.Cleanup(func() {
		udpServer.CloseSessionByRef(session)
	})

	oldAddr := testUDPAddr(10011)
	newAddr := testUDPAddr(10012)
	session.SetRemoteAddr(oldAddr)
	udpServer.addUdpSession(oldAddr, session)

	payload := []byte("hello-audio")
	packet, err := session.Encrypt(payload)
	require.NoError(t, err)

	udpServer.processPacket(newAddr, packet)

	assert.Nil(t, udpServer.getUdpSession(oldAddr))
	assert.Same(t, session, udpServer.getUdpSession(newAddr))

	remoteAddr := session.GetRemoteAddr()
	require.NotNil(t, remoteAddr)
	assert.Equal(t, newAddr.String(), remoteAddr.String())

	received := requireReceivePayload(t, session.RecvChannel)
	assert.Equal(t, payload, received)
}

func TestOldPacketCannotRebindAfterSessionRotation(t *testing.T) {
	udpServer := NewUDPServer(0, "", 0)
	adapter := &MqttUdpAdapter{udpServer: udpServer}

	oldSession := udpServer.CreateSession("device-3", "")
	require.NotNil(t, oldSession)

	oldAddr := testUDPAddr(10021)
	oldSession.SetRemoteAddr(oldAddr)
	udpServer.addUdpSession(oldAddr, oldSession)

	oldPacket, err := oldSession.Encrypt([]byte("stale-packet"))
	require.NoError(t, err)

	conn := NewMqttUdpConn("device-3", "topic/device-3", nil, udpServer, oldSession)
	t.Cleanup(func() {
		conn.ReleaseUdpSession()
	})
	adapter.bindUdpSessionData(conn, oldSession)

	newSession, err := adapter.rotateDeviceUdpSession(conn, "device-3")
	require.NoError(t, err)
	require.NotNil(t, newSession)

	udpServer.processPacket(oldAddr, oldPacket)

	assert.Nil(t, udpServer.GetNonce(oldSession.ConnId))
	assert.Nil(t, udpServer.getUdpSession(oldAddr))
	assert.Same(t, newSession, conn.GetUdpSession())
}

func testUDPAddr(port int) *net.UDPAddr {
	return &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: port,
	}
}

func requireReceivePayload(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()

	select {
	case payload := <-ch:
		return payload
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for decrypted payload")
		return nil
	}
}
