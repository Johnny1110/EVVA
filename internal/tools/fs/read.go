package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
)

// DefaultReadLimit caps an unbounded Read at this many lines. The model
// can pass an explicit larger limit when it really needs more, but the
// default protects the context window from accidental 50k-line dumps.
// Matches Claude Code's MAX_LINES_TO_READ.
const DefaultReadLimit = 2000

// fileUnchangedStub is what we return when the model re-reads a file
// whose mtime hasn't moved since the last full-file read. Reading it
// again would just burn cache tokens for the same content.
const fileUnchangedStub = "File unchanged since last read. The content from the earlier Read tool_result in this conversation is still current — refer to that instead of re-reading."

// imageExts is the set of extensions we route to the Phase 1b stub
// (images aren't yet round-tripped through tool_result blocks; see
// CLAUDE.md Phase 1b for the cross-cutting refactor). PDF lives in the
// shared binaryExtensions blocklist; this set is consulted before that
// check so we emit the Phase 1b message instead of a generic
// "binary file" rejection.
var imageExts = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".webp": {},
	".bmp":  {},
	".svg":  {},
}

type ReadTool struct {
	tracker *ReadTracker
}

func NewRead(tracker *ReadTracker) *ReadTool {
	return &ReadTool{tracker: tracker}
}

func (t *ReadTool) Name() string { return string(tools.READ_FILE) }

func (t *ReadTool) Description() string {
	return `Reads a file from the local filesystem. You can access any file directly by using this tool.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters
- Results are returned using cat -n format, with line numbers starting at 1
- This tool can read PDF files (.pdf). For large PDFs (more than 10 pages), you MUST provide the pages parameter to read specific page ranges (e.g., pages: "1-5"). Reading a large PDF without the pages parameter will fail. Maximum 20 pages per request.
- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.
- This tool can only read files, not directories. To list a directory, run ` + "`ls`" + ` via the bash tool.
- Image file reads (PNG/JPG/GIF/WebP/BMP/SVG) are not yet supported — tracked in CLAUDE.md as Phase 1b. Attempting to read an image file returns an error today.
- Binary files (executables, archives, fonts, native libraries, etc.) are rejected — their bytes would corrupt the conversation context. Use the bash tool with specialized utilities (file, hexdump, strings, jq, etc.) if you need to inspect a binary.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.
- Reading a file marks it as loaded into the session — edit_file and write_file (overwrite) refuse to touch a file you haven't read first, and force a re-read if the file's mtime advances on disk between reads.
- When editing text from this tool's output, strip the leading "<line_number>\t" prefix from each line — that's the line-number gutter, not file content.`
}

