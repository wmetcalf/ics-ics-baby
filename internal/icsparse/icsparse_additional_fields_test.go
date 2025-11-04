package icsparse

import (
	"path/filepath"
	"testing"
)

func TestParseAdditionalEventFields(t *testing.T) {
	path := filepath.Join("testdata", "event-additional-fields.ics")
	cal, err := ParseICSFile(path, "", 0)
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}
	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	event := cal.Events[0]
	if event.Color == nil || *event.Color != "blue" {
		t.Fatalf("expected event color blue, got %#v", event.Color)
	}
	if event.Geo == nil {
		t.Fatalf("expected geo coordinates, got nil")
	}
	if got, want := event.Geo.Latitude, 37.7749; got != want {
		t.Fatalf("unexpected latitude %v", got)
	}
	if got, want := event.Geo.Longitude, -122.4194; got != want {
		t.Fatalf("unexpected longitude %v", got)
	}
	if len(event.Contacts) != 1 || event.Contacts[0] != "Call +1-555-0100" {
		t.Fatalf("unexpected contacts %#v", event.Contacts)
	}
	if len(event.Comments) != 1 || event.Comments[0] != "Bring your laptop" {
		t.Fatalf("unexpected comments %#v", event.Comments)
	}
	if len(event.RelatedTo) != 1 || event.RelatedTo[0] != "prev-event@example.com" {
		t.Fatalf("unexpected related-to %#v", event.RelatedTo)
	}
	if len(event.RequestStatuses) != 1 || event.RequestStatuses[0] != "2.0;Success" {
		t.Fatalf("unexpected request statuses %#v", event.RequestStatuses)
	}
	if len(event.Images) != 1 {
		t.Fatalf("expected 1 event image, got %d", len(event.Images))
	}
	if event.Images[0].Source != "uri" {
		t.Fatalf("expected image source uri, got %s", event.Images[0].Source)
	}
	if event.Images[0].FmtType == nil || *event.Images[0].FmtType != "image/jpeg" {
		t.Fatalf("unexpected image fmt type %#v", event.Images[0].FmtType)
	}
	if event.Images[0].Display == nil || *event.Images[0].Display != "GRAPHIC" {
		t.Fatalf("unexpected image display %#v", event.Images[0].Display)
	}
	if len(event.Attendees) != 1 {
		t.Fatalf("expected 1 attendee, got %d", len(event.Attendees))
	}
	attendee := event.Attendees[0]
	if attendee.Cutype == nil || *attendee.Cutype != "INDIVIDUAL" {
		t.Fatalf("unexpected attendee cutype %#v (attendee=%+v)", attendee.Cutype, attendee)
	}
	if attendee.SentBy == nil || *attendee.SentBy != "dave@example.com" {
		t.Fatalf("unexpected attendee sent-by %#v (attendee=%+v)", attendee.SentBy, attendee)
	}
	if len(attendee.DelegatedFrom) != 1 || attendee.DelegatedFrom[0] != "bob@example.com" {
		t.Fatalf("unexpected delegated-from %#v", attendee.DelegatedFrom)
	}
	if len(attendee.DelegatedTo) != 1 || attendee.DelegatedTo[0] != "carol@example.com" {
		t.Fatalf("unexpected delegated-to %#v", attendee.DelegatedTo)
	}
	if attendee.Directory != nil {
		t.Fatalf("expected no directory, got %#v", attendee.Directory)
	}

	if cal.Color == nil || *cal.Color != "#123456" {
		t.Fatalf("unexpected calendar color %#v", cal.Color)
	}
	if cal.Source == nil || *cal.Source != "https://example.com/feed.ics" {
		t.Fatalf("unexpected calendar source %#v", cal.Source)
	}
	if cal.RefreshInterval == nil || *cal.RefreshInterval != "PT1H" {
		t.Fatalf("unexpected refresh interval %#v", cal.RefreshInterval)
	}
	if len(cal.Categories) != 2 {
		t.Fatalf("expected 2 calendar categories, got %d", len(cal.Categories))
	}
	if len(cal.Contacts) != 1 || cal.Contacts[0] != "security-team@example.com" {
		t.Fatalf("unexpected calendar contacts %#v", cal.Contacts)
	}
	if len(cal.Images) != 1 {
		t.Fatalf("expected 1 calendar image, got %d", len(cal.Images))
	}
	if cal.Images[0].Value != "https://example.com/logo.png" {
		t.Fatalf("unexpected calendar image value %s", cal.Images[0].Value)
	}
	if cal.Images[0].Display == nil || *cal.Images[0].Display != "BADGE" {
		t.Fatalf("unexpected calendar image display %#v", cal.Images[0].Display)
	}
}

