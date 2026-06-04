// Package service is the process-singleton swarm host: the 127.0.0.1:8888
// HTTP/WS server that fronts one or more isolated SwarmSpaces.
//
// Service is the multi-space container (design §3.1, invariant #2): a registry
// of fully-isolated spaces, each with its own store/bus/roster/agents. It fans
// every space's tagged event stream out to the right WebSocket (via
// webapi.Hub) and routes inbound browser commands to the right
// Controller/Supervisor. Multi-space is native — there is no single-space
// hardcode.
//
// SPRD-1-9 layers daemonization (pidfile/log under ~/.evva/service/) and the
// `evva swarm .` CLI on top of the Register/StopSpace surface exposed here.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/johnny1110/evva/internal/swarm"
	"github.com/johnny1110/evva/internal/swarm/agentdef"
	"github.com/johnny1110/evva/internal/swarm/store"
	swarmtools "github.com/johnny1110/evva/internal/swarm/tools"
	"github.com/johnny1110/evva/internal/swarm/webapi"
	"github.com/johnny1110/evva/pkg/common"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/ui"
	"github.com/johnny1110/evva/web"
)

// DefaultAddr is the loopback bind the service uses unless overridden. Binding
// to 127.0.0.1 (not 0.0.0.0) is the security baseline (invariant #6): agents
// run shell and edit files, so the workstation is RCE-equivalent and must not
// be reachable off-box by default.
const DefaultAddr = "127.0.0.1:8888"

// manifestFile is the per-workdir swarm declaration `evva swarm .` reads.
const manifestFile = "evva-swarm.yml"

// Service is the swarm host. One per process.
type Service struct {
	addr  string
	token string
	log   *slog.Logger

	hub *webapi.Hub
	srv *http.Server

	// rootCtx is the lifetime of the whole host; every space's supervisor and
	// event pump run as its children, so Stop cancels all of them at once.
	rootCtx    context.Context
	rootCancel context.CancelFunc

	mu     sync.RWMutex
	ln     net.Listener // bound listener, nil until Listen
	spaces map[string]*spaceEntry

	// stateDir, when set, is where the set of registered workdirs is persisted
	// (spaces.json) so Reconcile can rebuild every space after a restart. Empty
	// disables persistence (tests that register stub spaces in-memory).
	stateDir string

	// loadConfig builds the per-space *config.Config for a workdir. Overridable
	// in tests to inject a stub LLM provider without touching disk/env.
	loadConfig func(workdir string) (*config.Config, error)
}

// spacesFileName holds the registered workdirs across restarts (SPRD-1-11).
const spacesFileName = "spaces.json"

// spaceEntry holds one live space plus the handles needed to tear it down
// independently of its siblings.
type spaceEntry struct {
	space    *swarm.SwarmSpace
	super    *swarm.Supervisor
	cancel   context.CancelFunc // stops the supervisor's run loops + timer tick
	stopPump chan struct{}      // closed after Shutdown so the pump drains then exits
}

