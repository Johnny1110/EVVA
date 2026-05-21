package fs

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/johnny1110/evva/pkg/tools"
	"github.com/ledongthuc/pdf"
)

// pdfPageBudget caps how many pages a single read_file call may
// extract from a PDF. Matches Claude Code's 20-page cap so the
// per-call context cost stays bounded.
const pdfPageBudget = 20

// pdfRequirePagesAt is the page-count threshold above which the
// caller MUST supply a `pages` parameter. Mirrors the ref tool's
// "more than 10 pages" rule.
const pdfRequirePagesAt = 10

func readPDF(resolved string, pagesParam string) tools.Result {
	f, r, err := pdf.Open(resolved)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: could not open PDF %s: %s", resolved, err)}
	}
	defer f.Close()

	totalPages := r.NumPage()
	if totalPages <= 0 {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read: PDF %s has no pages.", resolved)}
	}

	var requested []int
	if pagesParam == "" {
		if totalPages > pdfRequirePagesAt {
			return tools.Result{
				IsError: true,
				Content: fmt.Sprintf(
					"read: PDF has %d pages — you must provide the `pages` parameter (e.g. \"1-5\") to read specific page ranges. Maximum %d pages per request.",
					totalPages, pdfPageBudget,
				),
			}
		}
		requested = makeRange(1, totalPages)
	} else {
		parsed, perr := parsePageRanges(pagesParam, totalPages)
		if perr != nil {
			return tools.Result{IsError: true, Content: "read: " + perr.Error()}
		}
		requested = parsed
	}

	if len(requested) > pdfPageBudget {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf(
				"read: requested %d pages exceeds the %d-page-per-call cap. Split into multiple read_file calls.",
				len(requested), pdfPageBudget,
			),
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "[PDF: %s (%d pages total), showing pages %s]\n", resolved, totalPages, formatPageList(requested))
	for _, pageNum := range requested {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			fmt.Fprintf(&out, "\n--- Page %d ---\n[page not found in PDF structure]\n", pageNum)
			continue
		}
		text, terr := page.GetPlainText(nil)
		if terr != nil {
			fmt.Fprintf(&out, "\n--- Page %d ---\n[extraction failed: %s]\n", pageNum, terr)
			continue
		}
		fmt.Fprintf(&out, "\n--- Page %d ---\n%s", pageNum, text)
		if !strings.HasSuffix(text, "\n") {
			out.WriteByte('\n')
		}
	}

	return tools.Result{Content: out.String()}
}

// parsePageRanges parses a Claude Code-style `pages` string like
// "1-5,7,10-12" into a deduplicated, sorted list of 1-based page
// numbers. Bounds are clamped against totalPages; any number out of
// range is an error.
func parsePageRanges(spec string, totalPages int) ([]int, error) {
	seen := make(map[int]struct{})
	for _, raw := range strings.Split(spec, ",") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		lo, hi, err := splitRange(part)
		if err != nil {
			return nil, fmt.Errorf("invalid pages range %q: %w", part, err)
		}
		if lo < 1 || hi < 1 {
			return nil, fmt.Errorf("page numbers must be >= 1 (got %q)", part)
		}
		if lo > totalPages || hi > totalPages {
			return nil, fmt.Errorf("page range %q exceeds document length (%d pages)", part, totalPages)
		}
		if lo > hi {
			return nil, fmt.Errorf("page range %q is reversed (lo > hi)", part)
		}
		for n := lo; n <= hi; n++ {
			seen[n] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("no pages specified in %q", spec)
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func splitRange(part string) (int, int, error) {
	if idx := strings.IndexByte(part, '-'); idx >= 0 {
		lo, err := strconv.Atoi(strings.TrimSpace(part[:idx]))
		if err != nil {
			return 0, 0, err
		}
		hi, err := strconv.Atoi(strings.TrimSpace(part[idx+1:]))
		if err != nil {
			return 0, 0, err
		}
		return lo, hi, nil
	}
	n, err := strconv.Atoi(part)
	if err != nil {
		return 0, 0, err
	}
	return n, n, nil
}

func makeRange(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for n := lo; n <= hi; n++ {
		out = append(out, n)
	}
	return out
}

// formatPageList renders a sorted page list back into the compact
// "1-3,5,8-9" form for the file header. Useful for the model so it
// can see exactly which pages it received.
func formatPageList(pages []int) string {
	if len(pages) == 0 {
		return ""
	}
	var parts []string
	start := pages[0]
	prev := pages[0]
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for i := 1; i < len(pages); i++ {
		if pages[i] == prev+1 {
			prev = pages[i]
			continue
		}
		flush()
		start = pages[i]
		prev = pages[i]
	}
	flush()
	return strings.Join(parts, ",")
}
