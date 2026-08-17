package server

import (
	"bytes"
	"testing"
)

func TestSanitizeHistoryBuffer(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "Normal plain text",
			input:    []byte("hello world\r\n"),
			expected: []byte("hello world\r\n"),
		},
		{
			name:     "Valid ANSI sequence preserved",
			input:    []byte("\x1b[31mhello\x1b[0m"),
			expected: []byte("\x1b[31mhello\x1b[0m"),
		},
		{
			name:     "Leading broken UTF-8 continuation bytes",
			input:    []byte{0x80, 0xbf, 'a', 'b', 'c'},
			expected: []byte("abc"),
		},
		{
			name:     "Broken ANSI CSI parameter without ESC",
			input:    []byte("[38;2;255;0;0mHello"),
			expected: []byte("Hello"),
		},
		{
			name:     "Broken ANSI parameters without ESC or bracket",
			input:    []byte("38;2;255;0;0mHello"),
			expected: []byte("Hello"),
		},
		{
			name:     "Broken DEC mode fragment",
			input:    []byte("?1049hHello"),
			expected: []byte("Hello"),
		},
		{
			name:     "DSR cursor query stripped",
			input:    []byte("Prompt>\x1b[6nAfter"),
			expected: []byte("Prompt>After"),
		},
		{
			name:     "OSC 10/11 color query stripped",
			input:    []byte("\x1b]10;?\x07\x1b]11;?\x1b\\Start"),
			expected: []byte("Start"),
		},
		{
			name:     "OSC 4 palette query stripped",
			input:    []byte("\x1b]4;0;?\x07Line"),
			expected: []byte("Line"),
		},
		{
			name:     "DECRQM query stripped",
			input:    []byte("A\x1b[?2004$pB\x1b[?1016$pC"),
			expected: []byte("ABC"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := SanitizeHistoryBuffer(tc.input)
			if !bytes.Equal(actual, tc.expected) {
				t.Errorf("SanitizeHistoryBuffer() = %q, expected %q", actual, tc.expected)
			}
		})
	}
}

func TestRingBufferWrapAroundSanitization(t *testing.T) {
	rb := NewRingBuffer(20)
	// Write initial data
	_, _ = rb.Write([]byte("1234567890\x1b[31mABCD")) // 16 bytes
	// Overwrite causing wrap-around that cuts into \x1b[31m
	_, _ = rb.Write([]byte("1234567890")) // 10 bytes -> total pushed 26 > 20

	out := rb.Bytes()
	// Should produce clean output without broken '[' or partial ANSI code
	if bytes.Contains(out, []byte("[31m")) {
		t.Errorf("Expected truncated ANSI to be sanitized, got: %q", out)
	}
}
