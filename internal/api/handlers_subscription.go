package api

import (
	"log/slog"
	"net/http"

	"github.com/rdborg/mediarium/internal/auth"
)

const noSubscriptionMessage = "This account has no subscription."

// directorySubject is who the Coordinator knows the account as: the id the
// directory gave the person. ok is false when subscriptions are off or the
// account has no directory sign-in.
func (s *Server) directorySubject(user *auth.User) (subject string, ok bool) {
	ids, err := s.Auth.IdentitiesOf(user.ID)
	if err != nil {
		return "", false
	}
	return s.subjectIn(ids)
}

// subjectIn is directorySubject for identities already read.
func (s *Server) subjectIn(ids []auth.Identity) (subject string, ok bool) {
	if s.coordinator == nil {
		return "", false
	}
	for _, id := range ids {
		if id.Provider == auth.SourceLDAP {
			return id.Subject, true
		}
	}
	return "", false
}

// coordinatorStatuses are the answers the Coordinator is known to give. Any
// other is still passed on, and noted in the log.
var coordinatorStatuses = map[int]bool{
	http.StatusOK:                  true,
	http.StatusBadRequest:          true,
	http.StatusNotFound:            true,
	http.StatusConflict:            true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
}

// forwardSubscription sends the call to the Coordinator for the signed-in
// account and answers with its status and body as they came, with two
// exceptions. The Coordinator refusing Mediarium's own token (401 or 403) is a
// setup problem for an administrator, so the member gets the plain server
// error. And the Coordinator not knowing the person (a 404 on reading) means
// they have no subscription, said in Mediarium's words.
func (s *Server) forwardSubscription(w http.ResponseWriter, r *http.Request, path string) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "You need to sign in first.")
		return
	}
	subject, ok := s.directorySubject(user)
	if !ok {
		writeError(w, http.StatusNotFound, noSubscriptionMessage)
		return
	}
	status, payload, err := s.coordinator.Forward(r.Context(), r.Method, subject, path)
	if err != nil {
		slog.Error("subscription: the Coordinator did not answer", "err", err)
		writeError(w, http.StatusBadGateway, genericServerError)
		return
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		slog.Warn("subscription: the Coordinator refused MEDIARIUM_COORDINATOR_TOKEN", "status", status)
		writeError(w, http.StatusBadGateway, genericServerError)
		return
	case status == http.StatusNotFound && r.Method == http.MethodGet:
		writeError(w, http.StatusNotFound, noSubscriptionMessage)
		return
	case !coordinatorStatuses[status]:
		slog.Warn("subscription: the Coordinator gave an answer it is not known to give", "status", status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

// forwardTo is the handler for one subscription call. None carries a body:
// whatever the person sent is not passed on.
func (s *Server) forwardTo(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.forwardSubscription(w, r, path) }
}
