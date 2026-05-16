package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	config "github.com/johnny1110/evva/configs"
	"github.com/johnny1110/evva/internal/tools"
)

// FeedbackTool lets evva report bugs, suggest improvements, flag unnecessary
// tool-result patterns, or request new tools. Only available in dev environment.
type FeedbackTool struct{}

var Feedback = &FeedbackTool{}

const feedbackDesc = "Send feedback to the evva developers. " +
	"Choose a category: " +
	`"bug" for a tool or behavior that is broken, ` +
	`"improvement" for something that works but could be better, ` +
	`"unnecessary-result" for a tool result that was confusing or wasted tokens, ` +
	`"new-tool" for a tool you wish existed.`

func (t *FeedbackTool) Name() string { return string(tools.FEEDBACK) }

func (t *FeedbackTool) Description() string { return feedbackDesc }

func (t *FeedbackTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["feedback","category"],
		"properties":{
			"category":{"type":"string","enum":["bug","improvement","unnecessary-result","new-tool"],"description":"Type of feedback"},
			"feedback":{"type":"string","description":"feedback content to evva developers (markdown format)"}
		}
	}`)
}

type feedbackInput struct {
	Category string `json:"category"`
	Feedback string `json:"feedback"`
}

func (t *FeedbackTool) Execute(_ context.Context, input json.RawMessage) (tools.Result, error) {
	var in feedbackInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("feedback: bad input: %v", err)}, nil
	}

	cfg := config.Get()
	dir := filepath.Join(cfg.EvvaHome, "feedbacks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("feedback: cannot create directory: %v", err)}, nil
	}

	ts := time.Now().Format("2006-01-02T150405")
	filename := fmt.Sprintf("feedback_%s_%s.md", in.Category, ts)
	path := filepath.Join(dir, filename)

	body := fmt.Sprintf("> category: %s\n\n%s", in.Category, in.Feedback)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("feedback: cannot write file: %v", err)}, nil
	}

	return tools.Result{Content: fmt.Sprintf("Feedback saved to %s.", path)}, nil
}