func TestParseAvailability(t *testing.T) {
	path := filepath.Join("testdata", "availability.ics")
	cal, err := ParseICSFile(path, "", 0)
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}
	if len(cal.Availabilities) != 1 {
		t.Fatalf("expected 1 availability, got %d", len(cal.Availabilities))
	}
	availability := cal.Availabilities[0]
	if availability.BusyType == nil || *availability.BusyType != "BUSY-UNAVAILABLE" {
		t.Fatalf("unexpected busy type %#v", availability.BusyType)
	}
	if availability.Start == nil || availability.Start.Location().String() != "America/New_York" {
		t.Fatalf("expected start timezone America/New_York, got %#v", availability.Start)
	}
	if len(availability.Categories) != 2 {
		t.Fatalf("expected 2 availability categories, got %d", len(availability.Categories))
	}
	if len(availability.Contacts) != 1 || availability.Contacts[0] != "mailto:support@example.com" {
		t.Fatalf("unexpected availability contacts %#v", availability.Contacts)
	}
	if len(availability.Available) != 1 {
		t.Fatalf("expected 1 available window, got %d", len(availability.Available))
	}
	window := availability.Available[0]
	if window.Recurrence == nil || window.Recurrence.RRule == nil {
		t.Fatalf("expected recurrence rule, got %#v", window.Recurrence)
	}
	if *window.Recurrence.RRule != "FREQ=DAILY;COUNT=5" {
		t.Fatalf("unexpected recurrence rule %s", *window.Recurrence.RRule)
	}
	if len(window.Contacts) != 1 || window.Contacts[0] != "Desk +1-555-0101" {
		t.Fatalf("unexpected available contacts %#v", window.Contacts)
	}
}

func TestParseJournal(t *testing.T) {
	path := filepath.Join("testdata", "journal.ics")
	cal, err := ParseICSFile(path, "", 0)
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}
	if len(cal.Journals) != 1 {
		t.Fatalf("expected 1 journal, got %d", len(cal.Journals))
	}
	jn := cal.Journals[0]
	if jn.Summary != "Incident Notes" {
		t.Fatalf("unexpected summary %s", jn.Summary)
	}
	if jn.DTStart == nil {
		t.Fatalf("expected dtstart for journal")
	}
	if jn.Status == nil || *jn.Status != "DRAFT" {
		t.Fatalf("unexpected journal status %#v", jn.Status)
	}
	if jn.Class == nil || *jn.Class != "CONFIDENTIAL" {
		t.Fatalf("unexpected journal class %#v", jn.Class)
	}
	if len(jn.RelatedTo) != 1 || jn.RelatedTo[0] != "incident-123" {
		t.Fatalf("unexpected related-to %#v", jn.RelatedTo)
	}
	if len(jn.Attachments) != 1 || jn.Attachments[0].Value != "https://example.com/notes.txt" {
		t.Fatalf("unexpected attachment %#v", jn.Attachments)
	}
	if len(jn.Images) != 1 || jn.Images[0].Value != "https://example.com/screenshot.png" {
		t.Fatalf("unexpected image %#v", jn.Images)
	}
	if len(jn.Attendees) != 1 || jn.Attendees[0].Mailto != "analyst@example.com" {
		t.Fatalf("unexpected attendees %#v", jn.Attendees)
	}
}
