package swarm

import "time"

// usage.go is the RP-13 metering state: per-member daily token counters and the
// budget-breaker bookkeeping. The supervisor feeds it at run boundaries
// (runOnce reads the controller's cumulative Usage before/after each run — the
// member's own loop goroutine, so no concurrent-session read) and trips the
// breaker; the state itself lives on the space so persistRuntime/Reload carry
// it across restarts alongside membership and schedules.

// usageMeter is the per-space daily ledger. Guarded by sp.mu.
//
// frozen maps a breaker-frozen member to the LOCAL DAY it tripped. Carrying the
// day on the mark (instead of comparing the meter's day at sweep time) is what
// makes the release edge un-stealable: any run ending after midnight advances
// meter.day via ensureMeterLocked, and a sweep that keyed off "day changed"
// would then never fire — leaving budget-frozen members (who never run) frozen
// forever. A mark is stale exactly when its own day != today.
type usageMeter struct {
	day    string            // local calendar day the counters belong to ("2006-01-02")
	daily  map[string]int    // member -> input+output tokens spent today
	frozen map[string]string // member frozen BY THE BREAKER -> the day it tripped
}

// localDay is the meter's day key — the LOCAL calendar date, matching the
// operator's wall clock and the reviewer-at-midnight rhythm (timezone semantics
// per pkg/common: bare local, stamped elsewhere).
func localDay(t time.Time) string { return t.Local().Format("2006-01-02") }

// ensureMeterLocked lazily initialises the meter maps and resets the counters
// when the calendar day moved on. It does NOT unfreeze anyone — the supervisor's
// tick owns that (it must also unfreeze members that never run). Caller holds
// sp.mu.
func (sp *SwarmSpace) ensureMeterLocked(today string) {
	if sp.meter.daily == nil {
		sp.meter.daily = map[string]int{}
	}
	if sp.meter.frozen == nil {
		sp.meter.frozen = map[string]string{}
	}
	if sp.meter.day != today {
		sp.meter.day = today
		sp.meter.daily = map[string]int{}
	}
}

// BudgetFor resolves a member's effective daily token budget: a manifest
// member-level override wins (>0 = own cap, <0 = unlimited even when the space
// sets a default), otherwise the space-wide Settings.DailyBudgetTokens.
// 0 means unlimited. Exported so list_members can render "today X/Y".
func (sp *SwarmSpace) BudgetFor(name string) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if ov, ok := sp.budgets[name]; ok {
		switch {
		case ov > 0:
			return ov
		case ov < 0:
			return 0 // explicitly unlimited
		}
	}
	return sp.settings.DailyBudgetTokens
}

// addDailyUsage folds one run's token delta into the member's counter for
// `today` and reports the new daily total. Day rollover of the counters happens
// here too (a run can end right after midnight, before the tick sweep), but
// unfreezing stays with the supervisor's sweep.
func (sp *SwarmSpace) addDailyUsage(name string, tokens int, today string) (total int) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.ensureMeterLocked(today)
	if tokens > 0 {
		sp.meter.daily[name] += tokens
	}
	return sp.meter.daily[name]
}

// dailyFor returns a member's spend for the current meter day.
func (sp *SwarmSpace) dailyFor(name string) int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.meter.daily[name]
}

// markBudgetFrozen records that the breaker (not the operator) froze a member
// today. Reports whether the mark was new — the caller only trips (freeze +
// notify) on a fresh mark, so a re-run after a manual unfreeze re-trips exactly
// once.
func (sp *SwarmSpace) markBudgetFrozen(name string) (fresh bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	today := localDay(time.Now())
	sp.ensureMeterLocked(today)
	if _, held := sp.meter.frozen[name]; held {
		return false
	}
	sp.meter.frozen[name] = today
	return true
}

// clearBudgetFrozen drops the breaker mark, e.g. when the operator manually
// unfreezes a member ("let it run" overrides the breaker until it trips again).
func (sp *SwarmSpace) clearBudgetFrozen(name string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	delete(sp.meter.frozen, name)
}

// isBudgetFrozen reports whether the breaker currently holds this member.
func (sp *SwarmSpace) isBudgetFrozen(name string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	_, held := sp.meter.frozen[name]
	return held
}

// sweepMeter advances the counters to `today` (idempotent — a run ending right
// after midnight may already have done it via ensureMeterLocked) and — unless
// the space pins budget-frozen members with BudgetStayFrozen — returns the
// members whose breaker mark is from an EARLIER day, clearing those marks so
// the caller (the supervisor tick) can unfreeze them. Keying the release on
// each mark's own day, not on observing the day change, means a counter
// rollover stolen by another member's run can never strand a frozen member.
func (sp *SwarmSpace) sweepMeter(today string, stayFrozen bool) (unfreeze []string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.ensureMeterLocked(today)
	if stayFrozen {
		return nil
	}
	for name, day := range sp.meter.frozen {
		if day != today {
			unfreeze = append(unfreeze, name)
			delete(sp.meter.frozen, name)
		}
	}
	return unfreeze
}
