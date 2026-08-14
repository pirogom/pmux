//go:build !windows

package git

import (
	"os/exec"
)

func prepareCommand(cmd *exec.Cmd) {
	// No-op for non-Windows OS
}
