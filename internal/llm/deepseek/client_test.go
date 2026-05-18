package deepseek

import (
	"testing"
)

func TestDeepseekEffort(t *testing.T) {
	tests := []struct {
		level        int
		wantThink    bool
		wantEffort   string
	}{
		{0, false, ""},
		{1, false, ""},        // low: no thinking
		{2, true, "high"},     // medium
		{3, true, "max"},      // high
		{4, true, "max"},      // ultra
		{5, false, ""},
	}
	for _, tt := range tests {
		think, effort := deepseekEffort(tt.level)
		if tt.wantThink && think == nil {
			t.Errorf("deepseekEffort(%d): expected thinking=enabled, got nil", tt.level)
		}
		if !tt.wantThink && think != nil {
			t.Errorf("deepseekEffort(%d): expected thinking=nil, got %+v", tt.level, think)
		}
		if effort != tt.wantEffort {
			t.Errorf("deepseekEffort(%d): effort = %q, want %q", tt.level, effort, tt.wantEffort)
		}
	}
}
