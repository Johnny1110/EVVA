package swarm

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/skill"
	"github.com/johnny1110/evva/pkg/tools"
)

// fakeLLM is a no-network llm.Client so agent.New constructs and runs without
// real API calls.
type fakeLLM struct{ model string }

func (f *fakeLLM) Name() string             { return stubProvider }
func (f *fakeLLM) Model() string            { return f.model }
func (*fakeLLM) SupportsDeferLoading() bool { return false }
func (*fakeLLM) Complete(context.Context, []llm.Message, []tools.Tool) (llm.Response, error) {
	return llm.Response{Content: "ok"}, nil
}
func (f *fakeLLM) Stream(ctx context.Context, m []llm.Message, ts []tools.Tool, _ llm.ChunkSink) (llm.Response, error) {
	return f.Complete(ctx, m, ts)
}
func (*fakeLLM) Apply(...llm.Option) {}

const (
	stubProvider = "swarm_stub"
	stubModel    = "stub-model"
)

func init() {
	if !llm.DefaultRegistry().Has(stubProvider) {
		_ = llm.DefaultRegistry().Register(stubProvider, func(_ llm.APIConfig, model string, _ ...llm.Option) (llm.Client, error) {
			return &fakeLLM{model: model}, nil
		})
	}
}

func stubConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(config.LoadOptions{AppName: "swarmtest", AppHome: t.TempDir(), WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.LLMProviderConfig[stubProvider] = config.APIConfig{ApiURL: "http://stub", ApiSecret: "x", Models: []constant.Model{stubModel}}
	cfg.DefaultProvider = constant.LLMProvider{Name: stubProvider, Models: []constant.Model{stubModel}}
	cfg.DefaultModel = constant.Model(stubModel)
	return cfg
}

func testManifest() agentdef.Manifest {
	return agentdef.Manifest{Name: "team", Settings: agentdef.Settings{PermissionMode: "bypass", MaxIterations: 5}}
}

func testLoaded() []agentdef.Loaded {
	mk := func(name string, role agentdef.Role) agentdef.Loaded {
		return agentdef.Loaded{
			Def:    agent.AgentDefinition{Name: name, SystemPrompt: "You are " + name + ".", Model: stubModel},
			Skills: skill.NewRegistry(),
			Role:   role,
		}
	}
	return []agentdef.Loaded{
		mk("leader", agentdef.RoleLeader),
		mk("worker-a", agentdef.RoleWorker),
		mk("worker-b", agentdef.RoleWorker),
	}
}

// AC#1 + AC#5: NewSpace constructs every member, all reachable by name, all
// active + idle, with accurate roster fields.
func TestNewSpaceConstructsRoster(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("space-1", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	snap := sp.Roster.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("roster has %d members, want 3", len(snap))
	}
	for _, mv := range snap {
		if mv.Membership != MembershipActive || mv.Run != RunIdle {
			t.Errorf("%s: %s/%s, want active/idle", mv.Name, mv.Membership, mv.Run)
		}
	}
	for _, n := range []string{"leader", "worker-a", "worker-b"} {
		if _, ok := sp.Roster.Controller(n); !ok {
			t.Errorf("member %q not reachable via roster", n)
		}
	}
	if snap[0].Name != "leader" || snap[0].Role != agentdef.RoleLeader {
		t.Errorf("entry[0] = %+v, want leader/leader", snap[0])
	}
	if snap[1].Role != agentdef.RoleWorker {
		t.Errorf("worker role = %s", snap[1].Role)
	}
}

// RP-24: a member-level permission_mode overrides the space setting; members
// without one inherit it; the roster carries each member's effective stance
// (read back off the constructed agent, so it reflects what the broker will
// actually enforce).
func TestPerMemberPermissionMode(t *testing.T) {
	cfg := stubConfig(t)
	loaded := testLoaded()
	loaded[1].PermissionMode = "default" // worker-a pinned stricter than the bypass space
	sp, err := NewSpace("space-pm", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	got := map[string]string{}
	for _, v := range sp.Roster.Snapshot() {
		got[v.Name] = v.PermissionMode
	}
	if got["leader"] != "bypass" || got["worker-b"] != "bypass" {
		t.Errorf("members without an override should inherit the space mode (bypass), got %v", got)
	}
	if got["worker-a"] != "default" {
		t.Errorf("worker-a mode = %q, want default (member override beats settings)", got["worker-a"])
	}
}

// RP-24: an invalid member permission_mode fails construction loudly (the
// programmatic-manifest guard; the yaml path already rejects at LoadManifest).
func TestPerMemberPermissionModeInvalid(t *testing.T) {
	cfg := stubConfig(t)
	loaded := testLoaded()
	loaded[2].PermissionMode = "yolo"
	sp, err := NewSpace("space-pm-bad", testManifest(), loaded, nil, cfg)
	if err == nil {
		sp.Shutdown()
		t.Fatal("NewSpace should reject an invalid member permission_mode")
	}
	if !strings.Contains(err.Error(), "permission_mode") {
		t.Errorf("err = %v, want a permission_mode error", err)
	}
}

// RP-25: every member gets its own memory dir at construction (first boot and
// hot-add share constructMember), and the wake reminder carries exactly the
// members' own MEMORY.md — absent index = zero wake noise.
func TestMemberMemoryDirAndWakeReminder(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("space-mem", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	leaderDir := filepath.Join(sp.Workdir, "agents", "main", "leader", "memory")
	workerDir := filepath.Join(sp.Workdir, "agents", "sub", "worker-a", "memory")
	for _, d := range []string{leaderDir, workerDir} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Errorf("memory dir not created: %s (%v)", d, err)
		}
	}

	// No MEMORY.md yet → empty reminder for everyone.
	if got := sp.memoryWakeReminder("worker-a"); got != "" {
		t.Errorf("memory-less member should wake with no index, got %q", got)
	}

	// worker-a saves an index → its wake reminder carries it, labeled with the
	// dir-relative path; worker-b stays silent.
	idx := "- [Exposure](exposure.md) — BTC delta as of 2026-06-11"
	if err := os.WriteFile(filepath.Join(workerDir, "MEMORY.md"), []byte(idx+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := sp.memoryWakeReminder("worker-a")
	if !strings.Contains(got, idx) || !strings.Contains(got, "agents/sub/worker-a/memory/MEMORY.md") {
		t.Errorf("wake reminder missing index or path label:\n%s", got)
	}
	if got := sp.memoryWakeReminder("worker-b"); got != "" {
		t.Errorf("worker-b has no memory, reminder should be empty, got %q", got)
	}

	// MemberMemoryFiles serves the same dir to the web, index first.
	if err := os.WriteFile(filepath.Join(workerDir, "exposure.md"), []byte("---\nname: exposure\n---\n\ndelta 0.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := sp.MemberMemoryFiles("worker-a")
	if err != nil || len(files) != 2 || files[0].Name != "MEMORY.md" || files[1].Name != "exposure.md" {
		t.Fatalf("MemberMemoryFiles = %+v, %v; want [MEMORY.md exposure.md]", files, err)
	}
	if _, err := sp.MemberMemoryFiles("ghost"); err == nil {
		t.Error("unknown member should error")
	}
}

// RP-25 acceptance: the static system prompt is byte-identical whether or not
// member memory files exist — memory reaches the model ONLY via wake prompts,
// so a weeks-long swarm keeps one cached prompt prefix (RP-5). The capture
// provider records each member's system prompt at LLM-client build.
func TestMemberPromptBitStableAcrossMemoryChange(t *testing.T) {
	var (
		capMu    sync.Mutex
		captured []string
	)
	const capProvider = "swarm_cap"
	if !llm.DefaultRegistry().Has(capProvider) {
		_ = llm.DefaultRegistry().Register(capProvider, func(_ llm.APIConfig, model string, opts ...llm.Option) (llm.Client, error) {
			var p llm.LLMParams
			for _, o := range opts {
				o(&p)
			}
			if p.System != "" {
				capMu.Lock()
				captured = append(captured, p.System)
				capMu.Unlock()
			}
			return &fakeLLM{model: model}, nil
		})
	}

	cfg := stubConfig(t)
	cfg.LLMProviderConfig[capProvider] = config.APIConfig{ApiURL: "http://stub", ApiSecret: "x", Models: []constant.Model{stubModel}}
	cfg.DefaultProvider = constant.LLMProvider{Name: capProvider, Models: []constant.Model{stubModel}}

	snapshot := func() []string {
		capMu.Lock()
		captured = nil
		capMu.Unlock()
		sp, err := NewSpace("space-bit", testManifest(), testLoaded(), nil, cfg)
		if err != nil {
			t.Fatalf("NewSpace: %v", err)
		}
		sp.Shutdown()
		capMu.Lock()
		out := append([]string{}, captured...)
		capMu.Unlock()
		sort.Strings(out)
		return out
	}

	before := snapshot()
	if len(before) == 0 {
		t.Fatal("capture provider saw no system prompts — capture seam broken")
	}

	// Grow worker-a's memory between builds; the prompts must not move a byte.
	memDir := filepath.Join(cfg.WorkDir, "agents", "sub", "worker-a", "memory")
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("- [X](x.md) — drift bait"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "x.md"), []byte("drift bait body"), 0o644); err != nil {
		t.Fatal(err)
	}

	after := snapshot()
	if len(before) != len(after) {
		t.Fatalf("prompt count changed: %d → %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("member prompt #%d changed after memory writes — static prefix must be memory-independent", i)
		}
	}
	// And no prompt may embed the index content.
	for _, p := range after {
		if strings.Contains(p, "drift bait") {
			t.Error("memory content leaked into a static system prompt")
		}
	}
}

// AC#2: an agent's events arrive on the space out channel stamped with the
// correct spaceID and AgentID.
func TestSpaceEventTagging(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("space-7", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	leaderID := sp.agents["leader"].AgentID()

	if _, err := sp.agents["leader"].Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // let any trailing events land in the buffer

	var events []SpacedEvent
drain:
	for {
		select {
		case e := <-sp.Events():
			events = append(events, e)
		default:
			break drain
		}
	}

	if len(events) == 0 {
		t.Fatal("no events arrived on the space channel")
	}
	sawLeader := false
	for _, e := range events {
		if e.SpaceID != "space-7" {
			t.Errorf("event SpaceID = %q, want space-7", e.SpaceID)
		}
		if e.Event.AgentID == leaderID {
			sawLeader = true
		}
	}
	if !sawLeader {
		t.Errorf("no event carried the leader's AgentID %q", leaderID)
	}
}

// AC#3: two spaces with the SAME member names share nothing.
func TestTwoSpaceIsolation(t *testing.T) {
	sp1, err := NewSpace("s1", testManifest(), testLoaded(), nil, stubConfig(t))
	if err != nil {
		t.Fatalf("space 1: %v", err)
	}
	defer sp1.Shutdown()
	sp2, err := NewSpace("s2", testManifest(), testLoaded(), nil, stubConfig(t))
	if err != nil {
		t.Fatalf("space 2: %v", err)
	}
	defer sp2.Shutdown()

	c1, _ := sp1.Roster.Controller("leader")
	c2, _ := sp2.Roster.Controller("leader")
	if c1 == nil || c2 == nil || c1 == c2 {
		t.Error("same-named leaders should be distinct controllers across spaces")
	}
	if sp1.Store == sp2.Store {
		t.Error("spaces share a store")
	}
	if sp1.Workdir == sp2.Workdir {
		t.Error("spaces share a workdir")
	}
	if sp1.agents["leader"].AgentID() == sp2.agents["leader"].AgentID() {
		t.Error("same AgentID across spaces")
	}
}

// AC#4: duplicate member names within one space error at construction.
func TestNewSpaceDuplicateNameErrors(t *testing.T) {
	dup := []agentdef.Loaded{
		{Def: agent.AgentDefinition{Name: "x", SystemPrompt: "a", Model: stubModel}, Skills: skill.NewRegistry(), Role: agentdef.RoleLeader},
		{Def: agent.AgentDefinition{Name: "x", SystemPrompt: "b", Model: stubModel}, Skills: skill.NewRegistry(), Role: agentdef.RoleWorker},
	}
	sp, err := NewSpace("dup", testManifest(), dup, nil, stubConfig(t))
	if err == nil {
		sp.Shutdown()
		t.Fatal("want a duplicate-name error")
	}
}
