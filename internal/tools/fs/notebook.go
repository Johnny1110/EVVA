package fs

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
)

// notebook is the subset of the Jupyter .ipynb v4 schema we render.
// We deliberately ignore execution_count, metadata, kernel info,
// attachments, and outputs that aren't plain text — they don't help
// the model reason about the code.
type notebook struct {
	NbformatMajor int            `json:"nbformat"`
	Cells         []notebookCell `json:"cells"`
}

type notebookCell struct {
	CellType string           `json:"cell_type"` // "code" | "markdown" | "raw"
	Source   sourceField      `json:"source"`
	Outputs  []notebookOutput `json:"outputs,omitempty"`
}

// notebookOutput keeps only the text-bearing fields. `text` is used
// by stream outputs; `data` is used by display_data / execute_result
// — when `data["text/plain"]` is present we render it, otherwise we
// skip the output (rich images / HTML aren't text-renderable).
type notebookOutput struct {
	OutputType string                     `json:"output_type"`
	Name       string                     `json:"name,omitempty"` // "stdout" / "stderr" for stream outputs
	Text       sourceField                `json:"text,omitempty"`
	Data       map[string]json.RawMessage `json:"data,omitempty"`
}

// sourceField handles the .ipynb quirk where text fields can be
// either a single string or an array of strings (joined on render).
type sourceField struct {
	lines []string
}

func (s *sourceField) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		s.lines = nil
		return nil
	}
	var asString string
	if err := json.Unmarshal(b, &asString); err == nil {
		s.lines = []string{asString}
		return nil
	}
	var asArray []string
	if err := json.Unmarshal(b, &asArray); err == nil {
		s.lines = asArray
		return nil
	}
	return fmt.Errorf("notebook source must be string or []string")
}

func (s sourceField) text() string {
	return strings.Join(s.lines, "")
}

func readNotebook(resolved string) tools.Result {
	data, err := os.ReadFile(resolved)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: could not read notebook %s: %s", resolved, err)}
	}

	var nb notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: %s is not a valid Jupyter notebook: %s", resolved, err)}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "[Notebook: %s (%d cells)]\n", resolved, len(nb.Cells))

	for i, cell := range nb.Cells {
		fmt.Fprintf(&out, "\n--- Cell %d (%s) ---\n", i+1, cell.CellType)
		src := cell.Source.text()
		out.WriteString(src)
		if !strings.HasSuffix(src, "\n") {
			out.WriteByte('\n')
		}
		if cell.CellType != "code" || len(cell.Outputs) == 0 {
			continue
		}
		fmt.Fprintf(&out, "[outputs]\n")
		for _, o := range cell.Outputs {
			text := renderNotebookOutput(o)
			if text == "" {
				continue
			}
			out.WriteString(text)
			if !strings.HasSuffix(text, "\n") {
				out.WriteByte('\n')
			}
		}
	}

	return tools.Result{Content: out.String()}
}

func renderNotebookOutput(o notebookOutput) string {
	switch o.OutputType {
	case "stream":
		// stream outputs (stdout / stderr) live in `text`.
		return o.Text.text()
	case "display_data", "execute_result":
		// Pick text/plain out of the MIME bundle. Skip if absent —
		// image/png and friends aren't text-renderable here.
		if raw, ok := o.Data["text/plain"]; ok {
			var sf sourceField
			if err := json.Unmarshal(raw, &sf); err == nil {
				return sf.text()
			}
		}
		return ""
	case "error":
		// Errors land in `data` under different keys depending on
		// version; many notebooks instead use a top-level
		// `traceback` array we don't currently decode. Skip with a
		// generic marker so the model knows something went wrong.
		return "[error output omitted]"
	default:
		return ""
	}
}
