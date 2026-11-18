package render

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/chromedp"
)

// fakeTransport hands chromedp one malformed message, which chromedp reports
// through its error logger, then blocks until the context ends.
type fakeTransport struct{ sent bool }

func (f *fakeTransport) Read(ctx context.Context, m *cdproto.Message) error {
	if !f.sent {
		f.sent = true
		return nil // no id, no method: "ignoring malformed incoming message"
	}
	<-ctx.Done()
	return ctx.Err()
}
func (f *fakeTransport) Write(context.Context, *cdproto.Message) error { return nil }
func (f *fakeTransport) Close() error                                  { return nil }

// Deferred minor: chromedp's own log lines went unprefixed to stderr through
// the standard logger. They must go through our prefixed sink instead.
func TestChromedpLogsRoutedThroughPrefix(t *testing.T) {
	var std bytes.Buffer
	log.SetOutput(&std)
	defer log.SetOutput(os.Stderr)

	var mu sync.Mutex
	var got bytes.Buffer
	sink := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(&got, format+"\n", args...)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := chromedp.NewBrowserTransport(ctx, &fakeTransport{}, chromedpLogOptions(sink)...); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		s := got.String()
		mu.Unlock()
		if strings.Contains(s, "ignoring malformed incoming message") {
			if !strings.HasPrefix(s, "[chromium] ") {
				t.Fatalf("chromedp line not prefixed: %q", s)
			}
			if std.Len() != 0 {
				t.Fatalf("chromedp also wrote to the standard logger: %q", std.String())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("chromedp error never reached the sink; std logger got %q", std.String())
}
