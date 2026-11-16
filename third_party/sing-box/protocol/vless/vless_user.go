package vless

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/v2bxusers"
	"github.com/sagernet/sing/common"
)

func (h *Inbound) AddUsers(users []option.VLESSUser) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.VLESSUser) string { return user.Name })
	h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VLESSUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.Flow
		}),
	)
	return nil
}
func (h *Inbound) DelUsers(names []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, names, func(user option.VLESSUser) string { return user.Name })
	h.service.UpdateUsers(
		common.MapIndexed(h.users, func(index int, it option.VLESSUser) int {
			return index
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.UUID
		}),
		common.Map(h.users, func(it option.VLESSUser) string {
			return it.Flow
		}),
	)
	return nil
}
