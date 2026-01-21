#!/usr/bin/env python3
"""
ICS Parser Test Suite Runner

This example script demonstrates how to test an ICS parser against the security test suite.
It performs basic pattern detection that your production parser should implement more robustly.

Usage:
    python example-test-runner.py [--verbose]

Requirements:
    None (uses only Python standard library)
"""

import json
import os
import re
import sys
from pathlib import Path
from typing import Dict, List, Set


class ICSSecurityAnalyzer:
    """
    Basic ICS security analyzer for demonstration purposes.
    Your production parser should be more sophisticated.
    """

    def __init__(self, ics_content: str, filename: str):
        self.content = ics_content
        self.filename = filename
        self.lines = ics_content.split('\n')
        self.flags: Set[str] = set()
        self.risk_score = 0
        self.properties: Dict[str, List[str]] = {}

    def parse(self):
        """Parse the ICS content and extract properties."""
        for line in self.lines:
            line = line.strip()
            if ':' in line and not line.startswith('#'):
                key, value = line.split(':', 1)
                if key not in self.properties:
                    self.properties[key] = []
                self.properties[key].append(value)

    def analyze(self) -> Dict:
        """Run security analysis on the parsed ICS file."""
        self.parse()

        # Check METHOD
        self._check_method()

        # Check PARTSTAT
        self._check_partstat()

        # Check vendor-specific properties
        self._check_microsoft_properties()
        self._check_google_properties()

        # Check for URLs
        self._check_urls()

        # Check for HTML content
        self._check_html()

        # Check for recurring patterns
        self._check_recurring()

        # Check for VALARM
        self._check_valarm()

        # Check for multiple events
        self._check_multiple_events()

        # Check event duration
        self._check_duration()

        # Determine risk level
        risk_level = self._calculate_risk_level()

        return {
            'filename': self.filename,
            'flags': sorted(list(self.flags)),
            'risk_score': self.risk_score,
            'risk_level': risk_level,
            'properties_found': list(self.properties.keys())
        }

    def _check_method(self):
        """Check METHOD property for suspicious values."""
        if 'METHOD' in self.properties:
            method = self.properties['METHOD'][0]

            if method == 'PUBLISH':
                self.flags.add('METHOD:PUBLISH detected')
                self.risk_score += 2

            elif method == 'REPLY':
                self.flags.add('METHOD:REPLY detected')
                self.risk_score += 3

            elif method == 'CANCEL':
                self.flags.add('METHOD:CANCEL detected')
                self.risk_score += 2

            elif method == 'REQUEST':
                # REQUEST is normal but combined with other factors can be risky
                pass

    def _check_partstat(self):
        """Check PARTSTAT for auto-accept patterns."""
        for line in self.lines:
            if 'PARTSTAT=ACCEPTED' in line and 'ATTENDEE' in line:
                self.flags.add('PARTSTAT=ACCEPTED on incoming invite')
                self.risk_score += 5
                break

            if 'PARTSTAT=TENTATIVE' in line and 'ATTENDEE' in line:
                self.flags.add('PARTSTAT=TENTATIVE detected')
                self.risk_score += 1

            if 'RSVP=FALSE' in line and 'ATTENDEE' in line:
                self.flags.add('RSVP=FALSE detected')
                self.risk_score += 1

    def _check_microsoft_properties(self):
        """Check for Microsoft-specific properties."""
        ms_props = {
            'X-MICROSOFT-CDO-IMPORTANCE': 'Microsoft importance flag',
            'X-MS-Has-Attach': 'Microsoft attachment indicator',
            'X-MICROSOFT-ISRESPONSEREQUESTED': 'Microsoft response suppression',
            'X-MICROSOFT-CDO-BUSYSTATUS': 'Microsoft busy status',
            'X-MICROSOFT-CDO-ALLDAYEVENT': 'Microsoft all-day flag',
            'X-MICROSOFT-LOCATIONSOURCE': 'Microsoft location source'
        }

        for prop, description in ms_props.items():
            if prop in self.properties:
                self.flags.add(f'{description} detected')

                # Check specific risky values
                if prop == 'X-MICROSOFT-CDO-IMPORTANCE' and '2' in self.properties[prop]:
                    self.flags.add('HIGH IMPORTANCE flag set')
                    self.risk_score += 3

                if prop == 'X-MS-Has-Attach' and 'YES' in self.properties[prop][0].upper():
                    self.flags.add('Fake attachment indicator detected')
                    self.risk_score += 4

                if prop == 'X-MICROSOFT-ISRESPONSEREQUESTED' and 'FALSE' in self.properties[prop][0].upper():
                    self.flags.add('Response suppression detected')
                    self.risk_score += 3

                if prop == 'X-MICROSOFT-CDO-BUSYSTATUS' and 'OOF' in self.properties[prop][0].upper():
                    self.flags.add('Out-of-office status manipulation')
                    self.risk_score += 2

    def _check_google_properties(self):
        """Check for Google-specific properties."""
        if 'X-GOOGLE-CONFERENCE' in self.properties:
            self.flags.add('Google conference property detected')
            self.risk_score += 1

    def _check_urls(self):
        """Check for URLs in various fields."""
        url_pattern = re.compile(r'https?://[^\s<>"]+')

        for line in self.lines:
            if 'DESCRIPTION:' in line or 'DESCRIPTION;' in line:
                urls = url_pattern.findall(line)
                if urls:
                    self.flags.add('URL in DESCRIPTION field')
                    self.risk_score += 2

            if 'LOCATION:' in line:
                urls = url_pattern.findall(line)
                if urls:
                    self.flags.add('URL in LOCATION field')
                    self.risk_score += 2

            if 'X-GOOGLE-CONFERENCE:' in line:
                urls = url_pattern.findall(line)
                if urls:
                    self.flags.add('URL in conference field')
                    self.risk_score += 1

    def _check_html(self):
        """Check for HTML content."""
        if 'X-ALT-DESC' in self.properties or any('X-ALT-DESC' in line for line in self.lines):
            self.flags.add('HTML content in X-ALT-DESC')
            self.risk_score += 4

            # Check for HTML tags
            html_pattern = re.compile(r'<[^>]+>')
            for line in self.lines:
                if 'X-ALT-DESC' in line and html_pattern.search(line):
                    self.flags.add('HTML tags detected')
                    if '<a href' in line.lower():
                        self.flags.add('HTML anchor tags with links')
                        self.risk_score += 2

    def _check_recurring(self):
        """Check for recurring patterns."""
        if 'RRULE' in self.properties:
            self.flags.add('Recurring event detected (RRULE)')
            self.risk_score += 2

            for rrule in self.properties['RRULE']:
                if 'FREQ=DAILY' in rrule:
                    self.flags.add('Daily recurring pattern')
                    self.risk_score += 1

    def _check_valarm(self):
        """Check for VALARM (reminders)."""
        if any('BEGIN:VALARM' in line for line in self.lines):
            self.flags.add('VALARM reminder detected')

            # Check for urgent language
            urgent_keywords = ['URGENT', 'CRITICAL', 'IMMEDIATELY', 'ACTION REQUIRED']
            for line in self.lines:
                if any(keyword in line.upper() for keyword in urgent_keywords):
                    self.flags.add('Urgent language in reminder')
                    self.risk_score += 2
                    break

    def _check_multiple_events(self):
        """Check for multiple events in single file."""
        event_count = sum(1 for line in self.lines if 'BEGIN:VEVENT' in line)
        if event_count > 1:
            self.flags.add(f'Multiple events detected ({event_count} events)')
            self.risk_score += 2

    def _check_duration(self):
        """Check for suspicious event durations."""
        dtstart = None
        dtend = None

        if 'DTSTART' in self.properties and 'DTEND' in self.properties:
            # Simple check - just look for events at 23:59
            for dt in self.properties['DTSTART']:
                if '2359' in dt or 'T235900' in dt:
                    self.flags.add('Event near midnight (potential spam)')
                    self.risk_score += 2

    def _calculate_risk_level(self) -> str:
        """Calculate risk level based on risk score and flags."""
        if self.risk_score >= 15:
            return 'Very High'
        elif self.risk_score >= 10:
            return 'High'
        elif self.risk_score >= 5:
            return 'Medium'
        elif self.risk_score >= 2:
            return 'Low-Medium'
        else:
            return 'Low'


