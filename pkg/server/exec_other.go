//go:build !windows

package server

import (
	"os/exec"
)

func prepareCommand(cmd *exec.Cmd) {
	// No-op
}
