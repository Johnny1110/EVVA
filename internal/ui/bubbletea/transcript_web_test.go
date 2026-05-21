package bubbletea

import (
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/agent/event"
	"github.com/johnny1110/evva/pkg/tools"
)

// TestWebFetchResultCollapsesToFirstLine verifies the user-facing
// transcript shows only the "[Fetched: ...]" header line for
// successful web_fetch results, not the raw page text the model
// receives. The model still gets the full body in its tool_result
// message — this only affects what the user sees.
func TestWebFetchResultCollapsesToFirstLine(t *testing.T) {
	tr := transcript{
		width:               80,
		textInflightIdx:     -1,
		thinkingInflightIdx: -1,
		bannerIdx:           -1,
		toolBlocks:          map[string]int{},
	}

	// Simulate KindToolUseStart → KindToolUseResult for web_fetch.
	tr.foldEvent(event.Event{
		Kind: event.KindToolUseStart,
		ToolUseStart: &event.ToolUseStartPayload{
			Name:   string(tools.WEB_FETCH),
			ToolID: "call_1",
			Input:  []byte(`{"url":"https://example.com"}`),
		},
	})

	header := "[Fetched: https://example.com (text/html, 5920 chars)]"
	bigBody := strings.Repeat("page text line\n", 200)
	tr.foldEvent(event.Event{
		Kind: event.KindToolUseResult,
		ToolUseResult: &event.ToolUseResultPayload{
			ToolID:  "call_1",
			Content: header + "\n\n" + bigBody,
		},
	})

	out := tr.String()
	if !strings.Contains(out, header) {
		t.Fatalf("transcript missing the header line: %s", out)
	}
	if strings.Contains(out, "page text line") {
		t.Fatalf("raw body leaked into transcript:\n%s", out)
	}
}

// TestWebSearchResultCollapsesToFirstLine: same contract for
// web_search — only the "Search results for ..." header is shown.
func TestWebSearchResultCollapsesToFirstLine(t *testing.T) {
	tr := transcript{
		width:               80,
		textInflightIdx:     -1,
		thinkingInflightIdx: -1,
		bannerIdx:           -1,
		toolBlocks:          map[string]int{},
	}

	tr.foldEvent(event.Event{
		Kind: event.KindToolUseStart,
		ToolUseStart: &event.ToolUseStartPayload{
			Name:   string(tools.WEB_SEARCH),
			ToolID: "call_2",
			Input:  []byte(`{"query":"go atomic"}`),
		},
	})

	header := `Search results for "go atomic":`
	bigBody := "1. **A** — https://a\n   long snippet\n\n2. **B** — https://b\n   another snippet\n"
	tr.foldEvent(event.Event{
		Kind: event.KindToolUseResult,
		ToolUseResult: &event.ToolUseResultPayload{
			ToolID:  "call_2",
			Content: header + "\n\n" + bigBody,
		},
	})

	out := tr.String()
	if !strings.Contains(out, header) {
		t.Fatalf("transcript missing the header line: %s", out)
	}
	if strings.Contains(out, "long snippet") || strings.Contains(out, "another snippet") {
		t.Fatalf("raw snippets leaked into transcript:\n%s", out)
	}
}

// TestWebFetchErrorStaysVerbose: when web_fetch fails, the user
// must see the error body — collapsing it would hide actionable
// information (404, DNS failure, etc.).
func TestWebFetchErrorStaysVerbose(t *testing.T) {
	tr := transcript{
		width:               80,
		textInflightIdx:     -1,
		thinkingInflightIdx: -1,
		bannerIdx:           -1,
		toolBlocks:          map[string]int{},
	}

	tr.foldEvent(event.Event{
		Kind: event.KindToolUseStart,
		ToolUseStart: &event.ToolUseStartPayload{
			Name:   string(tools.WEB_FETCH),
			ToolID: "call_3",
			Input:  []byte(`{"url":"https://nope.example.invalid"}`),
		},
	})

	errBody := "web_fetch: request failed: dial tcp: lookup nope.example.invalid: no such host"
	tr.foldEvent(event.Event{
		Kind: event.KindToolUseResult,
		ToolUseResult: &event.ToolUseResultPayload{
			ToolID:  "call_3",
			Content: errBody,
			IsError: true,
		},
	})

	out := tr.String()
	// The body wraps to the configured width — check for a substring
	// that's preserved verbatim regardless of how the line breaks land.
	if !strings.Contains(out, "request failed") {
		t.Fatalf("error body was collapsed; need full message for user:\n%s", out)
	}
}

// TestFirstNonEmptyLine: helper sanity.
func TestFirstNonEmptyLine(t *testing.T) {
	cases := map[string]string{
		"hello\nworld":         "hello",
		"\n\n  header\nbody":   "header",
		"  trim me  ":          "trim me",
		"":                     "",
		"single":               "single",
		"\n\n\n":               "\n\n\n", // all blank: function falls back to original input
	}
	for in, want := range cases {
		if got := firstNonEmptyLine(in); got != want {
			t.Errorf("firstNonEmptyLine(%q): want %q, got %q", in, want, got)
		}
	}
}
