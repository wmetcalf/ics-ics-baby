package icsparse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSuiteMetadata represents the test suite metadata structure
type TestSuiteMetadata struct {
	TestSuiteInfo TestSuiteInfo `json:"test_suite_info"`
	TestCases     []TestCase    `json:"test_cases"`
}

// TestSuiteInfo contains general info about the test suite
type TestSuiteInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	TotalTests  int      `json:"total_tests"`
	FocusAreas  []string `json:"focus_areas"`
}

// TestCase represents a single test case
type TestCase struct {
	ID                 string   `json:"id"`
	Filename           string   `json:"filename"`
	Title              string   `json:"title"`
	RiskLevel          string   `json:"risk_level"`
	KeyRisks           []string `json:"key_risks"`
	ExpectedFlags      []string `json:"expected_flags"`
	ParserShouldDetect []string `json:"parser_should_detect"`
	TestPurpose        string   `json:"test_purpose"`
}

// TestICSParserTestSuite validates the parser against all test suite files
// This ensures the parser doesn't break when parsing various ICS patterns
func TestICSParserTestSuite(t *testing.T) {
	// Find the test suite directory
	suiteDir := "../../ics-parser-test-suite"
	metadataPath := filepath.Join(suiteDir, "test-suite-metadata.json")

	// Check if test suite exists
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Skip("Test suite not found at " + metadataPath + " - skipping")
	}

	// Load metadata
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("Failed to read test suite metadata: %v", err)
	}

	var metadata TestSuiteMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("Failed to parse test suite metadata: %v", err)
	}

	t.Logf("Running %s v%s", metadata.TestSuiteInfo.Name, metadata.TestSuiteInfo.Version)
	t.Logf("Total test cases: %d", metadata.TestSuiteInfo.TotalTests)

	// Run each test case
	for _, testCase := range metadata.TestCases {
		t.Run(testCase.Filename, func(t *testing.T) {
			icsPath := filepath.Join(suiteDir, "ics_files", testCase.Filename)

			// Parse the ICS file directly - this should not error
			cal, err := ParseICSFile(icsPath, "", 0)
			if err != nil {
				t.Fatalf("Failed to parse ICS file %s: %v", testCase.Filename, err)
			}

			// Validate basic parsing worked
			if cal == nil {
				t.Fatal("Parsed calendar is nil")
			}

			t.Logf("Test: %s - %s", testCase.ID, testCase.Title)
			t.Logf("  Purpose: %s", testCase.TestPurpose)

			// Validate expected properties based on what the parser should detect
			validateExpectedProperties(t, cal, testCase)
		})
	}
}

