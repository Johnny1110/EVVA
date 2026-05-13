package shell

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
)

// Grep is the singleton GrepTool. Stateless.
var Grep tools.Tool = &GrepTool{}

type GrepTool struct{}

func NewGrep() *GrepTool { return &GrepTool{} }

func (t *GrepTool) Name() string { return string(tools.GREP) }

func (t *GrepTool) Description() string {
	return "Search for a regular-expression pattern across files. Defaults to content mode (path:line:text). " +
		"Output modes: \"content\" (default) lists matching lines, \"files_with_matches\" returns one path per match, " +
		"\"count\" returns one count per file. " +
		"Optional glob filter narrows by filename (e.g. \"*.go\"); head_limit caps total output rows. " +
		"Skips binary-looking files and common vendored/build directories (.git, node_modules) automatically."
}

func (t *GrepTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["pattern"],
		"properties":{
			"pattern":{"type":"string","description":"Regular expression to match (RE2 syntax)."},
			"path":{"type":"string","description":"Absolute path to a file or directory to search. Defaults to the current working directory."},
			"output_mode":{"type":"string","enum":["content","files_with_matches","count"],"default":"content","description":"What to return."},
			"case_insensitive":{"type":"boolean","default":false,"description":"Make the match case-insensitive."},
			"glob":{"type":"string","description":"Glob filter on filename (e.g. \"*.go\")."},
			"head_limit":{"type":"integer","minimum":1,"description":"Cap the number of output rows."}
		}
	}`)
}

type grepInput struct {
	Pattern         string `json:"pattern"`
	Path            string `json:"path"`
	OutputMode      string `json:"output_mode"`
	CaseInsensitive bool   `json:"case_insensitive"`
	Glob            string `json:"glob"`
	HeadLimit       int    `json:"head_limit"`
}

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("grep: decode: %v", err)}, nil
	}
	if in.Pattern == "" {
		return tools.Result{IsError: true, Content: "grep: pattern is required"}, nil
	}

	pat := in.Pattern
	if in.CaseInsensitive {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("grep: bad pattern: %v", err)}, nil
	}

	root := in.Path
	if root == "" {
		root, _ = os.Getwd()
	}
	if !filepath.IsAbs(root) {
		return tools.Result{IsError: true, Content: "grep: path must be absolute"}, nil
	}

	mode := in.OutputMode
	if mode == "" {
		mode = "content"
	}

	files, err := collectGrepFiles(root, in.Glob)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("grep: %v", err)}, nil
	}

	var (
		lines   []string
		matched = map[string]int{}
		total   int
	)
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return tools.Result{IsError: true, Content: "grep: cancelled"}, err
		}
		hits, perFile, err := grepFile(f, re, mode)
		if err != nil {
			// Per-file errors (permission denied on one path) shouldn't stop
			// the whole search — keep going.
			continue
		}
		if perFile == 0 {
			continue
		}
		matched[f] = perFile
		total += perFile
		lines = append(lines, hits...)
		if in.HeadLimit > 0 && len(lines) >= in.HeadLimit {
			lines = lines[:in.HeadLimit]
			break
		}
	}

	var body string
	switch mode {
	case "files_with_matches":
		paths := make([]string, 0, len(matched))
		for p := range matched {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		body = strings.Join(paths, "\n")
	case "count":
		paths := make([]string, 0, len(matched))
		for p := range matched {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		var b strings.Builder
		for _, p := range paths {
			fmt.Fprintf(&b, "%s:%d\n", p, matched[p])
		}
		body = strings.TrimRight(b.String(), "\n")
	default:
		body = strings.Join(lines, "\n")
	}

	if body == "" {
		body = "(no matches)"
	}
	return tools.Result{Content: body}, nil
}

// grepFile scans one file. Returns the matching lines (when mode is content)
// and the per-file match count. Returns (nil, 0, nil) for files that look
// binary so we don't spray non-text into the result.
func grepFile(path string, re *regexp.Regexp, mode string) ([]string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	// Quick binary sniff: peek up to 512 bytes for a NUL byte.
	if peek, _ := br.Peek(512); containsNUL(peek) {
		return nil, 0, nil
	}

	var (
		lines []string
		count int
		num   int
	)
	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		num++
		line := scanner.Text()
		if !re.MatchString(line) {
			continue
		}
		count++
		if mode == "content" {
			lines = append(lines, fmt.Sprintf("%s:%d:%s", path, num, line))
		}
	}
	return lines, count, nil
}

// collectGrepFiles returns the files to search starting from root. If root
// is a file, returns just that file; otherwise walks and applies glob +
// vendored-dir skip rules.
func collectGrepFiles(root, glob string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if glob != "" {
			if ok, _ := filepath.Match(glob, filepath.Base(root)); !ok {
				return nil, nil
			}
		}
		return []string{root}, nil
	}

	var out []string
	skip := skipDirs()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip on error, keep walking
		}
		name := d.Name()
		if d.IsDir() {
			if _, ok := skip[name]; ok && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			if ok, _ := filepath.Match(glob, name); !ok {
				return nil
			}
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

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

func containsNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}
