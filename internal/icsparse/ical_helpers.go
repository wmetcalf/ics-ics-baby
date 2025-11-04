package icsparse

import (
	"strconv"
	"strings"
	"time"
)

func ensureRecurrence(ev *EventInfo) *RecurrenceInfo {
	if ev.Recurrence == nil {
		ev.Recurrence = &RecurrenceInfo{}
	}
	return ev.Recurrence
}

func ensureTodoRecurrence(todo *TodoInfo) *RecurrenceInfo {
	if todo.Recurrence == nil {
		todo.Recurrence = &RecurrenceInfo{}
	}
	return todo.Recurrence
}

func ensureAvailableRecurrence(win *AvailableWindow) *RecurrenceInfo {
	if win.Recurrence == nil {
		win.Recurrence = &RecurrenceInfo{}
	}
	return win.Recurrence
}

func ensureJournalRecurrence(journal *JournalInfo) *RecurrenceInfo {
	if journal.Recurrence == nil {
		journal.Recurrence = &RecurrenceInfo{}
	}
	return journal.Recurrence
}

func copyParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

func addRawProp(props map[string][]string, key, value string) map[string][]string {
	if value == "" {
		return props
	}
	if props == nil {
		props = make(map[string][]string)
	}
	props[key] = append(props[key], value)
	return props
}

func parseICSMultiDates(value string, params map[string]string, locDefault *time.Location) (times []time.Time, raw []string) {
	segments := splitEscaped(value, ',', false)
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if t, ok := parseICSTime(seg, params, locDefault); ok {
			times = append(times, t)
		} else {
			raw = append(raw, seg)
		}
	}
	return
}

func parseInt(value string) (*int, bool) {
	if value == "" {
		return nil, false
	}
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		return ptr(n), true
	}
	return nil, false
}

func parseGeo(value string) (*GeoPoint, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), ";", 2)
	if len(parts) != 2 {
		return nil, false
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, false
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, false
	}
	return &GeoPoint{Latitude: lat, Longitude: lon}, true
}

func parseImage(value string, params map[string]string) ImageInfo {
	enc := strings.ToUpper(params["ENCODING"])
	valType := strings.ToUpper(params["VALUE"])
	fmtType := strings.TrimSpace(trimQuotes(params["FMTTYPE"]))
	display := strings.TrimSpace(trimQuotes(params["DISPLAY"]))
	altrep := strings.TrimSpace(trimQuotes(params["ALTREP"]))
	info := ImageInfo{Source: "uri"}
	if enc == "BASE64" || valType == "BINARY" {
		clean := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", ""), " ", "")
		info.Source = "binary"
		info.Value = clean
	} else {
		decoded := strings.TrimSpace(unescapeICSText(value))
		info.Value = decoded
		if strings.HasPrefix(strings.ToLower(decoded), "data:") {
			info.Source = "data"
		}
	}
	info.FmtType = nz(fmtType)
	info.Display = nz(display)
	info.AltRep = nz(altrep)
	return info
}

func parseValarm(lines []string, start int, locDefault *time.Location) (AlarmInfo, int) {
	alarm := AlarmInfo{RawProps: map[string][]string{}}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "END:VALARM") {
			if len(alarm.RawProps) == 0 {
				alarm.RawProps = nil
			}
			return alarm, i
		}
		name, params, value := splitProp(line)
		up := strings.ToUpper(name)
		switch up {
		case "ACTION":
			if value != "" {
				alarm.Action = ptr(value)
			}
		case "TRIGGER":
			trig := AlarmTrigger{Raw: value}
			if rel := params["RELATED"]; rel != "" {
				trig.Related = ptr(rel)
			}
			if strings.EqualFold(params["VALUE"], "DATE-TIME") {
				if t, ok := parseICSTime(value, params, locDefault); ok {
					trig.Time = &t
				}
			} else if strings.HasPrefix(strings.TrimSpace(value), "P") || strings.HasPrefix(strings.TrimSpace(value), "+P") || strings.HasPrefix(strings.TrimSpace(value), "-P") {
				dur := strings.TrimSpace(value)
				trig.Duration = &dur
			}
			alarm.Trigger = &trig
		case "DESCRIPTION":
			if value != "" {
				alarm.Description = ptr(value)
			}
		case "SUMMARY":
			if value != "" {
				alarm.Summary = ptr(value)
			}
		case "DURATION":
			if value != "" {
				alarm.Duration = ptr(value)
			}
		case "REPEAT":
			if n, ok := parseInt(value); ok {
				alarm.Repeat = n
			}
		case "ATTACH":
			att := parseAttach(value, params)
			alarm.Attachments = append(alarm.Attachments, att)
		case "ATTENDEE":
			alarm.Attendees = append(alarm.Attendees, parseAttendee(value, params))
		default:
			alarm.RawProps = addRawProp(alarm.RawProps, up, value)
		}
	}
	if len(alarm.RawProps) == 0 {
		alarm.RawProps = nil
	}
	return alarm, len(lines) - 1
}

