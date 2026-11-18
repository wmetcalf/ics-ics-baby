package clientview

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ics-ics-baby/internal/icsparse"
)

const suiteDir = "../../ics-parser-test-suite/ics_files/"

func parseFile(t *testing.T, path string) *icsparse.CalendarInfo {
	t.Helper()
	cal, err := icsparse.ParseICSFile(path, "", 1<<20)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cal
}

func parseICS(t *testing.T, body string) *icsparse.CalendarInfo {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.ics")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return parseFile(t, p)
}

// invite builds a one-event calendar; extra holds raw VEVENT lines.
func invite(method string, extra ...string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\n")
	if method != "" {
		b.WriteString("METHOD:" + method + "\r\n")
	}
	b.WriteString("BEGIN:VEVENT\r\nUID:t@test\r\nDTSTART:20260218T100000Z\r\nDTEND:20260218T110000Z\r\nSUMMARY:Quarterly review\r\n")
	b.WriteString("ORGANIZER;CN=\"Pat Doe\":mailto:pat@example.test\r\n")
	for _, l := range extra {
		b.WriteString(l + "\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return b.String()
}

func writeOWA(t *testing.T, cal *icsparse.CalendarInfo) (Sidecar, string, string) {
	t.Helper()
	dir := t.TempDir()
	sc, err := WriteOWA(cal, "abc123", dir)
	if err != nil {
		t.Fatalf("WriteOWA: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ics-ics-baby-owa.html"))
	if err != nil {
		t.Fatalf("read owa html: %v", err)
	}
	return sc, string(b), dir
}

func TestSelectEventMethods(t *testing.T) {
	for _, f := range []string{"01-basic-needs-action.ics", "04-publish-autoadd.ics"} {
		ev, canceled, reason := SelectEvent(parseFile(t, suiteDir+f))
		if ev == nil || canceled {
			t.Fatalf("%s: ev=%v canceled=%v reason=%q", f, ev, canceled, reason)
		}
	}
	if ev, _, reason := SelectEvent(parseICS(t, invite(""))); ev == nil {
		t.Fatalf("no METHOD: not selected (%q)", reason)
	}
	if ev, canceled, _ := SelectEvent(parseFile(t, suiteDir+"16-cancel-preaccepted.ics")); ev == nil || !canceled {
		t.Fatalf("CANCEL: ev=%v canceled=%v", ev, canceled)
	}
	if ev, _, reason := SelectEvent(parseFile(t, suiteDir+"20-reply-spoof.ics")); ev != nil || !strings.Contains(reason, "REPLY") {
		t.Fatalf("REPLY: ev=%v reason=%q", ev, reason)
	}
	if ev, _, reason := SelectEvent(parseFile(t, "../icsparse/testdata/journal.ics")); ev != nil || reason != "no VEVENT" {
		t.Fatalf("journal: ev=%v reason=%q", ev, reason)
	}
}

func TestJoinURLPrecedence(t *testing.T) {
	ev := &icsparse.EventInfo{
		RawProps: map[string][]string{
			"X-MICROSOFT-SKYPETEAMSMEETINGURL":  {"https://teams.example.test/l/meetup"},
			"X-MICROSOFT-ONLINEMEETINGCONFLINK": {"https://conf.example.test/x"},
		},
		Conferences: []icsparse.ConferenceInfo{{URI: "https://meet.example.test/abc"}},
	}
	if raw, active := JoinURL(ev); raw != "https://teams.example.test/l/meetup" || !active {
		t.Fatalf("teams: %q %v", raw, active)
	}
	delete(ev.RawProps, "X-MICROSOFT-SKYPETEAMSMEETINGURL")
	if raw, _ := JoinURL(ev); raw != "https://conf.example.test/x" {
		t.Fatalf("conflink: %q", raw)
	}
	delete(ev.RawProps, "X-MICROSOFT-ONLINEMEETINGCONFLINK")
	if raw, _ := JoinURL(ev); raw != "https://meet.example.test/abc" {
		t.Fatalf("conference: %q", raw)
	}
}

func TestJoinURLNonHTTPInert(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "data:text/html,hi", "file:///etc/passwd"} {
		cal := parseICS(t, invite("REQUEST", "X-MICROSOFT-SKYPETEAMSMEETINGURL:"+u))
		if raw, active := JoinURL(&cal.Events[0]); raw != u || active {
			t.Fatalf("%s: raw=%q active=%v", u, raw, active)
		}
		sc, html, _ := writeOWA(t, cal)
		if sc.JoinURL != u {
			t.Fatalf("sidecar join_url = %q", sc.JoinURL)
		}
		if regexp.MustCompile(`href="(javascript|data|file):`).MatchString(html) {
			t.Fatalf("%s rendered as live link", u)
		}
		if !strings.Contains(html, strings.ReplaceAll(u, "/", "/")) {
			t.Fatalf("%s not shown as text", u)
		}
	}
}

func TestOWAHTMLContents(t *testing.T) {
	sc, html, _ := writeOWA(t, parseFile(t, suiteDir+"09-combo-spoof-urgent.ics"))
	if sc.Status != "ok" || sc.Profile != "owa" || !sc.Emulation {
		t.Fatalf("sidecar %+v", sc)
	}
	for _, want := range []string{"CRITICAL: Action Required Immediately", "Executive", "exec@yourcompany.com",
		"Emulated Outlook-on-the-web rendering — ics-ics-baby", "abc123"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, label := range []string{"Accept", "Tentative", "Decline", "Propose new time"} {
		if !regexp.MustCompile(`<span[^>]*>\s*` + label + `\s*</span>`).MatchString(html) {
			t.Errorf("RSVP %q not an inert span", label)
		}
	}
	if regexp.MustCompile(`<a[^>]*>\s*(Accept|Tentative|Decline|Propose new time)`).MatchString(html) {
		t.Error("RSVP rendered as a link")
	}
	if n := strings.Count(html, "Outlook"); n != 1 {
		t.Errorf("'Outlook' appears %d times; only the footer may name it", n)
	}
	if strings.Contains(html, "Microsoft") {
		t.Error("template names Microsoft")
	}
	if regexp.MustCompile(`(?i)<img[^>]*logo`).MatchString(html) {
		t.Error("logo image present")
	}

	// With a meeting URL: a Join button labeled with the real target host.
	_, html, _ = writeOWA(t, parseICS(t, invite("REQUEST", "X-MICROSOFT-SKYPETEAMSMEETINGURL:https://evil.test/join?x=1")))
	if !regexp.MustCompile(`class="owa-join"[^>]*href="https://evil.test/join\?x=1"|href="https://evil.test/join\?x=1"[^>]*class="owa-join"`).MatchString(html) {
		t.Fatalf("no Join button: %s", html)
	}
	if !strings.Contains(html, `class="owa-join-host">evil.test<`) {
		t.Fatal("Join host label missing")
	}
}

func TestOWACanceledBanner(t *testing.T) {
	_, html, _ := writeOWA(t, parseFile(t, suiteDir+"16-cancel-preaccepted.ics"))
	if !strings.Contains(html, "Canceled: Benign Cancel Test (was accepted)") {
		t.Fatal("subject not prefixed with Canceled:")
	}
	if !strings.Contains(html, `class="owa-cancel-banner"`) {
		t.Fatal("no cancel banner")
	}
}

func TestBodyPlainWhenNotHTML(t *testing.T) {
	cases := map[string][]string{
		"plain fmttype": {`X-ALT-DESC;FMTTYPE=text/plain:<b>bold?</b> <img src=http://evil.test/x>`},
		"no fmttype":    {`X-ALT-DESC:<b>bold?</b> <img src=http://evil.test/x>`},
		"no alt desc":   {`DESCRIPTION:<b>bold?</b> <img src=http://evil.test/x>`},
	}
	for name, lines := range cases {
		sc, html, _ := writeOWA(t, parseICS(t, invite("REQUEST", lines...)))
		if !regexp.MustCompile(`<pre class="owa-plain">[^<]*&lt;b&gt;`).MatchString(html) &&
			!regexp.MustCompile(`<pre class="owa-plain">[^<]*bold\?`).MatchString(html) {
			t.Errorf("%s: body not escaped preformatted text: %s", name, html)
		}
		if strings.Contains(html, "<b>bold") || strings.Contains(html, "<img") {
			t.Errorf("%s: body parsed as HTML", name)
		}
		if len(sc.RemoteRefs) != 0 {
			t.Errorf("%s: remote refs %v", name, sc.RemoteRefs)
		}
	}
}

func TestOWAMultipleEvents(t *testing.T) {
	sc, html, _ := writeOWA(t, parseFile(t, suiteDir+"15-multiple-events.ics"))
	if !strings.Contains(html, "Morning Benign Meeting A") {
		t.Fatal("first event not rendered")
	}
	if len(sc.OtherEvents) != 1 || sc.OtherEvents[0] != "Afternoon Benign Meeting B" {
		t.Fatalf("OtherEvents = %v", sc.OtherEvents)
	}
}

func TestOWAHTMLBodySanitizedAndSidecarJSON(t *testing.T) {
	cal := parseICS(t, invite("REQUEST", `X-ALT-DESC;FMTTYPE=text/html:<p>Hi</p><img src="http://evil.test/p.gif"><script>x()</script>`))
	sc, html, _ := writeOWA(t, cal)
	if strings.Contains(html, "<script>x()") || !strings.Contains(html, "remote image: evil.test") {
		t.Fatalf("body not sanitized: %s", html)
	}
	if len(sc.RemoteRefs) != 1 || sc.RemoteRefs[0].Host != "evil.test" {
		t.Fatalf("refs %v", sc.RemoteRefs)
	}
	b, _ := json.Marshal(sc)
	for _, k := range []string{`"remote_refs":[`, `"links":[]`, `"dropped":[`, `"blocked_requests":[]`, `"emulation":true`, `"profile":"owa"`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("sidecar JSON lacks %s: %s", k, b)
		}
	}
}

func TestOWASkippedWritesReasonPage(t *testing.T) {
	sc, html, _ := writeOWA(t, parseFile(t, suiteDir+"20-reply-spoof.ics"))
	if sc.Status != "skipped" || !strings.Contains(sc.Reason, "REPLY") {
		t.Fatalf("sidecar %+v", sc)
	}
	if !strings.Contains(html, sc.Reason) {
		t.Fatal("skipped page does not state the reason")
	}
}

// Deferred minor: when building the view fails, an HTML page stating the
// failure is still written, consistent with a render_failed sidecar.
func TestOWAFailurePageWritten(t *testing.T) {
	orig := owaSanitize
	owaSanitize = func(string, string) (SanitizeResult, error) { return SanitizeResult{}, errors.New("boom in sanitizer") }
	defer func() { owaSanitize = orig }()
	cal := parseICS(t, invite("REQUEST", `X-ALT-DESC;FMTTYPE=text/html:<p>x</p>`))
	dir := t.TempDir()
	sc, err := WriteOWA(cal, "abc", dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if sc.Status != "render_failed" || !strings.Contains(sc.Error, "boom in sanitizer") {
		t.Fatalf("sidecar %+v", sc)
	}
	b, rerr := os.ReadFile(filepath.Join(dir, OWAHTMLName))
	if rerr != nil {
		t.Fatalf("no HTML written: %v", rerr)
	}
	if !strings.Contains(string(b), "boom in sanitizer") || !strings.Contains(string(b), "Emulated Outlook-on-the-web rendering") {
		t.Fatalf("failure page lacks reason/footer: %s", b)
	}
}
