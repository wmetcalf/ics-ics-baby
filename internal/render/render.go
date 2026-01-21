package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"ics-ics-baby/internal/fonts"
	"ics-ics-baby/internal/icsparse"
)

type palette struct {
	Bg             color.Color
	Panel          color.Color
	Text           color.Color
	Muted          color.Color
	Edge           color.Color
	Accent         color.Color
	ChipBg         color.Color
	WarningBg      color.Color
	WarningBorder  color.Color
	WarningText    color.Color
	CriticalBg     color.Color
	CriticalBorder color.Color
}

var (
	hexColorPattern     = regexp.MustCompile(`^#(?i:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	bareHexColorPattern = regexp.MustCompile(`^(?i:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
	cssKeywordPattern   = regexp.MustCompile(`^(?i:[a-z]+(?:-[a-z]+)*)$`)
)

func sanitizeColorValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if cssKeywordPattern.MatchString(lower) {
		return lower
	}
	if strings.HasPrefix(trimmed, "#") && hexColorPattern.MatchString(trimmed) {
		return strings.ToLower(trimmed)
	}
	if bareHexColorPattern.MatchString(trimmed) {
		return "#" + strings.ToLower(trimmed)
	}
	return ""
}

func sanitizeColorPtr(p *string) string {
	if p == nil {
		return ""
	}
	return sanitizeColorValue(*p)
}

func parseHexColor(hex string) (color.Color, bool) {
	trimmed := strings.TrimPrefix(hex, "#")
	var r, g, b, a uint64
	var err error
	switch len(trimmed) {
	case 3:
		r, err = strconv.ParseUint(strings.Repeat(string(trimmed[0]), 2), 16, 8)
		if err != nil {
			return nil, false
		}
		g, err = strconv.ParseUint(strings.Repeat(string(trimmed[1]), 2), 16, 8)
		if err != nil {
			return nil, false
		}
		b, err = strconv.ParseUint(strings.Repeat(string(trimmed[2]), 2), 16, 8)
		if err != nil {
			return nil, false
		}
		a = 0xff
	case 6:
		r, err = strconv.ParseUint(trimmed[0:2], 16, 8)
		if err != nil {
			return nil, false
		}
		g, err = strconv.ParseUint(trimmed[2:4], 16, 8)
		if err != nil {
			return nil, false
		}
		b, err = strconv.ParseUint(trimmed[4:6], 16, 8)
		if err != nil {
			return nil, false
		}
		a = 0xff
	case 8:
		r, err = strconv.ParseUint(trimmed[0:2], 16, 8)
		if err != nil {
			return nil, false
		}
		g, err = strconv.ParseUint(trimmed[2:4], 16, 8)
		if err != nil {
			return nil, false
		}
		b, err = strconv.ParseUint(trimmed[4:6], 16, 8)
		if err != nil {
			return nil, false
		}
		a, err = strconv.ParseUint(trimmed[6:8], 16, 8)
		if err != nil {
			return nil, false
		}
	default:
		return nil, false
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}, true
}

func parseAccentColor(colorPtr *string, fallback color.Color) color.Color {
	if hex := sanitizeColorPtr(colorPtr); hex != "" {
		if parsed, ok := parseHexColor(hex); ok {
			return parsed
		}
	}
	return fallback
}

func getPalette(style string) palette {
	if style == "dark" {
		return palette{
			Bg:             color.RGBA{0x2b, 0x2b, 0x2b, 0xff}, // Dark gray background
			Panel:          color.RGBA{0x3a, 0x3a, 0x3a, 0xff}, // Slightly lighter panel
			Text:           color.RGBA{0xff, 0xff, 0xff, 0xff}, // White text
			Muted:          color.RGBA{0xb0, 0xb0, 0xb0, 0xff}, // Light gray
			Edge:           color.RGBA{0x55, 0x55, 0x55, 0xff}, // Border color
			Accent:         color.RGBA{0xff, 0x66, 0x00, 0xff}, // Orange accent
			ChipBg:         color.RGBA{0x45, 0x45, 0x45, 0xff}, // Chip background
			WarningBg:      color.RGBA{0x4a, 0x20, 0x20, 0xff}, // Dark warning background
			WarningBorder:  color.RGBA{0xff, 0x6b, 0x6b, 0xff}, // Warning border
			WarningText:    color.RGBA{0xff, 0x6b, 0x6b, 0xff}, // Warning text
			CriticalBg:     color.RGBA{0x3d, 0x1f, 0x1f, 0xff}, // Dark critical background
			CriticalBorder: color.RGBA{0xff, 0x44, 0x44, 0xff}, // Critical border
		}
	}
	return palette{
		Bg:             color.RGBA{0xf5, 0xf5, 0xf5, 0xff}, // Light gray background
		Panel:          color.RGBA{0xff, 0xff, 0xff, 0xff}, // White panel
		Text:           color.RGBA{0x1a, 0x1a, 0x1a, 0xff}, // Dark text
		Muted:          color.RGBA{0x66, 0x66, 0x66, 0xff}, // Gray text
		Edge:           color.RGBA{0xcc, 0xcc, 0xcc, 0xff}, // Border color
		Accent:         color.RGBA{0xff, 0x66, 0x00, 0xff}, // Orange accent
		ChipBg:         color.RGBA{0xe8, 0xe8, 0xe8, 0xff}, // Light chip background
		WarningBg:      color.RGBA{0xff, 0xf3, 0xcd, 0xff}, // Light warning background
		WarningBorder:  color.RGBA{0xff, 0xc1, 0x07, 0xff}, // Warning border (amber)
		WarningText:    color.RGBA{0xd3, 0x2f, 0x2f, 0xff}, // Warning text (red)
		CriticalBg:     color.RGBA{0xff, 0xeb, 0xee, 0xff}, // Light critical background
		CriticalBorder: color.RGBA{0xd3, 0x2f, 0x2f, 0xff}, // Critical border (red)
	}
}

func loadFontFace(path string, size float64) font.Face {
	// Try user-specified font path first
	if path != "" {
		if face, err := gg.LoadFontFace(path, size); err == nil {
			return face
		}
	}

	// Use embedded composite font set for wide Unicode and emoji coverage.
	if face, _ := fonts.NewFallbackFace(size); face != nil {
		return face
	}

	// Fallback to Go regular font if the embedded bundle fails for some reason
	goRegularFont, err := truetype.Parse(goregular.TTF)
	if err == nil && goRegularFont != nil {
		return truetype.NewFace(goRegularFont, &truetype.Options{
			Size:    size,
			Hinting: font.HintingFull,
		})
	}

	// Last resort
	return basicfont.Face7x13
}

// cleanText processes ICS text fields by unescaping and normalizing
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return strings.TrimSpace(s)
}

// formatDateTime formats a time in a human-readable way
func formatDateTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	// Format: "Monday, January 2, 2006 at 3:04 PM MST"
	if t.Location() != nil && t.Location() != time.UTC {
		return t.Format("Monday, January 2, 2006 at 3:04 PM MST")
	}
	return t.Format("Monday, January 2, 2006 at 15:04 UTC")
}

// wordWrapWithSmartURLs wraps text and breaks URLs at logical points
// Preserves explicit newlines in the text
func wordWrapWithSmartURLs(dc *gg.Context, text string, width float64) []string {
	var allLines []string

	// First split by newlines to preserve them
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		// Wrap each paragraph/line separately
		wrappedLines := wrapSingleLine(dc, para, width)
		allLines = append(allLines, wrappedLines...)
	}

	return allLines
}

