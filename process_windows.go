//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

const createNoWindow = 0x08000000

const (
	jobObjectExtendedLimitInfoClass = 9
	jobObjectLimitKillOnJobClose    = 0x00002000
	processTerminate                = 0x0001
	processSetQuota                 = 0x0100
)

var (
	processKernel32          = syscall.NewLazyDLL("kernel32.dll")
	createJobObjectW         = processKernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = processKernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = processKernel32.NewProc("AssignProcessToJobObject")
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func hideCommandWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// attachKillOnCloseJob makes the companion the lifetime owner of command.
// Closing the returned handle, including when Windows tears it down after a
// crash, terminates the command and any descendants it created.
func attachKillOnCloseJob(command *exec.Cmd) (func(), error) {
	if command.Process == nil {
		return nil, fmt.Errorf("process has not started")
	}
	job, _, callErr := createJobObjectW.Call(0, 0)
	if job == 0 {
		return nil, fmt.Errorf("CreateJobObjectW: %w", callErr)
	}
	closeJob := func() { _ = syscall.CloseHandle(syscall.Handle(job)) }
	limits := jobObjectExtendedLimitInformation{}
	limits.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	ok, _, callErr := setInformationJobObject.Call(
		job,
		jobObjectExtendedLimitInfoClass,
		uintptr(unsafe.Pointer(&limits)),
		unsafe.Sizeof(limits),
	)
	if ok == 0 {
		closeJob()
		return nil, fmt.Errorf("SetInformationJobObject: %w", callErr)
	}
	processHandle, err := syscall.OpenProcess(
		processTerminate|processSetQuota,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		closeJob()
		return nil, fmt.Errorf("OpenProcess: %w", err)
	}
	defer syscall.CloseHandle(processHandle)
	ok, _, callErr = assignProcessToJobObject.Call(job, uintptr(processHandle))
	if ok == 0 {
		closeJob()
		return nil, fmt.Errorf("AssignProcessToJobObject: %w", callErr)
	}
	return closeJob, nil
}
