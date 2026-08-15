//go:build 386

// Package conptyassets embeds the redistributable ConPTY host (conpty.dll +
// OpenConsole.exe) matching the build's architecture, so pmux can run as a
// single portable executable without an installer placing them alongside it.
// See pkg/conpty/conpty_windows.go for how these are extracted to disk and
// loaded.
package conptyassets

import _ "embed"

//go:embed x86/conpty.dll
var ConptyDLL []byte

//go:embed x86/OpenConsole.exe
var OpenConsoleEXE []byte