// wrapSingleLine wraps a single line of text (no newlines)
func wrapSingleLine(dc *gg.Context, text string, width float64) []string {
	var lines []string
	var currentLine string

	words := strings.Fields(text)

	for _, word := range words {
		// Check if word is a URL
		isURL := strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://")

		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		w, _ := dc.MeasureString(testLine)

		if w <= width {
			// Fits on current line
			currentLine = testLine
		} else if isURL {
			// URL doesn't fit, need to break it intelligently
			if currentLine != "" {
				lines = append(lines, currentLine)
				currentLine = ""
			}

			// Break URL at logical points: /, ?, &, =
			urlParts := breakURL(word)
			for _, part := range urlParts {
				testLine := currentLine + part
				w, _ := dc.MeasureString(testLine)

				if w <= width {
					currentLine += part
				} else {
					if currentLine != "" {
						lines = append(lines, currentLine)
					}
					currentLine = part
				}
			}
		} else {
			// Regular word doesn't fit, start new line
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	// If the paragraph was empty, still add an empty line
	if len(lines) == 0 {
		lines = append(lines, "")
	}

	return lines
}

// breakURL splits a URL at logical break points for wrapping
func breakURL(url string) []string {
	var parts []string
	current := ""

	for i, ch := range url {
		current += string(ch)

		// Break after these characters
		if ch == '/' || ch == '?' || ch == '&' || ch == '=' {
			parts = append(parts, current)
			current = ""
		} else if i == len(url)-1 {
			// Last character
			if current != "" {
				parts = append(parts, current)
			}
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

func RenderAgendaPNG(cal *icsparse.CalendarInfo, outPath string, width int, style string) error {
	const pad = 22.0
	const cardPad = 18.0
	const eventPad = 12.0
	const lineH = 20.0
	const titleSize = 20.0
	const textSize = 14.0
	const smallSize = 12.0

	pal := getPalette(style)
	pal.Accent = parseAccentColor(cal.Color, pal.Accent)

	events := make([]icsparse.EventInfo, len(cal.Events))
	copy(events, cal.Events)
	sort.SliceStable(events, func(i, j int) bool { return lessByStart(events[i], events[j]) })

	h := int(2*pad + cardPad + 40)
	// Add space for calendar metadata if present
	metaLineCount := 0
	if cal.Calscale != nil && *cal.Calscale != "" {
		metaLineCount++
	}
	if cal.TimezoneID != nil && *cal.TimezoneID != "" {
		metaLineCount++
	}
	if len(cal.Timezones) > 0 {
		metaLineCount++
	}
	if metaLineCount > 0 {
		h += int(lineH * float64(metaLineCount))
	}
	for _, e := range events {
		cardBase := 90.0
		desc := ""
		if e.DescriptionHTML != nil && *e.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*e.DescriptionHTML)
		} else if e.Description != nil {
			desc = cleanText(*e.Description)
		}
		desc = strings.TrimSpace(desc)
		charsPerLine := float64(width-2*int(pad)-2*int(cardPad)) / 8.0
		lines := math.Ceil(float64(len([]rune(desc))) / charsPerLine)
		cardH := cardBase + lines*lineH + float64(len(e.Attachments))*18.0
		h += int(cardH + eventPad)
	}
	if len(cal.Availabilities) > 0 {
		h += int(lineH * 2)
	}
	for _, av := range cal.Availabilities {
		cardBase := 70.0
		desc := ""
		if av.DescriptionHTML != nil && *av.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*av.DescriptionHTML)
		} else if av.Description != nil {
			desc = cleanText(*av.Description)
		}
		desc = strings.TrimSpace(desc)
		descLines := math.Ceil(float64(len([]rune(desc))) / (float64(width-2*int(pad)-2*int(cardPad)) / 8.0))

		infoCount := 0.0
		if av.BusyType != nil && *av.BusyType != "" {
			infoCount++
		}
		if av.Start != nil {
			infoCount++
		}
		if av.End != nil {
			infoCount++
		}
		if av.Duration != nil && *av.Duration != "" {
			infoCount++
		}
		if av.Location != nil && *av.Location != "" {
			infoCount++
		}
		if len(av.Contacts) > 0 {
			infoCount++
		}
		if len(av.Categories) > 0 {
			infoCount++
		}
		if av.Organizer != nil && *av.Organizer != "" {
			infoCount++
		}
		if av.URL != nil && *av.URL != "" {
			infoCount++
		}
		for _, slot := range av.Available {
			infoCount++
			infoCount += float64(len(availabilityWindowDetailTexts(slot)))
		}

		cardH := cardBase + infoCount*lineH + descLines*lineH
		h += int(cardH + eventPad)
	}
	if len(cal.Journals) > 0 {
		h += int(lineH * 2)
	}
	for _, jn := range cal.Journals {
		cardBase := 90.0
		desc := ""
		if jn.DescriptionHTML != nil && *jn.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*jn.DescriptionHTML)
		} else if jn.Description != nil {
			desc = cleanText(*jn.Description)
		}
		desc = strings.TrimSpace(desc)
		descLines := math.Ceil(float64(len([]rune(desc))) / (float64(width-2*int(pad)-2*int(cardPad)) / 8.0))
		infoCount := 0.0
		if jn.DTStart != nil {
			infoCount++
		}
		if jn.Organizer != nil && *jn.Organizer != "" {
			infoCount++
		}
		if jn.Status != nil && *jn.Status != "" {
			infoCount++
		}
		if jn.Class != nil && *jn.Class != "" {
			infoCount++
		}
		if len(jn.Categories) > 0 {
			infoCount++
		}
		if len(jn.Contacts) > 0 {
			infoCount++
		}
		if len(jn.RelatedTo) > 0 {
			infoCount++
		}
		if jn.URL != nil && *jn.URL != "" {
			infoCount++
		}
		if jn.DateTimeStamp != nil {
			infoCount++
		}
		if jn.Created != nil {
			infoCount++
		}
		if jn.LastModified != nil {
			infoCount++
		}
		if jn.Recurrence != nil && (jn.Recurrence.RRule != nil || len(jn.Recurrence.RDates) > 0 || len(jn.Recurrence.ExDates) > 0) {
			infoCount++
		}
		if len(jn.Attendees) > 0 {
			infoCount++
		}
		if len(jn.Conferences) > 0 {
			infoCount++
		}
		if len(jn.Attachments) > 0 {
			infoCount++
		}
		if len(jn.Images) > 0 {
			infoCount++
		}
		cardH := cardBase + infoCount*lineH + descLines*lineH
		h += int(cardH + eventPad)
	}
	h += int(cardPad + pad)

	img := image.NewRGBA(image.Rect(0, 0, width, maxInt(h, 360)))
	draw.Draw(img, img.Bounds(), &image.Uniform{pal.Bg}, image.Point{}, draw.Src)

	dc := gg.NewContextForRGBA(img)

	panelX := pad
	panelY := pad
	panelW := float64(width) - 2*pad
	panelH := float64(h) - 2*pad
	dc.SetColor(pal.Panel)
	dc.DrawRoundedRectangle(panelX, panelY, panelW, panelH, 14)
	dc.Fill()
	dc.SetColor(pal.Edge)
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(panelX, panelY, panelW, panelH, 14)
	dc.Stroke()

	dc.SetColor(pal.Text)
	dc.SetFontFace(loadFontFace("", titleSize))
	title := "Calendar Preview"
	if cal.Name != nil && *cal.Name != "" {
		title = *cal.Name
	}
	dc.DrawStringAnchored(title, panelX+cardPad, panelY+cardPad+titleSize, 0, 0)

	y := panelY + cardPad + titleSize + 8

	// Display calendar metadata (scale and timezone) matching HTML format
	var calMeta []string
	if cal.Calscale != nil && *cal.Calscale != "" {
		calMeta = append(calMeta, "Calendar Scale: "+*cal.Calscale)
	}
	if cal.TimezoneID != nil && *cal.TimezoneID != "" {
		calMeta = append(calMeta, "Default TZ: "+*cal.TimezoneID)
	}
	// Add timezone definitions if present
	if len(cal.Timezones) > 0 {
		tzNames := make([]string, 0, len(cal.Timezones))
		for _, tz := range cal.Timezones {
			if tz.TZID != nil && *tz.TZID != "" {
				tzNames = append(tzNames, *tz.TZID)
			}
		}
		if len(tzNames) > 0 {
			calMeta = append(calMeta, "Timezones: "+strings.Join(tzNames, ", "))
		}
	}
	if len(calMeta) > 0 {
		dc.SetFontFace(loadFontFace("", smallSize))
		dc.SetColor(pal.Muted)
		for idx, line := range calMeta {
			dc.DrawStringAnchored(line, panelX+cardPad, y+smallSize+float64(idx)*lineH, 0, 0)
		}
		y += lineH * float64(len(calMeta))
	}

	for _, e := range events {
		cardX := panelX + cardPad
		cardW := panelW - 2*cardPad

		desc := ""
		if e.DescriptionHTML != nil && *e.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*e.DescriptionHTML)
		} else if e.Description != nil {
			desc = cleanText(*e.Description)
		}
		desc = strings.TrimSpace(desc)
		dc.SetFontFace(loadFontFace("", textSize))
		wrapWidth := cardW - 2*eventPad
		descLines := dc.WordWrap(desc, wrapWidth)
		descH := float64(len(descLines)) * lineH
		attH := 0.0
		if len(e.Attachments) > 0 {
			attH = float64((len(e.Attachments)+1)/2) * 24.0
		}

		var loc, org, uid string
		if e.Location != nil {
			loc = cleanText(*e.Location)
		}
		if e.Organizer != nil {
			org = cleanText(*e.Organizer)
		}
		if e.UID != nil {
			uid = strings.TrimSpace(*e.UID)
		}
		startS := fmtTime(e.DTStart)
		endS := fmtTime(e.DTEnd)
		metaLines := []string{fmt.Sprintf("Starts: %s   Ends: %s", startS, endS)}
		if uid != "" {
			metaLines = append(metaLines, "UID: "+uid)
		}
		if loc != "" {
			metaLines = append(metaLines, "Location: "+loc)
		}
		if org != "" {
			metaLines = append(metaLines, "Organizer: "+org)
		}

		cardH := 18.0 + lineH*(0.6+0.8*float64(len(metaLines))) + descH + attH + eventPad

		dc.SetColor(pal.Edge)
		dc.DrawRoundedRectangle(cardX, y, cardW, cardH, 10)
		dc.Fill()

		innerX := cardX + eventPad
		innerY := y + eventPad

		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize+2))
		dc.DrawStringAnchored(e.Summary, innerX, innerY+lineH, 0, 0)

		dc.SetFontFace(loadFontFace("", textSize))
		dc.SetColor(pal.Muted)
		metaBase := innerY + lineH*2.4
		metaStep := lineH * 0.8
		for idx, line := range metaLines {
			dc.DrawStringAnchored(line, innerX, metaBase+metaStep*float64(idx), 0, 0)
		}

		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize))
		textY := metaBase + metaStep*float64(len(metaLines)) + metaStep
		dc.DrawStringWrapped(desc, innerX, textY, 0, 0, wrapWidth, 1.5, gg.AlignLeft)

		if len(e.Attachments) > 0 {
			chipY := textY + descH + 8
			dc.SetFontFace(loadFontFace("", smallSize))
			x := innerX
			for _, a := range e.Attachments {
				label := chipLabel(a)
				w, h := dc.MeasureString(label)
				w += 16
				h += 8
				dc.SetColor(pal.ChipBg)
				dc.DrawRoundedRectangle(x, chipY, w, h, 10)
				dc.Fill()
				dc.SetColor(pal.Muted)
				dc.DrawStringAnchored(label, x+8, chipY+h*0.7, 0, 0.5)
				x += w + 8
				if x > innerX+wrapWidth {
					x = innerX
					chipY += h + 6
				}
			}
		}

		y += cardH + eventPad
	}

	if len(cal.Availabilities) > 0 {
		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize+2))
		dc.DrawStringAnchored("Published Availability", panelX+cardPad, y+lineH, 0, 0)
		y += lineH * 1.8
	}

	for _, av := range cal.Availabilities {
		cardX := panelX + cardPad
		cardW := panelW - 2*cardPad

		desc := ""
		if av.DescriptionHTML != nil && *av.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*av.DescriptionHTML)
		} else if av.Description != nil {
			desc = cleanText(*av.Description)
		}
		desc = strings.TrimSpace(desc)
		dc.SetFontFace(loadFontFace("", textSize))
		wrapWidth := cardW - 2*eventPad
		descLines := dc.WordWrap(desc, wrapWidth)
		descH := float64(len(descLines)) * lineH

		infoLines := availabilityInfoLines(av)
		infoH := float64(len(infoLines)) * lineH

		cardH := 18.0 + lineH*2 + infoH + descH + eventPad

		dc.SetColor(pal.Edge)
		dc.DrawRoundedRectangle(cardX, y, cardW, cardH, 10)
		dc.Fill()

		innerX := cardX + eventPad
		innerY := y + eventPad

		summary := "Availability Window"
		if av.Summary != nil && strings.TrimSpace(*av.Summary) != "" {
			summary = strings.TrimSpace(*av.Summary)
		}
		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize+2))
		dc.DrawStringAnchored(summary, innerX, innerY+lineH, 0, 0)

		dc.SetFontFace(loadFontFace("", textSize))
		dc.SetColor(pal.Muted)
		metaY := innerY + lineH*2.0
		for _, line := range infoLines {
			dc.DrawStringAnchored(line, innerX, metaY, 0, 0)
			metaY += lineH
		}

		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize))
		textY := innerY + lineH*2.0 + infoH + lineH*0.4
		dc.DrawStringWrapped(desc, innerX, textY, 0, 0, wrapWidth, 1.5, gg.AlignLeft)

		y += cardH + eventPad
	}

	if len(cal.Journals) > 0 {
		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize+2))
		dc.DrawStringAnchored("Journal Entries", panelX+cardPad, y+lineH, 0, 0)
		y += lineH * 1.8
	}

	for _, jn := range cal.Journals {
		cardX := panelX + cardPad
		cardW := panelW - 2*cardPad

		desc := ""
		if jn.DescriptionHTML != nil && *jn.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*jn.DescriptionHTML)
		} else if jn.Description != nil {
			desc = cleanText(*jn.Description)
		}
		desc = strings.TrimSpace(desc)
		dc.SetFontFace(loadFontFace("", textSize))
		wrapWidth := cardW - 2*eventPad
		descLines := dc.WordWrap(desc, wrapWidth)
		descH := float64(len(descLines)) * lineH

		infoLines := journalInfoLines(jn)
		infoH := float64(len(infoLines)) * lineH

		cardH := 18.0 + lineH*2 + infoH + descH + eventPad

		dc.SetColor(pal.Edge)
		dc.DrawRoundedRectangle(cardX, y, cardW, cardH, 10)
		dc.Fill()

		innerX := cardX + eventPad
		innerY := y + eventPad

		title := strings.TrimSpace(jn.Summary)
		if title == "" {
			title = "Journal Entry"
		}
		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize+2))
		dc.DrawStringAnchored(title, innerX, innerY+lineH, 0, 0)

		dc.SetFontFace(loadFontFace("", textSize))
		dc.SetColor(pal.Muted)
		metaY := innerY + lineH*2.0
		for _, line := range infoLines {
			dc.DrawStringAnchored(line, innerX, metaY, 0, 0)
			metaY += lineH
		}

		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize))
		textY := innerY + lineH*2.0 + infoH + lineH*0.4
		dc.DrawStringWrapped(desc, innerX, textY, 0, 0, wrapWidth, 1.5, gg.AlignLeft)

		y += cardH + eventPad
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return gg.SavePNG(outPath, img)
}

