package tuic

import (
	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/v2bxusers"
	E "github.com/sagernet/sing/common/exceptions"
)

func (h *Inbound) AddUsers(users []option.TUICUser, _ []int) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Upsert(h.users, users, func(user option.TUICUser) string { return user.Name })
	return h.updateUsers()
}

func (h *Inbound) updateUsers() error {
	userIDs := make([]int, len(h.users))
	userNames := make([]string, len(h.users))
	userUUIDs := make([][16]byte, len(h.users))
	userPasswords := make([]string, len(h.users))
	for i, user := range h.users {
		userIDs[i] = i
		userNames[i] = user.Name
		userPasswords[i] = user.Password
		userUUID, err := uuid.FromString(user.UUID)
		if err != nil {
			return E.Cause(err, "invalid uuid for user ", i)
		}
		userUUIDs[i] = userUUID
	}
	h.server.UpdateUsers(userIDs, userUUIDs, userPasswords)
	h.userNameList = userNames
	return nil
}

func (h *Inbound) DelUsers(names []string) error {
	h.usersAccess.Lock()
	defer h.usersAccess.Unlock()
	h.users = v2bxusers.Remove(h.users, names, func(user option.TUICUser) string { return user.Name })
	return h.updateUsers()
}

func (h *Inbound) userName(userID int) string {
	h.usersAccess.RLock()
	defer h.usersAccess.RUnlock()
	if userID < 0 || userID >= len(h.userNameList) {
		return ""
	}
	return h.userNameList[userID]
}
