package icsparse

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	htmlstd "html"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	htmlnode "golang.org/x/net/html"
	htmlatom "golang.org/x/net/html/atom"
)

type Attendee struct {
	Mailto        string   `json:"mailto,omitempty"`
	CN            *string  `json:"cn,omitempty"`
	Role          *string  `json:"role,omitempty"`
	PartStat      *string  `json:"partstat,omitempty"`
	RSVP          *string  `json:"rsvp,omitempty"`
	Cutype        *string  `json:"cutype,omitempty"`
	SentBy        *string  `json:"sent_by,omitempty"`
	Directory     *string  `json:"directory,omitempty"`
	Language      *string  `json:"language,omitempty"`
	DelegatedFrom []string `json:"delegated_from,omitempty"`
	DelegatedTo   []string `json:"delegated_to,omitempty"`
	Member        []string `json:"member,omitempty"`
}

type AttachmentInfo struct {
	Href    *string `json:"href,omitempty"`
	MD5     *string `json:"md5,omitempty"`
	SHA256  *string `json:"sha256,omitempty"`
	Mime    *string `json:"mime,omitempty"`
	Magic   *string `json:"magic,omitempty"`
	Source  string  `json:"source"`
	Value   string  `json:"value"`
	Fmt     *string `json:"fmt,omitempty"`
	Size    *int    `json:"size,omitempty"`
	SavedAs *string `json:"saved_as,omitempty"`
}

type EventInfo struct {
	DiscoveredURLs  []string            `json:"discovered_urls,omitempty"`
	UID             *string             `json:"uid,omitempty"`
	Summary         string              `json:"summary"`
	DTStart         *time.Time          `json:"dtstart,omitempty"`
	DTEnd           *time.Time          `json:"dtend,omitempty"`
	Location        *string             `json:"location,omitempty"`
	Description     *string             `json:"description,omitempty"`
	DescriptionHTML *string             `json:"description_html,omitempty"`
	Organizer       *string             `json:"organizer,omitempty"`
	Status          *string             `json:"status,omitempty"`
	Attendees       []Attendee          `json:"attendees,omitempty"`
	Attachments     []AttachmentInfo    `json:"attachments"`
	VCards          []VCard             `json:"vcards,omitempty"`
	Alarms          []AlarmInfo         `json:"alarms,omitempty"`
	Categories      []string            `json:"categories,omitempty"`
	Resources       []string            `json:"resources,omitempty"`
	Contacts        []string            `json:"contacts,omitempty"`
	Comments        []string            `json:"comments,omitempty"`
	RelatedTo       []string            `json:"related_to,omitempty"`
	RequestStatuses []string            `json:"request_statuses,omitempty"`
	Images          []ImageInfo         `json:"images,omitempty"`
	Conferences     []ConferenceInfo    `json:"conferences,omitempty"`
	Recurrence      *RecurrenceInfo     `json:"recurrence,omitempty"`
	Transparency    *string             `json:"transparency,omitempty"`
	Priority        *int                `json:"priority,omitempty"`
	Class           *string             `json:"class,omitempty"`
	Color           *string             `json:"color,omitempty"`
	Geo             *GeoPoint           `json:"geo,omitempty"`
	URL             *string             `json:"url,omitempty"`
	Created         *time.Time          `json:"created,omitempty"`
	LastModified    *time.Time          `json:"last_modified,omitempty"`
	DateTimeStamp   *time.Time          `json:"dtstamp,omitempty"`
	Sequence        *int                `json:"sequence,omitempty"`
	Duration        *string             `json:"duration,omitempty"`
	RawProps        map[string][]string `json:"raw_props"`
}

type CalendarInfo struct {
	DiscoveredURLs  []string           `json:"discovered_urls,omitempty"`
	Name            *string            `json:"name,omitempty"`
	Method          *string            `json:"method,omitempty"`
	ProdID          *string            `json:"prodid,omitempty"`
	Color           *string            `json:"color,omitempty"`
	Source          *string            `json:"source,omitempty"`
	RefreshInterval *string            `json:"refresh_interval,omitempty"`
	Categories      []string           `json:"categories,omitempty"`
	Contacts        []string           `json:"contacts,omitempty"`
	Images          []ImageInfo        `json:"images,omitempty"`
	Events          []EventInfo        `json:"events"`
	VCards          []VCard            `json:"vcards,omitempty"`
	Description     *string            `json:"description,omitempty"`
	URL             *string            `json:"url,omitempty"`
	Calscale        *string            `json:"calscale,omitempty"`
	TimezoneID      *string            `json:"timezone_id,omitempty"`
	Timezones       []TimezoneInfo     `json:"timezones,omitempty"`
	Todos           []TodoInfo         `json:"todos,omitempty"`
	FreeBusy        []FreeBusyInfo     `json:"freebusy,omitempty"`
	Availabilities  []AvailabilityInfo `json:"availabilities,omitempty"`
	Journals        []JournalInfo      `json:"journals,omitempty"`
}