// New builds the host bound (logically) to addr. An empty addr uses
// DefaultAddr. A session token is minted now and required on every /api + /ws
// request. Call Listen then Serve (Serve calls Listen if you skip it).
func New(addr string) *Service {
	if addr == "" {
		addr = DefaultAddr
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	s := &Service{
		addr:       addr,
		token:      common.GenUUID(),
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		hub:        webapi.NewHub(),
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		spaces:     make(map[string]*spaceEntry),
		loadConfig: defaultLoadConfig,
	}

	var spa fs.FS
	if sub, err := fs.Sub(web.Dist, "dist"); err == nil {
		spa = sub
	}
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           webapi.NewRouter(s, s.hub, spa),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func defaultLoadConfig(workdir string) (*config.Config, error) {
	return config.Load(config.LoadOptions{WorkDir: workdir})
}

// Token is the session token clients must present. Printed to the terminal on
// start so a local user can authenticate the browser.
func (s *Service) Token() string { return s.token }

// SetLogger swaps the host's structured logger (SPRD-1-9 wires the daemon log).
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetStateDir enables restart persistence: the set of registered workdirs is
// written under dir/spaces.json so Reconcile can rebuild every space after a
// process death (SPRD-1-11). Call before Reconcile / the first Register.
func (s *Service) SetStateDir(dir string) { s.stateDir = dir }

// Listen binds the configured address without serving. Exposed so callers
// (and tests using a :0 ephemeral port) can read Addr() before Serve blocks.
// Idempotent: a second call is a no-op once bound.
func (s *Service) Listen() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.addr = ln.Addr().String()
	return nil
}

// Serve serves until ctx is cancelled, then gracefully drains and tears down
// every registered space. It binds first if Listen was not already called. A
// context-triggered shutdown returns nil; any other server error is returned.
func (s *Service) Serve(ctx context.Context) error {
	if err := s.Listen(); err != nil {
		return err
	}

	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(s.ln) }()

	select {
	case <-ctx.Done():
		return s.Stop()
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Stop tears down every space and the HTTP server — a graceful process
// shutdown. Crucially it does NOT rewrite spaces.json: the registered set is
// preserved so the next start reconciles the same spaces back (SPRD-1-11). Use
// StopSpace to deliberately drop one from the reconcile set.
func (s *Service) Stop() error {
	s.mu.Lock()
	ents := make([]*spaceEntry, 0, len(s.spaces))
	for id, ent := range s.spaces {
		ents = append(ents, ent)
		delete(s.spaces, id)
	}
	s.mu.Unlock()
	for _, ent := range ents {
		teardownSpace(ent)
	}
	s.rootCancel()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.srv.Shutdown(shutCtx)
}

// teardownSpace stops a space's supervisor, shuts its agents + store down, and
// drains then stops its event pump. Shared by Stop (whole host) and StopSpace
// (one space).
func teardownSpace(ent *spaceEntry) {
	ent.cancel()         // stop run loops + timer (no new runs)
	ent.space.Shutdown() // cancel agents + close store; trailing events still buffered
	close(ent.stopPump)  // pump does a final drain, then exits
}

// Addr returns the address the service is bound to. Before Listen it is the
// configured address; after Listen it is the resolved one (the concrete port
// when :0 was requested).
func (s *Service) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// Register reads <workdir>/evva-swarm.yml, builds its agents, and brings the
// space up as a new isolated member of the registry. Returns the generated
// space id. This is the production path the `evva swarm .` CLI (SPRD-1-9) calls.
func (s *Service) Register(workdir string) (string, error) {
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("swarm: resolve workdir %q: %w", workdir, err)
	}
	cfg, err := s.loadConfig(abs)
	if err != nil {
		return "", fmt.Errorf("swarm: load config for %q: %w", abs, err)
	}
	m, err := agentdef.LoadManifest(filepath.Join(abs, manifestFile))
	if err != nil {
		return "", err
	}
	loaded, warnings, err := agentdef.NewLoader().BuildAll(abs, m)
	if err != nil {
		return "", err
	}
	for _, w := range warnings {
		s.log.Warn("swarm: agent load warning", "agent", w.Agent, "msg", w.Msg)
	}
	return s.register(m, loaded, cfg)
}

// register is the shared bring-up core: assemble the space, start its
// supervisor and event pump under a fresh child context, and add it to the
// registry. Split out so tests can register a stub-LLM space without disk/env.
func (s *Service) register(m agentdef.Manifest, loaded []agentdef.Loaded, cfg *config.Config) (string, error) {
	id := common.GenUUID()
	sp, err := swarm.NewSpace(id, m, loaded, swarmtools.Set{}, cfg)
	if err != nil {
		return "", err
	}

	// Restore any prior on-disk state (transcripts, unread mail, frozen
	// membership) before the supervisor starts the run loops — a no-op for a
	// fresh workdir, the restart-resume path for one that died (SPRD-1-11 §6.2).
	sp.Reload()

	super := swarm.NewSupervisor(sp)
	spaceCtx, cancel := context.WithCancel(s.rootCtx)
	super.Start(spaceCtx)

	stopPump := make(chan struct{})
	s.mu.Lock()
	s.spaces[id] = &spaceEntry{space: sp, super: super, cancel: cancel, stopPump: stopPump}
	s.mu.Unlock()

	go s.pump(sp, stopPump)
	s.persistSpaces()
	s.log.Info("swarm: space registered", "id", id, "name", m.Name, "members", len(loaded))
	return id, nil
}

// StopSpace tears one space down without touching the others (AC#2 isolation):
// stop its supervisor, shut its agents + store down, then drain and stop the
// pump. An unknown id is an error.
func (s *Service) StopSpace(id string) error {
	s.mu.Lock()
	ent, ok := s.spaces[id]
	if ok {
		delete(s.spaces, id)
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("swarm: unknown space %q", id)
	}

	teardownSpace(ent)
	s.persistSpaces() // a deliberate stop drops it from the reconcile set
	s.log.Info("swarm: space stopped", "id", id)
	return nil
}

// spacesFile is the persisted reconcile manifest path, or "" when persistence
// is disabled (no state dir).
func (s *Service) spacesFile() string {
	if s.stateDir == "" {
		return ""
	}
	return filepath.Join(s.stateDir, spacesFileName)
}

// persistedSpaces is the on-disk shape of spaces.json.
type persistedSpaces struct {
	Workdirs []string `json:"workdirs"`
}

// persistSpaces snapshots every live space's workdir to spaces.json so a later
// Reconcile rebuilds exactly this set. Best-effort: a write failure costs the
// post-restart auto-rebuild, never live correctness.
func (s *Service) persistSpaces() {
	path := s.spacesFile()
	if path == "" {
		return
	}
	s.mu.RLock()
	seen := map[string]bool{}
	var dirs []string
	for _, ent := range s.spaces {
		wd := ent.space.Workdir
		if wd != "" && !seen[wd] {
			seen[wd] = true
			dirs = append(dirs, wd)
		}
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(persistedSpaces{Workdirs: dirs}, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.log.Warn("swarm: persist spaces.json", "err", err)
	}
}

// Reconcile rebuilds every space recorded in spaces.json — the boot path after
// a process death (SPRD-1-11). A per-space failure is logged and skipped so one
// bad workdir never blocks the rest; the first error is returned for the caller
// to surface. A no-op when persistence is disabled or the file is absent.
func (s *Service) Reconcile() error {
	path := s.spacesFile()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("swarm: read spaces.json: %w", err)
	}
	var ps persistedSpaces
	if err := json.Unmarshal(b, &ps); err != nil {
		return fmt.Errorf("swarm: parse spaces.json: %w", err)
	}

	var firstErr error
	for _, wd := range ps.Workdirs {
		id, err := s.Register(wd)
		if err != nil {
			s.log.Warn("swarm: reconcile space failed", "workdir", wd, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		s.log.Info("swarm: reconciled space", "workdir", wd, "id", id)
	}
	return firstErr
}

// pump drains one space's event stream into the hub for the life of the space.
// On stop it makes a final non-blocking pass so events emitted during Shutdown
// (e.g. a run-cancelled) still reach any connected browser before it exits.
func (s *Service) pump(sp *swarm.SwarmSpace, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			for {
				select {
				case e := <-sp.Events():
					s.publish(e)
				default:
					return
				}
			}
		case e := <-sp.Events():
			s.publish(e)
		}
	}
}

// publish marshals one spaced event and fans it out by (spaceID, AgentID).
func (s *Service) publish(e swarm.SpacedEvent) {
	payload, err := json.Marshal(wireEvent{SpaceID: e.SpaceID, Event: e.Event})
	if err != nil {
		return
	}
	s.hub.Publish(e.SpaceID, e.Event.AgentID, payload)
}

// wireEvent is the JSON envelope pushed over the WebSocket: the raw agent
// event plus the space it belongs to (the AgentID is already on the event).
type wireEvent struct {
	SpaceID string `json:"spaceId"`
	Event   any    `json:"event"`
}


func (s *Service) entry(id string) (*spaceEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ent, ok := s.spaces[id]
	return ent, ok
}

// --- webapi.Backend implementation ---------------------------------------

// HasSpace reports whether a space id is registered.
func (s *Service) HasSpace(id string) bool {
	_, ok := s.entry(id)
	return ok
}

// ListSpaces returns a snapshot of every registered space (GET /api/swarms).
func (s *Service) ListSpaces() []webapi.SpaceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]webapi.SpaceInfo, 0, len(s.spaces))
	for id, ent := range s.spaces {
		out = append(out, webapi.SpaceInfo{
			ID:      id,
			Name:    ent.space.Name,
			Workdir: ent.space.Workdir,
			Members: len(ent.space.Roster.Snapshot()),
		})
	}
	return out
}

