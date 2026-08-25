//go:build windows

package opendir

import "os/exec"

// Open opens the given directory in the default file manager (Windows Explorer).
func Open(path string) error {
	if path == "" {
		return nil
	}
	return exec.Command("explorer.exe", path).Start()
}