func (c *CalendarInfo) Manifest() map[string]any {
	calendarMeta := map[string]any{
		"name":        c.Name,
		"prodid":      c.ProdID,
		"method":      c.Method,
		"description": c.Description,
		"url":         c.URL,
		"calscale":    c.Calscale,
		"timezone_id": c.TimezoneID,
	}
	if c.Color != nil {
		calendarMeta["color"] = c.Color
	}
	if c.Source != nil {
		calendarMeta["source"] = c.Source
	}
	if c.RefreshInterval != nil {
		calendarMeta["refresh_interval"] = c.RefreshInterval
	}
	if len(c.Categories) > 0 {
		calendarMeta["categories"] = c.Categories
	}
	if len(c.Contacts) > 0 {
		calendarMeta["contacts"] = c.Contacts
	}
	if len(c.Images) > 0 {
		calendarMeta["images"] = c.Images
	}
	out := map[string]any{
		"calendar": calendarMeta,
	}
	type ev struct {
		UID             *string             `json:"uid,omitempty"`
		Summary         string              `json:"summary"`
		DTStart         *time.Time          `json:"dtstart,omitempty"`
		DTEnd           *time.Time          `json:"dtend,omitempty"`
		Location        *string             `json:"location,omitempty"`
		Description     *string             `json:"description,omitempty"`
		DescriptionHTML *string             `json:"description_html,omitempty"`
		Organizer       *string             `json:"organizer,omitempty"`
		Status          *string             `json:"status,omitempty"`
		Attendees       []Attendee          `json:"attendees,omitempty"`
		Attachments     []AttachmentInfo    `json:"attachments"`
		VCards          []VCard             `json:"vcards,omitempty"`
		Alarms          []AlarmInfo         `json:"alarms,omitempty"`
		Categories      []string            `json:"categories,omitempty"`
		Resources       []string            `json:"resources,omitempty"`
		Contacts        []string            `json:"contacts,omitempty"`
		Comments        []string            `json:"comments,omitempty"`
		RelatedTo       []string            `json:"related_to,omitempty"`
		RequestStatuses []string            `json:"request_statuses,omitempty"`
		Images          []ImageInfo         `json:"images,omitempty"`
		Conferences     []ConferenceInfo    `json:"conferences,omitempty"`
		Recurrence      *RecurrenceInfo     `json:"recurrence,omitempty"`
		Transparency    *string             `json:"transparency,omitempty"`
		Priority        *int                `json:"priority,omitempty"`
		Class           *string             `json:"class,omitempty"`
		Color           *string             `json:"color,omitempty"`
		Geo             *GeoPoint           `json:"geo,omitempty"`
		URL             *string             `json:"url,omitempty"`
		Created         *time.Time          `json:"created,omitempty"`
		LastModified    *time.Time          `json:"last_modified,omitempty"`
		DateTimeStamp   *time.Time          `json:"dtstamp,omitempty"`
		Sequence        *int                `json:"sequence,omitempty"`
		Duration        *string             `json:"duration,omitempty"`
		RawProps        map[string][]string `json:"raw_props"`
		DiscoveredURLs  []string            `json:"discovered_urls,omitempty"`
	}
	type td struct {
		UID             *string             `json:"uid,omitempty"`
		Summary         string              `json:"summary"`
		Description     *string             `json:"description,omitempty"`
		DescriptionHTML *string             `json:"description_html,omitempty"`
		Location        *string             `json:"location,omitempty"`
		Organizer       *string             `json:"organizer,omitempty"`
		Status          *string             `json:"status,omitempty"`
		Priority        *int                `json:"priority,omitempty"`
		PercentComplete *int                `json:"percent_complete,omitempty"`
		Categories      []string            `json:"categories,omitempty"`
		Resources       []string            `json:"resources,omitempty"`
		Contacts        []string            `json:"contacts,omitempty"`
		Comments        []string            `json:"comments,omitempty"`
		RelatedTo       []string            `json:"related_to,omitempty"`
		RequestStatuses []string            `json:"request_statuses,omitempty"`
		Images          []ImageInfo         `json:"images,omitempty"`
		Color           *string             `json:"color,omitempty"`
		URL             *string             `json:"url,omitempty"`
		Start           *time.Time          `json:"dtstart,omitempty"`
		Due             *time.Time          `json:"due,omitempty"`
		Completed       *time.Time          `json:"completed,omitempty"`
		Created         *time.Time          `json:"created,omitempty"`
		LastModified    *time.Time          `json:"last_modified,omitempty"`
		DateTimeStamp   *time.Time          `json:"dtstamp,omitempty"`
		Sequence        *int                `json:"sequence,omitempty"`
		Duration        *string             `json:"duration,omitempty"`
		Recurrence      *RecurrenceInfo     `json:"recurrence,omitempty"`
		Attendees       []Attendee          `json:"attendees,omitempty"`
		Attachments     []AttachmentInfo    `json:"attachments,omitempty"`
		VCards          []VCard             `json:"vcards,omitempty"`
		Alarms          []AlarmInfo         `json:"alarms,omitempty"`
		RawProps        map[string][]string `json:"raw_props"`
		DiscoveredURLs  []string            `json:"discovered_urls,omitempty"`
	}
	type fb struct {
		UID         *string             `json:"uid,omitempty"`
		Organizer   *string             `json:"organizer,omitempty"`
		Contact     *string             `json:"contact,omitempty"`
		URL         *string             `json:"url,omitempty"`
		Start       *time.Time          `json:"dtstart,omitempty"`
		End         *time.Time          `json:"dtend,omitempty"`
		Periods     []FreeBusyPeriod    `json:"periods,omitempty"`
		Comments    []string            `json:"comments,omitempty"`
		VCards      []VCard             `json:"vcards,omitempty"`
		Attendees   []Attendee          `json:"attendees,omitempty"`
		Attachments []AttachmentInfo    `json:"attachments,omitempty"`
		RawProps    map[string][]string `json:"raw_props"`
	}
	type av struct {
		UID             *string             `json:"uid,omitempty"`
		Summary         *string             `json:"summary,omitempty"`
		Description     *string             `json:"description,omitempty"`
		DescriptionHTML *string             `json:"description_html,omitempty"`
		Organizer       *string             `json:"organizer,omitempty"`
		BusyType        *string             `json:"busy_type,omitempty"`
		Categories      []string            `json:"categories,omitempty"`
		Contacts        []string            `json:"contacts,omitempty"`
		URL             *string             `json:"url,omitempty"`
		Location        *string             `json:"location,omitempty"`
		Priority        *int                `json:"priority,omitempty"`
		Sequence        *int                `json:"sequence,omitempty"`
		Created         *time.Time          `json:"created,omitempty"`
		LastModified    *time.Time          `json:"last_modified,omitempty"`
		DateTimeStamp   *time.Time          `json:"dtstamp,omitempty"`
		Start           *time.Time          `json:"dtstart,omitempty"`
		End             *time.Time          `json:"dtend,omitempty"`
		Duration        *string             `json:"duration,omitempty"`
		Available       []AvailableWindow   `json:"available,omitempty"`
		RawProps        map[string][]string `json:"raw_props"`
	}
	type jr struct {
		UID             *string             `json:"uid,omitempty"`
		Summary         string              `json:"summary"`
		Description     *string             `json:"description,omitempty"`
		DescriptionHTML *string             `json:"description_html,omitempty"`
		DTStart         *time.Time          `json:"dtstart,omitempty"`
		Organizer       *string             `json:"organizer,omitempty"`
		Status          *string             `json:"status,omitempty"`
		Class           *string             `json:"class,omitempty"`
		Categories      []string            `json:"categories,omitempty"`
		Contacts        []string            `json:"contacts,omitempty"`
		RelatedTo       []string            `json:"related_to,omitempty"`
		Conferences     []ConferenceInfo    `json:"conferences,omitempty"`
		URL             *string             `json:"url,omitempty"`
		DateTimeStamp   *time.Time          `json:"dtstamp,omitempty"`
		Created         *time.Time          `json:"created,omitempty"`
		LastModified    *time.Time          `json:"last_modified,omitempty"`
		Recurrence      *RecurrenceInfo     `json:"recurrence,omitempty"`
		Attendees       []Attendee          `json:"attendees,omitempty"`
		Attachments     []AttachmentInfo    `json:"attachments,omitempty"`
		DiscoveredURLs  []string            `json:"discovered_urls,omitempty"`
		Images          []ImageInfo         `json:"images,omitempty"`
		RawProps        map[string][]string `json:"raw_props"`
	}
	events := make([]ev, 0, len(c.Events))
	for _, e := range c.Events {
		events = append(events, ev{
			UID: e.UID, Summary: e.Summary, DTStart: e.DTStart, DTEnd: e.DTEnd,
			Location: e.Location, Description: e.Description, DescriptionHTML: e.DescriptionHTML, Organizer: e.Organizer,
			Status: e.Status, Attendees: e.Attendees,
			Attachments: e.Attachments, VCards: e.VCards, Alarms: e.Alarms,
			Categories: e.Categories, Resources: e.Resources, Contacts: e.Contacts, Comments: e.Comments,
			RelatedTo: e.RelatedTo, RequestStatuses: e.RequestStatuses, Images: e.Images,
			Conferences: e.Conferences,
			Recurrence:  e.Recurrence, Transparency: e.Transparency, Priority: e.Priority,
			Class: e.Class, Color: e.Color, Geo: e.Geo, URL: e.URL, Created: e.Created, LastModified: e.LastModified,
			DateTimeStamp: e.DateTimeStamp, Sequence: e.Sequence, Duration: e.Duration,
			RawProps: e.RawProps, DiscoveredURLs: e.DiscoveredURLs,
		})
	}
	out["events"] = events
	out["discovered_urls"] = c.DiscoveredURLs
	if len(c.VCards) > 0 {
		out["vcards"] = c.VCards
	}
	if len(c.Timezones) > 0 {
		out["timezones"] = c.Timezones
	}
	if len(c.Todos) > 0 {
		items := make([]td, 0, len(c.Todos))
		for _, todo := range c.Todos {
			items = append(items, td{
				UID: todo.UID, Summary: todo.Summary, Description: todo.Description, DescriptionHTML: todo.DescriptionHTML, Location: todo.Location,
				Organizer: todo.Organizer, Status: todo.Status, Priority: todo.Priority, PercentComplete: todo.PercentComplete,
				Categories: todo.Categories, Resources: todo.Resources, Contacts: todo.Contacts, Comments: todo.Comments,
				RelatedTo: todo.RelatedTo, RequestStatuses: todo.RequestStatuses, Images: todo.Images, Color: todo.Color,
				URL: todo.URL, Start: todo.Start, Due: todo.Due,
				Completed: todo.Completed, Created: todo.Created, LastModified: todo.LastModified,
				DateTimeStamp: todo.DateTimeStamp, Sequence: todo.Sequence, Duration: todo.Duration,
				Recurrence: todo.Recurrence, Attendees: todo.Attendees, Attachments: todo.Attachments,
				VCards: todo.VCards, Alarms: todo.Alarms, RawProps: todo.RawProps,
				DiscoveredURLs: todo.DiscoveredURLs,
			})
		}
		out["todos"] = items
	}
	if len(c.FreeBusy) > 0 {
		items := make([]fb, 0, len(c.FreeBusy))
		for _, busy := range c.FreeBusy {
			items = append(items, fb{
				UID: busy.UID, Organizer: busy.Organizer, Contact: busy.Contact, URL: busy.URL,
				Start: busy.Start, End: busy.End, Periods: busy.Periods, Comments: busy.Comments,
				VCards: busy.VCards, Attendees: busy.Attendees, Attachments: busy.Attachments,
				RawProps: busy.RawProps,
			})
		}
		out["freebusy"] = items
	}
	if len(c.Availabilities) > 0 {
		items := make([]av, 0, len(c.Availabilities))
		for _, availability := range c.Availabilities {
			items = append(items, av{
				UID: availability.UID, Summary: availability.Summary, Description: availability.Description, DescriptionHTML: availability.DescriptionHTML,
				Organizer: availability.Organizer, BusyType: availability.BusyType, Categories: availability.Categories,
				Contacts: availability.Contacts, URL: availability.URL, Location: availability.Location,
				Priority: availability.Priority, Sequence: availability.Sequence, Created: availability.Created,
				LastModified: availability.LastModified, DateTimeStamp: availability.DateTimeStamp,
				Start: availability.Start, End: availability.End, Duration: availability.Duration,
				Available: availability.Available, RawProps: availability.RawProps,
			})
		}
		out["availabilities"] = items
	}
	if len(c.Journals) > 0 {
		items := make([]jr, 0, len(c.Journals))
		for _, journal := range c.Journals {
			items = append(items, jr{
				UID: journal.UID, Summary: journal.Summary, Description: journal.Description, DescriptionHTML: journal.DescriptionHTML,
				DTStart: journal.DTStart, Organizer: journal.Organizer, Status: journal.Status, Class: journal.Class, Categories: journal.Categories,
				Contacts: journal.Contacts, RelatedTo: journal.RelatedTo, Conferences: journal.Conferences, URL: journal.URL, DateTimeStamp: journal.DateTimeStamp,
				Created: journal.Created, LastModified: journal.LastModified, Recurrence: journal.Recurrence,
				Attendees: journal.Attendees, Attachments: journal.Attachments, DiscoveredURLs: journal.DiscoveredURLs,
				Images: journal.Images, RawProps: journal.RawProps,
			})
		}
		out["journals"] = items
	}
	return out
}