const (
	invitePad             = 30.0
	inviteHeaderPadX      = 20.0
	inviteHeaderPadY      = 18.0
	inviteHeaderMargin    = 20.0
	inviteEventSpacing    = 30.0
	inviteIconSize        = 20.0
	inviteIconGap         = 15.0
	inviteLabelWidth      = 110.0
	inviteLabelPadding    = 10.0
	inviteMinValueWidth   = 140.0
	inviteLabelMeasurePad = 6.0
	inviteHeaderLineH     = 22.0
	inviteLineHeight      = 20.0
	inviteBaselineAdjust  = 6.0
	inviteDescPadding     = 15.0
	inviteFieldSpacing    = 8.0
	inviteSectionSpacing  = 20.0
	inviteTitleSize       = 16.0
	inviteLabelSize       = 13.0
	inviteTextSize        = 13.0
	inviteMinHeight       = 500
)

type inviteField struct {
	label  string
	lines  []string
	height float64
}

type inviteEventLayout struct {
	summaryLines   []string
	fields         []inviteField
	descLines      []string
	headerHeight   float64
	fieldsHeight   float64
	descHeight     float64
	totalHeight    float64
	hasDescription bool
	iconColor      string
	labelWidth     float64
}

// hasAutoprocessingSignals checks if there are any signals to display
func hasAutoprocessingSignals(signals *icsparse.AutoprocessingSignals) bool {
	if signals == nil {
		return false
	}
	return len(signals.SuspiciousPatterns) > 0 ||
		len(signals.MicrosoftHeaders) > 0 ||
		len(signals.GoogleHeaders) > 0 ||
		signals.HasHighSequence ||
		signals.HasSentByMismatch
}

// isCriticalSignal checks if the signals indicate a critical threat
func isCriticalSignal(signals *icsparse.AutoprocessingSignals) bool {
	return signals != nil && signals.HasHighSequence
}

// hasAnySecuritySignals checks if calendar or any events have signals
func hasAnySecuritySignals(cal *icsparse.CalendarInfo) bool {
	if hasAutoprocessingSignals(cal.AutoprocessingSignals) {
		return true
	}
	for _, ev := range cal.Events {
		if hasAutoprocessingSignals(ev.AutoprocessingSignals) {
			return true
		}
	}
	return false
}

// isAnyCritical checks if any signals are critical
func isAnyCritical(cal *icsparse.CalendarInfo) bool {
	if isCriticalSignal(cal.AutoprocessingSignals) {
		return true
	}
	for _, ev := range cal.Events {
		if isCriticalSignal(ev.AutoprocessingSignals) {
			return true
		}
	}
	return false
}

// calculateAllSecurityAlertsHeight estimates height for all alerts (calendar + events)
func calculateAllSecurityAlertsHeight(cal *icsparse.CalendarInfo) float64 {
	lineCount := 1 // Title line

	// Calendar-level signals
	if cal.AutoprocessingSignals != nil {
		signals := cal.AutoprocessingSignals
		if len(signals.SuspiciousPatterns) > 0 || len(signals.MicrosoftHeaders) > 0 || len(signals.GoogleHeaders) > 0 {
			lineCount++ // "Calendar-Level Signals:" label
			lineCount += len(signals.SuspiciousPatterns)
			lineCount += len(signals.MicrosoftHeaders)
			lineCount += len(signals.GoogleHeaders)
		}
	}

	// Event-level signals
	for _, ev := range cal.Events {
		if hasAutoprocessingSignals(ev.AutoprocessingSignals) {
			lineCount++ // "Event: <summary>" label
			signals := ev.AutoprocessingSignals
			lineCount += len(signals.SuspiciousPatterns)
			lineCount += len(signals.MicrosoftHeaders)
			lineCount += len(signals.GoogleHeaders)
			if signals.OrganizerDetails != nil {
				lineCount += 2 // organizer info typically 2 lines
			}
		}
	}

	return inviteLineHeight*float64(lineCount) + inviteDescPadding*3
}

// renderAllSecurityAlerts draws combined security warning box for all signals
func renderAllSecurityAlerts(dc *gg.Context, cal *icsparse.CalendarInfo, x, y, contentWidth float64, pal palette, textFace font.Face) float64 {
	if !hasAnySecuritySignals(cal) {
		return y
	}

	critical := isAnyCritical(cal)
	bgColor := pal.WarningBg
	borderColor := pal.WarningBorder
	titleColor := pal.WarningText
	if critical {
		bgColor = pal.CriticalBg
		borderColor = pal.CriticalBorder
		titleColor = pal.CriticalBorder
	}

	// Calculate box height
	boxHeight := calculateAllSecurityAlertsHeight(cal)

	// Draw background
	dc.SetColor(bgColor)
	dc.DrawRoundedRectangle(x, y, contentWidth, boxHeight, 8)
	dc.Fill()

	// Draw border
	dc.SetColor(borderColor)
	dc.SetLineWidth(2)
	dc.DrawRoundedRectangle(x, y, contentWidth, boxHeight, 8)
	dc.Stroke()

	// Draw title
	dc.SetFontFace(textFace)
	dc.SetColor(titleColor)
	title := "⚠️ Security Alerts"
	if critical {
		title += " [CRITICAL]"
	} else {
		title += " [WARNING]"
	}
	textY := y + inviteDescPadding + inviteLineHeight - inviteBaselineAdjust
	dc.DrawString(title, x+inviteDescPadding, textY)
	textY += inviteLineHeight

	// Draw calendar-level signals
	if hasAutoprocessingSignals(cal.AutoprocessingSignals) {
		signals := cal.AutoprocessingSignals
		dc.SetColor(pal.Muted)
		dc.DrawString("Calendar-Level Signals:", x+inviteDescPadding, textY)
		textY += inviteLineHeight

		dc.SetColor(pal.Text)
		for _, pattern := range signals.SuspiciousPatterns {
			dc.DrawString("• "+pattern, x+inviteDescPadding+10, textY)
			textY += inviteLineHeight
		}
		for k, v := range signals.MicrosoftHeaders {
			dc.DrawString("  "+k+": "+v, x+inviteDescPadding+10, textY)
			textY += inviteLineHeight
		}
		for k, v := range signals.GoogleHeaders {
			dc.DrawString("  "+k+": "+v, x+inviteDescPadding+10, textY)
			textY += inviteLineHeight
		}
	}

	// Draw event-level signals
	for _, ev := range cal.Events {
		if hasAutoprocessingSignals(ev.AutoprocessingSignals) {
			signals := ev.AutoprocessingSignals
			dc.SetColor(pal.Muted)
			eventLabel := "Event: " + ev.Summary
			if len(eventLabel) > 60 {
				eventLabel = eventLabel[:57] + "..."
			}
			dc.DrawString(eventLabel, x+inviteDescPadding, textY)
			textY += inviteLineHeight

			dc.SetColor(pal.Text)
			for _, pattern := range signals.SuspiciousPatterns {
				dc.DrawString("• "+pattern, x+inviteDescPadding+10, textY)
				textY += inviteLineHeight
			}
			if signals.OrganizerDetails != nil {
				dc.DrawString("  Organizer: "+signals.OrganizerDetails.Value, x+inviteDescPadding+10, textY)
				textY += inviteLineHeight
			}
			for k, v := range signals.MicrosoftHeaders {
				dc.DrawString("  "+k+": "+v, x+inviteDescPadding+10, textY)
				textY += inviteLineHeight
			}
			for k, v := range signals.GoogleHeaders {
				dc.DrawString("  "+k+": "+v, x+inviteDescPadding+10, textY)
				textY += inviteLineHeight
			}
		}
	}

	return y + boxHeight
}

