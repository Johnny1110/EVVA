package mode

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnny1110/evva/pkg/tools/daemon"
)

// --- test helpers (build on newFakeRepo / fakeWorktreeController) ------

func gitRunT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// addWorktreeWithCommit provisions an evva-managed worktree off HEAD, writes
// file=content in it, and commits. Returns the session (Path/Branch/Original).
func addWorktreeWithCommit(t *testing.T, repo, name, file, content string) WorktreeSession {
	t.Helper()
	sess, err := CreateForSubagent(context.Background(), repo, name)
	if err != nil {
		t.Fatalf("CreateForSubagent(%q): %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(sess.Path, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	gitRunT(t, sess.Path, "add", file)
	gitRunT(t, sess.Path, "commit", "-q", "-m", "work: "+file)
	return sess
}

func mergeInput(branch string) json.RawMessage {
	if branch == "" {
		return json.RawMessage(`{"action":"merge"}`)
	}
	return json.RawMessage(fmt.Sprintf(`{"action":"merge","branch":%q}`, branch))
}

// --- merge: clean / active session (A2) -------------------------------

func TestMerge_CleanActiveSession(t *testing.T) {
	repo := newFakeRepo(t)
	sess := addWorktreeWithCommit(t, repo, "alpha", "alpha.txt", "alpha\n")

	ctrl := &fakeWorktreeController{workdir: sess.Path}
	ctrl.BeginWorktreeSession(sess)
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, err := tool.Execute(context.Background(), ctrl.Logger(), mergeInput(""))
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("clean merge should succeed; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "Merged") || !strings.Contains(res.Content, "1 commit") {
		t.Errorf("result should report the merge; got %q", res.Content)
	}
	// File integrated into the base checkout.
	if _, err := os.Stat(filepath.Join(repo, "alpha.txt")); err != nil {
		t.Errorf("merged file should exist in base: %v", err)
	}
	// Worktree torn down, branch deleted, session ended, workdir restored.
	if _, err := os.Stat(sess.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir should be removed; stat err=%v", err)
	}
	if b := strings.TrimSpace(gitRunT(t, repo, "branch", "--list", sess.Branch)); b != "" {
		t.Errorf("branch should be deleted; got %q", b)
	}
	if ctrl.WorktreeSession() != nil {
		t.Errorf("session should be ended after merge")
	}
	if ctrl.workdir != repo {
		t.Errorf("workdir should be restored to base; got %q", ctrl.workdir)
	}
}

// --- merge: targeted subagent worktree, no active session (A7) --------

func TestMerge_TargetedSubagentWorktree(t *testing.T) {
	repo := newFakeRepo(t)
	sess := addWorktreeWithCommit(t, repo, "beta", "beta.txt", "beta\n")

	// No active session — the lead merges a finished worker's worktree by branch.
	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), mergeInput(sess.Branch))
	if res.IsError {
		t.Fatalf("targeted merge should succeed; got %q", res.Content)
	}
	if _, err := os.Stat(filepath.Join(repo, "beta.txt")); err != nil {
		t.Errorf("merged file should exist in base: %v", err)
	}
	if _, err := os.Stat(sess.Path); !os.IsNotExist(err) {
		t.Errorf("worktree dir should be removed after targeted merge; stat err=%v", err)
	}
}

// --- merge: conflict aborts and reports, never half-applies (A3) ------

func TestMerge_ConflictAbortsAndReports(t *testing.T) {
	repo := newFakeRepo(t)
	// Worker rewrites README; base rewrites the same line differently.
	sess := addWorktreeWithCommit(t, repo, "conf", "README", "from-worktree\n")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("from-main\n"), 0o644); err != nil {
		t.Fatalf("write base README: %v", err)
	}
	gitRunT(t, repo, "add", "README")
	gitRunT(t, repo, "commit", "-q", "-m", "base change")

	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), mergeInput(sess.Branch))
	// A conflict is an actionable outcome, not a tool error.
	if res.IsError {
		t.Fatalf("conflict should not be an IsError result; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "CONFLICT") || !strings.Contains(res.Content, "README") {
		t.Errorf("result should name the conflicted path; got %q", res.Content)
	}
	// Worktree intact; base unchanged; no half-merged state.
	if _, err := os.Stat(sess.Path); err != nil {
		t.Errorf("worktree should survive a conflict; stat err=%v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "README")); string(b) != "from-main\n" {
		t.Errorf("base must be unchanged after aborted merge; got %q", b)
	}
	if st := strings.TrimSpace(gitRunT(t, repo, "status", "--porcelain", "--untracked-files=no")); st != "" {
		t.Errorf("base should be clean after merge --abort; got %q", st)
	}
}

// --- merge: refuses an unclean source (A4) ----------------------------

func TestMerge_RefusesUncleanSource(t *testing.T) {
	repo := newFakeRepo(t)
	sess := addWorktreeWithCommit(t, repo, "dirty", "x.txt", "x\n")
	// Leave an uncommitted file in the worktree.
	if err := os.WriteFile(filepath.Join(sess.Path, "scratch"), []byte("noise\n"), 0o644); err != nil {
		t.Fatalf("write scratch: %v", err)
	}

	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), mergeInput(sess.Branch))
	if !res.IsError || !strings.Contains(res.Content, "uncommitted") {
		t.Errorf("expected refusal for unclean source; got %q", res.Content)
	}
	if _, err := os.Stat(sess.Path); err != nil {
		t.Errorf("worktree should be left intact on refusal; stat err=%v", err)
	}
}

// --- merge: nothing to integrate is a no-op (A5) ----------------------

