package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/rdborg/mediarium/internal/httpsec"
)

// Resetting a forgotten password, for the basic accounts the account service
// holds (MEDIARIUM_ACCOUNT_SERVICE_URL). The service emails a code and, with
// it, sets the new password in the directory, so it works here, in the media
// server and everywhere else the person signs in. Administrators with a
// password here use the reset-password command instead. Without the setting
// the routes don't exist.

// passwordResetsPerAddress is how many reset codes and resets one address may
// ask for per signUpWindow.
const passwordResetsPerAddress = 10

type passwordResetCodeRequest struct {
	Email string `json:"email"`
}

type passwordResetRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

// handlePasswordResetCode asks the account service to email a password reset
// code. The answer is the same whether or not an account has the address.
func (s *Server) handlePasswordResetCode(w http.ResponseWriter, r *http.Request) {
	var req passwordResetCodeRequest
	ipKey, ok := s.startPasswordReset(w, r, &req)
	if !ok {
		return
	}
	err := s.accountService.RequestPasswordReset(r.Context(), strings.TrimSpace(req.Email))
	if err == nil {
		writeJSON(w, http.StatusAccepted, struct{}{})
		return
	}
	if giveBack := writeAccountServiceFailure(w, err, "That email address wasn't accepted. Check it and try again."); giveBack {
		s.passwordResets.Refund(ipKey)
	}
}

// handlePasswordReset sets a new password with the emailed code.
func (s *Server) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequest
	ipKey, ok := s.startPasswordReset(w, r, &req)
	if !ok {
		return
	}
	err := s.accountService.ResetPassword(r.Context(), strings.TrimSpace(req.Email), strings.TrimSpace(req.Code), req.Password)
	if err == nil {
		slog.Info("auth: password reset through the account service", "ip", s.clientIP(r))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if giveBack := writeAccountServiceFailure(w, err, "That code or password wasn't accepted. Check them and try again."); giveBack {
		s.passwordResets.Refund(ipKey)
	}
}

// startPasswordReset checks that resets are on and allowed from this address and
// reads the request into into. On failure it has already answered.
func (s *Server) startPasswordReset(w http.ResponseWriter, r *http.Request, into any) (ipKey string, ok bool) {
	if s.accountService == nil {
		http.NotFound(w, r)
		return "", false
	}
	ipKey = httpsec.LimitKey(s.clientIP(r))
	if !s.passwordResets.Reserve(ipKey) {
		writeError(w, http.StatusTooManyRequests, "Too many password resets were asked for from here. Try again later.")
		return "", false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPublicBody)
	if err := decodeJSON(r, into); err != nil {
		s.passwordResets.Refund(ipKey)
		writeError(w, http.StatusBadRequest, "Couldn't read that request. Reload the page and try again.")
		return "", false
	}
	return ipKey, true
}
