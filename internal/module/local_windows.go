package module

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/coco-papiyon/wintable/internal/result"
	"golang.org/x/sys/windows"
)

// RunLocalCommand starts the validated executable without a shell or elevation.
// The suspended process joins a kill-on-close job before any user code runs.
func RunLocalCommand(ctx context.Context, p Prepared, output bool) result.TaskResult {
	return runLocalProcess(ctx, p, output, nil)
}
func runLocalProcess(ctx context.Context, p Prepared, output bool, stdin []byte) result.TaskResult {
	fail := func(code string) result.TaskResult {
		row := result.TaskResult{Status: result.Failed, Error: result.PublicError("task_failed")}
		if code == "deadline_exceeded" || code == "internal_error" {
			row.Error = result.PublicError(code)
		}
		if code == "start_failed" {
			b := false
			row.Changed = &b
		}
		return row
	}
	var input struct {
		Args struct {
			Executable string
			Argv       []string
		}
	}
	if json.Unmarshal(p.JSON(), &input) != nil {
		return fail("internal_error")
	}
	if ctx.Err() != nil {
		return fail("deadline_exceeded")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fail("start_failed")
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fail("start_failed")
	}
	in, iw, err := os.Pipe()
	if err != nil {
		return fail("start_failed")
	}
	defer in.Close()
	defer iw.Close()
	out, ow, err := os.Pipe()
	if err != nil {
		return fail("start_failed")
	}
	defer out.Close()
	defer ow.Close()
	stderr, ew, err := os.Pipe()
	if err != nil {
		return fail("start_failed")
	}
	defer stderr.Close()
	defer ew.Close()
	handles := []windows.Handle{windows.Handle(in.Fd()), windows.Handle(ow.Fd()), windows.Handle(ew.Fd())}
	for _, h := range handles {
		if windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil {
			return fail("start_failed")
		}
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail("start_failed")
	}
	defer attrs.Delete()
	if attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])) != nil {
		return fail("start_failed")
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = handles[0], handles[1], handles[2]
	si.ProcThreadAttributeList = attrs.List()
	exe, _ := windows.UTF16PtrFromString(input.Args.Executable)
	line, _ := windows.UTF16PtrFromString(CommandLine(input.Args.Executable, input.Args.Argv))
	var proc windows.ProcessInformation
	if windows.CreateProcess(exe, line, nil, nil, true, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &si.StartupInfo, &proc) != nil {
		return fail("start_failed")
	}
	defer windows.CloseHandle(proc.Process)
	defer windows.CloseHandle(proc.Thread)
	reap := func() { windows.WaitForSingleObject(proc.Process, windows.INFINITE) }
	if windows.AssignProcessToJobObject(job, proc.Process) != nil {
		windows.TerminateProcess(proc.Process, 1)
		reap()
		return fail("start_failed")
	}
	if ctx.Err() != nil {
		windows.TerminateJobObject(job, 1)
		reap()
		return fail("deadline_exceeded")
	}
	if _, err = windows.ResumeThread(proc.Thread); err != nil {
		windows.TerminateJobObject(job, 1)
		reap()
		return fail("start_failed")
	}
	in.Close()
	go func() { defer iw.Close(); _, _ = iw.Write(stdin) }()
	ow.Close()
	ew.Close()
	a, b := result.NewCapture(output), result.NewCapture(output)
	var readers sync.WaitGroup
	readers.Add(2)
	var oe, ee error
	go func() { defer readers.Done(); _, oe = io.Copy(a, out) }()
	go func() { defer readers.Done(); _, ee = io.Copy(b, stderr) }()
	done := make(chan struct{})
	go func() { reap(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	// Also terminate descendants which outlive a normally exiting parent.
	cleanupErr := windows.TerminateJobObject(job, 1)
	<-done
	// TerminateJobObject initiates termination; retain the forks slot until all
	// descendants have left the job, including ones with no inherited pipes.
	if cleanupErr == nil {
		for {
			var accounting struct {
				User, Kernel, PeriodUser, PeriodKernel int64
				Faults, Total, Active, Terminated      uint32
			}
			cleanupErr = windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
			if cleanupErr != nil || accounting.Active == 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	// TerminateJobObject initiates process termination asynchronously. The
	// accounting count can reach zero before each process handle is signaled,
	// so wait for the job object's completion signal before returning.
	if cleanupErr == nil {
		state, err := windows.WaitForSingleObject(job, 10000)
		if err != nil {
			cleanupErr = err
		} else if state != windows.WAIT_OBJECT_0 {
			cleanupErr = errors.New("job termination timed out")
		}
	}
	readers.Wait()
	if ctx.Err() != nil {
		return fail("deadline_exceeded")
	}
	if cleanupErr != nil || oe != nil || ee != nil {
		return fail("execution_failed")
	}
	var code uint32
	if windows.GetExitCodeProcess(proc.Process, &code) != nil {
		return fail("execution_failed")
	}
	n := int(int32(code))
	changed := true
	row := result.TaskResult{Status: result.Changed, Changed: &changed, ExitCode: &n}
	if n != 0 {
		row.Status = result.Failed
		row.Changed = nil
		row.Error = result.PublicError("task_failed")
	}
	row.StdoutBase64, row.StdoutTruncated = a.Output()
	row.StderrBase64, row.StderrTruncated = b.Output()
	return row
}
