package agent

import (
	"slices"
	"testing"

	"github.com/johnny1110/evva/internal/memdir"
	config "github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/tools"
)

// TestWorktreeList_MainOnlyGating pins A8: worktree_list is advertised on the
// Main profile (deferred) but absent from the built-in subagent profiles —
// reconcile is a root-agent concern, and subagents can't spawn or merge.
func TestWorktreeList_MainOnlyGating(t *testing.T) {
	cfg := config.Get()

	main := mainProfile(cfg, cfg.DefaultProvider, cfg.DefaultModel, nil, memdir.Snapshot{}, nil, nil, "")
	if !slices.Contains(main.DeferredTools, tools.WORKTREE_LIST) {
		t.Errorf("Main profile should advertise worktree_list in DeferredTools; got %v", main.DeferredTools)
	}

	subagents := map[string]Profile{
		"explore": Explore(cfg, cfg.DefaultProvider, cfg.DefaultModel, nil),
		"plan":    Plan(cfg, cfg.DefaultProvider, cfg.DefaultModel, nil),
		"general": General(cfg, cfg.DefaultProvider, cfg.DefaultModel, nil, tools.READ_FILE, tools.BASH),
	}
	for name, p := range subagents {
		all := slices.Concat(p.ActiveTools, p.DeferredTools)
		if slices.Contains(all, tools.WORKTREE_LIST) {
			t.Errorf("subagent profile %q must not contain worktree_list; got %v", name, all)
		}
	}
}
