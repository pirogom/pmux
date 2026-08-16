package conpty

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	conptyassets "pmux/resources/conpty"
)

const createUnicodeEnvironment = 0x00000400

// createEnvBlock builds a double-NUL-terminated UTF16 environment block from
// "KEY=VALUE" strings, as required by CreateProcess when a custom environment
// is supplied. Returns nil (meaning "inherit caller's environment") if env is empty.
func createEnvBlock(env []string) *uint16 {
	if len(env) == 0 {
		return nil
	}
	var buf []uint16
	for _, s := range env {
		buf = append(buf, utf16.Encode([]rune(s))...)
		buf = append(buf, 0)
	}
	buf = append(buf, 0)
	return &buf[0]
}

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procInitializeProcThreadAttributeList = kernel32.NewProc("InitializeProcThreadAttributeList")
	procDeleteProcThreadAttributeList     = kernel32.NewProc("DeleteProcThreadAttributeList")
	procUpdateProcThreadAttribute         = kernel32.NewProc("UpdateProcThreadAttribute")

	// procCreatePseudoConsole/procResizePseudoConsole/procClosePseudoConsole are
	// resolved lazily on first use (see pseudoConsoleProcs) rather than at
	// package-init time, so the log line announcing which DLL got picked isn't
	// emitted before main() has had a chance to attach/redirect the console
	// (see attachConsole in console_windows.go).
	pseudoConsoleDLLOnce sync.Once
	procCreatePseudoConsole *windows.LazyProc
	procResizePseudoConsole *windows.LazyProc
	procClosePseudoConsole  *windows.LazyProc
)

// pseudoConsoleProcs resolves CreatePseudoConsole/ResizePseudoConsole/ClosePseudoConsole,
// preferring the bundled redistributable conpty.dll (paired with OpenConsole.exe,
// see resources/conpty/) over the OS's own kernel32.dll/system conhost.exe:
// MSYS2 -defterm shells have been observed losing their ConPTY connection
// (OutPipe EOF while the root process is still alive) under the system
// conhost.exe in a way that doesn't reproduce under Windows Terminal, which
// always uses its own more frequently-updated OpenConsole.exe build.
func pseudoConsoleProcs() {
	pseudoConsoleDLLOnce.Do(func() {
		dll := loadPseudoConsoleDLL()
		procCreatePseudoConsole = dll.NewProc("CreatePseudoConsole")
		procResizePseudoConsole = dll.NewProc("ResizePseudoConsole")
		procClosePseudoConsole = dll.NewProc("ClosePseudoConsole")
	})
}

// loadPseudoConsoleDLL resolves which conpty.dll to use, in order:
//  1. conpty.dll sitting next to the running executable (manual/legacy placement).
//  2. The architecture-matched conpty.dll+OpenConsole.exe embedded in the pmux
//     binary (see resources/conpty/), extracted/refreshed under ~/.pmux/bin/{64,32}
//     so pmux keeps working as a single portable executable.
//  3. The OS's system kernel32.dll (default conhost.exe-backed ConPTY), if
//     neither of the above is usable, so pmux still works either way.
func loadPseudoConsoleDLL() *windows.LazyDLL {
	if exe, err := os.Executable(); err == nil {
		adjacent := filepath.Join(filepath.Dir(exe), "conpty.dll")
		if _, err := os.Stat(adjacent); err == nil {
			if dll := windows.NewLazyDLL(adjacent); dll.Load() == nil {
				log.Printf("[conpty] using ConPTY host next to executable: %s", adjacent)
				return dll
			} else {
				log.Printf("[conpty] found %s but failed to load it, trying embedded ConPTY host next", adjacent)
			}
		}
	}

	if dllPath, ok := ensureBundledConPTY(); ok {
		dll := windows.NewLazyDLL(dllPath)
		if err := dll.Load(); err == nil {
			log.Printf("[conpty] using extracted ConPTY host: %s", dllPath)
			return dll
		}
		log.Printf("[conpty] failed to load extracted conpty.dll at %s, falling back to system kernel32.dll", dllPath)
	}

	log.Printf("[conpty] using system kernel32.dll for ConPTY")
	return kernel32
}

