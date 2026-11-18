package clientview

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"ics-ics-baby/internal/icsparse"
)

// abandoned is set when WriteOWABudgeted gives up on a build; the build
// goroutine then writes no view page over the failure page.
var abandoned atomic.Bool

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
	go func() {
		sc, err := WriteOWA(cal, inputSHA256, outDir)
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
	abandoned.Store(true)
	sc := Sidecar{
		Profile: "owa", Emulation: true, InputSHA256: inputSHA256,
		RemoteRefs: []RemoteRef{}, Links: []Link{}, Dropped: []string{"build_budget_exceeded"}, BlockedRequests: []string{},
	}
	return failView(sc, outDir, fmt.Errorf("%s", why))
}
