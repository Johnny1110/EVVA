package tools

import "encoding/json"

// Descriptor is the LLM-facing metadata for a tool — name, description, JSON
// input schema, and a short list of keyword tags for discovery.
//
// Describing a tool does NOT make it callable. TOOL_SEARCH and any future
// schema-introspection consumer use this type to surface deferred-tool
// metadata without paying for tool construction or state allocation.
//
// Lives in the tools package (not toolset) so meta/toolsearch can reference
// it without importing toolset — toolset already depends on meta, so the
// reverse edge would cycle.
type Descriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Tags        []string

	// SearchHint is a short curated capability phrase that the TOOL_SEARCH
	// matcher prioritizes over the long-form description. One sentence is
	// enough — think "what would the LLM type to find this tool that isn't
	// already in the tags or name?". Optional; empty means fall back to
	// description matching.
	//
	// Word-boundary scoring (+4 in TOOL_SEARCH ranking) gives hint matches
	// stronger signal than description hits (+2) but lower than exact name
	// part matches.
	SearchHint string
}
