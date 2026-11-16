package hysteria

import "github.com/sagernet/sing-box/option"
import "github.com/sagernet/sing-box/protocol/v2bxusers"

func (h *Inbound) AddUsers(users []option.HysteriaUser) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.HysteriaUser) string { return user.Name })
	h.updateUsers()
	return nil
}

func (h *Inbound) DelUsers(names []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, names, func(user option.HysteriaUser) string { return user.Name })
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
		if user.AuthString != "" {
			userPasswords[i] = user.AuthString
		} else {
			userPasswords[i] = string(user.Auth)
		}
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
