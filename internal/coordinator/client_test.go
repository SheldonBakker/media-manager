package coordinator

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewIsOffWithoutURLOrToken(t *testing.T) {
	for _, tc := range [][2]string{{"", ""}, {"http://coordinator.internal", ""}, {"", "a-service-token-of-some-length-1234"}} {
		if c := New(tc[0], tc[1]); c != nil {
			t.Errorf("New(%q, %q) = %v; want off", tc[0], tc[1], c)
		}
	}
	if c := New("http://coordinator.internal/", "a-service-token-of-some-length-1234"); c == nil {
		t.Error("New with both values is off; want a client")
	}
}

func TestForwardTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c := New(srv.URL, "a-service-token-of-some-length-1234")
	c.readTimeout = 50 * time.Millisecond
	start := time.Now()
	_, _, err := c.Forward(context.Background(), http.MethodGet, "sam", "")
	if err == nil {
		t.Fatal("a Coordinator that never answers gave no error")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s to give up", time.Since(start))
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("the error names the Coordinator address: %v", err)
	}
}

// Confirming a payment waits on the payment service, Emby and Mediarium in
// turn on the Coordinator's side, so a POST may take longer than a GET.
func TestForwardGivesAPostLongerThanAGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "a-service-token-of-some-length-1234")
	if c.readTimeout != 15*time.Second || c.writeTimeout != 30*time.Second {
		t.Errorf("timeouts = %s for a GET and %s for a POST, want 15s and 30s", c.readTimeout, c.writeTimeout)
	}
	c.readTimeout, c.writeTimeout = 50*time.Millisecond, 5*time.Second
	if _, _, err := c.Forward(context.Background(), http.MethodGet, "sam", ""); !errors.Is(err, ErrTimedOut) {
		t.Errorf("a slow GET: %v, want ErrTimedOut", err)
	}
	if status, _, err := c.Forward(context.Background(), http.MethodPost, "sam", "/checkout/confirm"); err != nil || status != http.StatusOK {
		t.Errorf("a POST as slow: %d %v, want 200", status, err)
	}
}

// An answer over the cap is not cut short and passed on as if it were whole.
func TestForwardRefusesAnAnswerOverTheCap(t *testing.T) {
	for _, tc := range []struct {
		size int
		ok   bool
	}{{maxBodyBytes, true}, {maxBodyBytes + 1, false}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(bytes.Repeat([]byte("x"), tc.size))
		}))
		c := New(srv.URL, "a-service-token-of-some-length-1234")
		_, payload, err := c.Forward(context.Background(), http.MethodGet, "sam", "")
		srv.Close()
		if tc.ok && (err != nil || len(payload) != tc.size) {
			t.Errorf("%d bytes: %d bytes, %v; want the whole answer", tc.size, len(payload), err)
		}
		if !tc.ok && !errors.Is(err, ErrUnavailable) {
			t.Errorf("%d bytes: %d bytes, %v; want ErrUnavailable", tc.size, len(payload), err)
		}
	}
}