// archDir maps the build architecture to the subdirectory name used under
// ~/.pmux/bin, matching which pair (x64/x86) got embedded at build time
// (see resources/conpty/embed_amd64.go and embed_386.go).
func archDir() (string, bool) {
	switch runtime.GOARCH {
	case "amd64":
		return "64", true
	case "386":
		return "32", true
	default:
		return "", false
	}
}

// ensureBundledConPTY extracts the embedded conpty.dll+OpenConsole.exe pair
// to ~/.pmux/bin/{64,32} if missing or stale (content hash mismatch), and
// returns the path to conpty.dll there. ok is false if extraction isn't
// possible (unsupported architecture, no home dir, write failure) — callers
// should fall back to the system kernel32.dll in that case.
func ensureBundledConPTY() (dllPath string, ok bool) {
	dir, supported := archDir()
	if !supported {
		log.Printf("[conpty] no embedded ConPTY host for GOARCH=%s", runtime.GOARCH)
		return "", false
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[conpty] cannot resolve home directory for embedded ConPTY extraction: %v", err)
		return "", false
	}

	destDir := filepath.Join(home, ".pmux", "bin", dir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		log.Printf("[conpty] cannot create %s: %v", destDir, err)
		return "", false
	}

	dllPath = filepath.Join(destDir, "conpty.dll")
	exePath := filepath.Join(destDir, "OpenConsole.exe")

	// A write/rename failure (e.g. the destination is locked by another
	// running pmux instance) isn't fatal as long as a usable file is already
	// there from a previous run — only fail outright if it's missing.
	if err := writeIfDifferent(dllPath, conptyassets.ConptyDLL); err != nil {
		if _, statErr := os.Stat(dllPath); statErr != nil {
			log.Printf("[conpty] failed to extract %s and no existing copy to fall back to: %v", dllPath, err)
			return "", false
		}
		log.Printf("[conpty] failed to refresh %s, using existing copy on disk: %v", dllPath, err)
	}
	if err := writeIfDifferent(exePath, conptyassets.OpenConsoleEXE); err != nil {
		if _, statErr := os.Stat(exePath); statErr != nil {
			log.Printf("[conpty] failed to extract %s and no existing copy to fall back to: %v", exePath, err)
			return "", false
		}
		log.Printf("[conpty] failed to refresh %s, using existing copy on disk: %v", exePath, err)
	}

	return dllPath, true
}

