package icsparse

// Finding is a structured, stable-coded form of one alert string. Message is
// exactly the string appended to SuspiciousPatterns or InformationalPatterns,
// so the manifest and HTML are unchanged. Codes are a contract once shipped.
type Finding struct {
	Code     string            `json:"code"`
	Severity string            `json:"severity"` // info | low | medium | high
	EventUID string            `json:"event_uid,omitempty"`
	Message  string            `json:"message"`
	Evidence map[string]string `json:"evidence,omitempty"`
}

// FindingsSchemaVersion identifies the --findings-json document format.
const FindingsSchemaVersion = "ics-ics-baby-findings/1"

type findingMeta struct {
	Severity string
}

// findingCodes is the single table of alert codes and their severities
// (first pass). "info" codes are the informational patterns; every other
// severity is a suspicious pattern. The same code is used at event and
// calendar level when the alert text is the same; calendar-level findings
// carry no event_uid.
var findingCodes = map[string]findingMeta{
	// organizer / identity
	"organizer_sent_by_mismatch": {"high"},
	"high_sequence":              {"medium"},
	// location
	"location_open_redirect": {"high"},
	"location_url":           {"medium"},
	// attendee status / RSVP
	"auto_accept_spoof":    {"high"},
	"auto_tentative_spoof": {"medium"},
	"rsvp_suppressed":      {"medium"},
	"status_tentative":     {"low"},
	// alarms / timing
	"alarm_spam":     {"medium"},
	"absolute_alarm": {"low"},
	"display_alarm":  {"info"},
	"short_duration": {"low"},
	// class / priority
	"class_sensitive":  {"low"},
	"priority_urgency": {"medium"},
	"priority_high":    {"info"},
	// Microsoft headers
	"importance_spoof":         {"medium"},
	"fake_attachment_flag":     {"medium"},
	"calendar_attachment_flag": {"low"},
	"response_suppressed":      {"medium"},
	"ms_busy_status":           {"info"},
	"busy_status_spoof":        {"low"},
	"ms_intended_status":       {"info"},
	"calendar_intended_status": {"info"},
	"force_inspector_open":     {"medium"},
	"appt_sequence_override":   {"medium"},
	"calendar_appt_sequence":   {"medium"},
	"all_day_flag":             {"low"},
	"teams_url_hijack":         {"high"},
	"teams_open_redirect":      {"high"},
	"unusual_teams_header":     {"low"},
	"ms_header_url":            {"medium"},
	"ms_header_open_redirect":  {"high"},
	// Google headers
	"google_conference_nonstandard": {"medium"},
	"google_header_url":             {"medium"},
	"google_trusted_domain_bypass":  {"high"},
	// discovered URLs
	"discovered_url_open_redirect": {"high"},
	// combinations
	"combo_preaccepted_malicious_url": {"high"},
	"combo_preaccepted_stealth":       {"high"},
	"combo_alarm_spam":                {"high"},
	// calendar METHOD
	"method_cancel":  {"medium"},
	"method_reply":   {"low"},
	"method_publish": {"medium"},
}

// alertSink appends an alert string to the existing pattern slices and the
// matching Finding, so the two can never drift.
type alertSink struct {
	signals  *AutoprocessingSignals
	findings []Finding
	uid      string
}

func (a *alertSink) add(code string, evidence map[string]string, message string) {
	meta, ok := findingCodes[code]
	if !ok {
		panic("icsparse: unknown finding code " + code)
	}
	if meta.Severity == "info" {
		a.signals.InformationalPatterns = append(a.signals.InformationalPatterns, message)
	} else {
		a.signals.SuspiciousPatterns = append(a.signals.SuspiciousPatterns, message)
	}
	if len(evidence) == 0 {
		evidence = nil
	}
	a.findings = append(a.findings, Finding{
		Code: code, Severity: meta.Severity, EventUID: a.uid, Message: message, Evidence: evidence,
	})
}

// kv is shorthand for evidence maps at alert sites.
type kv = map[string]string

// FindingsDoc builds the --findings-json document: calendar-level findings
// first, then each event's in event order.
func FindingsDoc(cal *CalendarInfo, inputSHA256 string) map[string]any {
	all := []Finding{}
	if cal != nil {
		all = append(all, cal.Findings...)
		for _, e := range cal.Events {
			all = append(all, e.Findings...)
		}
	}
	return map[string]any{
		"schema_version": FindingsSchemaVersion,
		"input_sha256":   inputSHA256,
		"findings":       all,
	}
}

// FindingCodes returns every finding code and its severity.
func FindingCodes() map[string]string {
	out := make(map[string]string, len(findingCodes))
	for code, meta := range findingCodes {
		out[code] = meta.Severity
	}
	return out
}
