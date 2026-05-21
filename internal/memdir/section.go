// Section parsing and merge helpers for the auto-memory files.
//
// USER_PROFILE.md and per-project MEMORY.md both follow a "fixed set of H2
// sections" shape: every save call hands us the bodies for a subset of the
// allowed sections, and we merge them into the existing file while
// preserving order, untouched sections, and any incidental whitespace.
//
// We deliberately do NOT parse arbitrary markdown — only top-level "## "
// headings count as section boundaries. Nested headings, fenced code blocks
// containing "## ...", lists with "## " prefixes — all are treated as
// section body content. The trade-off: the parser is small and predictable,
// and the section bodies stay machine-mergeable. Real markdown ASTs would
// introduce a dependency and a class of bugs the tool description already
// rules out (sections are flat, edits are surgical).
package memdir

import (
	"fmt"
	"strings"
)

// UserProfileSections is the closed set of headings allowed in USER_PROFILE.md.
// Order is the canonical render order for newly-scaffolded files.
var UserProfileSections = []string{
	"Preferences",
	"Working style",
	"Recurring topics",
}

// ProjectMemorySections is the closed set of headings allowed in
// <APP_HOME>/projects/<key>/MEMORY.md. Adapted from the ref taxonomy
// (user / feedback / project / reference) — "user" lives in USER_PROFILE.md,
// the remaining three are restated in project-centric language.
var ProjectMemorySections = []string{
	"Project facts",
	"Decisions",
	"Open issues",
	"References",
}

// MergeSections updates `existing` so that each entry in `updates` replaces
// the body of its corresponding "## <heading>" section. Sections not in
// `updates` are preserved verbatim. Headings that exist in `allowed` but not
// in the file are appended in `allowed` order. Headings present in
// `updates` but not in `allowed` cause an error before any merge happens.
//
// Body semantics: an entry's value is treated as the new section body
// (trimmed of leading/trailing whitespace). A value of "" clears the body —
// useful when the model wants to drop a section's content while keeping the
// heading visible. To leave a section untouched, omit the key entirely.
func MergeSections(existing string, updates map[string]string, allowed []string) (string, error) {
	allowedSet := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		allowedSet[h] = true
	}
	for k := range updates {
		if !allowedSet[k] {
			return "", fmt.Errorf("memdir: section %q is not in the allowed set %v", k, allowed)
		}
	}

	parsed := parseSections(existing)

	// Apply updates to known sections.
	for h, body := range updates {
		parsed.set(h, body)
	}

	// Ensure every allowed heading exists in render order — missing
	// headings get scaffolded with an empty body so the file shape
	// stabilizes after the first save.
	for _, h := range allowed {
		if !parsed.has(h) {
			parsed.append(h, "")
		}
	}

	return parsed.render(), nil
}

// IndexSummary renders a compact one-line-per-section overview of the
// supplied memory content, suitable for cache-static system-prompt injection.
//
// For each "## " section in `content`, the output contains:
//
//	## <heading> — <first non-empty line, truncated to maxBodyChars>
//
// Sections with empty bodies render as `## <heading> — (empty)`. The result
// is meant to be small (a few hundred chars even for a fully-populated file)
// so the model sees what's recorded without paying the full file cost.
func IndexSummary(content string, maxBodyChars int) string {
	if maxBodyChars <= 0 {
		maxBodyChars = 80
	}
	parsed := parseSections(strings.TrimSpace(content))
	if len(parsed.order) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range parsed.order {
		body := strings.TrimSpace(parsed.bodies[h])
		first := firstNonEmptyLine(body)
		if first == "" {
			fmt.Fprintf(&b, "## %s — (empty)\n", h)
			continue
		}
		if len(first) > maxBodyChars {
			first = first[:maxBodyChars] + "…"
		}
		fmt.Fprintf(&b, "## %s — %s\n", h, first)
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- internal section parser --------------------------------------------

type sectionDoc struct {
	preamble string            // text before the first "## " (rarely non-empty; preserved as-is)
	order    []string          // heading order as encountered
	bodies   map[string]string // heading -> body (without the "## " line; trimmed of trailing whitespace)
}

func (d *sectionDoc) has(h string) bool {
	_, ok := d.bodies[h]
	return ok
}

func (d *sectionDoc) set(h, body string) {
	if !d.has(h) {
		d.append(h, body)
		return
	}
	d.bodies[h] = strings.TrimSpace(body)
}

func (d *sectionDoc) append(h, body string) {
	d.order = append(d.order, h)
	d.bodies[h] = strings.TrimSpace(body)
}

func (d *sectionDoc) render() string {
	var b strings.Builder
	if pre := strings.TrimSpace(d.preamble); pre != "" {
		b.WriteString(pre)
		b.WriteString("\n\n")
	}
	for i, h := range d.order {
		fmt.Fprintf(&b, "## %s\n", h)
		body := strings.TrimSpace(d.bodies[h])
		if body != "" {
			b.WriteString(body)
			b.WriteString("\n")
		}
		if i < len(d.order)-1 {
			b.WriteString("\n")
		}
	}
	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

// parseSections splits `content` into (preamble, ordered sections). A
// section starts at a line whose trimmed-left form begins with "## " and
// continues until the next such line or EOF. Headings deeper than H2 stay
// inside whatever H2 owns them. Empty input yields an empty doc.
func parseSections(content string) *sectionDoc {
	doc := &sectionDoc{bodies: map[string]string{}}
	if strings.TrimSpace(content) == "" {
		return doc
	}
	lines := strings.Split(content, "\n")
	var (
		curHeading string
		curBody    []string
		preamble   []string
	)
	flush := func() {
		if curHeading == "" {
			return
		}
		body := strings.TrimRight(strings.Join(curBody, "\n"), "\n\t ")
		if _, dup := doc.bodies[curHeading]; dup {
			// Duplicate heading: concatenate bodies with a blank line so
			// content isn't silently lost. Real files shouldn't hit this
			// path; defensive only.
			doc.bodies[curHeading] = strings.TrimSpace(doc.bodies[curHeading] + "\n\n" + body)
		} else {
			doc.order = append(doc.order, curHeading)
			doc.bodies[curHeading] = strings.TrimSpace(body)
		}
		curBody = curBody[:0]
	}
	for _, ln := range lines {
		if h, ok := h2Heading(ln); ok {
			flush()
			curHeading = h
			continue
		}
		if curHeading == "" {
			preamble = append(preamble, ln)
		} else {
			curBody = append(curBody, ln)
		}
	}
	flush()
	doc.preamble = strings.TrimSpace(strings.Join(preamble, "\n"))
	return doc
}

// h2Heading returns (heading, true) when `line` is "## <text>" with optional
// trailing whitespace. Deeper headings ("### ...") are NOT matched.
func h2Heading(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "## ") {
		return "", false
	}
	if strings.HasPrefix(trimmed, "### ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), true
}

func firstNonEmptyLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		t := strings.TrimSpace(ln)
		if t != "" {
			return t
		}
	}
	return ""
}
