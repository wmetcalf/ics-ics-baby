package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ics-ics-baby/internal/attach"
	"ics-ics-baby/internal/icsparse"
	"ics-ics-baby/internal/render"
	"ics-ics-baby/internal/webview"
)

// These are overridden by -ldflags during build
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, " ")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func (m multiFlag) Slice() []string {
	out := make([]string, len(m))
	copy(out, m)
	return out
}

// sanitizeOutputPath cleans and validates an output path without restricting to a base directory
func sanitizeOutputPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path cannot be empty")
	}

	// Clean the path to resolve . and .. elements
	cleaned := filepath.Clean(path)

	// Resolve to absolute path
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Check for null bytes (can cause issues on some systems)
	if strings.Contains(absPath, "\x00") {
		return "", errors.New("path contains null byte")
	}

	// Warn about suspicious patterns but don't block (users may have legitimate uses)
	// Just ensure the path is cleaned and absolute
	return absPath, nil
}

func main() {
	var outDir string
	var screenshot string
	var htmlOut string
	var download bool
	var maxAttachBytes int64
	var maxICSBytes int64
	var tzDefault string
	var startStr string
	var endStr string
	var style string
	var width int
	var view string
	var screenshotEngine string
	var wkhtmlBin string
	var wkhtmlWrapBin string
	var wkhtmlAllowNetwork bool
	var wkhtmlTimeout time.Duration
	var wkhtmlArgs multiFlag
	var wkhtmlAllowPath multiFlag
	var hideSecurityAlerts bool

	flag.StringVar(&outDir, "out", "out", "Output directory")
	flag.StringVar(&screenshot, "screenshot", "", "Output PNG path (default: OUT/preview.png)")
	flag.StringVar(&htmlOut, "html", "", "Output HTML path (default: OUT/preview.html)")
	flag.BoolVar(&download, "download-attachments", false, "Download URL attachments")
	flag.Int64Var(&maxAttachBytes, "max-attachment-bytes", 100<<20, "Maximum size to allow per attachment in bytes (default 100 MiB)")
	flag.Int64Var(&maxICSBytes, "max-ics-bytes", 500<<20, "Maximum size to allow for ICS file in bytes (default 500 MiB)")
	flag.StringVar(&tzDefault, "timezone", "", "Default timezone for naive dates (e.g., America/Chicago)")
	flag.StringVar(&startStr, "start", "", "Filter start date (YYYY-MM-DD) inclusive")
	flag.StringVar(&endStr, "end", "", "Filter end date (YYYY-MM-DD) exclusive")
	flag.StringVar(&style, "style", "light", "Render style: light or dark")
	flag.IntVar(&width, "width", 1200, "PNG width in pixels")
	flag.StringVar(&view, "view", "invite", "PNG view: invite or agenda")
	flag.StringVar(&screenshotEngine, "screenshot-engine", "go", "PNG renderer: go or wkhtml")
	flag.StringVar(&wkhtmlBin, "wkhtml-bin", "wkhtmltoimage", "Path to wkhtmltoimage binary")
	flag.StringVar(&wkhtmlWrapBin, "wkhtml-wrap", "wkhtml-wrap", "Path to wkhtml-wrap sandbox binary (use empty string to disable sandbox)")
	flag.BoolVar(&wkhtmlAllowNetwork, "wkhtml-allow-network", false, "Allow wkhtmltoimage to contact remote hosts (defaults to blocking via a dead proxy)")
	flag.DurationVar(&wkhtmlTimeout, "wkhtml-timeout", 45*time.Second, "Timeout for wkhtmltoimage execution")
	flag.Var(&wkhtmlArgs, "wkhtml-arg", "Additional argument passed straight to wkhtmltoimage (repeatable)")
	flag.Var(&wkhtmlAllowPath, "wkhtml-allow-path", "Allow wkhtmltoimage to read this path when local file access is otherwise disabled (repeatable)")
	flag.BoolVar(&hideSecurityAlerts, "hide-security-alerts", false, "Hide security alert warnings in HTML/PNG output (default: show alerts)")

	flag.Parse()

	// version banner
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" || a == "version" {
			fmt.Printf("ics-ics-baby %s (commit %s, built %s) %s\n", Version, Commit, Date, runtime.GOOS+"/"+runtime.GOARCH)
			return
		}
	}

	if download {
		fmt.Fprintln(os.Stderr, "warning: --download-attachments will contact remote hosts; enable only for trusted invites and networks.")
	}

	if flag.NArg() < 1 {
		fmt.Println("Usage: ics-ics-baby <file.ics> [options]")
		fmt.Println("\nCommon options:")
		fmt.Println("  --out DIR                       Output directory (default: out)")
		fmt.Println("  --html PATH                     Output HTML path (default: OUT/ics-ics-baby-preview.html)")
		fmt.Println("  --screenshot PATH               Output PNG path (default: OUT/ics-ics-baby-preview.png)")
		fmt.Println("  --view invite|agenda            PNG view mode (default: invite)")
		fmt.Println("  --style light|dark              Render style (default: light)")
		fmt.Println("  --width N                       PNG width in pixels (default: 1200)")
		fmt.Println("  --hide-security-alerts          Hide security warnings in HTML/PNG (default: show)")
		fmt.Println("\nFiltering options:")
		fmt.Println("  --timezone ZONE                 Default timezone for naive dates (e.g., America/Chicago)")
		fmt.Println("  --start YYYY-MM-DD              Filter start date (inclusive)")
		fmt.Println("  --end YYYY-MM-DD                Filter end date (exclusive)")
		fmt.Println("\nAttachment options:")
		fmt.Println("  --download-attachments          Download URL attachments (contacts remote hosts)")
		fmt.Println("  --max-attachment-bytes N        Maximum size per attachment in bytes (default: 100 MiB)")
		fmt.Println("  --max-ics-bytes N               Maximum ICS file size in bytes (default: 500 MiB)")
		fmt.Println("\nScreenshot engine options:")
		fmt.Println("  --screenshot-engine go|wkhtml   PNG renderer (default: go)")
		fmt.Println("  --wkhtml-bin PATH               Path to wkhtmltoimage binary (default: wkhtmltoimage)")
		fmt.Println("  --wkhtml-wrap PATH              Path to wkhtml-wrap sandbox binary (default: wkhtml-wrap)")
		fmt.Println("  --wkhtml-allow-network          Allow wkhtmltoimage network access (default: blocked)")
		fmt.Println("  --wkhtml-timeout DURATION       Timeout for wkhtmltoimage (default: 45s)")
		fmt.Println("  --wkhtml-arg ARG                Additional wkhtmltoimage argument (repeatable)")
		fmt.Println("  --wkhtml-allow-path PATH        Allow wkhtmltoimage to read this path (repeatable)")
		fmt.Println("\nExamples:")
		fmt.Println("  # Basic usage with defaults")
		fmt.Println("  ics-ics-baby meeting.ics")
		fmt.Println("")
		fmt.Println("  # Generate agenda view in dark mode")
		fmt.Println("  ics-ics-baby --view agenda --style dark calendar.ics")
		fmt.Println("")
		fmt.Println("  # Filter events for January 2025")
		fmt.Println("  ics-ics-baby --start 2025-01-01 --end 2025-02-01 events.ics")
		fmt.Println("")
		fmt.Println("  # Use wkhtmltoimage renderer with sandbox wrapper")
		fmt.Println("  ics-ics-baby --screenshot-engine wkhtml --wkhtml-wrap /usr/local/bin/wkhtml-wrap invite.ics")
		fmt.Println("")
		fmt.Println("  # Download attachments and specify timezone")
		fmt.Println("  ics-ics-baby --download-attachments --timezone America/New_York invite.ics")
		fmt.Println("")
		fmt.Println("  # Custom output paths and high-resolution PNG")
		fmt.Println("  ics-ics-baby --out ./output --screenshot ./invite.png --width 2400 meeting.ics")
		os.Exit(2)
	}
	icsPath := flag.Arg(0)

	// Sanitize input ICS path
	sanitizedICSPath, err := sanitizeOutputPath(icsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid input file path: %v\n", err)
		os.Exit(1)
	}
	icsPath = sanitizedICSPath

	// Sanitize output directory path
	sanitizedOutDir, err := sanitizeOutputPath(outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid output directory: %v\n", err)
		os.Exit(1)
	}
	outDir = sanitizedOutDir

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create out dir: %v\n", err)
		os.Exit(1)
	}

	cal, err := icsparse.ParseICSFile(icsPath, tzDefault, maxICSBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	var start, end *time.Time
	if startStr != "" {
		t, err := time.Parse("2006-01-02", startStr)
		if err == nil {
			start = &t
		}
	}
	if endStr != "" {
		t, err := time.Parse("2006-01-02", endStr)
		if err == nil {
			end = &t
		}
	}
	if start != nil || end != nil {
		cal.FilterRange(start, end)
	}

	if err := attach.ExtractAll(cal, outDir, download, maxAttachBytes); err != nil {
		fmt.Fprintf(os.Stderr, "attachment error: %v\n", err)
	}

	icsparse.PopulateDiscoveredURLs(cal)
	manifest := cal.Manifest()
	manifestPath := filepath.Join(outDir, "ics-ics-baby-manifest.json")
	if f, err := os.Create(manifestPath); err == nil {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		_ = enc.Encode(manifest)
		_ = f.Close()
	} else {
		fmt.Fprintf(os.Stderr, "failed to write manifest: %v\n", err)
	}

	if htmlOut == "" {
		htmlOut = filepath.Join(outDir, "ics-ics-baby-preview.html")
	}
	// Sanitize HTML output path
	sanitizedHTMLOut, err := sanitizeOutputPath(htmlOut)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid HTML output path: %v\n", err)
		os.Exit(1)
	}
	htmlOut = sanitizedHTMLOut

	showSecurityAlerts := !hideSecurityAlerts
	if err := webview.WriteInviteHTML(cal, htmlOut, style, showSecurityAlerts); err != nil {
		fmt.Fprintf(os.Stderr, "html render error: %v\n", err)
	}

	if screenshot == "" {
		screenshot = filepath.Join(outDir, "ics-ics-baby-preview.png")
	}
	// Sanitize screenshot output path
	sanitizedScreenshot, err := sanitizeOutputPath(screenshot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid screenshot output path: %v\n", err)
		os.Exit(1)
	}
	screenshot = sanitizedScreenshot

	switch view {
	case "invite":
		if screenshotEngine == "wkhtml" {
			opts := render.WKHTMLSettings{
				Bin:          wkhtmlBin,
				WrapperBin:   wkhtmlWrapBin,
				ExtraArgs:    wkhtmlArgs.Slice(),
				AllowNetwork: wkhtmlAllowNetwork,
				AllowedPaths: wkhtmlAllowPath.Slice(),
				Timeout:      wkhtmlTimeout,
			}
			if err := render.RenderInvitePNGWithWKHTML(htmlOut, screenshot, width, opts); err != nil {
				fmt.Fprintf(os.Stderr, "wkhtml render error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := render.RenderInvitePNG(cal, screenshot, width, style, showSecurityAlerts); err != nil {
				fmt.Fprintf(os.Stderr, "render error: %v\n", err)
				os.Exit(1)
			}
		}
	default:
		if err := render.RenderAgendaPNG(cal, screenshot, width, style); err != nil {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("Wrote manifest:", manifestPath)
	fmt.Println("Wrote HTML preview:", htmlOut)
	fmt.Println("Wrote PNG screenshot:", screenshot)
}
