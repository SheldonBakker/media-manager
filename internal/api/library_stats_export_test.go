package api

import "time"

// TestHoldFirstLibraryWalk makes the first walk of the library look as if it
// were still running, as on a big library on a slow drive, until release is
// called.
func (s *Server) TestHoldFirstLibraryWalk() (release func()) {
	w := &statsWalk{done: make(chan struct{})}
	s.statsCache.mu.Lock()
	s.statsCache.first = w
	s.statsCache.mu.Unlock()
	return func() {
		s.statsCache.mu.Lock()
		s.statsCache.first = nil
		s.statsCache.mu.Unlock()
		close(w.done)
	}
}

// SetFirstStatsWaitForTest shortens how long a caller waits for the first
// walk, and returns the function that puts the real wait back.
func SetFirstStatsWaitForTest(d time.Duration) (restore func()) {
	old := firstStatsWait
	firstStatsWait = d
	return func() { firstStatsWait = old }
}
