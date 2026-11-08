package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/rdborg/mediarium/internal/auth"
	"github.com/rdborg/mediarium/internal/config"
)

const (
	provisionPath     = "/api/provisioning/directory-accounts"
	provisioningToken = "a-provisioning-token-long-enough-to-use"
)

// newProvisioningEnv is a signed-in server with the provisioning token set.
func newProvisioningEnv(t *testing.T) *signInEnv {
	t.Helper()
	return newSignInEnv(t, func(c *config.Config) { c.ProvisioningToken = provisioningToken })
}

// bearer is the header pair that carries token.
func bearer(token string) []string { return []string{"Authorization", "Bearer " + token} }

func TestProvisioningIsOffWithoutAToken(t *testing.T) {
	e := newSignInEnv(t, nil)
	body := map[string]string{"subject": "uuid-sam", "username": "sam"}
	for name, try := range map[string]struct {
		c       *http.Client
		headers []string
	}{
		"no token":                   {e.client(), nil},
		"a token":                    {e.client(), bearer(provisioningToken)},
		"an empty token":             {e.client(), []string{"Authorization", "Bearer "}},
		"an administrator's session": {e.admin, nil},
	} {
		if status, out := e.call(try.c, "POST", provisionPath, body, try.headers...); status != http.StatusNotFound {
			t.Errorf("%s: %d %v, want 404", name, status, out["_raw"])
		}
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "1" {
		t.Errorf("accounts = %s, want only the administrator", n)
	}
}

// Only the provisioning token opens the address: a session or an API key of
// any account, even an administrator's, does not.
func TestProvisioningNeedsTheToken(t *testing.T) {
	e := newProvisioningEnv(t)
	key := e.must(e.admin, "POST", "/api/auth/api-keys", map[string]string{"name": "script"}, http.StatusCreated)["key"].(string)
	body := map[string]string{"subject": "uuid-sam", "username": "sam"}
	for name, try := range map[string]struct {
		c       *http.Client
		headers []string
	}{
		"no token":                     {e.client(), nil},
		"a wrong token":                {e.client(), bearer(provisioningToken + "x")},
		"part of the token":            {e.client(), bearer(provisioningToken[:len(provisioningToken)-1])},
		"the token without Bearer":     {e.client(), []string{"Authorization", provisioningToken}},
		"the token as an API key":      {e.client(), []string{"X-API-Key", provisioningToken}},
		"an administrator's session":   {e.admin, nil},
		"an administrator's API key":   {e.client(), []string{"X-API-Key", key}},
		"an API key as a bearer token": {e.client(), bearer(key)},
	} {
		if status, out := e.call(try.c, "POST", provisionPath, body, try.headers...); status != http.StatusUnauthorized {
			t.Errorf("%s: %d %v, want 401", name, status, out["_raw"])
		}
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "1" {
		t.Errorf("accounts = %s, want only the administrator", n)
	}
}

// The token is for this one address: it signs nobody in anywhere else.
func TestProvisioningTokenOpensNothingElse(t *testing.T) {
	e := newProvisioningEnv(t)
	for _, path := range []string{"/api/auth/me", "/api/users", "/api/metrics"} {
		for _, headers := range [][]string{bearer(provisioningToken), {"X-API-Key", provisioningToken}} {
			if status, _ := e.call(e.client(), "GET", path, nil, headers...); status != http.StatusUnauthorized {
				t.Errorf("GET %s with %s: %d, want 401", path, headers[0], status)
			}
		}
	}
	newUser := map[string]string{"username": "kim", "password": "long-enough-password", "role": "admin"}
	if status, _ := e.call(e.client(), "POST", "/api/users", newUser, bearer(provisioningToken)...); status != http.StatusUnauthorized {
		t.Errorf("POST /api/users with the token: %d, want 401", status)
	}
}

func TestProvisioningMakesADirectoryAccount(t *testing.T) {
	e := newProvisioningEnv(t)
	body := map[string]any{"subject": "  uuid-sam  ", "username": "sam", "name": "Sam Example", "email": "sam@example.com", "role": "admin"}
	out := e.must(e.client(), "POST", provisionPath, body, http.StatusCreated, bearer(provisioningToken)...)
	id, _ := out["id"].(float64)
	if id == 0 || out["username"] != "sam" || out["role"] != "member" {
		t.Fatalf("answer = %v", out["_raw"])
	}

	u, err := e.server.Auth.UserByIdentity(auth.SourceLDAP, "uuid-sam")
	if err != nil {
		t.Fatalf("the directory sign-in can't find the account: %v", err)
	}
	if u.ID != int64(id) || u.Username != "sam" || u.Source != auth.SourceLDAP || u.Role() != auth.RoleMember {
		t.Errorf("account = %+v", u)
	}
	if u.Name() != "Sam Example" || u.Email != "sam@example.com" {
		t.Errorf("name %q, email %q", u.Name(), u.Email)
	}
	// No password signs it in: there is none.
	if hash := e.scalar(`SELECT password_hash FROM users WHERE id = ?`, int64(id)); hash != "" {
		t.Errorf("a password hash was stored: %q", hash)
	}
	if _, status, _ := e.login("sam", "any-password-at-all"); status == http.StatusOK {
		t.Error("a password signed the account in")
	}

	lines := activityLines(t, e.admin, e.base)
	if !strings.Contains(lines, `Account "sam" was created as a member for a directory sign-in (by provisioning`) {
		t.Errorf("the Activity feed lacks the new account:\n%s", lines)
	}
	if strings.Contains(lines, provisioningToken) {
		t.Error("the token reached the Activity feed")
	}
}

// Asking again for the same person answers with their account as it is now:
// nothing about it is changed, and no second account is made.
func TestProvisioningTheSamePersonAgainReturnsTheirAccount(t *testing.T) {
	e := newProvisioningEnv(t)
	first := e.must(e.client(), "POST", provisionPath, map[string]string{"subject": "uuid-sam", "username": "sam", "email": "sam@example.com"}, http.StatusCreated, bearer(provisioningToken)...)
	id := int64(first["id"].(float64))
	e.must(e.admin, "PUT", "/api/users/"+itoa(id), map[string]string{"role": "viewer"}, http.StatusOK)

	again := e.must(e.client(), "POST", provisionPath, map[string]string{"subject": "uuid-sam", "username": "samuel", "name": "Someone Else", "email": "else@example.com"}, http.StatusOK, bearer(provisioningToken)...)
	if again["id"] != first["id"] || again["username"] != "sam" || again["role"] != "viewer" {
		t.Errorf("second answer = %v, first = %v", again["_raw"], first["_raw"])
	}
	u, err := e.server.Auth.GetUser(id)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "sam" || u.Role() != auth.RoleViewer || u.Email != "sam@example.com" || u.Name() != "" {
		t.Errorf("the account changed: %+v", u)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "2" {
		t.Errorf("accounts = %s, want 2", n)
	}
}

// Calls for the same person that arrive together make one account, and every
// one of them gets it back.
func TestProvisioningCallsAtOnceMakeOneAccount(t *testing.T) {
	e := newProvisioningEnv(t)
	const calls = 8
	body, _ := json.Marshal(map[string]string{"subject": "uuid-sam", "username": "sam"})
	statuses, ids := make([]int, calls), make([]float64, calls)
	errs := make([]error, calls)
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", e.base+provisionPath, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+provisioningToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs[i] = err
				return
			}
			defer resp.Body.Close()
			var out struct {
				ID float64 `json:"id"`
			}
			errs[i] = json.NewDecoder(resp.Body).Decode(&out)
			statuses[i], ids[i] = resp.StatusCode, out.ID
		}()
	}
	wg.Wait()
	created := 0
	for i := range calls {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		switch statuses[i] {
		case http.StatusCreated:
			created++
		case http.StatusOK:
		default:
			t.Errorf("call %d: status %d", i, statuses[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("call %d got account %v, call 0 got %v", i, ids[i], ids[0])
		}
	}
	if created != 1 {
		t.Errorf("%d calls made an account, want 1", created)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "2" {
		t.Errorf("accounts = %s, want 2", n)
	}
}

// A user name another account has is an error: never a renamed account, and
// never the other account.
func TestProvisioningNeverTakesAUsernameInUse(t *testing.T) {
	e := newProvisioningEnv(t)
	kim := e.addAccount("kim", "member")
	for _, name := range []string{"kim", "KIM"} {
		status, out := e.call(e.client(), "POST", provisionPath, map[string]string{"subject": "uuid-kim", "username": name}, bearer(provisioningToken)...)
		if status != http.StatusConflict || out["error"] != "That username is already taken." {
			t.Errorf("%s: %d %v, want 409", name, status, out["_raw"])
		}
	}
	if _, err := e.server.Auth.UserByIdentity(auth.SourceLDAP, "uuid-kim"); !errors.Is(err, auth.ErrNoIdentity) {
		t.Errorf("the directory id was linked anyway: %v", err)
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "2" {
		t.Errorf("accounts = %s, want 2", n)
	}
	if src := e.scalar(`SELECT auth_source FROM users WHERE id = ?`, kim); src != auth.SourceLocal {
		t.Errorf("kim's account now signs in through %q", src)
	}
}

func TestProvisioningChecksWhatItIsSent(t *testing.T) {
	e := newProvisioningEnv(t)
	for name, body := range map[string]any{
		"not an object":               "subject=uuid-sam",
		"no subject":                  map[string]string{"username": "sam"},
		"a blank subject":             map[string]string{"subject": "   ", "username": "sam"},
		"a subject that's too long":   map[string]string{"subject": strings.Repeat("s", 256), "username": "sam"},
		"no username":                 map[string]string{"subject": "uuid-sam"},
		"a username that's too short": map[string]string{"subject": "uuid-sam", "username": "sa"},
		"a username with a space":     map[string]string{"subject": "uuid-sam", "username": "sam example"},
		"an email as the username":    map[string]string{"subject": "uuid-sam", "username": "sam@example.com"},
		"an email that's wrong":       map[string]string{"subject": "uuid-sam", "username": "sam", "email": "sam-at-example"},
		"a name that's too long":      map[string]string{"subject": "uuid-sam", "username": "sam", "name": strings.Repeat("n", 101)},
	} {
		status, out := e.call(e.client(), "POST", provisionPath, body, bearer(provisioningToken)...)
		if msg, _ := out["error"].(string); status != http.StatusBadRequest || msg == "" {
			t.Errorf("%s: %d %v, want 400 with a message", name, status, out["_raw"])
		}
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "1" {
		t.Errorf("accounts = %s, want only the administrator", n)
	}
	// The longest subject still fits.
	e.must(e.client(), "POST", provisionPath, map[string]string{"subject": strings.Repeat("s", 255), "username": "sam"}, http.StatusCreated, bearer(provisioningToken)...)
}

// The point of it all: the person's first directory sign-in lands in the
// account made for them, even where the directory makes no accounts itself.
func TestProvisionedAccountIsTheOneTheDirectorySignInFinds(t *testing.T) {
	e := &ldapEnv{signInEnv: newProvisioningEnv(t), dir: newFakeLDAP()}
	e.server.TestSetLDAPDial(e.dir.dial)
	e.saveConfig(map[string]any{"autoCreate": false})
	made := e.must(e.client(), "POST", provisionPath, map[string]string{"subject": "uuid-sam", "username": "sam"}, http.StatusCreated, bearer(provisioningToken)...)

	sam, status, out := e.login("sam", "dir-password")
	if status != http.StatusOK {
		t.Fatalf("sign-in: %d %v", status, out["_raw"])
	}
	me := e.must(sam, "GET", "/api/auth/me", nil, http.StatusOK)
	if me["id"] != made["id"] || me["username"] != "sam" || me["source"] != "ldap" {
		t.Errorf("signed in as %v, made %v", me, made["_raw"])
	}
	if n := e.scalar(`SELECT COUNT(*) FROM users`); n != "2" {
		t.Errorf("accounts = %s, want 2", n)
	}
}
