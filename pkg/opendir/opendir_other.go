//go:build !windows

package opendir

import "os/exec"

// Open opens the given directory in the default file manager.
func Open(path string) error {
	if path == "" {
		return nil
	}
	return exec.Command("xdg-open", path).Start()
}