// Spaces satisfies webapi.Backend.
func (s *Service) Spaces() []webapi.SpaceInfo { return s.ListSpaces() }

func (s *Service) Roster(id string) ([]webapi.MemberInfo, bool) {
	ent, ok := s.entry(id)
	if !ok {
		return nil, false
	}
	views := ent.space.Roster.Snapshot()
	out := make([]webapi.MemberInfo, 0, len(views))
	for _, v := range views {
		var agentID string
		if ctl, ok := ent.space.Roster.Controller(v.Name); ok {
			agentID = ctl.AgentID()
		}
		out = append(out, webapi.MemberInfo{
			Name:        v.Name,
			AgentID:     agentID,
			Role:        string(v.Role),
			Membership:  string(v.Membership),
			Run:         string(v.Run),
			CurrentTask: v.CurrentTask,
			WhenToUse:   v.WhenToUse,
		})
	}
	return out, true
}

func (s *Service) Tasks(id string) ([]webapi.TaskInfo, bool) {
	ent, ok := s.entry(id)
	if !ok {
		return nil, false
	}
	tasks, err := ent.space.Store.ListTasks(store.TaskFilter{})
	if err != nil {
		s.log.Warn("swarm: list tasks", "space", id, "err", err)
		return []webapi.TaskInfo{}, true
	}
	out := make([]webapi.TaskInfo, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, webapi.TaskInfo{
			ID: t.ID, Title: t.Title, Spec: t.Spec, Status: string(t.Status),
			Assignee: t.Assignee, CreatedBy: t.CreatedBy, Result: t.Result,
			VerifyNote: t.VerifyNote, ParentID: t.ParentID,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		})
	}
	return out, true
}

