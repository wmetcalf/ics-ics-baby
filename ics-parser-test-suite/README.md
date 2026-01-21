# ICS Parser Security Test Suite

A comprehensive test suite for validating ICS (iCalendar) parsers against calendar phishing patterns, auto-accept vulnerabilities, and vendor-specific exploitation techniques.

## Overview

This test suite contains **20 benign ICS files** designed to test your parser's ability to detect and flag potentially malicious calendar patterns commonly used in calendar phishing attacks. All files are completely safe—no real malicious payloads, phishing links, or exploits—but they incorporate patterns and techniques observed in real-world calendar attacks.

**Version:** 1.0.0
**Created:** January 20, 2026
**Focus:** Auto-accept detection, organizer spoofing, vendor-specific property abuse

## What This Tests

### Core Security Patterns

1. **Auto-Accept/Auto-Add Vulnerabilities**
   - `PARTSTAT=ACCEPTED` on incoming `METHOD:REQUEST`
   - `METHOD:PUBLISH` for subscription-style additions
   - Response suppression combined with pre-acceptance

2. **Spoofing Techniques**
   - Organizer domain mismatches
   - Fake `METHOD:REPLY` messages
   - Urgency manipulation via priority flags

3. **Vendor-Specific Properties**
   - Microsoft Outlook extensions (`X-MICROSOFT-*`, `X-MS-*`)
   - Google Calendar properties (`X-GOOGLE-*`)
   - Importance flags, attachment indicators, busy status

4. **Content-Based Attacks**
   - URLs in `DESCRIPTION`, `LOCATION`, `X-ALT-DESC`
   - HTML content in `X-ALT-DESC`
   - Urgent language in `VALARM` reminders

5. **Technical Edge Cases**
   - Recurring events (`RRULE`)
   - Multiple events in single file
   - Custom time zones (`VTIMEZONE`)
   - Escape sequence handling
   - Very short duration events

## Directory Structure

```
ics-parser-test-suite/
├── README.md                      # This file
├── test-suite-metadata.json       # Complete test metadata and expected results
├── example-test-runner.py         # Example Python test runner
└── ics_files/                     # 20 test ICS files
    ├── 01-basic-needs-action.ics
    ├── 02-pre-accepted-spoof.ics
    ├── 03-description-link.ics
    ├── ...
    └── 20-reply-spoof.ics
```

## Quick Start

### Manual Testing

1. **Email Test**: Attach ICS files to emails and send to test accounts (Outlook, Gmail)
2. **Import Test**: Directly import files into calendar applications
3. **Parser Test**: Feed files through your ICS parser and compare results with metadata

### Using the Test Runner (Python)

```bash
python example-test-runner.py
```

This will:
- Parse each ICS file
- Check for expected risk patterns
- Report detected flags
- Compare results against expected findings

### Reading the Metadata

The `test-suite-metadata.json` file contains comprehensive information for each test:

```json
{
  "id": "02",
  "filename": "02-pre-accepted-spoof.ics",
  "risk_level": "High",
  "key_risks": ["PARTSTAT=ACCEPTED on incoming REQUEST"],
  "expected_flags": ["auto-accept spoof detected"],
  "parser_should_detect": ["METHOD:REQUEST with PARTSTAT=ACCEPTED"],
  "outlook_behavior": "Likely tentative/auto",
  "gmail_behavior": "Likely yes"
}
```

## Test Cases by Risk Level

### Very High Risk (1 test)
- **09**: Combined multiple red flags (urgency + pre-accept + fake attach + suppress)

### High Risk (6 tests)
- **02**: Pre-accepted status spoofing
- **05**: High priority/importance spoofing
- **06**: Fake attachment indicator
- **07**: Suppressed response with pre-accept
- **11**: HTML description with links
- **20**: Fake REPLY from attacker

### Medium Risk (9 tests)
- **03**: Embedded URL in description
- **04**: Published event auto-add
- **08**: Google conference extension
- **10**: Busy/free status manipulation
- **12**: Recurring event with pre-accept
- **13**: Spoofed urgent reminder
- **14**: URL in location field
- **15**: Multiple events in one file
- **16**: Cancellation of pre-accepted event
- **18**: Very short duration event

### Low Risk (4 tests)
- **01**: Basic standard invite (baseline)
- **17**: Escaped characters (RFC compliance)
- **19**: Custom time zone (technical test)

## What Your Parser Should Detect

### Critical Checks

