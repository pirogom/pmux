package conpty

import "strings"

// quoteWindowsArg renders a single argument following the CommandLineToArgvW
// rules that the Windows CRT / CreateProcess use to split a command line.
//
// Without this, an executable path containing spaces ("C:\Program Files\...")
// could resolve to the wrong binary, and arguments containing spaces, quotes
// or trailing backslashes were passed through verbatim.
func quoteWindowsArg(arg string) string {
	if arg == "" {
		return `""`
	}

	needsQuoting := false
	for _, r := range arg {
		if r == ' ' || r == '\t' || r == '"' {
			needsQuoting = true
			break
		}
	}
	if !needsQuoting {
		return arg
	}

	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			// Escape the backslash run plus the quote itself.
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteByte('"')
			backslashes = 0
		default:
			b.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
			b.WriteRune(r)
		}
	}
	// Backslashes right before the closing quote must be doubled.
	b.WriteString(strings.Repeat(`\`, backslashes*2))
	b.WriteByte('"')
	return b.String()
}

// buildCommandLine assembles a CreateProcess command line from an executable
// path and its arguments, quoting each component.
func buildCommandLine(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	if command != "" {
		parts = append(parts, quoteWindowsArg(command))
	}
	for _, arg := range args {
		if arg == "" {
			continue
		}
		parts = append(parts, quoteWindowsArg(arg))
	}
	return strings.Join(parts, " ")
}
