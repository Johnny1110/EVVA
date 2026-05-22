package mode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/johnny1110/evva/internal/permission"
	"github.com/johnny1110/evva/pkg/tools"
)

// maxSlugLen mirrors the ref source: 64 chars total across all segments,
// long enough for a descriptive name without blowing past filesystem
// limits when combined with the worktree directory prefix.
const maxSlugLen = 64

// slugSegmentRE accepts one segment between the slash separators: letters,
// digits, dot, underscore, dash. Forbids "." / ".." entirely (the
// validateSlug check rejects them after splitting).
var slugSegmentRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// --- EnterWorktree -----------------------------------------------------

const enterWorktreeDescription = `Use this tool ONLY when the user explicitly asks to work in a worktree. This tool creates an isolated git worktree and switches the current session into it.

## When to Use

- The user explicitly says "worktree" (e.g., "start a worktree", "work in a worktree", "create a worktree", "use a worktree")

## When NOT to Use

- The user asks to create a branch, switch branches, or work on a different branch — use git commands instead
- The user asks to fix a bug or work on a feature — use normal git workflow unless they specifically mention worktrees
- Never use this tool unless the user explicitly mentions "worktree"

## Requirements

- Must be in a git repository
- Must not already be in a worktree session created by this tool

## Behavior

- Creates a new git worktree inside ` + "`.evva/worktrees/`" + ` at the repository root, on a new branch ` + "`worktree-<slug>`" + ` based on HEAD
- Switches the agent's working directory to the new worktree — subsequent file reads, edits, and bash commands run there
- Use exit_worktree to leave the worktree mid-session (keep or remove). The exit tool is a no-op when no worktree session is active

## Parameters

- ` + "`name`" + ` (optional): A name for the worktree. Each "/"-separated segment may contain only letters, digits, dots, underscores, and dashes; max 64 chars total. If omitted, a random name is generated.
`

const enterWorktreeSchema = `{
	"type":"object",
	"additionalProperties":false,
	"properties":{
		"name":{"type":"string","description":"Optional name for the new worktree. Each \"/\"-separated segment may contain only letters, digits, dots, underscores, and dashes; max 64 chars total."}
	}
}`

type enterWorktreeInput struct {
	Name string `json:"name"`
}

// EnterWorktreeTool creates `.evva/worktrees/<slug>/` on a new branch
// `worktree-<slug>` and switches the agent's workdir into it.
type EnterWorktreeTool struct {
	lookup WorktreeControllerLookup
}

func NewEnterWorktree(lookup WorktreeControllerLookup) *EnterWorktreeTool {
	return &EnterWorktreeTool{lookup: lookup}
}

func (t *EnterWorktreeTool) Name() string            { return string(tools.ENTER_WORKTREE) }
func (t *EnterWorktreeTool) Description() string     { return enterWorktreeDescription }
func (t *EnterWorktreeTool) Schema() json.RawMessage { return json.RawMessage(enterWorktreeSchema) }