1. **METHOD Validation**
   - `METHOD:REQUEST` with `PARTSTAT=ACCEPTED` → Flag as auto-accept spoof
   - `METHOD:PUBLISH` without user consent → Flag as auto-add risk
   - `METHOD:REPLY` from unexpected sender → Flag as spoofed response
   - `METHOD:CANCEL` from untrusted organizer → Flag as malicious removal

2. **Participation Status**
   - Incoming invites with `PARTSTAT=ACCEPTED` or `PARTSTAT=TENTATIVE`
   - `RSVP=FALSE` on unsolicited invites

3. **Organizer Verification**
   - Compare `ORGANIZER` email with actual email sender
   - Flag domain mismatches (e.g., ORGANIZER claims to be CEO but sender is external)

4. **Vendor Properties**
   - `X-MICROSOFT-CDO-IMPORTANCE:2` (high priority) → Urgency spoofing
   - `X-MS-Has-Attach:YES` without real attachment → UI spoofing
   - `X-MICROSOFT-ISRESPONSEREQUESTED:FALSE` → Response suppression
   - `X-MICROSOFT-CDO-BUSYSTATUS` manipulation
   - `X-GOOGLE-CONFERENCE` URL validation

5. **Content Scanning**
   - URLs in `DESCRIPTION`, `LOCATION`, `X-ALT-DESC`
   - HTML content in `X-ALT-DESC` (XSS risk)
   - Urgent language patterns ("URGENT", "CRITICAL", "ACTION REQUIRED")

6. **Structural Anomalies**
   - `RRULE` recurring patterns (calendar spam)
   - Multiple `VEVENT` in single file (batch spam)
   - Very short duration (<5 min) events
   - `VALARM` with suspicious content

### Expected Client Behavior

| Client | Default Auto-Add? | Notes |
|--------|-------------------|-------|
| **Outlook** | Tentative for REQUEST, possible for PUBLISH | Processes many vendor properties |
| **Gmail** | Often auto-adds from email | Strips some HTML, processes most invites |
| **Apple Calendar** | Prompts user typically | More conservative |
| **Others** | Varies widely | Test with your target clients |

## Test Case Descriptions

### File 01-10: Core Patterns

- **01-basic-needs-action.ics**: Baseline normal invite
- **02-pre-accepted-spoof.ics**: Pre-accepted status on incoming REQUEST
- **03-description-link.ics**: Benign URL in description (test link scanning)
- **04-publish-autoadd.ics**: PUBLISH method for auto-subscription
- **05-outlook-high-importance.ics**: High priority flag + urgent language
- **06-outlook-has-attach.ics**: Fake attachment indicator
- **07-outlook-suppress-reply.ics**: Suppressed response + pre-accepted
- **08-google-conference-hint.ics**: Google-specific conference extension
- **09-combo-spoof-urgent.ics**: **[HIGH ALERT]** Multiple combined techniques
- **10-busy-status-spoof.ics**: Busy/free status manipulation

### File 11-20: Advanced Patterns

- **11-html-alt-desc.ics**: HTML in X-ALT-DESC with anchor tags
- **12-recurring-daily.ics**: Recurring event (RRULE) with pre-accept
- **13-valarm-reminder.ics**: VALARM with urgent message
- **14-location-url.ics**: URL in LOCATION field
- **15-multiple-events.ics**: Multiple VEVENTs in one file (batch import)
- **16-cancel-preaccepted.ics**: METHOD:CANCEL on accepted event
- **17-escaped-chars.ics**: RFC 5545 escape sequences (\\;, \\,, \\\\, \\n)
- **18-short-duration.ics**: 1-minute event at 23:59 (minimal visibility)
- **19-tzid-custom.ics**: Custom VTIMEZONE definition
- **20-reply-spoof.ics**: Fake METHOD:REPLY acceptance

## Integration Guide

### Python Example

```python
import json
import glob

# Load metadata
with open('test-suite-metadata.json') as f:
    metadata = json.load(f)

# Test each file
for test_case in metadata['test_cases']:
    ics_path = f"ics_files/{test_case['filename']}"

    # Parse with your parser
    result = your_ics_parser.parse(ics_path)

    # Check expected detections
    for expected_flag in test_case['expected_flags']:
        if expected_flag not in result.flags:
            print(f"FAILED: {test_case['filename']} - Missing flag: {expected_flag}")

    # Check risk level
    if result.risk_level != test_case['risk_level']:
        print(f"WARNING: Risk level mismatch for {test_case['filename']}")
```

### JavaScript Example