// writeIfDifferent writes want to path only if the file doesn't already
// exist with matching content (by SHA256), replacing it atomically via a
// temp file + rename. If the destination is locked by another process (e.g.
// another running pmux instance with it loaded), the write/rename error is
// returned so the caller can fall back to whatever's already on disk instead
// of treating it as fatal.
func writeIfDifferent(path string, want []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if sha256.Sum256(existing) == sha256.Sum256(want) {
			return nil
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, want, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

type HPCON windows.Handle

type ConPTY struct {
	hPC        HPCON
	InPipe     io.WriteCloser
	OutPipe    io.ReadCloser
	ProcHandle windows.Handle
	Pid        int
	resizeMu   sync.Mutex
}

func New(command string, args []string, dir string, env []string, cols, rows int) (*ConPTY, error) {
	pseudoConsoleProcs()

	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	// 1. Create Input and Output Pipes for ConPTY (1MB buffer to absorb high-speed bursts)
	// inPipe: Host writes to inPipeWrite -> ConPTY reads from inPipeRead
	// outPipe: ConPTY writes to outPipeWrite -> Host reads from outPipeRead
	var inPipeRead, inPipeWrite windows.Handle
	var outPipeRead, outPipeWrite windows.Handle
	const pipeBufferSize = 1024 * 1024

	if err := windows.CreatePipe(&inPipeRead, &inPipeWrite, nil, pipeBufferSize); err != nil {
		return nil, fmt.Errorf("CreatePipe (in) failed: %w", err)
	}
	if err := windows.CreatePipe(&outPipeRead, &outPipeWrite, nil, pipeBufferSize); err != nil {
		windows.CloseHandle(inPipeRead)
		windows.CloseHandle(inPipeWrite)
		return nil, fmt.Errorf("CreatePipe (out) failed: %w", err)
	}

	// 2. Create Pseudo Console
	coordVal := uintptr(uint32(cols&0xFFFF) | (uint32(rows&0xFFFF) << 16))
	var hPC HPCON
	r1, _, err := procCreatePseudoConsole.Call(
		coordVal,
		uintptr(inPipeRead),
		uintptr(outPipeWrite),
		0,
		uintptr(unsafe.Pointer(&hPC)),
	)
	if r1 != 0 {
		windows.CloseHandle(inPipeRead)
		windows.CloseHandle(inPipeWrite)
		windows.CloseHandle(outPipeRead)
		windows.CloseHandle(outPipeWrite)
		return nil, fmt.Errorf("CreatePseudoConsole failed (HRESULT 0x%x): %v", r1, err)
	}

	// Close host-side references to internal ends immediately per ConPTY spec
	windows.CloseHandle(inPipeRead)
	windows.CloseHandle(outPipeWrite)

	// 3. Initialize Thread Attribute List
	var attrListSize uintptr
	_, _, _ = procInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrListSize)))

	attrListBuffer := make([]byte, attrListSize)
	attrListPtr := uintptr(unsafe.Pointer(&attrListBuffer[0]))

	rInit, _, errInit := procInitializeProcThreadAttributeList.Call(attrListPtr, 1, 0, uintptr(unsafe.Pointer(&attrListSize)))
	if rInit == 0 {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(inPipeWrite)
		windows.CloseHandle(outPipeRead)
		return nil, fmt.Errorf("InitializeProcThreadAttributeList failed: %v", errInit)
	}
	defer procDeleteProcThreadAttributeList.Call(attrListPtr)

	const PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
	rUpdate, _, errUpdate := procUpdateProcThreadAttribute.Call(
		attrListPtr,
		0,
		PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		uintptr(hPC),
		unsafe.Sizeof(hPC),
		0,
		0,
	)
	if rUpdate == 0 {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(inPipeWrite)
		windows.CloseHandle(outPipeRead)
		return nil, fmt.Errorf("UpdateProcThreadAttribute failed: %v", errUpdate)
	}

	// 4. Build Command Line string
	cmdLineStr := command
	for _, arg := range args {
		if arg != "" {
			cmdLineStr += " " + arg
		}
	}
	cmdLineUTF16, err := windows.UTF16PtrFromString(cmdLineStr)
	if err != nil {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(inPipeWrite)
		windows.CloseHandle(outPipeRead)
		return nil, err
	}

	var dirUTF16 *uint16
	if dir != "" {
		dirUTF16, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			procClosePseudoConsole.Call(uintptr(hPC))
			windows.CloseHandle(inPipeWrite)
			windows.CloseHandle(outPipeRead)
			return nil, err
		}
	}

	type STARTUPINFOEXW struct {
		windows.StartupInfo
		AttributeList uintptr
	}

	var siEx STARTUPINFOEXW
	siEx.Cb = uint32(unsafe.Sizeof(siEx))
	siEx.Flags = windows.STARTF_USESTDHANDLES
	siEx.StdInput = windows.Handle(hPC)
	siEx.StdOutput = windows.Handle(hPC)
	siEx.StdErr = windows.Handle(hPC)
	siEx.AttributeList = attrListPtr

	var pi windows.ProcessInformation

	creationFlags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT)

	envBlock := createEnvBlock(env)
	if envBlock != nil {
		creationFlags |= createUnicodeEnvironment
	}

	err = windows.CreateProcess(
		nil,
		cmdLineUTF16,
		nil,
		nil,
		false,
		creationFlags,
		envBlock,
		dirUTF16,
		(*windows.StartupInfo)(unsafe.Pointer(&siEx)),
		&pi,
	)
	if err != nil {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(inPipeWrite)
		windows.CloseHandle(outPipeRead)
		return nil, fmt.Errorf("CreateProcess failed for %s: %w", command, err)
	}

	windows.CloseHandle(pi.Thread)

	inPipeFile := os.NewFile(uintptr(inPipeWrite), "conpty-in")
	outPipeFile := os.NewFile(uintptr(outPipeRead), "conpty-out")

	return &ConPTY{
		hPC:        hPC,
		InPipe:     inPipeFile,
		OutPipe:    outPipeFile,
		ProcHandle: pi.Process,
		Pid:        int(pi.ProcessId),
	}, nil
}

