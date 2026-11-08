package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/config"
)

// fakeAccountService plays the account service people sign up with. Like the
// real one, a sign-up puts the person in the directory and makes their
// Mediarium account through the provisioning route, linked to their directory
// id.
type fakeAccountService struct {
	t      *testing.T
	mu     sync.Mutex
	emails map[string]bool
	codes  map[string]string // the code sent to each address
	// codeAnswer, when set, is how the code route answers instead of sending.
	codeAnswer int
	dir        *fakeLDAP
	mediarium  string // the Mediarium under test, for provisioning
	// pending makes a sign-up answer 202 before the Mediarium account exists.
	pending bool
}

func (f *fakeAccountService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	if r.Method+" "+r.URL.Path == "POST /api/v1/auth/password-resets" {
		var in struct{ Email string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		if f.emails[strings.ToLower(in.Email)] {
			f.codes["reset:"+strings.ToLower(in.Email)] = "654321"
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if r.Method+" "+r.URL.Path == "POST /api/v1/auth/password-resets/confirm" {
		var in struct{ Email, Code, Password string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		sent := f.codes["reset:"+strings.ToLower(in.Email)]
		f.mu.Unlock()
		if sent == "" || in.Code != sent {
			reply(http.StatusBadRequest, map[string]any{"code": "validation_failed", "errors": map[string][]string{
				"EmailCode.Invalid": {"The code is wrong or has expired. Ask for a new code."},
			}})
			return
		}
		f.dir.mu.Lock()
		for _, u := range f.dir.users {
			if strings.EqualFold(u.mail, in.Email) {
				u.password = in.Password
			}
		}
		f.dir.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method+" "+r.URL.Path == "POST /api/v1/auth/email-codes" {
		var in struct{ Email string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case f.codeAnswer == http.StatusTooManyRequests:
			reply(http.StatusTooManyRequests, map[string]any{"code": "SendEmailCode.TooSoon", "detail": "A code was sent a moment ago. Wait a minute before asking for another."})
		case f.emails[strings.ToLower(in.Email)]:
			reply(http.StatusConflict, map[string]any{"code": "SendEmailCode.EmailAlreadyRegistered"})
		default:
			f.codes[strings.ToLower(in.Email)] = "123456"
			w.WriteHeader(http.StatusAccepted)
		}
		return
	}
	if r.Method+" "+r.URL.Path != "POST /api/v1/auth/register" {
		http.NotFound(w, r)
		return
	}
	var in struct{ Email, Password, PhoneNumber, FirstName, LastName, EmailCode string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	sent := f.codes[strings.ToLower(in.Email)]
	f.mu.Unlock()
	if sent == "" || in.EmailCode != sent {
		reply(http.StatusBadRequest, map[string]any{"code": "validation_failed", "errors": map[string][]string{
			"Register.InvalidEmailCode": {"The confirmation code is wrong or has expired. Ask for a new code."},
		}})
		return
	}
	if len(in.Password) < 12 {
		reply(http.StatusBadRequest, map[string]any{"code": "validation_failed", "errors": map[string][]string{
			"Password":    {"The length of 'Password' must be at least 12 characters."},
			"PhoneNumber": {"Phone number must use international format, for example +27821234567."},
		}})
		return
	}
	f.mu.Lock()
	taken := f.emails[strings.ToLower(in.Email)]
	f.emails[strings.ToLower(in.Email)] = true
	pending := f.pending
	f.mu.Unlock()
	if taken {
		reply(http.StatusConflict, map[string]any{"code": "Register.EmailAlreadyRegistered"})
		return
	}
	username := strings.ToLower(in.FirstName + "." + in.LastName)
	f.dir.mu.Lock()
	f.dir.users = append(f.dir.users, &dirUser{dn: "uid=" + username + ",ou=people,dc=example,dc=com", uid: username,
		cn: in.FirstName + " " + in.LastName, mail: in.Email, uuid: "uid-" + username, password: in.Password})
	f.dir.mu.Unlock()
	status, ready := http.StatusAccepted, false
	if !pending {
		f.provision(username, in.FirstName+" "+in.LastName, in.Email)
		status, ready = http.StatusCreated, true
	}
	reply(status, map[string]any{
		"id": "0193", "email": in.Email, "firstName": in.FirstName, "lastName": in.LastName,
		"provisioning": map[string]any{"username": username, "status": map[bool]string{true: "completed", false: "pending"}[ready],
			"directoryReady": true, "mediaLibraryReady": ready, "mediaServerReady": ready},
		"tokenType": "Bearer", "accessToken": "not-used-by-mediarium", "expiresIn": 3600,
	})
}

// provision makes the person's Mediarium account the way the real service does.
func (f *fakeAccountService) provision(username, name, email string) {
	body, _ := json.Marshal(map[string]string{"subject": "uid-" + username, "username": username, "name": name, "email": email})
	req, _ := http.NewRequest(http.MethodPost, f.mediarium+"/api/provisioning/directory-accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provisioningToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Errorf("provision %s: %v", username, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		f.t.Errorf("provision %s: status %d", username, resp.StatusCode)
	}
}

// newSignUpEnv is a Mediarium with sign-up through the fake account service and
// directory sign-in switched on, without making accounts on a first sign-in
// (as in production: the account service makes them).
func newSignUpEnv(t *testing.T) (*ldapEnv, *fakeAccountService) {
	t.Helper()
	f := &fakeAccountService{t: t, emails: map[string]bool{}, codes: map[string]string{}, dir: newFakeLDAP()}
	svc := httptest.NewServer(f)
	t.Cleanup(svc.Close)
	e := &ldapEnv{signInEnv: newSignInEnv(t, func(c *config.Config) {
		c.ProvisioningToken = provisioningToken
		c.AccountServiceURL = svc.URL
	}), dir: f.dir}
	e.server.TestSetLDAPDial(e.dir.dial)
	e.saveConfig(map[string]any{"autoCreate": false})
	f.mediarium = e.base
	return e, f
}

var signUpBody = map[string]string{
	"firstName": "Sam", "lastName": "Signup", "email": "sam.signup@example.com",
	"phoneNumber": "+27821234567", "password": "a long unique passphrase", "code": "123456",
}

// sendCode asks for the confirmation code of email, as the sign-up dialog does
// before it signs up.
func sendCode(e *ldapEnv, email string) {
	e.t.Helper()
	e.must(e.client(), "POST", "/api/auth/signup/code", map[string]string{"email": email}, http.StatusAccepted)
}

func TestSignUpCodeGoesThroughTheAccountService(t *testing.T) {
	e, f := newSignUpEnv(t)
	sendCode(e, " sam.signup@example.com ")
	if f.codes["sam.signup@example.com"] == "" {
		t.Fatal("the account service was not asked to send a code")
	}
	e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusOK)
	out := e.must(e.client(), "POST", "/api/auth/signup/code", map[string]string{"email": "sam.signup@example.com"}, http.StatusConflict)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "already") {
		t.Fatalf("error = %q", msg)
	}
	f.codeAnswer = http.StatusTooManyRequests
	out = e.must(e.client(), "POST", "/api/auth/signup/code", map[string]string{"email": "new@example.com"}, http.StatusTooManyRequests)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "Wait a minute") {
		t.Fatalf("error = %q", msg)
	}
}

func TestSignUpNeedsTheEmailedCode(t *testing.T) {
	e, _ := newSignUpEnv(t)
	e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusBadRequest)
	sendCode(e, signUpBody["email"])
	wrong := map[string]string{}
	for k, v := range signUpBody {
		wrong[k] = v
	}
	wrong["code"] = "654321"
	out := e.must(e.client(), "POST", "/api/auth/signup", wrong, http.StatusBadRequest)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "confirmation code") {
		t.Fatalf("error = %q", msg)
	}
	e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusOK)
}

