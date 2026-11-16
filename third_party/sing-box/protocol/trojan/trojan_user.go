package trojan

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/v2bxusers"
	"github.com/sagernet/sing/common"
)

func (h *Inbound) AddUsers(users []option.TrojanUser) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.TrojanUser) string { return user.Name })
	err := h.service.UpdateUsers(common.MapIndexed(h.users, func(index int, user option.TrojanUser) int {
		return index
	}), common.Map(h.users, func(user option.TrojanUser) string {
		return user.Password
	}))
	return err
}
func (h *Inbound) DelUsers(names []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, names, func(user option.TrojanUser) string { return user.Name })
	err := h.service.UpdateUsers(common.MapIndexed(h.users, func(index int, user option.TrojanUser) int {
		return index
	}), common.Map(h.users, func(user option.TrojanUser) string {
		return user.Password
	}))
	return err
}
