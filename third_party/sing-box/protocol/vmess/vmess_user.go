package vmess

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/v2bxusers"
	"github.com/sagernet/sing/common"
)

func (h *Inbound) AddUsers(users []option.VMessUser) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.VMessUser) string { return user.Name })
	err := h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VMessUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VMessUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VMessUser) int {
			return it.AlterId
		}))
	if err != nil {
		return err
	}
	return nil
}
func (h *Inbound) DelUsers(uuids []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, uuids, func(user option.VMessUser) string { return user.Name })
	err := h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VMessUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VMessUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VMessUser) int {
			return it.AlterId
		}))
	if err != nil {
		return err
	}
	return nil
}
