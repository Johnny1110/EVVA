-- RP-20: runtime schedule overrides. A row exists ONLY when the schedule was
-- changed at runtime (leader schedule_set / operator web edit) — manifest and
-- profile.yml schedules never enter this table, so "no row" means "the
-- manifest is authoritative". A clear is a TOMBSTONE (cleared=1), not a row
-- delete: "the leader removed this member's crontab" must survive a restart
-- and beat a schedule the manifest still declares.
--
-- One db per space, so no space-id column. cron/every_ns mirror
-- agentdef.Schedule's two forms (exactly one is set); every_ns is the
-- interval in nanoseconds (lossless for time.Duration). updated_at is unix
-- millis like every other table here.

CREATE TABLE schedules (
  member     TEXT    PRIMARY KEY,
  cron       TEXT    NOT NULL DEFAULT '',
  every_ns   INTEGER NOT NULL DEFAULT 0,
  prompt     TEXT    NOT NULL DEFAULT '',
  cleared    INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);
