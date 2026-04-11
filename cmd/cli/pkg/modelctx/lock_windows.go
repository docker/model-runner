//go:build windows

package modelctx

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile acquires an exclusive lock on the given file using LockFileEx.
// The lock is automatically released when the file is closed.
func lockFile(f *os.File) error {
	// LOCKFILE_EXCLUSIVE_LOCK requests an exclusive lock.
	// The zero Overlapped struct locks starting at offset 0.
	ol := new(windows.Overlapped)
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, // reserved
		1, // nNumberOfBytesToLockLow
		0, // nNumberOfBytesToLockHigh
		ol,
	)
}

// unlockFile releases the lock on the given file.
func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0, // reserved
		1, // nNumberOfBytesToUnlockLow
		0, // nNumberOfBytesToUnlockHigh
		ol,
	)
}
