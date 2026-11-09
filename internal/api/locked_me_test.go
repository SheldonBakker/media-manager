package api_test

import (
	"net/http"
	"testing"
)

func TestMeReportsTheSubscriptionLock(t *testing.T) {
	server, base, admin, member, memberID := familyServer(t)

	me := getJSON[map[string]any](t, member, base+"/api/auth/me")
	if me["subscriptionLocked"] != false {
		t.Fatalf("an unlocked member should report subscriptionLocked false: %+v", me)
	}
	if err := server.TestExec(`UPDATE users SET subscription_locked = 1 WHERE id = ?`, int64(memberID)); err != nil {
		t.Fatalf("lock the account: %v", err)
	}
	me = getJSON[map[string]any](t, member, base+"/api/auth/me")
	if me["subscriptionLocked"] != true {
		t.Fatalf("a locked member should report subscriptionLocked true: %+v", me)
	}
	if status, _ := doStatus(t, member, http.MethodGet, base+"/api/movies"); status != http.StatusForbidden {
		t.Fatalf("a locked member reached /api/movies: %d", status)
	}
	if got := getJSON[map[string]any](t, admin, base+"/api/auth/me"); got["subscriptionLocked"] != false {
		t.Fatalf("an administrator is never locked: %+v", got)
	}
}
