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

func parseValarm(lines []string, start int, locDefault *time.Location) (AlarmInfo, int) {
	alarm := AlarmInfo{RawProps: map[string]string{}}
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
			if value != "" {
				alarm.RawProps[up] = value
			}
		}
	}
	if len(alarm.RawProps) == 0 {
		alarm.RawProps = nil
	}
	return alarm, len(lines) - 1
}

func parseVTimezone(lines []string, start int, locDefault *time.Location) (TimezoneInfo, int) {
	tz := TimezoneInfo{RawProps: map[string]string{}}
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
			if value != "" {
				tz.RawProps[up] = value
			}
		}
	}
	if len(tz.RawProps) == 0 {
		tz.RawProps = nil
	}
	return tz, len(lines) - 1
}

func parseTimezonePeriod(lines []string, start int, locDefault *time.Location, typ string) (TimezonePeriod, int) {
	period := TimezonePeriod{Type: typ, RawProps: map[string]string{}}
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
			if value != "" {
				period.RawProps[up] = value
			}
		}
	}
	if len(period.RawProps) == 0 {
		period.RawProps = nil
	}
	return period, len(lines) - 1
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
