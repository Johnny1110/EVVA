//go:build unix

package proc

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestGroupSetsPgid(t *testing.T) {
	cmd := exec.Command("sleep", "1")
	Group(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("Group did not set Setpgid")
	}
}

func TestKillTreeNoProcessIsNoop(t *testing.T) {
	if err := KillTree(exec.Command("sleep", "1")); err != nil {
		t.Fatalf("KillTree on un-started cmd: %v", err)
	}
}

// KillTree must take down descendants, not just the direct child — the
// exact scenario that motivated the process group: a shell whose
// grandchildren inherited the output pipes.
func TestKillTreeKillsDescendants(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & sleep 30 & wait")
	Group(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Give the shell a beat to fork its children before the kill.
	time.Sleep(100 * time.Millisecond)
	_ = KillTree(cmd)

	select {
	case <-done:
		// Wait returned promptly — the whole group is gone.
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after KillTree — descendants survived")
	}
}

func TestAlive(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Error("Alive(self) = false")
	}
	// A pid far outside any real allocation range: FindProcess always
	// succeeds on unix, so this exercises the Signal(0) ESRCH path.
	if Alive(1 << 30) {
		t.Error("Alive(2^30) = true")
	}
}

func TestTerminate(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := Terminate(cmd.Process.Pid); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("sleep exited cleanly; expected SIGTERM death")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process survived Terminate")
	}
}
