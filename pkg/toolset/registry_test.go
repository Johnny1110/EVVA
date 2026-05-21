package toolset

import (
	"strings"
	"testing"

	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/tools"
)

// stubState is a minimal tools.State implementation for exercising the
// registry mechanics. It returns nil Config and "" Workdir — enough for
// stateless factories (the registry only cares that something satisfying
// the interface is passed through).
type stubState struct{}

func (stubState) Config() *config.Config { return nil }
func (stubState) Workdir() string        { return "" }

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
	err := r.Register(name, func(tools.State) (tools.Tool, error) { return want, nil })
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Build(name, stubState{})
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
	f := func(tools.State) (tools.Tool, error) { return tools.NewStub(name, "", `{}`), nil }

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
	err := r.Register("", func(tools.State) (tools.Tool, error) { return nil, nil })
	if err == nil {
		t.Fatal("expected empty name to be rejected")
	}
}

func TestRegistry_BuildUnknownNameReturnsError(t *testing.T) {
	r := NewRegistry()
	_, err := r.Build(tools.ToolName("phase2_never_registered"), stubState{})
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
	_ = r.Register(name, func(tools.State) (tools.Tool, error) { return tools.NewStub(name, "", `{}`), nil })
	if !r.Has(name) {
		t.Errorf("Has should return true after Register")
	}
}

func TestDefaultRegistry_SameInstance(t *testing.T) {
	a := DefaultRegistry()
	b := DefaultRegistry()
	if a != b {
		t.Errorf("DefaultRegistry should return the same pointer across calls")
	}
}