```javascript
const fs = require('fs');
const metadata = require('./test-suite-metadata.json');

for (const testCase of metadata.test_cases) {
    const icsContent = fs.readFileSync(`ics_files/${testCase.filename}`, 'utf-8');

    // Parse with your parser
    const result = yourIcsParser.parse(icsContent);

    // Validate against expected flags
    testCase.expected_flags.forEach(expectedFlag => {
        if (!result.flags.includes(expectedFlag)) {
            console.error(`FAILED: ${testCase.filename} - Missing: ${expectedFlag}`);
        }
    });
}
```

## Recommended Parser Checks

Your parser should implement these detections:

- [ ] METHOD type validation (REQUEST, PUBLISH, REPLY, CANCEL)
- [ ] PARTSTAT verification on incoming requests
- [ ] ORGANIZER domain vs. email sender comparison
- [ ] URL detection in DESCRIPTION, LOCATION, X-ALT-DESC
- [ ] Vendor-specific property flagging (X-MICROSOFT-*, X-GOOGLE-*)
- [ ] HTML content sanitization in X-ALT-DESC
- [ ] Recurring pattern (RRULE) validation
- [ ] VALARM content inspection for urgency keywords
- [ ] METHOD:CANCEL sender verification
- [ ] METHOD:REPLY sender validation
- [ ] RFC 5545 escape sequence handling
- [ ] Event duration anomaly detection (<5 min, >24 hours)
- [ ] Multiple VEVENT batch processing limits
- [ ] Time zone (VTIMEZONE) validation
- [ ] RSVP=FALSE flag on unsolicited invites

## Known False Positives

Some legitimate uses may trigger flags:

- **Pre-accepted invites**: Some organizations pre-accept on behalf of employees
- **PUBLISH method**: Legitimate calendar subscriptions use this
- **High importance**: Real urgent meetings exist
- **Recurring events**: Normal for standup meetings, etc.

**Recommendation**: Use risk scoring with multiple factors rather than binary allow/block.

## Testing Strategy

### Phase 1: Baseline
- Test files 01, 17, 19 (low risk)
- Ensure parser doesn't crash, handles RFC correctly

### Phase 2: Core Threats
- Test files 02, 05, 06, 07, 20 (high risk)
- Validate critical security checks

### Phase 3: Advanced
- Test files 09 (very high), 11-16, 18 (medium-high)
- Check combined threat detection

### Phase 4: Edge Cases
- Test files 03, 04, 08, 10, 12-15 (medium)
- Ensure robust handling of unusual patterns

## Real-World Usage Notes

### Outlook Specifics
- **X-MICROSOFT-CDO-IMPORTANCE**: 0=Low, 1=Normal, 2=High
- **X-MICROSOFT-CDO-BUSYSTATUS**: FREE, TENTATIVE, BUSY, OOF (Out of Office)
- **X-MICROSOFT-ISRESPONSEREQUESTED**: TRUE/FALSE
- **X-MS-Has-Attach**: YES/NO (UI indicator, not actual attachment)

### Gmail Specifics
- Often auto-adds events from email invites
- **X-GOOGLE-CONFERENCE**: Populated with Meet links
- Strips some HTML but not all
- Clickable links in DESCRIPTION and LOCATION

### Common Attack Patterns
1. **CEO Fraud**: Spoofed high-importance invite from "CEO" requesting action
2. **Fake Meetings**: Pre-accepted invites with phishing links in description
3. **Calendar Spam**: PUBLISH method events with ads/scams
4. **Link Harvesting**: Benign-looking events with credential phishing links
5. **Persistent Spam**: Recurring events that repeatedly notify

## Contributing

This test suite is designed to be extensible. To add new tests:

1. Create new ICS file in `ics_files/`
2. Add entry to `test-suite-metadata.json`
3. Update README with test description
4. Test against multiple calendar clients

## License

This test suite is provided for security research and defensive purposes only. Use responsibly.

## References

- [RFC 5545 - iCalendar Specification](https://tools.ietf.org/html/rfc5545)
- [RFC 5546 - iTIP (iCalendar Transport-Independent Interoperability Protocol)](https://tools.ietf.org/html/rfc5546)
- [MITRE ATT&CK - Calendar Phishing Techniques](https://attack.mitre.org/)
- [OWASP - Social Engineering](https://owasp.org/www-community/attacks/Social_Engineering)

## Acknowledgments

Inspired by research on calendar phishing attacks targeting Microsoft Outlook, Google Calendar, and other email/calendar platforms. This suite is based on patterns observed in the wild and reported by security researchers.

## Support

For issues, questions, or contributions:
- GitHub: https://github.com/ineesdv/Tangled
- Report parser bugs specific to your implementation
- Share additional test cases or attack patterns

---

**Remember:** These are BENIGN test files. All links point to example.com and contain no actual malicious content. Always test in isolated environments first.
