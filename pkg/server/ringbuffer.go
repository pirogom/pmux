package server

import (
	"bytes"
	"strings"
	"sync"
)

type RingBuffer struct {
	buf     []byte
	size    int
	maxSize int
	head    int
	mu      sync.RWMutex
}

func NewRingBuffer(maxSize int) *RingBuffer {
	if maxSize <= 0 {
		maxSize = 512 * 1024 // Default 512KB
	}
	return &RingBuffer{
		buf:     make([]byte, maxSize),
		maxSize: maxSize,
	}
}

func (r *RingBuffer) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n = len(p)
	if n == 0 {
		return 0, nil
	}

	for _, b := range p {
		r.buf[r.head] = b
		r.head = (r.head + 1) % r.maxSize
		if r.size < r.maxSize {
			r.size++
		}
	}
	return n, nil
}

func (r *RingBuffer) Bytes() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return []byte{}
	}

	result := make([]byte, r.size)
	if r.size < r.maxSize {
		copy(result, r.buf[:r.size])
	} else {
		// Circular copy when full
		tail := r.head
		copy(result, r.buf[tail:])
		copy(result[r.maxSize-tail:], r.buf[:tail])
	}

	return SanitizeHistoryBuffer(result)
}

// SanitizeHistoryBuffer strips leading broken UTF-8 continuation bytes, partial ANSI escape sequences,
// and historical terminal queries (DSR, DA, OSC 10/11/4, DECRQM) to prevent xterm.js auto-reply echo loops.
func SanitizeHistoryBuffer(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	idx := 0
	maxScan := 128
	if maxScan > len(data) {
		maxScan = len(data)
	}

	// 1. Skip leading UTF-8 continuation bytes (0x80 - 0xBF)
	for idx < maxScan && (data[idx]&0xC0) == 0x80 {
		idx++
	}

	// 2. Check if the remaining prefix is a broken ANSI CSI/DEC/OSC fragment lacking the initial ESC (\x1b)
	// Example broken fragments: "[38;2;255;0;0m", "38;2;255;0;0m", "?1049h", "[?25h"
	if idx < maxScan && data[idx] != 0x1b {
		scanIdx := idx
		if scanIdx < maxScan && data[scanIdx] == '[' {
			scanIdx++
		}
		if scanIdx < maxScan && data[scanIdx] == '?' {
			scanIdx++
		}

		isParamChar := func(b byte) bool {
			return (b >= '0' && b <= '9') || b == ';' || b == '?' || b == ' '
		}

		paramCount := 0
		for scanIdx < maxScan && isParamChar(data[scanIdx]) {
			paramCount++
			scanIdx++
		}

		// If followed by an ANSI command terminator byte (e.g. 'm', 'h', 'l', 'H', 'J', 'K', 'r')
		if paramCount > 0 && scanIdx < maxScan {
			term := data[scanIdx]
			if (term >= 'A' && term <= 'Z') || (term >= 'a' && term <= 'z') || term == '~' {
				// Fragment detected: skip past this broken terminator
				idx = scanIdx + 1
			}
		}
	}

	cleaned := data
	if idx > 0 && idx < len(data) {
		cleaned = data[idx:]
	}

	// 3. Strip one-shot historical terminal query escape sequences so xterm.js won't generate auto-replies
	return StripTerminalQueries(cleaned)
}

// StripTerminalQueries removes terminal status queries (DSR, DA, OSC color queries, DECRQM) from history.
func StripTerminalQueries(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// Fast path: if there is no ESC byte, return directly
	if !bytes.ContainsRune(data, 0x1b) {
		return data
	}

	var buf bytes.Buffer
	buf.Grow(len(data))

	i := 0
	for i < len(data) {
		if data[i] == 0x1b && i+1 < len(data) {
			// Check CSI queries: \x1b[...
			if data[i+1] == '[' {
				// 1. DSR: \x1b[6n or \x1b[5n
				if i+3 < len(data) && (data[i+2] == '6' || data[i+2] == '5') && data[i+3] == 'n' {
					i += 4
					continue
				}
				// 2. Primary / Secondary DA: \x1b[c, \x1b[0c, \x1b[>c, \x1b[>0c
				if i+2 < len(data) && data[i+2] == 'c' {
					i += 3
					continue
				}
				if i+3 < len(data) && (data[i+2] == '0' || data[i+2] == '>') && data[i+3] == 'c' {
					i += 4
					continue
				}
				if i+4 < len(data) && data[i+2] == '>' && data[i+3] == '0' && data[i+4] == 'c' {
					i += 5
					continue
				}
				// 3. DECRQM queries: \x1b[?<num>$p or \x1b[<num>$p
				if i+2 < len(data) {
					p := i + 2
					if p < len(data) && data[p] == '?' {
						p++
					}
					numStart := p
					for p < len(data) && data[p] >= '0' && data[p] <= '9' {
						p++
					}
					if p > numStart && p+1 < len(data) && data[p] == '$' && data[p+1] == 'p' {
						i = p + 2
						continue
					}
				}
			} else if data[i+1] == ']' {
				// Check OSC queries: \x1b]10;? / \x1b]11;? / \x1b]4;...;? terminated by \x07 (BEL) or \x1b\ (ST)
				p := i + 2
				for p < len(data) && data[p] >= '0' && data[p] <= '9' {
					p++
				}
				if p < len(data) && data[p] == ';' {
					// Scan until terminator
					termPos := -1
					termLen := 0
					for j := p + 1; j < len(data) && j < p+64; j++ {
						if data[j] == 0x07 { // BEL
							termPos = j
							termLen = 1
							break
						}
						if data[j] == 0x1b && j+1 < len(data) && data[j+1] == '\\' { // ST
							termPos = j
							termLen = 2
							break
						}
						if data[j] == '\n' || data[j] == '\r' {
							break
						}
					}
					if termPos != -1 {
						oscPayload := string(data[p+1 : termPos])
						// If query contains '?' (e.g. "?", "0;?", "1;?")
						if strings.Contains(oscPayload, "?") {
							i = termPos + termLen
							continue
						}
					}
				}
			}
		}

		buf.WriteByte(data[i])
		i++
	}

	return buf.Bytes()
}


