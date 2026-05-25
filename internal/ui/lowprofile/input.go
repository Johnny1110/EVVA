package lowprofile

import (
	"strings"
	"unicode/utf8"
)

// Input is the prompt text buffer. Manages cursor, text content, paste
// compaction, and history navigation. The App drives it via keystrokes
// and reads rendered output via View.
type Input struct {
	text   []rune
	pos    int // cursor position in rune offset
	width  int // terminal columns for display

	pasted []string // bracketed paste buffer

	history      []string
	historyIdx   int    // -1 when not navigating
	historyDraft string // saved draft when nav engaged via Up

	th *Theme
}

// NewInput returns a fresh input component.
func NewInput(th *Theme) *Input {
	return &Input{th: th, historyIdx: -1}
}

// SetWidth updates the display column count.
func (in *Input) SetWidth(w int) { in.width = w }

// Value returns the raw text.
func (in *Input) Value() string { return string(in.text) }

// SetValue replaces the buffer.
func (in *Input) SetValue(s string) {
	in.text = []rune(s)
	in.pos = len(in.text)
}

// Reset clears the buffer and paste stash.
func (in *Input) Reset() {
	in.text = nil
	in.pos = 0
	in.pasted = nil
	in.historyIdx = -1
	in.historyDraft = ""
}

// Empty reports whether the trimmed text is blank.
func (in *Input) Empty() bool {
	return strings.TrimSpace(string(in.text)) == ""
}

// InsertRune adds a character at the cursor.
func (in *Input) InsertRune(r rune) {
	in.text = append(in.text, 0)
	copy(in.text[in.pos+1:], in.text[in.pos:])
	in.text[in.pos] = r
	in.pos++
}

// InsertString adds a string at the cursor.
func (in *Input) InsertString(s string) {
	rs := []rune(s)
	in.text = append(in.text, make([]rune, len(rs))...)
	copy(in.text[in.pos+len(rs):], in.text[in.pos:])
	copy(in.text[in.pos:], rs)
	in.pos += len(rs)
}

// DeleteBefore removes the rune before the cursor.
func (in *Input) DeleteBefore() {
	if in.pos > 0 {
		in.pos--
		copy(in.text[in.pos:], in.text[in.pos+1:])
		in.text = in.text[:len(in.text)-1]
	}
}

// DeleteAfter removes the rune at the cursor.
func (in *Input) DeleteAfter() {
	if in.pos < len(in.text) {
		copy(in.text[in.pos:], in.text[in.pos+1:])
		in.text = in.text[:len(in.text)-1]
	}
}

// MoveLeft moves the cursor one rune left.
func (in *Input) MoveLeft() {
	if in.pos > 0 {
		in.pos--
	}
}

// MoveRight moves the cursor one rune right.
func (in *Input) MoveRight() {
	if in.pos < len(in.text) {
		in.pos++
	}
}

// MoveHome moves to the start of the current line or the whole buffer.
func (in *Input) MoveHome() { in.pos = 0 }

// MoveEnd moves to the end of the buffer.
func (in *Input) MoveEnd() { in.pos = len(in.text) }

// HandlePaste stores a large paste compactly and inserts a placeholder.
func (in *Input) HandlePaste(content string) {
	if !shouldCompactPaste(content) {
		in.InsertString(content)
		return
	}
	in.pasted = append(in.pasted, content)
	ph := pastePlaceholder(len(content))
	in.InsertString(ph)
}

// ExpandPaste returns the agent-facing text with paste placeholders expanded.
func (in *Input) ExpandPaste() string {
	text := string(in.text)
	for _, p := range in.pasted {
		text = strings.Replace(text, pastePlaceholder(len(p)), p, 1)
	}
	return text
}

// ViewPaste returns the display-facing text with paste chips.
func (in *Input) ViewText() string {
	text := string(in.text)
	for _, p := range in.pasted {
		text = strings.Replace(text, pastePlaceholder(len(p)),
			in.th.PasteChip.Render("[paste "+humanBytes(len(p))+"]"), 1)
	}
	return text
}

// HistoryPrev walks back through history. Returns true when the key was
// consumed (caller should not forward to cursor movement).
func (in *Input) HistoryPrev() bool {
	if len(in.history) == 0 {
		return false
	}
	inNav := in.historyIdx != -1
	if !inNav && strings.TrimSpace(string(in.text)) != "" {
		return false
	}
	if !inNav {
		in.historyDraft = string(in.text)
		in.historyIdx = len(in.history) - 1
	} else if in.historyIdx > 0 {
		in.historyIdx--
	}
	in.SetValue(in.history[in.historyIdx])
	return true
}

// HistoryNext walks forward through history. Past the newest entry,
// restores the saved draft. Returns true only while nav is active.
func (in *Input) HistoryNext() bool {
	if in.historyIdx == -1 {
		return false
	}
	in.historyIdx++
	if in.historyIdx >= len(in.history) {
		in.historyIdx = -1
		draft := in.historyDraft
		in.historyDraft = ""
		in.SetValue(draft)
		return true
	}
	in.SetValue(in.history[in.historyIdx])
	return true
}

// RecordHistory appends the current text to history (skips duplicates).
func (in *Input) RecordHistory() {
	text := strings.TrimSpace(string(in.text))
	if text == "" {
		return
	}
	if n := len(in.history); n == 0 || in.history[n-1] != text {
		in.history = append(in.history, text)
	}
	in.historyIdx = -1
	in.historyDraft = ""
}

// View renders the input line with the gold ▸ prompt.
func (in *Input) View() string {
	if in.width < 10 {
		return in.th.InputPrompt.Render("▸ ")
	}
	before := string(in.text[:in.pos])
	atCursor := ""
	if in.pos < len(in.text) {
		atCursor = string(in.text[in.pos])
	}
	after := ""
	if in.pos+1 < len(in.text) {
		after = string(in.text[in.pos+1:])
	}

	// Show cursor as the character at position with inverted style,
	// or a block if at end.
	cursorChar := " "
	if atCursor != "" {
		cursorChar = atCursor
	}
	cursor := in.th.InputCursor.Reverse(true).Render(cursorChar)

	line := in.th.InputPrompt.Render("▸ ") +
		in.th.InputText.Render(before) +
		cursor +
		in.th.InputText.Render(after)

	// Truncate to width
	runes := []rune(line)
	if len(runes) > in.width {
		// show end of line
		start := len(runes) - in.width
		if start < 0 {
			start = 0
		}
		line = string(runes[start:])
	}
	return line
}

// --- helpers ---

func shouldCompactPaste(s string) bool {
	return strings.Count(s, "\n") > 1 || len(s) > 200
}

func pastePlaceholder(size int) string {
	return "«paste:" + humanBytes(size) + "»"
}

func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return itoa(n/(1024*1024)) + "MB"
	case n >= 1024:
		return itoa(n/1024) + "KB"
	default:
		return itoa(n) + "B"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// runeCount is used internally.
func runeCount(s string) int { return utf8.RuneCountInString(s) }
