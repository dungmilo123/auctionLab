package account

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIdentityPolicy(t *testing.T) {
	a, e := Email(" A@Example.TEST ")
	if e != nil || a != "a@example.test" {
		t.Fatal(a, e)
	}
	b, _ := Email("a@example.test")
	if a != b {
		t.Fatal("canonical collision")
	}
	for _, v := range []string{"", "Name <a@example.test>", "a b@example.test", "a@exámple.test", strings.Repeat("a", 255) + "@test"} {
		if _, e = Email(v); e == nil {
			t.Fatal("invalid email accepted")
		}
	}
	for _, v := range []string{"", strings.Repeat("a", 81), "a\nb", string([]byte{255})} {
		if _, e = Name(v); e == nil {
			t.Fatal("invalid name accepted")
		}
	}
	if v, e := Name("  Bình  "); e != nil || v != "Bình" {
		t.Fatal(v, e)
	}
	for _, v := range []string{strings.Repeat("a", 11), strings.Repeat("a", 129), string([]byte{255})} {
		if Password(v) == nil {
			t.Fatal("invalid password accepted")
		}
	}
	for _, v := range []string{strings.Repeat("a", 12), strings.Repeat("界", 128), "   mật khẩu   "} {
		if e := Password(v); e != nil {
			t.Fatal(e)
		}
	}
}
func TestRealCredentialPolicy(t *testing.T) {
	p := "  Unicode mật khẩu  "
	a, e := HashPassword(p)
	if e != nil {
		t.Fatal(e)
	}
	b, e := HashPassword(p)
	if e != nil || a == b {
		t.Fatal("salt reuse")
	}
	if !VerifyPassword(p, a) || VerifyPassword(strings.TrimSpace(p), a) || VerifyPassword("wrong", a) || VerifyPassword(p, DummyVerifier) {
		t.Fatal("verifier mismatch")
	}
	for _, v := range []string{"", a + "$extra", strings.Replace(a, "600000", "1", 1), strings.Replace(a, "v1", "v2", 1), verifierPrefix + "a$b"} {
		if VerifyPassword(p, v) {
			t.Fatal("malformed verifier accepted")
		}
	}
	token, e := Random(32)
	if e != nil || len(token) != 43 || len(Digest(token)) != 64 || Digest(token) == token {
		t.Fatal("token policy")
	}
	raw, _ := json.Marshal(Profile{ID: "id", Email: "e", DisplayName: "n", Role: "bidder"})
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	if len(fields) != 5 {
		t.Fatal(string(raw))
	}
}
func TestLimiterBoundsAndRecovery(t *testing.T) {
	now := time.Unix(1, 0)
	l := NewLimiter()
	l.Now = func() time.Time { return now }
	for i := 0; i < 10; i++ {
		done, ok := l.Enter("127.0.0.1:123")
		if !ok {
			t.Fatal(i)
		}
		done()
	}
	if _, ok := l.Enter("127.0.0.1:999"); ok {
		t.Fatal("port bypass")
	}
	now = now.Add(time.Minute)
	done, ok := l.Enter("127.0.0.1:1")
	if !ok {
		t.Fatal("permanent lockout")
	}
	done()
	l = NewLimiter()
	var release []func()
	for i := 0; i < 4; i++ {
		f, ok := l.Enter(fmt.Sprintf("192.0.2.%d:1", i))
		if !ok {
			t.Fatal(i)
		}
		release = append(release, f)
	}
	if _, ok := l.Enter("192.0.2.9:1"); ok {
		t.Fatal("unbounded work")
	}
	for _, f := range release {
		f()
	}
	l = NewLimiter()
	l.Now = func() time.Time { return now }
	for i := 0; i < 4096; i++ {
		f, ok := l.Enter(fmt.Sprint(i))
		if !ok {
			t.Fatal(i)
		}
		f()
	}
	if _, ok := l.Enter("new"); ok {
		t.Fatal("unbounded peers")
	}
	if len(l.peers) != 4096 {
		t.Fatal(len(l.peers))
	}
	now = now.Add(time.Minute)
	f, ok := l.Enter("new")
	if !ok {
		t.Fatal("no expiry")
	}
	f()
}
