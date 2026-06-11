//go:build unix

package proc

import (
	"os"
	"os/exec"
	"syscall"
)

func group(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Negative pid → the whole process group (set up by group()).
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0: the kernel's existence/permission check without
	// delivering anything. nil (or EPERM, which we don't expect for our
	// own daemons) means alive; ESRCH means gone.
	return p.Signal(syscall.Signal(0)) == nil
}

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
