package icsparse

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseICSFile_WithEmbeddedVCard(t *testing.T) {
	cal, err := ParseICSFile(filepath.Join("testdata", "event-with-vcard.ics"), "")
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]
	if len(ev.VCards) != 1 {
		t.Fatalf("expected 1 embedded vCard, got %d", len(ev.VCards))
	}

	card := ev.VCards[0]
	if card.Kind != "VCARD" {
		t.Fatalf("expected card kind VCARD, got %s", card.Kind)
	}

	if len(card.Properties) < 4 {
		t.Fatalf("expected at least 4 vCard properties, got %d", len(card.Properties))
	}

	fn := card.Properties[1]
	if fn.Name != "FN" {
		t.Fatalf("expected FN property, got %s", fn.Name)
	}
	if fn.Value == nil || *fn.Value != "John Doe" {
		t.Fatalf("unexpected FN value: %v", fn.Value)
	}

	email := card.Properties[2]
	if email.Params == nil {
		t.Fatalf("expected params on email property")
	}
	types := email.Params["TYPE"]
	if len(types) != 1 || types[0] != "work" {
		t.Fatalf("unexpected email TYPE params: %v", types)
	}

	tel := card.Properties[3]
	if tel.Params == nil {
		t.Fatalf("expected params on telephone property")
	}
	telTypes := tel.Params["TYPE"]
	if len(telTypes) != 2 {
		t.Fatalf("expected 2 TYPE values on telephone, got %v", telTypes)
	}
	if tel.Binary != nil {
		t.Fatalf("telephone property should not have binary payload")
	}

	if len(cal.VCards) != 0 {
		t.Fatalf("did not expect top-level vCards, got %d", len(cal.VCards))
	}
}

func TestParseICSFile_TopLevelVCardBinary(t *testing.T) {
	cal, err := ParseICSFile(filepath.Join("testdata", "top-level-vcard.ics"), "")
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}

	if len(cal.VCards) != 1 {
		t.Fatalf("expected 1 top-level vCard, got %d", len(cal.VCards))
	}
	card := cal.VCards[0]
	if len(card.Properties) < 2 {
		t.Fatalf("expected at least 2 properties, got %d", len(card.Properties))
	}

	photo := card.Properties[len(card.Properties)-1]
	if photo.Name != "PHOTO" {
		t.Fatalf("expected PHOTO property, got %s", photo.Name)
	}
	if photo.Binary == nil || *photo.Binary == "" {
		t.Fatalf("expected binary data for PHOTO")
	}
	if photo.Encoding == nil || *photo.Encoding != "b" {
		t.Fatalf("expected encoding 'b', got %v", photo.Encoding)
	}
}

func TestParseICSFile_WithRecurrenceAndTimezone(t *testing.T) {
	cal, err := ParseICSFile(filepath.Join("testdata", "event-with-recurrence.ics"), "")
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}

	if cal.Calscale == nil || *cal.Calscale != "GREGORIAN" {
		t.Fatalf("expected calendar calscale GREGORIAN, got %v", cal.Calscale)
	}
	if cal.Description == nil || *cal.Description == "" {
		t.Fatalf("expected calendar description")
	}
	if len(cal.Timezones) != 1 {
		t.Fatalf("expected 1 timezone, got %d", len(cal.Timezones))
	}

	tz := cal.Timezones[0]
	if tz.TZID == nil || *tz.TZID != "America/New_York" {
		t.Fatalf("unexpected timezone id: %v", tz.TZID)
	}
	if len(tz.Periods) != 2 {
		t.Fatalf("expected 2 timezone periods, got %d", len(tz.Periods))
	}
	if tz.Periods[0].Type == tz.Periods[1].Type {
		t.Fatalf("expected distinct timezone period types")
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]
	if ev.Transparency == nil || *ev.Transparency != "OPAQUE" {
		t.Fatalf("expected OPAQUE transparency, got %v", ev.Transparency)
	}
	if ev.Priority == nil || *ev.Priority != 5 {
		t.Fatalf("expected priority 5, got %v", ev.Priority)
	}
	if ev.Sequence == nil || *ev.Sequence != 2 {
		t.Fatalf("expected sequence 2, got %v", ev.Sequence)
	}
	if len(ev.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %v", ev.Categories)
	}
	if ev.URL == nil || *ev.URL == "" {
		t.Fatalf("expected event URL")
	}
	if len(ev.Conferences) != 1 {
		t.Fatalf("expected 1 conference entry, got %d", len(ev.Conferences))
	}
	conf := ev.Conferences[0]
	if conf.URI != "https://meet.example.com/abc" {
		t.Fatalf("unexpected conference URI: %s", conf.URI)
	}
	if feat := conf.Params["FEATURE"]; feat != "AUDIO" {
		t.Fatalf("expected conference FEATURE=AUDIO, got %s", feat)
	}

	if ev.Recurrence == nil {
		t.Fatalf("expected recurrence info")
	}
	if ev.Recurrence.RRule == nil || *ev.Recurrence.RRule == "" {
		t.Fatalf("expected recurrence rule")
	}
	if len(ev.Recurrence.RDates) != 1 {
		t.Fatalf("expected 1 RDATE, got %d", len(ev.Recurrence.RDates))
	}
	if len(ev.Recurrence.ExDates) != 1 {
		t.Fatalf("expected 1 EXDATE, got %d", len(ev.Recurrence.ExDates))
	}
	if ev.Duration == nil || *ev.Duration != "PT30M" {
		t.Fatalf("expected event duration PT30M, got %v", ev.Duration)
	}
	if ev.Recurrence.Duration == nil || *ev.Recurrence.Duration != "PT30M" {
		t.Fatalf("expected recurrence duration PT30M, got %v", ev.Recurrence.Duration)
	}

	rdate := ev.Recurrence.RDates[0].In(time.FixedZone("EDT", -4*3600))
	if rdate.Hour() != 9 {
		t.Fatalf("expected RDATE hour 9, got %d", rdate.Hour())
	}

	if len(ev.Alarms) != 1 {
		t.Fatalf("expected 1 alarm, got %d", len(ev.Alarms))
	}
	alarm := ev.Alarms[0]
	if alarm.Trigger == nil || alarm.Trigger.Duration == nil {
		t.Fatalf("expected alarm trigger duration")
	}
	if alarm.Description == nil || *alarm.Description != "Reminder" {
		t.Fatalf("expected alarm description 'Reminder', got %v", alarm.Description)
	}
}

