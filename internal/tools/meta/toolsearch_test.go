package meta

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/tools"
)

// ---- test fixtures ----

func testDescriptors() []tools.Descriptor {
	return []tools.Descriptor{
		{Name: "notebook_edit", Tags: []string{"notebook", "jupyter", "ipynb", "cell", "edit"}},
		{Name: "web_search", Tags: []string{"web", "search", "google", "internet", "lookup"}},
		{Name: "web_fetch", Tags: []string{"http", "url", "web", "fetch", "scrape"}},
		{Name: "task_create", Tags: []string{"task", "todo", "create", "track", "plan"}},
		{Name: "task_list", Tags: []string{"task", "todo", "list", "all", "overview"}},
		{Name: "task_update", Tags: []string{"task", "todo", "update", "status"}},
		{Name: "calc", Tags: []string{"math", "calculate", "sum", "product"}},
		{Name: "json_query", Tags: []string{"json", "query", "filter", "extract"}},
		{Name: "monitor", Tags: []string{"watch", "tail", "follow", "stream"}},
		{Name: "cron_create", Tags: []string{"schedule", "cron", "recurring", "timer"}},
	}
}

func names(ds []tools.Descriptor) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}

func namesEq(got []tools.Descriptor, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].Name != want[i] {
			return false
		}
	}
	return true
}

// ---- select: form ----

func TestSelectByName_ExactMatch(t *testing.T) {
	got := selectByName("notebook_edit", testDescriptors(), 5)
	if len(got) != 1 || got[0].Name != "notebook_edit" {
		t.Fatalf("expected [notebook_edit], got %v", names(got))
	}
}

func TestSelectByName_CaseInsensitive(t *testing.T) {
	got := selectByName("NOTEBOOK_EDIT", testDescriptors(), 5)
	if len(got) != 1 || got[0].Name != "notebook_edit" {
		t.Fatalf("expected [notebook_edit], got %v", names(got))
	}
}

func TestSelectByName_MultipleAndUnknown(t *testing.T) {
	// multiple names, unknown silently dropped, preserves order
	got := selectByName("notebook_edit, nonexistent , calc", testDescriptors(), 10)
	want := []string{"notebook_edit", "calc"}
	if !namesEq(got, want) {
		t.Fatalf("expected %v, got %v", want, names(got))
	}
}

func TestSelectByName_RespectsMaxResults(t *testing.T) {
	got := selectByName("notebook_edit, web_search, web_fetch, task_create", testDescriptors(), 2)
	if len(got) != 2 {
		t.Fatalf("expected cap at 2, got %d: %v", len(got), names(got))
	}
	if got[0].Name != "notebook_edit" || got[1].Name != "web_search" {
		t.Fatalf("expected first 2 in order, got %v", names(got))
	}
}

func TestSelectByName_EmptyList(t *testing.T) {
	got := selectByName("", testDescriptors(), 5)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", names(got))
	}
}

// ---- keyword search (fuzzy tags + name + description) ----

func TestSearch_ExactTagMatch(t *testing.T) {
	got := search("notebook", 5, testDescriptors())
	if len(got) == 0 || got[0].Name != "notebook_edit" {
		t.Fatalf("expected notebook_edit top via exact tag, got %v", names(got))
	}
}

func TestSearch_TagSubstring(t *testing.T) {
	got := search("calcu", 5, testDescriptors())
	// "calcu" is substring of tag "calculate" on calc -> score 2.
	if len(got) == 0 || got[0].Name != "calc" {
		t.Fatalf("expected calc top via substring, got %v", names(got))
	}
}

func TestSearch_NameSubstring(t *testing.T) {
	got := search("task", 5, testDescriptors())
	if len(got) < 3 {
		t.Fatalf("expected >=3 task tools, got %v", names(got))
	}
	for _, d := range got {
		if !strings.HasPrefix(d.Name, "task_") {
			t.Fatalf("expected only task_* tools, got %s", d.Name)
		}
	}
}

func TestSearch_FuzzyTypo(t *testing.T) {
	got := search("noteboook", 5, testDescriptors())
	// "noteboook" (extra 'o') has levenshtein=1 from tag "notebook" -> +2.
	if len(got) == 0 || got[0].Name != "notebook_edit" {
		t.Fatalf("expected notebook_edit via typo, got %v", names(got))
	}
}

func TestSearch_FuzzySubsequence(t *testing.T) {
	got := search("jpyter", 5, testDescriptors())
	// "jpyter" is subsequence of tag "jupyter" -> +1.
	if len(got) == 0 || got[0].Name != "notebook_edit" {
		t.Fatalf("expected notebook_edit via subsequence, got %v", names(got))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	got := search("zzzznonexistent", 5, testDescriptors())
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", names(got))
	}
}

// ---- required (+term) filtering ----

func TestSearch_RequiredTermFilters(t *testing.T) {
	got := search("+web search", 5, testDescriptors())
	if len(got) == 0 {
		t.Fatal("expected at least web_search")
	}
	for _, d := range got {
		if !strings.Contains(d.Name, "web") {
			t.Fatalf("expected only web_* tools, got %s", d.Name)
		}
	}
}

