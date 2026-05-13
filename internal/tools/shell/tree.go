package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
)

// Tree is the singleton TreeTool. Stateless.
var Tree tools.Tool = &TreeTool{}

type TreeTool struct{}

func NewTree() *TreeTool { return &TreeTool{} }

func (t *TreeTool) Name() string { return string(tools.TREE) }

func (t *TreeTool) Description() string {
	return "Print a directory tree to a given depth. Default max depth is 3. " +
		"Skips common vendored/build directories (.git, node_modules, vendor, dist, build, target). " +
		"Hidden entries (leading dot) are omitted unless show_hidden=true."
}

func (t *TreeTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["path"],
		"properties":{
			"path":{"type":"string","description":"Absolute path to the root directory."},
			"max_depth":{"type":"integer","minimum":1,"default":3,"description":"Recursion depth limit. Root is depth 0."},
			"show_hidden":{"type":"boolean","default":false,"description":"Include dot-prefixed entries."}
		}
	}`)
}

type treeInput struct {
	Path       string `json:"path"`
	MaxDepth   *int   `json:"max_depth"`
	ShowHidden bool   `json:"show_hidden"`
}

func (t *TreeTool) Execute(_ context.Context, input json.RawMessage) (tools.Result, error) {
	var in treeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("tree: decode: %v", err)}, nil
	}
	if in.Path == "" {
		return tools.Result{IsError: true, Content: "tree: path is required"}, nil
	}
	if !filepath.IsAbs(in.Path) {
		return tools.Result{IsError: true, Content: "tree: path must be absolute"}, nil
	}
	depth := 3
	if in.MaxDepth != nil && *in.MaxDepth > 0 {
		depth = *in.MaxDepth
	}

	var b strings.Builder
	b.WriteString(filepath.Base(in.Path))
	b.WriteByte('\n')
	if err := walkTree(&b, in.Path, "", 0, depth, in.ShowHidden); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("tree: %v", err)}, nil
	}
	return tools.Result{Content: strings.TrimRight(b.String(), "\n")}, nil
}

// walkTree appends an ASCII tree of dir into b. prefix tracks the current
// indentation (├──, └──, │  , spaces). Returns the first IO error if any.
func walkTree(b *strings.Builder, dir, prefix string, level, maxDepth int, showHidden bool) error {
	if level >= maxDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	skip := skipDirs()
	filtered := entries[:0]
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			if _, drop := skip[name]; drop {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].IsDir() != filtered[j].IsDir() {
			return filtered[i].IsDir()
		}
		return filtered[i].Name() < filtered[j].Name()
	})

	for i, e := range filtered {
		isLast := i == len(filtered)-1
		connector := "├── "
		nextPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			nextPrefix = prefix + "    "
		}
		fmt.Fprintf(b, "%s%s%s", prefix, connector, e.Name())
		if e.IsDir() {
			b.WriteByte('/')
		}
		b.WriteByte('\n')

		if e.IsDir() {
			if err := walkTree(b, filepath.Join(dir, e.Name()), nextPrefix, level+1, maxDepth, showHidden); err != nil {
				return err
			}
		}
	}
	return nil
}
