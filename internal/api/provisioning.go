package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/rdborg/mediarium/internal/auth"
)

// Provisioning: another service, such as a sign-up page in front of the
// directory, makes the account of a directory person before their first
// sign-in, so that sign-in finds this account instead of making a new one. The
// service signs in with a token of its own (MEDIARIUM_PROVISIONING_TOKEN). A
// session or an API key never opens these routes, and the token opens nothing
// else. Without a token the routes don't exist.

// provisioningActor names the provisioning service in the account log.
const provisioningActor = "provisioning"

// subjectMax is the longest directory id accepted for a person.
const subjectMax = 255

// provisioningRoutes adds the routes another service calls with the
// provisioning token. They sit on the signed-out mux because the token, not a
// session, decides who may call them.
func (s *Server) provisioningRoutes(provisioning *http.ServeMux) {
	provisioning.HandleFunc("POST /api/provisioning/directory-accounts", s.handleProvisionDirectoryAccount)
	provisioning.HandleFunc("PUT /api/provisioning/directory-accounts/{subject}/access", s.handleProvisionAccess)
}

// provisioningAllowed checks the caller's bearer token. On failure it has
// already answered: 404 while no token is set, as for an address that doesn't
// exist, and 401 for a missing or wrong token.
func (s *Server) provisioningAllowed(w http.ResponseWriter, r *http.Request) bool {
	want := s.cfg.ProvisioningToken
	if want == "" {
		http.NotFound(w, r)
		return false
	}
	// Comparing hashes takes the same time whatever the length of the guess.
	given, wanted := sha256.Sum256([]byte(bearerToken(r))), sha256.Sum256([]byte(want))
	if subtle.ConstantTimeCompare(given[:], wanted[:]) != 1 {
		slog.Warn("auth: refused a provisioning call without the right token", "ip", s.clientIP(r))
		writeError(w, http.StatusUnauthorized, "This needs the provisioning token.")
		return false
	}
	return true
}

type provisionAccountRequest struct {
	Subject  string `json:"subject"` // the directory's stable id for the person
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	// Locked makes a new account need a subscription. It is ignored for a
	// person who already has an account.
	Locked bool `json:"locked"`
}

// provisionedAccount is what the other service learns about the account.
type provisionedAccount struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func toProvisionedAccount(u *auth.User) provisionedAccount {
	return provisionedAccount{ID: u.ID, Username: u.Username, Role: u.Role()}
}

// handleProvisionDirectoryAccount makes the account of a directory person
// before their first sign-in, or answers with the one already linked to them.
// A new account is always a basic user, with no password here. An account that
// is already linked is returned as it is, never changed. A user name another
// account has is refused, never changed or taken over.
func (s *Server) handleProvisionDirectoryAccount(w http.ResponseWriter, r *http.Request) {
	if !s.provisioningAllowed(w, r) {
		return
	}
	var req provisionAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "That request couldn't be read.")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Username = strings.TrimSpace(req.Username)
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	if rejectBad(w,
		checkRequired(req.Subject, "Send the person's id in the directory as subject."),
		checkMaxLen(req.Subject, "Subject", subjectMax),
		checkUsername(req.Username),
		checkEmail(req.Email),
		checkMaxLen(req.Name, "Name", nameMax),
	) {
		return
	}
	linked, err := s.Auth.UserByIdentity(auth.SourceLDAP, req.Subject)
	if err == nil {
		writeJSON(w, http.StatusOK, toProvisionedAccount(linked))
		return
	}
	if !errors.Is(err, auth.ErrNoIdentity) {
		writeAccountError(w, err)
		return
	}
	created, err := s.Auth.CreateExternalUser(auth.ExternalProfile{
		Provider: auth.SourceLDAP, Subject: req.Subject, Username: req.Username, Name: req.Name, Email: req.Email,
		Role: auth.RoleMember, AllowSuffix: false, Locked: req.Locked,
	})
	if errors.Is(err, auth.ErrIdentityTaken) || errors.Is(err, auth.ErrUsernameTaken) {
		// Another call for the same person may have made the account a moment ago.
		if linked, err := s.Auth.UserByIdentity(auth.SourceLDAP, req.Subject); err == nil {
			writeJSON(w, http.StatusOK, toProvisionedAccount(linked))
			return
		}
	}
	switch {
	case errors.Is(err, auth.ErrIdentityTaken):
		writeError(w, http.StatusConflict, "That directory id is already linked to an account.")
		return
	case err != nil:
		writeAccountError(w, err)
		return
	}
	s.auditAccountBy(r, provisioningActor, fmt.Sprintf("Account %q was created as %s for a directory sign-in", created.Username, roleWords(created.Role())))
	writeJSON(w, http.StatusCreated, toProvisionedAccount(created))
}

// accessBodyMax is the biggest body the access call needs: one true or false.
const accessBodyMax = 1 << 10

type provisionAccessRequest struct {
	Locked bool `json:"locked"`
}

// handleProvisionAccess locks the account of a directory person, so that it
// needs a subscription, or unlocks it again. Nothing else about the account
// changes, and its sessions stay.
func (s *Server) handleProvisionAccess(w http.ResponseWriter, r *http.Request) {
	if !s.provisioningAllowed(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, accessBodyMax)
	var req provisionAccessRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "That request couldn't be read.")
		return
	}
	subject := strings.TrimSpace(r.PathValue("subject"))
	if rejectBad(w,
		checkRequired(subject, "Send the person's id in the directory as subject."),
		checkMaxLen(subject, "Subject", subjectMax),
	) {
		return
	}
	u, err := s.Auth.SetSubscriptionLocked(auth.SourceLDAP, subject, req.Locked)
	if errors.Is(err, auth.ErrNoIdentity) {
		writeError(w, http.StatusNotFound, "No account is linked to that directory id.")
		return
	}
	if err != nil {
		writeAccountError(w, err)
		return
	}
	if req.Locked {
		s.auditAccountBy(r, provisioningActor, fmt.Sprintf("Account %q needs a subscription", u.Username))
	} else {
		s.auditAccountBy(r, provisioningActor, fmt.Sprintf("Account %q has a subscription", u.Username))
	}
	w.WriteHeader(http.StatusNoContent)
}