func (c *CalendarInfo) FilterRange(start, end *time.Time) {
	if start == nil && end == nil {
		return
	}
	evs := make([]EventInfo, 0, len(c.Events))
	for _, e := range c.Events {
		if e.DTStart == nil {
			evs = append(evs, e)
			continue
		}
		ok := true
		if start != nil && e.DTStart.Before(*start) {
			ok = false
		}
		if end != nil && !e.DTStart.Before(*end) {
			ok = false
		}
		if ok {
			evs = append(evs, e)
		}
	}
	c.Events = evs
}

const (
	maxEvents      = 10000 // Maximum number of events per calendar
	maxTodos       = 10000 // Maximum number of todos
	maxAttachments = 1000  // Maximum attachments per event/todo
	maxAttendees   = 10000 // Maximum attendees per event/todo
	maxVCards      = 1000  // Maximum vcards per event/todo
	maxAlarms      = 100   // Maximum alarms per event/todo
)

func ParseICSFile(path string, defaultTZ string, maxBytes int64) (*CalendarInfo, error) {
	// Check file size before reading to prevent memory exhaustion
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && stat.Size() > maxBytes {
		return nil, fmt.Errorf("ICS file too large: %d bytes (max %d)", stat.Size(), maxBytes)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines, err := unfoldLines(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	var cal CalendarInfo
	var inEvent bool
	var cur map[string]string
	var ev EventInfo
	var inTodo bool
	var curTodo map[string]string
	var todo TodoInfo
	var inFreebusy bool
	var fb FreeBusyInfo
	var descEvent descriptionCapture
	var descTodo descriptionCapture
	var inJournal bool
	var jr JournalInfo
	var curJournal map[string]string
	var descJournal descriptionCapture

	locDefault := (*time.Location)(nil)
	if defaultTZ != "" {
		if l, err := time.LoadLocation(defaultTZ); err == nil {
			locDefault = l
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		upper := strings.ToUpper(line)
		if strings.HasPrefix(line, "BEGIN:VEVENT") {
			inEvent = true
			cur = map[string]string{}
			ev = EventInfo{
				Summary:         "Untitled",
				Attachments:     []AttachmentInfo{},
				Attendees:       []Attendee{},
				RawProps:        map[string][]string{},
				VCards:          []VCard{},
				Alarms:          []AlarmInfo{},
				Categories:      []string{},
				Resources:       []string{},
				Conferences:     []ConferenceInfo{},
				Contacts:        []string{},
				Comments:        []string{},
				RelatedTo:       []string{},
				RequestStatuses: []string{},
				Images:          []ImageInfo{},
			}
			descEvent = descriptionCapture{}
			continue
		}
		if strings.HasPrefix(line, "END:VEVENT") {
			if v, ok := cur["UID"]; ok && v != "" {
				ev.UID = ptr(unescapeICSText(v))
			}
			if v, ok := cur["SUMMARY"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					ev.Summary = cleaned
				}
			}
			if v, ok := cur["LOCATION"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					ev.Location = ptr(cleaned)
				}
			}
			if descEvent.hasPlain {
				ev.Description = ptr(descEvent.plain)
			}
			if descEvent.hasHTML {
				ev.DescriptionHTML = ptr(descEvent.html)
			}
			if v, ok := cur["ORGANIZER"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					ev.Organizer = ptr(cleaned)
				}
			}
			if v, ok := cur["STATUS"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					ev.Status = ptr(cleaned)
				}
			}
			if len(cal.Events) >= maxEvents {
				return nil, fmt.Errorf("too many events in calendar (max %d)", maxEvents)
			}
			cal.Events = append(cal.Events, ev)
			inEvent = false
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VTODO") {
			inTodo = true
			curTodo = map[string]string{}
			todo = TodoInfo{
				Summary:         "Untitled Task",
				Attachments:     []AttachmentInfo{},
				Attendees:       []Attendee{},
				VCards:          []VCard{},
				Alarms:          []AlarmInfo{},
				Categories:      []string{},
				Resources:       []string{},
				RawProps:        map[string][]string{},
				Contacts:        []string{},
				Comments:        []string{},
				RelatedTo:       []string{},
				RequestStatuses: []string{},
				Images:          []ImageInfo{},
			}
			descTodo = descriptionCapture{}
			continue
		}
		if strings.HasPrefix(upper, "END:VTODO") {
			if v, ok := curTodo["UID"]; ok && v != "" {
				todo.UID = ptr(unescapeICSText(v))
			}
			if v, ok := curTodo["SUMMARY"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					todo.Summary = cleaned
				}
			}
			if v, ok := curTodo["LOCATION"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					todo.Location = ptr(cleaned)
				}
			}
			if descTodo.hasPlain {
				todo.Description = ptr(descTodo.plain)
			}
			if descTodo.hasHTML {
				todo.DescriptionHTML = ptr(descTodo.html)
			}
			if v, ok := curTodo["ORGANIZER"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					todo.Organizer = ptr(cleaned)
				}
			}
			if v, ok := curTodo["STATUS"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					todo.Status = ptr(cleaned)
				}
			}
			if len(cal.Todos) >= maxTodos {
				return nil, fmt.Errorf("too many todos in calendar (max %d)", maxTodos)
			}
			cal.Todos = append(cal.Todos, todo)
			inTodo = false
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VJOURNAL") {
			inJournal = true
			curJournal = map[string]string{}
			jr = JournalInfo{
				Summary:     "Untitled Journal Entry",
				Categories:  []string{},
				Contacts:    []string{},
				RelatedTo:   []string{},
				Images:      []ImageInfo{},
				Conferences: []ConferenceInfo{},
				Attendees:   []Attendee{},
				Attachments: []AttachmentInfo{},
				RawProps:    map[string][]string{},
			}
			descJournal = descriptionCapture{}
			continue
		}
		if strings.HasPrefix(upper, "END:VJOURNAL") {
			if v, ok := curJournal["UID"]; ok && v != "" {
				jr.UID = ptr(unescapeICSText(v))
			}
			if v, ok := curJournal["SUMMARY"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					jr.Summary = cleaned
				}
			}
			if v, ok := curJournal["ORGANIZER"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					jr.Organizer = ptr(cleaned)
				}
			}
			if v, ok := curJournal["STATUS"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					jr.Status = ptr(cleaned)
				}
			}
			if v, ok := curJournal["CLASS"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					jr.Class = ptr(cleaned)
				}
			}
			if v, ok := curJournal["URL"]; ok {
				if cleaned := strings.TrimSpace(unescapeICSText(v)); cleaned != "" {
					jr.URL = ptr(cleaned)
					jr.DiscoveredURLs = append(jr.DiscoveredURLs, cleaned)
				}
			}
			if descJournal.hasPlain {
				jr.Description = ptr(descJournal.plain)
			}
			if descJournal.hasHTML {
				jr.DescriptionHTML = ptr(descJournal.html)
			}
			if len(cal.Journals) >= maxEvents {
				return nil, fmt.Errorf("too many journals in calendar (max %d)", maxEvents)
			}
			cal.Journals = append(cal.Journals, jr)
			inJournal = false
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VFREEBUSY") {
			inFreebusy = true
			fb = FreeBusyInfo{
				Periods:     []FreeBusyPeriod{},
				Comments:    []string{},
				VCards:      []VCard{},
				Attendees:   []Attendee{},
				Attachments: []AttachmentInfo{},
				RawProps:    map[string][]string{},
			}
			continue
		}
		if strings.HasPrefix(upper, "END:VFREEBUSY") {
			cal.FreeBusy = append(cal.FreeBusy, fb)
			inFreebusy = false
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VALARM") {
			alarm, endIdx := parseValarm(lines, i, locDefault)
			if inEvent {
				if len(ev.Alarms) >= maxAlarms {
					return nil, fmt.Errorf("too many alarms in event (max %d)", maxAlarms)
				}
				ev.Alarms = append(ev.Alarms, alarm)
			} else if inTodo {
				if len(todo.Alarms) >= maxAlarms {
					return nil, fmt.Errorf("too many alarms in todo (max %d)", maxAlarms)
				}
				todo.Alarms = append(todo.Alarms, alarm)
			}
			i = endIdx
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VCARD") {
			card, endIdx := parseVCardBlock(lines, i)
			if inEvent {
				if len(ev.VCards) >= maxVCards {
					return nil, fmt.Errorf("too many vcards in event (max %d)", maxVCards)
				}
				ev.VCards = append(ev.VCards, card)
			} else if inTodo {
				if len(todo.VCards) >= maxVCards {
					return nil, fmt.Errorf("too many vcards in todo (max %d)", maxVCards)
				}
				todo.VCards = append(todo.VCards, card)
			} else if inFreebusy {
				if len(fb.VCards) >= maxVCards {
					return nil, fmt.Errorf("too many vcards in freebusy (max %d)", maxVCards)
				}
				fb.VCards = append(fb.VCards, card)
			} else {
				if len(cal.VCards) >= maxVCards {
					return nil, fmt.Errorf("too many vcards in calendar (max %d)", maxVCards)
				}
				cal.VCards = append(cal.VCards, card)
			}
			i = endIdx
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VTIMEZONE") {
			tz, endIdx := parseVTimezone(lines, i, locDefault)
			cal.Timezones = append(cal.Timezones, tz)
			i = endIdx
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VAVAILABILITY") {
			availability, endIdx := parseAvailability(lines, i, locDefault)
			cal.Availabilities = append(cal.Availabilities, availability)
			i = endIdx
			continue
		}

		if strings.HasPrefix(line, "BEGIN:VCALENDAR") {
			continue
		}
		if strings.HasPrefix(line, "END:VCALENDAR") {
			continue
		}

		if !inEvent && !inTodo && !inFreebusy && !inJournal {
			name, params, value := splitProp(line)
			switch name {
			case "PRODID":
				if value != "" {
					cal.ProdID = ptr(value)
				}
			case "METHOD":
				if value != "" {
					cal.Method = ptr(value)
				}
			case "X-WR-CALNAME", "NAME", "SUMMARY":
				if value != "" && cal.Name == nil {
					cal.Name = ptr(value)
				}
			case "CALSCALE":
				if value != "" {
					cal.Calscale = ptr(value)
				}
			case "URL":
				if value != "" {
					cal.URL = ptr(value)
				}
			case "X-WR-TIMEZONE":
				if value != "" {
					cal.TimezoneID = ptr(value)
				}
			case "X-WR-CALDESC", "DESCRIPTION":
				if value != "" && cal.Description == nil {
					cal.Description = ptr(value)
				}
			case "COLOR":
				if value != "" {
					cal.Color = ptr(value)
				}
			case "SOURCE":
				if value != "" {
					cal.Source = ptr(value)
				}
			case "REFRESH-INTERVAL":
				if value != "" {
					cal.RefreshInterval = ptr(value)
				}
			case "CATEGORIES":
				parts := splitEscaped(value, ',', false)
				for _, p := range parts {
					p = strings.TrimSpace(unescapeICSText(p))
					if p != "" {
						cal.Categories = append(cal.Categories, p)
					}
				}
			case "CONTACT":
				contact := strings.TrimSpace(unescapeICSText(value))
				if contact != "" {
					cal.Contacts = append(cal.Contacts, contact)
				}
			case "IMAGE":
				img := parseImage(value, params)
				cal.Images = append(cal.Images, img)
			}
			_ = params
			continue
		}

		name, params, value := splitProp(line)
		up := strings.ToUpper(name)

		if inEvent {
			switch up {
			case "DESCRIPTION":
				descEvent.absorbDescription(value)
			case "UID", "SUMMARY", "LOCATION", "ORGANIZER", "STATUS":
				cur[up] = value
			case "X-ALT-DESC":
				descEvent.absorbAltDescription(value, params)
				ev.RawProps = addRawProp(ev.RawProps, up, value)
			case "DTSTART":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					ev.DTStart = &t
				}
			case "DTEND":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					ev.DTEnd = &t
				}
			case "DTSTAMP":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					ev.DateTimeStamp = &t
				}
			case "CREATED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					ev.Created = &t
				}
			case "LAST-MODIFIED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					ev.LastModified = &t
				}
			case "SEQUENCE":
				if n, ok := parseInt(value); ok {
					ev.Sequence = n
				}
			case "TRANSP":
				if value != "" {
					ev.Transparency = ptr(value)
				}
			case "CLASS":
				if value != "" {
					ev.Class = ptr(value)
				}
			case "PRIORITY":
				if n, ok := parseInt(value); ok {
					ev.Priority = n
				}
			case "CATEGORIES":
				parts := splitEscaped(value, ',', false)
				for _, p := range parts {
					p = strings.TrimSpace(unescapeICSText(p))
					if p != "" {
						ev.Categories = append(ev.Categories, p)
					}
				}
			case "RESOURCES":
				parts := splitEscaped(value, ',', false)
				for _, p := range parts {
					p = strings.TrimSpace(unescapeICSText(p))
					if p != "" {
						ev.Resources = append(ev.Resources, p)
					}
				}
			case "CONTACT":
				contact := strings.TrimSpace(unescapeICSText(value))
				if contact != "" {
					ev.Contacts = append(ev.Contacts, contact)
				}
			case "COMMENT":
				comment := strings.TrimSpace(unescapeICSText(value))
				if comment != "" {
					ev.Comments = append(ev.Comments, comment)
				}
			case "RELATED", "RELATED-TO":
				rel := strings.TrimSpace(unescapeICSText(value))
				if rel != "" {
					ev.RelatedTo = append(ev.RelatedTo, rel)
				}
			case "REQUEST-STATUS":
				if value != "" {
					ev.RequestStatuses = append(ev.RequestStatuses, value)
				}
			case "URL":
				if value != "" {
					ev.URL = ptr(value)
				}
			case "COLOR":
				if value != "" {
					ev.Color = ptr(value)
				}
			case "GEO":
				if geo, ok := parseGeo(value); ok {
					ev.Geo = geo
				}
			case "IMAGE":
				img := parseImage(value, params)
				ev.Images = append(ev.Images, img)
			case "CONFERENCE":
				conf := ConferenceInfo{URI: value}
				if paramsCopy := copyParams(params); paramsCopy != nil {
					conf.Params = paramsCopy
				}
				ev.Conferences = append(ev.Conferences, conf)
			case "RRULE":
				rec := ensureRecurrence(&ev)
				rec.RRule = ptr(value)
			case "RDATE":
				rec := ensureRecurrence(&ev)
				times, rawVals := parseICSMultiDates(value, params, locDefault)
				if len(times) > 0 {
					rec.RDates = append(rec.RDates, times...)
				}
				if len(rawVals) > 0 {
					rec.RDateRaw = append(rec.RDateRaw, rawVals...)
				}
			case "EXDATE":
				rec := ensureRecurrence(&ev)
				times, rawVals := parseICSMultiDates(value, params, locDefault)
				if len(times) > 0 {
					rec.ExDates = append(rec.ExDates, times...)
				}
				if len(rawVals) > 0 {
					rec.ExDateRaw = append(rec.ExDateRaw, rawVals...)
				}
			case "RECURRENCE-ID":
				rec := ensureRecurrence(&ev)
				if t, ok := parseICSTime(value, params, locDefault); ok {
					rec.RecurrenceID = &t
				} else if value != "" {
					rec.RecurrenceIDRaw = ptr(value)
				}
			case "DURATION":
				if value != "" {
					ev.Duration = ptr(value)
					rec := ensureRecurrence(&ev)
					rec.Duration = ptr(value)
				}
			case "ATTACH":
				if len(ev.Attachments) >= maxAttachments {
					return nil, fmt.Errorf("too many attachments in event (max %d)", maxAttachments)
				}
				att := parseAttach(value, params)
				ev.Attachments = append(ev.Attachments, att)
			case "ATTENDEE":
				if len(ev.Attendees) >= maxAttendees {
					return nil, fmt.Errorf("too many attendees in event (max %d)", maxAttendees)
				}
				ev.Attendees = append(ev.Attendees, parseAttendee(value, params))
			default:
				ev.RawProps = addRawProp(ev.RawProps, up, value)
			}
			continue
		}

		if inTodo {
			switch up {
			case "DESCRIPTION":
				descTodo.absorbDescription(value)
			case "UID", "SUMMARY", "LOCATION", "ORGANIZER", "STATUS":
				curTodo[up] = value
			case "X-ALT-DESC":
				descTodo.absorbAltDescription(value, params)
				todo.RawProps = addRawProp(todo.RawProps, up, value)
			case "DTSTART":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					todo.Start = &t
				}
			case "DUE":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					todo.Due = &t
				}
			case "COMPLETED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					todo.Completed = &t
				}
			case "CREATED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					todo.Created = &t
				}
			case "LAST-MODIFIED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					todo.LastModified = &t
				}
			case "DTSTAMP":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					todo.DateTimeStamp = &t
				}
			case "SEQUENCE":
				if n, ok := parseInt(value); ok {
					todo.Sequence = n
				}
			case "PRIORITY":
				if n, ok := parseInt(value); ok {
					todo.Priority = n
				}
			case "PERCENT-COMPLETE":
				if n, ok := parseInt(value); ok {
					todo.PercentComplete = n
				}
			case "CATEGORIES":
				parts := splitEscaped(value, ',', false)
				for _, p := range parts {
					p = strings.TrimSpace(unescapeICSText(p))
					if p != "" {
						todo.Categories = append(todo.Categories, p)
					}
				}
			case "RESOURCES":
				parts := splitEscaped(value, ',', false)
				for _, p := range parts {
					p = strings.TrimSpace(unescapeICSText(p))
					if p != "" {
						todo.Resources = append(todo.Resources, p)
					}
				}
			case "CONTACT":
				contact := strings.TrimSpace(unescapeICSText(value))
				if contact != "" {
					todo.Contacts = append(todo.Contacts, contact)
				}
			case "COMMENT":
				comment := strings.TrimSpace(unescapeICSText(value))
				if comment != "" {
					todo.Comments = append(todo.Comments, comment)
				}
			case "RELATED", "RELATED-TO":
				rel := strings.TrimSpace(unescapeICSText(value))
				if rel != "" {
					todo.RelatedTo = append(todo.RelatedTo, rel)
				}
			case "REQUEST-STATUS":
				if value != "" {
					todo.RequestStatuses = append(todo.RequestStatuses, value)
				}
			case "COLOR":
				if value != "" {
					todo.Color = ptr(value)
				}
			case "IMAGE":
				img := parseImage(value, params)
				todo.Images = append(todo.Images, img)
			case "URL":
				if value != "" {
					todo.URL = ptr(value)
				}
			case "RRULE":
				rec := ensureTodoRecurrence(&todo)
				rec.RRule = ptr(value)
			case "RDATE":
				rec := ensureTodoRecurrence(&todo)
				times, rawVals := parseICSMultiDates(value, params, locDefault)
				if len(times) > 0 {
					rec.RDates = append(rec.RDates, times...)
				}
				if len(rawVals) > 0 {
					rec.RDateRaw = append(rec.RDateRaw, rawVals...)
				}
			case "EXDATE":
				rec := ensureTodoRecurrence(&todo)
				times, rawVals := parseICSMultiDates(value, params, locDefault)
				if len(times) > 0 {
					rec.ExDates = append(rec.ExDates, times...)
				}
				if len(rawVals) > 0 {
					rec.ExDateRaw = append(rec.ExDateRaw, rawVals...)
				}
			case "RECURRENCE-ID":
				rec := ensureTodoRecurrence(&todo)
				if t, ok := parseICSTime(value, params, locDefault); ok {
					rec.RecurrenceID = &t
				} else if value != "" {
					rec.RecurrenceIDRaw = ptr(value)
				}
			case "DURATION":
				if value != "" {
					todo.Duration = ptr(value)
					rec := ensureTodoRecurrence(&todo)
					rec.Duration = ptr(value)
				}
			case "ATTACH":
				if len(todo.Attachments) >= maxAttachments {
					return nil, fmt.Errorf("too many attachments in todo (max %d)", maxAttachments)
				}
				att := parseAttach(value, params)
				todo.Attachments = append(todo.Attachments, att)
			case "ATTENDEE":
				if len(todo.Attendees) >= maxAttendees {
					return nil, fmt.Errorf("too many attendees in todo (max %d)", maxAttendees)
				}
				todo.Attendees = append(todo.Attendees, parseAttendee(value, params))
			default:
				todo.RawProps = addRawProp(todo.RawProps, up, value)
			}
			continue
		}

		if inJournal {
			switch up {
			case "DESCRIPTION":
				descJournal.absorbDescription(value)
			case "UID", "SUMMARY", "ORGANIZER", "STATUS", "CLASS", "URL":
				curJournal[up] = value
			case "CATEGORIES":
				parts := splitEscaped(value, ',', false)
				for _, p := range parts {
					p = strings.TrimSpace(unescapeICSText(p))
					if p != "" {
						jr.Categories = append(jr.Categories, p)
					}
				}
			case "CONTACT":
				contact := strings.TrimSpace(unescapeICSText(value))
				if contact != "" {
					jr.Contacts = append(jr.Contacts, contact)
				}
			case "RELATED", "RELATED-TO":
				rel := strings.TrimSpace(unescapeICSText(value))
				if rel != "" {
					jr.RelatedTo = append(jr.RelatedTo, rel)
				}
			case "DTSTART":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					jr.DTStart = &t
				}
			case "DTSTAMP":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					jr.DateTimeStamp = &t
				}
			case "CREATED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					jr.Created = &t
				}
			case "LAST-MODIFIED":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					jr.LastModified = &t
				}
			case "X-ALT-DESC":
				descJournal.absorbAltDescription(value, params)
				jr.RawProps = addRawProp(jr.RawProps, up, value)
			case "RRULE":
				rec := ensureJournalRecurrence(&jr)
				rec.RRule = ptr(value)
			case "RDATE":
				rec := ensureJournalRecurrence(&jr)
				times, rawVals := parseICSMultiDates(value, params, locDefault)
				if len(times) > 0 {
					rec.RDates = append(rec.RDates, times...)
				}
				if len(rawVals) > 0 {
					rec.RDateRaw = append(rec.RDateRaw, rawVals...)
				}
			case "EXDATE":
				rec := ensureJournalRecurrence(&jr)
				times, rawVals := parseICSMultiDates(value, params, locDefault)
				if len(times) > 0 {
					rec.ExDates = append(rec.ExDates, times...)
				}
				if len(rawVals) > 0 {
					rec.ExDateRaw = append(rec.ExDateRaw, rawVals...)
				}
			case "RECURRENCE-ID":
				rec := ensureJournalRecurrence(&jr)
				if t, ok := parseICSTime(value, params, locDefault); ok {
					rec.RecurrenceID = &t
				} else if value != "" {
					rec.RecurrenceIDRaw = ptr(value)
				}
			case "CONFERENCE":
				conf := ConferenceInfo{URI: value}
				if paramsCopy := copyParams(params); paramsCopy != nil {
					conf.Params = paramsCopy
				}
				jr.Conferences = append(jr.Conferences, conf)
			case "ATTACH":
				if len(jr.Attachments) >= maxAttachments {
					return nil, fmt.Errorf("too many attachments in journal (max %d)", maxAttachments)
				}
				att := parseAttach(value, params)
				jr.Attachments = append(jr.Attachments, att)
			case "ATTENDEE":
				if len(jr.Attendees) >= maxAttendees {
					return nil, fmt.Errorf("too many attendees in journal (max %d)", maxAttendees)
				}
				jr.Attendees = append(jr.Attendees, parseAttendee(value, params))
			case "IMAGE":
				img := parseImage(value, params)
				jr.Images = append(jr.Images, img)
			default:
				jr.RawProps = addRawProp(jr.RawProps, up, value)
			}
			continue
		}

		if inFreebusy {
			switch up {
			case "UID":
				if value != "" {
					fb.UID = ptr(value)
				}
			case "DTSTART":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					fb.Start = &t
				}
			case "DTEND":
				if t, ok := parseICSTime(value, params, locDefault); ok {
					fb.End = &t
				}
			case "FREEBUSY":
				periods := parseFreeBusyPeriods(value, params, locDefault)
				if len(periods) > 0 {
					fb.Periods = append(fb.Periods, periods...)
				}
			case "COMMENT":
				if value != "" {
					fb.Comments = append(fb.Comments, unescapeICSText(value))
				}
			case "ORGANIZER":
				if value != "" {
					fb.Organizer = ptr(value)
				}
			case "CONTACT":
				if value != "" {
					fb.Contact = ptr(value)
				}
			case "URL":
				if value != "" {
					fb.URL = ptr(value)
				}
			case "ATTENDEE":
				fb.Attendees = append(fb.Attendees, parseAttendee(value, params))
			case "ATTACH":
				att := parseAttach(value, params)
				fb.Attachments = append(fb.Attachments, att)
			default:
				fb.RawProps = addRawProp(fb.RawProps, up, value)
			}
			continue
		}
	}

	for i := 1; i < len(cal.Events); i++ {
		j := i
		for j > 0 && lt(cal.Events[j], cal.Events[j-1]) {
			cal.Events[j], cal.Events[j-1] = cal.Events[j-1], cal.Events[j]
			j--
		}
	}
	return &cal, nil
}