class TestSuiteRunner:
    """Main test suite runner."""

    def __init__(self, suite_dir: Path, verbose: bool = False):
        self.suite_dir = suite_dir
        self.verbose = verbose
        self.metadata = None
        self.results = []

    def load_metadata(self):
        """Load test suite metadata."""
        metadata_path = self.suite_dir / 'test-suite-metadata.json'
        with open(metadata_path, 'r') as f:
            self.metadata = json.load(f)

    def run_tests(self):
        """Run all tests in the suite."""
        print(f"Running ICS Parser Security Test Suite")
        print(f"{'=' * 70}\n")

        ics_dir = self.suite_dir / 'ics_files'
        test_cases = self.metadata['test_cases']

        passed = 0
        failed = 0
        warnings = 0

        for i, test_case in enumerate(test_cases, 1):
            filename = test_case['filename']
            file_path = ics_dir / filename

            print(f"[{i:2d}/20] Testing: {filename}")

            if not file_path.exists():
                print(f"  ❌ ERROR: File not found\n")
                failed += 1
                continue

            # Read ICS file
            with open(file_path, 'r') as f:
                ics_content = f.read()

            # Analyze
            analyzer = ICSSecurityAnalyzer(ics_content, filename)
            result = analyzer.analyze()

            # Compare with expected
            comparison = self._compare_results(result, test_case)

            # Store result
            self.results.append({
                'test_case': test_case,
                'actual_result': result,
                'comparison': comparison
            })

            # Display results
            expected_level = test_case['risk_level']
            actual_level = result['risk_level']

            if comparison['risk_match']:
                print(f"  ✓ Risk Level: {actual_level} (expected: {expected_level})")
            else:
                print(f"  ⚠ Risk Level: {actual_level} (expected: {expected_level})")
                warnings += 1

            if self.verbose:
                print(f"  Detected flags: {len(result['flags'])}")
                for flag in result['flags']:
                    print(f"    - {flag}")

                missing = comparison['missing_detections']
                if missing:
                    print(f"  Missing expected detections:")
                    for m in missing:
                        print(f"    - {m}")

            if comparison['critical_detection_passed']:
                passed += 1
                status = "✓ PASS"
            else:
                failed += 1
                status = "✗ FAIL"

            print(f"  {status}\n")

        # Summary
        print(f"{'=' * 70}")
        print(f"Test Summary:")
        print(f"  Total Tests: 20")
        print(f"  Passed: {passed}")
        print(f"  Failed: {failed}")
        print(f"  Warnings: {warnings}")
        print(f"  Success Rate: {passed / 20 * 100:.1f}%")
        print(f"{'=' * 70}\n")

        return passed, failed, warnings

    def _compare_results(self, actual: Dict, expected: Dict) -> Dict:
        """Compare actual results with expected results."""
        comparison = {
            'risk_match': False,
            'critical_detection_passed': True,
            'missing_detections': []
        }

        # Check risk level (approximate match is OK)
        expected_level = expected['risk_level']
        actual_level = actual['risk_level']

        risk_order = ['Low', 'Low-Medium', 'Medium', 'High', 'Very High']

        # Allow +/- 1 level difference
        expected_idx = risk_order.index(expected_level) if expected_level in risk_order else -1
        actual_idx = risk_order.index(actual_level) if actual_level in risk_order else -1

        if abs(expected_idx - actual_idx) <= 1:
            comparison['risk_match'] = True

        # Check critical detections
        critical_patterns = [
            ('PARTSTAT=ACCEPTED', ['pre-accept', 'accepted']),
            ('X-MS-Has-Attach', ['attachment', 'attach']),
            ('METHOD:PUBLISH', ['publish']),
            ('METHOD:REPLY', ['reply']),
            ('X-ALT-DESC', ['html', 'alt-desc']),
            ('RRULE', ['recurring', 'rrule'])
        ]

        for pattern, keywords in critical_patterns:
            if pattern in ' '.join(expected['parser_should_detect']):
                # Check if any keyword appears in actual flags
                found = any(
                    any(kw in flag.lower() for kw in keywords)
                    for flag in actual['flags']
                )
                if not found:
                    comparison['missing_detections'].append(pattern)
                    if expected['risk_level'] in ['High', 'Very High']:
                        comparison['critical_detection_passed'] = False

        return comparison

    def generate_report(self, output_path: Path):
        """Generate detailed JSON report."""
        report = {
            'test_suite_info': self.metadata['test_suite_info'],
            'test_results': self.results,
            'summary': {
                'total_tests': len(self.results),
                'passed': sum(1 for r in self.results if r['comparison']['critical_detection_passed']),
                'failed': sum(1 for r in self.results if not r['comparison']['critical_detection_passed'])
            }
        }

        with open(output_path, 'w') as f:
            json.dump(report, f, indent=2)

        print(f"Detailed report saved to: {output_path}")


def main():
    """Main entry point."""
    verbose = '--verbose' in sys.argv or '-v' in sys.argv

    # Determine suite directory
    script_dir = Path(__file__).parent
    suite_dir = script_dir

    if not (suite_dir / 'test-suite-metadata.json').exists():
        print("ERROR: test-suite-metadata.json not found")
        print(f"Expected location: {suite_dir / 'test-suite-metadata.json'}")
        sys.exit(1)

    if not (suite_dir / 'ics_files').exists():
        print("ERROR: ics_files directory not found")
        sys.exit(1)

    # Run tests
    runner = TestSuiteRunner(suite_dir, verbose=verbose)

    try:
        runner.load_metadata()
        passed, failed, warnings = runner.run_tests()

        # Generate report
        report_path = suite_dir / 'test-results.json'
        runner.generate_report(report_path)

        # Exit code based on results
        sys.exit(0 if failed == 0 else 1)

    except Exception as e:
        print(f"ERROR: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == '__main__':
    main()
