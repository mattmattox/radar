//go:build windows

package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// conPty wraps a Windows ConPTY pseudo-console
type conPty struct {
	console windows.Handle
	inWrite *os.File // our write end → ConPTY input
	outRead *os.File // our read end ← ConPTY output
}

func (p *conPty) Read(b []byte) (int, error)  { return p.outRead.Read(b) }
func (p *conPty) Write(b []byte) (int, error) { return p.inWrite.Write(b) }

func (p *conPty) Close() error {
	windows.ClosePseudoConsole(p.console)
	err1 := p.inWrite.Close()
	err2 := p.outRead.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (p *conPty) Resize(cols, rows uint16) error {
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func getDefaultShell() string {
	if ps, err := exec.LookPath("powershell.exe"); err == nil {
		return ps
	}
	if comspec := os.Getenv("COMSPEC"); comspec != "" {
		return comspec
	}
	return "cmd.exe"
}

func startShell(shell string, env []string, dir string) (*shellProcess, error) {
	// Create pipes: ConPTY reads from inRead, writes to outWrite.
	// We write to inWrite, read from outRead.
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create input pipe: %w", err)
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inRead.Close()
		inWrite.Close()
		return nil, fmt.Errorf("create output pipe: %w", err)
	}

	// Create the pseudo console
	var hPC windows.Handle
	size := windows.Coord{X: 80, Y: 24}
	err = windows.CreatePseudoConsole(
		size,
		windows.Handle(inRead.Fd()),
		windows.Handle(outWrite.Fd()),
		0,
		&hPC,
	)
	if err != nil {
		inRead.Close()
		inWrite.Close()
		outRead.Close()
		outWrite.Close()
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}

	// Close the pipe ends we gave to the console — they're now owned by ConPTY
	inRead.Close()
	outWrite.Close()

	// Set up process creation with the pseudo console attribute
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		inWrite.Close()
		outRead.Close()
		return nil, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	defer attrs.Delete()

	err = attrs.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(&hPC),
		unsafe.Sizeof(hPC),
	)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		inWrite.Close()
		outRead.Close()
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %w", err)
	}

	// Build the command line
	cmdLine, err := windows.UTF16PtrFromString(shell)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		inWrite.Close()
		outRead.Close()
		return nil, err
	}

	// Build environment block
	var envBlock *uint16
	if len(env) > 0 {
		envBlock = createEnvBlock(env)
	}

	// Build working directory
	var dirPtr *uint16
	if dir != "" {
		dirPtr, _ = windows.UTF16PtrFromString(dir)
	}

	// Create the process
	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
		},
		ProcThreadAttributeList: attrs.List(),
	}

	pi := &windows.ProcessInformation{}

	err = windows.CreateProcess(
		nil,
		cmdLine,
		nil,
		nil,
		false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT,
		envBlock,
		dirPtr,
		&si.StartupInfo,
		pi,
	)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		inWrite.Close()
		outRead.Close()
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}

	// Close handles we don't need
	windows.CloseHandle(pi.Thread)

	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		inWrite.Close()
		outRead.Close()
		windows.CloseHandle(pi.Process)
		return nil, fmt.Errorf("FindProcess: %w", err)
	}

	// FindProcess opened its own handle; close the one from CreateProcess
	windows.CloseHandle(pi.Process)

	return &shellProcess{
		pty: &conPty{
			console: hPC,
			inWrite: inWrite,
			outRead: outRead,
		},
		process: proc,
	}, nil
}

func signalProcess(p *os.Process) {
	// Windows doesn't have SIGHUP; just kill the process
	p.Kill()
}

func waitProcess(proc *shellProcess) {
	proc.process.Wait()
}

// createEnvBlock builds a Windows environment block (null-terminated UTF-16 strings, double-null terminated)
func createEnvBlock(env []string) *uint16 {
	// Calculate total length
	total := 0
	for _, s := range env {
		total += len(s) + 1 // each string + null terminator
	}
	total++ // final double-null

	block := make([]uint16, 0, total*2) // rough estimate, UTF-16 may expand
	for _, s := range env {
		u, _ := windows.UTF16FromString(s)
		block = append(block, u...)
	}
	block = append(block, 0) // final double-null

	return &block[0]
}

// Ensure conPty satisfies ptyHandle at compile time
var _ ptyHandle = (*conPty)(nil)

// Ensure io interfaces are satisfied
var _ io.ReadWriteCloser = (*conPty)(nil)
