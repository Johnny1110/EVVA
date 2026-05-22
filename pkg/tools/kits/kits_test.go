package kits

import (
	"testing"

	"github.com/johnny1110/evva/pkg/tools"
)

// TestGeneralPurposeKit_HasCoreMembers locks down what consumers expect
// to find in the default kit. Drift would silently change a downstream
// agent's capabilities, so freeze the contract.
func TestGeneralPurposeKit_HasCoreMembers(t *testing.T) {
	active, deferred := GeneralPurposeKit()
	mustHave(t, active, "active",
		tools.READ_FILE, tools.WRITE_FILE, tools.EDIT_FILE, tools.GLOB,
		tools.BASH, tools.GREP, tools.TREE,
		tools.TODO_WRITE, tools.JSON_QUERY, tools.CALC,
		tools.TOOL_SEARCH,
	)
	mustHave(t, deferred, "deferred", tools.WEB_SEARCH, tools.WEB_FETCH)
	mustNotHave(t, active, "active", tools.WEB_SEARCH, tools.WEB_FETCH)
}

func TestReadOnlyKit_NoMutating(t *testing.T) {
	got := ReadOnlyKit()
	mustHave(t, got, "ReadOnlyKit",
		tools.READ_FILE, tools.GREP, tools.GLOB, tools.TREE,
		tools.WEB_SEARCH, tools.WEB_FETCH, tools.JSON_QUERY,
	)
	mustNotHave(t, got, "ReadOnlyKit",
		tools.BASH, tools.WRITE_FILE, tools.EDIT_FILE, tools.TODO_WRITE,
	)
}

func TestCodingKit_ExtendsGeneralPurpose(t *testing.T) {
	active, deferred := CodingKit()
	mustHave(t, active, "CodingKit active",
		tools.READ_FILE, tools.BASH, tools.NOTEBOOK_EDIT, tools.MONITOR,
	)
	mustHave(t, deferred, "CodingKit deferred", tools.WEB_SEARCH)
}

func TestResearchKit_NoFilesystemMutation(t *testing.T) {
	got := ResearchKit()
	mustHave(t, got, "ResearchKit",
		tools.READ_FILE, tools.GREP, tools.GLOB,
		tools.WEB_SEARCH, tools.WEB_FETCH,
		tools.JSON_QUERY, tools.CALC, tools.TODO_WRITE,
	)
	mustNotHave(t, got, "ResearchKit",
		tools.BASH, tools.WRITE_FILE, tools.EDIT_FILE,
	)
}

// TestKits_ReturnFreshSlices verifies kits don't share backing arrays
// (so mutation by one caller can't poison another).
func TestKits_ReturnFreshSlices(t *testing.T) {
	a1, _ := GeneralPurposeKit()
	a2, _ := GeneralPurposeKit()
	if &a1[0] == &a2[0] {
		t.Error("GeneralPurposeKit must return fresh active slices, not a shared singleton")
	}
}

func mustHave(t *testing.T, got []tools.ToolName, label string, want ...tools.ToolName) {
	t.Helper()
	set := make(map[tools.ToolName]struct{}, len(got))
	for _, n := range got {
		set[n] = struct{}{}
	}
	for _, n := range want {
		if _, ok := set[n]; !ok {
			t.Errorf("%s: missing %q", label, n)
		}
	}
}

func mustNotHave(t *testing.T, got []tools.ToolName, label string, banned ...tools.ToolName) {
	t.Helper()
	set := make(map[tools.ToolName]struct{}, len(got))
	for _, n := range got {
		set[n] = struct{}{}
	}
	for _, n := range banned {
		if _, ok := set[n]; ok {
			t.Errorf("%s: should not contain %q", label, n)
		}
	}
}
