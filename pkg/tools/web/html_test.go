package web

import (
	"strings"
	"testing"
)

func TestExtractReadableText(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		contains []string
		absent   []string
	}{
		{
			name:     "plain paragraph",
			html:     `<html><body><p>hello world</p></body></html>`,
			contains: []string{"hello world"},
		},
		{
			name:     "script content stripped",
			html:     `<html><body><script>alert("nope")</script><p>kept</p></body></html>`,
			contains: []string{"kept"},
			absent:   []string{"alert", "nope"},
		},
		{
			name:     "style content stripped",
			html:     `<html><head><style>body{color:red}</style></head><body>body text</body></html>`,
			contains: []string{"body text"},
			absent:   []string{"color:red"},
		},
		{
			name:     "anchor renders with href",
			html:     `<html><body><a href="https://example.com">click me</a></body></html>`,
			contains: []string{"click me (https://example.com)"},
		},
		{
			name:     "anchor without href just renders text",
			html:     `<html><body><a>plain</a></body></html>`,
			contains: []string{"plain"},
			absent:   []string{"()"},
		},
		{
			name:     "nav and footer stripped",
			html:     `<html><body><nav>nav links</nav><main><p>real content</p></main><footer>copyright</footer></body></html>`,
			contains: []string{"real content"},
			absent:   []string{"nav links", "copyright"},
		},
		{
			name:     "block elements separated by blank lines",
			html:     `<html><body><p>first</p><p>second</p></body></html>`,
			contains: []string{"first\n\nsecond"},
		},
		{
			name:     "br produces newline",
			html:     `<html><body>line one<br>line two</body></html>`,
			contains: []string{"line one\nline two"},
		},
		{
			name:     "collapses excessive blank runs",
			html:     `<html><body><div></div><div></div><div></div><p>after</p></body></html>`,
			contains: []string{"after"},
			absent:   []string{"\n\n\n\n"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractReadableText(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("extractReadableText returned error: %v", err)
			}
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, got)
				}
			}
			for _, banned := range tc.absent {
				if strings.Contains(got, banned) {
					t.Errorf("output contained banned %q\nfull output:\n%s", banned, got)
				}
			}
		})
	}
}

func TestExtractReadableText_TrimsLeadingTrailing(t *testing.T) {
	got, err := extractReadableText(strings.NewReader(`<html><body><p>   surrounded   </p></body></html>`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(got, "\n") {
		t.Errorf("output not trimmed: %q", got)
	}
}
