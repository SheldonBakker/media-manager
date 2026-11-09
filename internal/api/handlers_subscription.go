package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/rdborg/mediarium/internal/auth"
)

const noSubscriptionMessage = "This account has no subscription."

// maxConfirmBody is the most the confirm call reads: a payment reference is a
// short word.
const maxConfirmBody = 1 << 10

// directorySubject is who the Coordinator knows the account as: the id the
// directory gave the person. ok is false when subscriptions are off or the
// account has no directory sign-in.
func (s *Server) directorySubject(user *auth.User) (subject string, ok bool) {
	if s.coordinator == nil {
		return "", false
	}
	ids, err := s.Auth.IdentitiesOf(user.ID)
	if err != nil {
		return "", false
	}
	for _, id := range ids {
		if id.Provider == auth.SourceLDAP {
			return id.Subject, true
		}
	}
	return "", false
}

// forwardSubscription sends the call to the Coordinator for the signed-in
// account and answers with its status and body as they came.
func (s *Server) forwardSubscription(w http.ResponseWriter, r *http.Request, path string, body []byte) {
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
	status, payload, err := s.coordinator.Forward(r.Context(), r.Method, subject, path, body)
	if err != nil {
		slog.Error("subscription: the Coordinator did not answer", "err", err)
		writeError(w, http.StatusBadGateway, genericServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

// forwardTo is the handler for a call that carries no body of its own.
func (s *Server) forwardTo(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.forwardSubscription(w, r, path, nil) }
}

// handleSubscriptionConfirm passes on the payment reference the person came
// back from the payment page with, and nothing else of what they sent.
func (s *Server) handleSubscriptionConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reference string `json:"reference"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConfirmBody)).Decode(&req); err != nil || req.Reference == "" {
		writeError(w, http.StatusBadRequest, "Couldn't read that request. Reload the page and try again.")
		return
	}
	body, _ := json.Marshal(req)
	s.forwardSubscription(w, r, "/checkout/confirm", body)
}
