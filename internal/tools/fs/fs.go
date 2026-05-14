// Package fs exposes filesystem tools (Read, Write, Edit) as stateless
// singletons. Construction policy (eager vs lazy) is decided by the agent;
// this package only knows how to produce tool instances.
package fs

import (
	"fmt"
	"os"
	"path/filepath"

	config "github.com/johnny1110/evva/configs"
	"github.com/johnny1110/evva/internal/tools"
)

// Names lists every tool name this package contributes, in canonical order.
func Names() []tools.ToolName {
	return []tools.ToolName{tools.READ_FILE, tools.WRITE_FILE, tools.EDIT_FILE}
}

// resolvePath validates that pathStr is absolute and returns its cleaned
// form. Matches Claude Code's contract — the LLM must supply absolute
// paths; relative paths are rejected up front with a hint pointing at the
// workdir, so a misconfigured agent never silently writes to /cwd by
// mistake.
func resolvePath(pathStr string) (string, error) {
	if pathStr == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if !filepath.IsAbs(pathStr) {
		cfg := config.Get()
		return "", fmt.Errorf("file_path must be absolute (relative paths are not supported; workdir is %s)", cfg.WorkDir)
	}
	return filepath.Clean(pathStr), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
