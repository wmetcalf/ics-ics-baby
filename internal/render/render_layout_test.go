package render

import (
	"testing"
	"time"

	"ics-ics-baby/internal/icsparse"
)

func strPtr(s string) *string        { return &s }
func timePtr(t time.Time) *time.Time { return &t }

func TestBuildInviteLayoutsIncludesNewFields(t *testing.T) {
	event := icsparse.EventInfo{
		Summary:         "Investigate Alert",
		DTStart:         timePtr(time.Date(2025, 1, 10, 14, 0, 0, 0, time.UTC)),
		DTEnd:           timePtr(time.Date(2025, 1, 10, 15, 0, 0, 0, time.UTC)),
		Location:        strPtr("Security War Room"),
		Organizer:       strPtr("mailto:lead@example.com"),
		Color:           strPtr("#445566"),
		Geo:             &icsparse.GeoPoint{Latitude: 51.5007, Longitude: -0.1246},
		Contacts:        []string{"call +1-555-0110"},
		Comments:        []string{"Bring badge"},
		RelatedTo:       []string{"case-42"},
		RequestStatuses: []string{"2.0;Success"},
		Images:          []icsparse.ImageInfo{{Source: "uri", Value: "https://example.com/badge.png"}},
	}

	layouts, _ := buildInviteLayouts([]icsparse.EventInfo{event}, 800)
	if len(layouts) != 1 {
		t.Fatalf("expected 1 layout, got %d", len(layouts))
	}
	layout := layouts[0]
	if got, want := layout.iconColor, "#445566"; got != want {
		t.Fatalf("unexpected icon color %q", got)
	}

	assertHasField(t, layout.fields, "Contacts:")
	assertHasField(t, layout.fields, "Comments:")
	assertHasField(t, layout.fields, "Related To:")
	assertHasField(t, layout.fields, "Request Status:")
	assertHasField(t, layout.fields, "Images:")
	assertHasField(t, layout.fields, "Coordinates:")
	assertHasField(t, layout.fields, "Color:")
}

func TestBuildAvailabilityLayoutsIncludesSlots(t *testing.T) {
	window := icsparse.AvailableWindow{
		Summary:  strPtr("Morning Block"),
		Start:    timePtr(time.Date(2025, 1, 12, 9, 0, 0, 0, time.UTC)),
		End:      timePtr(time.Date(2025, 1, 12, 11, 0, 0, 0, time.UTC)),
		Duration: strPtr("PT2H"),
		Contacts: []string{"Desk +1-555-0199"},
		Recurrence: &icsparse.RecurrenceInfo{
			RRule: strPtr("FREQ=DAILY;COUNT=2"),
		},
	}
	avail := icsparse.AvailabilityInfo{
		Summary:    strPtr("Tier-1 Coverage"),
		BusyType:   strPtr("BUSY-UNAVAILABLE"),
		Start:      timePtr(time.Date(2025, 1, 12, 8, 0, 0, 0, time.UTC)),
		End:        timePtr(time.Date(2025, 1, 12, 17, 0, 0, 0, time.UTC)),
		Contacts:   []string{"mailto:support@example.com"},
		Categories: []string{"Support", "Weekday"},
		Available:  []icsparse.AvailableWindow{window},
	}

	layouts := buildAvailabilityLayouts([]icsparse.AvailabilityInfo{avail}, 800)
	if len(layouts) != 1 {
		t.Fatalf("expected 1 layout, got %d", len(layouts))
	}
	layout := layouts[0]
	assertHasField(t, layout.fields, "Busy Type:")
	assertHasField(t, layout.fields, "Contacts:")
	assertHasField(t, layout.fields, "Categories:")
	assertHasField(t, layout.fields, "Slots:")
	if layout.fieldsHeight <= 0 {
		t.Fatalf("expected positive fields height, got %f", layout.fieldsHeight)
	}
}

func TestBuildJournalLayoutsIncludesConferenceDetails(t *testing.T) {
	journal := icsparse.JournalInfo{
		Summary:        "Incident Notes",
		DTStart:        timePtr(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)),
		Organizer:      strPtr("mailto:owner@example.com"),
		Conferences:    []icsparse.ConferenceInfo{{URI: "https://meet.example.com/room", Params: map[string]string{"LABEL": "Join Room"}}},
		Attachments:    []icsparse.AttachmentInfo{{Source: "url", Value: "https://example.com/notes.txt", SavedAs: strPtr("notes.txt")}},
		DiscoveredURLs: []string{"https://meet.example.com/room"},
		Attendees:      []icsparse.Attendee{{Mailto: "analyst@example.com"}},
	}

	layouts := buildJournalLayouts([]icsparse.JournalInfo{journal}, 800)
	if len(layouts) != 1 {
		t.Fatalf("expected 1 layout, got %d", len(layouts))
	}
	layout := layouts[0]
	assertHasField(t, layout.fields, "Conference:")
	assertHasField(t, layout.fields, "Attachments:")
	assertHasField(t, layout.fields, "Referenced URLs:")
	assertHasField(t, layout.fields, "Attendees:")
	if layout.fieldsHeight <= 0 {
		t.Fatalf("expected positive fields height, got %f", layout.fieldsHeight)
	}
}

func assertHasField(t *testing.T, fields []inviteField, label string) {
	t.Helper()
	for _, f := range fields {
		if f.label == label {
			return
		}
	}
	t.Fatalf("expected field %q not found", label)
}