func lt(a, b EventInfo) bool {
	if a.DTStart == nil && b.DTStart == nil {
		return false
	}
	if a.DTStart == nil {
		return false
	}
	if b.DTStart == nil {
		return true
	}
	return a.DTStart.Before(*b.DTStart)
}

const (
	maxFoldedLineLength     = 10 * 1024 * 1024  // 10MB max per logical line (normal properties)
	maxAttachPropertyLength = 150 * 1024 * 1024 // 150MB max for ATTACH properties (allows 100MB base64 + overhead)
)

func unfoldLines(r *bytes.Reader) ([]string, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var res []string
	var cur strings.Builder
	var isAttachProperty bool

	for s.Scan() {
		line := s.Text()
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			trimmed := strings.TrimLeft(line, " \t")
			// Use larger limit for ATTACH properties to handle inline base64 attachments
			limit := maxFoldedLineLength
			if isAttachProperty {
				limit = maxAttachPropertyLength
			}
			// Check folded line length limit to prevent memory exhaustion
			if cur.Len()+len(trimmed) > limit {
				return nil, fmt.Errorf("folded line exceeds maximum length of %d bytes", limit)
			}
			cur.WriteString(trimmed)
			continue
		}
		if cur.Len() > 0 {
			res = append(res, cur.String())
			cur.Reset()
			isAttachProperty = false
		}
		// Check if this is an ATTACH property
		if strings.HasPrefix(strings.ToUpper(line), "ATTACH") {
			isAttachProperty = true
		}
		limit := maxFoldedLineLength
		if isAttachProperty {
			limit = maxAttachPropertyLength
		}
		if len(line) > limit {
			return nil, fmt.Errorf("line exceeds maximum length of %d bytes", limit)
		}
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		res = append(res, cur.String())
	}
	return res, nil
}

