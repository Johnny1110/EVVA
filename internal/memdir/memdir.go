// Package memdir loads the on-disk memory files that seed the agent's
// system prompt at session start, and provides the write helpers the
// auto-memory tools call mid-session:
//
//   - <workdir>/EVVA.md                                       workdir memory — repo conventions (user-authored)
//   - <appHome>/USER_PROFILE.md                               user memory  — preferences, working style (auto)
//   - <appHome>/projects/<projectKey(workdir)>/MEMORY.md      project memory (auto)
//
// All files are optional. Missing files yield zero-value Snapshot fields
// and no warning; the prompt builder skips empty sections cleanly. Any
// non-missing read failure (permission, oversize) is recorded in
// Snapshot.Warnings — Load itself never returns an error so the agent can
// always boot.
//
// This package depends only on stdlib. It is not imported by the sysprompt
// package; the caller threads Snapshot fields into the prompt context,
// keeping the dependency arrow one-way.
package memdir

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// File names. Exposed so other packages (Phase 9 user-profile background
// agent, future /memory slash commands) can write to the same paths without
// re-spelling them.
const (
	ProjectMemoryFile = "EVVA.md"
	UserProfileFile   = "USER_PROFILE.md"
)

// MaxFileBytes caps each memory file at 64 KiB. Past that the user is
// almost certainly using EVVA.md for the wrong thing (knowledge base, not
// conventions doc); we truncate and warn rather than refuse outright so a
// bloated file doesn't break the session.
const MaxFileBytes = 64 * 1024

// Snapshot is one session's view of the on-disk memory files. Any body
// field may be empty when the file is missing, empty, or unreadable;
// callers treat empty as "skip the section."
type Snapshot struct {
	WorkdirMemory      string   // raw contents of <workdir>/EVVA.md (user-authored, repo-scoped)
	UserProfile        string   // raw contents of <appHome>/USER_PROFILE.md (auto, user-scoped)
	ProjectMemory      string   // raw contents of <appHome>/projects/<key>/MEMORY.md (auto, project-scoped)
	ProjectMemoryIndex string   // compact one-line-per-section summary of ProjectMemory; empty when no sections
	Warnings           []string // non-fatal: oversize-truncation, permission errors
}

// Load reads the memory files. Empty workdir or appHome silently skips
// the files anchored at that path. Files larger than MaxFileBytes are
// truncated with a warning. The function never returns an error.
//
// loadProjectMemory gates the per-project MEMORY.md read — callers set it
// to cfg.EnableAutoMemory so disabled users avoid the extra stat.
func Load(workdir, appHome string, loadProjectMemory bool) Snapshot {
	var snap Snapshot
	if workdir != "" {
		body, warn := readMemFile(filepath.Join(workdir, ProjectMemoryFile))
		snap.WorkdirMemory = body
		if warn != "" {
			snap.Warnings = append(snap.Warnings, warn)
		}
	}
	if appHome != "" {
		body, warn := readMemFile(filepath.Join(appHome, UserProfileFile))
		snap.UserProfile = body
		if warn != "" {
			snap.Warnings = append(snap.Warnings, warn)
		}
	}
	if loadProjectMemory && appHome != "" && workdir != "" {
		body, warn := ReadProjectMemory(appHome, workdir)
		snap.ProjectMemory = body
		if warn != "" {
			snap.Warnings = append(snap.Warnings, warn)
		}
		snap.ProjectMemoryIndex = IndexSummary(body, 80)
	}
	return snap
}

// readMemFile reads at most MaxFileBytes from path. Returns (body, warning).
// Missing files return ("", "") so the caller can skip cleanly. Read errors
// other than os.IsNotExist return ("", "<reason>"); oversize files return
// (truncated, "<reason>"). LimitReader bounds the read so a runaway 1 GB
// EVVA.md doesn't pull the world into memory before we truncate.
func readMemFile(path string) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ""
		}
		return "", fmt.Sprintf("memdir: cannot read %s: %v", path, err)
	}
	defer f.Close()

	buf, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return "", fmt.Sprintf("memdir: read %s: %v", path, err)
	}
	if len(buf) > MaxFileBytes {
		return string(buf[:MaxFileBytes]), fmt.Sprintf("memdir: %s truncated to %d bytes (cap %d)", path, MaxFileBytes, MaxFileBytes)
	}
	return string(buf), ""
}