func (s *Service) Messages(id string) ([]webapi.MessageInfo, bool) {
	ent, ok := s.entry(id)
	if !ok {
		return nil, false
	}
	msgs, err := ent.space.Store.ListMessages(0)
	if err != nil {
		s.log.Warn("swarm: list messages", "space", id, "err", err)
		return []webapi.MessageInfo{}, true
	}
	out := make([]webapi.MessageInfo, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, webapi.MessageInfo{
			ID: m.ID, Sender: m.Sender, Recipient: m.Recipient, Subject: m.Subject,
			Body: m.Body, RefTask: m.RefTask, ReadAt: m.ReadAt, CreatedAt: m.CreatedAt,
		})
	}
	return out, true
}

func (s *Service) Transcript(id, agent string) ([]webapi.TranscriptEntry, bool) {
	ctl, ok := s.controller(id, agent)
	if !ok {
		return nil, false
	}
	msgs := ctl.Messages()
	out := make([]webapi.TranscriptEntry, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, webapi.TranscriptEntry{Role: string(m.Role), Text: m.Content})
	}
	return out, true
}

// Run drives one member for a turn. It is asynchronous: the browser sees the
// turn via the event stream, so the HTTP/WS call returns immediately. A second
// concurrent run on the same agent is the agent layer's concern (it guards).
func (s *Service) Run(id, agent, prompt string) error {
	ctl, ok := s.controller(id, agent)
	if !ok {
		return fmt.Errorf("swarm: unknown space/agent %q/%q", id, agent)
	}
	go func() {
		if _, err := ctl.Run(s.rootCtx, prompt); err != nil {
			s.log.Warn("swarm: web-driven run", "space", id, "agent", agent, "err", err)
		}
	}()
	return nil
}

