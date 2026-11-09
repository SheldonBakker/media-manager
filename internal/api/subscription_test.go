package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rdborg/mediarium/internal/auth"
	"github.com/rdborg/mediarium/internal/config"
)

const coordinatorToken = "a-service-token-of-some-length-1234"

// seenCall is one request the stand-in Coordinator received.
type seenCall struct {
	method, path, auth, contentType, body string
}

// fakeCoordinator stands in for the Coordinator. It records what it was asked
// and answers every call with the given status and body.
func fakeCoordinator(t *testing.T, status int, body string) (url string, calls func() []seenCall) {
	t.Helper()
	var mu sync.Mutex
	var seen []seenCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, seenCall{r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []seenCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenCall(nil), seen...)
	}
}

func withCoordinator(url string) func(*config.Config) {
	return func(c *config.Config) {
		c.CoordinatorURL = url
		c.CoordinatorToken = coordinatorToken
	}
}

// directoryMember makes an account linked to a directory identity.
func (e *signInEnv) directoryMember(username, role, subject string) *http.Client {
	e.t.Helper()
	id := e.addAccount(username, role)
	if err := e.server.Auth.LinkIdentity(id, auth.SourceLDAP, subject); err != nil {
		e.t.Fatal(err)
	}
	c, status, _ := e.login(username, "long-enough-password")
	if status != http.StatusOK {
		e.t.Fatalf("sign-in: %d", status)
	}
	return c
}

func rawCall(e *signInEnv, c *http.Client, method, path, body string) (int, string) {
	e.t.Helper()
	req, err := http.NewRequest(method, e.base+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestSubscriptionForwardsWithTheSubjectAndToken(t *testing.T) {
	const answer = `{"status":"active","hasAccess":true,"complimentary":false,"paidThrough":"2026-12-01T00:00:00Z","plan":null}`
	url, calls := fakeCoordinator(t, http.StatusOK, answer)
	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "uid=sam/odd subject")

	tests := []struct{ method, path, wantPath, wantBody string }{
		{"GET", "/api/subscription", "", ""},
		{"POST", "/api/subscription/checkout", "/checkout", ""},
		{"POST", "/api/subscription/confirm", "/checkout/confirm", `{"reference":"ref_123"}`},
		{"POST", "/api/subscription/manage-link", "/manage-link", ""},
		{"POST", "/api/subscription/cancel", "/cancel", ""},
	}
	for _, tc := range tests {
		status, body := rawCall(e, sam, tc.method, tc.path, tc.wantBody)
		if status != http.StatusOK || body != answer {
			t.Errorf("%s %s: %d %q, want the Coordinator's answer unchanged", tc.method, tc.path, status, body)
		}
	}
	got := calls()
	if len(got) != len(tests) {
		t.Fatalf("the Coordinator saw %d calls, want %d: %+v", len(got), len(tests), got)
	}
	for i, tc := range tests {
		want := seenCall{tc.method, "/api/v1/members/uid=sam%2Fodd%20subject/subscription" + tc.wantPath, "Bearer " + coordinatorToken, "application/json", tc.wantBody}
		if got[i] != want {
			t.Errorf("call %d = %+v, want %+v", i, got[i], want)
		}
	}
}

func TestSubscriptionWithoutADirectoryIdentityIs404(t *testing.T) {
	url, calls := fakeCoordinator(t, http.StatusOK, `{}`)
	e := newSignInEnv(t, withCoordinator(url))
	e.addAccount("sam", "member")
	sam, _, _ := e.login("sam", "long-enough-password")
	status, body := rawCall(e, sam, "GET", "/api/subscription", "")
	if status != http.StatusNotFound || !strings.Contains(body, "This account has no subscription.") {
		t.Errorf("got %d %s", status, body)
	}
	if n := len(calls()); n != 0 {
		t.Errorf("the Coordinator was called %d times for an account with no directory identity", n)
	}
}

func TestSubscriptionWithoutCoordinatorSettingsIs404(t *testing.T) {
	e := newSignInEnv(t, nil)
	sam := e.directoryMember("sam", "member", "sam")
	for _, tc := range [][2]string{{"GET", "/api/subscription"}, {"POST", "/api/subscription/checkout"}, {"POST", "/api/subscription/cancel"}} {
		status, body := rawCall(e, sam, tc[0], tc[1], "")
		if status != http.StatusNotFound || !strings.Contains(body, "This account has no subscription.") {
			t.Errorf("%s %s: %d %s", tc[0], tc[1], status, body)
		}
	}
}

func TestSubscriptionPassesCoordinatorErrorsThrough(t *testing.T) {
	const problem = `{"type":"https://example/409","title":"Conflict","status":409,"code":"StartCheckout.AlreadyPaid"}`
	url, _ := fakeCoordinator(t, http.StatusConflict, problem)
	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "sam")
	status, body := rawCall(e, sam, "POST", "/api/subscription/checkout", "")
	if status != http.StatusConflict || body != problem {
		t.Errorf("got %d %q, want the 409 body unchanged", status, body)
	}
}

