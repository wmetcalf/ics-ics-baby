package webview

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

var hexColorPattern = regexp.MustCompile(`^#(?i:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)

func sanitizeColorValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "#") {
		trimmed = "#" + trimmed
	}
	if hexColorPattern.MatchString(trimmed) {
		return strings.ToLower(trimmed)
	}
	return ""
}

func sanitizeColorPtr(p *string) string {
	if p == nil {
		return ""
	}
	return sanitizeColorValue(*p)
}

func eventIconStyle(colorPtr *string, accent string) template.CSS {
	if sanitized := sanitizeColorPtr(colorPtr); sanitized != "" {
		return template.CSS("background:" + sanitized)
	}
	if accent != "" {
		return template.CSS("background:" + accent)
	}
	return ""
}

func geoLabel(geo *icsparse.GeoPoint) string {
	if geo == nil {
		return ""
	}
	return fmt.Sprintf("%.4f, %.4f", geo.Latitude, geo.Longitude)
}

func geoURL(geo *icsparse.GeoPoint) string {
	if geo == nil {
		return ""
	}
	return fmt.Sprintf("https://maps.google.com/?q=%f,%f", geo.Latitude, geo.Longitude)
}

func imageSummary(img icsparse.ImageInfo) string {
	parts := []string{}
	source := strings.ToLower(strings.TrimSpace(img.Source))
	if source == "" {
		source = "unknown"
	}
	parts = append(parts, "source="+source)
	if img.FmtType != nil && *img.FmtType != "" {
		parts = append(parts, "type="+*img.FmtType)
	}
	if img.Display != nil && *img.Display != "" {
		parts = append(parts, "display="+*img.Display)
	}
	if img.AltRep != nil && *img.AltRep != "" {
		parts = append(parts, "altrep="+*img.AltRep)
	}
	val := strings.TrimSpace(img.Value)
	if val != "" {
		if len(val) > 120 {
			val = val[:117] + "..."
		}
		parts = append(parts, "value="+val)
	}
	return strings.Join(parts, "; ")
}

func imageHref(img icsparse.ImageInfo) string {
	val := strings.TrimSpace(img.Value)
	if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") || strings.HasPrefix(val, "data:") {
		return val
	}
	return ""
}

func bulletStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, v := range items {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, "• "+v)
	}
	return out
}

func availableWindowSummary(win icsparse.AvailableWindow) string {
	parts := []string{}
	if win.Start != nil || win.End != nil {
		start := formatTimeValue(win.Start)
		end := formatTimeValue(win.End)
		if start != "" || end != "" {
			if start != "" && end != "" {
				parts = append(parts, fmt.Sprintf("%s → %s", start, end))
			} else if start != "" {
				parts = append(parts, "Starts "+start)
			} else {
				parts = append(parts, "Ends "+end)
			}
		}
	}
	if win.Duration != nil && *win.Duration != "" {
		parts = append(parts, "Duration "+*win.Duration)
	}
	if win.Recurrence != nil {
		rec := win.Recurrence
		if rec.RRule != nil && *rec.RRule != "" {
			parts = append(parts, "RRULE="+*rec.RRule)
		}
		if len(rec.RDates) > 0 {
			parts = append(parts, fmt.Sprintf("RDATE count=%d", len(rec.RDates)))
		}
		if len(rec.ExDates) > 0 {
			parts = append(parts, fmt.Sprintf("EXDATE count=%d", len(rec.ExDates)))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "Available window")
	}
	return strings.Join(parts, " | ")
}

func availabilityWindowDetails(win icsparse.AvailableWindow) []string {
	lines := []string{}
	if win.Location != nil && strings.TrimSpace(*win.Location) != "" {
		lines = append(lines, "• Location: "+strings.TrimSpace(*win.Location))
	}
	if len(win.Contacts) > 0 {
		lines = append(lines, bulletStrings(prefixAll("Contact: ", win.Contacts))...)
	}
	if len(win.Categories) > 0 {
		lines = append(lines, "• Categories: "+strings.Join(win.Categories, ", "))
	}
	return lines
}

func prefixAll(prefix string, items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, prefix+item)
	}
	return out
}

func conferenceLabel(conf icsparse.ConferenceInfo) string {
	if label := strings.TrimSpace(conf.Params["LABEL"]); label != "" {
		return label
	}
	if feature := strings.TrimSpace(conf.Params["FEATURE"]); feature != "" {
		parts := strings.Split(feature, ",")
		if len(parts) > 0 {
			name := strings.ToLower(strings.TrimSpace(parts[0]))
			if name != "" {
				return "Join " + strings.Title(name)
			}
		}
	}
	if conf.URI != "" {
		if u, err := url.Parse(conf.URI); err == nil && u.Host != "" {
			return "Open " + u.Host
		}
	}
	return "Join Conference"
}

func attachmentLabel(att icsparse.AttachmentInfo) string {
	if att.SavedAs != nil && *att.SavedAs != "" {
		return filepath.Base(*att.SavedAs)
	}
	if att.Source == "url" {
		if att.Value != "" {
			return truncateForDisplay(att.Value, 60)
		}
		return "Link"
	}
	if att.Fmt != nil && *att.Fmt != "" {
		return "Inline (" + *att.Fmt + ")"
	}
	return "Inline attachment"
}

func attachmentHref(att icsparse.AttachmentInfo) string {
	if att.Source == "url" && att.Value != "" {
		return att.Value
	}
	if strings.HasPrefix(att.Value, "http://") || strings.HasPrefix(att.Value, "https://") || strings.HasPrefix(att.Value, "data:") {
		return att.Value
	}
	return ""
}

func truncateForDisplay(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
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
		out = append(out, "🔔 "+strings.Join(parts, "; "))
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
	extra := make([]string, 0, 6)
	if a.Role != nil && *a.Role != "" {
		extra = append(extra, "role="+*a.Role)
	}
	if a.Cutype != nil && *a.Cutype != "" {
		extra = append(extra, "type="+*a.Cutype)
	}
	if a.SentBy != nil && *a.SentBy != "" {
		extra = append(extra, "sent-by="+*a.SentBy)
	}
	if len(a.DelegatedFrom) > 0 {
		extra = append(extra, "delegated-from="+strings.Join(a.DelegatedFrom, ", "))
	}
	if len(a.DelegatedTo) > 0 {
		extra = append(extra, "delegated-to="+strings.Join(a.DelegatedTo, ", "))
	}
	if len(a.Member) > 0 {
		extra = append(extra, "member="+strings.Join(a.Member, ", "))
	}
	if a.Directory != nil && *a.Directory != "" {
		extra = append(extra, "dir="+*a.Directory)
	}
	if a.Language != nil && *a.Language != "" {
		extra = append(extra, "lang="+*a.Language)
	}
	if len(extra) > 0 {
		name += " [" + strings.Join(extra, "; ") + "]"
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
	"str":                       derefString,
	"formatInt":                 formatIntPtr,
	"formatPercent":             percentLabel,
	"formatDuration":            durationLabel,
	"join":                      joinStrings,
	"fmtDateLines":              formatDateLines,
	"alarmSummaries":            alarmSummaries,
	"conferenceSummaries":       conferenceSummaries,
	"freeBusySummaries":         freeBusySummaries,
	"attendeeSummary":           attendeeSummary,
	"eventIconStyle":            eventIconStyle,
	"imageSummary":              imageSummary,
	"imageHref":                 imageHref,
	"geoLabel":                  geoLabel,
	"geoURL":                    geoURL,
	"availableWindowSummary":    availableWindowSummary,
	"availabilityWindowDetails": availabilityWindowDetails,
	"sanitizeColor":             sanitizeColorValue,
	"conferenceLabel":           conferenceLabel,
	"attachmentLabel":           attachmentLabel,
	"attachmentHref":            attachmentHref,
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
  --accent: {{ .AccentColor }};
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
.conference-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.conference-btn {
  display: inline-block;
  padding: 6px 12px;
  background: var(--accent);
  color: #fff;
  border-radius: 6px;
  font-size: 12px;
  text-decoration: none;
  font-weight: 600;
}
.conference-btn:hover {
  opacity: 0.85;
}
.attachment-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.attachment-item {
  display: flex;
  align-items: center;
  gap: 10px;
}
.attachment-btn {
  display: inline-block;
  padding: 4px 10px;
  background: var(--edge);
  border-radius: 5px;
  font-size: 11px;
  text-decoration: none;
  color: var(--text);
}
.attachment-btn:hover {
  background: var(--accent);
  color: #fff;
}
.progress-track {
  margin-top: 6px;
  width: 120px;
  height: 8px;
  border-radius: 4px;
  background: var(--edge);
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: var(--accent);
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
.color-chip {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 1px solid var(--edge);
  margin-right: 6px;
  vertical-align: middle;
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
  {{ if or .Description .URL .Calscale .TimezoneID .Timezones .CalCategories .CalContacts .CalImages .RefreshInterval .Source .CalendarColor }}
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
    {{ if .CalendarColor }}
      <div><strong>Color:</strong> <span class="color-chip" style="background: {{ .CalendarColor }}"></span>{{ .CalendarColor }}</div>
    {{ end }}
    {{ if .Source }}
      <div><strong>Source:</strong> <a href="{{ str .Source }}">{{ str .Source }}</a></div>
    {{ end }}
    {{ if .RefreshInterval }}
      <div><strong>Refresh Interval:</strong> {{ str .RefreshInterval }}</div>
    {{ end }}
    {{ if .CalContacts }}
      <div><strong>Contacts:</strong><br>
        {{ range .CalContacts }}• {{ . }}<br>{{ end }}
      </div>
    {{ end }}
    {{ if .CalCategories }}
      <div><strong>Categories:</strong> {{ join .CalCategories ", " }}</div>
    {{ end }}
    {{ if .CalImages }}
      <div><strong>Images:</strong><br>
        {{ range .CalImages }}
          {{ $href := imageHref . }}• {{ imageSummary . }}{{ if $href }} — <a href="{{ $href }}">Open</a>{{ end }}<br>
        {{ end }}
      </div>
    {{ end }}
  </div>
  {{ end }}

  {{ if .Events }}
    {{ range .Events }}
    <div class="event">
      <div class="header">
        <div class="icon" style="{{ eventIconStyle .Color $.AccentColor }}"></div>
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
        {{ if .Geo }}
        <div class="field">
          <div class="field-label">Coordinates:</div>
          <div class="field-value">{{ geoLabel .Geo }}{{ $geo := geoURL .Geo }}{{ if $geo }}<br><a href="{{ $geo }}">Open map</a>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Organizer }}
        <div class="field">
          <div class="field-label">Organizer:</div>
          <div class="field-value">{{ cleanText (str .Organizer) }}</div>
        </div>
        {{ end }}
        {{ if .Contacts }}
        <div class="field">
          <div class="field-label">Contacts:</div>
          <div class="field-value">{{ range .Contacts }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ $color := sanitizeColor (str .Color) }}
        {{ if $color }}
        <div class="field">
          <div class="field-label">Color:</div>
          <div class="field-value"><span class="color-chip" style="background: {{ $color }}"></span>{{ $color }}</div>
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
        {{ if .Comments }}
        <div class="field">
          <div class="field-label">Comments:</div>
          <div class="field-value">{{ range .Comments }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .RelatedTo }}
        <div class="field">
          <div class="field-label">Related To:</div>
          <div class="field-value">{{ range .RelatedTo }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .RequestStatuses }}
        <div class="field">
          <div class="field-label">Request Status:</div>
          <div class="field-value">{{ range .RequestStatuses }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Images }}
        <div class="field">
          <div class="field-label">Images:</div>
          <div class="field-value">
            {{ range .Images }}
              {{ $href := imageHref . }}• {{ imageSummary . }}{{ if $href }} — <a href="{{ $href }}">Open</a>{{ end }}<br>
            {{ end }}
          </div>
        </div>
        {{ end }}
        {{ if .Conferences }}
        <div class="field">
          <div class="field-label">Conference:</div>
          <div class="field-value conference-buttons">
            {{ range .Conferences }}
              {{ if .URI }}<a class="conference-btn" href="{{ .URI }}">{{ conferenceLabel . }}</a>{{ end }}
            {{ end }}
          </div>
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
        <div class="icon" style="{{ eventIconStyle .Color $.AccentColor }}"></div>
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
          <div class="field-value">{{ $pct }}
            <div class="progress-track"><div class="progress-fill" style="width: {{ $pct }};"></div></div>
          </div>
        </div>
        {{ end }}
        {{ if .Organizer }}
        <div class="field">
          <div class="field-label">Organizer:</div>
          <div class="field-value">{{ cleanText (str .Organizer) }}</div>
        </div>
        {{ end }}
        {{ if .Contacts }}
        <div class="field">
          <div class="field-label">Contacts:</div>
          <div class="field-value">{{ range .Contacts }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ $todoColor := sanitizeColor (str .Color) }}
        {{ if $todoColor }}
        <div class="field">
          <div class="field-label">Color:</div>
          <div class="field-value"><span class="color-chip" style="background: {{ $todoColor }}"></span>{{ $todoColor }}</div>
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
        {{ if .Conferences }}
        <div class="field">
          <div class="field-label">Conference:</div>
          <div class="field-value conference-buttons">
            {{ range .Conferences }}
              {{ if .URI }}<a class="conference-btn" href="{{ .URI }}">{{ conferenceLabel . }}</a>{{ end }}
            {{ end }}
          </div>
        </div>
        {{ end }}
        {{ if .Comments }}
        <div class="field">
          <div class="field-label">Comments:</div>
          <div class="field-value">{{ range .Comments }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .RelatedTo }}
        <div class="field">
          <div class="field-label">Related To:</div>
          <div class="field-value">{{ range .RelatedTo }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .RequestStatuses }}
        <div class="field">
          <div class="field-label">Request Status:</div>
          <div class="field-value">{{ range .RequestStatuses }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Images }}
        <div class="field">
          <div class="field-label">Images:</div>
          <div class="field-value">
            {{ range .Images }}
              {{ $href := imageHref . }}• {{ imageSummary . }}{{ if $href }} — <a href="{{ $href }}">Open</a>{{ end }}<br>
            {{ end }}
          </div>
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
        {{ if .Conferences }}
        <div class="field">
          <div class="field-label">Conference:</div>
          <div class="field-value conference-buttons">
            {{ range .Conferences }}
              {{ if .URI }}<a class="conference-btn" href="{{ .URI }}">{{ conferenceLabel . }}</a>{{ end }}
            {{ end }}
          </div>
        </div>
        {{ end }}
        {{ if .Attachments }}
        <div class="field">
          <div class="field-label">Attachments:</div>
          <div class="field-value attachment-list">{{ range .Attachments }}<div class="attachment-item"><span>{{ attachmentLabel . }}</span>{{ $href := attachmentHref . }}{{ if $href }}<a class="attachment-btn" href="{{ $href }}">Open</a>{{ end }}</div>{{ end }}</div>
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

  {{ if .Journals }}
    <h2 class="section-title">Journal Entries</h2>
    {{ range .Journals }}
    <div class="event">
      <div class="header">
        <div class="icon" style="{{ eventIconStyle nil $.AccentColor }}"></div>
        <div class="title">{{ .Summary }}</div>
      </div>

      <div class="fields">
        {{ if .DTStart }}
        <div class="field">
          <div class="field-label">Date:</div>
          <div class="field-value">{{ fmtTime .DTStart }}</div>
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
        {{ if .Class }}
        <div class="field">
          <div class="field-label">Class:</div>
          <div class="field-value">{{ str .Class }}</div>
        </div>
        {{ end }}
        {{ $cats := join .Categories ", " }}
        {{ if $cats }}
        <div class="field">
          <div class="field-label">Categories:</div>
          <div class="field-value">{{ $cats }}</div>
        </div>
        {{ end }}
        {{ if .Contacts }}
        <div class="field">
          <div class="field-label">Contacts:</div>
          <div class="field-value">{{ range .Contacts }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .RelatedTo }}
        <div class="field">
          <div class="field-label">Related To:</div>
          <div class="field-value">{{ range .RelatedTo }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .URL }}
        <div class="field">
          <div class="field-label">URL:</div>
          <div class="field-value"><a href="{{ str .URL }}">{{ str .URL }}</a></div>
        </div>
        {{ end }}
        {{ if .Conferences }}
        <div class="field">
          <div class="field-label">Conference:</div>
          <div class="field-value conference-buttons">
            {{ range .Conferences }}
              {{ if .URI }}<a class="conference-btn" href="{{ .URI }}">{{ conferenceLabel . }}</a>{{ end }}
            {{ end }}
          </div>
        </div>
        {{ end }}
        {{ if .DateTimeStamp }}
        <div class="field">
          <div class="field-label">Timestamp:</div>
          <div class="field-value">{{ fmtTime .DateTimeStamp }}</div>
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
        {{ with .Recurrence }}
          {{ if .RRule }}
          <div class="field">
            <div class="field-label">RRULE:</div>
            <div class="field-value">{{ str .RRule }}</div>
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
          <div class="field-value attachment-list">{{ range .Attachments }}<div class="attachment-item"><span>{{ attachmentLabel . }}</span>{{ $href := attachmentHref . }}{{ if $href }}<a class="attachment-btn" href="{{ $href }}">Open</a>{{ end }}</div>{{ end }}</div>
        </div>
        {{ end }}
        {{ $urls := join .DiscoveredURLs ", " }}
        {{ if $urls }}
        <div class="field">
          <div class="field-label">Referenced URLs:</div>
          <div class="field-value">{{ $urls }}</div>
        </div>
        {{ end }}
        {{ if .Images }}
        <div class="field">
          <div class="field-label">Images:</div>
          <div class="field-value">{{ range .Images }}{{ $href := imageHref . }}• {{ imageSummary . }}{{ if $href }} — <a href="{{ $href }}">Open</a>{{ end }}<br>{{ end }}</div>
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

  {{ if .Availabilities }}
    <h2 class="section-title">Published Availability</h2>
    {{ range .Availabilities }}
    <div class="event">
      <div class="header">
        <div class="icon" style="{{ eventIconStyle nil $.AccentColor }}"></div>
        <div class="title">{{ if .Summary }}{{ str .Summary }}{{ else }}Availability Window{{ end }}</div>
      </div>

      <div class="fields">
        {{ if .BusyType }}
        <div class="field">
          <div class="field-label">Busy Type:</div>
          <div class="field-value">{{ str .BusyType }}</div>
        </div>
        {{ end }}
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
        {{ if .Duration }}
        <div class="field">
          <div class="field-label">Duration:</div>
          <div class="field-value">{{ str .Duration }}</div>
        </div>
        {{ end }}
        {{ if .Organizer }}
        <div class="field">
          <div class="field-label">Organizer:</div>
          <div class="field-value">{{ str .Organizer }}</div>
        </div>
        {{ end }}
        {{ if .Location }}
        <div class="field">
          <div class="field-label">Location:</div>
          <div class="field-value">{{ cleanText (str .Location) }}</div>
        </div>
        {{ end }}
        {{ if .URL }}
        <div class="field">
          <div class="field-label">URL:</div>
          <div class="field-value"><a href="{{ str .URL }}">{{ str .URL }}</a></div>
        </div>
        {{ end }}
        {{ if .Contacts }}
        <div class="field">
          <div class="field-label">Contacts:</div>
          <div class="field-value">{{ range .Contacts }}• {{ . }}<br>{{ end }}</div>
        </div>
        {{ end }}
        {{ if .Categories }}
        <div class="field">
          <div class="field-label">Categories:</div>
          <div class="field-value">{{ join .Categories ", " }}</div>
        </div>
        {{ end }}
        {{ $prio := formatInt .Priority }}
        {{ if $prio }}
        <div class="field">
          <div class="field-label">Priority:</div>
          <div class="field-value">{{ $prio }}</div>
        </div>
        {{ end }}
        {{ $seq := formatInt .Sequence }}
        {{ if $seq }}
        <div class="field">
          <div class="field-label">Sequence:</div>
          <div class="field-value">{{ $seq }}</div>
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
        {{ if .Available }}
        <div class="field">
          <div class="field-label">Slots:</div>
          <div class="field-value">
            {{ range .Available }}
              • {{ availableWindowSummary . }}<br>
              {{ $details := availabilityWindowDetails . }}
              {{ range $details }}{{ . }}<br>{{ end }}
            {{ end }}
          </div>
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
          <div class="field-value attachment-list">{{ range .Attachments }}<div class="attachment-item"><span>{{ attachmentLabel . }}</span>{{ $href := attachmentHref . }}{{ if $href }}<a class="attachment-btn" href="{{ $href }}">Open</a>{{ end }}</div>{{ end }}</div>
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
	Name            *string
	ProdID          *string
	Method          *string
	Description     *string
	URL             *string
	Calscale        *string
	TimezoneID      *string
	Timezones       []icsparse.TimezoneInfo
	Events          []icsparse.EventInfo
	Todos           []icsparse.TodoInfo
	FreeBusy        []icsparse.FreeBusyInfo
	Availabilities  []icsparse.AvailabilityInfo
	Journals        []icsparse.JournalInfo
	Style           string
	AccentColor     string
	CalendarColor   string
	CalCategories   []string
	CalContacts     []string
	CalImages       []icsparse.ImageInfo
	RefreshInterval *string
	Source          *string
}

func WriteInviteHTML(cal *icsparse.CalendarInfo, outPath string, style string) error {
	accent := "#ff6600"
	calColor := sanitizeColorPtr(cal.Color)
	if calColor != "" {
		accent = calColor
	}
	data := viewData{
		Name:            cal.Name,
		ProdID:          cal.ProdID,
		Method:          cal.Method,
		Description:     cal.Description,
		URL:             cal.URL,
		Calscale:        cal.Calscale,
		TimezoneID:      cal.TimezoneID,
		Timezones:       cal.Timezones,
		Events:          cal.Events,
		Todos:           cal.Todos,
		FreeBusy:        cal.FreeBusy,
		Availabilities:  cal.Availabilities,
		Journals:        cal.Journals,
		Style:           style,
		AccentColor:     accent,
		CalendarColor:   calColor,
		CalCategories:   cal.Categories,
		CalContacts:     cal.Contacts,
		CalImages:       cal.Images,
		RefreshInterval: cal.RefreshInterval,
		Source:          cal.Source,
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
