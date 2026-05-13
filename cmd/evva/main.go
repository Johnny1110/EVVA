package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/internal/agent/event"
	"github.com/johnny1110/evva/internal/agent/profiles"
	"github.com/johnny1110/evva/internal/constant"
	"github.com/johnny1110/evva/internal/llm"
	"github.com/joho/godotenv"
)

// CLI driver for the agent loop.
//
// Usage:
//
//	evva [-temp 0.7] [-max-tokens 1024] [-max-iters 25] <prompt>
//
// If no positional prompt is given, the program reads from stdin (pipe / heredoc).
// Ctrl+C / SIGTERM cancels the in-flight loop and exits with code 130.
// When the iteration cap is hit the user is prompted to press Enter to continue.
func main() {
	_ = godotenv.Load()

	temp := flag.Float64("temp", -1, "sampling temperature (-1 → leave unset)")
	maxTokens := flag.Int("max-tokens", 1024, "max output tokens (0 → provider default)")
	maxIters := flag.Int("max-iters", agent.DefaultMaxIterations, "max loop iterations before pausing for Continue")
	flag.Parse()

	prompt, err := readPrompt(flag.Args())
	if err != nil {
		exitf(2, "evva: %v", err)
	}
	if prompt == "" {
		exitf(2, "usage: evva [-temp 0.7] [-max-tokens N] [-max-iters N] <prompt>")
	}

	prof := profiles.Main(constant.DEEPSEEK, constant.DEEPSEEK_V4_FLASH, buildOptions(*temp, *maxTokens))
	prof.SystemPrompt = "You are a helpful coding agent operating in a terminal. Use tools when they help."

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ag, err := agent.New(prof,
		agent.WithSink(cliSink{out: os.Stdout}),
		agent.WithMaxIterations(*maxIters),
	)
	if err != nil {
		exitf(1, "evva: %v", err)
	}

	resp, err := ag.Run(ctx, prompt)
	for errors.Is(err, agent.ErrIterLimit) {
		fmt.Fprint(os.Stderr, "\n[paused at iter limit — press Enter to continue, Ctrl+C to stop] ")
		if !waitEnter(ctx) {
			err = ctx.Err()
			break
		}
		resp, err = ag.Continue(ctx)
	}
	if err != nil {
		if errors.Is(err, llm.ErrInterrupted) {
			fmt.Fprintln(os.Stderr, "\ninterrupted")
			os.Exit(130)
		}
		exitf(1, "evva: %v", err)
	}

	// Final answer was already emitted as a Text event by the loop; no need
	// to repeat it. resp is here in case scripts want to read structured
	// state (provider, model, etc.) in the future.
	_ = resp
}

// --- CLI event sink -------------------------------------------------------

// cliSink writes a human-readable trace of the agent's events to out. It is
// the reference Sink implementation — a fine starting point for richer UIs
// (Bubble Tea TUI, web UI) which can replace it without changing the agent.
type cliSink struct {
	out io.Writer
}

func (s cliSink) Emit(e event.Event) {
	switch e.Kind {
	case event.KindRunStart:
		// stay quiet — the user already typed the prompt
	case event.KindRunResume:
		fmt.Fprintln(s.out, "[resume]")
	case event.KindThinking:
		if e.Thinking != nil {
			fmt.Fprintf(s.out, "\n--- thinking ---\n%s\n", e.Thinking.Text)
		}
	case event.KindText:
		if e.Text != nil && e.Text.Text != "" {
			fmt.Fprintf(s.out, "\n%s\n", e.Text.Text)
		}
	case event.KindToolUseStart:
		if e.ToolUseStart != nil {
			fmt.Fprintf(s.out, "\n→ %s %s\n", e.ToolUseStart.Name, compactJSON(e.ToolUseStart.Input))
		}
	case event.KindToolUseResult:
		if e.ToolUseResult == nil {
			return
		}
		prefix := "✓"
		if e.ToolUseResult.IsError {
			prefix = "✗"
		}
		body := truncate(e.ToolUseResult.Content, 600)
		fmt.Fprintf(s.out, "%s %s\n", prefix, body)
	case event.KindError:
		if e.Error != nil {
			fmt.Fprintf(s.out, "\n[error:%s] %v\n", e.Error.Stage, e.Error.Err)
		}
	case event.KindIterLimit:
		if e.IterLimit != nil {
			fmt.Fprintf(s.out, "\n[iter-limit] reached %d iterations\n", e.IterLimit.Reached)
		}
	case event.KindRunCancelled:
		fmt.Fprintln(s.out, "\n[cancelled]")
	case event.KindRunEnd:
		// final text already printed via KindText
	case event.KindTaskUpdate:
		if e.TaskUpdate != nil {
			fmt.Fprintf(s.out, "[task] %s [%s] %s\n", e.TaskUpdate.TaskID, e.TaskUpdate.Status, e.TaskUpdate.Subject)
		}
	case event.KindSubagent:
		if e.Subagent != nil {
			fmt.Fprintf(s.out, "[subagent:%s] %s (%s)\n", e.Subagent.Phase, e.Subagent.SubagentID, e.Subagent.AgentType)
		}
	case event.KindTurnStart, event.KindTurnEnd:
		// quiet — too chatty for the CLI; the structured log captures these
	}
}

func compactJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	out := truncate(string(raw), 200)
	// Squash runs of whitespace so multi-line tool inputs read as one line.
	out = strings.Join(strings.Fields(out), " ")
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// --- input plumbing -------------------------------------------------------

func readPrompt(args []string) (string, error) {
	if joined := strings.TrimSpace(strings.Join(args, " ")); joined != "" {
		return joined, nil
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	raw, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// waitEnter blocks until the user presses Enter on stdin, or ctx is cancelled.
// Returns true if Enter was pressed, false if cancelled.
func waitEnter(ctx context.Context) bool {
	ch := make(chan struct{}, 1)
	go func() {
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		ch <- struct{}{}
	}()
	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func buildOptions(temp float64, maxTokens int) []llm.Option {
	var out []llm.Option
	if temp >= 0 {
		out = append(out, llm.WithTemperature(temp))
	}
	if maxTokens > 0 {
		out = append(out, llm.WithMaxTokens(maxTokens))
	}
	return out
}

func exitf(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}
