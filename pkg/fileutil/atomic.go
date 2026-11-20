// Package fileutil provides small filesystem helpers shared by the config,
// note, todo and ssh stores.
package fileutil

import (
	"fmt"
	"os"
	"time"
)

// WriteFileAtomic writes data to path via a temp file in the same directory
// followed by a rename, so a crash can never leave a truncated file behind.
// On Windows a rename over a file that another process has open can fail
// transiently, so it is retried briefly before falling back to an in-place
// write.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}

	var renameErr error
	for i := 0; i < 5; i++ {
		if renameErr = os.Rename(tmp, path); renameErr == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Fall back to a direct write; the temp file is cleaned up either way.
	if err := os.WriteFile(path, data, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

// BackupCorrupt preserves an unparsable file next to the original as
// <path>.corrupt-<unix timestamp> and returns the backup path.
func BackupCorrupt(path string, data []byte) (string, error) {
	backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
	if err := os.WriteFile(backup, data, 0600); err != nil {
		return "", err
	}
	return backup, nil
}
