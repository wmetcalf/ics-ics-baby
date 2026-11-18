package clientview

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"ics-ics-baby/internal/icsparse"
)

// buildGuard lets WriteOWABudgeted abandon one build: once abandoned is
// set (under mu, which writeView also holds while writing), that build's
// goroutine writes no page over the failure page.
type buildGuard struct {
	mu         sync.Mutex
	abandoned  bool
	chromeOnly bool   // WriteOWAChromeOnly: no body
	htmlName   string // page file name (default OWAHTMLName)
}

// WriteOWABudgeted is WriteOWA bounded in time and Go heap. The sanitizer
// models the HTML parser to bound its work, but a model can be wrong; this
// is the backstop. When the build exceeds timeout or its heap passes
// maxHeap bytes, it returns a render_failed sidecar and writes a failure
// page. The abandoned build goroutine keeps running until the process
// exits, which the caller is expected to do soon (the CLI exits 1).
func WriteOWABudgeted(cal *icsparse.CalendarInfo, inputSHA256, outDir string, timeout time.Duration, maxHeap uint64) (Sidecar, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	type result struct {
		sc  Sidecar
		err error
	}
	done := make(chan result, 1)
	g := &buildGuard{}
	go func() {
		sc, err := writeOWAGuarded(cal, inputSHA256, outDir, g)
		done <- result{sc, err}
	}()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var why string
	for why == "" {
		select {
		case r := <-done:
			return r.sc, r.err
		case <-deadline.C:
			why = fmt.Sprintf("building the view took longer than %s", timeout)
		case <-tick.C:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			if maxHeap > 0 && ms.HeapAlloc > maxHeap {
				why = fmt.Sprintf("building the view used more than %d MiB of memory", maxHeap>>20)
			}
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.abandoned = true
	sc := Sidecar{
		Profile: "owa", Emulation: true, InputSHA256: inputSHA256,
		RemoteRefs: []RemoteRef{}, Links: []Link{}, Dropped: []string{"build_budget_exceeded"}, BlockedRequests: []string{},
	}
	return failView(sc, outDir, nil, fmt.Errorf("%s", why))
}
