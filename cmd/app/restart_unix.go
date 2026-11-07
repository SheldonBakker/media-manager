//go:build !windows

package main

import (
	"log"
	"os"
	"syscall"
)

// restartInPlace replaces this process with a fresh start of the same
// program. It returns only if that fails, and the process then exits as it
// always did.
func restartInPlace() {
	next, err := os.Executable()
	if err != nil {
		log.Printf("Mediarium could not restart in place, so it exits to be started again: %v", err)
		return
	}
	log.Printf("Mediarium is starting again in place: %s", next)
	if err := syscall.Exec(next, append([]string{next}, os.Args[1:]...), os.Environ()); err != nil {
		log.Printf("Mediarium could not restart in place, so it exits to be started again: %v", err)
	}
}
