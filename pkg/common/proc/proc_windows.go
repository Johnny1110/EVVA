//go:build windows

package proc

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// DETACHED_PROCESS is not exported by the windows syscall package.
const detachedProcess = 0x00000008

func group(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// taskkill /T walks the child tree; /F is TerminateProcess. Stateless
	// (no job-object handle to thread through call sites) and preinstalled
	// on every Windows. If bring-up shows orphaned trees, upgrade this to
	// a job object without touching any call site — that is this seam's
	// whole point (PRD §5.1, open question Q2).
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess
}

func alive(pid int) bool {
	// Unlike unix, FindProcess on Windows opens a real process handle and
	// fails when the pid is gone — Signal(0) is unsupported here, so the
	// handle probe IS the liveness check.
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
