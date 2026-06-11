package swarm

import (
	"errors"
	"testing"
	"time"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
	"github.com/johnny1110/evva/internal/swarm/store"
)

// RP-16: with retention_days set, the supervisor's daily sweep — which also
// fires once right after startup — archives-and-deletes consumed history.
func TestRetentionSweepAtStartup(t *testing.T) {
	sp, _ := ctlSpace(t, map[string]agentdef.Role{"w": agentdef.RoleWorker})
	sp.settings.RetentionDays = 30
	old := time.Now().AddDate(0, 0, -40).UnixMilli()
	if err := sp.Store.PutMessage(store.Message{
		ID: "m-old", Sender: "a", Recipient: "w", Body: "x", CreatedAt: old, ReadAt: &old,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	startSup(t, sp)
	waitFor(t, 5*time.Second, "old read message vacuumed by the startup sweep", func() bool {
		_, err := sp.Store.GetMessage("m-old")
		return errors.Is(err, store.ErrMessageNotFound)
	})
}

// RP-16: the zero-value (retention off — what a Go-built unit space gets, and
// what `retention_days: "0"` declares) keeps the pre-RP-16 never-delete
// behavior: the sweep never touches the ledger.
func TestRetentionSweepDisabledByZero(t *testing.T) {
	sp, _ := ctlSpace(t, map[string]agentdef.Role{"w": agentdef.RoleWorker})
	old := time.Now().AddDate(0, 0, -40).UnixMilli()
	if err := sp.Store.PutMessage(store.Message{
		ID: "m-old", Sender: "a", Recipient: "w", Body: "x", CreatedAt: old, ReadAt: &old,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	startSup(t, sp)
	time.Sleep(60 * time.Millisecond) // many 5ms ticks
	if _, err := sp.Store.GetMessage("m-old"); err != nil {
		t.Fatalf("retention-off sweep touched the ledger: %v", err)
	}
}

// RP-17: the scheduler feeds the per-member counters — a message wake counts
// once, its run counts once (clean), and a suspended run counts as an abort.
func TestMetricsCounting(t *testing.T) {
	sp, ctls := ctlSpace(t, map[string]agentdef.Role{"w": agentdef.RoleWorker})
	sp.metrics = newSpaceMetrics()
	startSup(t, sp)

	if _, err := sp.Bus.Send(store.Message{Sender: "user", Recipient: "w", Body: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, 5*time.Second, "clean run counted", func() bool {
		m, _ := sp.MetricsSnapshot()
		w := m["w"]
		return w.WakesMessage >= 1 && w.Runs >= 1 && w.Aborts == 0 && w.RunSeconds[0] >= 1
	})

	// A blocked run cancelled by Suspend lands in the abort column.
	ctls["w"].block = true
	if _, err := sp.Bus.Send(store.Message{Sender: "user", Recipient: "w", Body: "hang"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, 5*time.Second, "member busy", func() bool { return runStatusOf(sp, "w") == RunBusy })
	if err := sp.super.Suspend("w"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	waitFor(t, 5*time.Second, "abort counted", func() bool {
		m, _ := sp.MetricsSnapshot()
		return m["w"].Aborts >= 1
	})
}
