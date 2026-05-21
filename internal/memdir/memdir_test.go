package memdir

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoad_BothMissing(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	snap := Load(workdir, evvaHome, false)
	if snap.WorkdirMemory != "" {
		t.Errorf("WorkdirMemory: got %q, want empty", snap.WorkdirMemory)
	}
	if snap.UserProfile != "" {
		t.Errorf("UserProfile: got %q, want empty", snap.UserProfile)
	}
	if len(snap.Warnings) != 0 {
		t.Errorf("Warnings: got %v, want none", snap.Warnings)
	}
}

func TestLoad_OnlyProjectMemoryPresent(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(workdir, ProjectMemoryFile), "Conventions: use gofmt.")

	snap := Load(workdir, evvaHome, false)
	if snap.WorkdirMemory != "Conventions: use gofmt." {
		t.Errorf("WorkdirMemory: got %q", snap.WorkdirMemory)
	}
	if snap.UserProfile != "" {
		t.Errorf("UserProfile should be empty; got %q", snap.UserProfile)
	}
	if len(snap.Warnings) != 0 {
		t.Errorf("Warnings: got %v", snap.Warnings)
	}
}

func TestLoad_OnlyUserProfilePresent(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(evvaHome, UserProfileFile), "Preferences: terse output.")

	snap := Load(workdir, evvaHome, false)
	if snap.WorkdirMemory != "" {
		t.Errorf("WorkdirMemory should be empty; got %q", snap.WorkdirMemory)
	}
	if snap.UserProfile != "Preferences: terse output." {
		t.Errorf("UserProfile: got %q", snap.UserProfile)
	}
}

func TestLoad_BothPresent(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(workdir, ProjectMemoryFile), "proj-body")
	writeFile(t, filepath.Join(evvaHome, UserProfileFile), "user-body")

	snap := Load(workdir, evvaHome, false)
	if snap.WorkdirMemory != "proj-body" {
		t.Errorf("WorkdirMemory: got %q", snap.WorkdirMemory)
	}
	if snap.UserProfile != "user-body" {
		t.Errorf("UserProfile: got %q", snap.UserProfile)
	}
}

func TestLoad_FilePastSizeCap(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	// One byte over the cap so we definitely trip the truncation branch.
	oversize := strings.Repeat("x", MaxFileBytes+1024)
	writeFile(t, filepath.Join(workdir, ProjectMemoryFile), oversize)

	snap := Load(workdir, evvaHome, false)
	if len(snap.WorkdirMemory) != MaxFileBytes {
		t.Errorf("WorkdirMemory length: got %d, want %d", len(snap.WorkdirMemory), MaxFileBytes)
	}
	if len(snap.Warnings) != 1 {
		t.Fatalf("expected one warning; got %v", snap.Warnings)
	}
	if !strings.Contains(snap.Warnings[0], "truncated") {
		t.Errorf("warning should mention truncation; got %q", snap.Warnings[0])
	}
}

func TestLoad_UnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX chmod 000 semantics don't apply on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses POSIX read permission")
	}
	workdir, evvaHome := t.TempDir(), t.TempDir()
	p := filepath.Join(workdir, ProjectMemoryFile)
	writeFile(t, p, "secret")
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	snap := Load(workdir, evvaHome, false)
	if snap.WorkdirMemory != "" {
		t.Errorf("WorkdirMemory should be empty on permission error; got %q", snap.WorkdirMemory)
	}
	if len(snap.Warnings) != 1 {
		t.Fatalf("expected one warning; got %v", snap.Warnings)
	}
	if !strings.Contains(snap.Warnings[0], "cannot read") {
		t.Errorf("warning should mention read failure; got %q", snap.Warnings[0])
	}
}

func TestLoad_NilOrEmptyPaths(t *testing.T) {
	snap := Load("", "", false)
	if snap.WorkdirMemory != "" || snap.UserProfile != "" {
		t.Errorf("empty paths should produce empty snapshot; got %+v", snap)
	}
	if len(snap.Warnings) != 0 {
		t.Errorf("empty paths should produce no warnings; got %v", snap.Warnings)
	}
}

func TestLoad_EmptyFileIsEmptyBody(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(workdir, ProjectMemoryFile), "")

	snap := Load(workdir, evvaHome, false)
	if snap.WorkdirMemory != "" {
		t.Errorf("empty file should produce empty body; got %q", snap.WorkdirMemory)
	}
	if len(snap.Warnings) != 0 {
		t.Errorf("empty file should not warn; got %v", snap.Warnings)
	}
}

func TestLoad_ProjectMemoryAutoFile(t *testing.T) {
	workdir, evvaHome := t.TempDir(), t.TempDir()
	// Pre-seed the per-project MEMORY.md under <appHome>/projects/<key>/.
	wdAbs, err := filepath.Abs(workdir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	key := ProjectKey(wdAbs)
	dir := filepath.Join(evvaHome, ProjectsSubdir, key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, ProjectMemoryFileName), "## Project facts\nrepo uses goimports\n")

	snap := Load(workdir, evvaHome, true)
	if !strings.Contains(snap.ProjectMemory, "repo uses goimports") {
		t.Errorf("expected project memory body; got %q", snap.ProjectMemory)
	}
	if !strings.Contains(snap.ProjectMemoryIndex, "## Project facts") {
		t.Errorf("expected index summary; got %q", snap.ProjectMemoryIndex)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