func (t *EnterWorktreeTool) Execute(ctx context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	ctrl := resolveWorktreeController(t.lookup)
	if ctrl == nil {
		return tools.Result{
			IsError: true,
			Content: "enter_worktree: no worktree controller installed (only the root agent may enter a worktree)",
		}, nil
	}
	if ctrl.WorktreeSession() != nil {
		return tools.Result{
			IsError: true,
			Content: "enter_worktree: already in a worktree session — call exit_worktree first",
		}, nil
	}

	var in enterWorktreeInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return tools.Result{IsError: true, Content: fmt.Sprintf("enter_worktree: decode input: %v", err)}, nil
		}
	}

	slug, err := resolveSlug(in.Name)
	if err != nil {
		return tools.Result{IsError: true, Content: "enter_worktree: " + err.Error()}, nil
	}

	original := ctrl.Workdir()
	if original == "" {
		return tools.Result{IsError: true, Content: "enter_worktree: agent has no working directory"}, nil
	}
	repoRoot, err := gitTopLevel(ctx, original)
	if err != nil {
		return tools.Result{IsError: true, Content: "enter_worktree: not in a git repository (" + err.Error() + ")"}, nil
	}

	flat := flattenSlug(slug)
	worktreePath := worktreeDirFor(repoRoot, flat)
	branch := branchNameFor(flat)

	if _, statErr := os.Stat(worktreePath); statErr == nil {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("enter_worktree: %s already exists — pick a different name or remove it first", worktreePath),
		}, nil
	}

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return tools.Result{IsError: true, Content: "enter_worktree: cannot create worktree parent dir: " + err.Error()}, nil
	}

	if out, gErr := runGit(ctx, repoRoot, "worktree", "add", "-b", branch, worktreePath, "HEAD"); gErr != nil {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("enter_worktree: git worktree add failed: %v\n%s", gErr, out),
		}, nil
	}

	if err := ctrl.SwitchWorkdir(worktreePath); err != nil {
		// Roll back the worktree on switch failure so a half-created session
		// doesn't strand the workdir; best-effort, log on failure.
		if rmOut, rmErr := runGit(ctx, repoRoot, "worktree", "remove", "--force", worktreePath); rmErr != nil {
			logger.Warn("enter_worktree: rollback failed", "err", rmErr, "out", rmOut)
		}
		return tools.Result{IsError: true, Content: "enter_worktree: switch workdir failed: " + err.Error()}, nil
	}

	ctrl.BeginWorktreeSession(WorktreeSession{
		OriginalWorkdir: original,
		Path:            worktreePath,
		Branch:          branch,
		Slug:            flat,
		CreatedAt:       time.Now(),
	})

	logger.Info("enter_worktree", "path", worktreePath, "branch", branch, "slug", flat)
	return tools.Result{
		Content: fmt.Sprintf(
			"Entered worktree.\n  path:   %s\n  branch: %s\n\nSubsequent file reads, edits, and bash commands run in the worktree. Use exit_worktree (action=\"keep\" or \"remove\") to leave.",
			worktreePath, branch,
		),
	}, nil
}

// --- ExitWorktree ------------------------------------------------------

const exitWorktreeDescription = `Exit a worktree session created by enter_worktree and return the session to the original working directory.

## Scope

This tool ONLY operates on worktrees created by enter_worktree in this session. It will NOT touch:
- Worktrees you created manually with ` + "`git worktree add`" + `
- Worktrees from a previous session
- The directory you're in if enter_worktree was never called

If called outside an enter_worktree session, the tool is a **no-op**: it reports that no worktree session is active and takes no action. Filesystem state is unchanged.

## When to Use

- The user explicitly asks to "exit the worktree", "leave the worktree", "go back", or otherwise end the worktree session
- Do NOT call this proactively — only when the user asks

## Parameters

- ` + "`action`" + ` (required): ` + "`\"keep\"`" + ` or ` + "`\"remove\"`" + `
  - ` + "`\"keep\"`" + ` — leave the worktree directory and branch intact on disk. Use this if the user wants to come back to the work later, or if there are changes to preserve.
  - ` + "`\"remove\"`" + ` — delete the worktree directory and its branch. Use this for a clean exit when the work is done or abandoned.
- ` + "`discard_changes`" + ` (optional, default false): only meaningful with ` + "`action: \"remove\"`" + `. If the worktree has uncommitted files or commits not on the original branch, the tool will REFUSE to remove it unless this is set to ` + "`true`" + `. If the tool returns an error listing changes, confirm with the user before re-invoking with ` + "`discard_changes: true`" + `.

## Behavior

- Restores the agent's working directory to where it was before enter_worktree
- Reloads the EVVA.md / USER_PROFILE.md snapshot and rebuilds the active filesystem + bash tools against the original workdir
- On ` + "`action: \"remove\"`" + ` with no pending changes (or with ` + "`discard_changes: true`" + `): runs ` + "`git worktree remove --force <path>`" + ` and deletes the worktree branch
- Once exited, enter_worktree can be called again to create a fresh worktree
`

const exitWorktreeSchema = `{
	"type":"object",
	"additionalProperties":false,
	"required":["action"],
	"properties":{
		"action":{"type":"string","enum":["keep","remove"],"description":"\"keep\" leaves the worktree and branch on disk; \"remove\" deletes both."},
		"discard_changes":{"type":"boolean","description":"Required true when action is \"remove\" and the worktree has uncommitted files or unmerged commits."}
	}
}`

