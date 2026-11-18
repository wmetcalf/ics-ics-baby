package main

import (
	"encoding/json"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"ics-ics-baby/internal/render"
)

// TestICSProcessingIntegration tests end-to-end ICS file processing
func TestICSProcessingIntegration(t *testing.T) {
	// Build the tool first
	buildCmd := exec.Command("go", "build", "-o", "ics-ics-baby-test", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build tool: %v", err)
	}
	defer os.Remove("ics-ics-baby-test")

	testCases := []struct {
		name        string
		icsFile     string
		engine      string
		expectFiles []string
	}{
		{
			name:    "Appointment1_go",
			icsFile: "/home/coz/Appointment1.ics",
			engine:  "go",
			expectFiles: []string{
				"ics-ics-baby-manifest.json",
				"ics-ics-baby-preview.html",
				"ics-ics-baby-preview.png",
			},
		},
		{
			name:    "TestSuite_PreAccepted",
			icsFile: "../../ics-parser-test-suite/ics_files/02-pre-accepted-spoof.ics",
			engine:  "go",
			expectFiles: []string{
				"ics-ics-baby-manifest.json",
				"ics-ics-baby-preview.html",
				"ics-ics-baby-preview.png",
			},
		},
		{
			name:    "TestSuite_MultipleEvents",
			icsFile: "../../ics-parser-test-suite/ics_files/15-multiple-events.ics",
			engine:  "go",
			expectFiles: []string{
				"ics-ics-baby-manifest.json",
				"ics-ics-baby-preview.html",
				"ics-ics-baby-preview.png",
			},
		},
		{
			name:    "TestSuite_HTMLDescription",
			icsFile: "../../ics-parser-test-suite/ics_files/11-html-alt-desc.ics",
			engine:  "go",
			expectFiles: []string{
				"ics-ics-baby-manifest.json",
				"ics-ics-baby-preview.html",
				"ics-ics-baby-preview.png",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp output directory
			tmpDir := t.TempDir()

			// Check if input file exists
			if _, err := os.Stat(tc.icsFile); os.IsNotExist(err) {
				t.Skipf("Test file not found: %s", tc.icsFile)
			}

			// Run the tool
			binPath := "./ics-ics-baby-test"
			cmd := exec.Command(
				binPath,
				"-screenshot-engine", tc.engine,
				"-out", tmpDir,
				tc.icsFile,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Tool execution failed: %v\nOutput: %s", err, output)
			}

			t.Logf("Tool output: %s", output)

			// Verify all expected files were created
			for _, expectedFile := range tc.expectFiles {
				filePath := filepath.Join(tmpDir, expectedFile)
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Errorf("Expected file not created: %s", expectedFile)
				} else {
					t.Logf("✓ Created: %s", expectedFile)
				}
			}

			// Validate manifest JSON
			manifestPath := filepath.Join(tmpDir, "ics-ics-baby-manifest.json")
			validateManifest(t, manifestPath)

			// Validate HTML
			htmlPath := filepath.Join(tmpDir, "ics-ics-baby-preview.html")
			validateHTML(t, htmlPath)

			// Validate PNG
			pngPath := filepath.Join(tmpDir, "ics-ics-baby-preview.png")
			validatePNG(t, pngPath)
		})
	}
}

