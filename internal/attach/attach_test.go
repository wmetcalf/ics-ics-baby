package attach

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"ics-ics-baby/internal/icsparse"
)

func threeAttachments(t *testing.T) *icsparse.CalendarInfo {
	t.Helper()
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a@test\r\nSUMMARY:att\r\n")
	for i := 0; i < 3; i++ {
		b.WriteString("ATTACH;FMTTYPE=text/plain;ENCODING=BASE64;VALUE=BINARY:aGVsbG8=\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	p := filepath.Join(t.TempDir(), "a.ics")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cal, err := icsparse.ParseICSFile(p, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

func savedFiles(t *testing.T, out string) int {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(out, "ics-ics-baby-attachments"))
	if err != nil {
		t.Fatal(err)
	}
	return len(ents)
}

// Gap attachment-count: a per-calendar cap on saved attachment files.
func TestExtractAllLimited(t *testing.T) {
	cal := threeAttachments(t)
	out := t.TempDir()
	err := ExtractAllLimited(cal, out, false, 1<<20, 2)
	if err == nil || !strings.Contains(err.Error(), "attachment limit") {
		t.Fatalf("err = %v, want attachment limit", err)
	}
	if n := savedFiles(t, out); n != 2 {
		t.Fatalf("saved %d files, want 2", n)
	}
	if cal.Events[0].Attachments[2].SavedAs != nil {
		t.Fatal("third attachment recorded as saved")
	}

	cal = threeAttachments(t)
	out = t.TempDir()
	if err := ExtractAll(cal, out, false, 1<<20); err != nil {
		t.Fatal(err)
	}
	if n := savedFiles(t, out); n != 3 {
		t.Fatalf("unlimited saved %d files, want 3", n)
	}
}

// The cap bounds attempts, not just successes: failing downloads must not
// let a hostile calendar make unlimited requests.
func TestExtractAllLimitedCountsFailedDownloads(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:d@test\r\nSUMMARY:dl\r\n")
	for i := 0; i < 10; i++ {
		b.WriteString("ATTACH:" + srv.URL + "/f" + string(rune('0'+i)) + ".bin\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	p := filepath.Join(t.TempDir(), "d.ics")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cal, err := icsparse.ParseICSFile(p, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	_ = ExtractAllLimited(cal, t.TempDir(), true, 1<<20, 2)
	if n := hits.Load(); n != 2 {
		t.Fatalf("server saw %d requests with --max-attachments 2", n)
	}
}
