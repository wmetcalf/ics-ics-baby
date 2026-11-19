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

// Every non-empty FMTTYPE=text/html X-ALT-DESC is kept for the view.
func TestAltDescHTMLParts(t *testing.T) {
	cal := parseInline(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:p@test\r\nSUMMARY:x\r\n"+
		"X-ALT-DESC;FMTTYPE=text/plain:See HTML version\r\nX-ALT-DESC;FMTTYPE=text/html: \r\n"+
		"X-ALT-DESC;FMTTYPE=text/html:<a href=\"https://evil.example/p\">here</a>\r\nX-ALT-DESC;FMTTYPE=TEXT/HTML:<script>x</script>\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	ev := cal.Events[0]
	if len(ev.AltDescHTMLParts) != 2 || !strings.Contains(ev.AltDescHTMLParts[0], "evil.example") {
		t.Fatalf("parts = %q", ev.AltDescHTMLParts)
	}
	if ev.AltDescRaw == nil || *ev.AltDescRaw != "See HTML version" {
		t.Fatalf("AltDescRaw = %q", deref(ev.AltDescRaw))
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

func TestOrganizerCNFollowsLastOrganizer(t *testing.T) {
	cal := parseInline(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:o@test\r\nSUMMARY:x\r\n"+
		"ORGANIZER;CN=\"IT Helpdesk\":mailto:help@corp.example\r\nORGANIZER:mailto:attacker@evil.example\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	if ev := cal.Events[0]; ev.OrganizerCN != nil {
		t.Fatalf("CN %q paired with another ORGANIZER's address", *ev.OrganizerCN)
	}
}
