package permission

import "testing"

func TestTransition_NoOpOnSameMode(t *testing.T) {
	s := NewPlanModeState()
	s.Transition(ModeDefault, ModeDefault)
	if s.PrePlanMode() != "" {
		t.Errorf("no-op transition should not stash prePlanMode")
	}
	if s.ConsumePendingExitReminder() {
		t.Errorf("no-op transition should not queue an exit reminder")
	}
}

func TestTransition_EnterPlanMode_StashesPriorMode(t *testing.T) {
	s := NewPlanModeState()
	s.Transition(ModeAcceptEdits, ModePlan)
	if got := s.PrePlanMode(); got != ModeAcceptEdits {
		t.Errorf("PrePlanMode after enter: want %q, got %q", ModeAcceptEdits, got)
	}
	if s.AttachmentsSinceExit() != 0 {
		t.Errorf("attachments counter should reset on enter")
	}
	if s.TurnsSinceLastAttachment() != 0 {
		t.Errorf("throttle counter should reset on enter")
	}
}

func TestTransition_ExitPlanMode_QueuesExitReminderAndClearsStash(t *testing.T) {
	s := NewPlanModeState()
	s.Transition(ModeDefault, ModePlan)
	if s.PrePlanMode() != ModeDefault {
		t.Fatalf("setup: PrePlanMode after enter should be ModeDefault")
	}

	s.Transition(ModePlan, ModeDefault)
	if s.PrePlanMode() != "" {
		t.Errorf("PrePlanMode should clear on exit")
	}
	if !s.ConsumePendingExitReminder() {
		t.Errorf("exit transition should queue an exit reminder")
	}
	// One-shot: a second consume returns false.
	if s.ConsumePendingExitReminder() {
		t.Errorf("exit reminder should fire only once")
	}
}

func TestTransition_ExitMarksHasExited_DrivesReentry(t *testing.T) {
	s := NewPlanModeState()
	s.Transition(ModeDefault, ModePlan)
	s.Transition(ModePlan, ModeDefault)
	// HasExited stays true until ConsumeReentry fires.
	if !s.HasExited() {
		t.Errorf("HasExited should be true after exit")
	}
	if !s.ConsumeReentry() {
		t.Errorf("ConsumeReentry should return true the first time after exit")
	}
	if s.ConsumeReentry() {
		t.Errorf("ConsumeReentry should be a one-shot")
	}
}

func TestTransition_NonPlanTransitions_AreNoOps(t *testing.T) {
	s := NewPlanModeState()
	s.Transition(ModeDefault, ModeAcceptEdits)
	s.Transition(ModeAcceptEdits, ModeBypass)
	if s.PrePlanMode() != "" {
		t.Errorf("non-plan transitions should not stash prePlanMode")
	}
	if s.ConsumePendingExitReminder() {
		t.Errorf("non-plan transitions should not queue an exit reminder")
	}
	if s.HasExited() {
		t.Errorf("non-plan transitions should not mark hasExited")
	}
}

func TestTransition_AttachmentCounters(t *testing.T) {
	s := NewPlanModeState()

	// Pre-emission counts are zero.
	if s.AttachmentsSinceExit() != 0 || s.TurnsSinceLastAttachment() != 0 {
		t.Fatalf("zero state expected")
	}

	// Recording an emission bumps the cycle, resets the throttle.
	s.RecordAttachmentEmitted()
	if s.AttachmentsSinceExit() != 1 {
		t.Errorf("attachments cycle should increment on emission")
	}
	if s.TurnsSinceLastAttachment() != 0 {
		t.Errorf("throttle should reset on emission")
	}

	// Recording a turn without emission bumps the throttle, not the cycle.
	s.RecordTurnWithoutAttachment()
	s.RecordTurnWithoutAttachment()
	if s.TurnsSinceLastAttachment() != 2 {
		t.Errorf("throttle should increment per silent turn, got %d", s.TurnsSinceLastAttachment())
	}
	if s.AttachmentsSinceExit() != 1 {
		t.Errorf("silent turns should not bump the cycle counter")
	}
}

func TestPlanModeState_NilSafe(t *testing.T) {
	// Every accessor must tolerate nil — agents constructed without an
	// explicit PlanModeState (legacy tests) must keep compiling.
	var s *PlanModeState
	s.Transition(ModeDefault, ModePlan)
	_ = s.PrePlanMode()
	_ = s.HasExited()
	_ = s.ConsumeReentry()
	_ = s.ConsumePendingExitReminder()
	_ = s.AttachmentsSinceExit()
	_ = s.TurnsSinceLastAttachment()
	s.SetPrePlanMode(ModeDefault)
	s.RecordAttachmentEmitted()
	s.RecordTurnWithoutAttachment()
}
