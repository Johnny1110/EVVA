package proc

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// Windows-shaped fake paths use forward slashes so filepath.Join/Clean on
// the (possibly non-Windows) test host treats path elements the same way
// the real code does on Windows. The resolution ORDER and guards are what
// these tests pin down; separator rendering is the host's business.
func TestResolveShell(t *testing.T) {
	notFound := func(string) (string, error) { return "", errors.New("not found") }
	noEnv := func(string) string { return "" }
	never := func(string) bool { return false }

	gitBash := filepath.Join("C:/Program Files/Git", "bin", "bash.exe")

	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		look    map[string]string
		exist   []string
		want    string
		wantErr string
	}{
		{
			name:  "EVVA_SHELL override wins on any platform",
			goos:  "linux",
			env:   map[string]string{ShellEnv: "/opt/dash"},
			exist: []string{"/opt/dash"},
			want:  "/opt/dash",
		},
		{
			name:    "EVVA_SHELL pointing nowhere is a hard error",
			goos:    "windows",
			env:     map[string]string{ShellEnv: "C:/missing/bash.exe"},
			wantErr: ShellEnv,
		},
		{
			name: "unix default",
			goos: "darwin",
			want: "/bin/sh",
		},
		{
			name: "windows derives bash from cmd-dir git",
			goos: "windows",
			look: map[string]string{"git": "C:/Program Files/Git/cmd/git.exe"},
			exist: []string{
				gitBash,
			},
			want: gitBash,
		},
		{
			name: "windows derives bash from mingw64 git",
			goos: "windows",
			look: map[string]string{"git": "C:/Program Files/Git/mingw64/bin/git.exe"},
			exist: []string{
				gitBash,
			},
			want: gitBash,
		},
		{
			name: "windows falls back to well-known install dirs",
			goos: "windows",
			env:  map[string]string{"ProgramFiles": "C:/Program Files"},
			exist: []string{
				gitBash,
			},
			want: gitBash,
		},
		{
			name:    "windows rejects the System32 WSL launcher",
			goos:    "windows",
			look:    map[string]string{"bash": `C:\Windows\System32\bash.exe`},
			wantErr: "gitforwindows.org",
		},
		{
			name: "windows accepts a non-System32 PATH bash",
			goos: "windows",
			look: map[string]string{"bash": "C:/msys64/usr/bin/bash.exe"},
			want: "C:/msys64/usr/bin/bash.exe",
		},
		{
			name:    "windows with nothing installed errors with guidance",
			goos:    "windows",
			wantErr: "gitforwindows.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := noEnv
			if tt.env != nil {
				getenv = func(k string) string { return tt.env[k] }
			}
			look := notFound
			if tt.look != nil {
				look = func(name string) (string, error) {
					if p, ok := tt.look[name]; ok {
						return p, nil
					}
					return "", errors.New("not found")
				}
			}
			exists := never
			if tt.exist != nil {
				exists = func(p string) bool {
					for _, e := range tt.exist {
						if p == e {
							return true
						}
					}
					return false
				}
			}

			got, err := resolveShell(tt.goos, getenv, look, exists)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got path %q", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsSystem32(t *testing.T) {
	if !isSystem32(`C:\Windows\System32\bash.exe`) {
		t.Error("backslash System32 path not detected")
	}
	if !isSystem32("c:/windows/system32/bash.exe") {
		t.Error("lowercase slash System32 path not detected")
	}
	if isSystem32(`C:\Program Files\Git\bin\bash.exe`) {
		t.Error("Git Bash misdetected as System32")
	}
}
