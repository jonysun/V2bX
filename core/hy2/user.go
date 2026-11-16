package hy2

import (
	"fmt"
	"net"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/counter"
	"github.com/InazumaV/V2bX/common/format"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/apernet/hysteria/core/v2/server"
)

var _ server.Authenticator = &V2bX{}

type V2bX struct {
	Tag      string
	usersMap map[string]int
	mutex    sync.RWMutex
}

func (v *V2bX) Authenticate(addr net.Addr, auth string, tx uint64) (ok bool, id string) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()
	if _, exists := v.usersMap[auth]; exists {
		l, err := limiter.GetLimiter(v.Tag)
		if err != nil {
			return false, ""
		}
		if _, reject := l.CheckLimit(format.UserTag(v.Tag, auth), extractIPFromAddr(addr), false, true); reject {
			return false, ""
		}
		return true, auth
	}
	return false, ""
}

func (h *Hysteria2) AddUsers(p *vCore.AddUsersParams) (added int, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n, ok := h.Hy2nodes[p.Tag]
	if !ok {
		return 0, fmt.Errorf("unknown Hysteria2 node %q", p.Tag)
	}
	n.Auth.mutex.Lock()
	defer n.Auth.mutex.Unlock()
	for _, user := range p.Users {
		n.Auth.usersMap[user.Uuid] = user.Id
	}
	return len(p.Users), nil
}

func (h *Hysteria2) DelUsers(users []panel.UserInfo, tag string, _ *panel.NodeInfo) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n, ok := h.Hy2nodes[tag]
	if !ok {
		return nil
	}
	n.Auth.mutex.Lock()
	defer n.Auth.mutex.Unlock()
	for _, user := range users {
		if v, ok := n.TrafficLogger.(*HookServer).Counter.Load(tag); ok {
			c := v.(*counter.TrafficCounter)
			c.Delete(user.Uuid)
		}
		delete(n.Auth.usersMap, user.Uuid)
	}
	return nil
}

func (h *Hysteria2) GetUserTrafficSlice(tag string, reset bool) ([]panel.UserTraffic, error) {
	return h.getTraffic(tag, reset, false)
}

func (h *Hysteria2) DrainTraffic(tag string) ([]panel.UserTraffic, error) {
	return h.getTraffic(tag, true, true)
}

func (h *Hysteria2) getTraffic(tag string, reset, force bool) ([]panel.UserTraffic, error) {
	trafficSlice := make([]panel.UserTraffic, 0)
	h.mu.RLock()
	defer h.mu.RUnlock()
	n, ok := h.Hy2nodes[tag]
	if !ok {
		return nil, nil
	}
	n.Auth.mutex.RLock()
	defer n.Auth.mutex.RUnlock()
	hook := n.TrafficLogger.(*HookServer)
	if v, ok := hook.Counter.Load(tag); ok {
		c := v.(*counter.TrafficCounter)
		c.Counters.Range(func(key, value interface{}) bool {
			uuid := key.(string)
			traffic := value.(*counter.TrafficStorage)
			up := traffic.UpCounter.Load()
			down := traffic.DownCounter.Load()
			if up+down > hook.ReportMinTrafficBytes || force {
				up, down = traffic.Snapshot(reset)
				if n.Auth.usersMap[uuid] == 0 {
					c.Delete(uuid)
					return true
				}
				trafficSlice = append(trafficSlice, panel.UserTraffic{
					UID:      n.Auth.usersMap[uuid],
					Uuid:     uuid,
					Upload:   up,
					Download: down,
				})
			}
			return true
		})
		if len(trafficSlice) == 0 {
			return nil, nil
		}
		return trafficSlice, nil
	}
	return nil, nil
}