func TestCoordinatorUnreachableIsAPlainError(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(io.Discard) })

	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "sam")
	status, body := rawCall(e, sam, "GET", "/api/subscription", "")
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d %s, want 502", status, body)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(body), &out); err != nil || out["error"] != "Something went wrong on the server. An administrator can find the details in the log." {
		t.Errorf("body = %s, want the generic server-error message", body)
	}
	host := strings.TrimPrefix(url, "http://")
	for _, secret := range []string{host, coordinatorToken} {
		if strings.Contains(body, secret) || strings.Contains(logged.String(), secret) {
			t.Errorf("%q leaked into the response or the log:\n%s\n%s", secret, body, logged.String())
		}
	}
}

func TestViewerCanUseSubscriptionRoutes(t *testing.T) {
	url, calls := fakeCoordinator(t, http.StatusOK, `{"url":"https://paystack.example/manage"}`)
	e := newSignInEnv(t, withCoordinator(url))
	vera := e.directoryMember("vera", "viewer", "vera")
	for _, tc := range [][3]string{
		{"GET", "/api/subscription", ""},
		{"POST", "/api/subscription/checkout", ""},
		{"POST", "/api/subscription/confirm", `{"reference":"r"}`},
		{"POST", "/api/subscription/manage-link", ""},
		{"POST", "/api/subscription/cancel", ""},
	} {
		if status, body := rawCall(e, vera, tc[0], tc[1], tc[2]); status != http.StatusOK {
			t.Errorf("%s %s: a viewer got %d %s", tc[0], tc[1], status, body)
		}
	}
	if n := len(calls()); n != 5 {
		t.Errorf("the Coordinator saw %d calls, want 5", n)
	}
}

func TestLockedMemberCanCallAllSubscriptionRoutes(t *testing.T) {
	url, _ := fakeCoordinator(t, http.StatusOK, `{}`)
	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "sam")
	if err := e.server.TestExec(`UPDATE users SET subscription_locked = 1 WHERE username = 'sam'`); err != nil {
		t.Fatal(err)
	}
	if status, body := rawCall(e, sam, "GET", "/api/movies", ""); status != http.StatusForbidden {
		t.Fatalf("the lock is not in force: %d %s", status, body)
	}
	for _, tc := range [][3]string{
		{"GET", "/api/subscription", ""},
		{"POST", "/api/subscription/checkout", ""},
		{"POST", "/api/subscription/confirm", `{"reference":"r"}`},
		{"POST", "/api/subscription/manage-link", ""},
		{"POST", "/api/subscription/cancel", ""},
	} {
		if status, body := rawCall(e, sam, tc[0], tc[1], tc[2]); status != http.StatusOK {
			t.Errorf("%s %s: a locked member got %d %s", tc[0], tc[1], status, body)
		}
	}
}

func TestSubscriptionConfirmRefusesALongBody(t *testing.T) {
	url, calls := fakeCoordinator(t, http.StatusOK, `{}`)
	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "sam")
	status, _ := rawCall(e, sam, "POST", "/api/subscription/confirm", `{"reference":"`+strings.Repeat("x", 2000)+`"}`)
	if status != http.StatusBadRequest || len(calls()) != 0 {
		t.Errorf("a 2 KB reference: %d, %d calls", status, len(calls()))
	}
}

