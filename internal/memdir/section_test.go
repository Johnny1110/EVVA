package memdir

import (
	"strings"
	"testing"
)

func TestMergeSectionsScaffoldsAllowedHeadings(t *testing.T) {
	got, err := MergeSections("", map[string]string{}, UserProfileSections)
	if err != nil {
		t.Fatalf("MergeSections: %v", err)
	}
	for _, h := range UserProfileSections {
		if !strings.Contains(got, "## "+h) {
			t.Errorf("scaffold missing heading %q in:\n%s", h, got)
		}
	}
}

func TestMergeSectionsUpdatesOnlyTargeted(t *testing.T) {
	existing := "## Preferences\noriginal-pref\n\n## Working style\noriginal-ws\n\n## Recurring topics\noriginal-rt\n"
	got, err := MergeSections(existing, map[string]string{
		"Working style": "new-ws",
	}, UserProfileSections)
	if err != nil {
		t.Fatalf("MergeSections: %v", err)
	}
	if !strings.Contains(got, "original-pref") {
		t.Errorf("Preferences body clobbered:\n%s", got)
	}
	if !strings.Contains(got, "new-ws") || strings.Contains(got, "original-ws") {
		t.Errorf("Working style not updated:\n%s", got)
	}
	if !strings.Contains(got, "original-rt") {
		t.Errorf("Recurring topics body clobbered:\n%s", got)
	}
}

func TestMergeSectionsRejectsUnknownHeading(t *testing.T) {
	_, err := MergeSections("", map[string]string{
		"Bogus heading": "nope",
	}, UserProfileSections)
	if err == nil {
		t.Fatal("expected error for unknown section")
	}
}

func TestMergeSectionsIdempotentOnNoop(t *testing.T) {
	first, _ := MergeSections("", map[string]string{
		"Preferences": "a",
	}, UserProfileSections)
	second, _ := MergeSections(first, map[string]string{}, UserProfileSections)
	if first != second {
		t.Errorf("expected idempotence;\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestMergeSectionsEmptyBodyClearsSection(t *testing.T) {
	existing := "## Preferences\nold\n"
	got, err := MergeSections(existing, map[string]string{
		"Preferences": "",
	}, UserProfileSections)
	if err != nil {
		t.Fatalf("MergeSections: %v", err)
	}
	if strings.Contains(got, "old") {
		t.Errorf("expected Preferences body cleared:\n%s", got)
	}
	if !strings.Contains(got, "## Preferences") {
		t.Errorf("heading should still appear:\n%s", got)
	}
}

func TestIndexSummaryRendersFirstLineOnly(t *testing.T) {
	content := "## Project facts\nuses goimports, not gofmt\nMore detail here\n\n## Decisions\n\n## Open issues\nfoo\n"
	got := IndexSummary(content, 80)
	if !strings.Contains(got, "## Project facts — uses goimports") {
		t.Errorf("expected first-line excerpt; got:\n%s", got)
	}
	if !strings.Contains(got, "## Decisions — (empty)") {
		t.Errorf("expected empty marker; got:\n%s", got)
	}
	if strings.Contains(got, "More detail here") {
		t.Errorf("only first non-empty line should appear; got:\n%s", got)
	}
}

func TestIndexSummaryEmptyInput(t *testing.T) {
	if got := IndexSummary("", 80); got != "" {
		t.Errorf("expected empty summary for empty input, got %q", got)
	}
}
