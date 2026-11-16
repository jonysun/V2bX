package sing

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/common/rate"

	"github.com/InazumaV/V2bX/limiter"

	"github.com/InazumaV/V2bX/common/counter"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-tun"
	N "github.com/sagernet/sing/common/network"
)

var _ adapter.ConnectionTracker = (*HookServer)(nil)

type HookServer struct {
	counter       sync.Map //map[string]*counter.TrafficCounter
	connectionsMu sync.Mutex
	connections   map[string]map[*activeConnection]struct{}
	revoked       map[string]struct{}
}

type activeConnection struct {
	server    *HookServer
	userTag   string
	closeConn func() error
	once      sync.Once
	err       error
}

func (c *activeConnection) Close() error {
	c.once.Do(func() {
		c.err = c.closeConn()
	})
	return c.err
}

func (c *activeConnection) closed() {
	c.server.removeConnection(c)
}

func (h *HookServer) registerConnection(active *activeConnection) bool {
	h.connectionsMu.Lock()
	defer h.connectionsMu.Unlock()
	if _, blocked := h.revoked[active.userTag]; blocked {
		return false
	}
	if h.connections == nil {
		h.connections = make(map[string]map[*activeConnection]struct{})
	}
	if h.connections[active.userTag] == nil {
		h.connections[active.userTag] = make(map[*activeConnection]struct{})
	}
	h.connections[active.userTag][active] = struct{}{}
	return true
}

func (h *HookServer) removeConnection(active *activeConnection) {
	h.connectionsMu.Lock()
	defer h.connectionsMu.Unlock()
	connections := h.connections[active.userTag]
	delete(connections, active)
	if len(connections) == 0 {
		delete(h.connections, active.userTag)
	}
}

func (h *HookServer) AllowUser(tag, uuid string) {
	h.connectionsMu.Lock()
	delete(h.revoked, format.UserTag(tag, uuid))
	h.connectionsMu.Unlock()
}

func (h *HookServer) RevokeUser(tag, uuid string) {
	userTag := format.UserTag(tag, uuid)
	h.connectionsMu.Lock()
	if h.revoked == nil {
		h.revoked = make(map[string]struct{})
	}
	h.revoked[userTag] = struct{}{}
	activeConnections := make([]*activeConnection, 0, len(h.connections[userTag]))
	for active := range h.connections[userTag] {
		activeConnections = append(activeConnections, active)
	}
	h.connectionsMu.Unlock()
	for _, active := range activeConnections {
		_ = active.Close()
	}
}

func (h *HookServer) ModeList() []string {
	return nil
}

func (h *HookServer) RoutedConnection(_ context.Context, conn net.Conn, m adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) net.Conn {
	taguuid := format.UserTag(m.Inbound, m.User)
	l, err := limiter.GetLimiter(m.Inbound)
	if err != nil {
		log.Warn("get limiter for ", m.Inbound, " error: ", err)
		l = nil
	}
	if l != nil {
		ip := m.Source.Addr.String()
		if b, rejected := l.CheckLimit(taguuid, ip, true, true); rejected {
			_ = conn.Close()
			log.Error("[", m.Inbound, "] Limited ", m.User, " by ip or conn")
			return conn
		} else if b != nil {
			conn = rate.NewConnRateLimiter(conn, b)
		}
		destStr := m.Destination.AddrString()
		protocol := m.Protocol
		if l.CheckDomainRule(destStr) {
			log.Error(fmt.Sprintf("User %s access domain %s reject by rule", m.User, destStr))
			_ = conn.Close()
			return conn
		}
		if len(protocol) != 0 {
			if l.CheckProtocolRule(protocol) {
				log.Error(fmt.Sprintf("User %s access protocol %s reject by rule", m.User, protocol))
				_ = conn.Close()
				return conn
			}
		}
	}
	var t *counter.TrafficCounter
	if c, ok := h.counter.Load(m.Inbound); !ok {
		actual, _ := h.counter.LoadOrStore(m.Inbound, counter.NewTrafficCounter())
		t = actual.(*counter.TrafficCounter)
	} else {
		t = c.(*counter.TrafficCounter)
	}
	active := &activeConnection{server: h, userTag: taguuid}
	conn = counter.NewConnCounterWithClose(conn, t.GetCounter(m.User), active.closed)
	active.closeConn = conn.Close
	if !h.registerConnection(active) {
		_ = active.Close()
		return conn
	}
	return conn
}

func (h *HookServer) RoutedPacketConnection(_ context.Context, conn N.PacketConn, m adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) N.PacketConn {
	taguuid := format.UserTag(m.Inbound, m.User)
	l, err := limiter.GetLimiter(m.Inbound)
	if err != nil {
		log.Warn("get limiter for ", m.Inbound, " error: ", err)
		l = nil
	}
	if l != nil {
		ip := m.Source.Addr.String()
		if b, rejected := l.CheckLimit(taguuid, ip, false, true); rejected {
			_ = conn.Close()
			log.Error("[", m.Inbound, "] Limited ", m.User, " by ip or conn")
			return conn
		} else if b != nil {
			conn = rate.NewPacketConnRateLimiter(conn, b)
		}
		destStr := m.Destination.AddrString()
		protocol := m.Protocol
		if l.CheckDomainRule(destStr) {
			log.Error(fmt.Sprintf("User %s access domain %s reject by rule", m.User, destStr))
			_ = conn.Close()
			return conn
		}
		if len(protocol) != 0 {
			if l.CheckProtocolRule(protocol) {
				log.Error(fmt.Sprintf("User %s access protocol %s reject by rule", m.User, protocol))
				_ = conn.Close()
				return conn
			}
		}
	}
	var t *counter.TrafficCounter
	if c, ok := h.counter.Load(m.Inbound); !ok {
		actual, _ := h.counter.LoadOrStore(m.Inbound, counter.NewTrafficCounter())
		t = actual.(*counter.TrafficCounter)
	} else {
		t = c.(*counter.TrafficCounter)
	}
	active := &activeConnection{server: h, userTag: taguuid}
	conn = counter.NewPacketConnCounterWithClose(conn, t.GetCounter(m.User), active.closed)
	active.closeConn = conn.Close
	if !h.registerConnection(active) {
		_ = active.Close()
		return conn
	}
	return conn
}

func (h *HookServer) RoutedFlow(context.Context, adapter.InboundContext, adapter.Rule, adapter.Outbound) tun.FlowTracker {
	return nil
}
