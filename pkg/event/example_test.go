package event_test

import (
	"fmt"

	"github.com/johnny1110/evva/pkg/event"
)

// ExampleSinkFunc shows the function-shaped sink pattern. The smallest
// possible implementation of event.Sink — wrap any func(Event) and pass
// it to agent.WithSink.
func ExampleSinkFunc() {
	sink := event.SinkFunc(func(e event.Event) {
		if e.Kind == event.KindText && e.Text != nil {
			fmt.Println("assistant:", e.Text.Text)
		}
	})
	sink.Emit(event.Event{Kind: event.KindText, Text: &event.TextPayload{Text: "hi"}})
	// Output: assistant: hi
}

// ExampleEvent_Payload demonstrates the Phase 19a Payload() helper —
// type-switch on the returned pointer instead of remembering which
// Event.* field goes with each Kind.
func ExampleEvent_Payload() {
	events := []event.Event{
		{Kind: event.KindText, Text: &event.TextPayload{Text: "done"}},
		{Kind: event.KindToolUseStart, ToolUseStart: &event.ToolUseStartPayload{Name: "read"}},
		{Kind: event.KindRunEnd, RunEnd: &event.RunEndPayload{Iters: 3}},
	}
	for _, e := range events {
		switch p := e.Payload().(type) {
		case *event.TextPayload:
			fmt.Println("text:", p.Text)
		case *event.ToolUseStartPayload:
			fmt.Println("tool:", p.Name)
		case *event.RunEndPayload:
			fmt.Println("done in", p.Iters, "iters")
		}
	}
	// Output:
	// text: done
	// tool: read
	// done in 3 iters
}

// ExampleMulti shows how to fan one event to several sinks — useful
// for wiring a TUI plus a structured log at the same time.
func ExampleMulti() {
	var tuiTexts, logTexts []string
	tui := event.SinkFunc(func(e event.Event) {
		if e.Text != nil {
			tuiTexts = append(tuiTexts, e.Text.Text)
		}
	})
	logger := event.SinkFunc(func(e event.Event) {
		if e.Text != nil {
			logTexts = append(logTexts, e.Text.Text)
		}
	})

	fanout := event.Multi{Sinks: []event.Sink{tui, logger}}
	fanout.Emit(event.Event{Kind: event.KindText, Text: &event.TextPayload{Text: "broadcast"}})

	fmt.Println("tui:", tuiTexts)
	fmt.Println("log:", logTexts)
	// Output:
	// tui: [broadcast]
	// log: [broadcast]
}
