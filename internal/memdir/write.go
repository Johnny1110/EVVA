package memdir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ProjectMemoryFileName is the on-disk basename of per-project memory.
// Lives under <APP_HOME>/projects/<key>/MEMORY.md.
const ProjectMemoryFileName = "MEMORY.md"

// ProjectsSubdir is the directory under APP_HOME that holds per-project
// memory keyed by ProjectKey(workdir).
const ProjectsSubdir = "projects"

// UserProfilePath returns the absolute path of USER_PROFILE.md given the
// caller's APP_HOME. Empty appHome yields "".
func UserProfilePath(appHome string) string {
	if appHome == "" {
		return ""
	}
	return filepath.Join(appHome, UserProfileFile)
}

// ProjectMemoryPath returns the absolute path of MEMORY.md for the given
// workdir, scoped to APP_HOME. Empty inputs yield "".
func ProjectMemoryPath(appHome, workdir string) string {
	if appHome == "" || workdir == "" {
		return ""
	}
	abs := workdir
	if a, err := filepath.Abs(workdir); err == nil {
		abs = a
	}
	key := ProjectKey(abs)
	if key == "" {
		return ""
	}
	return filepath.Join(appHome, ProjectsSubdir, key, ProjectMemoryFileName)
}

// ReadProjectMemory reads the per-project MEMORY.md for the given workdir.
// Missing file returns ("", nil) — callers treat empty content as "no
// memory yet." Truncation and oversize behave like the read path in
// memdir.Load: cap at MaxFileBytes with a warning returned.
func ReadProjectMemory(appHome, workdir string) (string, string) {
	path := ProjectMemoryPath(appHome, workdir)
	if path == "" {
		return "", ""
	}
	return readMemFile(path)
}

// WriteUserProfile writes `content` to USER_PROFILE.md atomically (temp +
// rename in the same directory). Creates the parent directory if missing.
// Returns an error only on real I/O failure.
func WriteUserProfile(appHome, content string) error {
	path := UserProfilePath(appHome)
	if path == "" {
		return errors.New("memdir: empty appHome — refusing to write USER_PROFILE.md")
	}
	return writeAtomic(path, content)
}

// WriteProjectMemory writes `content` to <APP_HOME>/projects/<key>/MEMORY.md
// atomically. Creates the parent directory chain if missing.
func WriteProjectMemory(appHome, workdir, content string) error {
	path := ProjectMemoryPath(appHome, workdir)
	if path == "" {
		return fmt.Errorf("memdir: cannot resolve project memory path (appHome=%q, workdir=%q)", appHome, workdir)
	}
	return writeAtomic(path, content)
}

// writeAtomic writes `content` to `path` by creating a sibling temp file
// and renaming it into place. Atomic on POSIX; on Windows the rename
// across the same directory is still atomic per the os package contract.
func writeAtomic(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("memdir: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".memdir-*.tmp")
	if err != nil {
		return fmt.Errorf("memdir: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("memdir: write %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("memdir: close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("memdir: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// EnsureProjectMemoryDir creates the parent directory for the per-project
// MEMORY.md, mirroring the ref pattern of pre-creating memory dirs at boot
// so the model never has to mkdir before its first write.
func EnsureProjectMemoryDir(appHome, workdir string) error {
	path := ProjectMemoryPath(appHome, workdir)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}
