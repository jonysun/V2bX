package v2bxusers

import "testing"

type testUser struct {
	name     string
	password string
}

func TestUpsertAndRemove(t *testing.T) {
	name := func(user testUser) string { return user.name }
	users := Upsert([]testUser{{name: "a"}, {name: "b", password: "old"}}, []testUser{{name: "b", password: "new"}, {name: "c"}}, name)
	if len(users) != 3 || users[1].password != "new" || users[2].name != "c" {
		t.Fatalf("unexpected upsert result: %#v", users)
	}
	users = Remove(users, []string{"a", "c"}, name)
	if len(users) != 1 || users[0].name != "b" || users[0].password != "new" {
		t.Fatalf("unexpected remove result: %#v", users)
	}
}
