package entrypoint

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests run the real docker/entrypoint.sh in a fake container: a
// temporary config folder, media folders and an "app" that is a small script.

type entry struct {
	t    *testing.T
	root string // fake container: app, config/ and the media folders
	app  string // the program the entrypoint starts
}

// newEntry makes a fake container whose program prints the umask it was
// started with.
func newEntry(t *testing.T) *entry {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the entrypoint runs on Linux")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("needs sh")
	}
	root, err := os.MkdirTemp("", "mediarium-container-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	os.Chmod(root, 0o777) // an ordinary user must be able to get in
	for _, d := range []string{"config", "downloads", "movies", "tv"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o777); err != nil {
			t.Fatal(err)
		}
		os.Chmod(filepath.Join(root, d), 0o777)
	}
	e := &entry{t: t, root: root, app: filepath.Join(root, "app")}
	if err := os.WriteFile(e.app, []byte("#!/bin/sh\numask\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(e.app, 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

// run starts the entrypoint (as an ordinary user, like a container started
// with a user: setting) with the app as its command.
func (e *entry) run(env []string) (stdout, stderr string, code int) {
	e.t.Helper()
	script, err := filepath.Abs("../../docker/entrypoint.sh")
	if err != nil {
		e.t.Fatal(err)
	}
	var cmd *exec.Cmd
	if os.Getuid() == 0 {
		// Run as user 1000 so the script takes its non-root path, as it does in
		// a container started with "user: 1000:1000".
		if _, err := exec.LookPath("setpriv"); err != nil {
			e.t.Skip("running as root needs setpriv to drop to an ordinary user")
		}
		cmd = exec.Command("setpriv", "--reuid=1000", "--regid=1000", "--clear-groups", "sh", script, e.app)
	} else {
		cmd = exec.Command("sh", script, e.app)
	}
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"CONFIG_DIR=" + filepath.Join(e.root, "config"),
		"DOWNLOADS_DIR=" + filepath.Join(e.root, "downloads"),
		"MOVIES_DIR=" + filepath.Join(e.root, "movies"),
		"TV_DIR=" + filepath.Join(e.root, "tv"),
		"PUID=1000", "PGID=1000",
	}, env...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	code = 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		e.t.Fatalf("run entrypoint: %v", err)
	}
	return so.String(), se.String(), code
}

// UMASK is set before the app starts, the way the linuxserver.io images do it.
func TestEntrypointAppliesUMASK(t *testing.T) {
	e := newEntry(t)

	umaskWith := func(env ...string) (string, string) {
		t.Helper()
		out, errText, code := e.run(env)
		if code != 0 {
			t.Fatalf("entrypoint exited %d: %s", code, errText)
		}
		return strings.TrimSpace(out), errText
	}

	baseline, errText := umaskWith()
	if strings.Contains(errText, "UMASK") {
		t.Errorf("no UMASK is set, yet it says: %s", errText)
	}

	cases := []struct {
		name     string
		env      string
		wantMask string // as `umask` prints it, four digits; "" = unchanged
		wantNote string
	}{
		{name: "002", env: "UMASK=002", wantMask: "0002"},
		{name: "022", env: "UMASK=022", wantMask: "0022"},
		{name: "four digits", env: "UMASK=0077", wantMask: "0077"},
		{name: "not octal is ignored with a note", env: "UMASK=089", wantNote: "ignoring UMASK=089"},
		{name: "letters are ignored with a note", env: "UMASK=u=rwx", wantNote: "ignoring UMASK=u=rwx"},
		{name: "too long is ignored with a note", env: "UMASK=00022", wantNote: "ignoring UMASK=00022"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, errText := umaskWith(c.env)
			want := c.wantMask
			if want == "" {
				want = baseline
			}
			if got != want {
				t.Errorf("umask = %q, want %q (stderr: %s)", got, want, errText)
			}
			if c.wantNote != "" && !strings.Contains(errText, c.wantNote) {
				t.Errorf("stderr %q does not say %q", errText, c.wantNote)
			}
			if c.wantNote == "" && strings.Contains(errText, "UMASK") {
				t.Errorf("unexpected note about UMASK: %s", errText)
			}
		})
	}
}
