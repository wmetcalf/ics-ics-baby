package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"ics-ics-baby/internal/icsparse"
)

type palette struct {
	Bg     color.Color
	Panel  color.Color
	Text   color.Color
	Muted  color.Color
	Edge   color.Color
	Accent color.Color
	ChipBg color.Color
}

func getPalette(style string) palette {
	if style == "dark" {
		return palette{
			Bg:     color.RGBA{0x2b, 0x2b, 0x2b, 0xff}, // Dark gray background
			Panel:  color.RGBA{0x3a, 0x3a, 0x3a, 0xff}, // Slightly lighter panel
			Text:   color.RGBA{0xff, 0xff, 0xff, 0xff}, // White text
			Muted:  color.RGBA{0xb0, 0xb0, 0xb0, 0xff}, // Light gray
			Edge:   color.RGBA{0x55, 0x55, 0x55, 0xff}, // Border color
			Accent: color.RGBA{0xff, 0x66, 0x00, 0xff}, // Orange accent
			ChipBg: color.RGBA{0x45, 0x45, 0x45, 0xff}, // Chip background
		}
	}
	return palette{
		Bg:     color.RGBA{0xf5, 0xf5, 0xf5, 0xff}, // Light gray background
		Panel:  color.RGBA{0xff, 0xff, 0xff, 0xff}, // White panel
		Text:   color.RGBA{0x1a, 0x1a, 0x1a, 0xff}, // Dark text
		Muted:  color.RGBA{0x66, 0x66, 0x66, 0xff}, // Gray text
		Edge:   color.RGBA{0xcc, 0xcc, 0xcc, 0xff}, // Border color
		Accent: color.RGBA{0xff, 0x66, 0x00, 0xff}, // Orange accent
		ChipBg: color.RGBA{0xe8, 0xe8, 0xe8, 0xff}, // Light chip background
	}
}

var (
	goRegularFontOnce sync.Once
	goRegularFont     *truetype.Font
	goRegularErr      error
)

func loadFontFace(path string, size float64) font.Face {
	if path != "" {
		if face, err := gg.LoadFontFace(path, size); err == nil {
			return face
		}
	}
	goRegularFontOnce.Do(func() {
		goRegularFont, goRegularErr = truetype.Parse(goregular.TTF)
	})
	if goRegularErr == nil && goRegularFont != nil {
		return truetype.NewFace(goRegularFont, &truetype.Options{
			Size:    size,
			Hinting: font.HintingFull,
		})
	}
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

	events := make([]icsparse.EventInfo, len(cal.Events))
	copy(events, cal.Events)
	sort.SliceStable(events, func(i, j int) bool { return lessByStart(events[i], events[j]) })

	h := int(2*pad + cardPad + 40)
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
	meta := "Generated preview"
	if cal.ProdID != nil && *cal.ProdID != "" {
		meta = fmt.Sprintf("%s  •  PRODID: %s", meta, *cal.ProdID)
	}
	dc.SetFontFace(loadFontFace("", smallSize))
	dc.SetColor(pal.Muted)
	dc.DrawStringAnchored(meta, panelX+cardPad, y, 0, 0)

	y += 18

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
		cardH := 18.0 + lineH*3 + descH + attH + eventPad

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
		var loc, org string
		if e.Location != nil {
			loc = cleanText(*e.Location)
		}
		if e.Organizer != nil {
			org = cleanText(*e.Organizer)
		}
		startS := fmtTime(e.DTStart)
		endS := fmtTime(e.DTEnd)
		metaLine := fmt.Sprintf("Starts: %s   Ends: %s", startS, endS)
		dc.DrawStringAnchored(metaLine, innerX, innerY+lineH*2.4, 0, 0)
		if loc != "" {
			dc.DrawStringAnchored("Location: "+loc, innerX, innerY+lineH*3.2, 0, 0)
		}
		if org != "" {
			dc.DrawStringAnchored("Organizer: "+org, innerX, innerY+lineH*4.0, 0, 0)
		}

		dc.SetColor(pal.Text)
		dc.SetFontFace(loadFontFace("", textSize))
		textY := innerY + lineH*4.8
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

	foot := "This PNG was rendered locally without a browser."
	dc.SetColor(pal.Muted)
	dc.SetFontFace(loadFontFace("", smallSize))
	dc.DrawStringAnchored(foot, panelX+cardPad, float64(img.Bounds().Dy())-pad, 0, 1)

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return gg.SavePNG(outPath, img)
}

