package icsparse

import (
	"encoding/json"
	"path/filepath"
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
