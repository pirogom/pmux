//go:build !windows

package profile

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}
