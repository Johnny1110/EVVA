package fs

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// Op enumerates the kinds of file mutation a FileDiff describes. Kept as
// strings so the wire/UI side can render without importing this package.
const (
	OpCreate    = "create"
	OpEdit      = "edit"
	OpOverwrite = "overwrite"
)

// LineKind enumerates how a single DiffLine relates to the change. Same
// semantics as a unified diff's prefix character.
const (
	LineContext = "context"
	LineAdd     = "add"
	LineRemove  = "remove"
)

// FileDiff is the structured payload write_file and edit_file attach to
// tools.Result.Metadata so UIs can render git-style diffs (red removes,
// green adds, line numbers in two columns) without parsing text.
//
// LLMs never see this — Content carries the model-facing summary.
type FileDiff struct {
	Path  string
	Op    string // create / edit / overwrite
	Hunks []DiffHunk
}

// DiffHunk groups consecutive related lines, mirroring unified-diff hunks.
// Start counts are 1-based; Count is the number of lines in that hunk
// from the respective side (matches the "@@ -OldStart,OldCount
// +NewStart,NewCount @@" header).
type DiffHunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []DiffLine
}

// DiffLine carries one rendered row. Old and New are 1-based line numbers
// on each side; the one that doesn't apply (e.g. New on a "remove" line)
// is 0 so the UI can render an empty cell.
type DiffLine struct {
	Kind string // context / add / remove
	Old  int    // 0 if not present on the old side
	New  int    // 0 if not present on the new side
	Text string
}

// ContextLines is how many unchanged lines we include above and below an
// edit hunk. Matches `diff -u` default.
const ContextLines = 3

// buildEditHunk constructs a single hunk centered on a replacement.
//
// oldLines and newLines are the pre-split full-file contents BEFORE and
// AFTER all replacements. oldLineNum is the 1-based first line of the
// replaced region in the original; newLineNum is the corresponding line
// in the post-edit file (== oldLineNum + delta accumulated from prior
// hunks in a replace_all scenario).
//
// changedOld and changedNew are the line counts the replacement spans on
// each side — these come from splitting old_string and new_string. The
// hunk includes ContextLines lines above and below.
func buildEditHunk(oldLines, newLines []string, oldLineNum, newLineNum, changedOld, changedNew int) DiffHunk {
	ctxStart := oldLineNum - ContextLines
	if ctxStart < 1 {
		ctxStart = 1
	}
	ctxEndOld := oldLineNum + changedOld - 1 + ContextLines
	if ctxEndOld > len(oldLines) {
		ctxEndOld = len(oldLines)
	}

	leadingCount := oldLineNum - ctxStart
	trailingCount := ctxEndOld - (oldLineNum + changedOld) + 1
	if trailingCount < 0 {
		trailingCount = 0
	}

	h := DiffHunk{
		OldStart: ctxStart,
		OldCount: leadingCount + changedOld + trailingCount,
		NewStart: newLineNum - leadingCount,
		NewCount: leadingCount + changedNew + trailingCount,
	}

	// Leading context — identical on both sides modulo the running delta.
	for i := 0; i < leadingCount; i++ {
		oldLn := ctxStart + i
		newLn := h.NewStart + i
		h.Lines = append(h.Lines, DiffLine{Kind: LineContext, Old: oldLn, New: newLn, Text: oldLines[oldLn-1]})
	}
	// Removed lines (old side only).
	for i := 0; i < changedOld; i++ {
		h.Lines = append(h.Lines, DiffLine{Kind: LineRemove, Old: oldLineNum + i, Text: oldLines[oldLineNum+i-1]})
	}
	// Added lines (new side only).
	for i := 0; i < changedNew; i++ {
		h.Lines = append(h.Lines, DiffLine{Kind: LineAdd, New: newLineNum + i, Text: newLines[newLineNum+i-1]})
	}
	// Trailing context — shifted on the new side by the size delta of
	// this hunk's own replacement.
	for i := 0; i < trailingCount; i++ {
		oldLn := oldLineNum + changedOld + i
		newLn := newLineNum + changedNew + i
		h.Lines = append(h.Lines, DiffLine{Kind: LineContext, Old: oldLn, New: newLn, Text: oldLines[oldLn-1]})
	}

	return h
}

