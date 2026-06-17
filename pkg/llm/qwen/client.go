// Package qwen implements llm.Client for Alibaba Cloud's Tongyi/Qwen models
// reached over the DashScope OpenAI-compatible endpoint. The wire format is the
// same OpenAI chat-completions shape DeepSeek uses, so this is a copy of the
// deepseek engine with two differences: the base URL + Bearer key target
// DashScope, and thinking is controlled by DashScope's top-level
// `enable_thinking` / `thinking_budget` fields (not DeepSeek's `thinking` +
// `reasoning_effort`). Reasoning output rides back on `reasoning_content`, the
// same field DeepSeek uses.
package qwen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/johnny1110/evva/pkg/constant"

	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/tools"
)

const (
	DefaultModel = "qwen3.7-plus"
	chatPath     = "/chat/completions"
)

// Client implements llm.Client backed by DashScope's OpenAI-compatible chat API.
type Client struct {
	name   string
	apiURL string
	apiKey string
	model  string
	params llm.LLMParams
}

// New builds a Qwen client from provider config and applies the given options.
// Options can be re-applied at runtime via Apply.
func New(cfg llm.APIConfig, model string, opts ...llm.Option) *Client {
	if model == "" {
		model = DefaultModel
	}
	c := &Client{
		name:   constant.QWEN.Name,
		apiURL: strings.TrimRight(cfg.ApiURL, "/"),
		apiKey: cfg.ApiSecret,
		model:  model,
	}
	c.params.Apply(opts...)
	return c
}

func (c *Client) Apply(opts ...llm.Option) { c.params.Apply(opts...) }

// Name provider name
func (c *Client) Name() string {
	return c.name
}

// SupportsDeferLoading reports false — the OpenAI-compatible wire has no
// defer_loading mechanism. Mutating the tools array between turns changes the
// request prefix and invalidates the provider's KV cache.
func (c *Client) SupportsDeferLoading() bool { return false }

func (c *Client) Model() string     { return c.model }
func (c *Client) SetModel(m string) { c.model = m }

// --- API wire types -------------------------------------------------------

// apiMessage mirrors the OpenAI chat-completions message shape. Content is NOT
// tagged omitempty so an assistant message carrying only tool_calls still sends
// an explicit empty `content` field (strict OpenAI-compatible deserializers
// reject a missing one).
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ReasoningContent carries the model's thinking; echoed back on subsequent
	// assistant turns in the same conversation.
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ToolCalls        []apiToolCall `json:"tool_calls,omitempty"`
}

type apiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type apiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type apiRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Temperature *float64     `json:"temperature,omitempty"`
	TopP        *float64     `json:"top_p,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Stop        []string     `json:"stop,omitempty"`
	Tools       []apiTool    `json:"tools,omitempty"`
	// EnableThinking / ThinkingBudget are DashScope's top-level thinking
	// controls. EnableThinking nil → omit (let the model's native default
	// decide); never sent as false, which a thinking-native model could reject.
	EnableThinking *bool             `json:"enable_thinking,omitempty"`
	ThinkingBudget int               `json:"thinking_budget,omitempty"`
	Stream         bool              `json:"stream,omitempty"`
	StreamOptions  *apiStreamOptions `json:"stream_options,omitempty"`
}

// qwenThinking maps evva effort levels to DashScope's enable_thinking flag and
// thinking_budget (a token cap on the thinking phase). effort 0 omits the flag
// so the model's native default decides — it never sends enable_thinking:false
// (a thinking-native model like qwen3.7-max could reject it); 1–4 enable
// thinking with a climbing budget.
//
//	0 → enable=nil (omit, model default)
//	1 → enable=true, budget=8192   (evva "low")
//	2 → enable=true, budget=16384  (evva "medium")
//	3 → enable=true, budget=32768  (evva "high")
//	4 → enable=true, budget=81920  (evva "ultra")
func qwenThinking(effort int) (enable *bool, budget int) {
	if effort <= 0 {
		return nil, 0
	}
	on := true
	switch effort {
	case 1:
		return &on, 8192
	case 2:
		return &on, 16384
	case 3:
		return &on, 32768
	default: // 4+
		return &on, 81920
	}
}

// apiStreamOptions tweaks the OpenAI-compatible SSE response. include_usage asks
// the provider to send a final delta carrying the total token counts; without
// it the streaming response would never surface a Usage struct.
type apiStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type apiResponse struct {
	Choices []struct {
		Message      apiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details,omitempty"`
	} `json:"usage,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// --- Client interface -----------------------------------------------------

func (c *Client) Complete(ctx context.Context, messages []llm.Message, toolSet []tools.Tool) (llm.Response, error) {
	if c.apiKey == "" {
		return llm.Response{}, fmt.Errorf("qwen: missing API key (type in /config to setup)")
	}

	enableThinking, thinkingBudget := qwenThinking(c.params.Effort)
	body := apiRequest{
		Model:          c.model,
		Messages:       toAPIMessages(messages, c.params.System),
		Temperature:    c.params.Temperature,
		TopP:           c.params.TopP,
		MaxTokens:      c.params.MaxTokens,
		Stop:           c.params.StopSequences,
		Tools:          toAPITools(toolSet),
		EnableThinking: enableThinking,
		ThinkingBudget: thinkingBudget,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+chatPath, bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+c.apiKey)

	resp, err := c.params.HTTP().Do(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: http: %w", llm.NormalizeErr(err))
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: read body: %w", llm.NormalizeErr(err))
	}
	if resp.StatusCode/100 != 2 {
		return llm.Response{}, fmt.Errorf("qwen: http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed apiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("qwen: decode response: %w", err)
	}
	if parsed.Error != nil {
		return llm.Response{}, fmt.Errorf("qwen: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("qwen: empty choices")
	}

	msg := parsed.Choices[0].Message
	out := llm.Response{
		Content:  msg.Content,
		Thinking: msg.ReasoningContent,
	}
	for _, call := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, &tools.Call{
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: json.RawMessage(call.Function.Arguments),
		})
	}
	if parsed.Usage != nil {
		out.Usage = llm.Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		}
		if d := parsed.Usage.CompletionTokensDetails; d != nil {
			out.Usage.ReasoningTokens = d.ReasoningTokens
		}
	}
	return out, nil
}

// streamChunk is one SSE frame off DashScope's streaming chat completions
// endpoint. Deltas live under choices[i].Delta; the terminal frame (when
// include_usage is set) populates Usage; tool-call argument fragments
// accumulate per-index in Delta.ToolCalls[i].Function.Arguments.
type streamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string                `json:"role,omitempty"`
			Content          string                `json:"content,omitempty"`
			ReasoningContent string                `json:"reasoning_content,omitempty"`
			ToolCalls        []streamToolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		TotalTokens             int `json:"total_tokens"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details,omitempty"`
	} `json:"usage,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// streamToolCallDelta is one streaming fragment of a tool call. Index is stable
// across fragments for the same call; ID and Function.Name appear on the first
// fragment, Function.Arguments accumulates over subsequent ones.
type streamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// Stream is the chunked variant of Complete. It opens a streaming SSE
// connection, forwards each delta to sink as a Chunk, and returns the
// fully-assembled Response once the server emits its terminal data: [DONE].
func (c *Client) Stream(ctx context.Context, messages []llm.Message, toolSet []tools.Tool, sink llm.ChunkSink) (llm.Response, error) {
	if c.apiKey == "" {
		return llm.Response{}, fmt.Errorf("qwen: missing API key (type in /config to setup)")
	}
	if sink == nil {
		sink = llm.DiscardChunks
	}

	enableThinking, thinkingBudget := qwenThinking(c.params.Effort)
	body := apiRequest{
		Model:          c.model,
		Messages:       toAPIMessages(messages, c.params.System),
		Temperature:    c.params.Temperature,
		TopP:           c.params.TopP,
		MaxTokens:      c.params.MaxTokens,
		Stop:           c.params.StopSequences,
		Tools:          toAPITools(toolSet),
		EnableThinking: enableThinking,
		ThinkingBudget: thinkingBudget,
		Stream:         true,
		StreamOptions:  &apiStreamOptions{IncludeUsage: true},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+chatPath, bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("authorization", "Bearer "+c.apiKey)

	resp, err := c.params.HTTP().Do(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("qwen: http: %w", llm.NormalizeErr(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(resp.Body)
		return llm.Response{}, fmt.Errorf("qwen: http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return c.consumeStream(ctx, resp.Body, sink)
}

// consumeStream is the SSE decoding loop, factored out for testability with a
// synthetic io.Reader.
func (c *Client) consumeStream(ctx context.Context, body io.Reader, sink llm.ChunkSink) (llm.Response, error) {
	scanner := bufio.NewScanner(body)
	// Frames can exceed bufio's 64 KB default — long tool-call argument JSON in
	// particular. 1 MB is plenty headroom.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var (
		out         llm.Response
		text        strings.Builder
		reasoning   strings.Builder
		toolBuffers = map[int]*streamingToolCall{}
		toolOrder   []int
	)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return llm.Response{}, llm.ErrInterrupted
			}
			return llm.Response{}, err
		}
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				break
			}
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return llm.Response{}, fmt.Errorf("qwen: decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return llm.Response{}, fmt.Errorf("qwen: %s: %s", chunk.Error.Type, chunk.Error.Message)
		}

		for _, ch := range chunk.Choices {
			if d := ch.Delta.Content; d != "" {
				text.WriteString(d)
				sink.OnChunk(llm.Chunk{Kind: llm.ChunkText, Delta: d})
			}
			if d := ch.Delta.ReasoningContent; d != "" {
				reasoning.WriteString(d)
				sink.OnChunk(llm.Chunk{Kind: llm.ChunkThinking, Delta: d})
			}
			for _, tc := range ch.Delta.ToolCalls {
				buf, exists := toolBuffers[tc.Index]
				if !exists {
					buf = &streamingToolCall{}
					toolBuffers[tc.Index] = buf
					toolOrder = append(toolOrder, tc.Index)
				}
				if tc.ID != "" {
					buf.id = tc.ID
				}
				if tc.Function.Name != "" {
					buf.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					buf.args.WriteString(tc.Function.Arguments)
				}
			}
		}

		if chunk.Usage != nil {
			out.Usage = llm.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			if d := chunk.Usage.CompletionTokensDetails; d != nil {
				out.Usage.ReasoningTokens = d.ReasoningTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return llm.Response{}, llm.ErrInterrupted
		}
		return llm.Response{}, fmt.Errorf("qwen: stream: %w", llm.NormalizeErr(err))
	}

	out.Content = text.String()
	out.Thinking = reasoning.String()
	for _, idx := range toolOrder {
		buf := toolBuffers[idx]
		args := buf.args.String()
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, &tools.Call{
			ID:    buf.id,
			Name:  buf.name,
			Input: json.RawMessage(args),
		})
	}
	return out, nil
}

type streamingToolCall struct {
	id   string
	name string
	args strings.Builder
}

// --- Conversion helpers ---------------------------------------------------

func toAPIMessages(msgs []llm.Message, system string) []apiMessage {
	out := make([]apiMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, apiMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleSystem:
			out = append(out, apiMessage{Role: "system", Content: m.Content})
		case llm.RoleUser:
			out = append(out, apiMessage{Role: "user", Content: m.Content})
		case llm.RoleAssistant:
			am := apiMessage{Role: "assistant", Content: m.Content, ReasoningContent: m.Thinking}
			for _, c := range m.ToolCalls {
				tc := apiToolCall{ID: c.ID, Type: "function"}
				tc.Function.Name = c.Name
				tc.Function.Arguments = string(c.Input)
				am.ToolCalls = append(am.ToolCalls, tc)
			}
			out = append(out, am)
		case llm.RoleTool:
			// OpenAI-style: one tool-role message per tool_call_id.
			for _, tr := range m.ToolResults {
				content := tr.Content
				if len(tr.ContentBlocks) > 0 {
					content = llm.RenderContentBlocksAsText(tr.ContentBlocks)
				}
				out = append(out, apiMessage{
					Role:       "tool",
					Content:    content,
					ToolCallID: tr.ID,
				})
			}
		}
	}
	return out
}

func toAPITools(toolSet []tools.Tool) []apiTool {
	if len(toolSet) == 0 {
		return nil
	}
	out := make([]apiTool, 0, len(toolSet))
	for _, t := range toolSet {
		var entry apiTool
		entry.Type = "function"
		entry.Function.Name = t.Name()
		entry.Function.Description = t.Description()
		entry.Function.Parameters = llm.ToolSchema(t)
		out = append(out, entry)
	}
	return out
}
