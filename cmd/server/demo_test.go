package main

import (
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"bytes"
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestDemoOptions(t *testing.T) {
	valid := demoOptions{Prepare: true, Schema: "auctionlab_demo_test", SeedAt: "2026-10-21T00:00:00Z"}
	if e := valid.validate(false); e != nil {
		t.Fatal(e)
	}
	for _, o := range []demoOptions{{Schema: valid.Schema}, {Confirm: valid.Schema}, {Prepare: true, Schema: "auctionlab", SeedAt: valid.SeedAt}, {Prepare: true, Schema: valid.Schema}, {Status: true, Schema: valid.Schema, SeedAt: valid.SeedAt}, {Prepare: true, Reset: true, Schema: valid.Schema, SeedAt: valid.SeedAt}, {Reset: true, Schema: valid.Schema, SeedAt: valid.SeedAt}, {Prepare: true, Schema: valid.Schema, Confirm: valid.Schema, SeedAt: valid.SeedAt}} {
		if o.validate(false) == nil {
			t.Fatal(o)
		}
	}
	if valid.validate(true) == nil {
		t.Fatal("incompatible maintenance flags accepted")
	}
}
func TestDemoMaintenanceDrillAndNoCredentialRepeat(t *testing.T) {
	raw := testpg.NewURL(t)
	u, _ := url.Parse(raw)
	schema := "auctionlab_demo_" + u.Query().Get("search_path")
	t.Cleanup(func() {
		st, e := store.Open(raw)
		if e != nil {
			t.Error(e)
			return
		}
		defer st.Close()
		if _, e = st.DB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`); e != nil {
			t.Error(e)
		}
	})
	credentialReads := 0
	getenv := func(k string) string {
		switch k {
		case "DATABASE_URL":
			return os.Getenv("TEST_DATABASE_URL")
		case "DATABASE_SCHEMA":
			return "auctionlab"
		case "DEMO_BIDDER_ONE_PASSWORD", "DEMO_BIDDER_TWO_PASSWORD", "DEMO_ADMIN_PASSWORD":
			credentialReads++
			return "disposable-fixture-password"
		}
		return ""
	}
	o := demoOptions{Prepare: true, Schema: schema, SeedAt: "2026-10-21T00:00:00Z"}
	var out bytes.Buffer
	if e := demoMaintenance(o, getenv, &out); e != nil {
		t.Fatal(e)
	}
	if credentialReads != 3 || !strings.Contains(out.String(), `"status":"prepared"`) || strings.Contains(out.String(), "password") {
		t.Fatal("prepare contract")
	}
	out.Reset()
	credentialReads = 0
	if e := demoMaintenance(o, getenv, &out); e != nil {
		t.Fatal(e)
	}
	if credentialReads != 0 || !strings.Contains(out.String(), "already_prepared") {
		t.Fatal("repeat credential read")
	}
	// Maintenance has exited and released writer ownership, without creating game state.
	st, e := store.OpenConfig(store.Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: schema})
	if e != nil {
		t.Fatal(e)
	}
	var count int
	st.DB.QueryRow(`SELECT count(*) FROM game`).Scan(&count)
	if count != 0 {
		t.Fatal("started game lifecycle")
	}
	generation, e := st.DemoStatus(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	st.Close()
	out.Reset()
	if e = demoMaintenance(demoOptions{Status: true, Schema: schema}, getenv, &out); e != nil {
		t.Fatal(e)
	}
	if credentialReads != 0 || !strings.Contains(out.String(), generation.Generation) {
		t.Fatal("status wrote/read credentials")
	}
	out.Reset()
	reset := demoOptions{Reset: true, Schema: schema, Confirm: schema, SeedAt: "2026-10-22T00:00:00Z"}
	if e = demoMaintenance(reset, getenv, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), generation.Generation) || !strings.Contains(out.String(), `"status":"reset"`) {
		t.Fatal("no new generation")
	}
}
