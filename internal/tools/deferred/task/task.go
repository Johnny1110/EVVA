package task

import "github.com/johnny1110/evva/internal/tools"

// taskNames is the canonical order for the task subsystem. The Group's build
// must return instances in the same order — build resolves a tool by its
// member index, so order is load-bearing.
var taskNames = []tools.ToolName{
	tools.TASK_CREATE,
	tools.TASK_GET,
	tools.TASK_LIST,
	tools.TASK_UPDATE,
	tools.TASK_OUTPUT,
	tools.TASK_STOP,
}

func init() {
	tools.RegisterGroup(tools.Group{
		ToolNames: taskNames,
		Build:     buildTaskTools,
	})
}

// Names lists every tool name this package contributes.
func Names() []tools.ToolName { return taskNames }

// buildTaskTools allocates a fresh Store and returns the six task tools all
// bound to it. Called once per tools.build, so each agent gets isolated state.
func buildTaskTools() []tools.Tool {
	s := NewStore()
	return []tools.Tool{
		NewCreate(s),
		NewGet(s),
		NewList(s),
		NewUpdate(s),
		NewOutput(s),
		NewStop(s),
	}
}
