package anytls

import (
	anytls "github.com/anytls/sing-anytls"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/v2bxusers"
)

func (h *Inbound) AddUsers(users []option.AnyTLSUser) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.AnyTLSUser) string { return user.Name })
	h.updateUsers()
	return nil
}

func (h *Inbound) DelUsers(names []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, names, func(user option.AnyTLSUser) string { return user.Name })
	h.updateUsers()
	return nil
}

func (h *Inbound) updateUsers() {
	users := make([]anytls.User, len(h.users))
	for i, user := range h.users {
		users[i] = anytls.User(user)
	}
	h.service.UpdateUsers(users)
}