func splitProp(line string) (name string, params map[string]string, value string) {
	params = map[string]string{}
	i := -1
	inQuote := false
	for idx := 0; idx < len(line); idx++ {
		ch := line[idx]
		if ch == '"' {
			if idx == 0 || line[idx-1] != '\\' {
				inQuote = !inQuote
			}
			continue
		}
		if ch == ':' && !inQuote {
			i = idx
			break
		}
	}
	if i < 0 {
		return strings.ToUpper(line), params, ""
	}
	head := line[:i]
	value = line[i+1:]
	parts := strings.Split(head, ";")
	name = strings.ToUpper(parts[0])
	for _, p := range parts[1:] {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			params[strings.ToUpper(strings.TrimSpace(kv[0]))] = trimQuotes(strings.TrimSpace(kv[1]))
		} else {
			params[strings.ToUpper(strings.TrimSpace(p))] = ""
		}
	}
	return
}

func trimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func parseICSTime(value string, params map[string]string, locDefault *time.Location) (time.Time, bool) {
	if strings.EqualFold(params["VALUE"], "DATE") && len(value) == 8 {
		t, err := time.ParseInLocation("20060102", value, time.UTC)
		return t, err == nil
	}
	loc := time.Local
	if tzid, ok := params["TZID"]; ok && tzid != "" {
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		}
	} else if locDefault != nil {
		loc = locDefault
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		return t, err == nil
	}
	if len(value) == len("20060102T150405") {
		t, err := time.ParseInLocation("20060102T150405", value, loc)
		return t, err == nil
	}
	if len(value) == len("20060102T1504") {
		t, err := time.ParseInLocation("20060102T1504", value, loc)
		return t, err == nil
	}
	if t, err := time.ParseInLocation("20060102T150405", value, loc); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func parseAttach(value string, params map[string]string) AttachmentInfo {
	fmt := params["FMTTYPE"]
	enc := strings.ToUpper(params["ENCODING"])
	val := strings.ToUpper(params["VALUE"])

	if enc == "BASE64" || val == "BINARY" {
		clean := strings.ReplaceAll(value, "\n", "")
		clean = strings.ReplaceAll(clean, "\r", "")
		clean = strings.ReplaceAll(clean, " ", "")
		return AttachmentInfo{Source: "inline", Value: clean, Fmt: nz(fmt)}
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:") {
		return AttachmentInfo{Source: "url", Value: value, Fmt: nz(fmt)}
	}
	if _, err := base64.StdEncoding.DecodeString(value); err == nil {
		return AttachmentInfo{Source: "inline", Value: value, Fmt: nz(fmt)}
	}
	return AttachmentInfo{Source: "url", Value: value, Fmt: nz(fmt)}
}

