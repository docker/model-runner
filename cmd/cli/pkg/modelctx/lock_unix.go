//go:build !windows

package modelctx

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFile acquires an exclusive advisory lock on the given file using flock(2).
// The lock is automatically released when the file is closed.
func lockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX)
}

// unlockFile releases the advisory lock on the given file.
func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
