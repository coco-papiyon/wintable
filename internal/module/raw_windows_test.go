package module

import "syscall"

// CmdLine avoids Go re-quoting the cmd /c command, matching OpenSSH's raw line.
func rawCommand(command string) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CmdLine: WrapperLaunchLine(command), HideWindow: true}
}
