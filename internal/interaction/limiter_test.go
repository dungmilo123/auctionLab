package interaction

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiterBoundAndExpiry(t *testing.T) {
	l := NewLimiter()
	now := time.UnixMilli(100)
	l.Now = func() time.Time { return now }
	for i := 0; i < 30; i++ {
		if !l.Allow("127.0.0.1:1") {
			t.Fatal(i)
		}
	}
	if l.Allow("127.0.0.1:2") {
		t.Fatal("port bypass")
	}
	for i := 1; i < 4096; i++ {
		if !l.Allow(fmt.Sprint("peer", i)) {
			t.Fatal(i)
		}
	}
	if l.Allow("extra") || len(l.peers) != 4096 {
		t.Fatal("capacity")
	}
	now = now.Add(time.Minute)
	if !l.Allow("extra") || len(l.peers) != 1 {
		t.Fatal("expiry")
	}
}