func parseVTimezone(lines []string, start int, locDefault *time.Location) (TimezoneInfo, int) {
	tz := TimezoneInfo{RawProps: map[string][]string{}}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "BEGIN:STANDARD"):
			period, endIdx := parseTimezonePeriod(lines, i, locDefault, "STANDARD")
			tz.Periods = append(tz.Periods, period)
			i = endIdx
			continue
		case strings.HasPrefix(upper, "BEGIN:DAYLIGHT"):
			period, endIdx := parseTimezonePeriod(lines, i, locDefault, "DAYLIGHT")
			tz.Periods = append(tz.Periods, period)
			i = endIdx
			continue
		case strings.HasPrefix(upper, "END:VTIMEZONE"):
			if len(tz.RawProps) == 0 {
				tz.RawProps = nil
			}
			return tz, i
		}

		name, params, value := splitProp(line)
		up := strings.ToUpper(name)
		switch up {
		case "TZID":
			if value != "" {
				tz.TZID = ptr(value)
			}
		case "LAST-MODIFIED":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				tz.LastModified = &t
			}
		case "TZURL":
			if value != "" {
				tz.URL = ptr(value)
			}
		default:
			tz.RawProps = addRawProp(tz.RawProps, up, value)
		}
	}
	if len(tz.RawProps) == 0 {
		tz.RawProps = nil
	}
	return tz, len(lines) - 1
}

func parseTimezonePeriod(lines []string, start int, locDefault *time.Location, typ string) (TimezonePeriod, int) {
	period := TimezonePeriod{Type: typ, RawProps: map[string][]string{}}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "END:"+typ) {
			if len(period.RawProps) == 0 {
				period.RawProps = nil
			}
			return period, i
		}
		name, params, value := splitProp(line)
		up := strings.ToUpper(name)
		switch up {
		case "DTSTART":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				period.DTStart = &t
			}
		case "TZOFFSETFROM":
			if value != "" {
				period.OffsetFrom = ptr(value)
			}
		case "TZOFFSETTO":
			if value != "" {
				period.OffsetTo = ptr(value)
			}
		case "TZNAME":
			if value != "" {
				period.Name = ptr(value)
			}
		case "RRULE":
			if value != "" {
				period.RRule = ptr(value)
			}
		default:
			period.RawProps = addRawProp(period.RawProps, up, value)
		}
	}
	if len(period.RawProps) == 0 {
		period.RawProps = nil
	}
	return period, len(lines) - 1
}

func parseAvailableWindow(lines []string, start int, locDefault *time.Location) (AvailableWindow, int) {
	window := AvailableWindow{RawProps: map[string][]string{}, Categories: []string{}, Contacts: []string{}}
	desc := descriptionCapture{}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "END:AVAILABLE") {
			if desc.hasPlain {
				window.Description = ptr(desc.plain)
			}
			if desc.hasHTML {
				window.DescriptionHTML = ptr(desc.html)
			}
			if len(window.RawProps) == 0 {
				window.RawProps = nil
			}
			return window, i
		}
		name, params, value := splitProp(line)
		up := strings.ToUpper(name)
		switch up {
		case "UID":
			if value != "" {
				val := strings.TrimSpace(unescapeICSText(value))
				if val != "" {
					window.UID = ptr(val)
				}
			}
		case "SUMMARY":
			if value != "" {
				val := strings.TrimSpace(unescapeICSText(value))
				if val != "" {
					window.Summary = ptr(val)
				}
			}
		case "DESCRIPTION":
			desc.absorbDescription(value)
		case "X-ALT-DESC":
			desc.absorbAltDescription(value, params)
		case "LOCATION":
			loc := strings.TrimSpace(unescapeICSText(value))
			if loc != "" {
				window.Location = ptr(loc)
			}
		case "CATEGORIES":
			parts := splitEscaped(value, ',', false)
			for _, p := range parts {
				p = strings.TrimSpace(unescapeICSText(p))
				if p != "" {
					window.Categories = append(window.Categories, p)
				}
			}
		case "CONTACT":
			contact := strings.TrimSpace(unescapeICSText(value))
			if contact != "" {
				window.Contacts = append(window.Contacts, contact)
			}
		case "DTSTART":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				window.Start = &t
			}
		case "DTEND":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				window.End = &t
			}
		case "DURATION":
			if value != "" {
				window.Duration = ptr(value)
			}
		case "RRULE":
			rec := ensureAvailableRecurrence(&window)
			rec.RRule = ptr(value)
		case "RDATE":
			rec := ensureAvailableRecurrence(&window)
			times, rawVals := parseICSMultiDates(value, params, locDefault)
			if len(times) > 0 {
				rec.RDates = append(rec.RDates, times...)
			}
			if len(rawVals) > 0 {
				rec.RDateRaw = append(rec.RDateRaw, rawVals...)
			}
		case "EXDATE":
			rec := ensureAvailableRecurrence(&window)
			times, rawVals := parseICSMultiDates(value, params, locDefault)
			if len(times) > 0 {
				rec.ExDates = append(rec.ExDates, times...)
			}
			if len(rawVals) > 0 {
				rec.ExDateRaw = append(rec.ExDateRaw, rawVals...)
			}
		case "RECURRENCE-ID":
			rec := ensureAvailableRecurrence(&window)
			if t, ok := parseICSTime(value, params, locDefault); ok {
				rec.RecurrenceID = &t
			} else if value != "" {
				rec.RecurrenceIDRaw = ptr(value)
			}
		default:
			window.RawProps = addRawProp(window.RawProps, up, value)
		}
	}
	if len(window.RawProps) == 0 {
		window.RawProps = nil
	}
	return window, len(lines) - 1
}

