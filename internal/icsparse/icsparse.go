package icsparse

import (
	"bufio"
	"bytes"
	"encoding/base64"
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
	Mailto   string  `json:"mailto,omitempty"`
	CN       *string `json:"cn,omitempty"`
	Role     *string `json:"role,omitempty"`
	PartStat *string `json:"partstat,omitempty"`
	RSVP     *string `json:"rsvp,omitempty"`
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
	DiscoveredURLs  []string          `json:"discovered_urls,omitempty"`
	UID             *string           `json:"uid,omitempty"`
	Summary         string            `json:"summary"`
	DTStart         *time.Time        `json:"dtstart,omitempty"`
	DTEnd           *time.Time        `json:"dtend,omitempty"`
	Location        *string           `json:"location,omitempty"`
	Description     *string           `json:"description,omitempty"`
	DescriptionHTML *string           `json:"description_html,omitempty"`
	Organizer       *string           `json:"organizer,omitempty"`
	Status          *string           `json:"status,omitempty"`
	Attendees       []Attendee        `json:"attendees,omitempty"`
	Attachments     []AttachmentInfo  `json:"attachments"`
	VCards          []VCard           `json:"vcards,omitempty"`
	Alarms          []AlarmInfo       `json:"alarms,omitempty"`
	Categories      []string          `json:"categories,omitempty"`
	Resources       []string          `json:"resources,omitempty"`
	Conferences     []ConferenceInfo  `json:"conferences,omitempty"`
	Recurrence      *RecurrenceInfo   `json:"recurrence,omitempty"`
	Transparency    *string           `json:"transparency,omitempty"`
	Priority        *int              `json:"priority,omitempty"`
	Class           *string           `json:"class,omitempty"`
	URL             *string           `json:"url,omitempty"`
	Created         *time.Time        `json:"created,omitempty"`
	LastModified    *time.Time        `json:"last_modified,omitempty"`
	DateTimeStamp   *time.Time        `json:"dtstamp,omitempty"`
	Sequence        *int              `json:"sequence,omitempty"`
	Duration        *string           `json:"duration,omitempty"`
	RawProps        map[string]string `json:"raw_props"`
}

type CalendarInfo struct {
	DiscoveredURLs []string       `json:"discovered_urls,omitempty"`
	Name           *string        `json:"name,omitempty"`
	Method         *string        `json:"method,omitempty"`
	ProdID         *string        `json:"prodid,omitempty"`
	Events         []EventInfo    `json:"events"`
	VCards         []VCard        `json:"vcards,omitempty"`
	Description    *string        `json:"description,omitempty"`
	URL            *string        `json:"url,omitempty"`
	Calscale       *string        `json:"calscale,omitempty"`
	TimezoneID     *string        `json:"timezone_id,omitempty"`
	Timezones      []TimezoneInfo `json:"timezones,omitempty"`
	Todos          []TodoInfo     `json:"todos,omitempty"`
	FreeBusy       []FreeBusyInfo `json:"freebusy,omitempty"`
}

