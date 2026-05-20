package toolset

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/tools"
)

// TestNewRegistry_StartsEmpty guards against accidentally pre-populating
// fresh registries — external hosts that want their own catalog need a
// clean slate.
func TestNewRegistry_StartsEmpty(t *testing.T) {
	r := NewRegistry()
	if names := r.Names(); len(names) != 0 {
		t.Errorf("expected empty registry, got %d names: %v", len(names), names)
	}
}

func TestRegistry_RegisterAndBuild(t *testing.T) {
	r := NewRegistry()
	name := tools.ToolName("phase2_test_stub")

	want := tools.NewStub(name, "test desc", `{}`)
	err := r.Register(name, func(s *ToolState) (tools.Tool, error) { return want, nil })
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Build(name, NewToolState())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Name() != string(name) {
		t.Errorf("Build returned wrong tool: got name %q, want %q", got.Name(), name)
	}
}

func TestRegistry_RejectsDuplicateRegistration(t *testing.T) {
	r := NewRegistry()
	name := tools.ToolName("phase2_dup_stub")
	f := func(s *ToolState) (tools.Tool, error) { return tools.NewStub(name, "", `{}`), nil }

	if err := r.Register(name, f); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	err := r.Register(name, f)
	if err == nil {
		t.Fatal("expected duplicate Register to fail")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention 'duplicate', got: %v", err)
	}
}

func TestRegistry_RejectsNilFactory(t *testing.T) {
	r := NewRegistry()
	err := r.Register(tools.ToolName("phase2_nilfac"), nil)
	if err == nil {
		t.Fatal("expected nil factory to be rejected")
	}
}

func TestRegistry_RejectsEmptyName(t *testing.T) {
	r := NewRegistry()
	err := r.Register("", func(s *ToolState) (tools.Tool, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected empty name to be rejected")
	}
}

func TestRegistry_BuildUnknownNameReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.Build(tools.ToolName("phase2_never_registered"), NewToolState())
	if err == nil {
		t.Fatal("expected Build of unknown name to fail")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error should mention 'unknown tool', got: %v", err)
	}
}

func TestRegistry_Has(t *testing.T) {
	r := NewRegistry()
	name := tools.ToolName("phase2_has_stub")
	if r.Has(name) {
		t.Errorf("Has should return false for unregistered name")
	}
	_ = r.Register(name, func(s *ToolState) (tools.Tool, error) { return tools.NewStub(name, "", `{}`), nil })
	if !r.Has(name) {
		t.Errorf("Has should return true after Register")
	}
}

// TestDefaultRegistry_PopulatedWithBuiltins ensures registerBuiltins ran on
// first DefaultRegistry() access. We check a few representative names from
// different families; the exhaustive enumeration lives in
// TestAllToolSchemasAreValidJSON.
func TestDefaultRegistry_PopulatedWithBuiltins(t *testing.T) {
	reg := DefaultRegistry()
	for _, name := range []tools.ToolName{
		tools.READ_FILE, tools.BASH, tools.AGENT, tools.TOOL_SEARCH,
		tools.TODO_WRITE, tools.WEB_FETCH, tools.CALC,
	} {
		if !reg.Has(name) {
			t.Errorf("DefaultRegistry missing built-in tool %q", name)
		}
	}
}

// TestDefaultRegistry_BuildAllNamesProducesValidJSONSchemas mirrors the
// existing TestAllToolSchemasAreValidJSON but goes through the Registry
// path explicitly so a regression that breaks the registry without
// touching Build() would still surface here.
func TestDefaultRegistry_BuildAllNamesProducesValidJSONSchemas(t *testing.T) {
	reg := DefaultRegistry()
	state := NewToolState()
	for _, name := range reg.Names() {
		got, err := reg.Build(name, state)
		if err != nil {
			t.Errorf("%s: Build failed: %v", name, err)
			continue
		}
		schema := got.Schema()
		if len(schema) == 0 {
			continue // llm.ToolSchema substitutes a permissive default
		}
		var v any
		if err := json.Unmarshal(schema, &v); err != nil {
			t.Errorf("%s: schema is invalid JSON: %v", name, err)
		}
	}
}
