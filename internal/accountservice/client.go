// Package accountservice signs people up with the account service that holds
// basic accounts (MEDIARIUM_ACCOUNT_SERVICE_URL). A sign-up there makes the
// person's directory account and, through the provisioning route, their basic
// account in Mediarium. Mediarium calls it from the server; the sign-in page
// never talks to it directly.
//
// The package depends on nothing else in Mediarium except netguard.
package accountservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/netguard"
)

const (
	// requestTimeout covers the whole sign-up, which makes accounts in several
	// services before it answers.
	requestTimeout = 60 * time.Second
	maxBodyBytes   = 1 << 20
)

var (
	// ErrEmailTaken is a sign-up with an email address that already has an account.
	ErrEmailTaken = errors.New("accountservice: an account with this email address already exists")
	// ErrUnavailable is an account service that could not be reached or gave an
	// answer that could not be used.
	ErrUnavailable = errors.New("accountservice: the account service is unavailable")
)

// TooManyCodesError is a confirmation code asked for too often, with the
// service's sentence about when to try again.
type TooManyCodesError struct {
	Message string
}

func (e *TooManyCodesError) Error() string { return "accountservice: too many codes: " + e.Message }

// RejectedError is a sign-up the service refused, with its reasons in plain
// sentences (empty when it gave none).
type RejectedError struct {
	Problems []string
}

func (e *RejectedError) Error() string {
	return "accountservice: sign-up refused: " + strings.Join(e.Problems, " ")
}

// Client talks to one account service. It is safe for concurrent use.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for the service at baseURL (no trailing slash).
func New(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: netguard.Client(requestTimeout)}
}

// SignUp is what a person fills in to make an account.
type SignUp struct {
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	Password    string `json:"password"`
	// EmailCode is the confirmation code the service emailed (see SendCode).
	EmailCode string `json:"emailCode"`
}

// Account is what the service made. Username is empty while it has not given
// the person a username yet; MediaLibraryReady says whether their account in
// Mediarium exists.
type Account struct {
	Username          string
	MediaLibraryReady bool
}

type registerAnswer struct {
	Provisioning *struct {
		Username          string `json:"username"`
		MediaLibraryReady bool   `json:"mediaLibraryReady"`
	} `json:"provisioning"`
}

type problemAnswer struct {
	Errors map[string][]string `json:"errors"`
	Detail string              `json:"detail"`
}

// SendCode asks the service to email a confirmation code to email. Signing up
// needs that code.
func (c *Client) SendCode(ctx context.Context, email string) error {
	status, raw, err := c.post(ctx, "/api/v1/auth/email-codes", map[string]string{"email": email})
	if err != nil {
		return err
	}
	switch status {
	case http.StatusAccepted, http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusConflict:
		return ErrEmailTaken
	case http.StatusTooManyRequests:
		var answer problemAnswer
		_ = json.Unmarshal(raw, &answer)
		return &TooManyCodesError{Message: strings.TrimSpace(answer.Detail)}
	case http.StatusBadRequest:
		return &RejectedError{Problems: problems(raw)}
	}
	return fmt.Errorf("%w: status %d", ErrUnavailable, status)
}

// RequestPasswordReset asks the service to email a password reset code. It
// answers the same whether or not an account has that address.
func (c *Client) RequestPasswordReset(ctx context.Context, email string) error {
	status, raw, err := c.post(ctx, "/api/v1/auth/password-resets", map[string]string{"email": email})
	if err != nil {
		return err
	}
	return noContentAnswer(status, raw)
}

// ResetPassword sets a new password with the code from RequestPasswordReset.
// The service changes it everywhere the person signs in.
func (c *Client) ResetPassword(ctx context.Context, email, code, password string) error {
	status, raw, err := c.post(ctx, "/api/v1/auth/password-resets/confirm",
		map[string]string{"email": email, "code": code, "password": password})
	if err != nil {
		return err
	}
	return noContentAnswer(status, raw)
}

// noContentAnswer reads the answer of a request that returns nothing on success.
func noContentAnswer(status int, raw []byte) error {
	switch status {
	case http.StatusAccepted, http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusTooManyRequests:
		var answer problemAnswer
		_ = json.Unmarshal(raw, &answer)
		return &TooManyCodesError{Message: strings.TrimSpace(answer.Detail)}
	case http.StatusBadRequest:
		return &RejectedError{Problems: problems(raw)}
	}
	return fmt.Errorf("%w: status %d", ErrUnavailable, status)
}

// Register signs a person up. The password goes only to the service.
func (c *Client) Register(ctx context.Context, person SignUp) (Account, error) {
	status, raw, err := c.post(ctx, "/api/v1/auth/register", person)
	if err != nil {
		return Account{}, err
	}
	switch status {
	case http.StatusCreated, http.StatusAccepted:
		var answer registerAnswer
		if err := json.Unmarshal(raw, &answer); err != nil {
			return Account{}, fmt.Errorf("%w: unreadable answer", ErrUnavailable)
		}
		if answer.Provisioning == nil {
			return Account{}, nil
		}
		return Account{Username: answer.Provisioning.Username, MediaLibraryReady: answer.Provisioning.MediaLibraryReady}, nil
	case http.StatusConflict:
		return Account{}, ErrEmailTaken
	case http.StatusBadRequest:
		return Account{}, &RejectedError{Problems: problems(raw)}
	}
	return Account{}, fmt.Errorf("%w: status %d", ErrUnavailable, status)
}

// post sends body as JSON and returns the status and the start of the answer.
// A request that never got an answer is ErrUnavailable.
func (c *Client) post(ctx context.Context, path string, body any) (int, []byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("accountservice: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrUnavailable, netguard.CleanError(err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrUnavailable, netguard.CleanError(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: read answer: %v", ErrUnavailable, err)
	}
	return resp.StatusCode, raw, nil
}

// problems lists the reasons in a validation answer, ordered by field so the
// message is the same every time.
func problems(raw []byte) []string {
	var answer problemAnswer
	if json.Unmarshal(raw, &answer) != nil {
		return nil
	}
	fields := make([]string, 0, len(answer.Errors))
	for field := range answer.Errors {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	var out []string
	for _, field := range fields {
		for _, msg := range answer.Errors[field] {
			if msg = strings.TrimSpace(msg); msg != "" {
				out = append(out, msg)
			}
		}
	}
	return out
}
