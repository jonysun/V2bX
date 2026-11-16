package sing

import (
	"net"
	"testing"

	"github.com/InazumaV/V2bX/common/counter"
	"github.com/InazumaV/V2bX/common/format"
)

func TestRevokeUserClosesConnectionsAndAllowsReactivation(t *testing.T) {
	hook := &HookServer{}
	traffic := counter.NewTrafficCounter()

	register := func() (net.Conn, net.Conn, *activeConnection) {
		serverConn, clientConn := net.Pipe()
		active := &activeConnection{server: hook, userTag: format.UserTag("inbound", "user")}
		tracked := counter.NewConnCounterWithClose(serverConn, traffic.GetCounter("user"), active.closed)
		active.closeConn = tracked.Close
		return tracked, clientConn, active
	}

	tracked, peer, active := register()
	defer peer.Close()
	if !hook.registerConnection(active) {
		t.Fatal("first connection was rejected")
	}
	hook.RevokeUser("inbound", "user")
	if _, err := peer.Write([]byte("x")); err == nil {
		t.Fatal("revoked connection remained open")
	}

	blocked, blockedPeer, blockedActive := register()
	defer blockedPeer.Close()
	if hook.registerConnection(blockedActive) {
		_ = blocked.Close()
		t.Fatal("revoked user opened a new connection")
	}
	_ = blockedActive.Close()

	hook.AllowUser("inbound", "user")
	tracked, peer, active = register()
	defer peer.Close()
	if !hook.registerConnection(active) {
		t.Fatal("reactivated user was rejected")
	}
	if err := tracked.Close(); err != nil {
		t.Fatalf("close tracked connection: %v", err)
	}
	hook.connectionsMu.Lock()
	remaining := len(hook.connections[active.userTag])
	hook.connectionsMu.Unlock()
	if remaining != 0 {
		t.Fatalf("closed connection remained registered: %d", remaining)
	}
}
