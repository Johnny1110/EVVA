// Package memory hosts the auto-memory tools: update_user_profile and
// update_project_memory. They are the write counterparts to the read
// helpers in internal/memdir. The tools merge structured section updates
// into fixed markdown shapes:
//
//   - <APP_HOME>/USER_PROFILE.md             global user notes
//   - <APP_HOME>/projects/<key>/MEMORY.md    project-scoped notes
//
// The schemas constrain the section keys so updates always land in a
// known shape — the model cannot accidentally create new section names
// or clobber unrelated content. Anything not provided in `sections` is
// preserved verbatim by memdir.MergeSections.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/johnny1110/evva/internal/memdir"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/tools"
)

// Names lists every tool this package contributes.
func Names() []tools.ToolName {
	return []tools.ToolName{
		tools.UPDATE_USER_PROFILE,
		tools.UPDATE_PROJECT_MEMORY,
	}
}

// MemoryDiff is the UI-facing metadata carried on the tool Result. It
// mirrors the spirit of fs.FileDiff but stays in this package to avoid
// dragging the fs diff renderer into a tool that does not produce a
// line-level patch.
type MemoryDiff struct {
	Path           string   // absolute path of the file that was written
	SectionsUpdated []string // headings whose bodies were replaced this call
	WasCreated     bool     // true when the file did not exist before this call
}

// --- update_user_profile ----------------------------------------------

const updateUserProfileDescription = `Merge updates into <APP_HOME>/USER_PROFILE.md — your persistent notes about the user across all projects.

Use sparingly. Persist only things that will still be true next session: preferences the user has stated, working-style observations you have confirmed, recurring topics they return to. Do NOT save ephemeral task state, debugging steps, project-specific facts (use update_project_memory for those), or anything already in EVVA.md.

The "sections" field is a map keyed by section heading. Each value REPLACES the body of that section; sections not present in the call are preserved verbatim. Sending an empty string for a section clears it (but keeps the heading visible). Unknown section names are rejected.

Allowed sections:
- "Preferences"      — terse/verbose, tools to prefer, what the user dislikes
- "Working style"    — how the user collaborates: how much explanation they want, how they sign off
- "Recurring topics" — areas they come back to (a project, a tool, a body of work)

Examples:
- User says "I prefer tabs over spaces" → call with sections={"Preferences": "<existing-or-new prefs body that includes 'tabs over spaces'>"}.
- User corrects you on output verbosity → update "Working style".

Skip the call entirely if the fact only matters for this conversation. Memory grows across sessions; bad entries are loud and persistent.`

const updateUserProfileSchema = `{
	"type":"object",
	"additionalProperties":false,
	"required":["sections"],
	"properties":{
		"sections":{
			"type":"object",
			"description":"Map of section heading -> new body. Only headings in the allowed set are accepted: \"Preferences\", \"Working style\", \"Recurring topics\".",
			"additionalProperties":{"type":"string"}
		}
	}
}`

type updateUserProfileInput struct {
	Sections map[string]string `json:"sections"`
}

// UpdateUserProfileTool implements the update_user_profile tool.
type UpdateUserProfileTool struct {
	cfg *config.Config
}

// NewUpdateUserProfile constructs the tool. cfg may be nil in tests; in
// that case the tool reports a config-missing error at Execute time.
func NewUpdateUserProfile(cfg *config.Config) *UpdateUserProfileTool {
	return &UpdateUserProfileTool{cfg: cfg}
}

func (t *UpdateUserProfileTool) Name() string            { return string(tools.UPDATE_USER_PROFILE) }
func (t *UpdateUserProfileTool) Description() string     { return updateUserProfileDescription }
func (t *UpdateUserProfileTool) Schema() json.RawMessage { return json.RawMessage(updateUserProfileSchema) }

func (t *UpdateUserProfileTool) Execute(_ context.Context, logger *slog.Logger, raw json.RawMessage) (tools.Result, error) {
	if t.cfg == nil || t.cfg.AppHome == "" {
		return tools.Result{IsError: true, Content: "update_user_profile: no AppHome configured"}, nil
	}
	if !t.cfg.GetEnableAutoMemory() {
		return tools.Result{IsError: true, Content: "update_user_profile: auto-memory is disabled (toggle via /config)"}, nil
	}

	var in updateUserProfileInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tools.Result{IsError: true, Content: "update_user_profile: invalid input: " + err.Error()}, nil
	}
	if len(in.Sections) == 0 {
		return tools.Result{IsError: true, Content: "update_user_profile: sections map is required and must be non-empty"}, nil
	}

	path := memdir.UserProfilePath(t.cfg.AppHome)
	existing, warn := readExisting(path)
	if warn != "" {
		logger.Warn("update_user_profile.read", "warn", warn)
	}
	wasCreated := existing == ""

	merged, err := memdir.MergeSections(existing, in.Sections, memdir.UserProfileSections)
	if err != nil {
		return tools.Result{IsError: true, Content: "update_user_profile: " + err.Error()}, nil
	}
	if err := memdir.WriteUserProfile(t.cfg.AppHome, merged); err != nil {
		logger.Warn("update_user_profile.write", "err", err, "path", path)
		return tools.Result{IsError: true, Content: "update_user_profile: " + err.Error()}, nil
	}

	updated := sortedKeys(in.Sections)
	logger.Info("update_user_profile.ok", "path", path, "sections", updated)
	return tools.Result{
		Content: fmt.Sprintf("updated %s (sections: %s)", path, strings.Join(updated, ", ")),
		Metadata: &MemoryDiff{
			Path:            path,
			SectionsUpdated: updated,
			WasCreated:      wasCreated,
		},
	}, nil
}

