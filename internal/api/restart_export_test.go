package api

import (
	"context"
	"time"
)

// Hooks for the restart and watchdog tests (package api_test).

func (s *Server) TestSetSupervisor(k string) { s.rt.supervisorIs = k }
func (s *Server) TestSetExitWith(fn func(code int)) {
	s.rt.exitWith = fn
}

// TestWatchdog runs the watchdog with a short look interval and a short
// "stuck" time, and returns a function that stops it.
func (s *Server) TestWatchdog(every, stuckFor time.Duration) (stop func()) {
	old := watchdogStuckFor
	watchdogStuckFor = stuckFor
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.watchdogLoop(ctx, every)
	}()
	return func() {
		cancel()
		<-done
		watchdogStuckFor = old
	}
}

// TestNoteSelfRestart reads the note a watchdog restart left, as the start of
// the app does.
func (s *Server) TestNoteSelfRestart() {
	stop := s.StartWatchdog()
	stop()
}

func (s *Server) TestRestartDir() string { return s.restartDir() }
