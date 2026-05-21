package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/johnny1110/evva/pkg/tools"
)

// DeferredLookupFn is the late-binding shape NewToolSearch accepts; pass a
// method value bound to whatever owns the lookup (typically toolset.ToolState).
type DeferredLookupFn func() DeferredLookup

// ToolSearchTool exposes deferred-tool metadata to the model.
//
// Deferred tools appear by name in the system prompt; their full JSON Schemas
// are pre-injected into the same prompt at session start (see
// sysprompt.mainDeferredToolsSection). TOOL_SEARCH itself returns a compact
// JSON envelope — the model has already seen the schemas and uses
// TOOL_SEARCH for discovery / "is this one available", not schema fetching.
//
// This output shape mirrors ref/src/tools/ToolSearchTool/ToolSearchTool.ts.
// The divergence from ref: ref pre-injects only names and uses Anthropic
// tool_reference blocks to expand schemas on the wire; evva pre-injects full
// schemas because not every provider supports tool_reference expansion.
type ToolSearchTool struct {
	lookup DeferredLookupFn
}

// NewToolSearch constructs a ToolSearchTool that reads its lookup at
// Execute time. lookup may be nil (yields a clear runtime error); it may
// also return nil (same outcome).
func NewToolSearch(lookup DeferredLookupFn) *ToolSearchTool {
	return &ToolSearchTool{lookup: lookup}
}

func (t *ToolSearchTool) Name() string { return string(tools.TOOL_SEARCH) }

func (t *ToolSearchTool) Description() string {
	return "Fetches the names of deferred tools that match a query so they can be called.\n\n" +
		"Deferred tools appear by name in this session's system prompt and their full JSON schemas are pre-loaded there too. Use this tool to discover or confirm which deferred tools match a topic — you do NOT need to call it before invoking a deferred tool whose name you already know.\n\n" +
		"Result format: a JSON object `{\"matches\": [...], \"query\": \"...\", \"total_deferred_tools\": N}`. The matched names are wire-callable immediately; their schemas are already in the system prompt.\n\n" +
		"Query forms:\n" +
		"- \"select:Read,Edit,Grep\" — fetch these exact tools by name\n" +
		"- \"notebook jupyter\" — keyword search, up to max_results best matches\n" +
		"- \"+slack send\" — require \"slack\" in the name, rank by remaining terms"
}

