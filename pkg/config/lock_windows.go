//go:build windows

package config

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on the given file handle. The GUI process
// and the background daemon both write config.json, so a process-local mutex
// alone would allow lost updates.
func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, new(windows.Overlapped))
}

func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}
