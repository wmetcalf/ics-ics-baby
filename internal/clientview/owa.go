package clientview

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"ics-ics-baby/internal/icsparse"
)

// Output names written under the output directory.
const (
	OWAHTMLName    = "ics-ics-baby-owa.html"
	OWASidecarName = "ics-ics-baby-owa.json"
	OWAPNGName     = "ics-ics-baby-owa.png"
	InlineDirName  = "ics-ics-baby-inline"

	maxOtherEvents = 50
)

// Sidecar describes an OWA view: what was rendered and everything that was
// neutralized, so a pipeline need not re-parse the HTML.
type Sidecar struct {
	Profile         string      `json:"profile"`
	Emulation       bool        `json:"emulation"`
	Status          string      `json:"status"` // ok | skipped | render_failed
	Reason          string      `json:"reason,omitempty"`
	Error           string      `json:"error,omitempty"`
	InputSHA256     string      `json:"input_sha256"`
	EventUID        string      `json:"event_uid,omitempty"`
	OtherEvents     []string    `json:"other_events,omitempty"`
	JoinURL         string      `json:"join_url,omitempty"`
	RemoteRefs      []RemoteRef `json:"remote_refs"`
	Links           []Link      `json:"links"`
	Dropped         []string    `json:"dropped"`
	BlockedRequests []string    `json:"blocked_requests"`
}

//go:embed owa.html.tmpl
var owaTemplateText string

var owaTemplate = template.Must(template.New("owa").Parse(owaTemplateText))

// SelectEvent picks the event OWA would show as the meeting request.
// REQUEST, PUBLISH and no METHOD render; CANCEL renders as canceled; any other
// METHOD, or a calendar without VEVENTs, yields nil and a reason.
func SelectEvent(cal *icsparse.CalendarInfo) (ev *icsparse.EventInfo, canceled bool, reason string) {
	if cal == nil || len(cal.Events) == 0 {
		return nil, false, "no VEVENT"
	}
	method := ""
	if cal.Method != nil {
		method = strings.ToUpper(strings.TrimSpace(*cal.Method))
	}
	switch method {
	case "", "REQUEST", "PUBLISH":
		return &cal.Events[0], false, ""
	case "CANCEL":
		return &cal.Events[0], true, ""
	default:
		return nil, false, fmt.Sprintf("METHOD:%s is not shown as a meeting request", method)
	}
}

// JoinURL returns the meeting URL OWA would put behind its Join button, in
// precedence order X-MICROSOFT-SKYPETEAMSMEETINGURL,
// X-MICROSOFT-ONLINEMEETINGCONFLINK, then the first CONFERENCE URI. active is
// true only for an http(s) URL with a host.
func JoinURL(ev *icsparse.EventInfo) (raw string, active bool) {
	if ev == nil {
		return "", false
	}
	for _, key := range []string{"X-MICROSOFT-SKYPETEAMSMEETINGURL", "X-MICROSOFT-ONLINEMEETINGCONFLINK"} {
		if vals := ev.RawProps[key]; len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			raw = strings.TrimSpace(icsparse.UnescapeText(vals[0]))
			break
		}
		if ev.AutoprocessingSignals != nil {
			if v := strings.TrimSpace(ev.AutoprocessingSignals.MicrosoftHeaders[key]); v != "" {
				raw = strings.TrimSpace(icsparse.UnescapeText(v))
				break
			}
		}
	}
	if raw == "" {
		for _, c := range ev.Conferences {
			if strings.TrimSpace(c.URI) != "" {
				raw = strings.TrimSpace(c.URI)
				break
			}
		}
	}
	return raw, isWebURL(raw)
}

// isWebURL: http(s) scheme, a host, and no whitespace/control characters.
func isWebURL(raw string) bool {
	if raw == "" || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	s := strings.ToLower(u.Scheme)
	return (s == "http" || s == "https") && u.Host != ""
}

type attachmentView struct {
	Name string
	Size string
}

type owaView struct {
	Skipped       bool
	Reason        string
	Canceled      bool
	Subject       string
	Initials      string
	OrganizerName string
	OrganizerAddr string
	When          string
	Recurrence    string
	Location      string
	LocationLink  template.URL
	LocationHost  string
	JoinRaw       string
	JoinActive    bool
	JoinHref      template.URL
	JoinHost      string
	Required      []string
	Optional      []string
	BodyIsHTML    bool
	BodyHTML      template.HTML
	BodyText      string
	Attachments   []attachmentView
	OtherEvents   []string
	InputSHA256   string
}