func (t *ToolSearchTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["query"],
		"properties":{
			"query":{"type":"string","description":"Query: \"select:<name>[,<name>...]\" for exact names, or whitespace-separated keywords (prefix with \"+\" to require the term)."},
			"max_results":{"type":"integer","minimum":1,"default":5,"description":"Cap the number of returned matches. Default 5."}
		}
	}`)
}

type toolSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// searchOutput mirrors ref's output schema. Field order is fixed (matches,
// query, total_deferred_tools) so JSON byte-equality tests on the LLM-facing
// envelope are stable.
type searchOutput struct {
	Matches            []string `json:"matches"`
	Query              string   `json:"query"`
	TotalDeferredTools int      `json:"total_deferred_tools"`
}

func (t *ToolSearchTool) Execute(_ context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	var in toolSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("tool_search: decode: %v", err)}, nil
	}
	if strings.TrimSpace(in.Query) == "" {
		return tools.Result{IsError: true, Content: "tool_search: query is required"}, nil
	}
	if t.lookup == nil {
		return tools.Result{IsError: true, Content: "tool_search: no deferred-lookup configured"}, nil
	}
	lookup := t.lookup()
	if lookup == nil {
		return tools.Result{IsError: true, Content: "tool_search: no deferred-lookup configured (root agent only)"}, nil
	}
	max := in.MaxResults
	if max <= 0 {
		max = 5
	}
	logger.Debug("toolsearch.dispatch", "query", in.Query, "max", max)

	descriptors, err := allDescriptors(lookup)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("tool_search: enumerate: %v", err)}, nil
	}
	total := len(descriptors)

	matches := searchDescriptors(in.Query, max, descriptors)
	logger.Debug("toolsearch.result", "matched", len(matches), "total", total)

	out := searchOutput{
		Matches:            matches,
		Query:              in.Query,
		TotalDeferredTools: total,
	}
	body, err := json.Marshal(out)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("tool_search: marshal: %v", err)}, nil
	}
	return tools.Result{Content: string(body)}, nil
}

// allDescriptors fetches every deferred descriptor through the lookup and
// returns them sorted by name for stable output. Per-tool errors are
// surfaced as a single combined error — the model will rarely care about
// per-name failures, and aborting the whole search is friendlier than
// silently dropping one name.
func allDescriptors(lookup DeferredLookup) ([]tools.Descriptor, error) {
	names := lookup.DeferredNames()
	out := make([]tools.Descriptor, 0, len(names))
	var firstErr error
	for _, n := range names {
		d, err := lookup.Describe(n)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if firstErr != nil && len(out) == 0 {
		return nil, firstErr
	}
	return out, nil
}

// searchDescriptors ranks descriptors against query under the three documented
// query forms. Returns at most max matched names.
func searchDescriptors(query string, max int, all []tools.Descriptor) []string {
	q := strings.ToLower(strings.TrimSpace(query))

	// 1. select: form — exact name lookup, preserves the user's order.
	if rest, ok := strings.CutPrefix(q, "select:"); ok {
		return selectByName(rest, all, max)
	}

	// 2. Fast path: exact match on a tool name (case-insensitive). Catches
	//    models typing a bare tool name instead of `select:Name`.
	for _, d := range all {
		if strings.EqualFold(d.Name, q) {
			return []string{d.Name}
		}
	}

	// 3. MCP-prefix fast path: query like "mcp__server" returns every tool
	//    in that server's namespace. Unused today (evva has no MCP tools)
	//    but kept so Phase 13 doesn't reintroduce the logic.
	if strings.HasPrefix(q, "mcp__") && len(q) > 5 {
		var out []string
		for _, d := range all {
			if strings.HasPrefix(strings.ToLower(d.Name), q) {
				out = append(out, d.Name)
				if len(out) >= max {
					break
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// 4. Tokenize. "+keyword" tokens are required filters; the rest contribute
	//    to score.
	var required, optional []string
	for _, tok := range strings.FieldsFunc(q, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' }) {
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "+") {
			required = append(required, tok[1:])
		} else {
			optional = append(optional, tok)
		}
	}
	if len(required) == 0 && len(optional) == 0 {
		return nil
	}

	type scored struct {
		name  string
		score int
	}
	scoringTerms := append([]string{}, required...)
	scoringTerms = append(scoringTerms, optional...)
	var ranked []scored
	for _, d := range all {
		parsed := parseToolName(d.Name)
		descLower := strings.ToLower(d.Description)
		hintLower := strings.ToLower(d.SearchHint)

		// Required tokens must all hit somewhere.
		ok := true
		for _, r := range required {
			if !hitsToolForToken(d, parsed, descLower, hintLower, r) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}

		s := 0
		for _, tok := range scoringTerms {
			s += scoreToolForToken(d, parsed, descLower, hintLower, tok)
		}
		if s == 0 && len(optional) > 0 {
			continue
		}
		ranked = append(ranked, scored{name: d.Name, score: s})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].name < ranked[j].name
	})
	if len(ranked) > max {
		ranked = ranked[:max]
	}
	out := make([]string, len(ranked))
	for i, s := range ranked {
		out[i] = s.name
	}
	return out
}

// selectByName implements "select:a,b,c" — exact (case-insensitive) name
// lookup; unknown names are silently dropped. Capped at max to match the
// documented max_results behavior (schema default: 5).
func selectByName(list string, all []tools.Descriptor, max int) []string {
	wanted := strings.Split(list, ",")
	out := make([]string, 0, len(wanted))
	for _, w := range wanted {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		for _, d := range all {
			if strings.EqualFold(d.Name, w) {
				out = append(out, d.Name)
				break
			}
		}
		if len(out) >= max {
			break
		}
	}
	return out
}

// scoreToolForToken grants weighted credit for tok against one tool. Mirrors
// the layered scoring from ref TS, with evva's tag-fuzzy bonus added on top.
func scoreToolForToken(d tools.Descriptor, p parsedName, descLower, hintLower, tok string) int {
	score := namedPartScore(p, tok)

	// Full-name fallback: only when no part-match landed (matches ref's
	// `score === 0` guard).
	if score == 0 && strings.Contains(p.full, tok) {
		score += scoreFullNameFallback
	}

	if hintLower != "" {
		if wordBoundaryPattern(tok).MatchString(hintLower) {
			score += scoreSearchHint
		}
	}
	if descLower != "" {
		if wordBoundaryPattern(tok).MatchString(descLower) {
			score += scoreDescription
		}
	}
	// Tag fuzzy bonus — evva-specific (ref TS has no tags field).
	score += fuzzyTagScore(d.Tags, tok)
	return score
}

// hitsToolForToken is the binary version used by required "+token" filtering.
func hitsToolForToken(d tools.Descriptor, p parsedName, descLower, hintLower, tok string) bool {
	for _, part := range p.parts {
		if part == tok || strings.Contains(part, tok) {
			return true
		}
	}
	if hintLower != "" && wordBoundaryPattern(tok).MatchString(hintLower) {
		return true
	}
	if descLower != "" && wordBoundaryPattern(tok).MatchString(descLower) {
		return true
	}
	return fuzzyTagHit(d.Tags, tok)
}
