package fs

import (
	"strings"
	"testing"
	"time"
)

func TestReadTracker_NotReadRejected(t *testing.T) {
	tr := NewReadTracker()
	ok, reason := tr.CanEdit("/x/y/z.go", time.Now())
	if ok {
		t.Fatal("CanEdit must reject unread path")
	}
	if !strings.Contains(reason, "has not been read") {
		t.Errorf("reason = %q, want 'has not been read' phrase", reason)
	}
}

func TestReadTracker_PartialViewRejected(t *testing.T) {
	tr := NewReadTracker()
	now := time.Now()
	tr.Record("/x/file", now, true)
	ok, reason := tr.CanEdit("/x/file", now)
	if ok {
		t.Fatal("CanEdit must reject partial-view read")
	}
	if !strings.Contains(reason, "partially read") {
		t.Errorf("reason = %q, want 'partially read' phrase", reason)
	}
}

func TestReadTracker_MtimeDriftRejected(t *testing.T) {
	tr := NewReadTracker()
	earlier := time.Now().Add(-time.Hour)
	tr.Record("/x/file", earlier, false)
	ok, reason := tr.CanEdit("/x/file", time.Now())
	if ok {
		t.Fatal("CanEdit must reject when file mtime advanced")
	}
	if !strings.Contains(reason, "modified since") {
		t.Errorf("reason = %q, want 'modified since' phrase", reason)
	}
}

func TestReadTracker_HappyPath(t *testing.T) {
	tr := NewReadTracker()
	mtime := time.Now()
	tr.Record("/x/file", mtime, false)
	ok, reason := tr.CanEdit("/x/file", mtime)
	if !ok {
		t.Fatalf("CanEdit must accept fresh full read; reason=%q", reason)
	}
}

func TestReadTracker_CanWriteMirrorsCanEdit(t *testing.T) {
	tr := NewReadTracker()
	mtime := time.Now()
	tr.Record("/x/file", mtime, false)
	if ok, _ := tr.CanWrite("/x/file", mtime); !ok {
		t.Fatal("CanWrite must accept what CanEdit accepts")
	}
	tr.Record("/x/file", mtime, true)
	if ok, _ := tr.CanWrite("/x/file", mtime); ok {
		t.Fatal("CanWrite must reject partial-view, same as CanEdit")
	}
}

func TestReadTracker_PathsCleanedConsistently(t *testing.T) {
	tr := NewReadTracker()
	mtime := time.Now()
	tr.Record("/x/./y/../file", mtime, false)
	if ok, _ := tr.CanEdit("/x/file", mtime); !ok {
		t.Fatal("recorded path /x/./y/../file should match lookup /x/file after Clean")
	}
}

func TestReadTracker_Forget(t *testing.T) {
	tr := NewReadTracker()
	now := time.Now()
	tr.Record("/x/file", now, false)
	tr.Forget("/x/file")
	if ok, _ := tr.CanEdit("/x/file", now); ok {
		t.Fatal("after Forget, CanEdit must report not-read")
	}
}

func TestReadTracker_ConcurrentAccess(t *testing.T) {
	tr := NewReadTracker()
	mtime := time.Now()
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(i int) {
			path := "/x/" + string(rune('a'+(i%26)))
			tr.Record(path, mtime, false)
			tr.CanEdit(path, mtime)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