func parseAttendee(value string, params map[string]string) Attendee {
	a := Attendee{}
	lv := strings.ToLower(value)
	if strings.HasPrefix(lv, "mailto:") {
		a.Mailto = value[7:]
	} else {
		a.Mailto = value
	}
	if v := params["CN"]; v != "" {
		a.CN = &v
	}
	if v := params["ROLE"]; v != "" {
		a.Role = &v
	}
	if v := params["PARTSTAT"]; v != "" {
		a.PartStat = &v
	}
	if v := params["RSVP"]; v != "" {
		a.RSVP = &v
	}
	if v := params["CUTYPE"]; v != "" {
		a.Cutype = &v
	}
	if v := params["SENT-BY"]; v != "" {
		normalized := stripMailto(trimQuotes(strings.TrimSpace(v)))
		if normalized != "" {
			a.SentBy = &normalized
		}
	}
	if v := params["DIR"]; v != "" {
		dir := trimQuotes(v)
		if dir != "" {
			a.Directory = &dir
		}
	}
	if v := params["LANGUAGE"]; v != "" {
		lang := strings.TrimSpace(v)
		if lang != "" {
			a.Language = &lang
		}
	}
	if v := params["DELEGATED-FROM"]; v != "" {
		if list := parseCalAddressList(v); len(list) > 0 {
			a.DelegatedFrom = list
		}
	}
	if v := params["DELEGATED-TO"]; v != "" {
		if list := parseCalAddressList(v); len(list) > 0 {
			a.DelegatedTo = list
		}
	}
	if v := params["MEMBER"]; v != "" {
		if list := parseCalAddressList(v); len(list) > 0 {
			a.Member = list
		}
	}
	return a
}

