package icsparse

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// checkMirror asserts findings mirror the alert strings one-to-one, in order:
// info-severity findings are the informational patterns, the rest suspicious.
func checkMirror(t *testing.T, where string, sig *AutoprocessingSignals, fs []Finding) {
	t.Helper()
	var sus, info []string
	if sig != nil {
		sus, info = sig.SuspiciousPatterns, sig.InformationalPatterns
	}
	if len(fs) != len(sus)+len(info) {
		t.Fatalf("%s: %d findings for %d+%d alerts", where, len(fs), len(sus), len(info))
	}
	var gotSus, gotInfo []string
	for _, f := range fs {
		meta, ok := findingCodes[f.Code]
		if !ok {
			t.Fatalf("%s: code %q not in the code table", where, f.Code)
		}
		if f.Severity != meta.Severity {
			t.Fatalf("%s: %s severity %q, table says %q", where, f.Code, f.Severity, meta.Severity)
		}
		if f.Severity == "info" {
			gotInfo = append(gotInfo, f.Message)
		} else {
			gotSus = append(gotSus, f.Message)
		}
	}
	if strings.Join(gotSus, "\x00") != strings.Join(sus, "\x00") {
		t.Fatalf("%s: suspicious messages differ\n got %q\nwant %q", where, gotSus, sus)
	}
	if strings.Join(gotInfo, "\x00") != strings.Join(info, "\x00") {
		t.Fatalf("%s: informational messages differ\n got %q\nwant %q", where, gotInfo, info)
	}
}

func TestEveryAlertHasCode(t *testing.T) {
	files, err := filepath.Glob("../../ics-parser-test-suite/ics_files/*.ics")
	if err != nil || len(files) != 20 {
		t.Fatalf("suite files: %v (%d)", err, len(files))
	}
	extra, _ := filepath.Glob("testdata/*.ics")
	total := 0
	for _, f := range append(files, extra...) {
		cal, err := ParseICSFile(f, "", 1<<20)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		checkMirror(t, f+" (calendar)", cal.AutoprocessingSignals, cal.Findings)
		total += len(cal.Findings)
		for i, ev := range cal.Events {
			checkMirror(t, f+" event "+ev.Summary, ev.AutoprocessingSignals, ev.Findings)
			total += len(ev.Findings)
			for _, fd := range ev.Findings {
				if ev.UID != nil && fd.EventUID != *ev.UID {
					t.Fatalf("%s event %d: finding uid %q != %q", f, i, fd.EventUID, *ev.UID)
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("no findings across the suite")
	}
}

func TestTeamsHijackFinding(t *testing.T) {
	cal := parseInline(t, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:th@test\r\nSUMMARY:x\r\n"+
		"X-MICROSOFT-SKYPETEAMSMEETINGURL:https://evil.test/join\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	var found *Finding
	for i := range cal.Events[0].Findings {
		if cal.Events[0].Findings[i].Code == "teams_url_hijack" {
			found = &cal.Events[0].Findings[i]
		}
	}
	if found == nil || found.Severity != "high" || found.Evidence["url"] != "https://evil.test/join" || found.EventUID != "th@test" {
		t.Fatalf("teams finding = %+v (all: %+v)", found, cal.Events[0].Findings)
	}
}

func TestFindingsNotInManifest(t *testing.T) {
	cal, err := ParseICSFile("../../ics-parser-test-suite/ics_files/09-combo-spoof-urgent.ics", "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Events[0].Findings) == 0 {
		t.Fatal("fixture should have findings")
	}
	b, _ := json.Marshal(cal.Manifest())
	if strings.Contains(string(b), `"findings"`) || strings.Contains(string(b), `"Findings"`) {
		t.Fatal("findings leaked into manifest")
	}
}

func TestFindingsDoc(t *testing.T) {
	cal, err := ParseICSFile("../../ics-parser-test-suite/ics_files/09-combo-spoof-urgent.ics", "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	doc := FindingsDoc(cal, "deadbeef")
	if doc["schema_version"] != "ics-ics-baby-findings/1" || doc["input_sha256"] != "deadbeef" {
		t.Fatalf("doc header %v", doc)
	}
	fs, ok := doc["findings"].([]Finding)
	if !ok || len(fs) != len(cal.Findings)+len(cal.Events[0].Findings) {
		t.Fatalf("doc findings %T len %d", doc["findings"], len(fs))
	}
	empty := FindingsDoc(&CalendarInfo{}, "x")
	if b, _ := json.Marshal(empty); !strings.Contains(string(b), `"findings":[]`) {
		t.Fatalf("empty doc: %s", b)
	}
}

// TestEngineContractListsEveryCode keeps blastbox-engine.toml's [[detections]]
// in sync with the code table (codes and severities).
func TestEngineContractListsEveryCode(t *testing.T) {
	b, err := os.ReadFile("../../blastbox-engine.toml")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\[\[detections\]\]\ncode = "([^"]+)"\nseverity = "([^"]+)"`)
	got := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		got[m[1]] = m[2]
	}
	if len(got) != len(findingCodes) {
		t.Fatalf("contract lists %d codes, table has %d", len(got), len(findingCodes))
	}
	for code, meta := range findingCodes {
		if got[code] != meta.Severity {
			t.Errorf("contract %s severity %q, table %q", code, got[code], meta.Severity)
		}
	}
}

// Deferred minor: the input hash must be of the exact bytes parsed, not of a
// second read of the file.
func TestInputSHA256OfParsedBytes(t *testing.T) {
	p := "../../ics-parser-test-suite/ics_files/09-combo-spoof-urgent.ics"
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := ParseICSFile(p, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if cal.InputSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("InputSHA256 = %q", cal.InputSHA256)
	}
	b, _ := json.Marshal(cal)
	m, _ := json.Marshal(cal.Manifest())
	if strings.Contains(string(b)+string(m), cal.InputSHA256) {
		t.Fatal("input hash leaked into JSON/manifest")
	}
}

// Gap finding-order-map-dependent: alerts derived from vendor-header maps must
// come out in the same order on every parse.
func TestHeaderAlertOrderDeterministic(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nX-GOOGLE-CONFERENCE-A:https://a.example/x\r\nX-GOOGLE-MEET-B:https://b.example/y\r\nX-GOOGLE-CONFERENCE-C:https://c.example/z\r\n" +
		"BEGIN:VEVENT\r\nUID:o@test\r\nSUMMARY:x\r\n" +
		"X-MICROSOFT-MEETING-A:https://a.example/1\r\nX-MICROSOFT-TEAMS-B:https://b.example/2\r\nX-MICROSOFT-MEETING-C:https://c.example/3\r\nX-MICROSOFT-OTHER-D:https://d.example/4\r\n" +
		"X-GOOGLE-CONFERENCE-E:https://e.example/5\r\nX-GOOGLE-MEET-F:https://f.example/6\r\nX-GOOGLE-CONFERENCE-G:https://g.example/7\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	first := ""
	for i := 0; i < 30; i++ {
		cal := parseInline(t, ics)
		var parts []string
		for _, f := range cal.Findings {
			parts = append(parts, f.Message)
		}
		for _, f := range cal.Events[0].Findings {
			parts = append(parts, f.Message)
		}
		got := strings.Join(parts, "\n")
		if i == 0 {
			first = got
		} else if got != first {
			t.Fatalf("parse %d gave a different alert order", i)
		}
	}
}