func RenderInvitePNG(cal *icsparse.CalendarInfo, outPath string, width int, style string, showSecurityAlerts bool) error {
	pal := getPalette(style)
	pal.Accent = parseAccentColor(cal.Color, pal.Accent)

	layouts, _ := buildInviteLayouts(cal.Events, width)
	if len(cal.Events) == 0 && len(cal.Availabilities) > 0 {
		layouts = nil
	}
	availLayouts := buildAvailabilityLayouts(cal.Availabilities, width)
	layouts = append(layouts, availLayouts...)
	journalLayouts := buildJournalLayouts(cal.Journals, width)
	layouts = append(layouts, journalLayouts...)
	totalHeight := calculateLayoutsHeight(layouts)

	// Calculate security alert height (combined for calendar + all events)
	securityAlertHeight := 0.0
	if showSecurityAlerts && hasAnySecuritySignals(cal) {
		securityAlertHeight = calculateAllSecurityAlertsHeight(cal)
		totalHeight += securityAlertHeight + inviteHeaderMargin
	}

	// Add height for calendar metadata section if needed
	metaLineCount := 0
	if cal.Calscale != nil && *cal.Calscale != "" {
		metaLineCount++
	}
	if cal.TimezoneID != nil && *cal.TimezoneID != "" {
		metaLineCount++
	}
	if len(cal.Timezones) > 0 {
		metaLineCount++
	}
	if metaLineCount > 0 {
		metaHeight := inviteLineHeight*float64(metaLineCount) + inviteDescPadding*2
		totalHeight += metaHeight + inviteHeaderMargin
	}

	height := int(math.Ceil(math.Max(float64(inviteMinHeight), totalHeight)))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{pal.Bg}, image.Point{}, draw.Src)

	dc := gg.NewContextForRGBA(img)
	titleFace := loadFontFace("", inviteTitleSize)
	labelFace := loadFontFace("", inviteLabelSize)
	textFace := loadFontFace("", inviteTextSize)

	contentWidth := float64(width) - 2*invitePad
	headerTextX := invitePad + inviteHeaderPadX + inviteIconSize + inviteIconGap

	y := invitePad

	// Render all security alerts first if present (combined calendar + events)
	if showSecurityAlerts && hasAnySecuritySignals(cal) {
		y = renderAllSecurityAlerts(dc, cal, invitePad, y, contentWidth, pal, textFace)
		y += inviteHeaderMargin
	}

	// Display calendar metadata (scale and timezone) if available, matching HTML format
	var calMeta []string
	if cal.Calscale != nil && *cal.Calscale != "" {
		calMeta = append(calMeta, "Calendar Scale: "+*cal.Calscale)
	}
	if cal.TimezoneID != nil && *cal.TimezoneID != "" {
		calMeta = append(calMeta, "Default TZ: "+*cal.TimezoneID)
	}
	// Add timezone definitions if present
	if len(cal.Timezones) > 0 {
		tzNames := make([]string, 0, len(cal.Timezones))
		for _, tz := range cal.Timezones {
			if tz.TZID != nil && *tz.TZID != "" {
				tzNames = append(tzNames, *tz.TZID)
			}
		}
		if len(tzNames) > 0 {
			calMeta = append(calMeta, "Timezones: "+strings.Join(tzNames, ", "))
		}
	}
	if len(calMeta) > 0 {
		metaHeight := inviteLineHeight*float64(len(calMeta)) + inviteDescPadding*2
		dc.SetColor(pal.Panel)
		dc.DrawRoundedRectangle(invitePad, y, contentWidth, metaHeight, 6)
		dc.Fill()
		dc.SetColor(pal.Edge)
		dc.SetLineWidth(1.0)
		dc.DrawRoundedRectangle(invitePad, y, contentWidth, metaHeight, 6)
		dc.Stroke()

		dc.SetFontFace(textFace)
		dc.SetColor(pal.Muted)
		textX := invitePad + inviteDescPadding
		textY := y + inviteDescPadding + inviteLineHeight - inviteBaselineAdjust
		for _, line := range calMeta {
			dc.DrawString(line, textX, textY)
			textY += inviteLineHeight
		}
		y += metaHeight + inviteHeaderMargin
	}

	for i, layout := range layouts {
		// Header container
		dc.SetColor(pal.Panel)
		dc.DrawRoundedRectangle(invitePad, y, contentWidth, layout.headerHeight, 8)
		dc.Fill()

		// Accent square
		iconY := y + (layout.headerHeight-inviteIconSize)/2
		iconColor := pal.Accent
		if layout.iconColor != "" {
			if parsed, ok := parseHexColor(layout.iconColor); ok {
				iconColor = parsed
			}
		}
		dc.SetColor(iconColor)
		dc.DrawRoundedRectangle(invitePad+inviteHeaderPadX, iconY, inviteIconSize, inviteIconSize, 4)
		dc.Fill()

		// Summary text
		dc.SetColor(pal.Text)
		dc.SetFontFace(titleFace)
		textY := y + inviteHeaderPadY + inviteTitleSize
		for _, line := range layout.summaryLines {
			dc.DrawString(line, headerTextX, textY)
			textY += inviteHeaderLineH
		}

		y += layout.headerHeight + inviteHeaderMargin

		// Detail fields
		if len(layout.fields) > 0 {
			currentY := y
			for idx, field := range layout.fields {
				drawInviteField(dc, field, invitePad, currentY, pal, labelFace, textFace, layout.labelWidth)
				currentY += field.height
				if idx < len(layout.fields)-1 {
					currentY += inviteFieldSpacing
				}
			}
			y = currentY
		}

		// Description block
		if layout.hasDescription && len(layout.descLines) > 0 {
			if len(layout.fields) > 0 {
				y += inviteSectionSpacing
			}
			dc.SetColor(pal.Panel)
			dc.DrawRoundedRectangle(invitePad, y, contentWidth, layout.descHeight, 6)
			dc.Fill()
			dc.SetColor(pal.Edge)
			dc.SetLineWidth(1.5)
			dc.DrawRoundedRectangle(invitePad, y, contentWidth, layout.descHeight, 6)
			dc.Stroke()

			dc.SetFontFace(textFace)
			dc.SetColor(pal.Text)
			textX := invitePad + inviteDescPadding
			descY := y + inviteDescPadding + inviteLineHeight - inviteBaselineAdjust
			for _, line := range layout.descLines {
				if strings.TrimSpace(line) != "" {
					dc.DrawString(line, textX, descY)
				}
				descY += inviteLineHeight
			}

			y += layout.descHeight
		}

		if i < len(layouts)-1 {
			y += inviteEventSpacing
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return gg.SavePNG(outPath, img)
}

func buildInviteLayouts(events []icsparse.EventInfo, width int) ([]inviteEventLayout, float64) {
	contentWidth := float64(width) - 2*invitePad
	if contentWidth < 200 {
		contentWidth = 200
	}
	descWidth := contentWidth - inviteDescPadding*2
	if descWidth < 140 {
		descWidth = 140
	}
	headerTextWidth := contentWidth - (inviteHeaderPadX*2 + inviteIconSize + inviteIconGap)
	if headerTextWidth < 140 {
		headerTextWidth = 140
	}

	ctx := gg.NewContext(width, 100)
	titleFace := loadFontFace("", inviteTitleSize)
	textFace := loadFontFace("", inviteTextSize)
	labelFace := loadFontFace("", inviteLabelSize)

	layouts := make([]inviteEventLayout, 0, maxInt(len(events), 1))

	if len(events) == 0 {
		ctx.SetFontFace(titleFace)
		summaryLines := ctx.WordWrap("No events to display", headerTextWidth)
		if len(summaryLines) == 0 {
			summaryLines = []string{"No events to display"}
		}
		headerHeight := math.Max(inviteIconSize, inviteHeaderLineH*float64(len(summaryLines))) + inviteHeaderPadY*2
		layout := inviteEventLayout{
			summaryLines: summaryLines,
			headerHeight: headerHeight,
			totalHeight:  headerHeight + inviteHeaderMargin,
		}
		layouts = append(layouts, layout)
	} else {
		for _, e := range events {
			var layout inviteEventLayout

			summary := strings.TrimSpace(e.Summary)
			if summary == "" {
				summary = "Untitled Event"
			}
			ctx.SetFontFace(titleFace)
			layout.summaryLines = ctx.WordWrap(summary, headerTextWidth)
			if len(layout.summaryLines) == 0 {
				layout.summaryLines = []string{summary}
			}
			titleHeight := inviteHeaderLineH * float64(len(layout.summaryLines))
			layout.headerHeight = math.Max(inviteIconSize, titleHeight) + inviteHeaderPadY*2
			layout.iconColor = sanitizeColorPtr(e.Color)

			baseValueWidth := contentWidth - inviteLabelWidth - inviteLabelPadding
			if baseValueWidth < inviteMinValueWidth {
				baseValueWidth = inviteMinValueWidth
			}

			buildFields := func(w float64) []inviteField {
				ctx.SetFontFace(textFace)
				addField := func(label string, lines []string) []inviteField {
					filtered := filterLines(lines, false)
					if len(filtered) == 0 {
						return nil
					}
					height := inviteLineHeight * float64(len(filtered))
					if height < inviteLineHeight {
						height = inviteLineHeight
					}
					return []inviteField{{label: label, lines: filtered, height: height}}
				}

				fields := make([]inviteField, 0, 20)
				if e.UID != nil && strings.TrimSpace(*e.UID) != "" {
					fields = append(fields, addField("UID:", []string{strings.TrimSpace(*e.UID)})...)
				}
				if e.DTStart != nil {
					fields = append(fields, addField("Start Date:", []string{formatDateTime(e.DTStart)})...)
				}
				if e.DTEnd != nil {
					fields = append(fields, addField("End Date:", []string{formatDateTime(e.DTEnd)})...)
				}
				if e.Location != nil {
					loc := cleanText(*e.Location)
					if loc != "" {
						fields = append(fields, addField("Location:", wordWrapWithSmartURLs(ctx, loc, w))...)
					}
				}
				if geo := geoLines(ctx, e.Geo, w); len(geo) > 0 {
					fields = append(fields, addField("Coordinates:", geo)...)
				}
				if e.Organizer != nil {
					org := cleanText(*e.Organizer)
					if org != "" {
						fields = append(fields, addField("Organizer:", wordWrapWithSmartURLs(ctx, org, w))...)
					}
				}
				if contacts := bulletListLines(ctx, e.Contacts, w); len(contacts) > 0 {
					fields = append(fields, addField("Contacts:", contacts)...)
				}
				if e.Status != nil && *e.Status != "" {
					fields = append(fields, addField("Status:", []string{*e.Status})...)
				}
				if e.Transparency != nil && *e.Transparency != "" {
					fields = append(fields, addField("Transparency:", []string{*e.Transparency})...)
				}
				if e.Priority != nil {
					fields = append(fields, addField("Priority:", []string{fmt.Sprintf("%d", *e.Priority)})...)
				}
				if e.Class != nil && *e.Class != "" {
					fields = append(fields, addField("Class:", []string{*e.Class})...)
				}
				if e.Sequence != nil {
					fields = append(fields, addField("Sequence:", []string{fmt.Sprintf("%d", *e.Sequence)})...)
				}
				if hex := sanitizeColorPtr(e.Color); hex != "" {
					fields = append(fields, addField("Color:", []string{hex})...)
				}
				if e.Duration != nil && *e.Duration != "" {
					fields = append(fields, addField("Duration:", []string{*e.Duration})...)
				}
				if e.URL != nil && *e.URL != "" {
					fields = append(fields, addField("URL:", wordWrapWithSmartURLs(ctx, *e.URL, w))...)
				}
				if urls := urlListLines(ctx, e.DiscoveredURLs, w); len(urls) > 0 {
					fields = append(fields, addField("Referenced URLs:", urls)...)
				}
				if len(e.Categories) > 0 {
					joined := strings.Join(e.Categories, ", ")
					fields = append(fields, addField("Categories:", wordWrapWithSmartURLs(ctx, joined, w))...)
				}
				if len(e.Resources) > 0 {
					joined := strings.Join(e.Resources, ", ")
					fields = append(fields, addField("Resources:", wordWrapWithSmartURLs(ctx, joined, w))...)
				}
				if comments := bulletListLines(ctx, e.Comments, w); len(comments) > 0 {
					fields = append(fields, addField("Comments:", comments)...)
				}
				if related := bulletListLines(ctx, e.RelatedTo, w); len(related) > 0 {
					fields = append(fields, addField("Related To:", related)...)
				}
				if statuses := requestStatusLines(ctx, e.RequestStatuses, w); len(statuses) > 0 {
					fields = append(fields, addField("Request Status:", statuses)...)
				}
				if images := imageLines(ctx, e.Images, w); len(images) > 0 {
					fields = append(fields, addField("Images:", images)...)
				}
				if conf := conferenceLines(ctx, e.Conferences, w); len(conf) > 0 {
					fields = append(fields, addField("Conference:", conf)...)
				}
				if rec := recurrenceLines(ctx, e.Recurrence, w); len(rec) > 0 {
					fields = append(fields, addField("Recurrence:", rec)...)
				}
				if e.Created != nil {
					fields = append(fields, addField("Created:", []string{formatDateTime(e.Created)})...)
				}
				if e.LastModified != nil {
					fields = append(fields, addField("Last Modified:", []string{formatDateTime(e.LastModified)})...)
				}
				if e.DateTimeStamp != nil {
					fields = append(fields, addField("Timestamp:", []string{formatDateTime(e.DateTimeStamp)})...)
				}

				if lines := attendeeLines(ctx, e.Attendees, w); len(lines) > 0 {
					fields = append(fields, addField("Attendees:", lines)...)
				}

				if len(e.Attachments) > 0 {
					entries := make([]string, 0, len(e.Attachments))
					for _, att := range e.Attachments {
						entries = append(entries, "• "+chipLabel(att))
					}
					fields = append(fields, addField("Attachments:", wrapListLines(ctx, entries, w))...)
				}

				if alarms := alarmSummaryLines(ctx, e.Alarms, w); len(alarms) > 0 {
					fields = append(fields, addField("Alarms:", alarms)...)
				}

				return fields
			}

			fields := buildFields(baseValueWidth)
			labelWidth := measureLabelColumnWidth(ctx, labelFace, fields, contentWidth)
			actualValueWidth := contentWidth - labelWidth - inviteLabelPadding
			if actualValueWidth < inviteMinValueWidth {
				actualValueWidth = inviteMinValueWidth
				labelWidth = contentWidth - actualValueWidth - inviteLabelPadding
			}
			if math.Abs(actualValueWidth-baseValueWidth) > 0.1 {
				fields = buildFields(actualValueWidth)
			}

			layout.fields = fields
			layout.labelWidth = labelWidth
			if len(fields) > 0 {
				height := 0.0
				for idx, f := range fields {
					height += f.height
					if idx < len(fields)-1 {
						height += inviteFieldSpacing
					}
				}
				layout.fieldsHeight = height
			}

			desc := ""
			if e.DescriptionHTML != nil && *e.DescriptionHTML != "" {
				desc = icsparse.PlainTextFromHTML(*e.DescriptionHTML)
			} else if e.Description != nil {
				desc = cleanText(*e.Description)
			}
			if strings.TrimSpace(desc) != "" {
				descLines := wordWrapWithSmartURLs(ctx, desc, descWidth)
				layout.descLines = filterLines(descLines, true)
				if len(layout.descLines) > 0 {
					layout.descHeight = inviteDescPadding*2 + inviteLineHeight*float64(len(layout.descLines))
					layout.hasDescription = true
				}
			}

			height := layout.headerHeight + inviteHeaderMargin
			if len(layout.fields) > 0 {
				height += layout.fieldsHeight
				if layout.hasDescription {
					height += inviteSectionSpacing
				}
			}
			if layout.hasDescription {
				height += layout.descHeight
			}
			layout.totalHeight = height
			layouts = append(layouts, layout)
		}
	}

	total := invitePad
	for i, layout := range layouts {
		total += layout.totalHeight
		if i < len(layouts)-1 {
			total += inviteEventSpacing
		}
	}
	total += invitePad

	return layouts, total
}

func buildAvailabilityLayouts(avails []icsparse.AvailabilityInfo, width int) []inviteEventLayout {
	if len(avails) == 0 {
		return nil
	}
	contentWidth := float64(width) - 2*invitePad
	if contentWidth < 200 {
		contentWidth = 200
	}
	descWidth := contentWidth - inviteDescPadding*2
	if descWidth < 140 {
		descWidth = 140
	}
	headerTextWidth := contentWidth - (inviteHeaderPadX*2 + inviteIconSize + inviteIconGap)
	if headerTextWidth < 140 {
		headerTextWidth = 140
	}

	ctx := gg.NewContext(width, 100)
	titleFace := loadFontFace("", inviteTitleSize)
	textFace := loadFontFace("", inviteTextSize)
	labelFace := loadFontFace("", inviteLabelSize)

	layouts := make([]inviteEventLayout, 0, len(avails))

	for _, av := range avails {
		var layout inviteEventLayout
		summary := "Availability Window"
		if av.Summary != nil && strings.TrimSpace(*av.Summary) != "" {
			summary = strings.TrimSpace(*av.Summary)
		}
		ctx.SetFontFace(titleFace)
		layout.summaryLines = ctx.WordWrap(summary, headerTextWidth)
		if len(layout.summaryLines) == 0 {
			layout.summaryLines = []string{summary}
		}
		titleHeight := inviteHeaderLineH * float64(len(layout.summaryLines))
		layout.headerHeight = math.Max(inviteIconSize, titleHeight) + inviteHeaderPadY*2

		baseValueWidth := contentWidth - inviteLabelWidth - inviteLabelPadding
		if baseValueWidth < inviteMinValueWidth {
			baseValueWidth = inviteMinValueWidth
		}

		buildFields := func(w float64) []inviteField {
			ctx.SetFontFace(textFace)
			addField := func(label string, lines []string) []inviteField {
				filtered := filterLines(lines, false)
				if len(filtered) == 0 {
					return nil
				}
				height := inviteLineHeight * float64(len(filtered))
				if height < inviteLineHeight {
					height = inviteLineHeight
				}
				return []inviteField{{label: label, lines: filtered, height: height}}
			}

			fields := make([]inviteField, 0, 16)
			if av.UID != nil && strings.TrimSpace(*av.UID) != "" {
				fields = append(fields, addField("UID:", []string{strings.TrimSpace(*av.UID)})...)
			}
			if av.BusyType != nil && *av.BusyType != "" {
				fields = append(fields, addField("Busy Type:", []string{*av.BusyType})...)
			}
			if av.Start != nil {
				fields = append(fields, addField("Start:", []string{formatDateTime(av.Start)})...)
			}
			if av.End != nil {
				fields = append(fields, addField("End:", []string{formatDateTime(av.End)})...)
			}
			if av.Duration != nil && *av.Duration != "" {
				fields = append(fields, addField("Duration:", []string{*av.Duration})...)
			}
			if av.Organizer != nil && *av.Organizer != "" {
				fields = append(fields, addField("Organizer:", wordWrapWithSmartURLs(ctx, *av.Organizer, w))...)
			}
			if av.Location != nil && *av.Location != "" {
				fields = append(fields, addField("Location:", wordWrapWithSmartURLs(ctx, *av.Location, w))...)
			}
			if av.URL != nil && *av.URL != "" {
				fields = append(fields, addField("URL:", wordWrapWithSmartURLs(ctx, *av.URL, w))...)
			}
			if contacts := bulletListLines(ctx, av.Contacts, w); len(contacts) > 0 {
				fields = append(fields, addField("Contacts:", contacts)...)
			}
			if len(av.Categories) > 0 {
				fields = append(fields, addField("Categories:", wordWrapWithSmartURLs(ctx, strings.Join(av.Categories, ", "), w))...)
			}
			if av.Priority != nil {
				fields = append(fields, addField("Priority:", []string{fmt.Sprintf("%d", *av.Priority)})...)
			}
			if av.Sequence != nil {
				fields = append(fields, addField("Sequence:", []string{fmt.Sprintf("%d", *av.Sequence)})...)
			}
			if av.Created != nil {
				fields = append(fields, addField("Created:", []string{formatDateTime(av.Created)})...)
			}
			if av.LastModified != nil {
				fields = append(fields, addField("Last Modified:", []string{formatDateTime(av.LastModified)})...)
			}
			if av.DateTimeStamp != nil {
				fields = append(fields, addField("Timestamp:", []string{formatDateTime(av.DateTimeStamp)})...)
			}
			if slots := availabilityWindowLines(ctx, av.Available, w); len(slots) > 0 {
				fields = append(fields, addField("Slots:", slots)...)
			}
			return fields
		}

		fields := buildFields(baseValueWidth)
		labelWidth := measureLabelColumnWidth(ctx, labelFace, fields, contentWidth)
		actualValueWidth := contentWidth - labelWidth - inviteLabelPadding
		if actualValueWidth < inviteMinValueWidth {
			actualValueWidth = inviteMinValueWidth
			labelWidth = contentWidth - actualValueWidth - inviteLabelPadding
		}
		if math.Abs(actualValueWidth-baseValueWidth) > 0.1 {
			fields = buildFields(actualValueWidth)
		}

		layout.fields = fields
		layout.labelWidth = labelWidth
		if len(fields) > 0 {
			height := 0.0
			for idx, f := range fields {
				height += f.height
				if idx < len(fields)-1 {
					height += inviteFieldSpacing
				}
			}
			layout.fieldsHeight = height
		}

		desc := ""
		if av.DescriptionHTML != nil && *av.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*av.DescriptionHTML)
		} else if av.Description != nil {
			desc = cleanText(*av.Description)
		}
		if strings.TrimSpace(desc) != "" {
			descLines := wordWrapWithSmartURLs(ctx, desc, descWidth)
			layout.descLines = filterLines(descLines, true)
			if len(layout.descLines) > 0 {
				layout.descHeight = inviteDescPadding*2 + inviteLineHeight*float64(len(layout.descLines))
				layout.hasDescription = true
			}
		}

		height := layout.headerHeight + inviteHeaderMargin
		if len(layout.fields) > 0 {
			height += layout.fieldsHeight
			if layout.hasDescription {
				height += inviteSectionSpacing
			}
		}
		if layout.hasDescription {
			height += layout.descHeight
		}
		layout.totalHeight = height
		layouts = append(layouts, layout)
	}

	return layouts
}