// SendUserMessage drops an operator message onto a member's mailbox as sender
// "user" (or broadcasts when to == "all"). It deliberately reuses Bus.Send — the
// exact path inter-agent mail takes — so the supervisor's wake/drain delivers it
// without any new orchestration: an idle member is woken (drain A), a busy one
// folds it mid-run (drain B), and the task ledger is untouched. This is the
// non-disruptive core of flat operator↔member comms.
func (s *Service) SendUserMessage(id, to, subject, body string) error {
	ent, ok := s.entry(id)
	if !ok {
		return fmt.Errorf("swarm: unknown space %q", id)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("swarm: message body is required")
	}
	if to != store.RecipientAll {
		if _, known := ent.space.Roster.Controller(to); !known {
			return fmt.Errorf("swarm: unknown member %q", to)
		}
	}
	_, err := ent.space.Bus.Send(store.Message{
		Sender:    "user",
		Recipient: to,
		Subject:   subject,
		Body:      body,
	})
	return err
}

func (s *Service) RespondPermission(id, agent, reqID, behavior, reason string) error {
	ctl, ok := s.controller(id, agent)
	if !ok {
		return fmt.Errorf("swarm: unknown space/agent %q/%q", id, agent)
	}
	return ctl.RespondPermission(reqID, ui.PermissionDecision{Behavior: behavior, Reason: reason})
}

func (s *Service) RespondQuestion(id, agent, reqID string, answers map[string]string) error {
	ctl, ok := s.controller(id, agent)
	if !ok {
		return fmt.Errorf("swarm: unknown space/agent %q/%q", id, agent)
	}
	return ctl.RespondQuestion(reqID, ui.QuestionResponse{Answers: answers})
}

func (s *Service) Suspend(id, agent string) error  { return s.superCmd(id, agent, (*swarm.Supervisor).Suspend) }
func (s *Service) Resume(id, agent string) error   { return s.superCmd(id, agent, (*swarm.Supervisor).Resume) }
func (s *Service) Freeze(id, agent string) error   { return s.superCmd(id, agent, (*swarm.Supervisor).Freeze) }
func (s *Service) Unfreeze(id, agent string) error { return s.superCmd(id, agent, (*swarm.Supervisor).Unfreeze) }

func (s *Service) AddMember(id, agent string) error {
	return s.superCmd(id, agent, (*swarm.Supervisor).AddMember)
}

func (s *Service) HaltAll(id string) error {
	ent, ok := s.entry(id)
	if !ok {
		return fmt.Errorf("swarm: unknown space %q", id)
	}
	return ent.super.HaltAll()
}

// superCmd routes a one-member supervisor command, surfacing an "unknown space"
// error the HTTP layer maps to 404.
func (s *Service) superCmd(id, agent string, fn func(*swarm.Supervisor, string) error) error {
	ent, ok := s.entry(id)
	if !ok {
		return fmt.Errorf("swarm: unknown space %q", id)
	}
	return fn(ent.super, agent)
}

// controller resolves a member's Controller within a space.
func (s *Service) controller(id, agent string) (ui.Controller, bool) {
	ent, ok := s.entry(id)
	if !ok {
		return nil, false
	}
	return ent.space.Roster.Controller(agent)
}
