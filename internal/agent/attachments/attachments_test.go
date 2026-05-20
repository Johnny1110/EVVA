package attachments

import (
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/permission"
)

func newState() *permission.PlanModeState { return permission.NewPlanModeState() }

func input(state *permission.PlanModeState, mode permission.Mode) Input {
	return Input{
		State:        state,
		Mode:         mode,
		PlanFilePath: "/tmp/.evva/plans/current.md",
		PlanExists:   false,
	}
}

func TestCompute_NotInPlanMode_NoOp(t *testing.T) {
	got := ComputePlanMode(input(newState(), permission.ModeDefault))
	if len(got) != 0 {
		t.Errorf("non-plan mode should produce no attachments, got %v", got)
	}
}

func TestCompute_FirstTurnInPlanMode_EmitsFullReminder(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)

	got := ComputePlanMode(input(state, permission.ModePlan))
	if len(got) != 1 {
		t.Fatalf("first turn should emit one reminder, got %d", len(got))
	}
	if !strings.Contains(got[0], "<system-reminder>") {
		t.Errorf("reminder should be wrapped in <system-reminder>")
	}
	if !strings.Contains(got[0], "Iterative Planning Workflow") {
		t.Errorf("first turn should emit FULL reminder; got:\n%s", got[0])
	}
	if state.AttachmentsSinceExit() != 1 {
		t.Errorf("cycle counter should bump to 1 after first emission")
	}
}

func TestCompute_ThrottlesSubsequentTurns(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)
	_ = ComputePlanMode(input(state, permission.ModePlan)) // first turn — emits

	// Next 3 user turns are silent (throttled by TurnsBetweenAttachments=4).
	for i := 0; i < TurnsBetweenAttachments-1; i++ {
		got := ComputePlanMode(input(state, permission.ModePlan))
		if len(got) != 0 {
			t.Errorf("turn %d should be throttled, got %v", i+2, got)
		}
	}
	// 5th user turn (4 silent turns elapsed) fires the next reminder.
	got := ComputePlanMode(input(state, permission.ModePlan))
	if len(got) != 1 {
		t.Errorf("post-throttle turn should emit a reminder, got %v", got)
	}
}

func TestCompute_FullVsSparseCycle(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)

	// Drive 5 emissions; assert which ones are FULL.
	emissions := []bool{}
	for i := 0; i < 5; i++ {
		// Burn the throttle window between consecutive emissions: after
		// each emit there are TurnsBetweenAttachments-1 silent turns,
		// and the next call emits. First emission (i==0) has no
		// pre-throttle.
		if i > 0 {
			for j := 0; j < TurnsBetweenAttachments-1; j++ {
				ComputePlanMode(input(state, permission.ModePlan))
			}
		}
		got := ComputePlanMode(input(state, permission.ModePlan))
		if len(got) == 0 {
			t.Fatalf("emission %d: expected reminder", i+1)
		}
		// First-turn reminder uses interview workflow (FULL has the
		// Iterative Planning Workflow header). Sparse is a one-liner.
		emissions = append(emissions, strings.Contains(got[len(got)-1], "Iterative Planning Workflow"))
	}
	// emission #1 must be FULL (first turn); #2..#4 sparse; #5 sparse (count % 5 == 0);
	// #6 would be full (count % 5 == 1).
	want := []bool{true, false, false, false, false}
	for i, isFull := range emissions {
		if isFull != want[i] {
			t.Errorf("emission %d: full=%v, want %v", i+1, isFull, want[i])
		}
	}
}

func TestCompute_ExitReminder_FiresOnceAfterTransition(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)
	state.Transition(permission.ModePlan, permission.ModeDefault)

	// Next user-prompt turn (now in default mode) should carry the exit reminder.
	got := ComputePlanMode(input(state, permission.ModeDefault))
	if len(got) != 1 {
		t.Fatalf("post-exit turn should emit one reminder, got %d", len(got))
	}
	if !strings.Contains(got[0], "exited plan mode") {
		t.Errorf("exit reminder should say 'exited plan mode'; got:\n%s", got[0])
	}

	// Second user-prompt turn produces nothing — one-shot.
	got = ComputePlanMode(input(state, permission.ModeDefault))
	if len(got) != 0 {
		t.Errorf("second post-exit turn should be silent, got %v", got)
	}
}

func TestCompute_ReentryReminder_FiresOnceWhenComingBack(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)
	state.Transition(permission.ModePlan, permission.ModeDefault)
	// Discard the exit reminder so the next turn is clean.
	ComputePlanMode(input(state, permission.ModeDefault))

	// Re-enter plan mode.
	state.Transition(permission.ModeDefault, permission.ModePlan)
	got := ComputePlanMode(input(state, permission.ModePlan))
	if len(got) != 2 {
		t.Fatalf("re-entry turn should emit re-entry reminder + full reminder, got %d", len(got))
	}
	if !strings.Contains(got[0], "re-entered plan mode") {
		t.Errorf("first attachment should be the re-entry reminder; got:\n%s", got[0])
	}
}

func TestCompute_V2WorkflowVariant_UsesPhaseLayout(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)

	in := input(state, permission.ModePlan)
	in.WorkflowVariant = WorkflowV2
	got := ComputePlanMode(in)
	if len(got) != 1 || !strings.Contains(got[0], "### Phase 1: Initial Understanding") {
		t.Errorf("v2 variant should include Phase 1 header; got:\n%s", got[0])
	}
}

func TestCompute_PlanExists_BranchesPlanFileInfo(t *testing.T) {
	state := newState()
	state.Transition(permission.ModeDefault, permission.ModePlan)
	in := input(state, permission.ModePlan)
	in.PlanExists = true
	got := ComputePlanMode(in)
	if len(got) == 0 {
		t.Fatal("expected a reminder")
	}
	if !strings.Contains(got[0], "A plan file already exists at") {
		t.Errorf("planExists=true branch should mention existing file; got:\n%s", got[0])
	}
}