func (t *ReadTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["file_path"],
		"properties":{
			"file_path":{"type":"string","description":"The absolute path to the file to read."},
			"offset":{"type":"integer","minimum":1,"description":"The line number to start reading from. Only provide if the file is too large to read at once."},
			"limit":{"type":"integer","exclusiveMinimum":0,"description":"The number of lines to read. Only provide if the file is too large to read at once."},
			"pages":{"type":"string","description":"Page range for PDF files (e.g., \"1-5\", \"3\", \"10-20\"). Only applicable to PDF files. Maximum 20 pages per request."}
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

	resolved, err := resolvePath(in.FilePath)
	if err != nil {
		return tools.Result{IsError: true, Content: "read: " + err.Error()}, nil
	}

	// Device-path check runs before stat — /dev/tty etc. would block
	// or stream forever if we let os.Stat or os.ReadFile reach them.
	if isBlockedDevicePath(resolved) {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("read: cannot read '%s': this device file would block or produce infinite output.", in.FilePath),
		}, nil
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: file not found: %s", in.FilePath)}, nil
	}
	if info.IsDir() {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("read: %s is a directory — this tool only reads files. To list the directory run `ls` via the bash tool.", in.FilePath),
		}, nil
	}

	ext := strings.ToLower(filepath.Ext(resolved))

	if _, isImage := imageExts[ext]; isImage {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("read: image file reads (%s) are not yet supported in evva. Tracked in CLAUDE.md as Phase 1b — multimodal tool_result blocks across providers.", ext),
		}, nil
	}

	if ext == ".pdf" {
		res := readPDF(resolved, in.Pages)
		if !res.IsError && t.tracker != nil {
			t.tracker.Record(resolved, info.ModTime(), in.Pages != "")
		}
		return res, nil
	}

	if ext == ".ipynb" {
		if in.Offset > 0 || in.Limit > 0 || in.Pages != "" {
			return tools.Result{
				IsError: true,
				Content: "read: offset/limit/pages are not supported for Jupyter notebooks (.ipynb). Drop those parameters and re-call.",
			}, nil
		}
		res := readNotebook(resolved)
		if !res.IsError && t.tracker != nil {
			t.tracker.Record(resolved, info.ModTime(), false)
		}
		return res, nil
	}

	// Binary-extension rejection runs after PDF / notebook / image
	// dispatch so handled formats aren't caught by the generic
	// blocklist (PDF is in BINARY_EXTENSIONS in ref).
	if hasBinaryExtension(resolved) {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("read: %s appears to be a binary %s file — its bytes can't be meaningfully presented as text. Use the bash tool with file/hexdump/strings/jq for binary inspection.", in.FilePath, ext),
		}, nil
	}

	if in.Pages != "" {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("read: the `pages` parameter is only valid for PDF files; %s is not a PDF.", in.FilePath),
		}, nil
	}

	return t.readText(resolved, info, in)
}

func (t *ReadTool) readText(resolved string, info os.FileInfo, in readInput) (tools.Result, error) {
	explicitOffset := in.Offset > 0
	explicitLimit := in.Limit > 0

	// File-unchanged stub: a re-read of the same full file at the same
	// mtime is a no-op for the model — point them at the earlier
	// tool_result instead of dumping the same bytes again.
	if t.tracker != nil && !explicitOffset && !explicitLimit {
		if entry, ok := t.tracker.Lookup(resolved); ok && !entry.IsPartialView && entry.Timestamp.Equal(info.ModTime()) {
			return tools.Result{Content: fileUnchangedStub}, nil
		}
	}

	// readFileWithEncoding handles UTF-16 LE BOM detection, UTF-8 BOM
	// stripping, and CRLF→LF normalization. The model sees clean
	// LF-only UTF-8 regardless of how Windows / Notepad / cloud-sync
	// shaped the file.
	mem, err := readFileWithEncoding(resolved)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: could not read %s: %s", resolved, err)}, nil
	}

	allLines := splitForRead(mem.content)
	totalLines := len(allLines)

	if totalLines == 0 {
		if t.tracker != nil {
			t.tracker.Record(resolved, info.ModTime(), false)
		}
		// System-reminder framing (ref FILE_READ_TOOL behavior) so
		// the model treats this as a content warning, not actual
		// file content.
		return tools.Result{Content: fmt.Sprintf(
			"[File: %s, 0 lines]\n<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>",
			resolved,
		)}, nil
	}

	start := in.Offset
	if start < 1 {
		start = 1
	}
	startIdx := start - 1
	if startIdx >= totalLines {
		// Past-EOF doesn't update the tracker — the model didn't
		// actually see file content. System-reminder framing per ref.
		return tools.Result{Content: fmt.Sprintf(
			"[File: %s (%d lines)]\n<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>",
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

	partial := explicitOffset || endIdx < totalLines
	if t.tracker != nil {
		t.tracker.Record(resolved, info.ModTime(), partial)
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

// splitForRead splits file content into lines using "\n" as terminator
// (not separator). An empty string yields zero lines; "a\n" yields one;
// "a\nb" yields two; "a\nb\n" yields two. Matches what the user means
// by "line count" rather than strings.Split's separator semantics.
func splitForRead(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// formatLines renders the cat -n style prefix: 6-char right-aligned
// 1-based line number, then a tab, then the line text.
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
