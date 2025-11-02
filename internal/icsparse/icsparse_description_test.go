package icsparse

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseICSFile_DescriptionHTML(t *testing.T) {
	cal, err := ParseICSFile(filepath.Join("testdata", "description-html.ics"), "")
	if err != nil {
		t.Fatalf("ParseICSFile returned error: %v", err)
	}
	if len(cal.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(cal.Events))
	}

	first := cal.Events[0]
	if first.Description == nil {
		t.Fatalf("expected plain description for first event")
	}
	if !strings.Contains(*first.Description, "Welcome Friend") {
		t.Errorf("plain description missing expected text: %q", *first.Description)
	}
	if strings.Contains(*first.Description, "javascript") {
		t.Errorf("plain description should not contain script content: %q", *first.Description)
	}
	if first.DescriptionHTML == nil {
		t.Fatalf("expected sanitized HTML for first event")
	}
	html := *first.DescriptionHTML
	if strings.Contains(strings.ToLower(html), "script") {
		t.Errorf("sanitized HTML still contains script tag: %q", html)
	}
	if strings.Contains(strings.ToLower(html), "javascript:") {
		t.Errorf("sanitized HTML still contains dangerous href: %q", html)
	}
	if !strings.Contains(html, `<a href="https://example.com">Good Link</a>`) {
		t.Errorf("sanitized HTML missing safe link: %q", html)
	}

	second := cal.Events[1]
	if second.Description == nil || *second.Description != "Plain fallback line" {
		t.Errorf("unexpected plain description for second event: %v", second.Description)
	}
	if second.DescriptionHTML == nil {
		t.Fatalf("expected sanitized HTML for second event")
	}
	altHTML := *second.DescriptionHTML
	if !strings.Contains(altHTML, `<a href="tel:+18005551234">+1-800-555-1234</a>`) {
		t.Errorf("tel link missing from sanitized HTML: %q", altHTML)
	}
	if strings.Contains(strings.ToLower(altHTML), "style=") {
		t.Errorf("style attribute should be stripped: %q", altHTML)
	}

	manifest := cal.Manifest()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	if !strings.Contains(string(data), `"description_html":`) {
		t.Errorf("manifest JSON missing description_html: %s", string(data))
	}
}