func availabilityWindowLines(ctx *gg.Context, wins []icsparse.AvailableWindow, width float64) []string {
	if len(wins) == 0 {
		return nil
	}
	entries := make([]string, 0, len(wins)*2)
	for _, win := range wins {
		summary := availableWindowText(win)
		entries = append(entries, "• "+summary)
		for _, detail := range availabilityWindowDetailTexts(win) {
			entries = append(entries, "  - "+detail)
		}
	}
	return wrapListLines(ctx, entries, width)
}

func availableWindowText(win icsparse.AvailableWindow) string {
	parts := []string{}
	if win.Start != nil || win.End != nil {
		start := formatDateTime(win.Start)
		end := formatDateTime(win.End)
		if start != "" && end != "" {
			parts = append(parts, fmt.Sprintf("%s → %s", start, end))
		} else if start != "" {
			parts = append(parts, "Starts "+start)
		} else if end != "" {
			parts = append(parts, "Ends "+end)
		}
	}
	if win.Duration != nil && *win.Duration != "" {
		parts = append(parts, "Duration "+*win.Duration)
	}
	if win.Recurrence != nil {
		rec := win.Recurrence
		if rec.RRule != nil && *rec.RRule != "" {
			parts = append(parts, "RRULE="+*rec.RRule)
		}
		if len(rec.RDates) > 0 {
			parts = append(parts, fmt.Sprintf("RDATE count=%d", len(rec.RDates)))
		}
		if len(rec.ExDates) > 0 {
			parts = append(parts, fmt.Sprintf("EXDATE count=%d", len(rec.ExDates)))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "Available window")
	}
	return strings.Join(parts, " | ")
}

