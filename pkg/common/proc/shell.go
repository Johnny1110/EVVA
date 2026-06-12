package proc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ShellEnv is the operator escape hatch: point it at any POSIX shell
// (must accept `-c <command>`) and every shell-backed surface — the bash
// tool, monitor daemons, lifecycle hooks — uses it verbatim.
const ShellEnv = "EVVA_SHELL"

var shellOnce = sync.OnceValues(func() (string, error) {
	return resolveShell(runtime.GOOS, os.Getenv, exec.LookPath, fileExists)
})

// Shell returns the POSIX shell used for `<shell> -c <command>` children:
// EVVA_SHELL if set, /bin/sh elsewhere, Git Bash on Windows. Resolved once
// per process. The error text is model/user-facing and says how to fix
// the environment.
func Shell() (string, error) { return shellOnce() }

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// resolveShell is the pure resolution logic — goos and the environment
// probes are parameters (the unitFor pattern) so every platform's
// behavior is testable from any platform.
func resolveShell(
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
	exists func(string) bool,
) (string, error) {
	if s := getenv(ShellEnv); s != "" {
		if !exists(s) {
			return "", fmt.Errorf("%s points at %q, which does not exist", ShellEnv, s)
		}
		return s, nil
	}
	if goos != "windows" {
		return "/bin/sh", nil
	}

	// Windows: find Git Bash. Deriving from git's own location comes
	// first — git is effectively an evva prerequisite, and Git for
	// Windows does not put bash.exe on PATH by default. Use
	// <Git>\bin\bash.exe (the shim that sets up PATH for the bundled
	// unix tools), never usr\bin\bash.exe directly.
	if git, err := lookPath("git"); err == nil {
		dir := filepath.Dir(git)
		for _, rel := range [][]string{
			{"..", "bin", "bash.exe"},       // <Git>\cmd\git.exe
			{"..", "..", "bin", "bash.exe"}, // <Git>\mingw64\bin\git.exe
		} {
			if p := filepath.Join(append([]string{dir}, rel...)...); exists(p) {
				return p, nil
			}
		}
	}
	for _, base := range []string{
		getenv("ProgramFiles"),
		getenv("ProgramFiles(x86)"),
		filepath.Join(getenv("LOCALAPPDATA"), "Programs"),
	} {
		if base == "" || base == filepath.Join("", "Programs") {
			continue
		}
		if p := filepath.Join(base, "Git", "bin", "bash.exe"); exists(p) {
			return p, nil
		}
	}
	// PATH last, and never System32: that bash.exe is the WSL launcher,
	// which runs commands inside a WSL distro, not on this filesystem.
	if p, err := lookPath("bash"); err == nil && !isSystem32(p) {
		return p, nil
	}
	return "", fmt.Errorf(
		"no bash found — evva on Windows needs Git for Windows (https://gitforwindows.org); set %s to a bash.exe to override",
		ShellEnv,
	)
}

func isSystem32(path string) bool {
	lower := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	return strings.Contains(lower, "/system32/")
}
