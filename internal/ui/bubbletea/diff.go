package bubbletea

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/johnny1110/evva/internal/tools/fs"
)

// renderFileDiff returns a multi-line string showing the diff in git
// style: two columns of line numbers (old | new) and colored +/-/context
// rows. Used by the transcript renderer when a tool result carries an
// *fs.FileDiff in its Metadata.
func renderFileDiff(d *fs.FileDiff) string {
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
			var row string
			switch ln.Kind {
			case fs.LineAdd:
				row = styles.DiffAdd.Render(fmt.Sprintf("%4s %4s + %s", oldCol, newCol, ln.Text))
			case fs.LineRemove:
				row = styles.DiffRemove.Render(fmt.Sprintf("%4s %4s - %s", oldCol, newCol, ln.Text))
			default:
				row = styles.DiffContext.Render(fmt.Sprintf("%4s %4s   %s", oldCol, newCol, ln.Text))
			}
			b.WriteString(row)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func blankIfZero(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