func TestParseICSFile_TodosAndFreeBusy(t *testing.T) {
	cal, err := ParseICSFile(filepath.Join("testdata", "todos-freebusy.ics"), "")
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}

	if len(cal.Todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(cal.Todos))
	}
	todo := cal.Todos[0]
	if todo.Summary != "Finish Quarterly Report" {
		t.Fatalf("unexpected todo summary: %s", todo.Summary)
	}
	if todo.Status == nil || *todo.Status != "IN-PROCESS" {
		t.Fatalf("unexpected todo status: %v", todo.Status)
	}
	if todo.Priority == nil || *todo.Priority != 3 {
		t.Fatalf("expected priority 3, got %v", todo.Priority)
	}
	if todo.PercentComplete == nil || *todo.PercentComplete != 50 {
		t.Fatalf("expected percent complete 50, got %v", todo.PercentComplete)
	}
	if len(todo.Categories) != 2 || todo.Categories[0] != "Work" {
		t.Fatalf("unexpected todo categories: %v", todo.Categories)
	}
	if len(todo.Resources) != 1 || todo.Resources[0] != "Laptop" {
		t.Fatalf("unexpected todo resources: %v", todo.Resources)
	}
	if todo.Start == nil || todo.Due == nil {
		t.Fatalf("expected start and due dates")
	}
	if todo.URL == nil || *todo.URL == "" {
		t.Fatalf("expected todo URL")
	}
	if len(todo.Alarms) != 1 {
		t.Fatalf("expected 1 todo alarm, got %d", len(todo.Alarms))
	}
	tAlarm := todo.Alarms[0]
	if tAlarm.Trigger == nil || tAlarm.Trigger.Duration == nil || *tAlarm.Trigger.Duration != "-PT30M" {
		t.Fatalf("unexpected todo alarm trigger: %+v", tAlarm.Trigger)
	}
	if len(cal.FreeBusy) != 1 {
		t.Fatalf("expected 1 freebusy entry, got %d", len(cal.FreeBusy))
	}
	fb := cal.FreeBusy[0]
	if fb.Organizer == nil || *fb.Organizer != "mailto:boss@example.com" {
		t.Fatalf("unexpected freebusy organizer: %v", fb.Organizer)
	}
	if len(fb.Periods) != 2 {
		t.Fatalf("expected 2 busy periods, got %d", len(fb.Periods))
	}
	if fb.Periods[0].Type == nil || *fb.Periods[0].Type != "BUSY" {
		t.Fatalf("unexpected freebusy type: %v", fb.Periods[0].Type)
	}
	if !fb.Periods[0].Start.Equal(time.Date(2024, 5, 1, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected busy start: %v", fb.Periods[0].Start)
	}
	if !fb.Periods[0].End.Equal(time.Date(2024, 5, 1, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected busy end: %v", fb.Periods[0].End)
	}
	if len(fb.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %v", fb.Comments)
	}
}