// TestRenderingEngines tests that rendering engines work
func TestRenderingEngines(t *testing.T) {
	// Test go engine by default; wkhtml requires external binary
	engines := []string{"go"}

	// Check if wkhtmltoimage is available and add it to tests
	if _, err := exec.LookPath("wkhtmltoimage"); err == nil {
		engines = append(engines, "wkhtml")
	} else {
		t.Log("wkhtmltoimage not found, skipping wkhtml engine tests")
	}

	testFile := "../../ics-parser-test-suite/ics_files/01-basic-needs-action.ics"

	// Check if test file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test suite not found")
	}

	// Build the tool
	buildCmd := exec.Command("go", "build", "-o", "ics-ics-baby-test", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build tool: %v", err)
	}
	defer os.Remove("ics-ics-baby-test")

	for _, engine := range engines {
		t.Run(engine, func(t *testing.T) {
			tmpDir := t.TempDir()

			binPath := "./ics-ics-baby-test"
			cmd := exec.Command(
				binPath,
				"-screenshot-engine", engine,
				"-out", tmpDir,
				testFile,
			)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Engine %s failed: %v\nOutput: %s", engine, err, output)
			}

			// Verify PNG was created and is valid
			pngPath := filepath.Join(tmpDir, "ics-ics-baby-preview.png")
			img, err := loadPNG(pngPath)
			if err != nil {
				t.Fatalf("Failed to load PNG from %s engine: %v", engine, err)
			}

			bounds := img.Bounds()
			t.Logf("Engine %s: PNG size = %dx%d", engine, bounds.Dx(), bounds.Dy())

			if bounds.Dx() == 0 || bounds.Dy() == 0 {
				t.Errorf("Engine %s produced invalid PNG dimensions", engine)
			}
		})
	}
}

// TestTestSuiteFiles processes all test suite files
func TestTestSuiteFiles(t *testing.T) {
	suiteDir := "../../ics-parser-test-suite/ics_files"

	// Check if test suite exists
	if _, err := os.Stat(suiteDir); os.IsNotExist(err) {
		t.Skip("Test suite not found")
	}

	// Build the tool
	buildCmd := exec.Command("go", "build", "-o", "ics-ics-baby-test", ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build tool: %v", err)
	}
	defer os.Remove("ics-ics-baby-test")

	// Read all ICS files
	entries, err := os.ReadDir(suiteDir)
	if err != nil {
		t.Fatalf("Failed to read test suite directory: %v", err)
	}

	successCount := 0
	failCount := 0

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".ics") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			icsFile := filepath.Join(suiteDir, entry.Name())
			tmpDir := t.TempDir()

			binPath := "./ics-ics-baby-test"
			cmd := exec.Command(
				binPath,
				"-screenshot-engine", "go",
				"-out", tmpDir,
				icsFile,
			)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("Failed to process %s: %v\nOutput: %s", entry.Name(), err, output)
				failCount++
				return
			}

			// Verify outputs exist
			for _, file := range []string{
				"ics-ics-baby-manifest.json",
				"ics-ics-baby-preview.html",
				"ics-ics-baby-preview.png",
			} {
				if _, err := os.Stat(filepath.Join(tmpDir, file)); os.IsNotExist(err) {
					t.Errorf("Missing output file: %s", file)
					failCount++
					return
				}
			}

			successCount++
			t.Logf("✓ Successfully processed %s", entry.Name())
		})
	}

	t.Logf("Processed %d files successfully, %d failures", successCount, failCount)
}

// Helper functions

func validateManifest(t *testing.T, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read manifest: %v", err)
		return
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Errorf("Manifest is not valid JSON: %v", err)
		return
	}

	// Check for expected top-level keys
	expectedKeys := []string{"calendar"}
	for _, key := range expectedKeys {
		if _, exists := manifest[key]; !exists {
			t.Errorf("Manifest missing expected key: %s", key)
		}
	}

	t.Logf("✓ Manifest JSON is valid")
}

func validateHTML(t *testing.T, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read HTML: %v", err)
		return
	}

	html := string(data)

	// Basic HTML validation
	if !strings.Contains(html, "<!DOCTYPE html>") && !strings.Contains(html, "<html") {
		t.Errorf("HTML file appears invalid (missing DOCTYPE or html tag)")
	}

	// Check for key elements
	if !strings.Contains(html, "<title>") {
		t.Logf("⚠ HTML missing <title> tag")
	}

	if len(data) == 0 {
		t.Errorf("HTML file is empty")
	}

	t.Logf("✓ HTML file is valid (%d bytes)", len(data))
}

