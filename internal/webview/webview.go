package webview

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ics-ics-baby/internal/icsparse"
)

func formatTimeValue(t *time.Time) string {
	if t == nil {
		return ""
	}
	if loc := t.Location(); loc != nil && loc != time.UTC {
		return t.Format("Monday, January 2, 2006 at 3:04 PM MST")
	}
	return t.Format("Monday, January 2, 2006 at 15:04 UTC")
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func formatIntPtr(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

func percentLabel(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d%%", *p)
}

func durationLabel(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func joinStrings(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, sep)
}

func formatDateLines(times []time.Time) []string {
	if len(times) == 0 {
		return nil
	}
	out := make([]string, 0, len(times))
	for _, t := range times {
		tt := t
		out = append(out, formatTimeValue(&tt))
	}
	return out
}

func alarmSummaries(alarms []icsparse.AlarmInfo) []string {
	if len(alarms) == 0 {
		return nil
	}
	out := make([]string, 0, len(alarms))
	for _, a := range alarms {
		parts := []string{}
		if a.Trigger != nil {
			if a.Trigger.Duration != nil && *a.Trigger.Duration != "" {
				parts = append(parts, "Trigger "+*a.Trigger.Duration)
			} else if a.Trigger.Time != nil {
				parts = append(parts, "Trigger "+formatTimeValue(a.Trigger.Time))
			}
			if a.Trigger.Related != nil && *a.Trigger.Related != "" {
				parts = append(parts, "Related="+*a.Trigger.Related)
			}
		}
		if a.Action != nil && *a.Action != "" {
			parts = append(parts, "Action "+*a.Action)
		}
		if a.Summary != nil && *a.Summary != "" {
			parts = append(parts, "Summary "+*a.Summary)
		}
		if a.Description != nil && *a.Description != "" {
			parts = append(parts, "Note "+*a.Description)
		}
		if a.Repeat != nil {
			parts = append(parts, fmt.Sprintf("Repeat %d", *a.Repeat))
		}
		if a.Duration != nil && *a.Duration != "" {
			parts = append(parts, "Interval "+*a.Duration)
		}
		if len(parts) == 0 {
			parts = append(parts, "Alarm configured")
		}
		out = append(out, strings.Join(parts, "; "))
	}
	return out
}

func conferenceSummaries(confs []icsparse.ConferenceInfo) []string {
	if len(confs) == 0 {
		return nil
	}
	out := make([]string, 0, len(confs))
	for _, c := range confs {
		line := strings.TrimSpace(c.URI)
		if len(c.Params) > 0 {
			keys := make([]string, 0, len(c.Params))
			for k := range c.Params {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%s", k, c.Params[k]))
			}
			params := strings.Join(parts, ", ")
			if line == "" {
				line = params
			} else {
				line = line + " (" + params + ")"
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func freeBusySummaries(periods []icsparse.FreeBusyPeriod) []string {
	if len(periods) == 0 {
		return nil
	}
	out := make([]string, 0, len(periods))
	for _, p := range periods {
		start := p.Start
		end := p.End
		text := fmt.Sprintf("%s → %s", formatTimeValue(&start), formatTimeValue(&end))
		if p.Type != nil && *p.Type != "" {
			text += " (" + *p.Type + ")"
		}
		out = append(out, text)
	}
	return out
}

func attendeeSummary(a icsparse.Attendee) string {
	name := strings.TrimSpace(a.Mailto)
	if a.CN != nil && strings.TrimSpace(*a.CN) != "" {
		label := strings.TrimSpace(*a.CN)
		if a.Mailto != "" {
			name = fmt.Sprintf("%s <%s>", label, a.Mailto)
		} else {
			name = label
		}
	} else if a.Mailto != "" {
		name = a.Mailto
	} else if name == "" {
		name = "Unknown attendee"
	}
	if a.PartStat != nil && *a.PartStat != "" {
		name += " (" + *a.PartStat + ")"
	}
	if a.RSVP != nil && *a.RSVP != "" {
		name += " [RSVP " + *a.RSVP + "]"
	}
	return name
}

var tmpl = template.Must(template.New("invite").Funcs(template.FuncMap{
	"fmtTime": formatTimeValue,
	"cleanText": func(s string) string {
		s = strings.ReplaceAll(s, "\\n", "\n")
		s = strings.ReplaceAll(s, "\\,", ",")
		s = strings.ReplaceAll(s, "\\;", ";")
		s = strings.ReplaceAll(s, "\\\\", "\\")
		return s
	},
	"safeHTML": func(p *string) template.HTML {
		if p == nil {
			return ""
		}
		return template.HTML(*p)
	},
	"str":                 derefString,
	"formatInt":           formatIntPtr,
	"formatPercent":       percentLabel,
	"formatDuration":      durationLabel,
	"join":                joinStrings,
	"fmtDateLines":        formatDateLines,
	"alarmSummaries":      alarmSummaries,
	"conferenceSummaries": conferenceSummaries,
	"freeBusySummaries":   freeBusySummaries,
	"attendeeSummary":     attendeeSummary,
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ if .Name }}{{ .Name }}{{ else }}Calendar Preview{{ end }}</title>
<style>
:root {
  --bg: {{if eq .Style "dark"}}#2b2b2b{{else}}#f5f5f5{{end}};
  --panel: {{if eq .Style "dark"}}#3a3a3a{{else}}#ffffff{{end}};
  --text: {{if eq .Style "dark"}}#ffffff{{else}}#1a1a1a{{end}};
  --muted: {{if eq .Style "dark"}}#b0b0b0{{else}}#666666{{end}};
  --edge: {{if eq .Style "dark"}}#555555{{else}}#cccccc{{end}};
  --accent: #ff6600;
  --desc-bg: {{if eq .Style "dark"}}#3a3a3a{{else}}#ffffff{{end}};
}
html,body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif;
}
.wrap {
  max-width: 1000px;
  margin: 24px auto;
  padding: 0 16px 40px;
}
.event {
  margin-bottom: 30px;
}
.header {
  background: var(--panel);
  border-radius: 8px;
  padding: 15px 20px;
  margin-bottom: 20px;
  display: flex;
  align-items: center;
  gap: 15px;
}
.icon {
  width: 20px;
  height: 20px;
  background: var(--accent);
  flex-shrink: 0;
}
.title {
  font-size: 16px;
  font-weight: 600;
  line-height: 1.3;
}
.fields {
  margin-bottom: 20px;
}
.field {
  display: flex;
  margin-bottom: 8px;
  font-size: 13px;
  line-height: 1.6;
}
.field-label {
  color: var(--muted);
  width: 100px;
  flex-shrink: 0;
  padding-left: 10px;
}
.field-value {
  color: var(--text);
  flex: 1;
  word-wrap: break-word;
  overflow-wrap: break-word;
}
.desc-box {
  background: var(--desc-bg);
  border: 1.5px solid var(--edge);
  border-radius: 6px;
  padding: 15px 20px;
  line-height: 1.6;
  font-size: 13px;
  word-wrap: break-word;
  overflow-wrap: break-word;
}
.desc-box.desc-text {
  white-space: pre-wrap;
}
.desc-box.desc-html {
  white-space: normal;
}
.desc-box.desc-html p {
  margin: 0 0 12px;
}
.desc-box.desc-html ul,
.desc-box.desc-html ol {
  margin: 0 0 12px 20px;
  padding-left: 18px;
}
.desc-box.desc-html li {
  margin: 4px 0;
}
.desc-box.desc-html table {
  width: 100%;
  margin: 12px 0;
  border-collapse: collapse;
}
.desc-box.desc-html th,
.desc-box.desc-html td {
  border: 1px solid var(--edge);
  padding: 6px 8px;
  vertical-align: top;
}
.desc-box.desc-html a {
  color: var(--accent);
  text-decoration: none;
}
.desc-box.desc-html a:hover {
  text-decoration: underline;
}
.field-value a {
  color: var(--accent);
  text-decoration: none;
}
.field-value a:hover {
  text-decoration: underline;
}
.calendar-info {
  background: var(--panel);
  border-radius: 10px;
  padding: 18px 20px 12px;
  margin-bottom: 28px;
  font-size: 13px;
  line-height: 1.6;
}
.calendar-info strong {
  display: inline-block;
  min-width: 120px;
  color: var(--muted);
  font-weight: 500;
}
.section-title {
  font-size: 18px;
  font-weight: 600;
  margin: 32px 0 16px;
}
</style>
</head>
<body>
<div class="wrap">
  {{ if or .Description .URL .Calscale .TimezoneID .Timezones }}
  <div class="calendar-info">
    {{ if .Description }}<div><strong>Description:</strong> {{ cleanText (str .Description) }}</div>{{ end }}
    {{ if .URL }}<div><strong>URL:</strong> <a href="{{ str .URL }}">{{ str .URL }}</a></div>{{ end }}
    {{ if .Calscale }}<div><strong>Calendar Scale:</strong> {{ str .Calscale }}</div>{{ end }}
    {{ if .TimezoneID }}<div><strong>Default TZ:</strong> {{ str .TimezoneID }}</div>{{ end }}
    {{ if .Timezones }}
      <div><strong>Timezones:</strong>
        {{ if .Timezones }}<br>{{ end }}
        {{ range .Timezones }}
          {{ if .TZID }}• {{ str .TZID }}<br>{{ end }}
        {{ end }}
      </div>
    {{ end }}
  </div>
  {{ end }}

  {{ if .Events }}
    {{ range .Events }}
    <div class="event">
      <div class="header">
        <div class="icon"></div>
        <div class="title">{{ .Summary }}</div>
      </div>

      <div class="fields">
        {{ if .DTStart }}
        <div class="field">
          <div class="field-label">Start Date:</div>
          <div class="field-value">{{ fmtTime .DTStart }}</div>
        </div>
        {{ end }}
        {{ if .DTEnd }}
        <div class="field">
          <div class="field-label">End Date:</div>
          <div class="field-value">{{ fmtTime .DTEnd }}</div>
        </div>
        {{ end }}
        {{ if .Location }}
        <div class="field">
          <div class="field-label">Location:</div>
          <div class="field-value">{{ cleanText (str .Location) }}</div>
        </div>
        {{ end }}
        {{ if .Organizer }}
        <div class="field">
          <div class="field-label">Organizer:</div>
          <div class="field-value">{{ cleanText (str .Organizer) }}</div>
        </div>
        {{ end }}
        {{ if .Status }}
        <div class="field">
          <div class="field-label">Status:</div>
          <div class="field-value">{{ str .Status }}</div>
        </div>
        {{ end }}
        {{ if .Transparency }}
        <div class="field">
          <div class="field-label">Transparency:</div>
          <div class="field-value">{{ str .Transparency }}</div>
        </div>
        {{ end }}
        {{ $priority := formatInt .Priority }}
        {{ if $priority }}
        <div class="field">
          <div class="field-label">Priority:</div>
          <div class="field-value">{{ $priority }}</div>
        </div>
        {{ end }}
        {{ if .Class }}
        <div class="field">
          <div class="field-label">Class:</div>
          <div class="field-value">{{ str .Class }}</div>
        </div>
        {{ end }}
        {{ $sequence := formatInt .Sequence }}
        {{ if $sequence }}
        <div class="field">
          <div class="field-label">Sequence:</div>
          <div class="field-value">{{ $sequence }}</div>
        </div>
        {{ end }}
        {{ $duration := str .Duration }}
        {{ if $duration }}
        <div class="field">
          <div class="field-label">Duration:</div>
          <div class="field-value">{{ $duration }}</div>
        </div>
        {{ end }}
        {{ if .URL }}
        <div class="field">
          <div class="field-label">URL:</div>
          <div class="field-value"><a href="{{ str .URL }}">{{ str .URL }}</a></div>
        </div>
        {{ end }}
        {{ $discovered := join .DiscoveredURLs ", " }}
        {{ if $discovered }}
        <div class="field">
          <div class="field-label">Referenced URLs:</div>
          <div class="field-value">{{ $discovered }}</div>
        </div>
        {{ end }}
        {{ $cats := join .Categories ", " }}
        {{ if $cats }}
        <div class="field">
          <div class="field-label">Categories:</div>
          <div class="field-value">{{ $cats }}</div>
        </div>
        {{ end }}
        {{ $resources := join .Resources ", " }}
        {{ if $resources }}
        <div class="field">
          <div class="field-label">Resources:</div>
          <div class="field-value">{{ $resources }}</div>
        </div>
        {{ end }}
        {{ $confs := conferenceSummaries .Conferences }}
        {{ if $confs }}
        <div class="field">
          <div class="field-label">Conference:</div>
          <div class="field-value">{{ range $confs }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ with .Recurrence }}
          {{ if .RRule }}
          <div class="field">
            <div class="field-label">RRULE:</div>
            <div class="field-value">{{ str .RRule }}</div>
          </div>
          {{ end }}
          {{ $rdates := fmtDateLines .RDates }}
          {{ if $rdates }}
          <div class="field">
            <div class="field-label">RDATE:</div>
            <div class="field-value">{{ range $rdates }}• {{ . }}<br>{{ end }}</div>
          </div>
          {{ end }}
          {{ $rdateRaw := join .RDateRaw ", " }}
          {{ if $rdateRaw }}
          <div class="field">
            <div class="field-label">RDATE Raw:</div>
            <div class="field-value">{{ $rdateRaw }}</div>
          </div>
          {{ end }}
          {{ $exdates := fmtDateLines .ExDates }}
          {{ if $exdates }}
          <div class="field">
            <div class="field-label">EXDATE:</div>
            <div class="field-value">{{ range $exdates }}• {{ . }}<br>{{ end }}</div>
          </div>
          {{ end }}
          {{ $exdateRaw := join .ExDateRaw ", " }}
          {{ if $exdateRaw }}
          <div class="field">
            <div class="field-label">EXDATE Raw:</div>
            <div class="field-value">{{ $exdateRaw }}</div>
          </div>
          {{ end }}
          {{ if .RecurrenceID }}
          <div class="field">
            <div class="field-label">Recurrence ID:</div>
            <div class="field-value">{{ fmtTime .RecurrenceID }}</div>
          </div>
          {{ else if .RecurrenceIDRaw }}
          <div class="field">
            <div class="field-label">Recurrence ID:</div>
            <div class="field-value">{{ str .RecurrenceIDRaw }}</div>
          </div>
          {{ end }}
          {{ if .Duration }}
          <div class="field">
            <div class="field-label">Recurrence Duration:</div>
            <div class="field-value">{{ str .Duration }}</div>
          </div>
          {{ end }}
        {{ end }}
        {{ if .Created }}
        <div class="field">
          <div class="field-label">Created:</div>
          <div class="field-value">{{ fmtTime .Created }}</div>
        </div>
        {{ end }}
        {{ if .LastModified }}
        <div class="field">
          <div class="field-label">Last Modified:</div>
          <div class="field-value">{{ fmtTime .LastModified }}</div>
        </div>
        {{ end }}
        {{ if .DateTimeStamp }}
        <div class="field">
          <div class="field-label">Timestamp:</div>
          <div class="field-value">{{ fmtTime .DateTimeStamp }}</div>
        </div>
        {{ end }}
        {{ if .Attendees }}
        <div class="field">
          <div class="field-label">Attendees:</div>
          <div class="field-value">
            {{ range .Attendees }}• {{ attendeeSummary . }}<br>{{ end }}
          </div>
        </div>
        {{ end }}
        {{ if .Attachments }}
        <div class="field">
          <div class="field-label">Attachments:</div>
          <div class="field-value">
            {{ range .Attachments }}• {{ if .SavedAs }}{{ .SavedAs }}{{ if .Mime }} ({{ .Mime }}){{ end }}{{ else if eq .Source "url" }}Link{{ if .Fmt }} ({{ .Fmt }}){{ end }}{{ else }}Inline{{ if .Fmt }} ({{ .Fmt }}){{ end }}{{ end }}<br>{{ end }}
          </div>
        </div>
        {{ end }}
        {{ $alarms := alarmSummaries .Alarms }}
        {{ if $alarms }}
        <div class="field">
          <div class="field-label">Alarms:</div>
          <div class="field-value">{{ range $alarms }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
      </div>

      {{ if .DescriptionHTML }}
      <div class="desc-box desc-html">{{ safeHTML .DescriptionHTML }}</div>
      {{ else if .Description }}
      <div class="desc-box desc-text">{{ cleanText (str .Description) }}</div>
      {{ end }}
    </div>
    {{ end }}
  {{ else }}
    <div class="event">
      <div class="header">
        <div class="icon"></div>
        <div class="title">No events to display</div>
      </div>
    </div>
  {{ end }}

  {{ if .Todos }}
    <h2 class="section-title">Tasks</h2>
    {{ range .Todos }}
    <div class="event">
      <div class="header">
        <div class="icon"></div>
        <div class="title">{{ .Summary }}</div>
      </div>

      <div class="fields">
        {{ if .Start }}
        <div class="field">
          <div class="field-label">Start Date:</div>
          <div class="field-value">{{ fmtTime .Start }}</div>
        </div>
        {{ end }}
        {{ if .Due }}
        <div class="field">
          <div class="field-label">Due Date:</div>
          <div class="field-value">{{ fmtTime .Due }}</div>
        </div>
        {{ end }}
        {{ if .Completed }}
        <div class="field">
          <div class="field-label">Completed:</div>
          <div class="field-value">{{ fmtTime .Completed }}</div>
        </div>
        {{ end }}
        {{ if .Status }}
        <div class="field">
          <div class="field-label">Status:</div>
          <div class="field-value">{{ str .Status }}</div>
        </div>
        {{ end }}
        {{ $prio := formatInt .Priority }}
        {{ if $prio }}
        <div class="field">
          <div class="field-label">Priority:</div>
          <div class="field-value">{{ $prio }}</div>
        </div>
        {{ end }}
        {{ $pct := formatPercent .PercentComplete }}
        {{ if $pct }}
        <div class="field">
          <div class="field-label">Progress:</div>
          <div class="field-value">{{ $pct }}</div>
        </div>
        {{ end }}
        {{ if .Organizer }}
        <div class="field">
          <div class="field-label">Organizer:</div>
          <div class="field-value">{{ cleanText (str .Organizer) }}</div>
        </div>
        {{ end }}
        {{ if .URL }}
        <div class="field">
          <div class="field-label">URL:</div>
          <div class="field-value"><a href="{{ str .URL }}">{{ str .URL }}</a></div>
        </div>
        {{ end }}
        {{ $cats := join .Categories ", " }}
        {{ if $cats }}
        <div class="field">
          <div class="field-label">Categories:</div>
          <div class="field-value">{{ $cats }}</div>
        </div>
        {{ end }}
        {{ $res := join .Resources ", " }}
        {{ if $res }}
        <div class="field">
          <div class="field-label">Resources:</div>
          <div class="field-value">{{ $res }}</div>
        </div>
        {{ end }}
        {{ $seq := formatInt .Sequence }}
        {{ if $seq }}
        <div class="field">
          <div class="field-label">Sequence:</div>
          <div class="field-value">{{ $seq }}</div>
        </div>
        {{ end }}
        {{ $dur := str .Duration }}
        {{ if $dur }}
        <div class="field">
          <div class="field-label">Duration:</div>
          <div class="field-value">{{ $dur }}</div>
        </div>
        {{ end }}
        {{ if .Created }}
        <div class="field">
          <div class="field-label">Created:</div>
          <div class="field-value">{{ fmtTime .Created }}</div>
        </div>
        {{ end }}
        {{ if .LastModified }}
        <div class="field">
          <div class="field-label">Last Modified:</div>
          <div class="field-value">{{ fmtTime .LastModified }}</div>
        </div>
        {{ end }}
        {{ if .DateTimeStamp }}
        <div class="field">
          <div class="field-label">Timestamp:</div>
          <div class="field-value">{{ fmtTime .DateTimeStamp }}</div>
        </div>
        {{ end }}
        {{ with .Recurrence }}
          {{ if .RRule }}
          <div class="field">
            <div class="field-label">RRULE:</div>
            <div class="field-value">{{ str .RRule }}</div>
          </div>
          {{ end }}
          {{ $rdates := fmtDateLines .RDates }}
          {{ if $rdates }}
          <div class="field">
            <div class="field-label">RDATE:</div>
            <div class="field-value">{{ range $rdates }}• {{ . }}<br>{{ end }}</div>
          </div>
          {{ end }}
          {{ $exdates := fmtDateLines .ExDates }}
          {{ if $exdates }}
          <div class="field">
            <div class="field-label">EXDATE:</div>
            <div class="field-value">{{ range $exdates }}• {{ . }}<br>{{ end }}</div>
          </div>
          {{ end }}
          {{ if .Duration }}
          <div class="field">
            <div class="field-label">Recurrence Duration:</div>
            <div class="field-value">{{ str .Duration }}</div>
          </div>
          {{ end }}
        {{ end }}
        {{ if .Attendees }}
        <div class="field">
          <div class="field-label">Attendees:</div>
          <div class="field-value">{{ range .Attendees }}• {{ attendeeSummary . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Attachments }}
        <div class="field">
          <div class="field-label">Attachments:</div>
          <div class="field-value">{{ range .Attachments }}• {{ if .SavedAs }}{{ .SavedAs }}{{ if .Mime }} ({{ .Mime }}){{ end }}{{ else if eq .Source "url" }}Link{{ if .Fmt }} ({{ .Fmt }}){{ end }}{{ else }}Inline{{ if .Fmt }} ({{ .Fmt }}){{ end }}{{ end }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ $alarms := alarmSummaries .Alarms }}
        {{ if $alarms }}
        <div class="field">
          <div class="field-label">Alarms:</div>
          <div class="field-value">{{ range $alarms }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ $urls := join .DiscoveredURLs ", " }}
        {{ if $urls }}
        <div class="field">
          <div class="field-label">URLs:</div>
          <div class="field-value">{{ $urls }}</div>
        </div>
        {{ end }}
      </div>

      {{ if .DescriptionHTML }}
      <div class="desc-box desc-html">{{ safeHTML .DescriptionHTML }}</div>
      {{ else if .Description }}
      <div class="desc-box desc-text">{{ cleanText (str .Description) }}</div>
      {{ end }}
    </div>
    {{ end }}
  {{ end }}

  {{ if .FreeBusy }}
    <h2 class="section-title">Availability</h2>
    {{ range .FreeBusy }}
    <div class="event">
      <div class="header">
        <div class="icon"></div>
        <div class="title">Free/Busy Window</div>
      </div>
      <div class="fields">
        {{ if .Start }}
        <div class="field">
          <div class="field-label">Start:</div>
          <div class="field-value">{{ fmtTime .Start }}</div>
        </div>
        {{ end }}
        {{ if .End }}
        <div class="field">
          <div class="field-label">End:</div>
          <div class="field-value">{{ fmtTime .End }}</div>
        </div>
        {{ end }}
        {{ if .Organizer }}
        <div class="field">
          <div class="field-label">Organizer:</div>
          <div class="field-value">{{ cleanText (str .Organizer) }}</div>
        </div>
        {{ end }}
        {{ if .Contact }}
        <div class="field">
          <div class="field-label">Contact:</div>
          <div class="field-value">{{ cleanText (str .Contact) }}</div>
        </div>
        {{ end }}
        {{ if .URL }}
        <div class="field">
          <div class="field-label">URL:</div>
          <div class="field-value"><a href="{{ str .URL }}">{{ str .URL }}</a></div>
        </div>
        {{ end }}
        {{ $comments := .Comments }}
        {{ if $comments }}
        <div class="field">
          <div class="field-label">Comments:</div>
          <div class="field-value">{{ range $comments }}• {{ cleanText . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ $periods := freeBusySummaries .Periods }}
        {{ if $periods }}
        <div class="field">
          <div class="field-label">Busy:</div>
          <div class="field-value">{{ range $periods }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Attendees }}
        <div class="field">
          <div class="field-label">Attendees:</div>
          <div class="field-value">{{ range .Attendees }}• {{ attendeeSummary . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Attachments }}
        <div class="field">
          <div class="field-label">Attachments:</div>
          <div class="field-value">{{ range .Attachments }}• {{ if .SavedAs }}{{ .SavedAs }}{{ if .Mime }} ({{ .Mime }}){{ end }}{{ else if eq .Source "url" }}Link{{ if .Fmt }} ({{ .Fmt }}){{ end }}{{ else }}Inline{{ if .Fmt }} ({{ .Fmt }}){{ end }}{{ end }}<br>{{ end }}</div>
        </div>
        {{ end }}
      </div>
    </div>
    {{ end }}
  {{ end }}
</div>
</body>
</html>`))

type viewData struct {
	Name        *string
	ProdID      *string
	Method      *string
	Description *string
	URL         *string
	Calscale    *string
	TimezoneID  *string
	Timezones   []icsparse.TimezoneInfo
	Events      []icsparse.EventInfo
	Todos       []icsparse.TodoInfo
	FreeBusy    []icsparse.FreeBusyInfo
	Style       string
}

func WriteInviteHTML(cal *icsparse.CalendarInfo, outPath string, style string) error {
	data := viewData{
		Name:        cal.Name,
		ProdID:      cal.ProdID,
		Method:      cal.Method,
		Description: cal.Description,
		URL:         cal.URL,
		Calscale:    cal.Calscale,
		TimezoneID:  cal.TimezoneID,
		Timezones:   cal.Timezones,
		Events:      cal.Events,
		Todos:       cal.Todos,
		FreeBusy:    cal.FreeBusy,
		Style:       style,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}
