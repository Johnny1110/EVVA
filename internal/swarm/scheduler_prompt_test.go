package swarm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/johnny1110/evva/internal/swarm/store"
	"github.com/johnny1110/evva/pkg/common"
)

// Every wall-clock string handed to a member must carry an explicit UTC
// offset: a zone-less stamp reads as UTC to the model, which misreads it by
// the local offset in any non-UTC deployment (the PRD-001 phantom-skew bug).

func TestScheduledWakePromptCarriesOffset(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 25, 0, 0, time.FixedZone("HKT", 8*3600))
	p := scheduledWakePrompt(now, "do the patrol", "")
	want := fmt.Sprintf("<system-reminder>currenttime: %s, do the patrol</system-reminder>", common.Stamp(now))
	if p != want {
		t.Errorf("scheduledWakePrompt = %q, want %q", p, want)
	}
	if !strings.Contains(p, "currenttime: ") {
		t.Errorf("wake prompt lost the currenttime marker: %q", p)
	}
}

func TestScheduledWakePromptFallsBackToStandingDuty(t *testing.T) {
	p := scheduledWakePrompt(time.Now(), "   ", "")
	if !strings.Contains(p, scheduledDutyPrompt) {
		t.Errorf("blank prompt should fall back to the standing-duty text: %q", p)
	}
}

// RP-25: a member's memory index rides INSIDE the wake <system-reminder> —
// same channel as the wall clock, for the same bit-stable-prefix reason — and
// an empty index leaves the reminder byte-identical to the pre-RP-25 form.
func TestWakePromptsCarryMemoryIndex(t *testing.T) {
	now := time.Date(2026, 6, 11, 9, 0, 0, 0, time.FixedZone("HKT", 8*3600))
	idx := "Your memory index (agents/sub/w/memory/MEMORY.md — read a linked file before relying on it):\n- [Lead](lead.md) — open lead"

	p := scheduledWakePrompt(now, "patrol", idx)
	if !strings.Contains(p, idx) {
		t.Errorf("timer wake missing the memory index:\n%s", p)
	}
	if !strings.HasSuffix(p, "</system-reminder>") || strings.Index(p, idx) > strings.Index(p, "</system-reminder>") {
		t.Errorf("memory index must sit INSIDE the wake reminder:\n%s", p)
	}

	m := composeMailPrompt(now, []store.Message{{Sender: "lead", Body: "go"}}, idx)
	head, _, ok := strings.Cut(m, "</system-reminder>")
	if !ok || !strings.Contains(head, idx) {
		t.Errorf("mail wake must carry the index inside the opening reminder:\n%s", m)
	}

	// Empty index → byte-identical to the index-less form (zero wake noise).
	if scheduledWakePrompt(now, "patrol", "") != strings.Replace(p, "\n\n"+idx, "", 1) {
		t.Errorf("empty index should leave the timer wake unchanged")
	}
}

func TestComposeMailPromptCarriesTimes(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 25, 0, 0, time.FixedZone("HKT", 8*3600))
	sent := time.Date(2026, 6, 10, 11, 0, 0, 0, time.FixedZone("HKT", 8*3600))
	batch := []store.Message{
		{Sender: "friday", Subject: "risk", Body: "check exposure", CreatedAt: sent.UnixMilli()},
		{Sender: "user", Body: "no timestamp on this one"}, // CreatedAt zero — no [sent] marker
	}
	p := composeMailPrompt(now, batch, "")

	if want := fmt.Sprintf("<system-reminder>currenttime: %s</system-reminder>", common.Stamp(now)); !strings.Contains(p, want) {
		t.Errorf("mail prompt missing currenttime header %q:\n%s", want, p)
	}
	if want := fmt.Sprintf("--- Message from friday (re: risk) [sent %s] ---", common.Stamp(sent)); !strings.Contains(p, want) {
		t.Errorf("mail prompt missing sent stamp %q:\n%s", want, p)
	}
	if strings.Contains(p, "from user (re:") || strings.Contains(p, "user [sent") {
		t.Errorf("zero CreatedAt must not render a sent stamp:\n%s", p)
	}
	if !strings.Contains(p, "check exposure") || !strings.Contains(p, "no timestamp on this one") {
		t.Errorf("mail bodies missing:\n%s", p)
	}
}
