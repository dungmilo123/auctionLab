package main

import (
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"bytes"
	"strings"
	"testing"
)

func TestAccountMaintenance(t *testing.T) {
	for _, tc := range []struct {
		catalog bool
		path    string
		verify  bool
		at      string
		tunnel  bool
	}{{true, "", false, "", false}, {false, "source", false, "", false}, {false, "", true, "", false}, {false, "", false, "date", false}, {false, "", false, "", true}} {
		if validateAccountSeedMode(true, tc.catalog, tc.path, tc.verify, tc.at, tc.tunnel) == nil {
			t.Fatal("conflicting flags")
		}
	}
	raw := testpg.NewURL(t)
	st, e := store.Open(raw)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	if e = st.Migrate(); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e = seedAccountMaintenance(st, func(string) string { return "" }, &out); e == nil || out.Len() != 0 {
		t.Fatal("missing seed credentials")
	}
	if other, e := store.Open(raw); e == nil {
		other.Close()
		t.Fatal("active writer accepted")
	}
	if e = seedAccountMaintenance(st, func(string) string { return "operator supplied fixture" }, &out); e != nil || strings.TrimSpace(out.String()) != `{"status":"seeded"}` {
		t.Fatal("seed maintenance", e)
	}
	out.Reset()
	if e = seedAccountMaintenance(st, func(string) string { t.Fatal("repeat read credentials"); return "" }, &out); e != nil || strings.TrimSpace(out.String()) != `{"status":"already_applied"}` {
		t.Fatal("repeat", e)
	}
}
