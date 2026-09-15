package hub

import (
	"reflect"
	"testing"
)

// TestProtocol pins the frozen envelope (D-21) on the wire: the five fields,
// the per-connection seq stream, and the rule research Pitfall 3 is about —
// two messages may legitimately share one revision, so dedupe is on seq only.
//
// The catalog itself is pinned in the "frozen catalog" cases below, which
// compare the exported constants against literal strings: the literal is the
// contract and the constant is what production claims to export, so a rename
// fails here instead of at a client.
func TestProtocol(t *testing.T) {
	t.Run("the envelope carries exactly type/seq/rev/ts/data", func(t *testing.T) {
		conn := dialTracer(t)
		tracerWrite(t, conn, `{"type":"join","role":"player"}`)

		env := tracerRead(t, conn)
		assertEnvelopeKeys(t, env)
		if env.TS <= 0 {
			t.Errorf("ts = %d, want a positive server millisecond clock (D-21)", env.TS)
		}
	})

	t.Run("two messages may share one revision", func(t *testing.T) {
		conn := dialTracer(t)
		tracerWrite(t, conn, `{"type":"join","role":"player"}`)

		first := tracerRead(t, conn)
		second := tracerRead(t, conn)

		if first.Rev != second.Rev {
			t.Fatalf("rev %d then %d, want the same revision — one mutation legitimately produces several messages (D-22)",
				first.Rev, second.Rev)
		}
		if second.Seq != first.Seq+1 {
			t.Errorf("seq %d then %d, want consecutive numbers (D-22: gap detection is seq's job, never rev's)",
				first.Seq, second.Seq)
		}
	})
}

// assertEnvelopeKeys pins the exact key set of a frame. Exact, not a superset:
// D-24's freeze means a renamed or removed field has to be a deliberate edit
// that breaks this test, and D-21 means a frame without seq/rev is not this
// protocol at all.
func assertEnvelopeKeys(t *testing.T, env tracerFrame) {
	t.Helper()

	want := []string{"data", "rev", "seq", "ts", "type"}
	if !reflect.DeepEqual(env.keys, want) {
		t.Fatalf("envelope keys = %v, want exactly %v (D-21/D-24)", env.keys, want)
	}
}
