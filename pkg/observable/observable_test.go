package observable

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Phase 1 analysis — Observable surface:
//   - Subscribe(fn): appends fn to observer list; nil is silently dropped
//   - Notify(c): fans c out to every subscriber; fills c.Time when zero;
//     snapshots the observer list under read-lock so Subscribe-during-Notify
//     is safe (the new observer is NOT invoked for that in-flight Notify
//     but IS invoked for subsequent ones)
//   - Observable is safe for concurrent Subscribe / Notify

// collect builds an observer that records every Change it receives, plus
// a thread-safe reader for the recorded slice. Each test gets its own
// collector so no test leaks state to another.
func collect() (Observer, func() []Change) {
	var mu sync.Mutex
	var got []Change
	obs := func(c Change) {
		mu.Lock()
		got = append(got, c)
		mu.Unlock()
	}
	read := func() []Change {
		mu.Lock()
		defer mu.Unlock()
		return append([]Change(nil), got...)
	}
	return obs, read
}

func TestSubscribe_NilIsDropped(t *testing.T) {
	// Arrange
	var o Observable

	// Act — must not panic.
	o.Subscribe(nil)

	// Assert — Notify on an Observable with only-nil "subscribers" is a no-op.
	o.Notify(Change{Domain: "x", Op: "y", ID: "z"})
	// If we reach here without panic / deadlock the test passes.
}

func TestNotify_FansOutToAllObservers(t *testing.T) {
	var o Observable
	obs1, read1 := collect()
	obs2, read2 := collect()
	o.Subscribe(obs1)
	o.Subscribe(obs2)

	o.Notify(Change{Domain: "task", Op: "created", ID: "t1"})

	for name, read := range map[string]func() []Change{"obs1": read1, "obs2": read2} {
		got := read()
		if len(got) != 1 {
			t.Errorf("%s: len got %d, want 1", name, len(got))
			continue
		}
		if got[0].Domain != "task" || got[0].Op != "created" || got[0].ID != "t1" {
			t.Errorf("%s: got %+v", name, got[0])
		}
	}
}

func TestNotify_FillsZeroTime(t *testing.T) {
	var o Observable
	obs, read := collect()
	o.Subscribe(obs)

	before := time.Now()
	o.Notify(Change{Domain: "x"})
	after := time.Now()

	got := read()
	if len(got) != 1 {
		t.Fatalf("len: got %d, want 1", len(got))
	}
	if got[0].Time.IsZero() {
		t.Fatal("Time was not auto-filled")
	}
	if got[0].Time.Before(before) || got[0].Time.After(after) {
		t.Errorf("Time outside expected window: got %v (before=%v after=%v)",
			got[0].Time, before, after)
	}
}

func TestNotify_PreservesCallerTime(t *testing.T) {
	var o Observable
	obs, read := collect()
	o.Subscribe(obs)

	when := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	o.Notify(Change{Domain: "x", Time: when})

	got := read()
	if !got[0].Time.Equal(when) {
		t.Errorf("caller-supplied Time clobbered: got %v, want %v", got[0].Time, when)
	}
}

func TestNotify_PayloadPassesThroughUntyped(t *testing.T) {
	// The package promises payload is typed-but-opaque; tests assert the
	// pointer flows through unchanged so consumers can type-assert.
	var o Observable
	obs, read := collect()
	o.Subscribe(obs)

	payload := struct{ Name string }{"hello"}
	o.Notify(Change{Domain: "x", Payload: payload})

	got := read()
	v, ok := got[0].Payload.(struct{ Name string })
	if !ok {
		t.Fatalf("payload type lost: got %T", got[0].Payload)
	}
	if v.Name != "hello" {
		t.Errorf("payload content: got %+v", v)
	}
}

func TestNotify_NoObservers_IsNoOp(t *testing.T) {
	var o Observable
	// Must not panic.
	o.Notify(Change{Domain: "x"})
}

func TestSubscribe_DuringNotify_NewObserverMissesInFlight(t *testing.T) {
	// Snapshot-then-iterate contract: an observer that subscribes from
	// inside a Notify callback must NOT receive that same Notify event
	// (otherwise observers could see events in surprising re-entrant
	// order). It DOES receive every subsequent Notify.
	var o Observable

	var lateCount atomic.Int32
	late := func(_ Change) { lateCount.Add(1) }

	var subOnce sync.Once
	first := func(_ Change) {
		// Re-entrant subscribe from inside the first observer's callback,
		// but only on the FIRST Notify — the observer keeps firing on
		// subsequent ones, and we don't want to attach `late` over and
		// over (it would skew the count).
		subOnce.Do(func() { o.Subscribe(late) })
	}
	o.Subscribe(first)

	o.Notify(Change{Domain: "first"})

	if got := lateCount.Load(); got != 0 {
		t.Errorf("late observer should NOT have seen the in-flight Notify; got %d invocations", got)
	}

	// Subsequent Notify must reach the late observer.
	o.Notify(Change{Domain: "second"})
	if got := lateCount.Load(); got != 1 {
		t.Errorf("late observer should have seen the second Notify; got %d", got)
	}
}

func TestSubscribe_ConcurrentSubscribersAllReceive(t *testing.T) {
	// Concurrent Subscribe from many goroutines must all land in the
	// observer list without races / lost subscriptions.
	var o Observable
	const N = 50

	var counts [N]atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			o.Subscribe(func(_ Change) {
				counts[i].Add(1)
			})
		}()
	}
	wg.Wait()

	o.Notify(Change{Domain: "x"})

	for i := 0; i < N; i++ {
		if got := counts[i].Load(); got != 1 {
			t.Errorf("observer %d: got %d invocations, want 1", i, got)
		}
	}
}

func TestNotify_ConcurrentNotifiersAllReachAllObservers(t *testing.T) {
	// N notifiers × 1 observer → observer must see exactly N events
	// (snapshot-on-read means each Notify's fan-out is atomic per call).
	var o Observable
	var seen atomic.Int32
	o.Subscribe(func(_ Change) { seen.Add(1) })

	const N = 100
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.Notify(Change{Domain: "x"})
		}()
	}
	wg.Wait()

	if got := seen.Load(); got != N {
		t.Errorf("observer call count: got %d, want %d", got, N)
	}
}