func stripMailto(s string) string {
	ls := strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(ls, "mailto:") {
		return strings.TrimSpace(s)[7:]
	}
	return strings.TrimSpace(s)
}

func parseCalAddressList(raw string) []string {
	parts := splitEscaped(raw, ',', false)
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		val := stripMailto(trimQuotes(strings.TrimSpace(part)))
		if val == "" {
			continue
		}
		list = append(list, val)
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

func unescapeICSText(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\N", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\T", "\t")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

var (
	skipElements        = map[string]bool{"script": true, "style": true, "noscript": true}
	blockBefore         = map[string]bool{"p": true, "div": true, "section": true, "article": true, "header": true, "footer": true, "table": true, "tr": true, "tbody": true, "thead": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}
	blockAfter          = map[string]bool{"p": true, "div": true, "section": true, "article": true, "header": true, "footer": true, "table": true, "tr": true, "tbody": true, "thead": true, "ul": true, "ol": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true}
	spaceCollapse       = regexp.MustCompile(`[ \t\f\r\v]+`)
	htmlTagDetector     = regexp.MustCompile(`(?is)</?[a-z][^>]*>`)
	allowedDescElements = map[string]bool{
		"a": true, "abbr": true, "b": true, "br": true, "code": true, "div": true, "em": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"hr": true, "i": true, "li": true, "ol": true, "p": true, "span": true, "strong": true,
		"sub": true, "sup": true, "table": true, "tbody": true, "thead": true, "tfoot": true,
		"th": true, "td": true, "tr": true, "u": true, "ul": true,
	}
	allowedDescVoidElements = map[string]bool{"br": true, "hr": true}
	allowedDescAttributes   = map[string]map[string]bool{
		"a":  {"href": true, "title": true},
		"th": {"colspan": true, "rowspan": true, "scope": true},
		"td": {"colspan": true, "rowspan": true},
	}
	allowedDescGlobalAttrs = map[string]bool{
		"title": true,
	}
	safeURLSchemes     = map[string]bool{"http": true, "https": true, "mailto": true, "tel": true}
	allowedScopeValues = map[string]bool{"row": true, "col": true, "rowgroup": true, "colgroup": true}
)

type descriptionCapture struct {
	plain    string
	html     string
	hasPlain bool
	hasHTML  bool
}

func (d *descriptionCapture) absorbDescription(value string) {
	text := strings.TrimSpace(unescapeICSText(value))
	if text == "" {
		return
	}
	if looksLikeHTML(text) {
		if html := sanitizeDescriptionHTML(text); html != "" {
			d.html = html
			d.hasHTML = true
		}
		plain := strings.TrimSpace(htmlToPlainText(text))
		if plain == "" {
			plain = strings.TrimSpace(fallbackPlainText(text))
		}
		if plain != "" {
			d.plain = plain
			d.hasPlain = true
		} else {
			cleaned := strings.TrimSpace(htmlstd.UnescapeString(text))
			if cleaned != "" {
				d.plain = cleaned
				d.hasPlain = true
			}
		}
		return
	}
	cleaned := htmlstd.UnescapeString(text)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned != "" {
		d.plain = cleaned
		d.hasPlain = true
	}
}

func (d *descriptionCapture) absorbAltDescription(value string, params map[string]string) {
	text := strings.TrimSpace(unescapeICSText(value))
	if text == "" {
		return
	}
	fmtType := strings.ToLower(params["FMTTYPE"])
	htmlHint := strings.Contains(fmtType, "html")
	if htmlHint || looksLikeHTML(text) {
		if html := sanitizeDescriptionHTML(text); html != "" {
			d.html = html
			d.hasHTML = true
		}
		if !d.hasPlain {
			plain := strings.TrimSpace(htmlToPlainText(text))
			if plain == "" {
				plain = strings.TrimSpace(fallbackPlainText(text))
			}
			if plain != "" {
				d.plain = plain
				d.hasPlain = true
			} else {
				cleaned := strings.TrimSpace(htmlstd.UnescapeString(text))
				if cleaned != "" {
					d.plain = cleaned
					d.hasPlain = true
				}
			}
		}
		return
	}
	if !d.hasPlain {
		cleaned := htmlstd.UnescapeString(text)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			d.plain = cleaned
			d.hasPlain = true
		}
	}
}
func htmlToPlainText(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	doc, err := htmlnode.Parse(strings.NewReader(trimmed))
	if err != nil {
		return fallbackPlainText(trimmed)
	}
	var writer plainTextWriter
	extractPlainText(doc, &writer, 0)
	out := normalizeWhitespace(writer.String())
	if out == "" {
		return fallbackPlainText(trimmed)
	}
	return out
}

func fallbackPlainText(input string) string {
	// Use the HTML parser to safely extract text instead of vulnerable regex patterns
	// This avoids ReDoS attacks from malicious nested tags
	doc, err := htmlnode.Parse(strings.NewReader(input))
	if err != nil {
		// If parsing fails completely, just unescape HTML entities and return
		return strings.TrimSpace(htmlstd.UnescapeString(input))
	}
	var writer plainTextWriter
	extractPlainText(doc, &writer, 0)
	result := normalizeWhitespace(writer.String())
	if result == "" {
		// Last resort: just unescape and return
		return strings.TrimSpace(htmlstd.UnescapeString(input))
	}
	return result
}

type plainTextWriter struct {
	b    strings.Builder
	last rune
}

func (w *plainTextWriter) writeText(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if w.last != 0 && w.last != '\n' && w.last != ' ' {
		w.b.WriteByte(' ')
	}
	w.b.WriteString(s)
	if r, size := utf8.DecodeLastRuneInString(s); size > 0 {
		w.last = r
	}
}

type sanitizedAttr struct {
	key string
	val string
}

func looksLikeHTML(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if !strings.Contains(trimmed, "<") || !strings.Contains(trimmed, ">") {
		return false
	}
	return htmlTagDetector.MatchString(trimmed)
}

const (
	maxHTMLDepth = 1000 // Maximum depth for HTML node recursion to prevent stack overflow
)

func sanitizeDescriptionHTML(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	ctx := &htmlnode.Node{Type: htmlnode.ElementNode, DataAtom: htmlatom.Div, Data: "div"}
	nodes, err := htmlnode.ParseFragment(strings.NewReader(trimmed), ctx)
	if err != nil {
		return ""
	}
	var buf strings.Builder
	for _, n := range nodes {
		renderSanitizedNode(&buf, n, 0)
	}
	out := strings.TrimSpace(buf.String())
	return out
}

func renderSanitizedNode(buf *strings.Builder, node *htmlnode.Node, depth int) {
	// Prevent stack overflow from deeply nested HTML
	if depth > maxHTMLDepth {
		return
	}
	switch node.Type {
	case htmlnode.TextNode:
		text := htmlstd.UnescapeString(node.Data)
		if text != "" {
			buf.WriteString(htmlstd.EscapeString(text))
		}
	case htmlnode.ElementNode:
		name := strings.ToLower(node.Data)
		if skipElements[name] {
			return
		}
		if !allowedDescElements[name] {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				renderSanitizedNode(buf, c, depth+1)
			}
			return
		}
		buf.WriteByte('<')
		buf.WriteString(name)
		for _, attr := range sanitizeDescAttributes(name, node.Attr) {
			buf.WriteByte(' ')
			buf.WriteString(attr.key)
			buf.WriteString(`="`)
			buf.WriteString(htmlstd.EscapeString(attr.val))
			buf.WriteByte('"')
		}
		if allowedDescVoidElements[name] {
			buf.WriteByte('>')
			return
		}
		buf.WriteByte('>')
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			renderSanitizedNode(buf, c, depth+1)
		}
		buf.WriteString("</")
		buf.WriteString(name)
		buf.WriteByte('>')
	default:
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			renderSanitizedNode(buf, c, depth+1)
		}
	}
}

