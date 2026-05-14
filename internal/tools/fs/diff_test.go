package fs

import (
	"testing"
)

func TestBuildEditHunkSingleOccurrence(t *testing.T) {
	// 20-line file, replace line 10 (1 line) with 2 lines.
	oldLines := make([]string, 20)
	for i := range oldLines {
		oldLines[i] = "old" + itoa(i+1)
	}
	newLines := make([]string, 21)
	copy(newLines, oldLines[:9])
	newLines[9] = "new10a"
	newLines[10] = "new10b"
	copy(newLines[11:], oldLines[10:])

	h := buildEditHunk(oldLines, newLines, 10, 10, 1, 2)

	if h.OldStart != 7 || h.OldCount != 7 {
		t.Fatalf("old range want 7,7 got %d,%d", h.OldStart, h.OldCount)
	}
	if h.NewStart != 7 || h.NewCount != 8 {
		t.Fatalf("new range want 7,8 got %d,%d", h.NewStart, h.NewCount)
	}

	// Layout: 3 ctx, 1 remove, 2 add, 3 ctx = 9 lines.
	if got := len(h.Lines); got != 9 {
		t.Fatalf("lines want 9 got %d", got)
	}
	if h.Lines[0].Kind != LineContext || h.Lines[0].Old != 7 || h.Lines[0].New != 7 {
		t.Errorf("first leading ctx wrong: %+v", h.Lines[0])
	}
	if h.Lines[3].Kind != LineRemove || h.Lines[3].Old != 10 || h.Lines[3].New != 0 {
		t.Errorf("remove line wrong: %+v", h.Lines[3])
	}
	if h.Lines[4].Kind != LineAdd || h.Lines[4].Old != 0 || h.Lines[4].New != 10 {
		t.Errorf("first add line wrong: %+v", h.Lines[4])
	}
	if h.Lines[5].Kind != LineAdd || h.Lines[5].Old != 0 || h.Lines[5].New != 11 {
		t.Errorf("second add line wrong: %+v", h.Lines[5])
	}
	if h.Lines[6].Kind != LineContext || h.Lines[6].Old != 11 || h.Lines[6].New != 12 {
		t.Errorf("trailing ctx wrong: %+v", h.Lines[6])
	}
}

func TestBuildEditHunkSecondOccurrenceTracksDelta(t *testing.T) {
	// Two edits in a 20-line file: lines 10 and 18, each 1→2.
	// After first edit the file is 21 lines; second edit's new-side line
	// numbers should reflect delta=1.
	oldLines := make([]string, 20)
	newLines := make([]string, 22)

	// Second hunk: oldLineNum=18, newLineNum=19, delta=1.
	h := buildEditHunk(oldLines, newLines, 18, 19, 1, 2)

	if h.OldStart != 15 || h.NewStart != 16 {
		t.Fatalf("starts want old=15 new=16 got old=%d new=%d", h.OldStart, h.NewStart)
	}
	// leading=3 + removed=1 + added=2 + trailing=2 (file ends at line 20) = 8 lines
	if got := len(h.Lines); got != 8 {
		t.Fatalf("lines want 8 got %d", got)
	}
	if h.Lines[0].Old != 15 || h.Lines[0].New != 16 {
		t.Errorf("leading ctx must carry delta: got %+v", h.Lines[0])
	}
	// remove line at old=18, no new
	if h.Lines[3].Kind != LineRemove || h.Lines[3].Old != 18 {
		t.Errorf("remove line wrong: %+v", h.Lines[3])
	}
	// first add at new=19
	if h.Lines[4].Kind != LineAdd || h.Lines[4].New != 19 {
		t.Errorf("first add wrong: %+v", h.Lines[4])
	}
	// trailing ctx shifted: old=19 → new=21
	if h.Lines[6].Kind != LineContext || h.Lines[6].Old != 19 || h.Lines[6].New != 21 {
		t.Errorf("trailing ctx must shift: got %+v", h.Lines[6])
	}
}

func TestCreateDiff(t *testing.T) {
	d := buildCreateDiff("/tmp/x", "a\nb\nc")
	if d.Op != OpCreate || d.Path != "/tmp/x" {
		t.Fatalf("header wrong: %+v", d)
	}
	if len(d.Hunks) != 1 {
		t.Fatalf("hunks want 1 got %d", len(d.Hunks))
	}
	h := d.Hunks[0]
	if h.NewStart != 1 || h.NewCount != 3 {
		t.Fatalf("new range want 1,3 got %d,%d", h.NewStart, h.NewCount)
	}
	if h.OldStart != 0 || h.OldCount != 0 {
		t.Fatalf("old range want 0,0 got %d,%d", h.OldStart, h.OldCount)
	}
	for i, ln := range h.Lines {
		if ln.Kind != LineAdd || ln.New != i+1 {
			t.Errorf("line %d wrong: %+v", i, ln)
		}
	}
}

func TestLineNumberOf(t *testing.T) {
	s := "a\nb\nc\nd\n"
	cases := []struct {
		off  int
		want int
	}{
		{0, 1},
		{1, 1},  // still on line 1 (\n is end-of-line-1)
		{2, 2},  // start of line 2
		{4, 3},
		{6, 4},
		{8, 5}, // at trailing \n: byte index 8 is the start of a hypothetical line 5
		{99, 5}, // past end clamps to len(s)
	}
	for _, c := range cases {
		if got := lineNumberOf(s, c.off); got != c.want {
			t.Errorf("lineNumberOf(%d) want %d got %d", c.off, c.want, got)
		}
	}
}

func TestLineSpan(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"foo", 1},
		{"foo\n", 1},
		{"foo\nbar", 2},
		{"foo\nbar\n", 2},
	}
	for _, c := range cases {
		if got := lineSpan(c.in); got != c.want {
			t.Errorf("lineSpan(%q) want %d got %d", c.in, c.want, got)
		}
	}
}

// tiny local helper to avoid importing strconv just for tests
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
