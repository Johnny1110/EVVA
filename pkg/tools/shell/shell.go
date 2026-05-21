// Package shell hosts shell-side tools: Bash, Ls, Grep, Tree.
//
// Bash is synchronous shell execution; Ls/Tree are filesystem listing
// helpers (cheaper than a Bash round-trip when you just want to look at a
// directory); Grep is a regex search across files. Long-running process
// monitoring is intentionally elsewhere (the monitor package).
package shell

import "github.com/johnny1110/evva/pkg/tools"

// Names lists every tool name this package contributes, in canonical order.
func Names() []tools.ToolName {
	return []tools.ToolName{tools.BASH, tools.GREP, tools.TREE}
}

// skipDirs returns the set of directory names to skip during tree/grep walks.
func skipDirs() map[string]struct{} {
	return map[string]struct{}{
		".git":         {},
		"node_modules": {},
		"vendor":       {},
		".idea":        {},
		".vscode":      {},
		"dist":         {},
		"build":        {},
		"target":       {},
	}
}
