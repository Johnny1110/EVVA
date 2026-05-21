package todos

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/johnny1110/evva/pkg/tools/todo"
	"github.com/johnny1110/evva/internal/toolset"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/theme"
)

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// newTSWithTodos builds a fresh ToolState and seeds the store via Replace.
func newTSWithTodos(_ *testing.T, items []todo.Todo) *toolset.ToolState {
	ts := toolset.NewToolState()
	ts.TodoStore().Replace(items)
	return ts
}

func TestRenderEmpty(t *testing.T) {
	ts := toolset.NewToolState()
	if got := Render(ts, 80, theme.Default()); got != "" {
		t.Errorf("empty store should render empty, got %q", got)
	}
}

func TestRenderNilToolState(t *testing.T) {
	if got := Render(nil, 80, theme.Default()); got != "" {
		t.Errorf("nil ToolState should render empty, got %q", got)
	}
}

func TestRenderShowsContent(t *testing.T) {
	ts := newTSWithTodos(t, []todo.Todo{
		{Content: "still here", ActiveForm: "still being here", Status: todo.StatusPending},
	})
	out := Render(ts, 80, theme.Default())
	if !strings.Contains(out, "still here") {
		t.Errorf("missing todo content: %q", out)
	}
}

func TestRenderInProgressUsesActiveForm(t *testing.T) {
	ts := newTSWithTodos(t, []todo.Todo{
		{Content: "Run tests", ActiveForm: "Running tests", Status: todo.StatusInProgress},
	})
	out := Render(ts, 80, theme.Default())
	if !strings.Contains(out, "Running tests") {
		t.Errorf("in_progress row should show ActiveForm: %q", out)
	}
	if strings.Contains(out, "Run tests") {
		t.Errorf("in_progress row should hide content (imperative form): %q", out)
	}
}

func TestRenderIncludesHeader(t *testing.T) {
	ts := newTSWithTodos(t, []todo.Todo{{Content: "x", ActiveForm: "x", Status: todo.StatusInProgress}})
	out := Render(ts, 80, theme.Default())
	if !strings.Contains(out, "TODOS") {
		t.Errorf("header missing TODOS label: %q", out)
	}
}

func TestRenderTruncatesLongContent(t *testing.T) {
	long := strings.Repeat("x", 200)
	ts := newTSWithTodos(t, []todo.Todo{{Content: long, ActiveForm: long, Status: todo.StatusPending}})
	out := Render(ts, 40, theme.Default())
	if !strings.Contains(out, "…") {
		t.Errorf("expected ellipsis on truncated long content: %q", out)
	}
}

func TestAllCompleted(t *testing.T) {
	cases := []struct {
		name  string
		todos []todo.Todo
		want  bool
	}{
		{"empty", nil, false},
		{"one pending", []todo.Todo{{Content: "x", ActiveForm: "x", Status: todo.StatusPending}}, false},
		{"all completed", []todo.Todo{
			{Content: "x", ActiveForm: "x", Status: todo.StatusCompleted},
			{Content: "y", ActiveForm: "y", Status: todo.StatusCompleted},
		}, true},
		{"one in progress", []todo.Todo{
			{Content: "x", ActiveForm: "x", Status: todo.StatusCompleted},
			{Content: "y", ActiveForm: "y", Status: todo.StatusInProgress},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTSWithTodos(t, tc.todos)
			if got := AllCompleted(ts); got != tc.want {
				t.Errorf("AllCompleted = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRenderCompleteSnapshot(t *testing.T) {
	ts := newTSWithTodos(t, []todo.Todo{
		{Content: "ship the feature", ActiveForm: "shipping the feature", Status: todo.StatusCompleted},
		{Content: "write docs", ActiveForm: "writing docs", Status: todo.StatusCompleted},
	})
	out := RenderCompleteSnapshot(ts, 80, theme.Default())
	if !strings.Contains(out, "TODOS COMPLETE") {
		t.Errorf("snapshot header missing: %q", out)
	}
	if !strings.Contains(out, "ship the feature") || !strings.Contains(out, "write docs") {
		t.Errorf("snapshot missing todos: %q", out)
	}
}