// WriteOWA writes ics-ics-baby-owa.html (and ics-ics-baby-inline/ when the
// body carries data: images) into outDir and returns the sidecar. It does not
// write the sidecar or render a PNG; the caller does both.
func WriteOWA(cal *icsparse.CalendarInfo, inputSHA256, outDir string) (Sidecar, error) {
	sc := Sidecar{
		Profile:         "owa",
		Emulation:       true,
		Status:          "ok",
		InputSHA256:     inputSHA256,
		RemoteRefs:      []RemoteRef{},
		Links:           []Link{},
		Dropped:         []string{},
		BlockedRequests: []string{},
	}
	view := owaView{InputSHA256: inputSHA256}

	ev, canceled, reason := SelectEvent(cal)
	if ev == nil {
		sc.Status = "skipped"
		sc.Reason = reason
		view.Skipped = true
		view.Reason = reason
		return sc, writeView(outDir, view)
	}

	if ev.UID != nil {
		sc.EventUID = *ev.UID
	}
	for i := range cal.Events {
		if &cal.Events[i] == ev {
			continue
		}
		if len(sc.OtherEvents) >= maxOtherEvents {
			sc.Dropped = append(sc.Dropped, "other_events_capped")
			break
		}
		sc.OtherEvents = append(sc.OtherEvents, cal.Events[i].Summary)
	}
	view.OtherEvents = sc.OtherEvents

	view.Canceled = canceled
	view.Subject = ev.Summary
	if canceled {
		view.Subject = "Canceled: " + ev.Summary
	}

	view.OrganizerAddr = organizerAddress(ev.Organizer)
	if ev.AutoprocessingSignals != nil && ev.AutoprocessingSignals.OrganizerDetails != nil && ev.AutoprocessingSignals.OrganizerDetails.CN != nil {
		view.OrganizerName = strings.TrimSpace(*ev.AutoprocessingSignals.OrganizerDetails.CN)
	}
	if view.OrganizerName == "" {
		view.OrganizerName = view.OrganizerAddr
	}
	if view.OrganizerName == "" {
		view.OrganizerName = "Unknown organizer"
	}
	view.Initials = initials(view.OrganizerName)

	view.When = whenText(ev.DTStart, ev.DTEnd)
	if ev.Recurrence != nil && ev.Recurrence.RRule != nil {
		view.Recurrence = "Repeats: " + *ev.Recurrence.RRule
	}

	if ev.Location != nil {
		view.Location = strings.TrimSpace(*ev.Location)
		if isWebURL(view.Location) {
			u, _ := url.Parse(view.Location)
			view.LocationLink = template.URL(view.Location)
			view.LocationHost = u.Host
		}
	}

	raw, active := JoinURL(ev)
	sc.JoinURL = raw
	view.JoinRaw = raw
	view.JoinActive = active
	if active {
		u, _ := url.Parse(raw)
		view.JoinHref = template.URL(raw)
		view.JoinHost = u.Host
	}

	for _, a := range ev.Attendees {
		label := attendeeLabel(a)
		role := ""
		if a.Role != nil {
			role = strings.ToUpper(strings.TrimSpace(*a.Role))
		}
		switch role {
		case "OPT-PARTICIPANT", "NON-PARTICIPANT":
			view.Optional = append(view.Optional, label)
		default: // REQ-PARTICIPANT (the RFC 5545 default) and CHAIR
			view.Required = append(view.Required, label)
		}
	}

	if ev.AltDescRaw != nil && ev.AltDescFmtType != nil && strings.Contains(*ev.AltDescFmtType, "html") {
		res, err := OWASanitize(*ev.AltDescRaw, filepath.Join(outDir, InlineDirName))
		if err != nil {
			return sc, err
		}
		view.BodyIsHTML = true
		view.BodyHTML = template.HTML(res.HTML) // sanitized by OWASanitize
		if res.RemoteRefs != nil {
			sc.RemoteRefs = res.RemoteRefs
		}
		if res.Links != nil {
			sc.Links = res.Links
		}
		sc.Dropped = append(sc.Dropped, res.Dropped...)
	} else {
		switch {
		case ev.Description != nil:
			view.BodyText = *ev.Description
		case ev.AltDescRaw != nil:
			view.BodyText = *ev.AltDescRaw
		}
	}

	for _, att := range ev.Attachments {
		view.Attachments = append(view.Attachments, attachmentView{Name: attachmentName(att), Size: sizeText(att.Size)})
	}

	return sc, writeView(outDir, view)
}

func writeView(outDir string, view owaView) error {
	var buf bytes.Buffer
	if err := owaTemplate.Execute(&buf, view); err != nil {
		return fmt.Errorf("render owa template: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, OWAHTMLName), buf.Bytes(), 0o644)
}

func organizerAddress(org *string) string {
	if org == nil {
		return ""
	}
	v := strings.TrimSpace(*org)
	if len(v) >= 7 && strings.EqualFold(v[:7], "mailto:") {
		v = v[7:]
	}
	return v
}

func attendeeLabel(a icsparse.Attendee) string {
	addr := organizerAddress(&a.Mailto)
	if a.CN != nil && strings.TrimSpace(*a.CN) != "" {
		if addr != "" {
			return strings.TrimSpace(*a.CN) + " <" + addr + ">"
		}
		return strings.TrimSpace(*a.CN)
	}
	return addr
}

func initials(name string) string {
	var out []rune
	for _, f := range strings.Fields(name) {
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

// formatTime matches the preview's webview.formatTimeValue.
func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	if loc := t.Location(); loc != nil && loc != time.UTC {
		return t.Format("Monday, January 2, 2006 at 3:04 PM MST")
	}
	return t.Format("Monday, January 2, 2006 at 15:04 UTC")
}

func whenText(start, end *time.Time) string {
	s, e := formatTime(start), formatTime(end)
	switch {
	case s != "" && e != "":
		return s + " – " + e
	case s != "":
		return s
	default:
		return e
	}
}

func attachmentName(a icsparse.AttachmentInfo) string {
	for _, p := range []*string{a.SavedAs, a.Href} {
		if p != nil && strings.TrimSpace(*p) != "" {
			return filepath.Base(strings.TrimSpace(*p))
		}
	}
	if a.Fmt != nil && *a.Fmt != "" {
		return "attachment (" + *a.Fmt + ")"
	}
	return "attachment"
}

func sizeText(n *int) string {
	if n == nil || *n < 0 {
		return ""
	}
	switch v := float64(*n); {
	case v >= 1<<20:
		return fmt.Sprintf("%.1f MB", v/(1<<20))
	case v >= 1<<10:
		return fmt.Sprintf("%.0f KB", v/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", *n)
	}
}