// buildOverwriteDiff renders a unified-style diff between the pre- and
// post-overwrite content. Uses difflib's SequenceMatcher to compute
// minimal opcode ranges (replace / delete / insert / equal) and groups
// them into hunks with ContextLines lines of leading/trailing context,
// matching the convention of `diff -u`.
//
// Output mirrors what edit_file produces, so renderFileDiff can render
// overwrite payloads the same way as edit payloads — colored +/-, line
// numbers on both sides, hunk headers.
func buildOverwriteDiff(path, before, after string) *FileDiff {
	oldLines := splitLinesPreservingEnd(before)
	newLines := splitLinesPreservingEnd(after)

	matcher := difflib.NewMatcher(oldLines, newLines)
	groups := matcher.GetGroupedOpCodes(ContextLines)

	hunks := make([]DiffHunk, 0, len(groups))
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}

		// Hunk header spans the first opcode's start to the last
		// opcode's end on each side. Indices from difflib are 0-based
		// and end-exclusive; FileDiff line numbers are 1-based.
		first, last := group[0], group[len(group)-1]
		h := DiffHunk{
			OldStart: first.I1 + 1,
			OldCount: last.I2 - first.I1,
			NewStart: first.J1 + 1,
			NewCount: last.J2 - first.J1,
		}
		// difflib uses an "empty" convention where Start counts at 0
		// when the count is 0 (matches `diff -u`). Preserve that so
		// the rendered "@@ -0,0 +1,N @@" header for pure inserts /
		// "@@ -1,N +0,0 @@" for pure deletes reads correctly.
		if h.OldCount == 0 {
			h.OldStart = 0
		}
		if h.NewCount == 0 {
			h.NewStart = 0
		}

		for _, c := range group {
			switch c.Tag {
			case 'e': // equal
				for k := c.I1; k < c.I2; k++ {
					h.Lines = append(h.Lines, DiffLine{
						Kind: LineContext,
						Old:  k + 1,
						New:  c.J1 + (k - c.I1) + 1,
						Text: oldLines[k],
					})
				}
			case 'r': // replace — emit removes then adds
				for k := c.I1; k < c.I2; k++ {
					h.Lines = append(h.Lines, DiffLine{Kind: LineRemove, Old: k + 1, Text: oldLines[k]})
				}
				for k := c.J1; k < c.J2; k++ {
					h.Lines = append(h.Lines, DiffLine{Kind: LineAdd, New: k + 1, Text: newLines[k]})
				}
			case 'd': // delete (old-only)
				for k := c.I1; k < c.I2; k++ {
					h.Lines = append(h.Lines, DiffLine{Kind: LineRemove, Old: k + 1, Text: oldLines[k]})
				}
			case 'i': // insert (new-only)
				for k := c.J1; k < c.J2; k++ {
					h.Lines = append(h.Lines, DiffLine{Kind: LineAdd, New: k + 1, Text: newLines[k]})
				}
			}
		}
		hunks = append(hunks, h)
	}

	return &FileDiff{Path: path, Op: OpOverwrite, Hunks: hunks}
}

// buildCreateDiff renders the diff for a brand-new file: every line is an
// add. Empty content yields a hunk with zero lines.
func buildCreateDiff(path, content string) *FileDiff {
	lines := splitLinesPreservingEnd(content)
	hunk := DiffHunk{
		OldStart: 0,
		OldCount: 0,
		NewStart: 1,
		NewCount: len(lines),
	}
	for i, line := range lines {
		hunk.Lines = append(hunk.Lines, DiffLine{Kind: LineAdd, New: i + 1, Text: line})
	}
	return &FileDiff{Path: path, Op: OpCreate, Hunks: []DiffHunk{hunk}}
}

// splitLinesPreservingEnd splits s on "\n" without yielding an empty
// trailing element when s ends with a newline. An empty string yields
// no lines (distinct from `strings.Split` which yields one empty element).
func splitLinesPreservingEnd(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lineNumberOf returns the 1-based line number of the byte at index off
// inside s. Counts '\n' occurrences before off.
func lineNumberOf(s string, off int) int {
	if off <= 0 {
		return 1
	}
	if off > len(s) {
		off = len(s)
	}
	return strings.Count(s[:off], "\n") + 1
}