func (c *ConPTY) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	c.resizeMu.Lock()
	defer c.resizeMu.Unlock()

	return c.resizeLocked(cols, rows)
}

func (c *ConPTY) resizeLocked(cols, rows int) error {
	coordVal := uintptr(uint32(cols&0xFFFF) | (uint32(rows&0xFFFF) << 16))
	r1, _, err := procResizePseudoConsole.Call(
		uintptr(c.hPC),
		coordVal,
	)
	if r1 != 0 {
		return fmt.Errorf("ResizePseudoConsole error (0x%x): %v", r1, err)
	}
	return nil
}

// ForceRedraw momentarily alters cols to force Win32 ConPTY to emit a full screen redraw stream.
func (c *ConPTY) ForceRedraw(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	c.resizeMu.Lock()
	defer c.resizeMu.Unlock()

	jitterCols := cols + 1
	if jitterCols > 300 {
		jitterCols = cols - 1
	}
	_ = c.resizeLocked(jitterCols, rows)
	time.Sleep(15 * time.Millisecond)
	return c.resizeLocked(cols, rows)
}

func (c *ConPTY) Close() error {
	return c.close(true)
}

// CloseKeepProcess tears down the pseudo console and pipes without
// terminating the root process. Use when the process is known to still be
// running (STILL_ACTIVE) despite the ConPTY connection having gone away, so
// pmux doesn't kill a process it merely lost the ability to talk to.
func (c *ConPTY) CloseKeepProcess() error {
	return c.close(false)
}

func (c *ConPTY) close(terminateProcess bool) error {
	if c.InPipe != nil {
		c.InPipe.Close()
	}
	if c.OutPipe != nil {
		c.OutPipe.Close()
	}
	if c.hPC != 0 {
		procClosePseudoConsole.Call(uintptr(c.hPC))
		c.hPC = 0
	}
	if c.ProcHandle != 0 {
		if terminateProcess {
			_ = windows.TerminateProcess(c.ProcHandle, 1)
		}
		windows.CloseHandle(c.ProcHandle)
		c.ProcHandle = 0
	}
	return nil
}

// ExitCode returns the current exit code of the root process without blocking.
// Returns (259 /* STILL_ACTIVE */, nil) if the process has not exited yet.
func (c *ConPTY) ExitCode() (uint32, error) {
	if c.ProcHandle == 0 {
		return 0, fmt.Errorf("no process handle")
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(c.ProcHandle, &exitCode); err != nil {
		return 0, err
	}
	return exitCode, nil
}

func (c *ConPTY) Wait() (uint32, error) {
	if c.ProcHandle == 0 {
		return 0, nil
	}
	event, err := windows.WaitForSingleObject(c.ProcHandle, windows.INFINITE)
	if err != nil {
		return 0, err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(c.ProcHandle, &exitCode); err != nil {
		return 0, err
	}
	_ = event
	return exitCode, nil
}
