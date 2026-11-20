package conpty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConPTYInputExecution(t *testing.T) {
	cmdPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	ptyInst, err := New(cmdPath, []string{}, "", nil, 80, 24)
	if err != nil {
		t.Fatalf("Failed to create ConPTY: %v", err)
	}
	defer ptyInst.Close()

	readChan := make(chan string, 100)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptyInst.OutPipe.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				readChan <- string(buf[:n])
			}
		}
	}()

	// Give ConPTY 500ms to initialize and emit initial prompt
	time.Sleep(500 * time.Millisecond)

	// Write command "echo PMUX_INPUT_OK\r\n" to ConPTY InPipe
	_, err = ptyInst.InPipe.Write([]byte("echo PMUX_INPUT_OK\r\n"))
	if err != nil {
		t.Fatalf("Failed to write to InPipe: %v", err)
	}

	var fullOutput strings.Builder
	timeout := time.After(3 * time.Second)

	for {
		select {
		case chunk := <-readChan:
			fullOutput.WriteString(chunk)
			outputStr := fullOutput.String()
			if strings.Contains(outputStr, "PMUX_INPUT_OK") {
				t.Logf("PASS: Successfully received command echo output:\n%s", outputStr)
				return
			}
		case <-timeout:
			t.Fatalf("FAIL: Timeout waiting for echo output. Accumulated output:\n%q", fullOutput.String())
		}
	}
}

func TestConPTYConcurrentResizeAndForceRedraw(t *testing.T) {
	cmdPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	ptyInst, err := New(cmdPath, []string{}, "", nil, 80, 24)
	if err != nil {
		t.Fatalf("Failed to create ConPTY: %v", err)
	}
	defer ptyInst.Close()

	// Launch multiple concurrent goroutines rapidly invoking Resize and ForceRedraw
	done := make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			_ = ptyInst.Resize(80+idx, 24+idx%5)
			done <- true
		}(i)
		go func(idx int) {
			_ = ptyInst.ForceRedraw(80+idx, 24+idx%5)
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestQuoteWindowsArg(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"cmd.exe", "cmd.exe"},
		{`C:\Windows\System32\cmd.exe`, `C:\Windows\System32\cmd.exe`},
		{`C:\Program Files\Git\bin\bash.exe`, `"C:\Program Files\Git\bin\bash.exe"`},
		{`-Command`, `-Command`},
		{`& {Import-Module 'x'; Enter-VsDevShell}`, `"& {Import-Module 'x'; Enter-VsDevShell}"`},
		{`say "hi"`, `"say \"hi\""`},
		{`C:\dir with space\`, `"C:\dir with space\\"`},
		{"", `""`},
	}
	for _, tc := range tests {
		if got := quoteWindowsArg(tc.in); got != tc.want {
			t.Errorf("quoteWindowsArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildCommandLine(t *testing.T) {
	got := buildCommandLine(`C:\Program Files\Git\bin\bash.exe`, []string{"--login", "-i"})
	want := `"C:\Program Files\Git\bin\bash.exe" --login -i`
	if got != want {
		t.Fatalf("buildCommandLine = %q, want %q", got, want)
	}

	// Empty args are skipped rather than producing empty quoted arguments.
	got = buildCommandLine("cmd.exe", []string{"", "/k"})
	if got != "cmd.exe /k" {
		t.Fatalf("buildCommandLine = %q, want %q", got, "cmd.exe /k")
	}
}
