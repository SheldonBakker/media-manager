package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Several callers that arrive before the first statistics exist share one walk
// of the library instead of each starting their own.
func TestFirstStatsWalkIsSharedBetweenCallers(t *testing.T) {
	var c statsCache
	var walks atomic.Int32
	release := make(chan struct{})
	walk := func() (libraryStats, error) {
		walks.Add(1)
		<-release
		return libraryStats{movieBytes: 42, qualities: map[string]int{}}, nil
	}

	var wg sync.WaitGroup
	results := make([]int64, 5)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.mu.Lock()
			st, _ := c.firstWalk(walk, time.Minute)
			results[i] = st.movieBytes
		}()
	}
	time.Sleep(100 * time.Millisecond) // let them all queue up behind the first
	close(release)
	wg.Wait()

	if n := walks.Load(); n != 1 {
		t.Errorf("%d walks, want 1", n)
	}
	for i, r := range results {
		if r != 42 {
			t.Errorf("caller %d got %d", i, r)
		}
	}
	if c.stats.movieBytes != 42 {
		t.Error("the result was not kept")
	}
}

// A caller stops waiting for the first walk once the wait is up (a big library
// on a network drive can take longer than a page may), and the walk carries
// on without it: its result is kept for the next caller.
func TestFirstStatsWalkCarriesOnAfterTheCallerStopsWaiting(t *testing.T) {
	var c statsCache
	release := make(chan struct{})
	walk := func() (libraryStats, error) {
		<-release
		return libraryStats{movieBytes: 42, qualities: map[string]int{}}, nil
	}

	gaveUp := make(chan error, 1)
	go func() {
		c.mu.Lock()
		_, err := c.firstWalk(walk, 20*time.Millisecond)
		gaveUp <- err
	}()
	select {
	case err := <-gaveUp:
		if !errors.Is(err, errStatsPending) {
			t.Fatalf("err = %v, want errStatsPending", err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("the caller kept waiting for the walk")
	}

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.mu.Lock()
		kept, running := c.stats.movieBytes, c.first != nil
		c.mu.Unlock()
		if !running {
			if kept != 42 {
				t.Fatalf("kept %d, want the walk's 42", kept)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the walk never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