func TestMerge_NothingToIntegrate(t *testing.T) {
	repo := newFakeRepo(t)
	sess, err := CreateForSubagent(context.Background(), repo, "empty")
	if err != nil {
		t.Fatalf("CreateForSubagent: %v", err)
	}

	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), mergeInput(sess.Branch))
	if res.IsError {
		t.Fatalf("empty merge should be a no-op, not an error; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "No changes to integrate") {
		t.Errorf("expected no-op message; got %q", res.Content)
	}
	// Worktree left untouched.
	if _, err := os.Stat(sess.Path); err != nil {
		t.Errorf("worktree should be left untouched; stat err=%v", err)
	}
}

// --- merge: input-shape guards ----------------------------------------

func TestMerge_NoSessionNoBranch(t *testing.T) {
	repo := newFakeRepo(t)
	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), mergeInput(""))
	if !res.IsError || !strings.Contains(res.Content, "no active worktree session") {
		t.Errorf("expected no-session error; got %q", res.Content)
	}
}

func TestMerge_UnknownBranch(t *testing.T) {
	repo := newFakeRepo(t)
	_ = addWorktreeWithCommit(t, repo, "real", "r.txt", "r\n")

	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewExitWorktree(func() WorktreeController { return ctrl })

	res, _ := tool.Execute(context.Background(), ctrl.Logger(),
		mergeInput("worktree-does-not-exist"))
	if !res.IsError || !strings.Contains(res.Content, "no live worktree") {
		t.Errorf("expected unknown-branch error; got %q", res.Content)
	}
}

// --- worktree_list ----------------------------------------------------

func TestWorktreeList_NilController(t *testing.T) {
	tool := NewList(func() WorktreeController { return nil }, nil)
	res, _ := tool.Execute(context.Background(), slog.Default(), nil)
	if !res.IsError || !strings.Contains(res.Content, "no worktree controller") {
		t.Errorf("nil controller should error; got %q", res.Content)
	}
}

func TestWorktreeList_EmptyIsClean(t *testing.T) {
	repo := newFakeRepo(t)
	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewList(func() WorktreeController { return ctrl }, nil)

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), nil)
	if res.IsError {
		t.Fatalf("empty list must not error; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "no worktrees") {
		t.Errorf("expected 'no worktrees'; got %q", res.Content)
	}
}

func TestWorktreeList_EnumeratesAheadAndDirty(t *testing.T) {
	repo := newFakeRepo(t)
	ahead := addWorktreeWithCommit(t, repo, "gamma", "g.txt", "g\n") // 1 ahead, clean
	dirty, err := CreateForSubagent(context.Background(), repo, "delta")
	if err != nil {
		t.Fatalf("CreateForSubagent: %v", err)
	}
	if werr := os.WriteFile(filepath.Join(dirty.Path, "scratch"), []byte("noise\n"), 0o644); werr != nil {
		t.Fatalf("write scratch: %v", werr)
	}

	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewList(func() WorktreeController { return ctrl }, nil)

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), nil)
	if res.IsError {
		t.Fatalf("list errored: %q", res.Content)
	}
	if !strings.Contains(res.Content, "2 worktree(s)") {
		t.Errorf("expected two worktrees; got %q", res.Content)
	}
	if !strings.Contains(res.Content, ahead.Branch) || !strings.Contains(res.Content, "ahead=1") {
		t.Errorf("ahead worktree row missing ahead=1; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "dirty") {
		t.Errorf("dirty worktree should be flagged; got %q", res.Content)
	}
}

func TestWorktreeList_DaemonCrossRef(t *testing.T) {
	repo := newFakeRepo(t)
	sess := addWorktreeWithCommit(t, repo, "owned", "o.txt", "o\n")

	st := daemon.NewState(nil)
	st.Register(fakeDaemon{snap: daemon.DaemonSnapshot{
		ID:     "atest1234",
		Kind:   daemon.KindLocalAgent,
		Status: daemon.StatusRunning,
		Metadata: daemon.LocalAgentMeta{
			AgentType:    "explore",
			WorktreePath: sess.Path,
		},
	}})

	ctrl := &fakeWorktreeController{workdir: repo}
	tool := NewList(func() WorktreeController { return ctrl }, st)

	res, _ := tool.Execute(context.Background(), ctrl.Logger(), nil)
	if res.IsError {
		t.Fatalf("list errored: %q", res.Content)
	}
	if !strings.Contains(res.Content, "owner=atest1234(running)") {
		t.Errorf("expected daemon cross-ref for the owned worktree; got %q", res.Content)
	}
}

// TestCleanupSubagentWorktree_PreservesCommitted pins the A7 enabler: a
// finished subagent that COMMITTED its slice (clean tree, commits ahead of the
// base) must be preserved, not auto-removed — otherwise there's nothing for
// the lead to review with worktree_list and merge.
func TestCleanupSubagentWorktree_PreservesCommitted(t *testing.T) {
	repo := newFakeRepo(t)
	ctx := context.Background()
	sess := addWorktreeWithCommit(t, repo, "committed", "done.txt", "done\n")

	removed, summary := CleanupSubagentWorktree(ctx, sess, false)
	if removed {
		t.Errorf("committed worktree should be preserved for later merge; summary=%q", summary)
	}
	if _, err := os.Stat(sess.Path); err != nil {
		t.Errorf("preserved worktree should still exist; stat err=%v", err)
	}
}

// fakeDaemon is a minimal daemon.Daemon for the cross-ref test.
type fakeDaemon struct{ snap daemon.DaemonSnapshot }

func (f fakeDaemon) Snapshot() daemon.DaemonSnapshot { return f.snap }
func (f fakeDaemon) Kill(context.Context) error      { return nil }
func (f fakeDaemon) Output() string                  { return "" }
