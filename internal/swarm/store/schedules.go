package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// schedules.go is the RP-20 runtime-schedule DAO. The table holds one row per
// member whose schedule was changed AT RUNTIME (schedule_set / web edit /
// schedule_clear); manifest-declared schedules never enter it. A cleared
// schedule is a tombstone row (Cleared=true) so "the leader removed this
// crontab" survives a restart instead of resurrecting from the manifest.

// ErrEmptyScheduleMember guards the schedule DAO (rows are keyed by member).
var ErrEmptyScheduleMember = errors.New("store: schedule requires a non-empty member")

// ScheduleRow is one runtime schedule override. Exactly one of Cron / EveryNS
// is set for a live schedule; both are zero on a tombstone. EveryNS is the
// interval in nanoseconds (a verbatim time.Duration); UpdatedAt is unix millis.
type ScheduleRow struct {
	Member    string
	Cron      string
	EveryNS   int64
	Prompt    string
	Cleared   bool
	UpdatedAt int64
}

// PutSchedule upserts a member's runtime schedule override — a live schedule
// when row.Cleared is false, a tombstone when true. UpdatedAt defaults to now
// when zero. The caller validates the schedule itself (agentdef.Schedule owns
// cron parsing); the store only persists.
func (s *Store) PutSchedule(row ScheduleRow) error {
	if strings.TrimSpace(row.Member) == "" {
		return ErrEmptyScheduleMember
	}
	if row.UpdatedAt == 0 {
		row.UpdatedAt = time.Now().UnixMilli()
	}
	cleared := 0
	if row.Cleared {
		cleared = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO schedules (member, cron, every_ns, prompt, cleared, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(member) DO UPDATE SET
		   cron = excluded.cron, every_ns = excluded.every_ns, prompt = excluded.prompt,
		   cleared = excluded.cleared, updated_at = excluded.updated_at`,
		row.Member, row.Cron, row.EveryNS, row.Prompt, cleared, row.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("store: put schedule %s: %w", row.Member, err)
	}
	return nil
}

// ListSchedules returns every runtime schedule row (live and tombstoned),
// member-ordered. The restart rebuild applies these over the manifest seeds.
func (s *Store) ListSchedules() ([]ScheduleRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT member, cron, every_ns, prompt, cleared, updated_at FROM schedules ORDER BY member`)
	if err != nil {
		return nil, fmt.Errorf("store: list schedules: %w", err)
	}
	defer rows.Close()

	var out []ScheduleRow
	for rows.Next() {
		var r ScheduleRow
		var cleared int
		if err := rows.Scan(&r.Member, &r.Cron, &r.EveryNS, &r.Prompt, &cleared, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan schedule: %w", err)
		}
		r.Cleared = cleared != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteSchedule forgets one member's runtime override entirely (row and
// tombstone alike). Used when a member is removed from the space — its
// override dies with it, so a later re-add starts from the manifest.
func (s *Store) DeleteSchedule(member string) error {
	if strings.TrimSpace(member) == "" {
		return ErrEmptyScheduleMember
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM schedules WHERE member = ?`, member); err != nil {
		return fmt.Errorf("store: delete schedule %s: %w", member, err)
	}
	return nil
}

// ClearSchedules drops every runtime override for the space. Called on a
// fresh register (`evva swarm .`) — re-registering is the operator's explicit
// "take the manifest as written", so runtime history yields (RP-20 §2.4).
func (s *Store) ClearSchedules() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM schedules`); err != nil {
		return fmt.Errorf("store: clear schedules: %w", err)
	}
	return nil
}
