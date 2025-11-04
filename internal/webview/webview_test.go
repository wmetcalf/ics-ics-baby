package webview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ics-ics-baby/internal/icsparse"
)

func strPtr(s string) *string { return &s }

func TestWriteInviteHTMLIncludesEnhancedMetadata(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "preview.html")

	geo := &icsparse.GeoPoint{Latitude: 37.7749, Longitude: -122.4194}
	event := icsparse.EventInfo{
		Summary:         "Urgent Support Call",
		DTStart:         timePtr(time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)),
		DTEnd:           timePtr(time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)),
		Location:        strPtr("Conference Room"),
		Organizer:       strPtr("mailto:organizer@example.com"),
		Color:           strPtr("#112233"),
		Geo:             geo,
		Contacts:        []string{"Call +1-555-0100"},
		Comments:        []string{"Bring your laptop"},
		RelatedTo:       []string{"incident-123"},
		RequestStatuses: []string{"2.0;Success"},
		Conferences: []icsparse.ConferenceInfo{{
			URI:    "https://zoom.example.com/j/meeting",
			Params: map[string]string{"LABEL": "Join Zoom"},
		}},
		Attachments: []icsparse.AttachmentInfo{{
			Source: "url",
			Value:  "https://example.com/runbook.pdf",
			Fmt:    strPtr("application/pdf"),
		}},
		Images: []icsparse.ImageInfo{{
			Source:  "uri",
			Value:   "https://example.com/banner.jpg",
			FmtType: strPtr("image/jpeg"),
			Display: strPtr("GRAPHIC"),
		}},
		Attendees: []icsparse.Attendee{{
			Mailto:        "alice@example.com",
			CN:            strPtr("Alice"),
			Role:          strPtr("CHAIR"),
			PartStat:      strPtr("ACCEPTED"),
			RSVP:          strPtr("TRUE"),
			Cutype:        strPtr("INDIVIDUAL"),
			SentBy:        strPtr("dave@example.com"),
			DelegatedFrom: []string{"bob@example.com"},
			DelegatedTo:   []string{"carol@example.com"},
		}},
	}

	availability := icsparse.AvailabilityInfo{
		Summary:    strPtr("Support Coverage"),
		BusyType:   strPtr("BUSY-UNAVAILABLE"),
		Start:      timePtr(time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC)),
		End:        timePtr(time.Date(2025, 1, 2, 17, 0, 0, 0, time.UTC)),
		Contacts:   []string{"mailto:support@example.com"},
		Categories: []string{"Support", "Weekday"},
		Location:   strPtr("Headquarters"),
		Available: []icsparse.AvailableWindow{{
			Summary:  strPtr("Morning Slot"),
			Start:    timePtr(time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC)),
			End:      timePtr(time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)),
			Duration: strPtr("PT3H"),
			Contacts: []string{"Desk +1-555-0101"},
			Recurrence: &icsparse.RecurrenceInfo{
				RRule: strPtr("FREQ=DAILY;COUNT=3"),
			},
		}},
	}

	journal := icsparse.JournalInfo{
		Summary:        "Incident Journal",
		DTStart:        timePtr(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)),
		Organizer:      strPtr("mailto:owner@example.com"),
		Description:    strPtr("Post-incident notes"),
		Conferences:    []icsparse.ConferenceInfo{{URI: "https://meet.example.com/room", Params: map[string]string{"FEATURE": "VIDEO"}}},
		Attachments:    []icsparse.AttachmentInfo{{Source: "url", Value: "https://example.com/analysis.txt", SavedAs: strPtr("/tmp/analysis.txt")}},
		DiscoveredURLs: []string{"https://meet.example.com/room"},
	}

	cal := &icsparse.CalendarInfo{
		Name:            strPtr("Security Calendar"),
		ProdID:          strPtr("-//ics-ics-baby//EN"),
		Method:          strPtr("PUBLISH"),
		Description:     strPtr("Brand-aligned phishing calendar"),
		URL:             strPtr("https://example.com/feed.ics"),
		Calscale:        strPtr("GREGORIAN"),
		TimezoneID:      strPtr("America/New_York"),
		Categories:      []string{"Security", "Phishing"},
		Contacts:        []string{"security-team@example.com"},
		Images:          []icsparse.ImageInfo{{Source: "uri", Value: "https://example.com/logo.png", Display: strPtr("BADGE"), FmtType: strPtr("image/png")}},
		RefreshInterval: strPtr("PT30M"),
		Source:          strPtr("https://example.com/feed.ics"),
		Color:           strPtr("#123456"),
		Events:          []icsparse.EventInfo{event},
		Availabilities:  []icsparse.AvailabilityInfo{availability},
		Journals:        []icsparse.JournalInfo{journal},
	}

	if err := WriteInviteHTML(cal, out, "light"); err != nil {
		t.Fatalf("WriteInviteHTML failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated html: %v", err)
	}
	html := string(data)
	assertContains(t, html, "color-chip")
	assertContains(t, html, "Security Calendar")
	assertContains(t, html, "Coordinates:")
	assertContains(t, html, "Contacts:")
	assertContains(t, html, "delegated-from=bob@example.com")
	assertContains(t, html, "Images:")
	assertContains(t, html, "Published Availability")
	assertContains(t, html, "Slots:")
	assertContains(t, html, "Join Zoom")
	assertContains(t, html, "Journal Entries")
	assertContains(t, html, "Join Video")
	assertContains(t, html, "attachment-btn")
	assertContains(t, html, "Referenced URLs:")
	assertContains(t, html, "analysis.txt")
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q", want)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
