package xray

import (
	"context"
	"fmt"
	"strings"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/counter"
	"github.com/InazumaV/V2bX/common/format"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/core/xray/app/dispatcher"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/proxy"
	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func (c *Xray) GetUserManager(tag string) (proxy.UserManager, error) {
	handler, err := c.ihm.GetHandler(context.Background(), tag)
	if err != nil {
		return nil, fmt.Errorf("no such inbound tag: %s", err)
	}
	inboundInstance, ok := handler.(proxy.GetInbound)
	if !ok {
		return nil, fmt.Errorf("handler %s is not implement proxy.GetInbound", tag)
	}
	userManager, ok := inboundInstance.GetInbound().(proxy.UserManager)
	if !ok {
		return nil, fmt.Errorf("handler %s is not implement proxy.UserManager", tag)
	}
	return userManager, nil
}

func (c *Xray) DelUsers(users []panel.UserInfo, tag string, _ *panel.NodeInfo) error {
	c.access.Lock()
	defer c.access.Unlock()
	userManager, err := c.GetUserManager(tag)
	if err != nil {
		return fmt.Errorf("get user manager error: %s", err)
	}
	var user string
	c.users.mapLock.Lock()
	defer c.users.mapLock.Unlock()
	for i := range users {
		user = format.UserTag(tag, users[i].Uuid)
		if userManager.GetUser(context.Background(), user) != nil {
			if err = userManager.RemoveUser(context.Background(), user); err != nil {
				return err
			}
		}
		delete(c.users.uidMap, user)
		if v, ok := c.dispatcher.Counter.Load(tag); ok {
			tc := v.(*counter.TrafficCounter)
			tc.Delete(user)
		}
		if v, ok := c.dispatcher.LinkManagers.Load(user); ok {
			lm := v.(*dispatcher.LinkManager)
			lm.CloseAll()
			c.dispatcher.LinkManagers.Delete(user)
		}
	}
	return nil
}

func (x *Xray) GetUserTrafficSlice(tag string, reset bool) ([]panel.UserTraffic, error) {
	return x.getTraffic(tag, reset, false)
}

func (x *Xray) DrainTraffic(tag string) ([]panel.UserTraffic, error) {
	return x.getTraffic(tag, true, true)
}

func (x *Xray) getTraffic(tag string, reset, force bool) ([]panel.UserTraffic, error) {
	x.access.Lock()
	defer x.access.Unlock()
	trafficSlice := make([]panel.UserTraffic, 0)
	x.users.mapLock.RLock()
	defer x.users.mapLock.RUnlock()
	if v, ok := x.dispatcher.Counter.Load(tag); ok {
		c := v.(*counter.TrafficCounter)
		c.Counters.Range(func(key, value interface{}) bool {
			email := key.(string)
			traffic := value.(*counter.TrafficStorage)
			up := traffic.UpCounter.Load()
			down := traffic.DownCounter.Load()
			if up+down > x.nodeReportMinTrafficBytes[tag] || force {
				up, down = traffic.Snapshot(reset)
				if x.users.uidMap[email] == 0 {
					c.Delete(email)
					return true
				}
				trafficSlice = append(trafficSlice, panel.UserTraffic{
					UID:      x.users.uidMap[email],
					Uuid:     strings.TrimPrefix(email, tag+"|"),
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

func (c *Xray) AddUsers(p *vCore.AddUsersParams) (added int, err error) {
	c.access.Lock()
	defer c.access.Unlock()
	c.users.mapLock.Lock()
	defer c.users.mapLock.Unlock()
	if p.NodeInfo.Type == "shadowsocks" && p.Shadowsocks.ServerKey == "" && getCipherFromString(p.Shadowsocks.Cipher) == shadowsocks.CipherType_UNKNOWN {
		return 0, fmt.Errorf("unsupported Shadowsocks cipher %q", p.Shadowsocks.Cipher)
	}
	if p.NodeInfo.Type == "shadowsocks" && p.Shadowsocks.ServerKey != "" {
		length := 32
		if p.Shadowsocks.Cipher == "2022-blake3-aes-128-gcm" {
			length = 16
		}
		for _, u := range p.Users {
			if len(u.Uuid) < length {
				return 0, fmt.Errorf("Shadowsocks 2022 user %d credential requires at least %d bytes", u.Id, length)
			}
		}
	}
	var users []*protocol.User
	switch p.NodeInfo.Type {
	case "vmess":
		users = buildVmessUsers(p.Tag, p.Users)
	case "vless":
		users = buildVlessUsers(p.Tag, p.Users, p.VAllss.Flow)
	case "trojan":
		users = buildTrojanUsers(p.Tag, p.Users)
	case "shadowsocks":
		ivCheck := true
		if value, ok := c.ivCheck.Load(p.Tag); ok {
			ivCheck = value.(bool)
		}
		users = buildSSUsers(p.Tag,
			p.Users,
			p.Shadowsocks.Cipher,
			p.Shadowsocks.ServerKey,
			ivCheck)
	default:
		return 0, fmt.Errorf("unsupported node type: %s", p.NodeInfo.Type)
	}
	man, err := c.GetUserManager(p.Tag)
	if err != nil {
		return 0, fmt.Errorf("get user manager error: %s", err)
	}
	memory := make([]*protocol.MemoryUser, 0, len(users))
	for _, u := range users {
		mUser, err := u.ToMemoryUser()
		if err != nil {
			return 0, err
		}
		memory = append(memory, mUser)
	}
	var inserted []string
	for _, u := range memory {
		if existing := man.GetUser(context.Background(), u.Email); existing != nil {
			continue
		}
		err = man.AddUser(context.Background(), u)
		if err != nil {
			for _, email := range inserted {
				_ = man.RemoveUser(context.Background(), email)
			}
			return 0, err
		}
		inserted = append(inserted, u.Email)
	}
	for _, u := range p.Users {
		c.users.uidMap[format.UserTag(p.Tag, u.Uuid)] = u.Id
	}
	return len(users), nil
}