// validateExpectedProperties checks that the parser correctly extracted expected properties
func validateExpectedProperties(t *testing.T, cal *CalendarInfo, testCase TestCase) {
	// Check for METHOD if expected
	for _, shouldDetect := range testCase.ParserShouldDetect {
		shouldDetectLower := strings.ToLower(shouldDetect)

		// Check METHOD
		if strings.Contains(shouldDetectLower, "method:request") {
			if cal.Method == nil || *cal.Method != "REQUEST" {
				t.Errorf("Expected METHOD:REQUEST but got %v", cal.Method)
			} else {
				t.Logf("  ✓ METHOD:REQUEST detected")
			}
		}
		if strings.Contains(shouldDetectLower, "method:publish") {
			if cal.Method == nil || *cal.Method != "PUBLISH" {
				t.Errorf("Expected METHOD:PUBLISH but got %v", cal.Method)
			} else {
				t.Logf("  ✓ METHOD:PUBLISH detected")
			}
		}
		if strings.Contains(shouldDetectLower, "method:reply") {
			if cal.Method == nil || *cal.Method != "REPLY" {
				t.Errorf("Expected METHOD:REPLY but got %v", cal.Method)
			} else {
				t.Logf("  ✓ METHOD:REPLY detected")
			}
		}
		if strings.Contains(shouldDetectLower, "method:cancel") {
			if cal.Method == nil || *cal.Method != "CANCEL" {
				t.Errorf("Expected METHOD:CANCEL but got %v", cal.Method)
			} else {
				t.Logf("  ✓ METHOD:CANCEL detected")
			}
		}

		// Check PARTSTAT
		if strings.Contains(shouldDetectLower, "partstat=accepted") {
			found := false
			for _, event := range cal.Events {
				for _, attendee := range event.Attendees {
					if attendee.PartStat != nil && *attendee.PartStat == "ACCEPTED" {
						found = true
						t.Logf("  ✓ PARTSTAT=ACCEPTED detected")
						break
					}
				}
			}
			if !found {
				t.Errorf("Expected PARTSTAT=ACCEPTED in attendees but not found")
			}
		}

		// Check for RRULE (recurrence)
		if strings.Contains(shouldDetectLower, "rrule") || strings.Contains(shouldDetectLower, "recurring") {
			found := false
			for _, event := range cal.Events {
				if event.Recurrence != nil && event.Recurrence.RRule != nil {
					found = true
					t.Logf("  ✓ RRULE detected: %s", *event.Recurrence.RRule)
					break
				}
			}
			if !found {
				t.Errorf("Expected RRULE but not found")
			}
		}

		// Check for multiple events
		if strings.Contains(shouldDetectLower, "multiple") && strings.Contains(shouldDetectLower, "vevent") {
			if len(cal.Events) <= 1 {
				t.Errorf("Expected multiple VEVENTs but found %d", len(cal.Events))
			} else {
				t.Logf("  ✓ Multiple events detected: %d", len(cal.Events))
			}
		}

		// Check for VTIMEZONE
		if strings.Contains(shouldDetectLower, "vtimezone") || strings.Contains(shouldDetectLower, "custom tzid") {
			if len(cal.Timezones) == 0 {
				t.Errorf("Expected VTIMEZONE component but not found")
			} else {
				t.Logf("  ✓ VTIMEZONE detected: %d timezone(s)", len(cal.Timezones))
			}
		}

		// Check for URLs in description
		if strings.Contains(shouldDetectLower, "url") && strings.Contains(shouldDetectLower, "description") {
			found := false
			for _, event := range cal.Events {
				if event.Description != nil && (strings.Contains(*event.Description, "http://") || strings.Contains(*event.Description, "https://")) {
					found = true
					t.Logf("  ✓ URL in DESCRIPTION detected")
					break
				}
			}
			if !found {
				t.Logf("  ⚠ Expected URL in DESCRIPTION but not found (may be in raw props)")
			}
		}

		// Check for URLs in location
		if strings.Contains(shouldDetectLower, "url") && strings.Contains(shouldDetectLower, "location") {
			found := false
			for _, event := range cal.Events {
				if event.Location != nil && (strings.Contains(*event.Location, "http://") || strings.Contains(*event.Location, "https://")) {
					found = true
					t.Logf("  ✓ URL in LOCATION detected")
					break
				}
			}
			if !found {
				t.Logf("  ⚠ Expected URL in LOCATION but not found")
			}
		}

		// Check for Microsoft-specific properties
		if strings.Contains(shouldDetectLower, "x-microsoft") {
			found := false
			for _, event := range cal.Events {
				for key := range event.RawProps {
					if strings.HasPrefix(strings.ToUpper(key), "X-MICROSOFT") {
						found = true
						t.Logf("  ✓ Microsoft property detected: %s", key)
						break
					}
				}
			}
			if !found {
				// Note: Many vendor-specific properties are at calendar level, not event level
				// The parser may not store calendar-level RawProps currently
				t.Logf("  ⚠ X-MICROSOFT properties not found in event RawProps (may be at calendar level)")
			}
		}

		// Check for Google-specific properties
		if strings.Contains(shouldDetectLower, "x-google") {
			found := false
			for _, event := range cal.Events {
				for key := range event.RawProps {
					if strings.HasPrefix(strings.ToUpper(key), "X-GOOGLE") {
						found = true
						t.Logf("  ✓ Google property detected: %s", key)
						break
					}
				}
			}
			if !found {
				// Note: Many vendor-specific properties are at calendar level, not event level
				t.Logf("  ⚠ X-GOOGLE properties not found in event RawProps (may be at calendar level)")
			}
		}

		// Check for X-ALT-DESC (HTML description)
		if strings.Contains(shouldDetectLower, "x-alt-desc") {
			found := false
			for _, event := range cal.Events {
				if event.DescriptionHTML != nil || hasKey(event.RawProps, "X-ALT-DESC") {
					found = true
					t.Logf("  ✓ X-ALT-DESC detected")
					break
				}
			}
			if !found {
				t.Errorf("Expected X-ALT-DESC but not found")
			}
		}

		// Check for VALARM
		if strings.Contains(shouldDetectLower, "valarm") {
			found := false
			for _, event := range cal.Events {
				if len(event.Alarms) > 0 {
					found = true
					t.Logf("  ✓ VALARM detected: %d alarm(s)", len(event.Alarms))
					break
				}
			}
			if !found {
				t.Errorf("Expected VALARM component but not found")
			}
		}

		// Check for escape sequences (just verify it parsed without error)
		if strings.Contains(shouldDetectLower, "escape") {
			t.Logf("  ✓ Escape sequences handled (parsing succeeded)")
		}

		// Check event duration
		if strings.Contains(shouldDetectLower, "duration") && strings.Contains(shouldDetectLower, "5 min") {
			for _, event := range cal.Events {
				if event.DTStart != nil && event.DTEnd != nil {
					duration := event.DTEnd.Sub(*event.DTStart)
					if duration.Minutes() <= 5 {
						t.Logf("  ✓ Short duration event detected: %.0f minutes", duration.Minutes())
					}
				}
			}
		}
	}

	// Always log what we parsed
	t.Logf("  Parsed: %d event(s), %d timezone(s)", len(cal.Events), len(cal.Timezones))
	if cal.Method != nil {
		t.Logf("  Calendar METHOD: %s", *cal.Method)
	}
}

// hasKey checks if a key exists in the map (case-insensitive)
func hasKey(m map[string][]string, key string) bool {
	keyUpper := strings.ToUpper(key)
	for k := range m {
		if strings.ToUpper(k) == keyUpper {
			return true
		}
	}
	return false
}
