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
	c.http.Timeout = 50 * time.Millisecond
	start := time.Now()
	_, _, err := c.Forward(context.Background(), http.MethodGet, "sam", "", nil)
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
