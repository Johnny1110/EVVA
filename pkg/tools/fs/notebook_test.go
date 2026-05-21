package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadNotebook_MarkdownAndCodeCells(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nb.ipynb")
	content := `{
		"cells": [
			{"cell_type": "markdown", "source": ["# Hello\n", "Some text"]},
			{"cell_type": "code", "source": "print('hi')\n", "outputs": [
				{"output_type": "stream", "name": "stdout", "text": "hi\n"}
			]}
		],
		"nbformat": 4
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	res := readNotebook(path)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	for _, want := range []string{
		"[Notebook:",
		"2 cells",
		"--- Cell 1 (markdown) ---",
		"# Hello",
		"Some text",
		"--- Cell 2 (code) ---",
		"print('hi')",
		"[outputs]",
		"hi",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %q in:\n%s", want, res.Content)
		}
	}
}

func TestReadNotebook_SourceAsStringOrArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nb.ipynb")
	// First cell: source is array. Second: source is plain string.
	content := `{
		"cells": [
			{"cell_type": "markdown", "source": ["line1\n", "line2"]},
			{"cell_type": "code", "source": "single string"}
		]
	}`
	os.WriteFile(path, []byte(content), 0o644)

	res := readNotebook(path)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "line1\nline2") {
		t.Errorf("array source should join without separator; got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "single string") {
		t.Errorf("string source should appear verbatim; got: %s", res.Content)
	}
}

func TestReadNotebook_TextPlainOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nb.ipynb")
	content := `{
		"cells": [
			{"cell_type": "code", "source": "1+1", "outputs": [
				{"output_type": "execute_result", "data": {"text/plain": "2"}}
			]}
		]
	}`
	os.WriteFile(path, []byte(content), 0o644)

	res := readNotebook(path)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "[outputs]") {
		t.Errorf("expected [outputs] marker; got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "2") {
		t.Errorf("expected text/plain output value '2'; got: %s", res.Content)
	}
}

func TestReadNotebook_MalformedJSONError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nb.ipynb")
	os.WriteFile(path, []byte("not json at all"), 0o644)

	res := readNotebook(path)
	if !res.IsError {
		t.Fatal("expected error for malformed notebook JSON")
	}
	if !strings.Contains(res.Content, "not a valid Jupyter notebook") {
		t.Errorf("error should mention invalid notebook; got: %s", res.Content)
	}
}

func TestReadNotebook_MissingFile(t *testing.T) {
	res := readNotebook("/no/such/path.ipynb")
	if !res.IsError {
		t.Fatal("expected error for missing notebook")
	}
}