func validatePNG(t *testing.T, path string) {
	img, err := loadPNG(path)
	if err != nil {
		t.Errorf("Failed to validate PNG: %v", err)
		return
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width == 0 || height == 0 {
		t.Errorf("PNG has invalid dimensions: %dx%d", width, height)
	}

	// Get file size
	stat, _ := os.Stat(path)

	t.Logf("✓ PNG is valid: %dx%d pixels, %d bytes", width, height, stat.Size())
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

const fixtureDir = "../../ics-parser-test-suite/ics_files/"

func requireChromiumCLI(t *testing.T) {
	t.Helper()
	if _, err := render.FindChromium(""); err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCLIChromiumEngine(t *testing.T) {
	requireChromiumCLI(t)
	fx := fixtureDir + "11-html-alt-desc.ics"
	chromeOut := filepath.Join(t.TempDir(), "c")
	res := runCLI(t, "--out", chromeOut, "--screenshot-engine", "chromium", fx)
	if res.exit != 0 {
		t.Fatalf("exit %d, stderr: %s", res.exit, res.stderr)
	}
	goOut := filepath.Join(t.TempDir(), "g")
	if r := runCLI(t, "--out", goOut, fx); r.exit != 0 {
		t.Fatalf("go engine exit %d", r.exit)
	}
	cpng := filepath.Join(chromeOut, "ics-ics-baby-preview.png")
	if _, err := loadPNG(cpng); err != nil {
		t.Fatalf("chromium PNG does not decode: %v", err)
	}
	a, _ := os.ReadFile(cpng)
	b, _ := os.ReadFile(filepath.Join(goOut, "ics-ics-baby-preview.png"))
	if string(a) == string(b) {
		t.Fatal("chromium PNG identical to go PNG; chromium engine not used")
	}
}

func TestCLIChromiumMissing(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--screenshot-engine", "chromium", "--chromium-bin", "/nonexistent", fixtureDir+"11-html-alt-desc.ics")
	if res.exit != 1 {
		t.Fatalf("exit = %d, want 1 (stderr %q)", res.exit, res.stderr)
	}
	if !strings.Contains(res.stderr, "chromium render error") {
		t.Fatalf("stderr %q lacks 'chromium render error'", res.stderr)
	}
	for _, f := range []string{"ics-ics-baby-manifest.json", "ics-ics-baby-preview.html"} {
		if !fileExists(filepath.Join(out, f)) {
			t.Fatalf("%s missing after chromium failure", f)
		}
	}
}

func readSidecar(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "ics-ics-baby-owa.json"))
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("sidecar json: %v", err)
	}
	return m
}

func TestCLIInviteViewOK(t *testing.T) {
	requireChromiumCLI(t)
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--invite-view", "owa", fixtureDir+"11-html-alt-desc.ics")
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	for _, f := range []string{"ics-ics-baby-preview.png", "ics-ics-baby-owa.html", "ics-ics-baby-owa.json", "ics-ics-baby-owa.png"} {
		if !fileExists(filepath.Join(out, f)) {
			t.Fatalf("%s missing", f)
		}
	}
	if _, err := loadPNG(filepath.Join(out, "ics-ics-baby-owa.png")); err != nil {
		t.Fatalf("owa png: %v", err)
	}
	sc := readSidecar(t, out)
	if sc["status"] != "ok" || sc["profile"] != "owa" || sc["emulation"] != true {
		t.Fatalf("sidecar %v", sc)
	}
	for _, line := range []string{"Wrote OWA view:", "Wrote OWA sidecar:", "Wrote OWA PNG:"} {
		if !strings.Contains(res.stdout, line) {
			t.Fatalf("stdout lacks %q: %s", line, res.stdout)
		}
	}
}

func TestCLIInviteViewSkipped(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--invite-view", "owa", fixtureDir+"20-reply-spoof.ics")
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	if sc := readSidecar(t, out); sc["status"] != "skipped" {
		t.Fatalf("sidecar %v", sc)
	}
	if fileExists(filepath.Join(out, "ics-ics-baby-owa.png")) {
		t.Fatal("OWA PNG written for skipped view")
	}
}

func TestCLIInviteViewUnknown(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--invite-view", "gmail", fixtureDir+"11-html-alt-desc.ics")
	if res.exit != 1 || !strings.Contains(res.stderr, "unknown invite view") {
		t.Fatalf("exit %d stderr %q", res.exit, res.stderr)
	}
}