// --- update_project_memory --------------------------------------------

const updateProjectMemoryDescription = `Merge updates into <APP_HOME>/projects/<slug>/MEMORY.md — your persistent notes scoped to the current project. The slug is derived from the workdir absolute path so each project gets its own memory file.

Use this for facts that are true about THIS project but not derivable from the code or git history: deadlines, decisions and their motivations, open bugs the user has flagged, pointers to external dashboards or Linear projects, conventions the user has confirmed apply here.

The "sections" field is a map keyed by section heading. Each value REPLACES the body of that section; sections not present in the call are preserved verbatim. Sending an empty string for a section clears it (but keeps the heading visible). Unknown section names are rejected.

Allowed sections:
- "Project facts" — context that is not in the code: deadlines, headcount, scope decisions
- "Decisions"    — choices the team / user has made and the reason
- "Open issues"  — bugs / regressions / TODOs the user wants you to remember between sessions
- "References"   — pointers to external systems (Linear project IDs, Grafana boards, Slack channels)

Do NOT save: code patterns, file paths, architectural shape (re-deriving from the code is cheap), git history, in-flight task state (use todo_write), anything already in EVVA.md. If a fact only matters for this conversation, skip the call.

Before saving a Decision or Open issue that names a file/function/flag, briefly verify it still exists with read or grep — memory that points at deleted code is worse than no memory.`

const updateProjectMemorySchema = `{
	"type":"object",
	"additionalProperties":false,
	"required":["sections"],
	"properties":{
		"sections":{
			"type":"object",
			"description":"Map of section heading -> new body. Only headings in the allowed set are accepted: \"Project facts\", \"Decisions\", \"Open issues\", \"References\".",
			"additionalProperties":{"type":"string"}
		}
	}
}`

type updateProjectMemoryInput struct {
	Sections map[string]string `json:"sections"`
}

// UpdateProjectMemoryTool implements the update_project_memory tool.
type UpdateProjectMemoryTool struct {
	cfg     *config.Config
	workdir string
}

// NewUpdateProjectMemory constructs the tool. workdir is captured at
// agent construction so the project key is stable across the session
// even if the user runs `cd` from a tool call.
func NewUpdateProjectMemory(cfg *config.Config, workdir string) *UpdateProjectMemoryTool {
	return &UpdateProjectMemoryTool{cfg: cfg, workdir: workdir}
}

func (t *UpdateProjectMemoryTool) Name() string            { return string(tools.UPDATE_PROJECT_MEMORY) }
func (t *UpdateProjectMemoryTool) Description() string     { return updateProjectMemoryDescription }
func (t *UpdateProjectMemoryTool) Schema() json.RawMessage { return json.RawMessage(updateProjectMemorySchema) }

func (t *UpdateProjectMemoryTool) Execute(_ context.Context, logger *slog.Logger, raw json.RawMessage) (tools.Result, error) {
	if t.cfg == nil || t.cfg.AppHome == "" {
		return tools.Result{IsError: true, Content: "update_project_memory: no AppHome configured"}, nil
	}
	if t.workdir == "" {
		return tools.Result{IsError: true, Content: "update_project_memory: no workdir captured"}, nil
	}
	if !t.cfg.GetEnableAutoMemory() {
		return tools.Result{IsError: true, Content: "update_project_memory: auto-memory is disabled (toggle via /config)"}, nil
	}

	var in updateProjectMemoryInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tools.Result{IsError: true, Content: "update_project_memory: invalid input: " + err.Error()}, nil
	}
	if len(in.Sections) == 0 {
		return tools.Result{IsError: true, Content: "update_project_memory: sections map is required and must be non-empty"}, nil
	}

	path := memdir.ProjectMemoryPath(t.cfg.AppHome, t.workdir)
	if path == "" {
		return tools.Result{IsError: true, Content: "update_project_memory: cannot resolve project memory path"}, nil
	}
	existing, warn := readExisting(path)
	if warn != "" {
		logger.Warn("update_project_memory.read", "warn", warn)
	}
	wasCreated := existing == ""

	merged, err := memdir.MergeSections(existing, in.Sections, memdir.ProjectMemorySections)
	if err != nil {
		return tools.Result{IsError: true, Content: "update_project_memory: " + err.Error()}, nil
	}
	if err := memdir.WriteProjectMemory(t.cfg.AppHome, t.workdir, merged); err != nil {
		logger.Warn("update_project_memory.write", "err", err, "path", path)
		return tools.Result{IsError: true, Content: "update_project_memory: " + err.Error()}, nil
	}

	updated := sortedKeys(in.Sections)
	logger.Info("update_project_memory.ok", "path", path, "sections", updated)
	return tools.Result{
		Content: fmt.Sprintf("updated %s (sections: %s)", path, strings.Join(updated, ", ")),
		Metadata: &MemoryDiff{
			Path:            path,
			SectionsUpdated: updated,
			WasCreated:      wasCreated,
		},
	}, nil
}

// --- helpers -----------------------------------------------------------

// readExisting returns the body and an optional warning. Missing files
// return ("", "") so the merge step treats the missing case as "scaffold
// a fresh file with the allowed sections."
func readExisting(path string) (string, string) {
	if path == "" {
		return "", ""
	}
	// memdir.readMemFile is unexported, but Load doesn't fit our shape
	// (it reads two known files). Inline the small open+read+cap call so
	// the dependency stays one-way (we don't add a getter just for this).
	body, warn := readFileCapped(path)
	return body, warn
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
