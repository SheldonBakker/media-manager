// Package coordinator forwards a member's subscription calls to the
// Coordinator, the service that owns paid subscriptions (MEDIARIUM_COORDINATOR_URL
// and MEDIARIUM_COORDINATOR_TOKEN). Mediarium never keeps card data, payment
// keys or manage links; it passes the Coordinator's answer on as it is.
//
// The package depends on nothing else in Mediarium except netguard.
package coordinator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/netguard"
)

const (
	// A GET only reads. A POST can wait on the payment service, Emby and
	// Mediarium in turn on the Coordinator's side (confirming a payment), so
	// it gets longer.
	readTimeout  = 15 * time.Second
	writeTimeout = 30 * time.Second
	maxBodyBytes = 1 << 20
)

var (
	// ErrTimedOut is a Coordinator that did not answer in time.
	ErrTimedOut = errors.New("coordinator: the request timed out")
	// ErrUnavailable is a Coordinator that could not be reached or gave an
	// answer that could not be read. The error never names its address.
	ErrUnavailable = errors.New("coordinator: the Coordinator is unavailable")
)

// Client talks to one Coordinator. It is safe for concurrent use.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	// How long a GET, and any other call, may take.
	readTimeout, writeTimeout time.Duration
}

// New returns a client for the Coordinator at baseURL (no trailing slash
// needed; config checks that it is an http or https address) that signs in
// with token. It returns nil when either is empty, which means subscriptions
// are off.
func New(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" || token == "" {
		return nil
	}
	return &Client{baseURL: baseURL, token: token, http: netguard.Client(0), readTimeout: readTimeout, writeTimeout: writeTimeout}
}

// Forward makes one call for the member the directory knows as subject and
// returns the Coordinator's status and body untouched. path is what follows
// the member's subscription address: "" or "/checkout", for example. A
// Coordinator that cannot be reached gives an error that is safe to log.
func (c *Client) Forward(ctx context.Context, method, subject, path string) (int, []byte, error) {
	timeout := c.writeTimeout
	if method == http.MethodGet {
		timeout = c.readTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	target := c.baseURL + "/api/v1/members/" + url.PathEscape(subject) + "/subscription" + path
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// The transport's error carries the address; say only what went wrong.
		var ne interface{ Timeout() bool }
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			return 0, nil, ErrTimedOut
		}
		return 0, nil, ErrUnavailable
	}
	defer resp.Body.Close()
	// One byte past the cap tells a whole answer from one that was cut short.
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: the answer could not be read", ErrUnavailable)
	}
	if len(payload) > maxBodyBytes {
		return 0, nil, fmt.Errorf("%w: the answer is larger than %d bytes", ErrUnavailable, maxBodyBytes)
	}
	return resp.StatusCode, payload, nil
}
