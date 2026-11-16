package hysteria2

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/v2bxusers"
)

func (h *Inbound) AddUsers(users []option.Hysteria2User, _ []int) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.Hysteria2User) string { return user.Name })
	h.updateUsers()
	return nil
}

func (h *Inbound) DelUsers(names []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, names, func(user option.Hysteria2User) string { return user.Name })
	h.updateUsers()
	return nil
}

func (h *Inbound) updateUsers() {
	userIDs := make([]int, len(h.users))
	h.userNameList = make([]string, len(h.users))
	userPasswords := make([]string, len(h.users))
	for i, user := range h.users {
		userIDs[i] = i
		h.userNameList[i] = user.Name
		userPasswords[i] = user.Password
	}
	h.service.UpdateUsers(userIDs, userPasswords)
}

func (h *Inbound) userName(userID int) string {
	h.usersAccess.RLock()
	defer h.usersAccess.RUnlock()
	if userID < 0 || userID >= len(h.userNameList) {
		return ""
	}
	return h.userNameList[userID]
}