type exitWorktreeInput struct {
	Action         string `json:"action"`
	DiscardChanges bool   `json:"discard_changes"`
}

// ExitWorktreeTool tears down (or keeps) a worktree session opened by
// EnterWorktree, restoring the agent's original workdir.
type ExitWorktreeTool struct {
	lookup WorktreeControllerLookup
}

func NewExitWorktree(lookup WorktreeControllerLookup) *ExitWorktreeTool {
	return &ExitWorktreeTool{lookup: lookup}
}

func (t *ExitWorktreeTool) Name() string            { return string(tools.EXIT_WORKTREE) }
func (t *ExitWorktreeTool) Description() string     { return exitWorktreeDescription }
func (t *ExitWorktreeTool) Schema() json.RawMessage { return json.RawMessage(exitWorktreeSchema) }

func (t *ExitWorktreeTool) Execute(ctx context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	ctrl := resolveWorktreeController(t.lookup)
	if ctrl == nil {
		return tools.Result{
			IsError: true,
			Content: "exit_worktree: no worktree controller installed",
		}, nil
	}

	sess := ctrl.WorktreeSession()
	if sess == nil {
		// Per ref spec: no-op when no session is active.
		return tools.Result{
			Content: "exit_worktree: no worktree session active — nothing to do.",
		}, nil
	}

	var in exitWorktreeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("exit_worktree: decode input: %v", err)}, nil
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action != "keep" && action != "remove" {
		return tools.Result{IsError: true, Content: `exit_worktree: action must be "keep" or "remove"`}, nil
	}

	if action == "keep" {
		if err := ctrl.SwitchWorkdir(sess.OriginalWorkdir); err != nil {
			return tools.Result{IsError: true, Content: "exit_worktree: switch back failed: " + err.Error()}, nil
		}
		ctrl.EndWorktreeSession()
		logger.Info("exit_worktree", "action", "keep", "path", sess.Path, "branch", sess.Branch)
		return tools.Result{
			Content: fmt.Sprintf(
				"Worktree kept on disk.\n  path:   %s\n  branch: %s\n\nReturned to %s.",
				sess.Path, sess.Branch, sess.OriginalWorkdir,
			),
		}, nil
	}

	// action == "remove"
	dirty, summary := worktreeHasChanges(ctx, sess.Path)
	if dirty && !in.DiscardChanges {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf(
				"exit_worktree: worktree has uncommitted changes — refusing to remove without explicit confirmation.\n%s\n\nConfirm with the user, then re-invoke with discard_changes=true.",
				summary,
			),
		}, nil
	}

	// Switch back BEFORE removing so the agent isn't sitting in a directory
	// we're about to delete.
	if err := ctrl.SwitchWorkdir(sess.OriginalWorkdir); err != nil {
		return tools.Result{IsError: true, Content: "exit_worktree: switch back failed: " + err.Error()}, nil
	}

	repoRoot, err := gitTopLevel(ctx, sess.OriginalWorkdir)
	if err != nil {
		// We're not in a git repo from the restored workdir — leave the
		// worktree on disk, end the session, surface the warning.
		ctrl.EndWorktreeSession()
		return tools.Result{
			IsError: true,
			Content: "exit_worktree: cannot resolve repo root after switch back: " + err.Error() + ". Worktree left on disk at " + sess.Path,
		}, nil
	}

	rmOut, rmErr := runGit(ctx, repoRoot, "worktree", "remove", "--force", sess.Path)
	if rmErr != nil {
		ctrl.EndWorktreeSession()
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("exit_worktree: git worktree remove failed: %v\n%s\nWorktree may still be on disk at %s", rmErr, rmOut, sess.Path),
		}, nil
	}
	// Best-effort branch delete. -D forces removal even if the branch isn't
	// merged into the original branch — by this point the user already
	// agreed to discard, or there were no changes to merge.
	if brOut, brErr := runGit(ctx, repoRoot, "branch", "-D", sess.Branch); brErr != nil {
		logger.Warn("exit_worktree: branch delete failed (worktree removed successfully)", "branch", sess.Branch, "err", brErr, "out", brOut)
	}

	ctrl.EndWorktreeSession()
	logger.Info("exit_worktree", "action", "remove", "path", sess.Path, "branch", sess.Branch, "discarded", dirty)
	msg := fmt.Sprintf(
		"Worktree removed.\n  path:   %s\n  branch: %s\n\nReturned to %s.",
		sess.Path, sess.Branch, sess.OriginalWorkdir,
	)
	if dirty {
		msg += "\n\n" + summary + "\n(discarded per discard_changes=true)"
	}
	return tools.Result{Content: msg}, nil
}