const (
	invitePad            = 30.0
	inviteHeaderPadX     = 20.0
	inviteHeaderPadY     = 18.0
	inviteHeaderMargin   = 20.0
	inviteEventSpacing   = 30.0
	inviteIconSize       = 20.0
	inviteIconGap        = 15.0
	inviteLabelWidth     = 110.0
	inviteLabelPadding   = 10.0
	inviteHeaderLineH    = 22.0
	inviteLineHeight     = 20.0
	inviteBaselineAdjust = 6.0
	inviteDescPadding    = 15.0
	inviteFieldSpacing   = 8.0
	inviteSectionSpacing = 20.0
	inviteTitleSize      = 16.0
	inviteLabelSize      = 13.0
	inviteTextSize       = 13.0
	inviteMinHeight      = 500
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
}

func RenderInvitePNG(cal *icsparse.CalendarInfo, outPath string, width int, style string) error {
	pal := getPalette(style)

	layouts, totalHeight := buildInviteLayouts(cal.Events, width)
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
	for i, layout := range layouts {
		// Header container
		dc.SetColor(pal.Panel)
		dc.DrawRoundedRectangle(invitePad, y, contentWidth, layout.headerHeight, 8)
		dc.Fill()

		// Accent square
		iconY := y + (layout.headerHeight-inviteIconSize)/2
		dc.SetColor(pal.Accent)
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
				drawInviteField(dc, field, invitePad, currentY, pal, labelFace, textFace)
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

	// Footer note
	foot := "This PNG was rendered locally without a browser."
	dc.SetColor(pal.Muted)
	dc.SetFontFace(labelFace)
	dc.DrawStringAnchored(foot, invitePad, float64(img.Bounds().Dy())-invitePad*0.5, 0, 1)

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
	valueWidth := contentWidth - inviteLabelWidth - inviteLabelPadding
	if valueWidth < 140 {
		valueWidth = 140
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
			if e.DTStart != nil {
				fields = append(fields, addField("Start Date:", []string{formatDateTime(e.DTStart)})...)
			}
			if e.DTEnd != nil {
				fields = append(fields, addField("End Date:", []string{formatDateTime(e.DTEnd)})...)
			}
			if e.Location != nil {
				loc := cleanText(*e.Location)
				if loc != "" {
					fields = append(fields, addField("Location:", wordWrapWithSmartURLs(ctx, loc, valueWidth))...)
				}
			}
			if e.Organizer != nil {
				org := cleanText(*e.Organizer)
				if org != "" {
					fields = append(fields, addField("Organizer:", wordWrapWithSmartURLs(ctx, org, valueWidth))...)
				}
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
			if e.Duration != nil && *e.Duration != "" {
				fields = append(fields, addField("Duration:", []string{*e.Duration})...)
			}
			if e.URL != nil && *e.URL != "" {
				fields = append(fields, addField("URL:", wordWrapWithSmartURLs(ctx, *e.URL, valueWidth))...)
			}
			if urls := urlListLines(ctx, e.DiscoveredURLs, valueWidth); len(urls) > 0 {
				fields = append(fields, addField("Referenced URLs:", urls)...)
			}
			if len(e.Categories) > 0 {
				joined := strings.Join(e.Categories, ", ")
				fields = append(fields, addField("Categories:", wordWrapWithSmartURLs(ctx, joined, valueWidth))...)
			}
			if len(e.Resources) > 0 {
				joined := strings.Join(e.Resources, ", ")
				fields = append(fields, addField("Resources:", wordWrapWithSmartURLs(ctx, joined, valueWidth))...)
			}
			if conf := conferenceLines(ctx, e.Conferences, valueWidth); len(conf) > 0 {
				fields = append(fields, addField("Conference:", conf)...)
			}
			if rec := recurrenceLines(ctx, e.Recurrence, valueWidth); len(rec) > 0 {
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

			if lines := attendeeLines(ctx, e.Attendees, valueWidth); len(lines) > 0 {
				fields = append(fields, addField("Attendees:", lines)...)
			}

			if len(e.Attachments) > 0 {
				entries := make([]string, 0, len(e.Attachments))
				for _, att := range e.Attachments {
					entries = append(entries, "• "+chipLabel(att))
				}
				fields = append(fields, addField("Attachments:", wrapListLines(ctx, entries, valueWidth))...)
			}

			if alarms := alarmSummaryLines(ctx, e.Alarms, valueWidth); len(alarms) > 0 {
				fields = append(fields, addField("Alarms:", alarms)...)
			}

			layout.fields = fields
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

func drawInviteField(dc *gg.Context, field inviteField, x, y float64, pal palette, labelFace, textFace font.Face) {
	labelX := x + inviteLabelPadding
	valueX := x + inviteLabelWidth + inviteLabelPadding
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
