package accountservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var person = SignUp{FirstName: "Sam", LastName: "Signup", Email: "sam@example.com", PhoneNumber: "+27821234567", Password: "a long unique passphrase", EmailCode: "123456"}

func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestRegisterSendsThePersonAndReadsTheirAccount(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusAccepted} {
		c := serve(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/register" {
				t.Errorf("request = %s %s", r.Method, r.URL.Path)
			}
			if r.ContentLength <= 0 || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("body length %d, content type %q", r.ContentLength, r.Header.Get("Content-Type"))
			}
			var got map[string]string
			_ = json.NewDecoder(r.Body).Decode(&got)
			want := map[string]string{"firstName": "Sam", "lastName": "Signup", "email": "sam@example.com",
				"phoneNumber": "+27821234567", "password": "a long unique passphrase", "emailCode": "123456"}
			for k, v := range want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"email":"sam@example.com","provisioning":{"username":"sam.signup","status":"pending","mediaLibraryReady":true,"directorySubject":"uid-42"},"accessToken":"x"}`))
		})
		got, err := c.Register(context.Background(), person)
		if err != nil {
			t.Fatalf("status %d: %v", status, err)
		}
		if got != (Account{Username: "sam.signup", MediaLibraryReady: true, DirectorySubject: "uid-42"}) {
			t.Fatalf("status %d: account = %+v", status, got)
		}
	}
}

func TestRegisterWithoutProvisioningHasNoUsername(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"email":"sam@example.com","provisioning":null}`))
	})
	got, err := c.Register(context.Background(), person)
	if err != nil || got != (Account{}) {
		t.Fatalf("account = %+v, err = %v", got, err)
	}
}

func TestRegisterTranslatesRefusals(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"email taken", http.StatusConflict, `{"code":"Register.EmailAlreadyRegistered"}`, func(err error) bool { return errors.Is(err, ErrEmailTaken) }},
		{"details refused", http.StatusBadRequest, `{"errors":{"Password":["Too short."],"Email":["Not an email."]}}`, func(err error) bool {
			var rejected *RejectedError
			return errors.As(err, &rejected) && strings.Join(rejected.Problems, " ") == "Not an email. Too short."
		}},
		{"refused without details", http.StatusBadRequest, `{}`, func(err error) bool {
			var rejected *RejectedError
			return errors.As(err, &rejected) && len(rejected.Problems) == 0
		}},
		{"server error", http.StatusInternalServerError, `{}`, func(err error) bool { return errors.Is(err, ErrUnavailable) }},
		{"unreadable answer", http.StatusCreated, `not json`, func(err error) bool { return errors.Is(err, ErrUnavailable) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := serve(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.Register(context.Background(), person)
			if !tc.check(err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRegisterUnreachableIsUnavailableWithoutThePassword(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := New(url).Register(context.Background(), person)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), person.Password) {
		t.Fatalf("the password is in the error: %v", err)
	}
}

func TestSendCodeAsksTheServiceToEmailACode(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/auth/email-codes" || r.ContentLength <= 0 {
			t.Errorf("request = %s %s (%d bytes)", r.Method, r.URL.Path, r.ContentLength)
		}
		var got map[string]string
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got["email"] != "sam@example.com" {
			t.Errorf("email = %q", got["email"])
		}
		w.WriteHeader(http.StatusAccepted)
	})
	if err := c.SendCode(context.Background(), "sam@example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestSendCodeTranslatesRefusals(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"email taken", http.StatusConflict, `{}`, func(err error) bool { return errors.Is(err, ErrEmailTaken) }},
		{"too often", http.StatusTooManyRequests, `{"detail":"Wait a minute."}`, func(err error) bool {
			var limited *TooManyCodesError
			return errors.As(err, &limited) && limited.Message == "Wait a minute."
		}},
		{"bad address", http.StatusBadRequest, `{"errors":{"Email":["Not an email."]}}`, func(err error) bool {
			var rejected *RejectedError
			return errors.As(err, &rejected) && strings.Join(rejected.Problems, " ") == "Not an email."
		}},
		{"server error", http.StatusInternalServerError, `{}`, func(err error) bool { return errors.Is(err, ErrUnavailable) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := serve(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			if err := c.SendCode(context.Background(), "sam@example.com"); !tc.check(err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestPasswordResetCallsTheService(t *testing.T) {
	var paths []string
	var bodies []map[string]string
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var got map[string]string
		_ = json.NewDecoder(r.Body).Decode(&got)
		bodies = append(bodies, got)
		if r.URL.Path == "/api/v1/auth/password-resets" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.RequestPasswordReset(context.Background(), "sam@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := c.ResetPassword(context.Background(), "sam@example.com", "123456", "a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, " ") != "/api/v1/auth/password-resets /api/v1/auth/password-resets/confirm" {
		t.Fatalf("paths = %v", paths)
	}
	if bodies[1]["code"] != "123456" || bodies[1]["password"] != "a brand new passphrase" || bodies[1]["email"] != "sam@example.com" {
		t.Fatalf("confirm body = %v", bodies[1])
	}
}

func TestPasswordResetTranslatesRefusals(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusBadRequest, `{"errors":{"EmailCode.Invalid":["The code is wrong."]}}`, func(err error) bool {
			var rejected *RejectedError
			return errors.As(err, &rejected) && rejected.Problems[0] == "The code is wrong."
		}},
		{http.StatusTooManyRequests, `{"detail":"Wait."}`, func(err error) bool {
			var limited *TooManyCodesError
			return errors.As(err, &limited)
		}},
		{http.StatusInternalServerError, `{}`, func(err error) bool { return errors.Is(err, ErrUnavailable) }},
	} {
		c := serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		if err := c.ResetPassword(context.Background(), "sam@example.com", "123456", "pw"); !tc.check(err) {
			t.Fatalf("status %d: err = %v", tc.status, err)
		}
	}
}
