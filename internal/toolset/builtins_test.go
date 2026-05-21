package toolset

import (
	"encoding/json"
	"testing"

	pubtoolset "github.com/johnny1110/evva/pkg/toolset"
	"github.com/johnny1110/evva/pkg/tools"
)

// TestDefaultRegistry_PopulatedWithBuiltins ensures the init() side-effect
// in builtins.go ran — pkg/toolset.DefaultRegistry should have every
// bundled tool registered when internal/toolset is imported (which the
// agent does transitively).
func TestDefaultRegistry_PopulatedWithBuiltins(t *testing.T) {
	reg := pubtoolset.DefaultRegistry()
	for _, name := range []tools.ToolName{
		tools.READ_FILE, tools.BASH, tools.AGENT, tools.TOOL_SEARCH,
		tools.TODO_WRITE, tools.WEB_FETCH, tools.CALC,
	} {
		if !reg.Has(name) {
			t.Errorf("DefaultRegistry missing built-in tool %q", name)
		}
	}
}

// TestDefaultRegistry_BuildAllNamesProducesValidJSONSchemas exercises every
// registered factory against a real ToolState to catch regressions where
// a factory's type assertion or accessor wiring breaks.
func TestDefaultRegistry_BuildAllNamesProducesValidJSONSchemas(t *testing.T) {
	reg := pubtoolset.DefaultRegistry()
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
