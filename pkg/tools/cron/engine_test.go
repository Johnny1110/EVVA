package cron

import (
	"testing"
	"time"
)

func TestParseExpr(t *testing.T) {
	cases := []struct {
		spec string
		ok   bool
	}{
		{"*/5 * * * *", true},
		{"0 17 * * *", true},
		{"30 14 * * 1-5", true},
		{"0 0 1 1 *", true},
		{"0,15,30,45 * * * *", true},
		{"*/1 * * * *", true},
		{"0 9-17 * * 1-5", true},
		{"bad", false},
		{"* * *", false},
		{"* * * * * *", false},
		{"@daily", false},
		{"TZ=UTC * * * * *", false},
		{"60 * * * *", false},
		{"* 25 * * *", false},
		{"* * 0 * *", false},
		{"* * * 13 *", false},
		{"* * * * 8", false},
		{"L * * * *", false},
	}
	for _, c := range cases {
		_, err := ParseExpr(c.spec)
		if c.ok && err != nil {
			t.Errorf("ParseExpr(%q): unexpected error: %v", c.spec, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ParseExpr(%q): expected error, got nil", c.spec)
		}
	}
}

func TestExprNextAfter(t *testing.T) {
	// NextAfter works in Local time; use time.Local for all test inputs.
	loc := time.Local
	cases := []struct {
		spec  string
		after time.Time
		want  time.Time
	}{
		// Every minute: next minute after now.
		{"*/1 * * * *", time.Date(2026, 6, 17, 10, 30, 0, 0, loc),
			time.Date(2026, 6, 17, 10, 31, 0, 0, loc)},
		// Every 5 minutes.
		{"*/5 * * * *", time.Date(2026, 6, 17, 10, 32, 0, 0, loc),
			time.Date(2026, 6, 17, 10, 35, 0, 0, loc)},
		// Specific time: 2:30 PM every day.
		{"30 14 * * *", time.Date(2026, 6, 17, 10, 0, 0, 0, loc),
			time.Date(2026, 6, 17, 14, 30, 0, 0, loc)},
		// 2:30 PM — after 2:30 PM today, should be tomorrow.
		{"30 14 * * *", time.Date(2026, 6, 17, 14, 31, 0, 0, loc),
			time.Date(2026, 6, 18, 14, 30, 0, 0, loc)},
	}
	for _, c := range cases {
		expr, err := ParseExpr(c.spec)
		if err != nil {
			t.Errorf("ParseExpr(%q): %v", c.spec, err)
			continue
		}
		got, err := expr.NextAfter(c.after)
		if err != nil {
			t.Errorf("NextAfter(%q, %v): %v", c.spec, c.after, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("NextAfter(%q, %v) = %v, want %v", c.spec, c.after, got, c.want)
		}
	}
}

func TestExprNextAfter_Impossible(t *testing.T) {
	// Feb 30 never exists.
	expr, err := ParseExpr("0 0 30 2 *")
	if err != nil {
		t.Fatalf("ParseExpr: %v", err)
	}
	_, err = expr.NextAfter(time.Now())
	if err == nil {
		t.Error("NextAfter for impossible date should error")
	}
}
