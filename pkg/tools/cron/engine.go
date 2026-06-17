package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expr is a parsed 5-field cron expression (minute hour day-of-month month
// day-of-week). It is a plain comparable value safe for concurrent use.
//
// The parser supports *, */n, n, a-b, a-b/n, and comma lists. Day-of-month and
// day-of-week follow standard cron OR semantics (when both are restricted, a
// day matches if EITHER matches). No external dependency — the engine is
// self-contained.
type Expr struct {
	min, hour, dom, month, dow uint64 // bitsets
	domStar, dowStar           bool
}

// ParseExpr parses a standard 5-field cron expression. Returns an error for
// unsupported syntax (@-aliases, TZ= prefix, 6-field, L/W/#/? specials).
func ParseExpr(spec string) (Expr, error) {
	if strings.HasPrefix(strings.TrimSpace(spec), "@") {
		return Expr{}, fmt.Errorf("cron %q: @-aliases (@daily, @hourly, @every …) are not supported — write the 5-field form (e.g. %q)", spec, "0 0 * * *")
	}
	fields := strings.Fields(spec)
	if len(fields) > 0 && strings.HasPrefix(fields[0], "TZ=") {
		return Expr{}, fmt.Errorf("cron %q: a TZ= prefix is not supported — schedules always match the system's LOCAL wall clock", spec)
	}
	if len(fields) != 5 {
		hint := ""
		if len(fields) == 6 {
			hint = " — a seconds field is not supported; minute resolution only"
		}
		return Expr{}, fmt.Errorf("cron %q: want 5 fields (minute hour day-of-month month day-of-week), got %d%s", spec, len(fields), hint)
	}
	var c Expr
	var err error
	if c.min, _, err = parseField(fields[0], 0, 59); err != nil {
		return Expr{}, fmt.Errorf("cron %q minute: %w", spec, err)
	}
	if c.hour, _, err = parseField(fields[1], 0, 23); err != nil {
		return Expr{}, fmt.Errorf("cron %q hour: %w", spec, err)
	}
	if c.dom, c.domStar, err = parseField(fields[2], 1, 31); err != nil {
		return Expr{}, fmt.Errorf("cron %q day-of-month: %w", spec, err)
	}
	if c.month, _, err = parseField(fields[3], 1, 12); err != nil {
		return Expr{}, fmt.Errorf("cron %q month: %w", spec, err)
	}
	if c.dow, c.dowStar, err = parseField(fields[4], 0, 7); err != nil {
		return Expr{}, fmt.Errorf("cron %q day-of-week: %w", spec, err)
	}
	if c.dow&(1<<7) != 0 {
		c.dow |= 1 << 0
		c.dow &^= 1 << 7
	}
	return c, nil
}

// NextAfter returns the first activation strictly after `after`, in Local time.
// Steps minute-by-minute, bounded to ~5 years so an impossible spec (e.g.
// Feb 30) returns an error instead of looping forever.
func (c Expr) NextAfter(after time.Time) (time.Time, error) {
	t := after.In(time.Local).Truncate(time.Minute).Add(time.Minute)
	const maxMinutes = 5 * 366 * 24 * 60
	for range maxMinutes {
		if bitSet(c.month, int(t.Month())) && c.matchDay(t) &&
			bitSet(c.hour, t.Hour()) && bitSet(c.min, t.Minute()) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron: no activation within 5 years")
}

func parseField(spec string, lo, hi int) (uint64, bool, error) {
	var mask uint64
	star := false
	for _, part := range strings.Split(spec, ",") {
		if strings.ContainsAny(part, "LW#?") {
			return 0, false, fmt.Errorf("%q: L/W/#/? specials are not supported — only plain values, ranges (a-b), steps (*/n, a-b/n) and comma lists", part)
		}
		base, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			s, err := strconv.Atoi(stepStr)
			if err != nil || s <= 0 {
				return 0, false, fmt.Errorf("bad step in %q", part)
			}
			step = s
		}

		var from, to int
		switch {
		case base == "*":
			from, to = lo, hi
			if !hasStep {
				star = true
			}
		case strings.ContainsRune(base, '-'):
			a, b, _ := strings.Cut(base, "-")
			x, err1 := strconv.Atoi(a)
			y, err2 := strconv.Atoi(b)
			if err1 != nil || err2 != nil {
				return 0, false, fmt.Errorf("bad range %q", part)
			}
			from, to = x, y
		default:
			x, err := strconv.Atoi(base)
			if err != nil {
				return 0, false, fmt.Errorf("bad value %q", part)
			}
			from, to = x, x
		}

		if from < lo || to > hi || from > to {
			return 0, false, fmt.Errorf("%q out of range [%d,%d]", part, lo, hi)
		}
		for v := from; v <= to; v += step {
			mask |= 1 << uint(v)
		}
	}
	return mask, star, nil
}

func bitSet(mask uint64, v int) bool { return mask&(1<<uint(v)) != 0 }

func (c Expr) matchDay(t time.Time) bool {
	domMatch := bitSet(c.dom, t.Day())
	dowMatch := bitSet(c.dow, int(t.Weekday()))
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dowMatch
	case c.dowStar:
		return domMatch
	default:
		return domMatch || dowMatch
	}
}
