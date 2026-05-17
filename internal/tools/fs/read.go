package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
	"github.com/johnny1110/evva/internal/tools/shell"
)

// DefaultReadLimit caps an unbounded Read at this many lines, matching
// Claude Code. The model can pass an explicit larger limit when it really
// needs more, but the default protects the context window from accidental
// 50k-line dumps.
const DefaultReadLimit = 2000

// ReadTool reads a file and returns it in cat -n format.
type ReadTool struct {
	tracker *ReadTracker
}

// NewRead creates a ReadTool that records reads in the given tracker.
func NewRead(tracker *ReadTracker) *ReadTool {
	return &ReadTool{tracker: tracker}
}

func (t *ReadTool) Name() string { return string(tools.READ_FILE) }

func (t *ReadTool) Description() string {
	return "Reads a file from the local filesystem. file_path must be absolute.\n\n" +
		"Output is cat -n format: each line is prefixed with its 1-based " +
		"line number and a tab (e.g. `   42\\thello`). A header " +
		"`[File: <path> (N lines)]` precedes the body and notes the slice " +
		"when offset/limit are used.\n\n" +
		"By default at most 2000 lines are returned starting from line 1. " +
		"Use `offset` (1-based) to start later in the file and `limit` to " +
		"control the slice size — pass an explicit larger `limit` when you " +
		"need more than 2000 lines.\n\n" +
		"Reading marks the file as loaded into the session — edit_file and " +
		"write_file (overwrite) refuse to touch a file you haven't read " +
		"first. When you later call edit_file, DO NOT include the " +
		"`<line>\\t` prefix in old_string — strip it. Only the raw line " +
		"content is what's actually in the file.\n\n" +
		"Multimodal reads (images, PDFs, Jupyter notebooks) are not yet " +
		"supported — see the README wish list. The `pages` parameter is " +
		"reserved for future PDF support and is rejected today."
}

func (t *ReadTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["file_path"],
		"properties":{
			"file_path":{"type":"string","description":"Absolute path to the file to read."},
			"offset":{"type":"integer","minimum":1,"description":"1-based line number to start reading from. Defaults to 1."},
			"limit":{"type":"integer","exclusiveMinimum":0,"description":"Maximum number of lines to return. Defaults to 2000. Pass a larger value to read more."},
			"pages":{"type":"string","description":"Reserved for PDF page range (not yet supported)."}
		}
	}`)
}

type readInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
	Pages    string `json:"pages"`
}

func (t *ReadTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: "read: decode input: " + err.Error()}, nil
	}

	if in.Pages != "" {
		return tools.Result{
			IsError: true,
			Content: "read: PDF/pages support is not yet implemented (tracked in README wish list). Use this tool only on text files.",
		}, nil
	}

	resolved, err := resolvePath(in.FilePath)
	if err != nil {
		return tools.Result{IsError: true, Content: "read: " + err.Error()}, nil
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: file not found: %s", in.FilePath)}, nil
	}
	if info.IsDir() {
		treeInput := fmt.Sprintf(`{"path":"%s"}`, resolved)
		return shell.Tree.Execute(ctx, json.RawMessage(treeInput))
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: could not read %s: %s", in.FilePath, err)}, nil
	}

	if t.tracker != nil {
		t.tracker.MarkRead(resolved)
	}

	content := string(data)
	allLines := strings.Split(content, "\n")
	// Strip the trailing empty line from Split when the file ends with \n.
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" && strings.HasSuffix(content, "\n") {
		allLines = allLines[:len(allLines)-1]
	}
	totalLines := len(allLines)

	if totalLines == 0 {
		return tools.Result{Content: fmt.Sprintf("[File: %s, 0 lines]", resolved)}, nil
	}

	start := in.Offset
	if start < 1 {
		start = 1
	}
	startIdx := start - 1
	if startIdx >= totalLines {
		return tools.Result{Content: fmt.Sprintf(
			"[File: %s (%d lines), showing lines %d-%d (offset past end)]",
			resolved, totalLines, start, totalLines,
		)}, nil
	}

	limit := in.Limit
	if limit <= 0 {
		limit = DefaultReadLimit
	}
	endIdx := startIdx + limit
	if endIdx > totalLines {
		endIdx = totalLines
	}

	selected := allLines[startIdx:endIdx]

	var header string
	if startIdx == 0 && endIdx == totalLines {
		header = fmt.Sprintf("[File: %s (%d lines)]", resolved, totalLines)
	} else {
		header = fmt.Sprintf("[File: %s (%d lines), showing lines %d-%d]",
			resolved, totalLines, startIdx+1, endIdx)
	}

	body := formatLines(selected, startIdx+1)
	return tools.Result{Content: header + "\n" + body}, nil
}

// formatLines renders lines with cat -n style prefix: 6-char right-aligned
// line number + tab + content.
func formatLines(lines []string, startLine int) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%6d\t%s", startLine+i, line)
	}
	return b.String()
}
