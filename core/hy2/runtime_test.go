package hy2

import (
	"net"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	"go.uber.org/zap"
)

func TestCrossNodeAuthenticationAndDeletionAreIsolated(t *testing.T) {
	limiter.Init()
	h := &Hysteria2{Hy2nodes: map[string]Hysteria2node{}}
	for _, tag := range []string{"a", "b"} {
		h.Hy2nodes[tag] = Hysteria2node{Auth: &V2bX{Tag: tag, usersMap: map[string]int{}}, TrafficLogger: &HookServer{Tag: tag}}
		limiter.AddLimiter(tag, &conf.LimitConfig{}, []panel.UserInfo{{Id: 1, Uuid: "shared"}, {Id: 2, Uuid: "only-a"}}, nil)
	}
	for _, tag := range []string{"a", "b"} {
		_, err := h.AddUsers(&vCore.AddUsersParams{Tag: tag, Users: []panel.UserInfo{{Id: 1, Uuid: "shared"}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _ = h.AddUsers(&vCore.AddUsersParams{Tag: "a", Users: []panel.UserInfo{{Id: 2, Uuid: "only-a"}}})
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
	if ok, _ := h.Hy2nodes["b"].Auth.Authenticate(addr, "only-a", 0); ok {
		t.Fatal("node b accepted node a's credential")
	}
	if err := h.DelUsers([]panel.UserInfo{{Uuid: "shared"}}, "a", nil); err != nil {
		t.Fatal(err)
	}
	if ok, _ := h.Hy2nodes["b"].Auth.Authenticate(addr, "shared", 0); !ok {
		t.Fatal("deleting from a removed b's credential")
	}
}

func TestListenerCleanupAfterObfuscationAndMasqueradeFailure(t *testing.T) {
	n := &Hysteria2node{Logger: zap.NewNop()}
	for _, obfs := range []string{"unsupported", "salamander"} {
		info := &panel.NodeInfo{Common: &panel.CommonNode{}, Hysteria2: &panel.Hysteria2Node{ObfsType: obfs}}
		if conn, err := n.getConn(info, &conf.Options{ListenIP: "127.0.0.1"}); err == nil {
			_ = conn.Close()
			t.Fatalf("accepted invalid obfuscation %q", obfs)
		}
	}
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	n.conn = conn
	addr := conn.LocalAddr().String()
	if err := n.close(); err != nil {
		t.Fatal(err)
	}
	rebound, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("UDP listener leaked: %v", err)
	}
	_ = rebound.Close()
}

func TestTrafficHookRejectsDeletedUser(t *testing.T) {
	limiter.Init()
	l := limiter.AddLimiter("n", &conf.LimitConfig{}, []panel.UserInfo{{Id: 1, Uuid: "u"}}, nil)
	h := &HookServer{Tag: "n", logger: zap.NewNop()}
	if !h.LogTraffic("u", 10, 20) {
		t.Fatal("valid user rejected")
	}
	l.UpdateUser("n", nil, []panel.UserInfo{{Id: 1, Uuid: "u"}})
	if h.LogTraffic("u", 1, 1) {
		t.Fatal("deleted user traffic was accepted")
	}
}