func TestSignUpCodesAreLimitedPerAddress(t *testing.T) {
	e, _ := newSignUpEnv(t)
	last := 0
	for i := 0; i <= api.SignUpCodesPerAddress; i++ {
		last, _ = e.call(e.client(), "POST", "/api/auth/signup/code", map[string]string{"email": "person" + string(rune('a'+i)) + "@example.com"})
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status after %d codes from one address = %d, want 429", api.SignUpCodesPerAddress, last)
	}
}

func TestSignUpMakesABasicAccountAndSignsIn(t *testing.T) {
	e, _ := newSignUpEnv(t)
	sendCode(e, signUpBody["email"])
	c := e.client()
	out := e.must(c, "POST", "/api/auth/signup", signUpBody, http.StatusOK)
	if out["username"] != "sam.signup" || out["role"] != "member" || out["source"] != "ldap" {
		t.Fatalf("answer = %v", out["_raw"])
	}
	if user, role := e.signedInAs(c); user != "sam.signup" || role != "member" {
		t.Fatalf("signed in as %q (%s), want sam.signup (member)", user, role)
	}
	if got := e.scalar(`SELECT COUNT(*) FROM users WHERE username = 'sam.signup' AND auth_source = 'ldap'`); got != "1" {
		t.Fatalf("directory accounts named sam.signup = %s, want 1", got)
	}
	if got := e.scalar(`SELECT method FROM sessions ORDER BY rowid DESC LIMIT 1`); got != "ldap" {
		t.Fatalf("session method = %q, want ldap", got)
	}
	if _, status, out := e.login("sam.signup", signUpBody["password"]); status != http.StatusOK {
		t.Fatalf("signing in again: status %d: %v", status, out["_raw"])
	}
}

