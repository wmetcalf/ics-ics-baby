package icsparse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseInline(t *testing.T, body string) *CalendarInfo {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.ics")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cal, err := ParseICSFile(p, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

const altDescICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:alt@test\r\nSUMMARY:Alt\r\n" +
	`X-ALT-DESC;FMTTYPE=TEXT/HTML:<p>a\, b\; c\\n</p>\n<b>x</b>` + "\r\n" +
	"X-ALT-DESC;FMTTYPE=text/plain:second\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

func TestAltDescRawUnescaped(t *testing.T) {
	cal := parseInline(t, altDescICS)
	if len(cal.Events) != 1 {
		t.Fatalf("events = %d", len(cal.Events))
	}
	ev := cal.Events[0]
	if ev.AltDescRaw == nil || *ev.AltDescRaw != "<p>a, b; c\\n</p>\n<b>x</b>" {
		t.Fatalf("AltDescRaw = %q", deref(ev.AltDescRaw))
	}
	if ev.AltDescFmtType == nil || *ev.AltDescFmtType != "text/html" {
		t.Fatalf("AltDescFmtType = %q", deref(ev.AltDescFmtType))
	}
}

func TestAltDescRawNotInManifest(t *testing.T) {
	cal := parseInline(t, altDescICS)
	b, err := json.Marshal(cal.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	b2, err := json.Marshal(cal)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{string(b), string(b2)} {
		if strings.Contains(s, "alt_desc_raw") || strings.Contains(s, "AltDescRaw") || strings.Contains(s, "AltDescFmtType") {
			t.Fatalf("raw alt desc leaked into JSON: %s", s)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
