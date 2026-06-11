package lsp

import "testing"

func TestURIRoundTrip(t *testing.T) {
	tests := []struct {
		goos, path, uri string
	}{
		{"linux", "/home/u/x.go", "file:///home/u/x.go"},
		{"darwin", "/Users/u/x.go", "file:///Users/u/x.go"},
		{"windows", `C:\Users\u\x.go`, "file:///C:/Users/u/x.go"},
		{"windows", `D:\a b\x.go`, "file:///D:/a b/x.go"},
	}
	for _, tt := range tests {
		if got := uriFromPathFor(tt.goos, tt.path); got != tt.uri {
			t.Errorf("uriFromPathFor(%s, %q) = %q, want %q", tt.goos, tt.path, got, tt.uri)
		}
		if got := pathFromURIFor(tt.goos, tt.uri); got != tt.path {
			t.Errorf("pathFromURIFor(%s, %q) = %q, want %q", tt.goos, tt.uri, got, tt.path)
		}
	}
}

func TestPathFromURIEdgeCases(t *testing.T) {
	// Lowercase drive letters (gopls on Windows emits them).
	if got := pathFromURIFor("windows", "file:///c:/x/y.go"); got != `c:\x\y.go` {
		t.Errorf("lowercase drive: got %q", got)
	}
	// Non-file URIs pass through untouched.
	if got := pathFromURIFor("windows", "untitled:Untitled-1"); got != "untitled:Untitled-1" {
		t.Errorf("non-file URI mangled: %q", got)
	}
}
