package main

import (
	"auctionlab"
	"auctionlab/internal/account"
	"auctionlab/internal/api"
	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

type demoOptions struct {
	Prepare, Reset, Status  bool
	Schema, Confirm, SeedAt string
}

func (o demoOptions) active() bool { return o.Prepare || o.Reset || o.Status }
func (o demoOptions) validate(other bool) error {
	n := 0
	for _, v := range []bool{o.Prepare, o.Reset, o.Status} {
		if v {
			n++
		}
	}
	if n == 0 {
		if o.Schema != "" || o.Confirm != "" {
			return errors.New("demo options require a demo mode")
		}
		return nil
	}
	if n != 1 || other || !store.ValidDemoSchema(o.Schema) || o.Reset && o.Confirm != o.Schema || !o.Reset && o.Confirm != "" || o.Status && o.SeedAt != "" {
		return errors.New("demo mode requires its explicit disposable target and compatible options; reset also requires exact confirmation")
	}
	if !o.Status {
		at, e := time.Parse(time.RFC3339, o.SeedAt)
		if e != nil || at.UnixMilli() < 0 || at.UnixMilli() > catalog.MaxInstant-4*86400000 {
			return errors.New("demo write modes require --seed-at <RFC3339> within the supported schedule range")
		}
	}
	return nil
}
func demoMaintenance(o demoOptions, getenv func(string) string, out io.Writer) error {
	cfg, e := store.ConfigFromEnv(func(k string) string {
		if k == "DATABASE_SCHEMA" {
			return o.Schema
		}
		return getenv(k)
	})
	if e != nil {
		return e
	}
	cfg.ReadOnly = o.Status
	st, e := store.OpenConfig(cfg)
	if e != nil {
		return e
	}
	defer st.Close()
	ctx := context.Background()
	if o.Status {
		d, e := st.DemoStatus(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(d)
	}
	if o.Prepare {
		d, found, e := st.DemoPrepared(ctx)
		if e != nil {
			return e
		}
		if found {
			return json.NewEncoder(out).Encode(d)
		}
	} else {
		if _, e := st.DemoStatus(ctx); e != nil {
			return e
		}
	}
	svc := account.New(store.AccountRepository{Store: st})
	rows, e := svc.PrepareSeed([3]string{getenv("DEMO_BIDDER_ONE_PASSWORD"), getenv("DEMO_BIDDER_TWO_PASSWORD"), getenv("DEMO_ADMIN_PASSWORD")})
	if e != nil {
		return e
	}
	spa, e := auctionlab.SPA()
	if e != nil {
		return e
	}
	media := catalog.LocalMedia{ImagesDir: api.ImagesDir(getenv), Embedded: spa}
	// Validate media and seed contents transactionally; an initial failure may leave
	// only an empty migrated schema, never a disposable marker or partial records.
	if o.Prepare {
		if e = st.Migrate(); e != nil {
			return e
		}
	} else {
		v, e := st.Ready(ctx)
		if e != nil || v != store.UserVersion {
			return store.ErrDemoTarget
		}
	}
	reference, _ := time.Parse(time.RFC3339, o.SeedAt)
	d, e := st.SeedDemo(ctx, o.Reset, o.Confirm, reference, rows, media)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(d)
}
