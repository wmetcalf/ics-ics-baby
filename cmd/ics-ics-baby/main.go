package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		fmt.Println("Usage: ics-ics-baby <file.ics> [--out out] [--html out/preview.html] [--screenshot out/preview.png] [--view invite|agenda] [--download-attachments] [--timezone Zone] [--start YYYY-MM-DD] [--end YYYY-MM-DD] [--style light|dark] [--width 1200] [--max-ics-bytes N] [--max-attachment-bytes N]")
		os.Exit(2)
	}
	icsPath := flag.Arg(0)

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
	if err := webview.WriteInviteHTML(cal, htmlOut, style); err != nil {
		fmt.Fprintf(os.Stderr, "html render error: %v\n", err)
	}

	if screenshot == "" {
		screenshot = filepath.Join(outDir, "ics-ics-baby-preview.png")
	}
	switch view {
	case "invite":
		if err := render.RenderInvitePNG(cal, screenshot, width, style); err != nil {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
			os.Exit(1)
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
