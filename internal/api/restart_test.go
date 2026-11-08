package api_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
)

// restartEnv is a running server that believes a container's restart policy
// starts it again after it exits.
type restartEnv struct {
	server *api.Server
	base   string
	client *http.Client
	exited <-chan struct{}
	dir    string
}

func newRestartEnv(t *testing.T) *restartEnv {
	t.Helper()
	server, base, client := loginNewServer(t)
	server.TestSetSupervisor("docker")
	return &restartEnv{server: server, base: base, client: client, exited: exitSpy(server), dir: server.TestRestartDir()}
}

// control is what GET /api/system/control answers.
func (e *restartEnv) control(t *testing.T) map[string]any {
	t.Helper()
	return getJSON[map[string]any](t, e.client, e.base+"/api/system/control")
}

func (e *restartEnv) mustNotExit(t *testing.T) {
	t.Helper()
	select {
	case <-e.exited:
		t.Fatal("the app stopped although it should not have")
	case <-time.After(120 * time.Millisecond):
	}
}

func (e *restartEnv) mustExit(t *testing.T) {
	t.Helper()
	select {
	case <-e.exited:
	case <-time.After(3 * time.Second):
		t.Fatal("the app did not restart")
	}
}

func message(m map[string]any) string {
	s, _ := m["error"].(string)
	return s
}

func TestRestartInDocker(t *testing.T) {
	e := newRestartEnv(t)
	control := e.control(t)
	if control["kind"] != "docker" || control["canRestart"] != true || control["canShutdown"] != false {
		t.Fatalf("control = %v", control)
	}

	out := postJSON[map[string]any](t, e.client, e.base+"/api/system/restart", nil, http.StatusAccepted)
	if out["restarting"] != true || out["safe"] != false {
		t.Fatalf("%v", out)
	}
	e.mustExit(t)
	if _, err := os.Stat(filepath.Join(e.dir, "safe-once")); err == nil {
		t.Fatal("a plain restart must not ask for safe mode")
	}
	found := false
	for _, a := range getJSON[[]map[string]any](t, e.client, e.base+"/api/activity") {
		if a["eventType"] == "update" && strings.Contains(a["message"].(string), "restarted by ryan") {
			found = true
		}
	}
	if !found {
		t.Fatal("the restart is not on the Activity page")
	}
}

func TestRestartWithAutomationPausedLeavesAOneShotNote(t *testing.T) {
	e := newRestartEnv(t)
	out := postJSON[map[string]any](t, e.client, e.base+"/api/system/restart?safe=true", nil, http.StatusAccepted)
	if out["safe"] != true {
		t.Fatalf("%v", out)
	}
	e.mustExit(t)
	if _, err := os.Stat(filepath.Join(e.dir, "safe-once")); err != nil {
		t.Fatalf("the safe-mode note was not written: %v", err)
	}
}

func TestRestartWhereNothingWouldStartTheAppAgain(t *testing.T) {
	e := newRestartEnv(t)
	e.server.TestSetSupervisor("none")
	control := e.control(t)
	if control["canRestart"] != false || control["canShutdown"] != true {
		t.Fatalf("control = %v", control)
	}
	for _, path := range []string{"/api/system/restart", "/api/system/restart?safe=true"} {
		code, body := doStatus(t, e.client, http.MethodPost, e.base+path)
		if code != http.StatusConflict || message(body) != "Restart is not available here. Start Mediarium again yourself." {
			t.Fatalf("%s: %d %v", path, code, body)
		}
	}
	e.mustNotExit(t)
	if _, err := os.Stat(filepath.Join(e.dir, "safe-once")); err == nil {
		t.Fatal("a refused restart must not leave a safe-mode note")
	}
	// Shutting down is what is offered instead.
	out := postJSON[map[string]any](t, e.client, e.base+"/api/system/shutdown", nil, http.StatusAccepted)
	if out["shuttingDown"] != true {
		t.Fatalf("%v", out)
	}
	e.mustExit(t)
}

func TestShutdownIsNotOfferedUnderAContainerOrServiceManager(t *testing.T) {
	for _, kind := range []string{"docker", "systemd", "launchd", "custom"} {
		e := newRestartEnv(t)
		e.server.TestSetSupervisor(kind)
		code, body := doStatus(t, e.client, http.MethodPost, e.base+"/api/system/shutdown")
		if code != http.StatusConflict || !strings.Contains(message(body), "container manager") {
			t.Fatalf("%s: %d %v", kind, code, body)
		}
		e.mustNotExit(t)
	}
}

// ---- The watchdog ----

type exits struct {
	mu    sync.Mutex
	codes []int
}

func (x *exits) record(code int) { x.mu.Lock(); x.codes = append(x.codes, code); x.mu.Unlock() }
func (x *exits) list() []int     { x.mu.Lock(); defer x.mu.Unlock(); return append([]int(nil), x.codes...) }

func waitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// In these tests the app's own web address is not listening (its port is 0),
// so the watchdog sees it as not answering.
func TestWatchdogRestartsAStuckApp(t *testing.T) {
	e := newRestartEnv(t)
	x := &exits{}
	e.server.TestSetExitWith(x.record)
	stop := e.server.TestWatchdog(10*time.Millisecond, 150*time.Millisecond)
	defer stop()

	waitUntil(t, func() bool { return len(x.list()) > 0 }, "the watchdog to restart the app")
	if got := x.list(); got[0] != 1 {
		t.Fatalf("exit codes = %v, want status 1", got)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "stuck-restart")); err != nil {
		t.Fatalf("no note for the next start: %v", err)
	}
	// The next start says so.
	e.server.TestNoteSelfRestart()
	st := e.control(t)
	self, _ := st["selfRestart"].(map[string]any)
	if self == nil || !strings.Contains(self["reason"].(string), "web server answered: false") {
		t.Fatalf("selfRestart = %v", st["selfRestart"])
	}
	if _, err := os.Stat(filepath.Join(e.dir, "stuck-restart")); err == nil {
		t.Fatal("the note must be read once")
	}
	diag := getJSON[map[string]any](t, e.client, e.base+"/api/system/diagnostics")
	if rs := diag["restart"].(map[string]any); rs["selfRestart"] == nil {
		t.Fatalf("diagnostics do not mention the self-restart: %v", rs)
	}
}

func TestWatchdogStaysQuietWhen(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *restartEnv) (undo func())
	}{
		{"the switch is off", func(e *restartEnv) func() {
			postJSONMethod[map[string]any](t, e.client, http.MethodPut, e.base+"/api/system/options", map[string]any{"autoRestartWhenStuck": false}, http.StatusOK)
			return func() {}
		}},
		{"nothing would start the app again", func(e *restartEnv) func() {
			e.server.TestSetSupervisor("none")
			return func() {}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newRestartEnv(t)
			x := &exits{}
			e.server.TestSetExitWith(x.record)
			undo := tc.setup(e)
			stop := e.server.TestWatchdog(10*time.Millisecond, 100*time.Millisecond)
			time.Sleep(500 * time.Millisecond)
			stop()
			undo()
			if got := x.list(); len(got) != 0 {
				t.Fatalf("the watchdog restarted the app: %v", got)
			}
			if _, err := os.Stat(filepath.Join(e.dir, "stuck-restart")); err == nil {
				t.Fatal("a note was left although nothing restarted")
			}
		})
	}
}
