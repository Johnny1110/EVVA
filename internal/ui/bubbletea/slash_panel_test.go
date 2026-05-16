package bubbletea

import (
	"testing"
)

func TestMatchSlashCommands(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string // expected command names in order
	}{
		{"empty", "", nil},
		{"plain text", "hello", nil},
		{"path-like input (no /)", "config", nil},
		{"single slash shows all", "/", []string{"/config", "/model", "/clear", "/exit", "/quit"}},
		{"prefix narrows", "/c", []string{"/config", "/clear"}},
		{"unique prefix", "/co", []string{"/config"}},
		{"exact match collapses", "/config", []string{"/config"}},
		{"case-insensitive", "/CL", []string{"/clear"}},
		{"no match", "/zzz", []string{}},
		{"leading whitespace trimmed", "  /m  ", []string{"/model"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchSlashCommands(c.input)
			if len(got) != len(c.want) {
				t.Fatalf("len: want %d (%v), got %d (%v)", len(c.want), c.want, len(got), got)
			}
			for i := range got {
				if got[i].name != c.want[i] {
					t.Errorf("index %d: want %s, got %s", i, c.want[i], got[i].name)
				}
			}
		})
	}
}