func sanitizeDescAttributes(tag string, attrs []htmlnode.Attribute) []sanitizedAttr {
	allowed := allowedDescAttributes[tag]
	out := make([]sanitizedAttr, 0, len(attrs))
	for _, attr := range attrs {
		key := strings.ToLower(attr.Key)
		if key == "" {
			continue
		}
		if !(allowedDescGlobalAttrs[key] || (allowed != nil && allowed[key])) {
			continue
		}
		val := strings.TrimSpace(attr.Val)
		if val == "" {
			continue
		}
		switch key {
		case "href":
			if !isSafeLink(val) {
				continue
			}
		case "colspan", "rowspan":
			val = sanitizeSpanDigits(val)
			if val == "" {
				continue
			}
		case "scope":
			valLower := strings.ToLower(val)
			if !allowedScopeValues[valLower] {
				continue
			}
			val = valLower
		default:
			val = htmlstd.UnescapeString(val)
		}
		out = append(out, sanitizedAttr{key: key, val: val})
	}
	return out
}

func sanitizeSpanDigits(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	for _, r := range v {
		if !unicode.IsDigit(r) {
			return ""
		}
	}
	return v
}

func isSafeLink(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "//") {
		return false
	}
	lower := strings.ToLower(raw)

	// Block dangerous scheme prefixes from being rendered as clickable links
	// Note: The raw URL is still preserved in the manifest and plain text output
	// for security analysis - we just don't make them clickable in HTML
	dangerousSchemes := []string{
		"javascript:",
		"vbscript:",
		"data:",
		"file:",
		"about:",
	}
	for _, scheme := range dangerousSchemes {
		if strings.HasPrefix(lower, scheme) {
			return false
		}
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme == "" {
		// Scheme-less URLs - only allow if they don't look dangerous
		return true
	}
	return safeURLSchemes[strings.ToLower(parsed.Scheme)]
}
func (w *plainTextWriter) writeNewline() {
	if w.last == '\n' {
		return
	}
	if w.b.Len() > 0 {
		w.b.WriteByte('\n')
	}
	w.last = '\n'
}

func (w *plainTextWriter) ensureBlock() {
	if w.last == 0 || w.last == '\n' {
		return
	}
	w.writeNewline()
}

func (w *plainTextWriter) startBullet() {
	if w.last != 0 && w.last != '\n' {
		w.writeNewline()
	}
	w.b.WriteString("• ")
	w.last = ' '
}

func (w *plainTextWriter) String() string {
	return w.b.String()
}

func extractPlainText(n *htmlnode.Node, w *plainTextWriter, depth int) {
	// Prevent stack overflow from deeply nested HTML
	if depth > maxHTMLDepth {
		return
	}
	if n.Type == htmlnode.ElementNode {
		name := strings.ToLower(n.Data)
		if skipElements[name] {
			return
		}
		switch name {
		case "br":
			w.writeNewline()
		case "li":
			w.startBullet()
		default:
			if blockBefore[name] {
				w.ensureBlock()
			}
		}
	}
	if n.Type == htmlnode.TextNode {
		w.writeText(htmlstd.UnescapeString(n.Data))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractPlainText(c, w, depth+1)
	}
	if n.Type == htmlnode.ElementNode {
		name := strings.ToLower(n.Data)
		switch name {
		case "br":
			w.writeNewline()
		case "li":
			w.writeNewline()
		default:
			if blockAfter[name] {
				w.writeNewline()
			}
		}
	}
}

func normalizeWhitespace(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	trimmed := make([]string, 0, len(lines))
	prevBlank := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if prevBlank {
				continue
			}
			prevBlank = true
			trimmed = append(trimmed, "")
			continue
		}
		prevBlank = false
		trimmed = append(trimmed, line)
	}
	result := strings.Join(trimmed, "\n")
	result = spaceCollapse.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

func nz(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func ptr[T any](v T) *T { return &v }

// PlainTextFromHTML converts sanitized HTML fragments to readable text while
// preserving paragraph and list structure. It is safe to call on already
// sanitized markup.
func PlainTextFromHTML(input string) string {
	text := htmlToPlainText(input)
	if text == "" {
		return fallbackPlainText(input)
	}
	return text
}

// ExtractURLs finds http/https URLs in free text.
func ExtractURLs(s string) []string {
	// very simple matcher; good enough for manifest enrichment
	re := regexp.MustCompile(`(?i)\bhttps?://[^\s<>"']+`)
	m := re.FindAllString(s, -1)
	// de-dup preserving order
	seen := map[string]bool{}
	out := make([]string, 0, len(m))
	for _, u := range m {
		// Clean up URL - remove trailing escape sequences
		u = strings.TrimSuffix(u, "\\n")
		u = strings.TrimSuffix(u, "\\")
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// PopulateDiscoveredURLs scans summaries/descriptions/locations and attachments to fill DiscoveredURLs.
func PopulateDiscoveredURLs(c *CalendarInfo) {
	agg := []string{}
	seen := map[string]bool{}
	add := func(u string) {
		if !seen[u] {
			seen[u] = true
			agg = append(agg, u)
		}
	}
	for i := range c.Events {
		e := &c.Events[i]
		e.DiscoveredURLs = nil
		urls := []string{}
		urls = append(urls, ExtractURLs(e.Summary)...)
		if e.Description != nil {
			urls = append(urls, ExtractURLs(*e.Description)...)
		}
		if e.Location != nil {
			urls = append(urls, ExtractURLs(*e.Location)...)
		}
		if e.Organizer != nil {
			urls = append(urls, ExtractURLs(*e.Organizer)...)
		}
		if e.URL != nil {
			urls = append(urls, *e.URL)
		}
		for _, conf := range e.Conferences {
			if conf.URI != "" {
				urls = append(urls, conf.URI)
			}
		}
		for _, a := range e.Attachments {
			if a.Source == "url" && a.Value != "" {
				urls = append(urls, a.Value)
			}
		}
		// dedup per-event
		localSeen := map[string]bool{}
		for _, u := range urls {
			if !localSeen[u] {
				localSeen[u] = true
				e.DiscoveredURLs = append(e.DiscoveredURLs, u)
				add(u)
			}
		}
	}
	for i := range c.Todos {
		td := &c.Todos[i]
		td.DiscoveredURLs = nil
		urls := []string{}
		urls = append(urls, ExtractURLs(td.Summary)...)
		if td.Description != nil {
			urls = append(urls, ExtractURLs(*td.Description)...)
		}
		if td.Location != nil {
			urls = append(urls, ExtractURLs(*td.Location)...)
		}
		if td.Organizer != nil {
			urls = append(urls, ExtractURLs(*td.Organizer)...)
		}
		if td.URL != nil {
			urls = append(urls, *td.URL)
		}
		for _, att := range td.Attachments {
			if att.Source == "url" && att.Value != "" {
				urls = append(urls, att.Value)
			}
		}
		localSeen := map[string]bool{}
		for _, u := range urls {
			if !localSeen[u] {
				localSeen[u] = true
				td.DiscoveredURLs = append(td.DiscoveredURLs, u)
				add(u)
			}
		}
	}
	for i := range c.Journals {
		jn := &c.Journals[i]
		jn.DiscoveredURLs = nil
		urls := []string{}
		urls = append(urls, ExtractURLs(jn.Summary)...)
		if jn.Description != nil {
			urls = append(urls, ExtractURLs(*jn.Description)...)
		}
		if jn.Organizer != nil {
			urls = append(urls, ExtractURLs(*jn.Organizer)...)
		}
		if jn.URL != nil {
			urls = append(urls, *jn.URL)
		}
		for _, att := range jn.Attachments {
			if att.Source == "url" && att.Value != "" {
				urls = append(urls, att.Value)
			}
		}
		for _, conf := range jn.Conferences {
			if conf.URI != "" {
				urls = append(urls, conf.URI)
			}
		}
		localSeen := map[string]bool{}
		for _, u := range urls {
			if !localSeen[u] {
				localSeen[u] = true
				jn.DiscoveredURLs = append(jn.DiscoveredURLs, u)
				add(u)
			}
		}
	}
	for _, fb := range c.FreeBusy {
		if fb.URL != nil {
			add(*fb.URL)
		}
	}
	if c.URL != nil {
		add(*c.URL)
	}
	c.DiscoveredURLs = agg
}
