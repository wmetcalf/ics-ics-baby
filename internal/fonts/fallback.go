package fonts

import (
	"errors"
	"fmt"
	"image"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	parseOnce   sync.Once
	parsedFonts []*opentype.Font
	parseErr    error
)

func embeddedFonts() ([]*opentype.Font, error) {
	parseOnce.Do(func() {
		var errs error
		add := func(data []byte, label string) {
			if len(data) == 0 {
				return
			}
			f, err := opentype.Parse(data)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("parse %s: %w", label, err))
				return
			}
			parsedFonts = append(parsedFonts, f)
		}
		// NotoSans has better kerning than DejaVu for capital letters like T
		add(NotoSansRegular, "NotoSans-Regular.ttf")
		add(DejaVuSans, "DejaVuSans.ttf")
		// Note: EmojiOneColor.otf removed - color emoji not supported by opentype renderer
		if len(parsedFonts) == 0 {
			if errs != nil {
				parseErr = errs
			} else {
				parseErr = errors.New("no embedded fonts parsed")
			}
			return
		}
		parseErr = errs
	})

	if len(parsedFonts) == 0 {
		if parseErr != nil {
			return nil, parseErr
		}
		return nil, errors.New("no embedded fonts available")
	}
	return parsedFonts, parseErr
}

// NewFallbackFace constructs a font.Face that automatically falls back across
// the embedded font set when glyphs are missing in the primary font.
func NewFallbackFace(size float64) (font.Face, error) {
	fonts, err := embeddedFonts()
	if len(fonts) == 0 {
		return nil, err
	}

	var (
		faces   []font.Face
		warning error
	)

	for _, f := range fonts {
		face, faceErr := opentype.NewFace(f, &opentype.FaceOptions{
			Size:    size,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if faceErr != nil {
			warning = errors.Join(warning, faceErr)
			continue
		}
		faces = append(faces, face)
	}

	if len(faces) == 0 {
		if warning != nil {
			return nil, warning
		}
		return nil, fmt.Errorf("unable to create fallback face at size %.2f", size)
	}

	return &fallbackFace{faces: faces}, warning
}

type fallbackFace struct {
	faces []font.Face
}

func (ff *fallbackFace) Close() error {
	var err error
	for _, face := range ff.faces {
		err = errors.Join(err, face.Close())
	}
	return err
}

func (ff *fallbackFace) Metrics() font.Metrics {
	return ff.faces[0].Metrics()
}

func (ff *fallbackFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	for _, face := range ff.faces {
		dr, mask, maskp, advance, ok := face.Glyph(dot, r)
		if ok {
			return dr, mask, maskp, advance, true
		}
	}
	return ff.faces[0].Glyph(dot, '�')
}

func (ff *fallbackFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	for _, face := range ff.faces {
		if bounds, advance, ok := face.GlyphBounds(r); ok {
			return bounds, advance, true
		}
	}
	return ff.faces[0].GlyphBounds('�')
}

func (ff *fallbackFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	for _, face := range ff.faces {
		if adv, ok := face.GlyphAdvance(r); ok {
			return adv, true
		}
	}
	return ff.faces[0].GlyphAdvance('�')
}

func (ff *fallbackFace) Kern(r0, r1 rune) fixed.Int26_6 {
	for _, face := range ff.faces {
		if _, ok := face.GlyphAdvance(r0); !ok {
			continue
		}
		if _, ok := face.GlyphAdvance(r1); !ok {
			continue
		}
		return face.Kern(r0, r1)
	}
	return ff.faces[0].Kern(r0, r1)
}
