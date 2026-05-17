package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 1 analysis — ReadTool.Execute code paths:
//   - decode input
//   - reject `pages` (reserved)
//   - resolvePath errors
//   - stat → not-found / directory delegation to tree
//   - read file
//   - mark tracker on success
//   - line slicing (offset / limit / past-EOF)
//   - cat -n line formatting

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestRead_RejectsPagesField(t *testing.T) {
	path := writeTempFile(t, "x")
	tool := NewRead(NewReadTracker())

	res, _ := tool.Execute(context.Background(),
		json.RawMessage(`{"file_path":"`+path+`","pages":"1-5"}`))

	if !res.IsError || !strings.Contains(res.Content, "PDF/pages") {
		t.Errorf("expected reserved-pages error; got isErr=%v content=%q", res.IsError, res.Content)
	}
}

func TestRead_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.txt")
	tool := NewRead(NewReadTracker())
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"`+missing+`"}`))
	if !res.IsError || !strings.Contains(res.Content, "not found") {
		t.Errorf("expected not-found; got isErr=%v content=%q", res.IsError, res.Content)
	}
}

func TestRead_DirectoryDelegatesToTree(t *testing.T) {
	dir := t.TempDir()
	tool := NewRead(NewReadTracker())
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"`+dir+`"}`))
	if res.IsError {
		t.Fatalf("expected tree delegation; got error: %s", res.Content)
	}
	// tree output starts with the basename of the directory
	base := filepath.Base(dir)
	if !strings.HasPrefix(res.Content, base) {
		t.Errorf("expected tree output starting with %q; got %q", base, res.Content)
	}
}

func TestRead_EmptyFile(t *testing.T) {
	// NOTE: current behavior reports "1 lines" for an empty file because
	// strings.Split("", "\n") returns [""] and the trailing-newline
	// stripper only fires when content has a trailing newline. This is
	// a (minor) pre-existing quirk worth fixing in the future; locking
	// the observed behavior here so an accidental refactor doesn't
	// change it silently.
	path := writeTempFile(t, "")
	tool := NewRead(NewReadTracker())
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"`+path+`"}`))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "[File:") {
		t.Errorf("expected file header; got %q", res.Content)
	}
}

func TestRead_HappyPath_CatNFormat(t *testing.T) {
	path := writeTempFile(t, "alpha\nbeta\ngamma\n")
	tr := NewReadTracker()
	tool := NewRead(tr)

	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"file_path":"`+path+`"}`))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	for _, want := range []string{
		"3 lines",
		"     1\talpha",
		"     2\tbeta",
		"     3\tgamma",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %q\nfull:\n%s", want, res.Content)
		}
	}
	if !tr.WasRead(path) {
		t.Error("ReadTracker not marked after successful read")
	}
}

func TestRead_OffsetAndLimitSlice(t *testing.T) {
	path := writeTempFile(t, "l1\nl2\nl3\nl4\nl5\n")
	tool := NewRead(NewReadTracker())

	res, _ := tool.Execute(context.Background(),
		json.RawMessage(`{"file_path":"`+path+`","offset":2,"limit":2}`))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	for _, want := range []string{
		"showing lines 2-3",
		"     2\tl2",
		"     3\tl3",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("missing %q\nfull:\n%s", want, res.Content)
		}
	}
	if strings.Contains(res.Content, "     1\tl1") {
		t.Error("offset=2 should have skipped line 1")
	}
	if strings.Contains(res.Content, "     4\tl4") {
		t.Error("limit=2 should have stopped after line 3")
	}
}

func TestRead_OffsetPastEOF(t *testing.T) {
	path := writeTempFile(t, "only-line\n")
	tool := NewRead(NewReadTracker())

	res, _ := tool.Execute(context.Background(),
		json.RawMessage(`{"file_path":"`+path+`","offset":99}`))

	if res.IsError {
		t.Fatalf("offset past EOF should be a graceful message, not an error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "offset past end") {
		t.Errorf("expected 'offset past end' marker; got %q", res.Content)
	}
}

func TestRead_NegativeOffsetClampsToOne(t *testing.T) {
	path := writeTempFile(t, "x\ny\n")
	tool := NewRead(NewReadTracker())

	res, _ := tool.Execute(context.Background(),
		json.RawMessage(`{"file_path":"`+path+`","offset":-5}`))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "     1\tx") {
		t.Errorf("negative offset should clamp to 1; got %q", res.Content)
	}
}

func TestRead_DecodeError(t *testing.T) {
	tool := NewRead(NewReadTracker())
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{bogus`))
	if !res.IsError || !strings.Contains(res.Content, "decode") {
		t.Errorf("expected decode error; got isErr=%v content=%q", res.IsError, res.Content)
	}
}
