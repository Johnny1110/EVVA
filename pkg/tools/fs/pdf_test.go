package fs

import (
	"reflect"
	"strings"
	"testing"
)

// Note: parsePageRanges is the easy half to test cleanly without a
// real PDF fixture; full end-to-end PDF reading is exercised through
// the smoke test in the verification pass.

func TestParsePageRanges_SingleNumber(t *testing.T) {
	got, err := parsePageRanges("3", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{3}) {
		t.Errorf("got %v, want [3]", got)
	}
}

func TestParsePageRanges_Range(t *testing.T) {
	got, err := parsePageRanges("1-5", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("got %v, want [1..5]", got)
	}
}

func TestParsePageRanges_Mixed(t *testing.T) {
	got, err := parsePageRanges("1-3,5,8-9", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 5, 8, 9}) {
		t.Errorf("got %v, want [1,2,3,5,8,9]", got)
	}
}

func TestParsePageRanges_DedupAndSort(t *testing.T) {
	got, err := parsePageRanges("5,1-3,2,7", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 5, 7}) {
		t.Errorf("got %v, want sorted/deduped [1,2,3,5,7]", got)
	}
}

func TestParsePageRanges_OutOfBoundsRejected(t *testing.T) {
	if _, err := parsePageRanges("11", 10); err == nil {
		t.Error("expected error for page beyond doc length")
	}
	if _, err := parsePageRanges("1-12", 10); err == nil {
		t.Error("expected error for range exceeding doc length")
	}
}

func TestParsePageRanges_ZeroOrNegativeRejected(t *testing.T) {
	if _, err := parsePageRanges("0", 10); err == nil {
		t.Error("expected error for page 0")
	}
	if _, err := parsePageRanges("-2", 10); err == nil {
		t.Error("expected error for negative page")
	}
}

func TestParsePageRanges_ReversedRangeRejected(t *testing.T) {
	if _, err := parsePageRanges("5-2", 10); err == nil {
		t.Error("expected error for reversed range")
	}
}

func TestParsePageRanges_Empty(t *testing.T) {
	if _, err := parsePageRanges("", 10); err == nil {
		t.Error("expected error for empty pages string")
	}
}

func TestFormatPageList(t *testing.T) {
	cases := []struct {
		in   []int
		want string
	}{
		{[]int{1, 2, 3}, "1-3"},
		{[]int{1, 3, 5}, "1,3,5"},
		{[]int{1, 2, 3, 5, 8, 9}, "1-3,5,8-9"},
		{[]int{7}, "7"},
		{[]int{}, ""},
	}
	for _, c := range cases {
		got := formatPageList(c.in)
		if got != c.want {
			t.Errorf("formatPageList(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReadPDF_BadFileError(t *testing.T) {
	res := readPDF("/no/such/path.pdf", "")
	if !res.IsError {
		t.Fatal("expected error for missing PDF")
	}
	if !strings.Contains(res.Content, "could not open PDF") {
		t.Errorf("error should mention 'could not open PDF'; got %q", res.Content)
	}
}