func TestSearch_RequiredTermTypo(t *testing.T) {
	// "ntebook" missing 'o' has levenshtein=1 from tag "notebook".
	got := search("+ntebook", 5, testDescriptors())
	if len(got) == 0 || got[0].Name != "notebook_edit" {
		t.Fatalf("expected notebook_edit via fuzzy required, got %v", names(got))
	}
}

func TestSearch_OnlyRequiredTerms(t *testing.T) {
	got := search("+web +search", 5, testDescriptors())
	if len(got) == 0 || got[0].Name != "web_search" {
		t.Fatalf("expected web_search, got %v", names(got))
	}
}

func TestSearch_RequiredTermNoMatch(t *testing.T) {
	got := search("+nonexistent web", 5, testDescriptors())
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", names(got))
	}
}

// ---- max_results cap & ranking ----

func TestSearch_RespectsMaxResults(t *testing.T) {
	got := search("task", 2, testDescriptors())
	if len(got) != 2 {
		t.Fatalf("expected cap at 2, got %d: %v", len(got), names(got))
	}
}

func TestSearch_RankingByScore(t *testing.T) {
	descs := []tools.Descriptor{
		{Name: "a", Tags: []string{"exactmatch"}},
		{Name: "b", Tags: []string{"exactmatcx"}}, // typo
		{Name: "c", Tags: []string{"other"}},       // only name hit
	}
	got := search("exactmatch", 3, descs)
	if len(got) < 2 {
		t.Fatalf("expected >=2, got %v", names(got))
	}
	if got[0].Name != "a" {
		t.Fatalf("expected 'a' top (exact tag +4), got %s", got[0].Name)
	}
	if got[1].Name != "b" {
		t.Fatalf("expected 'b' second (typo +2), got %s", got[1].Name)
	}
}

// ---- ToolSearchTool.Execute integration ----

type fakeLookup struct {
	descs []tools.Descriptor
}

func (f *fakeLookup) DeferredNames() []tools.ToolName {
	out := make([]tools.ToolName, len(f.descs))
	for i, d := range f.descs {
		out[i] = tools.ToolName(d.Name)
	}
	return out
}

func (f *fakeLookup) Describe(name tools.ToolName) (tools.Descriptor, error) {
	for _, d := range f.descs {
		if string(name) == d.Name {
			return d, nil
		}
	}
	return tools.Descriptor{}, nil
}

func newToolSearchWith(descs []tools.Descriptor) *ToolSearchTool {
	return NewToolSearch(func() DeferredLookup {
		return &fakeLookup{descs: descs}
	})
}

func TestExecute_SelectReturnsFunctionsBlock(t *testing.T) {
	ts := newToolSearchWith(testDescriptors())
	input := json.RawMessage(`{"query":"select:calc,web_search"}`)
	res, err := ts.Execute(nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.HasPrefix(res.Content, "<functions>") || !strings.HasSuffix(res.Content, "</functions>") {
		t.Fatalf("expected <functions> wrapper, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "calc") || !strings.Contains(res.Content, "web_search") {
		t.Fatalf("expected calc and web_search, got: %s", res.Content)
	}
}

func TestExecute_KeywordReturnsTopMatch(t *testing.T) {
	ts := newToolSearchWith(testDescriptors())
	input := json.RawMessage(`{"query":"notebook jupyter"}`)
	res, err := ts.Execute(nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "notebook_edit") {
		t.Fatalf("expected notebook_edit, got: %s", res.Content)
	}
}

func TestExecute_EmptyQueryError(t *testing.T) {
	ts := newToolSearchWith(testDescriptors())
	input := json.RawMessage(`{"query":"   "}`)
	res, _ := ts.Execute(nil, input)
	if !res.IsError {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(res.Content, "query is required") {
		t.Fatalf("expected 'query is required', got: %s", res.Content)
	}
}

func TestExecute_NilLookupError(t *testing.T) {
	ts := NewToolSearch(nil)
	input := json.RawMessage(`{"query":"task"}`)
	res, _ := ts.Execute(nil, input)
	if !res.IsError {
		t.Fatal("expected error for nil lookup")
	}
}

func TestExecute_LookupReturnsNilError(t *testing.T) {
	ts := NewToolSearch(func() DeferredLookup { return nil })
	input := json.RawMessage(`{"query":"task"}`)
	res, _ := ts.Execute(nil, input)
	if !res.IsError {
		t.Fatal("expected error when lookup returns nil")
	}
}

func TestExecute_NoDeferredTools(t *testing.T) {
	ts := newToolSearchWith(nil)
	input := json.RawMessage(`{"query":"task"}`)
	res, _ := ts.Execute(nil, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "no deferred tools") {
		t.Fatalf("expected 'no deferred tools', got: %s", res.Content)
	}
}

func TestExecute_NoMatchMessage(t *testing.T) {
	ts := newToolSearchWith(testDescriptors())
	input := json.RawMessage(`{"query":"zzzznonexistent"}`)
	res, _ := ts.Execute(nil, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "no matches") {
		t.Fatalf("expected 'no matches' message, got: %s", res.Content)
	}
}
