//go:build windows

package main

import (
	"os"
	"syscall"
)

func attachConsole() {
	modkernel32 := syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole := modkernel32.NewProc("AttachConsole")

	const ATTACH_PARENT_PROCESS = ^uintptr(0) // (uintptr)-1

	r1, _, _ := procAttachConsole.Call(ATTACH_PARENT_PROCESS)
	if r1 != 0 {
		// Successfully attached to parent console (CMD, PowerShell, Windows Terminal)
		stdout, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0666)
		if err == nil {
			os.Stdout = stdout
		}
		stderr, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0666)
		if err == nil {
			os.Stderr = stderr
		}
	}
}
