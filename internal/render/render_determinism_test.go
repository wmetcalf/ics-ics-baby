package render

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ics-ics-baby/internal/icsparse"
)

// TestInvitePNGDeterministic: header maps (MicrosoftHeaders/GoogleHeaders)
// must not make the Go PNG depend on map iteration order.
func TestInvitePNGDeterministic(t *testing.T) {
	cal, err := icsparse.ParseICSFile("../../ics-parser-test-suite/ics_files/09-combo-spoof-urgent.ics", "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var first []byte
	for i := 0; i < 12; i++ {
		p := filepath.Join(dir, "p.png")
		if err := RenderInvitePNG(cal, p, 1200, "light", true); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = b
		} else if !bytes.Equal(first, b) {
			t.Fatalf("render %d differs from render 0 (map-order dependent output)", i)
		}
	}
}
