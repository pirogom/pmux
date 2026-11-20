package server

import (
	"bytes"
	"fmt"
	"strconv"
)

// maxPendingVTEscape bounds the partial escape sequence carried between reads.
const maxPendingVTEscape = 256

// VTModeTracker tracks active DECSET/DECRST private terminal modes (e.g. Alternate Buffer, SGR Mouse, Bracketed Paste)
// so that when a new client attaches to an existing running session, the active modes are re-emitted to put the new
// client's terminal into the identical mode state.
//
// ConPTY output arrives in arbitrary chunks, so a single escape sequence can be
// split across two reads. An incomplete tail is kept in pending and prepended
// to the next chunk; previously such a sequence was silently lost.
type VTModeTracker struct {
	activeModes map[int]bool
	pending     []byte
}

func NewVTModeTracker() *VTModeTracker {
	return &VTModeTracker{
		activeModes: make(map[int]bool),
	}
}

// Process scans a byte stream for DECSET (\x1b[?{params}h) and DECRST (\x1b[?{params}l) sequences.
func (t *VTModeTracker) Process(data []byte) {
	if len(t.pending) > 0 {
		combined := make([]byte, 0, len(t.pending)+len(data))
		combined = append(combined, t.pending...)
		combined = append(combined, data...)
		data = combined
		t.pending = nil
	}

	idx := 0
	for idx < len(data) {
		start := bytes.Index(data[idx:], []byte("\x1b[?"))
		if start == -1 {
			t.pending = carryEscapePrefix(data[idx:])
			return
		}
		start += idx

		pos := start + 3
		paramStart := pos
		for pos < len(data) && ((data[pos] >= '0' && data[pos] <= '9') || data[pos] == ';') {
			pos++
		}
		if pos >= len(data) {
			// The sequence continues in the next chunk.
			t.carry(data[start:])
			return
		}
		if data[pos] == 'h' || data[pos] == 'l' {
			action := data[pos] == 'h'
			paramStr := string(data[paramStart:pos])
			params := bytes.Split([]byte(paramStr), []byte(";"))
			for _, p := range params {
				if len(p) > 0 {
					if modeNum, err := strconv.Atoi(string(p)); err == nil {
						if action {
							t.activeModes[modeNum] = true
						} else {
							delete(t.activeModes, modeNum)
						}
					}
				}
			}
		}
		idx = pos + 1
	}
}

// carry stores an incomplete escape sequence for the next Process call.
func (t *VTModeTracker) carry(partial []byte) {
	if len(partial) == 0 || len(partial) > maxPendingVTEscape {
		t.pending = nil
		return
	}
	t.pending = append(t.pending[:0], partial...)
}

// carryEscapePrefix returns the trailing bytes that could be the beginning of
// a DECSET/DECRST sequence ("\x1b", "\x1b[", "\x1b[?").
func carryEscapePrefix(tail []byte) []byte {
	const prefix = "\x1b[?"
	for n := len(prefix) - 1; n >= 1; n-- {
		if len(tail) >= n && bytes.HasSuffix(tail, []byte(prefix[:n])) {
			return append([]byte(nil), tail[len(tail)-n:]...)
		}
	}
	return nil
}

// ActivePreamble returns the VT escape sequences required to activate all currently enabled modes.
func (t *VTModeTracker) ActivePreamble() []byte {
	if len(t.activeModes) == 0 {
		return nil
	}

	var buf bytes.Buffer
	// Order: Alternate buffer first (47, 1047, 1049), then mouse modes, bracketed paste, etc.
	priorityOrder := []int{47, 1047, 1049, 1, 1000, 1002, 1003, 1004, 1005, 1006, 1015, 2004, 25}
	emitted := make(map[int]bool)

	for _, m := range priorityOrder {
		if t.activeModes[m] {
			buf.WriteString(fmt.Sprintf("\x1b[?%dh", m))
			emitted[m] = true
		}
	}

	for m := range t.activeModes {
		if !emitted[m] {
			buf.WriteString(fmt.Sprintf("\x1b[?%dh", m))
		}
	}

	return buf.Bytes()
}
