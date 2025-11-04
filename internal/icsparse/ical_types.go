package icsparse

import "time"

// RecurrenceInfo captures recurrence-related properties for an event.
type RecurrenceInfo struct {
	RRule           *string     `json:"rrule,omitempty"`
	RDates          []time.Time `json:"rdates,omitempty"`
	RDateRaw        []string    `json:"rdate_raw,omitempty"`
	ExDates         []time.Time `json:"exdates,omitempty"`
	ExDateRaw       []string    `json:"exdate_raw,omitempty"`
	RecurrenceID    *time.Time  `json:"recurrence_id,omitempty"`
	RecurrenceIDRaw *string     `json:"recurrence_id_raw,omitempty"`
	Duration        *string     `json:"duration,omitempty"`
}

// GeoPoint captures a geographic coordinate pair from a GEO property.
type GeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// ConferenceInfo reflects CONFERENCE properties that reference meeting joins.
type ConferenceInfo struct {
	URI    string            `json:"uri"`
	Params map[string]string `json:"params,omitempty"`
}

// ImageInfo represents RFC 7986 IMAGE properties the invite exposes.
type ImageInfo struct {
	Source  string  `json:"source"`
	Value   string  `json:"value"`
	FmtType *string `json:"fmt_type,omitempty"`
	Display *string `json:"display,omitempty"`
	AltRep  *string `json:"altrep,omitempty"`
}

// AlarmTrigger stores both raw and parsed interpretations of a VALARM trigger.
type AlarmTrigger struct {
	Raw      string     `json:"raw"`
	Time     *time.Time `json:"time,omitempty"`
	Duration *string    `json:"duration,omitempty"`
	Related  *string    `json:"related,omitempty"`
}

// AlarmInfo represents an alarm component embedded within an event.
type AlarmInfo struct {
	Action      *string             `json:"action,omitempty"`
	Trigger     *AlarmTrigger       `json:"trigger,omitempty"`
	Description *string             `json:"description,omitempty"`
	Summary     *string             `json:"summary,omitempty"`
	Duration    *string             `json:"duration,omitempty"`
	Repeat      *int                `json:"repeat,omitempty"`
	Attendees   []Attendee          `json:"attendees,omitempty"`
	Attachments []AttachmentInfo    `json:"attachments,omitempty"`
	RawProps    map[string][]string `json:"raw_props,omitempty"`
}

// TimezoneInfo persists VTIMEZONE definitions for reference by calendar consumers.
type TimezoneInfo struct {
	TZID         *string             `json:"tzid,omitempty"`
	LastModified *time.Time          `json:"last_modified,omitempty"`
	URL          *string             `json:"url,omitempty"`
	RawProps     map[string][]string `json:"raw_props,omitempty"`
	Periods      []TimezonePeriod    `json:"periods,omitempty"`
}

// TimezonePeriod represents STANDARD/DAYLIGHT sub-components inside a VTIMEZONE.
type TimezonePeriod struct {
	Type       string              `json:"type"`
	DTStart    *time.Time          `json:"dtstart,omitempty"`
	OffsetFrom *string             `json:"tzoffset_from,omitempty"`
	OffsetTo   *string             `json:"tzoffset_to,omitempty"`
	Name       *string             `json:"name,omitempty"`
	RRule      *string             `json:"rrule,omitempty"`
	RawProps   map[string][]string `json:"raw_props,omitempty"`
}

// TodoInfo captures data from VTODO components.
type TodoInfo struct {
	UID             *string             `json:"uid,omitempty"`
	Summary         string              `json:"summary"`
	Description     *string             `json:"description,omitempty"`
	DescriptionHTML *string             `json:"description_html,omitempty"`
	Location        *string             `json:"location,omitempty"`
	Organizer       *string             `json:"organizer,omitempty"`
	Status          *string             `json:"status,omitempty"`
	Priority        *int                `json:"priority,omitempty"`
	PercentComplete *int                `json:"percent_complete,omitempty"`
	Color           *string             `json:"color,omitempty"`
	Categories      []string            `json:"categories,omitempty"`
	Resources       []string            `json:"resources,omitempty"`
	Contacts        []string            `json:"contacts,omitempty"`
	Comments        []string            `json:"comments,omitempty"`
	RelatedTo       []string            `json:"related_to,omitempty"`
	RequestStatuses []string            `json:"request_statuses,omitempty"`
	Images          []ImageInfo         `json:"images,omitempty"`
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
	RawProps        map[string][]string `json:"raw_props,omitempty"`
	DiscoveredURLs  []string            `json:"discovered_urls,omitempty"`
}

// FreeBusyPeriod represents a busy period from a VFREEBUSY component.
type FreeBusyPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Type  *string   `json:"type,omitempty"`
}

// FreeBusyInfo captures VFREEBUSY data blocks for availability windows.
type FreeBusyInfo struct {
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
	RawProps    map[string][]string `json:"raw_props,omitempty"`
}

// AvailableWindow represents a VAVAILABLE block inside a VAVAILABILITY component.
type AvailableWindow struct {
	UID             *string             `json:"uid,omitempty"`
	Summary         *string             `json:"summary,omitempty"`
	Description     *string             `json:"description,omitempty"`
	DescriptionHTML *string             `json:"description_html,omitempty"`
	Location        *string             `json:"location,omitempty"`
	Categories      []string            `json:"categories,omitempty"`
	Contacts        []string            `json:"contacts,omitempty"`
	Start           *time.Time          `json:"dtstart,omitempty"`
	End             *time.Time          `json:"dtend,omitempty"`
	Duration        *string             `json:"duration,omitempty"`
	Recurrence      *RecurrenceInfo     `json:"recurrence,omitempty"`
	RawProps        map[string][]string `json:"raw_props,omitempty"`
}

// AvailabilityInfo captures a VAVAILABILITY component and its AVAILABLE entries.
type AvailabilityInfo struct {
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
	RawProps        map[string][]string `json:"raw_props,omitempty"`
}

type JournalInfo struct {
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
	RawProps        map[string][]string `json:"raw_props,omitempty"`
}
