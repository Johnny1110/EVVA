package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
)

// Grep is the singleton GrepTool. Delegates to system grep via Bash.
var Grep tools.Tool = &GrepTool{}

type GrepTool struct{}

func NewGrep() *GrepTool { return &GrepTool{} }

func (t *GrepTool) Name() string { return string(tools.GREP) }

func (t *GrepTool) Description() string {
	return "Search for a regular-expression pattern across files. Defaults to content mode (path:line:text). " +
		"Output modes: \"content\" (default) lists matching lines, \"files_with_matches\" returns one path per match, " +
		"\"count\" returns one count per file. " +
		"Optional glob filter narrows by filename (e.g. \"*.go\"); head_limit caps total output rows. " +
		"context_around / context_before / context_after show N lines of surrounding context for each match (like grep -C/-B/-A). " +
		"Skips common vendored/build directories (.git, node_modules) automatically. " +
		"Delegates to the system grep command via bash."
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
			"head_limit":{"type":"integer","minimum":1,"description":"Cap the number of output rows."},
			"context_around":{"type":"integer","minimum":0,"description":"Show N lines of context before and after each match (like grep -C N)."},
			"context_before":{"type":"integer","minimum":0,"description":"Show N lines of context before each match (like grep -B N)."},
			"context_after":{"type":"integer","minimum":0,"description":"Show N lines of context after each match (like grep -A N)."}
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
	ContextAround   int    `json:"context_around"`
	ContextBefore   int    `json:"context_before"`
	ContextAfter    int    `json:"context_after"`
}

func (t *GrepTool) Execute(ctx context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("grep: decode: %v", err)}, nil
	}
	if in.Pattern == "" {
		return tools.Result{IsError: true, Content: "grep: pattern is required"}, nil
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
	logger.Debug("grep.dispatch", "pattern", in.Pattern, "path", root, "mode", mode, "glob", in.Glob)

	cmd, err := buildGrepCmd(in.Pattern, root, mode, in.CaseInsensitive, in.Glob,
		in.ContextBefore, in.ContextAfter, in.ContextAround, in.HeadLimit)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("grep: %v", err)}, nil
	}

	res, _ := Bash.Execute(ctx, logger, json.RawMessage(
		`{"command":`+strconv.Quote(cmd)+`,"description":"grep for pattern"}`,
	))

	// grep exit code 1 = no matches — treat as success with placeholder.
	if res.IsError && strings.Contains(res.Content, "exit code 1") {
		return tools.Result{Content: "(no matches)"}, nil
	}

	// Strip "Binary file X matches" lines from output.
	cleaned := stripBinaryLines(res.Content)

	return tools.Result{Content: cleaned, IsError: res.IsError}, nil
}

// stripBinaryLines removes "Binary file X matches" lines that grep emits
// when it encounters non-text files (not all systems support -I).
func stripBinaryLines(s string) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "Binary file ") && strings.HasSuffix(line, " matches") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// buildGrepCmd constructs a system grep command from the tool parameters.
func buildGrepCmd(pattern, root, mode string, caseInsensitive bool, glob string,
	ctxBefore, ctxAfter, ctxAround, headLimit int) (string, error) {

	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat %s: %v", root, err)
	}
	isDir := info.IsDir()

	var flags []string
	flags = append(flags, "grep")

	if caseInsensitive {
		flags = append(flags, "-i")
	}

	switch mode {
	case "files_with_matches":
		flags = append(flags, "-rl")
	case "count":
		flags = append(flags, "-rc")
	default:
		flags = append(flags, "-Hn")
		if isDir {
			flags = append(flags, "-r")
		}
	}

	// Context flags.
	ctxB := ctxBefore
	ctxA := ctxAfter
	if ctxAround > 0 {
		ctxB = ctxAround
		ctxA = ctxAround
	}
	if ctxB > 0 {
		flags = append(flags, fmt.Sprintf("-B%d", ctxB))
	}
	if ctxA > 0 {
		flags = append(flags, fmt.Sprintf("-A%d", ctxA))
	}

	// Exclude vendored directories.
	skip := skipDirs()
	for dir := range skip {
		flags = append(flags, fmt.Sprintf("--exclude-dir=%s", dir))
	}

	// Glob filter (--include only makes sense with -r on directories).
	if glob != "" && isDir {
		flags = append(flags, fmt.Sprintf("--include=%s", shellQuote(glob)))
	}

	flags = append(flags, "--")
	flags = append(flags, shellQuote(pattern))
	flags = append(flags, root)

	cmd := strings.Join(flags, " ")

	// Pipe through head when head_limit is set.
	if headLimit > 0 {
		cmd = fmt.Sprintf("%s | head -n %d", cmd, headLimit)
	}

	return cmd, nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