func TestCLIInviteViewChromiumMissing(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--invite-view", "owa", "--chromium-bin", "/nonexistent", fixtureDir+"11-html-alt-desc.ics")
	if res.exit != 1 {
		t.Fatalf("exit = %d, want 1", res.exit)
	}
	for _, f := range []string{"ics-ics-baby-preview.png", "ics-ics-baby-owa.html", "ics-ics-baby-owa.json"} {
		if !fileExists(filepath.Join(out, f)) {
			t.Fatalf("%s missing", f)
		}
	}
	if fileExists(filepath.Join(out, "ics-ics-baby-owa.png")) {
		t.Fatal("OWA PNG exists despite failed render")
	}
	sc := readSidecar(t, out)
	if sc["status"] != "render_failed" || sc["error"] == "" || sc["error"] == nil {
		t.Fatalf("sidecar %v", sc)
	}
}

func TestCLIInviteViewNoBeacon(t *testing.T) {
	requireChromiumCLI(t)
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	body := `<p>Hi</p><img src="` + srv.URL + `/pixel.gif" width=1 height=1>` +
		`<div style="background:url(` + srv.URL + `/bg.png)">x</div>` +
		`<style>p{background:url(` + srv.URL + `/css.png)}</style>` +
		`<link rel=stylesheet href="` + srv.URL + `/s.css"><script>new Image().src="` + srv.URL + `/js"</script>`
	esc := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`).Replace(body)
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:beacon@test\r\n" +
		"DTSTART:20260218T100000Z\r\nDTEND:20260218T110000Z\r\nSUMMARY:Beacon\r\n" +
		"ORGANIZER;CN=X:mailto:x@example.test\r\nLOCATION:" + srv.URL + "/loc\r\n" +
		"X-MICROSOFT-SKYPETEAMSMEETINGURL:" + srv.URL + "/join\r\n" +
		"X-ALT-DESC;FMTTYPE=text/html:" + esc + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	dir := t.TempDir()
	in := filepath.Join(dir, "beacon.ics")
	if err := os.WriteFile(in, []byte(ics), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "o")
	res := runCLI(t, "--out", out, "--screenshot-engine", "chromium", "--invite-view", "owa", in)
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("render contacted the beacon server %d times", n)
	}
	sc := readSidecar(t, out)
	b, _ := json.Marshal(sc["remote_refs"])
	if !strings.Contains(string(b), host) {
		t.Fatalf("remote_refs do not name %s: %s", host, b)
	}
}

func TestCLIInviteViewOrder(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--screenshot-engine", "chromium", "--chromium-bin", "/nonexistent", "--invite-view", "owa", fixtureDir+"11-html-alt-desc.ics")
	if res.exit != 1 {
		t.Fatalf("exit = %d, want 1", res.exit)
	}
	for _, f := range []string{"ics-ics-baby-manifest.json", "ics-ics-baby-preview.html", "ics-ics-baby-owa.html", "ics-ics-baby-owa.json"} {
		if !fileExists(filepath.Join(out, f)) {
			t.Fatalf("%s missing after preview render failure", f)
		}
	}
	if strings.Contains(res.stdout, "Wrote PNG screenshot:") {
		t.Fatal("claimed a preview PNG that failed to render")
	}
}

func TestCLIFindingsJSON(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	res := runCLI(t, "--out", out, "--findings-json", fixtureDir+"09-combo-spoof-urgent.ics")
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	b, err := os.ReadFile(filepath.Join(out, "ics-ics-baby-findings.json"))
	if err != nil {
		t.Fatalf("findings file: %v", err)
	}
	var doc struct {
		Schema   string           `json:"schema_version"`
		SHA      string           `json:"input_sha256"`
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != "ics-ics-baby-findings/1" || len(doc.SHA) != 64 || len(doc.Findings) == 0 {
		t.Fatalf("doc = %+v", doc)
	}
	if !strings.Contains(res.stdout, "Wrote findings:") {
		t.Fatalf("stdout: %s", res.stdout)
	}

	out2 := filepath.Join(t.TempDir(), "o")
	if r := runCLI(t, "--out", out2, fixtureDir+"09-combo-spoof-urgent.ics"); r.exit != 0 {
		t.Fatalf("exit %d", r.exit)
	}
	if fileExists(filepath.Join(out2, "ics-ics-baby-findings.json")) {
		t.Fatal("findings written without --findings-json")
	}
}

// Review finding 6: a skipped rerun into the same --out must not leave the
// previous run's OWA PNG or inline images next to a "skipped" sidecar.
func TestCLIInviteViewNoStaleOutputs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	if err := os.MkdirAll(filepath.Join(out, "ics-ics-baby-inline"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ics-ics-baby-owa.png", "ics-ics-baby-inline/old.png"} {
		if err := os.WriteFile(filepath.Join(out, f), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := runCLI(t, "--out", out, "--invite-view", "owa", fixtureDir+"20-reply-spoof.ics")
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	for _, f := range []string{"ics-ics-baby-owa.png", "ics-ics-baby-inline"} {
		if fileExists(filepath.Join(out, f)) {
			t.Errorf("stale %s survived a skipped run", f)
		}
	}
}

func TestCLIMaxAttachments(t *testing.T) {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a@test\r\nSUMMARY:att\r\n")
	for i := 0; i < 3; i++ {
		b.WriteString("ATTACH;FMTTYPE=text/plain;ENCODING=BASE64;VALUE=BINARY:aGVsbG8=\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	dir := t.TempDir()
	in := filepath.Join(dir, "a.ics")
	if err := os.WriteFile(in, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "o")
	res := runCLI(t, "--out", out, "--max-attachments", "2", in)
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	ents, _ := os.ReadDir(filepath.Join(out, "ics-ics-baby-attachments"))
	if len(ents) != 2 || !strings.Contains(res.stderr, "attachment limit reached") {
		t.Fatalf("files=%d stderr=%q", len(ents), res.stderr)
	}
}

// The engine contract (kept with the blastbox engine) checks its detection
// codes against this list, so the code table stays the single source.
func TestCLIListFindingCodes(t *testing.T) {
	res := runCLI(t, "--list-finding-codes")
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	var codes map[string]string
	if err := json.Unmarshal([]byte(res.stdout), &codes); err != nil {
		t.Fatalf("stdout not JSON: %v: %s", err, res.stdout)
	}
	if codes["teams_url_hijack"] != "high" || codes["display_alarm"] != "info" || len(codes) < 40 {
		t.Fatalf("codes = %v", codes)
	}
}

// A requested findings file that cannot be written is a failed run, not a
// silent success with missing (or a previous run's) detections.
func TestCLIFindingsJSONWriteFailure(t *testing.T) {
	out := filepath.Join(t.TempDir(), "o")
	if err := os.MkdirAll(filepath.Join(out, "ics-ics-baby-findings.json", "blocker"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, "--out", out, "--findings-json", fixtureDir+"09-combo-spoof-urgent.ics")
	if res.exit != 1 || !strings.Contains(res.stderr, "failed to write findings") {
		t.Fatalf("exit %d stderr %q", res.exit, res.stderr)
	}
	if strings.Contains(res.stdout, "Wrote findings:") {
		t.Fatalf("stdout claims findings were written: %s", res.stdout)
	}
}

// A view taller than --chromium-max-height is cut; the sidecar says so.
func TestCLIInviteViewCaptureTruncated(t *testing.T) {
	requireChromiumCLI(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "tall.ics")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:t@test\r\nSUMMARY:Tall\r\n" +
		`X-ALT-DESC;FMTTYPE=text/html:<p>See below</p><div style="height:6000px"></div><p>PAY NOW</p>` + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if err := os.WriteFile(in, []byte(ics), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "o")
	res := runCLI(t, "--out", out, "--invite-view", "owa", "--chromium-max-height", "2000", in)
	if res.exit != 0 {
		t.Fatalf("exit %d: %s", res.exit, res.stderr)
	}
	sc := readSidecar(t, out)
	dropped, _ := sc["dropped"].([]any)
	found := false
	for _, d := range dropped {
		found = found || d == "capture_truncated"
	}
	if !found {
		t.Fatalf("sidecar dropped = %v, want capture_truncated", sc["dropped"])
	}
}
