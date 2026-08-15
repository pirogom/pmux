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

