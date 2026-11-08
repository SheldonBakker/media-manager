package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestForgottenPasswordIsResetWhereThePersonSignsIn(t *testing.T) {
	e, f := newSignUpEnv(t)
	sendCode(e, signUpBody["email"])
	e.must(e.client(), "POST", "/api/auth/signup", signUpBody, http.StatusOK)

	e.must(e.client(), "POST", "/api/auth/password-reset/code", map[string]string{"email": " SAM.SIGNUP@example.com "}, http.StatusAccepted)
	if f.codes["reset:sam.signup@example.com"] == "" {
		t.Fatal("the account service was not asked for a reset code")
	}
	wrong := map[string]string{"email": signUpBody["email"], "code": "111111", "password": "a brand new passphrase"}
	out := e.must(e.client(), "POST", "/api/auth/password-reset", wrong, http.StatusBadRequest)
	if msg, _ := out["error"].(string); !strings.Contains(msg, "wrong or has expired") {
		t.Fatalf("error = %q", msg)
	}
	right := map[string]string{"email": signUpBody["email"], "code": " 654321 ", "password": "a brand new passphrase"}
	e.must(e.client(), "POST", "/api/auth/password-reset", right, http.StatusNoContent)

	if _, status, _ := e.login("sam.signup", signUpBody["password"]); status != http.StatusUnauthorized {
		t.Fatalf("old password: status %d, want 401", status)
	}
	if _, status, out := e.login("sam.signup", "a brand new passphrase"); status != http.StatusOK {
		t.Fatalf("new password: status %d: %v", status, out["_raw"])
	}
}

func TestPasswordResetForAnUnknownAddressAnswersTheSame(t *testing.T) {
	e, f := newSignUpEnv(t)
	e.must(e.client(), "POST", "/api/auth/password-reset/code", map[string]string{"email": "nobody@example.com"}, http.StatusAccepted)
	if len(f.codes) != 0 {
		t.Fatalf("codes = %v, want none", f.codes)
	}
}

func TestPasswordResetIsOffWithoutTheAccountService(t *testing.T) {
	e := newSignInEnv(t, nil)
	e.must(e.client(), "POST", "/api/auth/password-reset/code", map[string]string{"email": "sam@example.com"}, http.StatusNotFound)
	e.must(e.client(), "POST", "/api/auth/password-reset", map[string]string{"email": "sam@example.com"}, http.StatusNotFound)
	if out := e.must(e.client(), "GET", "/api/auth/methods", nil, http.StatusOK); out["passwordReset"] != false {
		t.Fatalf("passwordReset = %v, want false", out["passwordReset"])
	}
	on, _ := newSignUpEnv(t)
	if out := on.must(on.client(), "GET", "/api/auth/methods", nil, http.StatusOK); out["passwordReset"] != true {
		t.Fatalf("passwordReset = %v, want true", out["passwordReset"])
	}
}

func TestPasswordResetsAreLimitedPerAddress(t *testing.T) {
	e, _ := newSignUpEnv(t)
	last := 0
	for i := 0; i <= 10; i++ {
		last, _ = e.call(e.client(), "POST", "/api/auth/password-reset/code", map[string]string{"email": "nobody@example.com"})
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("eleventh reset from one address: status %d, want 429", last)
	}
}
