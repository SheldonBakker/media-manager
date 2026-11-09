package coordinator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewIsOffWithoutURLOrToken(t *testing.T) {
	for _, tc := range [][2]string{{"", ""}, {"http://coordinator.internal", ""}, {"", "a-service-token-of-some-length-1234"}} {
		c, err := New(tc[0], tc[1])
		if c != nil || err != nil {
			t.Errorf("New(%q, %q) = %v, %v; want off", tc[0], tc[1], c, err)
		}
	}
	if c, err := New("http://coordinator.internal/", "a-service-token-of-some-length-1234"); c == nil || err != nil {
		t.Errorf("New with both values = %v, %v; want a client", c, err)
	}
	if c, err := New("not a url", "a-service-token-of-some-length-1234"); c != nil || err == nil {
		t.Errorf("New with a broken address = %v, %v; want an error", c, err)
	}
}

func TestForwardTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c, err := New(srv.URL, "a-service-token-of-some-length-1234")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Timeout = 50 * time.Millisecond
	start := time.Now()
	_, _, err = c.Forward(context.Background(), http.MethodGet, "sam", "", nil)
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
