package conpty

import (
	"fmt"
	"io"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                               = windows.NewLazySystemDLL("kernel32.dll")
	procCreatePseudoConsole                = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole                = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole                 = kernel32.NewProc("ClosePseudoConsole")
	procInitializeProcThreadAttributeList  = kernel32.NewProc("InitializeProcThreadAttributeList")
	procDeleteProcThreadAttributeList      = kernel32.NewProc("DeleteProcThreadAttributeList")
	procUpdateProcThreadAttribute          = kernel32.NewProc("UpdateProcThreadAttribute")
)

type HPCON windows.Handle

type ConPTY struct {
	hPC        HPCON
	InPipe     io.WriteCloser
	OutPipe    io.ReadCloser
	ProcHandle windows.Handle
	Pid        int
}

func New(command string, args []string, dir string, env []string, cols, rows int) (*ConPTY, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	// 1. Create Input and Output Pipes for ConPTY
	// inPipe: Host writes to inPipeWrite -> ConPTY reads from inPipeRead
	// outPipe: ConPTY writes to outPipeWrite -> Host reads from outPipeRead
	var inPipeRead, inPipeWrite windows.Handle
	var outPipeRead, outPipeWrite windows.Handle

	if err := windows.CreatePipe(&inPipeRead, &inPipeWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe (in) failed: %w", err)
	}
	if err := windows.CreatePipe(&outPipeRead, &outPipeWrite, nil, 0); err != nil {
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

	err = windows.CreateProcess(
		nil,
		cmdLineUTF16,
		nil,
		nil,
		false,
		creationFlags,
		nil,
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
	jitterCols := cols + 1
	if jitterCols > 300 {
		jitterCols = cols - 1
	}
	_ = c.Resize(jitterCols, rows)
	time.Sleep(15 * time.Millisecond)
	return c.Resize(cols, rows)
}

func (c *ConPTY) Close() error {
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
		_ = windows.TerminateProcess(c.ProcHandle, 1)
		windows.CloseHandle(c.ProcHandle)
		c.ProcHandle = 0
	}
	return nil
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
