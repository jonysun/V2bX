package node

import (
	"github.com/InazumaV/V2bX/api/panel"
)

func compareUserList(old, new []panel.UserInfo) (deleted, added []panel.UserInfo) {
	oldMap := make(map[string]panel.UserInfo, len(old))
	for _, user := range old {
		oldMap[user.Uuid] = user
	}

	for _, user := range new {
		previous, exists := oldMap[user.Uuid]
		if !exists {
			added = append(added, user)
			continue
		}
		if previous != user {
			deleted = append(deleted, previous)
			added = append(added, user)
		}
		delete(oldMap, user.Uuid)
	}

	for _, user := range oldMap {
		deleted = append(deleted, user)
	}

	return deleted, added
}