func TestSignUpStillBeingSetUpIsAcceptedWithoutSigningIn(t *testing.T) {
	e, f := newSignUpEnv(t)
	f.pending = true
	sendCode(e, signUpBody["email"])
	c := e.client()
	out := e.must(c, "POST", "/api/auth/signup", signUpBody, http.StatusAccepted)
	if out["pending"] != true || out["username"] != "sam.signup" {
		t.Fatalf("answer = %v", out["_raw"])
	}
	if user, _ := e.signedInAs(c); user != "" {
		t.Fatalf("signed in as %q while the account is still being set up", user)
	}
}

func TestSignUpPassesOnWhatTheAccountServiceRefuses(t *testing.T) {
	e, _ := newSignUpEnv(t)
	sendCode(e, signUpBody["email"])
	short := map[string]string{}
	for k, v := range signUpBody {
		short[k] = v
	}
	short["password"] = "too short"
	out := e.must(e.client(), "POST", "/api/auth/signup", short, http.StatusBadRequest)
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "at least 12 characters") || !strings.Contains(msg, "international format") {
		t.Fatalf("error = %q", msg)
	}
	e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusOK)
	out = e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusConflict)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "already") {
		t.Fatalf("error = %q", msg)
	}
}

func TestSignUpIsLimitedPerAddress(t *testing.T) {
	e, f := newSignUpEnv(t)
	f.pending = true
	statuses := []int{}
	for i := 0; i < 6; i++ {
		body := map[string]string{}
		for k, v := range signUpBody {
			body[k] = v
		}
		body["email"] = "sam" + string(rune('a'+i)) + "@example.com"
		body["lastName"] = "Signup" + string(rune('a'+i))
		f.codes[body["email"]] = "123456"
		status, _ := e.call(e.client(), "POST", "/api/auth/signup", body)
		statuses = append(statuses, status)
	}
	if statuses[4] != http.StatusAccepted || statuses[5] != http.StatusTooManyRequests {
		t.Fatalf("statuses = %v, want five accepted then 429", statuses)
	}
}

func TestSignUpIsOffWithoutTheAccountService(t *testing.T) {
	e := newSignInEnv(t, nil)
	e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusNotFound)
	e.must(e.client(), "POST", "/api/auth/signup/code", map[string]string{"email": "sam@example.com"}, http.StatusNotFound)
	if out := e.must(e.client(), "GET", "/api/auth/methods", nil, http.StatusOK); out["signup"] != false {
		t.Fatalf("signup = %v, want false", out["signup"])
	}
	on, _ := newSignUpEnv(t)
	if out := on.must(on.client(), "GET", "/api/auth/methods", nil, http.StatusOK); out["signup"] != true {
		t.Fatalf("signup = %v, want true", out["signup"])
	}
}

func TestSignUpReportsAnUnreachableAccountService(t *testing.T) {
	svc := httptest.NewServer(http.NotFoundHandler())
	url := svc.URL
	svc.Close()
	e := newSignInEnv(t, func(c *config.Config) { c.AccountServiceURL = url })
	e.must(e.client(), "POST", "/api/auth/signup/code", map[string]string{"email": "sam@example.com"}, http.StatusBadGateway)
	out := e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusBadGateway)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "couldn't be reached") {
		t.Fatalf("error = %q", msg)
	}
}