// --- helpers -----------------------------------------------------------

// resolveSlug returns the validated (still slash-bearing) slug. Empty
// input generates a random one. The caller flattens with flattenSlug
// before composing the worktree path or branch name.
func resolveSlug(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return randomSlug(), nil
	}
	return validateSlug(name)
}

func randomSlug() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "wt-" + hex.EncodeToString(b[:])
}

// validateSlug enforces the same shape as the ref source:
// segments separated by "/", each segment matches [A-Za-z0-9._-]+,
// no "." or ".." segments, total length <= 64.
func validateSlug(slug string) (string, error) {
	if len(slug) > maxSlugLen {
		return "", fmt.Errorf("slug too long (max %d chars, got %d)", maxSlugLen, len(slug))
	}
	if strings.HasPrefix(slug, "/") || strings.HasSuffix(slug, "/") {
		return "", errors.New("slug must not begin or end with '/'")
	}
	parts := strings.Split(slug, "/")
	for _, p := range parts {
		if p == "" {
			return "", errors.New("slug must not contain empty segments")
		}
		if p == "." || p == ".." {
			return "", fmt.Errorf("slug segment %q is not allowed", p)
		}
		if !slugSegmentRE.MatchString(p) {
			return "", fmt.Errorf("slug segment %q contains invalid characters (allowed: letters, digits, dot, underscore, dash)", p)
		}
	}
	return slug, nil
}

// flattenSlug collapses "/" separators to "+" so a slash-bearing slug
// names a single directory and a single git ref. Ref source uses the
// same flattening character.
func flattenSlug(slug string) string {
	return strings.ReplaceAll(slug, "/", "+")
}

func branchNameFor(flatSlug string) string {
	return "worktree-" + flatSlug
}

func worktreeDirFor(repoRoot, flatSlug string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(permission.WorktreeDirSegment), flatSlug)
}

