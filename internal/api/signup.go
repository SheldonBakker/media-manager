package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/accountservice"
	"github.com/rdborg/mediarium/internal/auth"
	"github.com/rdborg/mediarium/internal/httpsec"
)

// Signing up: a person makes their own basic account from the sign-in page.
// They first ask for a confirmation code, which the account service emails to
// them, and sign up with it. The account service (MEDIARIUM_ACCOUNT_SERVICE_URL)
// signs them up, which puts them in the directory with the password they chose
// and makes their account here through the provisioning route. Mediarium then
// signs them in to that account, found by the directory id the service answers
// with, without asking the directory to check the password it has just set.
// Without the setting the route doesn't exist.

const (
	// signUpsPerAddress is how many accounts one address may make per
	// signUpWindow. Anyone who can reach the sign-in page can sign up.
	signUpsPerAddress = 5
	signUpWindow      = time.Hour
	// signUpCodesPerAddress is how many confirmation codes one address may ask
	// for per signUpWindow, whichever addresses they go to.
	signUpCodesPerAddress = 10
)

type signUpCodeRequest struct {
	Email string `json:"email"`
}

// handleSignUpCode asks the account service to email a confirmation code, which
// signing up needs.
func (s *Server) handleSignUpCode(w http.ResponseWriter, r *http.Request) {
	if s.accountService == nil {
		http.NotFound(w, r)
		return
	}
	ipKey := httpsec.LimitKey(s.clientIP(r))
	if !s.signUpCodes.Reserve(ipKey) {
		writeError(w, http.StatusTooManyRequests, "Too many codes were asked for from here. Try again later.")
		return
	}
	var req signUpCodeRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPublicBody)
	if err := decodeJSON(r, &req); err != nil {
		s.signUpCodes.Refund(ipKey)
		writeError(w, http.StatusBadRequest, "Couldn't read that request. Reload the page and try again.")
		return
	}
	err := s.accountService.SendCode(r.Context(), strings.TrimSpace(req.Email))
	if err == nil {
		writeJSON(w, http.StatusAccepted, struct{}{})
		return
	}
	if giveBack := writeAccountServiceFailure(w, err, "That email address wasn't accepted. Check it and try again."); giveBack {
		s.signUpCodes.Refund(ipKey)
	}
}

// writeAccountServiceFailure answers a request the account service didn't
// carry out. rejected is the answer when it refused without saying why. It
// reports whether the attempt should be given back to the caller's limit: a
// refusal of what they typed or a service that couldn't answer isn't counted
// against them.
func writeAccountServiceFailure(w http.ResponseWriter, err error, rejected string) (giveBack bool) {
	var refused *accountservice.RejectedError
	var limited *accountservice.TooManyCodesError
	switch {
	case errors.As(err, &limited):
		if limited.Message == "" {
			writeError(w, http.StatusTooManyRequests, "A code was sent to this address a moment ago. Wait a minute and try again.")
			return false
		}
		writeError(w, http.StatusTooManyRequests, limited.Message)
		return false
	case errors.As(err, &refused):
		if len(refused.Problems) == 0 {
			writeError(w, http.StatusBadRequest, rejected)
			return true
		}
		writeError(w, http.StatusBadRequest, strings.Join(refused.Problems, " "))
		return true
	case errors.Is(err, accountservice.ErrEmailTaken):
		writeError(w, http.StatusConflict, "An account with this email address already exists. Sign in instead.")
		return false
	case errors.Is(err, accountservice.ErrUnavailable):
		slog.Error("auth: the account service is unavailable", "err", err)
		writeError(w, http.StatusBadGateway, "The account service couldn't be reached. Try again in a moment.")
		return true
	}
	slog.Error("auth: account service request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "That didn't work. Try again.")
	return true
}

type signUpRequest struct {
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	Password    string `json:"password"`
	Code        string `json:"code"`
}

// signUpPending answers a sign-up whose account here isn't ready yet.
type signUpPending struct {
	Pending  bool   `json:"pending"`
	Username string `json:"username"`
}

// handleSignUp makes a basic account through the account service and signs the
// person in, or says the account is still being set up.
func (s *Server) handleSignUp(w http.ResponseWriter, r *http.Request) {
	if s.accountService == nil {
		http.NotFound(w, r)
		return
	}
	ip := s.clientIP(r)
	ipKey := httpsec.LimitKey(ip)
	if !s.signUps.Reserve(ipKey) {
		writeError(w, http.StatusTooManyRequests, "Too many accounts were made from here. Try again later.")
		return
	}
	// Attempts that made an account, or that tried an email address already in
	// use, stay counted. The others are given back.
	giveBack := func() { s.signUps.Refund(ipKey) }

	var req signUpRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPublicBody)
	if err := decodeJSON(r, &req); err != nil {
		giveBack()
		writeError(w, http.StatusBadRequest, "Couldn't read that request. Reload the page and try again.")
		return
	}
	account, err := s.accountService.Register(r.Context(), accountservice.SignUp{
		FirstName:   strings.TrimSpace(req.FirstName),
		LastName:    strings.TrimSpace(req.LastName),
		Email:       strings.TrimSpace(req.Email),
		PhoneNumber: strings.TrimSpace(req.PhoneNumber),
		Password:    req.Password,
		EmailCode:   strings.TrimSpace(req.Code),
	})
	var rejected *accountservice.RejectedError
	switch {
	case errors.As(err, &rejected):
		giveBack()
		if len(rejected.Problems) == 0 {
			writeError(w, http.StatusBadRequest, "Those details weren't accepted. Check them and try again.")
			return
		}
		writeError(w, http.StatusBadRequest, strings.Join(rejected.Problems, " "))
		return
	case errors.Is(err, accountservice.ErrEmailTaken):
		writeError(w, http.StatusConflict, "An account with this email address already exists. Sign in instead.")
		return
	case errors.Is(err, accountservice.ErrUnavailable):
		giveBack()
		slog.Error("auth: sign-up failed, the account service is unavailable", "err", err)
		writeError(w, http.StatusBadGateway, "The account service couldn't be reached. Try again in a moment.")
		return
	case err != nil:
		giveBack()
		slog.Error("auth: sign-up failed", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't make the account. Try again.")
		return
	}
	slog.Info("auth: account made by signing up", "user", account.Username, "ip", ip)
	user := s.signInAfterSignUp(account)
	if user == nil {
		writeJSON(w, http.StatusAccepted, signUpPending{Pending: true, Username: account.Username})
		return
	}
	s.finishSignInVia(w, r, user, auth.MethodLDAP, nil)
}

// signInAfterSignUp is the account the service has just made for the new
// person: the one linked to the directory id it answered with, under the user
// name it gave. It is nil while that account isn't ready, or while directory
// sign-in is off (the account couldn't sign in either way).
func (s *Server) signInAfterSignUp(account accountservice.Account) *auth.User {
	if account.Username == "" || account.DirectorySubject == "" || !account.MediaLibraryReady {
		return nil
	}
	if _, on := s.ldapActive(); !on {
		return nil
	}
	user, err := s.Auth.UserByIdentity(auth.SourceLDAP, account.DirectorySubject)
	if err != nil || user.Disabled || user.Username != account.Username {
		slog.Warn("auth: a new account could not be signed in yet", "user", account.Username, "err", err)
		return nil
	}
	return user
}
