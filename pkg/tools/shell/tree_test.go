package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnny1110/evva/pkg/tools"
)

// Phase 1 analysis — TreeTool.Execute code paths:
//   - decode input
//   - empty path rejected
//   - non-absolute path rejected
//   - default max_depth = 3
//   - explicit max_depth honored
//   - max_depth <= 0 falls through to default
//   - show_hidden gates dot-prefixed entries
//   - vendored dirs (.git, node_modules, ...) auto-skipped
//   - directories sort before files; both sorted alphabetically
//   - missing root → IO error surfaced

// writeTreeFixture builds a structured directory tree:
//   root/
//     .hidden/keep.txt          (hidden — only shown with show_hidden)
//     .git/HEAD                 (always skipped)
//     node_modules/lib.js       (always skipped)
//     pkg/
//       deep/
//         file.go
//     src/
//       a.go
//       b.go
//     README.md
func writeTreeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustWrite := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	mustWrite(".hidden/keep.txt", "x")
	mustWrite(".git/HEAD", "x")
	mustWrite("node_modules/lib.js", "x")
	mustWrite("pkg/deep/file.go", "x")
	mustWrite("src/a.go", "x")
	mustWrite("src/b.go", "x")
	mustWrite("README.md", "x")

	return root
}

func TestTree_RejectsEmptyPath(t *testing.T) {
	tool := &TreeTool{}
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"path":""}`))
	if !res.IsError || !strings.Contains(res.Content, "required") {
		t.Errorf("expected 'required'; got %q", res.Content)
	}
}

func TestTree_RejectsRelativePath(t *testing.T) {
	tool := &TreeTool{}
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"path":"relative/dir"}`))
	if !res.IsError || !strings.Contains(res.Content, "absolute") {
		t.Errorf("expected 'absolute'; got %q", res.Content)
	}
}

func TestTree_BasicWalk_SkipsHiddenAndVendoredByDefault(t *testing.T) {
	root := writeTreeFixture(t)
	tool := &TreeTool{}

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q}`, root)))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// Root name on the first line.
	first := strings.SplitN(res.Content, "\n", 2)[0]
	if first != filepath.Base(root) {
		t.Errorf("first line: got %q, want root basename %q", first, filepath.Base(root))
	}

	// Visible entries present.
	for _, want := range []string{"pkg/", "src/", "README.md", "a.go", "b.go"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("expected %q in tree; got\n%s", want, res.Content)
		}
	}
	// Hidden and vendored entries absent.
	for _, banned := range []string{".hidden", ".git", "node_modules"} {
		if strings.Contains(res.Content, banned) {
			t.Errorf("did NOT expect %q in default tree; got\n%s", banned, res.Content)
		}
	}
}

func TestTree_ShowHiddenIncludesDotEntries(t *testing.T) {
	root := writeTreeFixture(t)
	tool := &TreeTool{}

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q,"show_hidden":true}`, root)))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, ".hidden") {
		t.Errorf("show_hidden should reveal .hidden; got\n%s", res.Content)
	}
	// .git is still in skipDirs (filtered separately) and should NOT appear
	// even with show_hidden — vendored directories are unconditionally
	// hidden because they're always noise.
	if strings.Contains(res.Content, ".git/") {
		t.Errorf(".git should be skipped even with show_hidden; got\n%s", res.Content)
	}
}

func TestTree_MaxDepthLimitsRecursion(t *testing.T) {
	root := writeTreeFixture(t)
	tool := &TreeTool{}

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q,"max_depth":1}`, root)))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// At depth 1 we see pkg/ and src/, but NOT their children.
	if !strings.Contains(res.Content, "pkg/") {
		t.Errorf("expected pkg/ at depth 1; got\n%s", res.Content)
	}
	if strings.Contains(res.Content, "file.go") {
		t.Errorf("max_depth=1 should not descend into pkg/deep; got\n%s", res.Content)
	}
	if strings.Contains(res.Content, "a.go") {
		t.Errorf("max_depth=1 should not descend into src/; got\n%s", res.Content)
	}
}

func TestTree_NegativeMaxDepthFallsThroughToDefault(t *testing.T) {
	root := writeTreeFixture(t)
	tool := &TreeTool{}

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q,"max_depth":0}`, root)))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// max_depth=0 → falls through to default (3), so deep file is reached.
	if !strings.Contains(res.Content, "file.go") {
		t.Errorf("max_depth=0 should default; expected file.go in output\n%s", res.Content)
	}
}

func TestTree_DirectoriesSortBeforeFiles(t *testing.T) {
	root := writeTreeFixture(t)
	tool := &TreeTool{}

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q}`, root)))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	pkgIdx := strings.Index(res.Content, "pkg/")
	readmeIdx := strings.Index(res.Content, "README.md")
	if pkgIdx < 0 || readmeIdx < 0 {
		t.Fatalf("missing expected entries; got\n%s", res.Content)
	}
	if pkgIdx >= readmeIdx {
		t.Errorf("directories should sort BEFORE files; pkg/ at %d, README.md at %d", pkgIdx, readmeIdx)
	}
}

func TestTree_DirectorySuffixedWithSlash(t *testing.T) {
	root := writeTreeFixture(t)
	tool := &TreeTool{}

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q,"max_depth":1}`, root)))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// Every directory entry should carry a trailing slash so a reader can
	// distinguish dirs from files at a glance.
	if !strings.Contains(res.Content, "pkg/\n") && !strings.HasSuffix(res.Content, "pkg/") {
		t.Errorf("pkg should be rendered with trailing slash; got\n%s", res.Content)
	}
}

func TestTree_MissingPathSurfacesIOError(t *testing.T) {
	tool := &TreeTool{}
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(fmt.Sprintf(`{"path":%q}`, missing)))
	if !res.IsError {
		t.Fatalf("expected IsError for missing path; got %q", res.Content)
	}
}

func TestTree_DecodeError(t *testing.T) {
	tool := &TreeTool{}
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{nope`))
	if !res.IsError || !strings.Contains(res.Content, "decode") {
		t.Errorf("expected decode error; got %q", res.Content)
	}
}
