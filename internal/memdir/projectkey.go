package memdir

import (
	"path/filepath"
	"strings"
)

// ProjectKey turns an absolute filesystem path into a stable directory key
// used under <APP_HOME>/projects/. Mirrors the convention Claude Code uses
// at ~/.claude/projects/ — the path's separators are flattened to "-" so
// the result is one segment that round-trips losslessly enough for human
// inspection.
//
// Examples (POSIX):
//
//	/Users/johnny/lab/evva     -> "-Users-johnny-lab-evva"
//	/home/alice/work/api       -> "-home-alice-work-api"
//
// On Windows the volume colon is dropped and backslashes are flattened the
// same way:
//
//	C:\Users\Alice\proj        -> "C-Users-Alice-proj"
//
// An empty input yields "". Inputs are cleaned via filepath.Clean first so
// "/a//b/../c" and "/a/c" produce the same key.
func ProjectKey(absPath string) string {
	if absPath == "" {
		return ""
	}
	clean := filepath.Clean(absPath)
	// Drop a Windows-style drive colon ("C:\foo" -> "C\foo"). Done by hand
	// because filepath.VolumeName only recognizes drive prefixes when the
	// binary is running on Windows; we want the slug to be stable across
	// hosts (tests on macOS verify Windows-shaped inputs).
	if len(clean) >= 2 && clean[1] == ':' &&
		((clean[0] >= 'A' && clean[0] <= 'Z') || (clean[0] >= 'a' && clean[0] <= 'z')) {
		clean = clean[:1] + clean[2:]
	}
	// Flatten both separators to "-" so the key is one filesystem segment.
	clean = strings.ReplaceAll(clean, "\\", "/")
	clean = strings.ReplaceAll(clean, "/", "-")
	// Collapse runs of "-" introduced by leading "/" / "//" / trailing "/".
	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}
	// Trim a trailing "-" but keep a leading one — the leading dash is a
	// visual marker that the path was absolute (matches ~/.claude/projects/).
	clean = strings.TrimRight(clean, "-")
	return clean
}
