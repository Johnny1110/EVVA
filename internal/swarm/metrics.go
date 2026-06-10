package swarm

import (
	"sync"
	"time"
)

// MemberMetrics counts one member's scheduler lifecycle (RP-17): wakes by
// source, runs, non-clean exits, and a coarse run-duration histogram. Exported
// as a snapshot value through SwarmSpace.MetricsSnapshot.
type MemberMetrics struct {
	WakesMessage int64
	WakesTimer   int64
	Runs         int64
	Aborts       int64
	// RunSeconds buckets completed runs by wall-clock duration:
	// [0] <10s, [1] <1m, [2] <10m, [3] ≥10m.
	RunSeconds [4]int64
}

// spaceMetrics aggregates per-member counters. Mutex-guarded plain ints —
// increments happen once per wake/run, never per token, so contention is nil.
// Every method is nil-receiver-safe: hand-built test spaces simply skip
// metrics, the same stance the meter and schedules take.
type spaceMetrics struct {
	mu      sync.Mutex
	started time.Time
	members map[string]*MemberMetrics
}

func newSpaceMetrics() *spaceMetrics {
	return &spaceMetrics{started: time.Now(), members: map[string]*MemberMetrics{}}
}

func (m *spaceMetrics) memberLocked(name string) *MemberMetrics {
	mm, ok := m.members[name]
	if !ok {
		mm = &MemberMetrics{}
		m.members[name] = mm
	}
	return mm
}

// countWake tallies one serve of an active member, by wake source.
func (m *spaceMetrics) countWake(name string, r wakeReason) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	mm := m.memberLocked(name)
	if r == wakeTimer {
		mm.WakesTimer++
	} else {
		mm.WakesMessage++
	}
}

// countRun tallies one finished run: total, abort-or-clean, duration bucket.
func (m *spaceMetrics) countRun(name string, d time.Duration, clean bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	mm := m.memberLocked(name)
	mm.Runs++
	if !clean {
		mm.Aborts++
	}
	switch {
	case d < 10*time.Second:
		mm.RunSeconds[0]++
	case d < time.Minute:
		mm.RunSeconds[1]++
	case d < 10*time.Minute:
		mm.RunSeconds[2]++
	default:
		mm.RunSeconds[3]++
	}
}

// MetricsSnapshot copies every member's counters plus the space's start time
// (the uptime anchor). Exported for the service's metrics endpoint (RP-17).
func (sp *SwarmSpace) MetricsSnapshot() (map[string]MemberMetrics, time.Time) {
	m := sp.metrics
	if m == nil {
		return map[string]MemberMetrics{}, time.Time{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]MemberMetrics, len(m.members))
	for k, v := range m.members {
		out[k] = *v
	}
	return out, m.started
}
