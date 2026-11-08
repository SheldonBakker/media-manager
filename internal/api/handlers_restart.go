package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/auth"
	"github.com/rdborg/mediarium/internal/restartnote"
	"github.com/rdborg/mediarium/internal/settings"
)

// restartRuntime is what the Server keeps about restarting itself: the note
// a previous run left when the watchdog restarted it, and test hooks.
type restartRuntime struct {
	mu    sync.Mutex
	stuck stuckRestart // set at start when the app had restarted itself

	exitWith     func(code int) // replaces os.Exit in tests
	supervisorIs string         // pretends this is what starts Mediarium again (tests)
}

// stuckRestart is what the previous run left behind when the watchdog
// restarted the app.
type stuckRestart struct {
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
}

// restartAfter gives the reply to Restart or Shut down time to reach the
// browser before the app goes away.
const restartAfter = 2 * time.Second

func (s *Server) restartDir() string { return restartnote.Dir(s.cfg.ConfigDir) }

// autoRestartEnabled is the "restart when stuck" switch (on by default).
func (s *Server) autoRestartEnabled() bool { return s.settingOn(settings.KeyAutoRestartStuck, true) }

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// controlPayload says what the Restart and Shut down buttons can do here.
type controlPayload struct {
	Kind        string `json:"kind"`        // docker | systemd | launchd | custom | none
	CanRestart  bool   `json:"canRestart"`  // something starts Mediarium again after it exits
	CanShutdown bool   `json:"canShutdown"` // Mediarium can stop itself for good (only when nothing would start it again)
	Note        string `json:"note"`
}

// detectSupervisor works out what starts Mediarium again after it exits:
// "docker" (the container's restart policy), "systemd", "launchd", "custom"
// (MEDIARIUM_SUPERVISED is set for a service manager we do not know), or
// "none". getenv and exists are passed in so this can be tested.
func detectSupervisor(supervisedByHand bool, getenv func(string) string, exists func(string) bool) string {
	switch {
	case exists("/.dockerenv") || getenv("container") != "":
		return "docker"
	case supervisedByHand:
		return "custom"
	case getenv("INVOCATION_ID") != "":
		return "systemd"
	}
	if v := getenv("XPC_SERVICE_NAME"); v != "" && v != "0" {
		return "launchd"
	}
	return "none"
}

func (s *Server) control() controlPayload {
	kind := detectSupervisor(s.cfg.Supervised, os.Getenv, pathExists)
	if s.rt.supervisorIs != "" {
		kind = s.rt.supervisorIs
	}
	c := controlPayload{Kind: kind, CanRestart: kind != "none", CanShutdown: kind == "none"}
	switch kind {
	case "docker":
		c.Note = "Docker starts Mediarium again by itself when the container's restart policy is \"unless stopped\", which is what the supplied compose file uses. To stop Mediarium, use your container manager."
	case "systemd", "launchd", "custom":
		c.Note = "Your service manager starts Mediarium again by itself. To stop it, use the service manager."
	default:
		c.Note = "Mediarium isn't run by anything that starts it again, so it can't restart itself. Start it again yourself after it stops."
	}
	return c
}

// controlState is the answer of GET /api/system/control: what the Restart and
// Shut down buttons can do, and whether this run began after the app
// restarted itself.
type controlState struct {
	controlPayload
	SelfRestart *stuckRestart `json:"selfRestart,omitempty"`
}

// handleControl reports what the Restart card shows (administrators only).
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	out := controlState{controlPayload: s.control()}
	if st := s.selfRestartNote(); st.Reason != "" {
		out.SelfRestart = &st
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- The switches ----

type optionsPayload struct {
	AutoRestartWhenStuck bool `json:"autoRestartWhenStuck"` // restart when nothing has answered for three minutes
}

func (s *Server) options() optionsPayload {
	return optionsPayload{AutoRestartWhenStuck: s.autoRestartEnabled()}
}

type optionsRequest struct {
	AutoRestartWhenStuck *bool `json:"autoRestartWhenStuck"`
}

// handleGetOptions reports the switch for restarting when stuck.
func (s *Server) handleGetOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.options())
}