func parseAvailability(lines []string, start int, locDefault *time.Location) (AvailabilityInfo, int) {
	availability := AvailabilityInfo{RawProps: map[string][]string{}, Categories: []string{}, Contacts: []string{}, Available: []AvailableWindow{}}
	desc := descriptionCapture{}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "BEGIN:AVAILABLE"):
			window, endIdx := parseAvailableWindow(lines, i, locDefault)
			availability.Available = append(availability.Available, window)
			i = endIdx
			continue
		case strings.HasPrefix(upper, "END:VAVAILABILITY"):
			if desc.hasPlain {
				availability.Description = ptr(desc.plain)
			}
			if desc.hasHTML {
				availability.DescriptionHTML = ptr(desc.html)
			}
			if len(availability.RawProps) == 0 {
				availability.RawProps = nil
			}
			return availability, i
		}
		name, params, value := splitProp(line)
		up := strings.ToUpper(name)
		switch up {
		case "UID":
			if value != "" {
				val := strings.TrimSpace(unescapeICSText(value))
				if val != "" {
					availability.UID = ptr(val)
				}
			}
		case "SUMMARY":
			if value != "" {
				val := strings.TrimSpace(unescapeICSText(value))
				if val != "" {
					availability.Summary = ptr(val)
				}
			}
		case "DESCRIPTION":
			desc.absorbDescription(value)
		case "X-ALT-DESC":
			desc.absorbAltDescription(value, params)
		case "ORGANIZER":
			if value != "" {
				availability.Organizer = ptr(value)
			}
		case "BUSYTYPE":
			if value != "" {
				availability.BusyType = ptr(value)
			}
		case "CATEGORIES":
			parts := splitEscaped(value, ',', false)
			for _, p := range parts {
				p = strings.TrimSpace(unescapeICSText(p))
				if p != "" {
					availability.Categories = append(availability.Categories, p)
				}
			}
		case "CONTACT":
			contact := strings.TrimSpace(unescapeICSText(value))
			if contact != "" {
				availability.Contacts = append(availability.Contacts, contact)
			}
		case "URL":
			if value != "" {
				availability.URL = ptr(value)
			}
		case "LOCATION":
			loc := strings.TrimSpace(unescapeICSText(value))
			if loc != "" {
				availability.Location = ptr(loc)
			}
		case "PRIORITY":
			if n, ok := parseInt(value); ok {
				availability.Priority = n
			}
		case "SEQUENCE":
			if n, ok := parseInt(value); ok {
				availability.Sequence = n
			}
		case "CREATED":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				availability.Created = &t
			}
		case "LAST-MODIFIED":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				availability.LastModified = &t
			}
		case "DTSTAMP":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				availability.DateTimeStamp = &t
			}
		case "DTSTART":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				availability.Start = &t
			}
		case "DTEND":
			if t, ok := parseICSTime(value, params, locDefault); ok {
				availability.End = &t
			}
		case "DURATION":
			if value != "" {
				availability.Duration = ptr(value)
			}
		default:
			availability.RawProps = addRawProp(availability.RawProps, up, value)
		}
	}
	if len(availability.RawProps) == 0 {
		availability.RawProps = nil
	}
	return availability, len(lines) - 1
}

func parseFreeBusyPeriods(value string, params map[string]string, locDefault *time.Location) []FreeBusyPeriod {
	segments := strings.Split(value, ",")
	var out []FreeBusyPeriod
	typ := strings.TrimSpace(params["FBTYPE"])
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		parts := strings.Split(segment, "/")
		if len(parts) != 2 {
			continue
		}
		startStr := strings.TrimSpace(parts[0])
		endStr := strings.TrimSpace(parts[1])
		start, okStart := parseICSTime(startStr, params, locDefault)
		if !okStart {
			continue
		}
		end, okEnd := parseICSTime(endStr, params, locDefault)
		if !okEnd {
			continue
		}
		period := FreeBusyPeriod{Start: start, End: end}
		if typ != "" {
			t := typ
			period.Type = &t
		}
		out = append(out, period)
	}
	return out
}
