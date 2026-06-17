package qwen

import "testing"

// qwenThinking maps evva's effort levels onto DashScope's enable_thinking /
// thinking_budget. effort 0 OMITS the flag (nil — never sends false, which a
// thinking-native model could reject); 1–4 enable thinking with a climbing
// budget; anything above 4 clamps to the top tier.
func TestQwenThinking(t *testing.T) {
	tests := []struct {
		level      int
		wantOmit   bool // enable is nil
		wantBudget int
	}{
		{0, true, 0},      // omit — model's native default decides
		{1, false, 8192},  // evva "low"
		{2, false, 16384}, // evva "medium" (default)
		{3, false, 32768}, // evva "high"
		{4, false, 81920}, // evva "ultra"
		{9, false, 81920}, // clamps to the max tier
	}
	for _, tt := range tests {
		enable, budget := qwenThinking(tt.level)
		switch {
		case tt.wantOmit && enable != nil:
			t.Errorf("qwenThinking(%d): expected nil enable (omit), got %v", tt.level, *enable)
		case !tt.wantOmit && enable == nil:
			t.Errorf("qwenThinking(%d): expected enable_thinking=true, got nil", tt.level)
		case !tt.wantOmit && !*enable:
			t.Errorf("qwenThinking(%d): enable = false, want true (never send false)", tt.level)
		}
		if budget != tt.wantBudget {
			t.Errorf("qwenThinking(%d): budget = %d, want %d", tt.level, budget, tt.wantBudget)
		}
	}
}