func (c *CalendarInfo) Manifest() map[string]any {
	out := map[string]any{
		"calendar": map[string]any{
			"name":        c.Name,
			"prodid":      c.ProdID,
			"method":      c.Method,
			"description": c.Description,
			"url":         c.URL,
			"calscale":    c.Calscale,
			"timezone_id": c.TimezoneID,
		},
	}
	type ev struct {
		UID             *string           `json:"uid,omitempty"`
		Summary         string            `json:"summary"`
		DTStart         *time.Time        `json:"dtstart,omitempty"`
		DTEnd           *time.Time        `json:"dtend,omitempty"`
		Location        *string           `json:"location,omitempty"`
		Description     *string           `json:"description,omitempty"`
		DescriptionHTML *string           `json:"description_html,omitempty"`
		Organizer       *string           `json:"organizer,omitempty"`
		Status          *string           `json:"status,omitempty"`
		Attendees       []Attendee        `json:"attendees,omitempty"`
		Attachments     []AttachmentInfo  `json:"attachments"`
		VCards          []VCard           `json:"vcards,omitempty"`
		Alarms          []AlarmInfo       `json:"alarms,omitempty"`
		Categories      []string          `json:"categories,omitempty"`
		Resources       []string          `json:"resources,omitempty"`
		Conferences     []ConferenceInfo  `json:"conferences,omitempty"`
		Recurrence      *RecurrenceInfo   `json:"recurrence,omitempty"`
		Transparency    *string           `json:"transparency,omitempty"`
		Priority        *int              `json:"priority,omitempty"`
		Class           *string           `json:"class,omitempty"`
		URL             *string           `json:"url,omitempty"`
		Created         *time.Time        `json:"created,omitempty"`
		LastModified    *time.Time        `json:"last_modified,omitempty"`
		DateTimeStamp   *time.Time        `json:"dtstamp,omitempty"`
		Sequence        *int              `json:"sequence,omitempty"`
		Duration        *string           `json:"duration,omitempty"`
		RawProps        map[string]string `json:"raw_props"`
		DiscoveredURLs  []string          `json:"discovered_urls,omitempty"`
	}
	type td struct {
		UID             *string           `json:"uid,omitempty"`
		Summary         string            `json:"summary"`
		Description     *string           `json:"description,omitempty"`
		DescriptionHTML *string           `json:"description_html,omitempty"`
		Location        *string           `json:"location,omitempty"`
		Organizer       *string           `json:"organizer,omitempty"`
		Status          *string           `json:"status,omitempty"`
		Priority        *int              `json:"priority,omitempty"`
		PercentComplete *int              `json:"percent_complete,omitempty"`
		Categories      []string          `json:"categories,omitempty"`
		Resources       []string          `json:"resources,omitempty"`
		URL             *string           `json:"url,omitempty"`
		Start           *time.Time        `json:"dtstart,omitempty"`
		Due             *time.Time        `json:"due,omitempty"`
		Completed       *time.Time        `json:"completed,omitempty"`
		Created         *time.Time        `json:"created,omitempty"`
		LastModified    *time.Time        `json:"last_modified,omitempty"`
		DateTimeStamp   *time.Time        `json:"dtstamp,omitempty"`
		Sequence        *int              `json:"sequence,omitempty"`
		Duration        *string           `json:"duration,omitempty"`
		Recurrence      *RecurrenceInfo   `json:"recurrence,omitempty"`
		Attendees       []Attendee        `json:"attendees,omitempty"`
		Attachments     []AttachmentInfo  `json:"attachments,omitempty"`
		VCards          []VCard           `json:"vcards,omitempty"`
		Alarms          []AlarmInfo       `json:"alarms,omitempty"`
		RawProps        map[string]string `json:"raw_props"`
		DiscoveredURLs  []string          `json:"discovered_urls,omitempty"`
	}
	type fb struct {
		UID         *string           `json:"uid,omitempty"`
		Organizer   *string           `json:"organizer,omitempty"`
		Contact     *string           `json:"contact,omitempty"`
		URL         *string           `json:"url,omitempty"`
		Start       *time.Time        `json:"dtstart,omitempty"`
		End         *time.Time        `json:"dtend,omitempty"`
		Periods     []FreeBusyPeriod  `json:"periods,omitempty"`
		Comments    []string          `json:"comments,omitempty"`
		VCards      []VCard           `json:"vcards,omitempty"`
		Attendees   []Attendee        `json:"attendees,omitempty"`
		Attachments []AttachmentInfo  `json:"attachments,omitempty"`
		RawProps    map[string]string `json:"raw_props"`
	}
	events := make([]ev, 0, len(c.Events))
	for _, e := range c.Events {
		events = append(events, ev{
			UID: e.UID, Summary: e.Summary, DTStart: e.DTStart, DTEnd: e.DTEnd,
			Location: e.Location, Description: e.Description, DescriptionHTML: e.DescriptionHTML, Organizer: e.Organizer,
			Status: e.Status, Attendees: e.Attendees,
			Attachments: e.Attachments, VCards: e.VCards, Alarms: e.Alarms,
			Categories: e.Categories, Resources: e.Resources, Conferences: e.Conferences,
			Recurrence: e.Recurrence, Transparency: e.Transparency, Priority: e.Priority,
			Class: e.Class, URL: e.URL, Created: e.Created, LastModified: e.LastModified,
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
				Categories: todo.Categories, Resources: todo.Resources, URL: todo.URL, Start: todo.Start, Due: todo.Due,
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

func ParseICSFile(path string, defaultTZ string) (*CalendarInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := unfoldLines(bytes.NewReader(b))

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
				Summary:     "Untitled",
				Attachments: []AttachmentInfo{},
				Attendees:   []Attendee{},
				RawProps:    map[string]string{},
				VCards:      []VCard{},
				Alarms:      []AlarmInfo{},
				Categories:  []string{},
				Resources:   []string{},
				Conferences: []ConferenceInfo{},
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
			cal.Events = append(cal.Events, ev)
			inEvent = false
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VTODO") {
			inTodo = true
			curTodo = map[string]string{}
			todo = TodoInfo{
				Summary:     "Untitled Task",
				Attachments: []AttachmentInfo{},
				Attendees:   []Attendee{},
				VCards:      []VCard{},
				Alarms:      []AlarmInfo{},
				Categories:  []string{},
				Resources:   []string{},
				RawProps:    map[string]string{},
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
			cal.Todos = append(cal.Todos, todo)
			inTodo = false
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
				RawProps:    map[string]string{},
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
				ev.Alarms = append(ev.Alarms, alarm)
			} else if inTodo {
				todo.Alarms = append(todo.Alarms, alarm)
			}
			i = endIdx
			continue
		}

		if strings.HasPrefix(upper, "BEGIN:VCARD") {
			card, endIdx := parseVCardBlock(lines, i)
			if inEvent {
				ev.VCards = append(ev.VCards, card)
			} else if inTodo {
				todo.VCards = append(todo.VCards, card)
			} else if inFreebusy {
				fb.VCards = append(fb.VCards, card)
			} else {
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

		if strings.HasPrefix(line, "BEGIN:VCALENDAR") {
			continue
		}
		if strings.HasPrefix(line, "END:VCALENDAR") {
			continue
		}

		if !inEvent && !inTodo && !inFreebusy {
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
				if value != "" {
					ev.RawProps[up] = value
				}
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
			case "URL":
				if value != "" {
					ev.URL = ptr(value)
				}
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
				att := parseAttach(value, params)
				ev.Attachments = append(ev.Attachments, att)
			case "ATTENDEE":
				ev.Attendees = append(ev.Attendees, parseAttendee(value, params))
			default:
				if value != "" {
					ev.RawProps[up] = value
				}
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
				if value != "" {
					todo.RawProps[up] = value
				}
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
				att := parseAttach(value, params)
				todo.Attachments = append(todo.Attachments, att)
			case "ATTENDEE":
				todo.Attendees = append(todo.Attendees, parseAttendee(value, params))
			default:
				if value != "" {
					todo.RawProps[up] = value
				}
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
				if value != "" {
					fb.RawProps[up] = value
				}
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

func unfoldLines(r *bytes.Reader) []string {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var res []string
	var cur string
	for s.Scan() {
		line := s.Text()
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			cur += strings.TrimLeft(line, " \t")
			continue
		}
		if cur != "" {
			res = append(res, cur)
		}
		cur = line
	}
	if cur != "" {
		res = append(res, cur)
	}
	return res
}

func splitProp(line string) (name string, params map[string]string, value string) {
	params = map[string]string{}
	i := strings.IndexByte(line, ':')
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
	return a
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
	scriptStripper      = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleStripper       = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	tagStripper         = regexp.MustCompile(`(?s)<[^>]+>`)
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
	extractPlainText(doc, &writer)
	out := normalizeWhitespace(writer.String())
	if out == "" {
		return fallbackPlainText(trimmed)
	}
	return out
}

func fallbackPlainText(input string) string {
	clean := scriptStripper.ReplaceAllString(input, "")
	clean = styleStripper.ReplaceAllString(clean, "")
	clean = tagStripper.ReplaceAllString(clean, " ")
	clean = spaceCollapse.ReplaceAllString(clean, " ")
	return strings.TrimSpace(htmlstd.UnescapeString(clean))
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
		renderSanitizedNode(&buf, n)
	}
	out := strings.TrimSpace(buf.String())
	return out
}

func renderSanitizedNode(buf *strings.Builder, node *htmlnode.Node) {
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
				renderSanitizedNode(buf, c)
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
			renderSanitizedNode(buf, c)
		}
		buf.WriteString("</")
		buf.WriteString(name)
		buf.WriteByte('>')
	default:
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			renderSanitizedNode(buf, c)
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
	if strings.HasPrefix(lower, "javascript:") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme == "" {
		return !strings.HasPrefix(lower, "data:")
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

func extractPlainText(n *htmlnode.Node, w *plainTextWriter) {
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
		extractPlainText(c, w)
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
