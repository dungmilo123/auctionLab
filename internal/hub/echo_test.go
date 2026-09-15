package hub

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The Phase 1 echo round-trip tests (TestEcho, TestEchoKeepalive) are retired
// here: the handler they described was temporary and is replaced wholesale in
// Phase 2 (D-01), so their assertions on the {type,ts,data} envelope would pin
// the superseded contract (D-21). Their coverage is replaced by the tracer's
// handshake/protocol tests plus 02-03's ordering and keepalive tests. The
// origin guard below still belongs to this file, and plan 02-03 deletes the
// file wholesale when it replaces the handler.
//
// TestOriginRejectionIsLoggedOncePerInterval pins the abuse guard (IN-07): a
// cross-origin handshake is rejected with 403 and its log line is collapsed to
// one per interval — /ws is unauthenticated, so a scanner must not be able to
// drive unbounded log writes once the app is published. This is also the one
// test that exercises gorilla's same-origin CheckOrigin: DefaultDialer sends no
// Origin header, so every other test here is blind to it.
func TestOriginRejectionIsLoggedOncePerInterval(t *testing.T) {
	lastOriginRejectLog.Store(0) // this test owns the package-level gate
	defer lastOriginRejectLog.Store(0)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	srv := httptest.NewServer(http.HandlerFunc(Echo))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	for attempt := 1; attempt <= 2; attempt++ {
		conn, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"http://evil.example"}})
		if conn != nil {
			_ = conn.Close()
		}
		if err == nil {
			t.Fatalf("attempt %d: cross-origin upgrade succeeded; CheckOrigin is not armed", attempt)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			status := 0
			if resp != nil {
				status = resp.StatusCode
				_ = resp.Body.Close()
			}
			t.Fatalf("attempt %d: HTTP status = %d, want %d", attempt, status, http.StatusForbidden)
		}
		_ = resp.Body.Close()
	}

	// The log call happens in the server handler goroutine, after the 403
	// reaches this one: wait for the first line, then settle briefly so a
	// second (buggy) line has time to land before counting.
	deadline := time.Now().Add(2 * time.Second)
	for buf.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	if got := strings.Count(buf.String(), "ws: upgrade failed"); got != 1 {
		t.Errorf("logged %d upgrade failures for 2 cross-origin attempts, want 1:\n%s", got, buf.String())
	}
}
