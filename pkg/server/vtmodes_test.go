package server

import (
	"strings"
	"testing"
)

func TestVTModeTracker(t *testing.T) {
	tracker := NewVTModeTracker()

	// 1. Initially empty
	if preamble := tracker.ActivePreamble(); len(preamble) != 0 {
		t.Errorf("Expected empty preamble, got %q", string(preamble))
	}

	// 2. Process TUI startup sequence: Alternate Buffer (1049) + Mouse Tracking (1000, 1002, 1006) + Bracketed Paste (2004)
	stream := []byte("Some normal text\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[?2004hHello TUI")
	tracker.Process(stream)

	preamble := string(tracker.ActivePreamble())
	t.Logf("Active Preamble after TUI start: %q", preamble)

	if !strings.Contains(preamble, "\x1b[?1049h") {
		t.Errorf("Expected alternate buffer 1049h in preamble")
	}
	if !strings.Contains(preamble, "\x1b[?1006h") {
		t.Errorf("Expected SGR mouse mode 1006h in preamble")
	}
	if !strings.Contains(preamble, "\x1b[?2004h") {
		t.Errorf("Expected bracketed paste 2004h in preamble")
	}

	// 3. Process TUI exit sequence: reset mouse and exit alternate buffer
	exitStream := []byte("\x1b[?1000l\x1b[?1002l\x1b[?1006l\x1b[?1049lGoodbye TUI")
	tracker.Process(exitStream)

	preambleAfterExit := string(tracker.ActivePreamble())
	t.Logf("Active Preamble after TUI exit: %q", preambleAfterExit)

	if strings.Contains(preambleAfterExit, "1049") || strings.Contains(preambleAfterExit, "1006") {
		t.Errorf("Expected cleared modes after exit, got %q", preambleAfterExit)
	}
	if !strings.Contains(preambleAfterExit, "2004") {
		t.Errorf("Expected 2004h to remain active since it was not reset")
	}
}