func availabilityWindowDetailTexts(win icsparse.AvailableWindow) []string {
	lines := []string{}
	if win.UID != nil && strings.TrimSpace(*win.UID) != "" {
		lines = append(lines, "UID: "+strings.TrimSpace(*win.UID))
	}
	if win.Location != nil && strings.TrimSpace(*win.Location) != "" {
		lines = append(lines, "Location: "+strings.TrimSpace(*win.Location))
	}
	if len(win.Contacts) > 0 {
		for _, c := range win.Contacts {
			c = strings.TrimSpace(c)
			if c != "" {
				lines = append(lines, "Contact: "+c)
			}
		}
	}
	if len(win.Categories) > 0 {
		lines = append(lines, "Categories: "+strings.Join(win.Categories, ", "))
	}
	return lines
}

func availabilityInfoLines(av icsparse.AvailabilityInfo) []string {
	lines := make([]string, 0, 8)
	if av.UID != nil && strings.TrimSpace(*av.UID) != "" {
		lines = append(lines, "UID: "+strings.TrimSpace(*av.UID))
	}
	if av.BusyType != nil && *av.BusyType != "" {
		lines = append(lines, "Busy: "+*av.BusyType)
	}
	if av.Start != nil {
		lines = append(lines, "Start: "+formatDateTime(av.Start))
	}
	if av.End != nil {
		lines = append(lines, "End: "+formatDateTime(av.End))
	}
	if av.Duration != nil && *av.Duration != "" {
		lines = append(lines, "Duration: "+*av.Duration)
	}
	if av.Location != nil && *av.Location != "" {
		lines = append(lines, "Location: "+cleanText(*av.Location))
	}
	if av.Organizer != nil && *av.Organizer != "" {
		lines = append(lines, "Organizer: "+cleanText(*av.Organizer))
	}
	if av.URL != nil && *av.URL != "" {
		lines = append(lines, "URL: "+*av.URL)
	}
	if len(av.Contacts) > 0 {
		for _, c := range av.Contacts {
			c = strings.TrimSpace(c)
			if c != "" {
				lines = append(lines, "Contact: "+c)
			}
		}
	}
	if len(av.Categories) > 0 {
		lines = append(lines, "Categories: "+strings.Join(av.Categories, ", "))
	}
	for _, slot := range av.Available {
		lines = append(lines, "Slot: "+availableWindowText(slot))
		for _, detail := range availabilityWindowDetailTexts(slot) {
			lines = append(lines, "  "+detail)
		}
	}
	return lines
}