func TestMeSaysWhetherSubscriptionsAreAvailable(t *testing.T) {
	url, _ := fakeCoordinator(t, http.StatusOK, `{}`)
	on := newSignInEnv(t, withCoordinator(url))
	sam := on.directoryMember("sam", "member", "sam")
	on.addAccount("local", "member")
	local, _, _ := on.login("local", "long-enough-password")
	if got := on.must(sam, "GET", "/api/auth/me", nil, http.StatusOK)["subscriptionAvailable"]; got != true {
		t.Errorf("directory account with the Coordinator set: %v", got)
	}
	if got := on.must(local, "GET", "/api/auth/me", nil, http.StatusOK)["subscriptionAvailable"]; got != false {
		t.Errorf("local account: %v", got)
	}
	off := newSignInEnv(t, nil)
	sam = off.directoryMember("sam", "member", "sam")
	if got := off.must(sam, "GET", "/api/auth/me", nil, http.StatusOK)["subscriptionAvailable"]; got != false {
		t.Errorf("without Coordinator settings: %v", got)
	}
}

// captureLog sends the log to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	return &logged
}

// A Coordinator that refuses Mediarium's own token is a setup problem for an
// administrator: the member gets the plain server error, the log the status.
func TestCoordinatorRefusingTheTokenIsAServerError(t *testing.T) {
	for _, refused := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		const problem = `{"title":"Unauthorized","detail":"The service token is wrong."}`
		url, _ := fakeCoordinator(t, refused, problem)
		logged := captureLog(t)
		e := newSignInEnv(t, withCoordinator(url))
		sam := e.directoryMember("sam", "member", "sam")
		status, body := rawCall(e, sam, "POST", "/api/subscription/checkout", "")
		if status != http.StatusBadGateway || !strings.Contains(body, "Something went wrong on the server. An administrator can find the details in the log.") {
			t.Errorf("Coordinator %d: %d %s, want 502 with the generic server error", refused, status, body)
		}
		line := ""
		for l := range strings.Lines(logged.String()) {
			if strings.Contains(l, "WARN subscription:") {
				line = l
			}
		}
		if !strings.Contains(line, "status="+strconv.Itoa(refused)) || strings.Contains(line, "service token is wrong") {
			t.Errorf("Coordinator %d: the warning is %q, want the status and not the body", refused, line)
		}
	}
}

// The Coordinator not knowing the person is no subscription, in Mediarium's
// own words, never the Coordinator's.
func TestCoordinatorNotKnowingTheMemberIsNoSubscription(t *testing.T) {
	url, _ := fakeCoordinator(t, http.StatusNotFound, `{"title":"Not Found","code":"Subscription.MemberNotFound","detail":"No member with that subject."}`)
	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "sam")
	status, body := rawCall(e, sam, "GET", "/api/subscription", "")
	if status != http.StatusNotFound || body != `{"error":"This account has no subscription."}`+"\n" {
		t.Errorf("got %d %q, want 404 with Mediarium's no-subscription message", status, body)
	}
}

// An answer the Coordinator is not known to give is passed on, and noted.
func TestCoordinatorUnexpectedStatusIsLogged(t *testing.T) {
	url, _ := fakeCoordinator(t, http.StatusTeapot, `{"secret":"never logged"}`)
	logged := captureLog(t)
	e := newSignInEnv(t, withCoordinator(url))
	sam := e.directoryMember("sam", "member", "sam")
	if status, _ := rawCall(e, sam, "GET", "/api/subscription", ""); status != http.StatusTeapot {
		t.Errorf("status = %d, want the Coordinator's 418 passed on", status)
	}
	if !strings.Contains(logged.String(), "status=418") || strings.Contains(logged.String(), "never logged") {
		t.Errorf("the log lacks the status, or holds the body:\n%s", logged.String())
	}
}
