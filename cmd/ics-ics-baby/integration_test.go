package main

import (
	"encoding/json"
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
