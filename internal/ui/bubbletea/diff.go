package bubbletea

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/johnny1110/evva/pkg/tools/fs"
)

// renderFileDiff returns a multi-line string showing the diff in git
// style: two columns of line numbers (old | new) and colored +/-/context
// rows. Used by the transcript renderer when a tool result carries an
// *fs.FileDiff in its Metadata.
//
// width is the column count to fill — each `+` and `-` row is padded
// out to width so its background tint reads as a solid block stretching
// across the transcript column. Zero or negative width disables the
// fill (lines render at their natural length); useful for tests.
func renderFileDiff(d *fs.FileDiff, width int) string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.DiffHeader.Render(fmt.Sprintf("diff %s a/%s b/%s", d.Op, d.Path, d.Path)))
	b.WriteByte('\n')

	for _, h := range d.Hunks {
		b.WriteString(styles.DiffHeader.Render(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)))
		b.WriteByte('\n')
		for _, ln := range h.Lines {
			oldCol := blankIfZero(ln.Old)
			newCol := blankIfZero(ln.New)
			text := fmt.Sprintf("%4s %4s %s %s", oldCol, newCol, signFor(ln.Kind), ln.Text)
			var row string
			switch ln.Kind {
			case fs.LineAdd:
				row = fillStyle(styles.DiffAdd, text, width)
			case fs.LineRemove:
				row = fillStyle(styles.DiffRemove, text, width)
			default:
				// Context rows stay un-filled — only the change rows
				// get the colored block treatment so the eye lands
				// on additions and removals first.
				row = styles.DiffContext.Render(text)
			}
			b.WriteString(row)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// fillStyle renders s through style, padding the output to width so
// the style's Background extends across the whole row. width<=0 falls
// back to natural rendering (no fill) so the function is safe in test
// contexts that don't know the terminal column count.
func fillStyle(style lipgloss.Style, s string, width int) string {
	if width <= 0 {
		return style.Render(s)
	}
	return style.Width(width).Render(s)
}

// signFor returns the unified-diff sign character for the line kind.
func signFor(kind string) string {
	switch kind {
	case fs.LineAdd:
		return "+"
	case fs.LineRemove:
		return "-"
	default:
		return " "
	}
}

func blankIfZero(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