func buildJournalLayouts(journals []icsparse.JournalInfo, width int) []inviteEventLayout {
	if len(journals) == 0 {
		return nil
	}
	contentWidth := float64(width) - 2*invitePad
	if contentWidth < 200 {
		contentWidth = 200
	}
	descWidth := contentWidth - inviteDescPadding*2
	if descWidth < 140 {
		descWidth = 140
	}
	headerTextWidth := contentWidth - (inviteHeaderPadX*2 + inviteIconSize + inviteIconGap)
	if headerTextWidth < 140 {
		headerTextWidth = 140
	}

	ctx := gg.NewContext(width, 100)
	titleFace := loadFontFace("", inviteTitleSize)
	textFace := loadFontFace("", inviteTextSize)
	labelFace := loadFontFace("", inviteLabelSize)

	layouts := make([]inviteEventLayout, 0, len(journals))

	for _, jn := range journals {
		var layout inviteEventLayout
		summary := strings.TrimSpace(jn.Summary)
		if summary == "" {
			summary = "Journal Entry"
		}
		ctx.SetFontFace(titleFace)
		layout.summaryLines = ctx.WordWrap(summary, headerTextWidth)
		if len(layout.summaryLines) == 0 {
			layout.summaryLines = []string{summary}
		}
		titleHeight := inviteHeaderLineH * float64(len(layout.summaryLines))
		layout.headerHeight = math.Max(inviteIconSize, titleHeight) + inviteHeaderPadY*2

		baseValueWidth := contentWidth - inviteLabelWidth - inviteLabelPadding
		if baseValueWidth < inviteMinValueWidth {
			baseValueWidth = inviteMinValueWidth
		}

		buildFields := func(w float64) []inviteField {
			ctx.SetFontFace(textFace)
			addField := func(label string, lines []string) []inviteField {
				filtered := filterLines(lines, false)
				if len(filtered) == 0 {
					return nil
				}
				height := inviteLineHeight * float64(len(filtered))
				if height < inviteLineHeight {
					height = inviteLineHeight
				}
				return []inviteField{{label: label, lines: filtered, height: height}}
			}

			fields := make([]inviteField, 0, 16)
			if jn.UID != nil && strings.TrimSpace(*jn.UID) != "" {
				fields = append(fields, addField("UID:", []string{strings.TrimSpace(*jn.UID)})...)
			}
			if jn.DTStart != nil {
				fields = append(fields, addField("Date:", []string{formatDateTime(jn.DTStart)})...)
			}
			if jn.Organizer != nil && *jn.Organizer != "" {
				fields = append(fields, addField("Organizer:", wordWrapWithSmartURLs(ctx, cleanText(*jn.Organizer), w))...)
			}
			if jn.Status != nil && *jn.Status != "" {
				fields = append(fields, addField("Status:", []string{*jn.Status})...)
			}
			if jn.Class != nil && *jn.Class != "" {
				fields = append(fields, addField("Class:", []string{*jn.Class})...)
			}
			if len(jn.Categories) > 0 {
				fields = append(fields, addField("Categories:", wordWrapWithSmartURLs(ctx, strings.Join(jn.Categories, ", "), w))...)
			}
			if contacts := bulletListLines(ctx, jn.Contacts, w); len(contacts) > 0 {
				fields = append(fields, addField("Contacts:", contacts)...)
			}
			if related := bulletListLines(ctx, jn.RelatedTo, w); len(related) > 0 {
				fields = append(fields, addField("Related To:", related)...)
			}
			if jn.URL != nil && *jn.URL != "" {
				fields = append(fields, addField("URL:", wordWrapWithSmartURLs(ctx, *jn.URL, w))...)
			}
			if jn.DateTimeStamp != nil {
				fields = append(fields, addField("Timestamp:", []string{formatDateTime(jn.DateTimeStamp)})...)
			}
			if jn.Created != nil {
				fields = append(fields, addField("Created:", []string{formatDateTime(jn.Created)})...)
			}
			if jn.LastModified != nil {
				fields = append(fields, addField("Last Modified:", []string{formatDateTime(jn.LastModified)})...)
			}
			if rec := recurrenceLines(ctx, jn.Recurrence, w); len(rec) > 0 {
				fields = append(fields, addField("Recurrence:", rec)...)
			}
			if lines := attendeeLines(ctx, jn.Attendees, w); len(lines) > 0 {
				fields = append(fields, addField("Attendees:", lines)...)
			}
			if images := imageLines(ctx, jn.Images, w); len(images) > 0 {
				fields = append(fields, addField("Images:", images)...)
			}
			if conf := conferenceLines(ctx, jn.Conferences, w); len(conf) > 0 {
				fields = append(fields, addField("Conference:", conf)...)
			}
			if len(jn.Attachments) > 0 {
				entries := make([]string, 0, len(jn.Attachments))
				for _, att := range jn.Attachments {
					entries = append(entries, "• "+chipLabel(att))
				}
				fields = append(fields, addField("Attachments:", wrapListLines(ctx, entries, w))...)
			}
			if urls := urlListLines(ctx, jn.DiscoveredURLs, w); len(urls) > 0 {
				fields = append(fields, addField("Referenced URLs:", urls)...)
			}
			return fields
		}

		fields := buildFields(baseValueWidth)
		labelWidth := measureLabelColumnWidth(ctx, labelFace, fields, contentWidth)
		actualValueWidth := contentWidth - labelWidth - inviteLabelPadding
		if actualValueWidth < inviteMinValueWidth {
			actualValueWidth = inviteMinValueWidth
			labelWidth = contentWidth - actualValueWidth - inviteLabelPadding
		}
		if math.Abs(actualValueWidth-baseValueWidth) > 0.1 {
			fields = buildFields(actualValueWidth)
		}

		layout.fields = fields
		layout.labelWidth = labelWidth
		if len(fields) > 0 {
			height := 0.0
			for idx, f := range fields {
				height += f.height
				if idx < len(fields)-1 {
					height += inviteFieldSpacing
				}
			}
			layout.fieldsHeight = height
		}

		desc := ""
		if jn.DescriptionHTML != nil && *jn.DescriptionHTML != "" {
			desc = icsparse.PlainTextFromHTML(*jn.DescriptionHTML)
		} else if jn.Description != nil {
			desc = cleanText(*jn.Description)
		}
		if strings.TrimSpace(desc) != "" {
			descLines := wordWrapWithSmartURLs(ctx, desc, descWidth)
			layout.descLines = filterLines(descLines, true)
			if len(layout.descLines) > 0 {
				layout.descHeight = inviteDescPadding*2 + inviteLineHeight*float64(len(layout.descLines))
				layout.hasDescription = true
			}
		}

		height := layout.headerHeight + inviteHeaderMargin
		if len(layout.fields) > 0 {
			height += layout.fieldsHeight
			if layout.hasDescription {
				height += inviteSectionSpacing
			}
		}
		if layout.hasDescription {
			height += layout.descHeight
		}
		layout.totalHeight = height
		layouts = append(layouts, layout)
	}

	return layouts
}

func journalInfoLines(jn icsparse.JournalInfo) []string {
	lines := make([]string, 0, 16)
	if jn.UID != nil && strings.TrimSpace(*jn.UID) != "" {
		lines = append(lines, "UID: "+strings.TrimSpace(*jn.UID))
	}
	if jn.DTStart != nil {
		lines = append(lines, "Date: "+formatDateTime(jn.DTStart))
	}
	if jn.Organizer != nil && *jn.Organizer != "" {
		lines = append(lines, "Organizer: "+cleanText(*jn.Organizer))
	}
	if jn.Status != nil && *jn.Status != "" {
		lines = append(lines, "Status: "+*jn.Status)
	}
	if jn.Class != nil && *jn.Class != "" {
		lines = append(lines, "Class: "+*jn.Class)
	}
	if len(jn.Categories) > 0 {
		lines = append(lines, "Categories: "+strings.Join(jn.Categories, ", "))
	}
	for _, c := range jn.Contacts {
		c = strings.TrimSpace(c)
		if c != "" {
			lines = append(lines, "Contact: "+c)
		}
	}
	for _, rel := range jn.RelatedTo {
		rel = strings.TrimSpace(rel)
		if rel != "" {
			lines = append(lines, "Related: "+rel)
		}
	}
	if jn.URL != nil && *jn.URL != "" {
		lines = append(lines, "URL: "+*jn.URL)
	}
	for _, u := range jn.DiscoveredURLs {
		u = strings.TrimSpace(u)
		if u != "" {
			lines = append(lines, "Referenced: "+u)
		}
	}
	if jn.DateTimeStamp != nil {
		lines = append(lines, "Timestamp: "+formatDateTime(jn.DateTimeStamp))
	}
	if jn.Created != nil {
		lines = append(lines, "Created: "+formatDateTime(jn.Created))
	}
	if jn.LastModified != nil {
		lines = append(lines, "Last Modified: "+formatDateTime(jn.LastModified))
	}
	if jn.Recurrence != nil && jn.Recurrence.RRule != nil && *jn.Recurrence.RRule != "" {
		lines = append(lines, "RRULE: "+*jn.Recurrence.RRule)
	}
	if len(jn.Conferences) > 0 {
		entries := conferenceSummariesForPNG(jn.Conferences)
		lines = append(lines, entries...)
	}
	if len(jn.Attendees) > 0 {
		for _, att := range jn.Attendees {
			lines = append(lines, formatAttendeeDisplay(att))
		}
	}
	if len(jn.Attachments) > 0 {
		for _, att := range jn.Attachments {
			lines = append(lines, chipLabel(att))
		}
	}
	if len(jn.Images) > 0 {
		for _, img := range jn.Images {
			lines = append(lines, imageSummaryText(img))
		}
	}
	return lines
}

func calculateLayoutsHeight(layouts []inviteEventLayout) float64 {
	if len(layouts) == 0 {
		return invitePad * 2
	}
	total := 0.0
	for i, layout := range layouts {
		total += layout.totalHeight
		if i < len(layouts)-1 {
			total += inviteEventSpacing
		}
	}
	return total + invitePad*2
}

func wrapListLines(dc *gg.Context, entries []string, width float64) []string {
	if len(entries) == 0 {
		return nil
	}
	var lines []string
	for _, entry := range entries {
		wrapped := wordWrapWithSmartURLs(dc, entry, width)
		lines = append(lines, wrapped...)
	}
	return filterLines(lines, false)
}

func bulletListLines(dc *gg.Context, entries []string, width float64) []string {
	if len(entries) == 0 {
		return nil
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		items = append(items, "• "+entry)
	}
	return wrapListLines(dc, items, width)
}

func imageSummaryText(img icsparse.ImageInfo) string {
	parts := []string{}
	source := strings.ToLower(strings.TrimSpace(img.Source))
	if source == "" {
		source = "unknown"
	}
	parts = append(parts, "source="+source)
	if img.FmtType != nil && *img.FmtType != "" {
		parts = append(parts, "type="+*img.FmtType)
	}
	if img.Display != nil && *img.Display != "" {
		parts = append(parts, "display="+*img.Display)
	}
	if img.AltRep != nil && *img.AltRep != "" {
		parts = append(parts, "altrep="+*img.AltRep)
	}
	val := strings.TrimSpace(img.Value)
	if val != "" {
		if len(val) > 80 {
			val = val[:77] + "..."
		}
		parts = append(parts, "value="+val)
	}
	return strings.Join(parts, "; ")
}

func imageLines(dc *gg.Context, images []icsparse.ImageInfo, width float64) []string {
	if len(images) == 0 {
		return nil
	}
	items := make([]string, 0, len(images))
	for _, img := range images {
		items = append(items, "• "+imageSummaryText(img))
	}
	return wrapListLines(dc, items, width)
}

func geoLines(dc *gg.Context, geo *icsparse.GeoPoint, width float64) []string {
	if geo == nil {
		return nil
	}
	coords := fmt.Sprintf("• Coordinates: %.4f, %.4f", geo.Latitude, geo.Longitude)
	mapURL := fmt.Sprintf("• https://maps.google.com/?q=%f,%f", geo.Latitude, geo.Longitude)
	return wrapListLines(dc, []string{coords, mapURL}, width)
}

