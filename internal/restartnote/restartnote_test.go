package restartnote

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNotes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName) // does not exist yet
	if TakeSafeOnce(dir) {
		t.Fatal("no safe-once was asked for")
	}
	if err := WriteSafeOnce(dir); err != nil {
		t.Fatal(err)
	}
	if !TakeSafeOnce(dir) {
		t.Fatal("safe-once was not seen")
	}
	if TakeSafeOnce(dir) {
		t.Fatal("safe-once must apply once")
	}

	if _, _, ok := TakeStuck(dir); ok {
		t.Fatal("nothing was recorded")
	}
	if err := WriteStuck(dir, "the database stopped answering\nfor 3 minutes"); err != nil {
		t.Fatal(err)
	}
	reason, at, ok := TakeStuck(dir)
	if !ok || reason != "the database stopped answering for 3 minutes" || time.Since(at) > time.Minute {
		t.Fatalf("TakeStuck = %q, %v, %v", reason, at, ok)
	}
	if _, _, ok := TakeStuck(dir); ok {
		t.Fatal("the mark must be read once")
	}
}
