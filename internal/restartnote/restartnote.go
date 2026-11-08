// Package restartnote keeps small note files in <config>/restart. They carry a
// message across a restart, from the process that is stopping to the one that
// starts.
package restartnote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DirName is the folder inside the config folder that holds the notes.
const DirName = "restart"

// The note files.
const (
	FileSafeOnce = "safe-once"     // start once with automation paused
	FileStuck    = "stuck-restart" // the app restarted itself because it stopped answering
)

// Dir is the notes folder inside the config folder.
func Dir(configDir string) string { return filepath.Join(configDir, DirName) }

// WriteSafeOnce asks for the next start to run in safe mode, once.
func WriteSafeOnce(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the restart folder: %w", err)
	}
	return writeAtomic(filepath.Join(dir, FileSafeOnce), time.Now().UTC().Format(time.RFC3339)+"\n")
}

// TakeSafeOnce reports whether a safe-mode start was asked for, and removes
// the request so it applies to this start only.
func TakeSafeOnce(dir string) bool {
	path := filepath.Join(dir, FileSafeOnce)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	_ = os.Remove(path)
	return true
}

// WriteStuck records that the app is about to restart itself, and why.
func WriteStuck(dir, reason string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the restart folder: %w", err)
	}
	reason = strings.NewReplacer("\n", " ", "\r", " ").Replace(reason)
	return writeAtomic(filepath.Join(dir, FileStuck), time.Now().UTC().Format(time.RFC3339)+" "+reason+"\n")
}

// TakeStuck returns the reason and time of a restart the app made because it
// stopped answering, and removes the mark.
func TakeStuck(dir string) (reason string, at time.Time, ok bool) {
	path := filepath.Join(dir, FileStuck)
	line := readLine(path)
	if line == "" {
		return "", time.Time{}, false
	}
	_ = os.Remove(path)
	first, rest, _ := strings.Cut(line, " ")
	at, _ = time.Parse(time.RFC3339, first)
	return rest, at, true
}

func readLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > 256 {
		b = b[:256]
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

func writeAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