// handlePutOptions changes the switches named in the body and leaves the rest.
func (s *Server) handlePutOptions(w http.ResponseWriter, r *http.Request) {
	var req optionsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Couldn't read that request. Reload the page and try again.")
		return
	}
	if req.AutoRestartWhenStuck != nil {
		if err := setBool(s.Settings, settings.KeyAutoRestartStuck, *req.AutoRestartWhenStuck); err != nil {
			slog.Error("system options: could not save a setting", "key", settings.KeyAutoRestartStuck, "err", err)
			writeError(w, http.StatusInternalServerError, "Couldn't save that. Try again.")
			return
		}
	}
	writeJSON(w, http.StatusOK, s.options())
}

// ---- Restart and shut down ----

const notRestartableMessage = "Restart is not available here. Start Mediarium again yourself."

// handleRestart restarts Mediarium. With ?safe=true it starts again once with
// automatic searching, downloading and refreshing paused (the same as safe
// mode), so the pages load and settings can be changed.
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if !s.control().CanRestart {
		writeError(w, http.StatusConflict, notRestartableMessage)
		return
	}
	if s.maintenanceBusy() {
		writeError(w, http.StatusConflict, "Mediarium is busy with a restore or import. Try again in a minute.")
		return
	}
	safe := r.URL.Query().Get("safe") == "true" || r.URL.Query().Get("safe") == "1"
	if safe {
		if err := restartnote.WriteSafeOnce(s.restartDir()); err != nil {
			slog.Error("restart: could not write the safe-mode marker", "err", err)
			writeError(w, http.StatusInternalServerError, "Couldn't prepare the restart. Check the restart folder in your config folder.")
			return
		}
	}
	actor := actorName(r)
	slog.Warn("restart requested", "by", actor, "from", s.clientIP(r), "safe", safe)
	msg := fmt.Sprintf("Mediarium was restarted by %s.", actor)
	if safe {
		msg = fmt.Sprintf("Mediarium was restarted by %s with automation paused.", actor)
	}
	_ = s.QueueRepo.LogActivity(0, "update", msg)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "restarting": true, "safe": safe, "restartingIn": int(restartAfter.Seconds())})
	flush(w)
	s.restartInPlace.Store(true)
	s.restartSoon(restartAfter)
}

// handleShutdown stops Mediarium for good. Only offered when nothing would
// start it again; in Docker (or under a service manager) it would just come
// back, so those are told to use their own tools.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	c := s.control()
	if !c.CanShutdown {
		writeError(w, http.StatusConflict, "To stop Mediarium, use your container manager or service manager.")
		return
	}
	actor := actorName(r)
	slog.Warn("shutdown requested", "by", actor, "from", s.clientIP(r))
	_ = s.QueueRepo.LogActivity(0, "update", fmt.Sprintf("Mediarium was shut down by %s.", actor))
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "shuttingDown": true})
	flush(w)
	s.restartSoon(restartAfter)
}

// RestartWanted reports whether the app is exiting to start again (a restore
// or the Restart button) rather than to stop; main then starts the program
// again in the same process, so the container keeps running.
func (s *Server) RestartWanted() bool { return s.restartInPlace.Load() }

// restartSoon exits the app after a short pause, so the reply that was just
// sent gets through. The container's restart policy (or the service manager)
// starts it again.
func (s *Server) restartSoon(after time.Duration) {
	if s.restartDelay != 0 {
		after = s.restartDelay
	}
	go func() {
		time.Sleep(after)
		s.exit()
	}()
}

// flush pushes a written reply to the client before the app goes away.
func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// actorName is who is making the request, for the record.
func actorName(r *http.Request) string {
	if u := auth.UserFromContext(r.Context()); u != nil {
		return u.Username
	}
	return "an unknown account"
}
