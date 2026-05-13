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
}
