package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/auth"
)

// lockedProbe sends one request for every registered route, as user, through a
// table with the same patterns and access levels as the real one but handlers
// that only answer 204. It returns the status and the error message each pattern
// gave.
func lockedProbe(t *testing.T, user *auth.User) map[string]struct {
	status int
	body   string
} {
	t.Helper()
	real := (&Server{}).protectedRoutes()
	table := newRouteTable()
	for pattern, level := range real.access {
		table.group(level).HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}
	out := map[string]struct {
		status int
		body   string
	}{}
	for pattern := range real.access {
		method, path, _ := strings.Cut(pattern, " ")
		for strings.Contains(path, "{") {
			start := strings.Index(path, "{")
			end := strings.Index(path, "}")
			path = path[:start] + "1" + path[end+1:]
		}
		req := httptest.NewRequest(method, path, nil)
		req = req.WithContext(auth.WithUser(req.Context(), user))
		if _, got := table.mux.Handler(req); got != pattern {
			t.Fatalf("%s resolves to %q, not itself", pattern, got)
		}
		rec := httptest.NewRecorder()
		table.ServeHTTP(rec, req)
		var msg struct{ Error string }
		_ = json.Unmarshal(rec.Body.Bytes(), &msg)
		out[pattern] = struct {
			status int
			body   string
		}{rec.Code, msg.Error}
	}
	return out
}

func TestLockedAccountReachesOnlyTheAllowlist(t *testing.T) {
	got := lockedProbe(t, &auth.User{ID: 2, Username: "sam", SubscriptionLocked: true})
	for pattern := range lockedAllowed {
		if _, ok := got[pattern]; !ok {
			t.Errorf("lockedAllowed has %q, which is not a registered route", pattern)
		}
	}
	table := (&Server{}).protectedRoutes()
	for pattern, res := range got {
		lockedOut := res.status == http.StatusForbidden && strings.Contains(res.body, lockedMessage)
		switch {
		case lockedAllowed[pattern] && lockedOut:
			t.Errorf("%s is allowed for a locked account but was refused", pattern)
		case !lockedAllowed[pattern] && table.access[pattern] == accessAdmin && res.status != http.StatusForbidden:
			// An administrator route is refused on its own, before the lock is looked at.
			t.Errorf("%s answered %d for a locked member, want 403", pattern, res.status)
		case !lockedAllowed[pattern] && table.access[pattern] == accessMember && !lockedOut:
			t.Errorf("%s answered %d %s for a locked account, want 403 %q", pattern, res.status, res.body, lockedMessage)
		}
	}
}

func TestLockedAdministratorIsNotLocked(t *testing.T) {
	got := lockedProbe(t, &auth.User{ID: 1, Username: "ryan", IsAdmin: true, SubscriptionLocked: true})
	for pattern, res := range got {
		if strings.Contains(res.body, lockedMessage) {
			t.Errorf("%s locked an administrator out", pattern)
		}
	}
}

func TestUnlockedMemberIsUnaffected(t *testing.T) {
	got := lockedProbe(t, &auth.User{ID: 2, Username: "sam"})
	for pattern, res := range got {
		if strings.Contains(res.body, lockedMessage) {
			t.Errorf("%s locked an account with no lock", pattern)
		}
	}
}
