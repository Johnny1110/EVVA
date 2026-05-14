package main

import (
	"encoding/json"
	"fmt"
	"github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/internal/constant"
	"github.com/johnny1110/evva/internal/llm"
	"github.com/johnny1110/evva/internal/toolset"
)

func main() {
	prof := agent.Main(constant.DEEPSEEK, constant.DEEPSEEK_V4_FLASH, nil)
	state := &toolset.ToolState{}
	built, err := toolset.Build(prof.ActiveTools, state)
	if err != nil { fmt.Println("build:", err); return }

	for _, t := range built {
		sch := llm.ToolSchema(t)
		var v any
		if err := json.Unmarshal(sch, &v); err != nil {
			fmt.Printf("INVALID %s: %v\nschema bytes:\n%s\n", t.Name(), err, string(sch))
		}
	}

	// Deferred tools — each on its own ToolState since some need stateful constructors
	for _, n := range prof.DeferredTools {
		st := &toolset.ToolState{}
		built, err := toolset.Build([]struct{}{}, st)
		_ = built
		_ = err
		_ = n
	}
}
