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

// The view's body follows the preview: a non-empty HTML X-ALT-DESC wins
// over a plain or empty one, whichever comes first.
func TestAltDescRawPrefersHTML(t *testing.T) {
	for name, props := range map[string]string{
		"plain first": "X-ALT-DESC;FMTTYPE=text/plain:See HTML version\r\nX-ALT-DESC;FMTTYPE=text/html:<a href=\"https://evil.example/p\">here</a>\r\n",
		"empty first": "X-ALT-DESC;FMTTYPE=text/html: \r\nX-ALT-DESC;FMTTYPE=text/html:<a href=\"https://evil.example/p\">here</a>\r\n",
	} {
		cal := parseInline(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:p@test\r\nSUMMARY:x\r\n"+props+"END:VEVENT\r\nEND:VCALENDAR\r\n")
		ev := cal.Events[0]
		if ev.AltDescRaw == nil || !strings.Contains(*ev.AltDescRaw, "evil.example") || ev.AltDescFmtType == nil || *ev.AltDescFmtType != "text/html" {
			t.Fatalf("%s: AltDescRaw = %q (%q)", name, deref(ev.AltDescRaw), deref(ev.AltDescFmtType))
		}
	}
}

// The organizer's display name is known whether or not an alert fired.
func TestOrganizerCNWithoutSignals(t *testing.T) {
	cal := parseInline(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:c@test\r\nSUMMARY:x\r\n"+
		"ORGANIZER;CN=\"Microsoft Security Team\":mailto:attacker@evil.example\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	ev := cal.Events[0]
	if ev.OrganizerCN == nil || *ev.OrganizerCN != "Microsoft Security Team" {
		t.Fatalf("OrganizerCN = %q", deref(ev.OrganizerCN))
	}
}

// A trailing HTML part that sanitizes to nothing does not replace the body
// the preview shows.
func TestAltDescRawFollowsPreview(t *testing.T) {
	cal := parseInline(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:q@test\r\nSUMMARY:x\r\n"+
		"X-ALT-DESC;FMTTYPE=text/html:<p>Quarterly review agenda</p>\r\nX-ALT-DESC;FMTTYPE=text/html:<script>x</script>\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	if ev := cal.Events[0]; ev.AltDescRaw == nil || !strings.Contains(*ev.AltDescRaw, "Quarterly review agenda") {
		t.Fatalf("AltDescRaw = %q", deref(ev.AltDescRaw))
	}
}
