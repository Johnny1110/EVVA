package store

import (
	"errors"
	"testing"
	"time"
)

// RP-20: the runtime schedule override DAO — upsert/replace, tombstones,
// per-member delete, and the fresh-register wipe.

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestScheduleRowRoundTrip(t *testing.T) {
	s := openTestStore(t)

	in := ScheduleRow{Member: "analyst", Cron: "*/5 * * * *", Prompt: "patrol", UpdatedAt: 12345}
	if err := s.PutSchedule(in); err != nil {
		t.Fatalf("PutSchedule: %v", err)
	}
	rows, err := s.ListSchedules()
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if len(rows) != 1 || rows[0] != in {
		t.Fatalf("rows = %+v, want exactly %+v", rows, in)
	}

	// Upsert replaces in place — one row per member.
	repl := ScheduleRow{Member: "analyst", EveryNS: int64(90 * time.Second), Prompt: "faster", UpdatedAt: 23456}
	if err := s.PutSchedule(repl); err != nil {
		t.Fatalf("PutSchedule replace: %v", err)
	}
	rows, _ = s.ListSchedules()
	if len(rows) != 1 || rows[0] != repl {
		t.Fatalf("after replace rows = %+v, want exactly %+v", rows, repl)
	}
}

func TestScheduleTombstoneAndDelete(t *testing.T) {
	s := openTestStore(t)

	if err := s.PutSchedule(ScheduleRow{Member: "w", Cron: "0 9 * * *", UpdatedAt: 1}); err != nil {
		t.Fatalf("PutSchedule: %v", err)
	}
	// A clear is a tombstone row, not a delete — it must come back from List.
	if err := s.PutSchedule(ScheduleRow{Member: "w", Cleared: true, UpdatedAt: 2}); err != nil {
		t.Fatalf("PutSchedule tombstone: %v", err)
	}
	rows, _ := s.ListSchedules()
	if len(rows) != 1 || !rows[0].Cleared || rows[0].Cron != "" {
		t.Fatalf("tombstone rows = %+v, want one cleared row with no cadence", rows)
	}

	// DeleteSchedule forgets the member entirely (the RemoveMember path).
	if err := s.DeleteSchedule("w"); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}
	if rows, _ := s.ListSchedules(); len(rows) != 0 {
		t.Fatalf("after delete rows = %+v, want none", rows)
	}
}

func TestClearSchedulesWipesAll(t *testing.T) {
	s := openTestStore(t)
	_ = s.PutSchedule(ScheduleRow{Member: "a", Cron: "* * * * *", UpdatedAt: 1})
	_ = s.PutSchedule(ScheduleRow{Member: "b", Cleared: true, UpdatedAt: 1})

	if err := s.ClearSchedules(); err != nil {
		t.Fatalf("ClearSchedules: %v", err)
	}
	if rows, _ := s.ListSchedules(); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none after the fresh-register wipe", rows)
	}
}

func TestPutScheduleRequiresMember(t *testing.T) {
	s := openTestStore(t)
	if err := s.PutSchedule(ScheduleRow{Cron: "* * * * *"}); !errors.Is(err, ErrEmptyScheduleMember) {
		t.Fatalf("err = %v, want ErrEmptyScheduleMember", err)
	}
	if err := s.DeleteSchedule(" "); !errors.Is(err, ErrEmptyScheduleMember) {
		t.Fatalf("delete err = %v, want ErrEmptyScheduleMember", err)
	}
}

func TestPutScheduleDefaultsUpdatedAt(t *testing.T) {
	s := openTestStore(t)
	before := time.Now().UnixMilli()
	if err := s.PutSchedule(ScheduleRow{Member: "w", Cron: "* * * * *"}); err != nil {
		t.Fatalf("PutSchedule: %v", err)
	}
	rows, _ := s.ListSchedules()
	if len(rows) != 1 || rows[0].UpdatedAt < before {
		t.Fatalf("UpdatedAt = %d, want >= %d (defaulted to now)", rows[0].UpdatedAt, before)
	}
}