func requestStatusLines(dc *gg.Context, statuses []string, width float64) []string {
	return bulletListLines(dc, statuses, width)
}

func filterLines(lines []string, keepEmpty bool) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if keepEmpty {
				out = append(out, "")
			}
			continue
		}
		out = append(out, strings.TrimRight(line, " "))
	}
	return out
}

func drawInviteField(dc *gg.Context, field inviteField, x, y float64, pal palette, labelFace, textFace font.Face, labelWidth float64) {
	labelX := x + inviteLabelPadding
	columnWidth := labelWidth
	if columnWidth < inviteLabelWidth {
		columnWidth = inviteLabelWidth
	}
	valueX := x + columnWidth + inviteLabelPadding
	baseline := y + inviteLineHeight - inviteBaselineAdjust

	dc.SetFontFace(labelFace)
	dc.SetColor(pal.Muted)
	dc.DrawString(field.label, labelX, baseline)

	dc.SetFontFace(textFace)
	dc.SetColor(pal.Text)
	lineY := baseline
	for _, line := range field.lines {
		if strings.TrimSpace(line) != "" {
			dc.DrawString(line, valueX, lineY)
		}
		lineY += inviteLineHeight
	}
}

func measureLabelColumnWidth(ctx *gg.Context, face font.Face, fields []inviteField, contentWidth float64) float64 {
	if len(fields) == 0 {
		return inviteLabelWidth
	}
	ctx.SetFontFace(face)
	maxWidth := inviteLabelWidth
	for _, field := range fields {
		if w, _ := ctx.MeasureString(field.label); w > maxWidth {
			maxWidth = w
		}
	}
	maxWidth += inviteLabelMeasurePad
	maxAllowed := contentWidth - inviteMinValueWidth - inviteLabelPadding
	if maxAllowed < inviteLabelWidth {
		maxAllowed = inviteLabelWidth
	}
	if maxWidth > maxAllowed {
		return maxAllowed
	}
	if maxWidth < inviteLabelWidth {
		return inviteLabelWidth
	}
	return maxWidth
}

func chipLabel(a icsparse.AttachmentInfo) string {
	if a.SavedAs != nil {
		// Show filename only, not full path
		name := filepath.Base(*a.SavedAs)
		if a.Size != nil {
			sizeKB := float64(*a.Size) / 1024.0
			if sizeKB < 1024 {
				return fmt.Sprintf("%s (%.1f KB)", name, sizeKB)
			}
			return fmt.Sprintf("%s (%.1f MB)", name, sizeKB/1024.0)
		}
		return name
	}
	if a.Source == "url" {
		if a.Fmt != nil {
			return "Link (" + *a.Fmt + ")"
		}
		return "Link"
	}
	if a.Fmt != nil {
		return "Inline (" + *a.Fmt + ")"
	}
	return "Inline attachment"
}

func conferenceLines(ctx *gg.Context, confs []icsparse.ConferenceInfo, width float64) []string {
	if len(confs) == 0 {
		return nil
	}
	entries := make([]string, 0, len(confs))
	for _, c := range confs {
		line := strings.TrimSpace(c.URI)
		if len(c.Params) > 0 {
			keys := make([]string, 0, len(c.Params))
			for k := range c.Params {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%s", k, c.Params[k]))
			}
			paramStr := strings.Join(parts, ", ")
			if line == "" {
				line = paramStr
			} else {
				line = line + " (" + paramStr + ")"
			}
		}
		if line != "" {
			entries = append(entries, "• "+line)
		}
	}
	return wrapListLines(ctx, entries, width)
}

func conferenceSummariesForPNG(confs []icsparse.ConferenceInfo) []string {
	if len(confs) == 0 {
		return nil
	}
	ctx := gg.NewContext(1, 1)
	ctx.SetFontFace(loadFontFace("", inviteTextSize))
	return conferenceLines(ctx, confs, 220)
}

func recurrenceLines(ctx *gg.Context, rec *icsparse.RecurrenceInfo, width float64) []string {
	if rec == nil {
		return nil
	}
	lines := make([]string, 0, 8)
	if rec.RRule != nil && *rec.RRule != "" {
		lines = append(lines, "RRULE: "+*rec.RRule)
	}
	for _, t := range rec.RDates {
		tt := t
		lines = append(lines, "RDATE: "+formatDateTime(&tt))
	}
	for _, raw := range rec.RDateRaw {
		lines = append(lines, "RDATE (raw): "+raw)
	}
	for _, t := range rec.ExDates {
		tt := t
		lines = append(lines, "EXDATE: "+formatDateTime(&tt))
	}
	for _, raw := range rec.ExDateRaw {
		lines = append(lines, "EXDATE (raw): "+raw)
	}
	if rec.RecurrenceID != nil {
		lines = append(lines, "Recurrence ID: "+formatDateTime(rec.RecurrenceID))
	} else if rec.RecurrenceIDRaw != nil && *rec.RecurrenceIDRaw != "" {
		lines = append(lines, "Recurrence ID: "+*rec.RecurrenceIDRaw)
	}
	if rec.Duration != nil && *rec.Duration != "" {
		lines = append(lines, "Duration: "+*rec.Duration)
	}
	return wrapListLines(ctx, lines, width)
}

func alarmSummaryLines(ctx *gg.Context, alarms []icsparse.AlarmInfo, width float64) []string {
	if len(alarms) == 0 {
		return nil
	}
	entries := make([]string, 0, len(alarms))
	for _, a := range alarms {
		parts := []string{}
		if a.Trigger != nil {
			if a.Trigger.Duration != nil && *a.Trigger.Duration != "" {
				parts = append(parts, "Trigger "+*a.Trigger.Duration)
			} else if a.Trigger.Time != nil {
				parts = append(parts, "Trigger "+formatDateTime(a.Trigger.Time))
			}
			if a.Trigger.Related != nil && *a.Trigger.Related != "" {
				parts = append(parts, "Related="+*a.Trigger.Related)
			}
		}
		if a.Action != nil && *a.Action != "" {
			parts = append(parts, "Action "+*a.Action)
		}
		if a.Description != nil && *a.Description != "" {
			parts = append(parts, "Note "+cleanText(*a.Description))
		}
		if a.Summary != nil && *a.Summary != "" {
			parts = append(parts, "Summary "+cleanText(*a.Summary))
		}
		if a.Repeat != nil {
			parts = append(parts, fmt.Sprintf("Repeat %d", *a.Repeat))
		}
		if a.Duration != nil && *a.Duration != "" {
			parts = append(parts, "Every "+*a.Duration)
		}
		if len(parts) == 0 {
			parts = append(parts, "Configured")
		}
		entries = append(entries, "• "+strings.Join(parts, "; "))
	}
	return wrapListLines(ctx, entries, width)
}

func urlListLines(ctx *gg.Context, urls []string, width float64) []string {
	if len(urls) == 0 {
		return nil
	}
	entries := make([]string, 0, len(urls))
	for _, u := range urls {
		if u == "" {
			continue
		}
		entries = append(entries, "• "+u)
	}
	return wrapListLines(ctx, entries, width)
}

func attendeeLines(ctx *gg.Context, attendees []icsparse.Attendee, width float64) []string {
	if len(attendees) == 0 {
		return nil
	}
	entries := make([]string, 0, len(attendees))
	for _, att := range attendees {
		entries = append(entries, "• "+formatAttendeeDisplay(att))
	}
	return wrapListLines(ctx, entries, width)
}

func formatAttendeeDisplay(att icsparse.Attendee) string {
	name := strings.TrimSpace(att.Mailto)
	if att.CN != nil && strings.TrimSpace(*att.CN) != "" {
		label := strings.TrimSpace(*att.CN)
		if att.Mailto != "" {
			name = fmt.Sprintf("%s <%s>", label, att.Mailto)
		} else {
			name = label
		}
	} else if att.Mailto != "" {
		name = att.Mailto
	} else if name == "" {
		name = "Unknown attendee"
	}
	if att.PartStat != nil && *att.PartStat != "" {
		name += " (" + *att.PartStat + ")"
	}
	if att.RSVP != nil && *att.RSVP != "" {
		name += " [RSVP " + *att.RSVP + "]"
	}
	extra := make([]string, 0, 6)
	if att.Role != nil && *att.Role != "" {
		extra = append(extra, "role="+*att.Role)
	}
	if att.Cutype != nil && *att.Cutype != "" {
		extra = append(extra, "type="+*att.Cutype)
	}
	if att.SentBy != nil && *att.SentBy != "" {
		extra = append(extra, "sent-by="+*att.SentBy)
	}
	if len(att.DelegatedFrom) > 0 {
		extra = append(extra, "delegated-from="+strings.Join(att.DelegatedFrom, ", "))
	}
	if len(att.DelegatedTo) > 0 {
		extra = append(extra, "delegated-to="+strings.Join(att.DelegatedTo, ", "))
	}
	if len(att.Member) > 0 {
		extra = append(extra, "member="+strings.Join(att.Member, ", "))
	}
	if att.Directory != nil && *att.Directory != "" {
		extra = append(extra, "dir="+*att.Directory)
	}
	if att.Language != nil && *att.Language != "" {
		extra = append(extra, "lang="+*att.Language)
	}
	if len(extra) > 0 {
		name += " [" + strings.Join(extra, "; ") + "]"
	}
	return name
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	if t.Location() != nil && t.Location() != time.UTC {
		return t.Format("Mon, Jan 2 2006 • 3:04 PM MST")
	}
	return t.Format("Mon, Jan 2 2006 • 15:04")
}

func lessByStart(a, b icsparse.EventInfo) bool {
	if a.DTStart == nil && b.DTStart == nil {
		return false
	}
	if a.DTStart == nil {
		return false
	}
	if b.DTStart == nil {
		return true
	}
	return a.DTStart.Before(*b.DTStart)
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