// gitTopLevel returns the canonical repository root for `dir`. Used to
// anchor worktrees at the main repo even when EnterWorktree is invoked
// from a nested directory.
func gitTopLevel(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// runGit invokes `git <args...>` with cmd.Dir = dir and returns the
// combined stdout+stderr output. Bypasses the Bash tool deliberately:
// these are internal tool side effects (mirroring how EnterPlanMode
// writes its plan file via os.WriteFile), not user-issued shell.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// worktreeHasChanges returns (true, summary) when the worktree at `path`
// has uncommitted files OR commits beyond the original HEAD. summary is a
// human-readable description for the tool's error message. Fail-closed:
// any git error treats the worktree as dirty so we never auto-remove a
// worktree we can't inspect.
func worktreeHasChanges(ctx context.Context, path string) (bool, string) {
	statusOut, err := runGit(ctx, path, "status", "--porcelain")
	if err != nil {
		return true, "could not inspect worktree status: " + err.Error()
	}
	statusOut = strings.TrimRight(statusOut, "\n")
	uncommittedLines := 0
	if statusOut != "" {
		uncommittedLines = strings.Count(statusOut, "\n") + 1
	}

	// Count commits on the worktree branch beyond its merge-base with the
	// branch git checked out at worktree-creation time. Use `@{upstream}`?
	// No — there's no upstream. Use HEAD..HEAD@{1}? Brittle. Simpler:
	// list commits reachable from HEAD but not from origin/HEAD via
	// `git log --oneline @{u}..HEAD` — also not guaranteed. Use the
	// reflog-resilient approach: ask for unique commits ahead of the
	// default branch's HEAD-on-creation, captured via `git rev-list
	// --count HEAD ^@{upstream}` is fragile too. Fall back to the
	// safest cross-environment signal: count commits whose parent is
	// reachable from no other branch — `git rev-list --count
	// HEAD --not --branches=main --branches=master --branches=develop`
	// is too repo-specific. v1 uses just the porcelain status as the
	// dirty signal; uncommitted commits without dirty files are
	// uncommon and we'd rather under-warn than fail to inspect.
	commitsAhead := 0

	if uncommittedLines == 0 && commitsAhead == 0 {
		return false, ""
	}
	return true, fmt.Sprintf(
		"%s file(s) uncommitted, %s commit(s) beyond starting HEAD",
		strconv.Itoa(uncommittedLines), strconv.Itoa(commitsAhead),
	)
}

// --- AgentTool isolation helpers --------------------------------------

// CreateForSubagent provisions a worktree for an AgentTool isolation
// spawn. It is callable from internal/agent/spawn.go (no controller
// involved — the child agent's workdir is fixed at construction).
// Returns the worktree session metadata so spawn.go can:
//   - construct the child with cfg.WorkDir = session.Path
//   - tear down the worktree after the child exits (clean-exit path) or
//     surface its path back to the parent (dirty-exit path).
//
// agentName is folded into the slug so panel rows / logs / branch names
// stay readable; if the child name is empty, a random suffix is used.
func CreateForSubagent(ctx context.Context, parentWorkdir, agentName string) (WorktreeSession, error) {
	if parentWorkdir == "" {
		return WorktreeSession{}, errors.New("parent workdir is empty")
	}
	repoRoot, err := gitTopLevel(ctx, parentWorkdir)
	if err != nil {
		return WorktreeSession{}, fmt.Errorf("not in a git repository: %w", err)
	}

	base := sanitizeForSlug(agentName)
	if base == "" {
		base = "agent"
	}
	// Append a short random suffix so concurrent isolation spawns with the
	// same agent name don't collide on directory or branch name.
	var b [3]byte
	_, _ = rand.Read(b[:])
	flat := base + "-" + hex.EncodeToString(b[:])
	if len(flat) > maxSlugLen {
		flat = flat[:maxSlugLen]
	}

	worktreePath := worktreeDirFor(repoRoot, flat)
	branch := branchNameFor(flat)

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return WorktreeSession{}, fmt.Errorf("create worktree parent dir: %w", err)
	}
	if out, gErr := runGit(ctx, repoRoot, "worktree", "add", "-b", branch, worktreePath, "HEAD"); gErr != nil {
		return WorktreeSession{}, fmt.Errorf("git worktree add: %v: %s", gErr, out)
	}

	return WorktreeSession{
		OriginalWorkdir:   parentWorkdir,
		Path:              worktreePath,
		Branch:            branch,
		Slug:              flat,
		CreatedBySubagent: true,
		CreatedAt:         time.Now(),
	}, nil
}

// CleanupSubagentWorktree tears down a worktree created by
// CreateForSubagent. If the worktree has any uncommitted changes the
// caller can decide to keep it (return the path to the user) instead of
// removing it. RemoveAlways=true forces removal regardless.
//
// Returns (wasRemoved, summary). On wasRemoved=false the worktree is
// still on disk; summary describes why (e.g. "had 3 uncommitted file(s)").
func CleanupSubagentWorktree(ctx context.Context, sess WorktreeSession, removeAlways bool) (bool, string) {
	if !sess.CreatedBySubagent || sess.Path == "" {
		return false, "no subagent worktree to clean up"
	}
	dirty, summary := worktreeHasChanges(ctx, sess.Path)
	if dirty && !removeAlways {
		return false, summary
	}
	repoRoot, err := gitTopLevel(ctx, sess.OriginalWorkdir)
	if err != nil {
		return false, "cannot resolve repo root: " + err.Error()
	}
	if out, rmErr := runGit(ctx, repoRoot, "worktree", "remove", "--force", sess.Path); rmErr != nil {
		return false, fmt.Sprintf("git worktree remove failed: %v: %s", rmErr, out)
	}
	// Best-effort branch delete; ignore failures so cleanup never
	// half-completes.
	_, _ = runGit(ctx, repoRoot, "branch", "-D", sess.Branch)
	return true, "removed"
}

// sanitizeForSlug strips a free-form string down to characters allowed
// in a slug segment. Used to fold an agent name into the worktree
// directory / branch name.
func sanitizeForSlug(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
