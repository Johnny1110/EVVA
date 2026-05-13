package tools

import "fmt"

// Group is a unit of registration: a list of tool names whose instances are
// produced together by a single build call. Stateless tools form one-name
// groups; tools that share backing state (e.g. the six task tools sharing one
// *Store) form a multi-name group whose build allocates that state once.
//
// build MUST return exactly len(ToolNames) tools, in the same order as ToolNames.
type Group struct {
	GroupName string
	ToolNames []ToolName
	Build     func() []Tool
}

var (
	groups   []Group
	memberOf = map[ToolName]groupRef{}
)

type groupRef struct {
	group  int
	member int
}

// RegisterGroup adds a group to the global registry. Panics on duplicate
// names — registration is wiring code, not runtime config; a clash is always
// a bug. Conventionally called from each tool package's init().
func RegisterGroup(g Group) {
	idx := len(groups)
	groups = append(groups, g)
	for i, n := range g.ToolNames {
		if _, dup := memberOf[n]; dup {
			panic(fmt.Sprintf("tools: duplicate registration for %q", n))
		}
		memberOf[n] = groupRef{group: idx, member: i}
	}
}

// Register is sugar for a single stateless tool. The group's build returns
// the same instance every call, so no state is allocated.
func Register(name ToolName, t Tool) {
	RegisterGroup(Group{
		ToolNames: []ToolName{name},
		Build:     func() []Tool { return []Tool{t} },
	})
}

// build resolves a list of tool names to fresh instances. Tools that belong
// to the same Group share the backing state allocated for this call; a
// separate call yields independent state, so each agent gets isolated tools.
//
// Returns an error if any name has no registered group.
func build(names []ToolName) ([]Tool, error) {
	if len(names) == 0 {
		return []Tool{}, nil
	}

	out := make([]Tool, 0, len(names))

	// cache id is toolRef.group, like Task Tool Group contain many tools, they are in same group.
	groupCache := map[int][]Tool{}

	for _, toolName := range names {
		toolRef, ok := memberOf[toolName]
		if !ok {
			return nil, fmt.Errorf("tools: no factory for %q", toolName)
		}
		instances, ok := groupCache[toolRef.group] // find other members tool in group
		if !ok {                                   // not in cache (stateful tool)
			instances = groups[toolRef.group].Build() // new tool instance (for stateful tools)
			if got, want := len(instances), len(groups[toolRef.group].ToolNames); got != want {
				return nil, fmt.Errorf("tools: group for %q returned %d tools, want %d", names, got, want)
			}
			// store group tools in cache
			groupCache[toolRef.group] = instances
		}
		out = append(out, instances[toolRef.member])
	}

	return out, nil
}
