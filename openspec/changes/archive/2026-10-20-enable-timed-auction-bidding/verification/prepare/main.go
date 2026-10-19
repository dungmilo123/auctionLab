// Prepares only an explicitly owned disposable rehearsal schema, with no live writer.
package main

import (
	"auctionlab"
	"auctionlab/internal/api"
	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func ptr(v int64) *int64 { return &v }
func run() error {
	cfg, e := store.ConfigFromEnv(os.Getenv)
	if e != nil {
		return e
	}
	if !regexp.MustCompile(`^test_load_[a-f0-9]{32}$`).MatchString(cfg.Schema) {
		return fmt.Errorf("owned disposable schema required")
	}
	st, e := store.OpenConfig(cfg)
	if e != nil {
		return e
	}
	defer st.Close()
	if e = st.Migrate(); e != nil {
		return e
	}
	spa, e := auctionlab.SPA()
	if e != nil {
		return e
	}
	svc := catalog.New(store.CatalogRepository{Store: st}, time.Now, catalog.LocalMedia{ImagesDir: api.ImagesDir(os.Getenv), Embedded: spa})
	ctx := context.Background()
	now := time.Now()
	if _, e = svc.Seed(ctx, now.Add(-24*time.Hour)); e != nil {
		return e
	}
	data, e := svc.Snapshot(ctx)
	if e != nil {
		return e
	}
	out := []catalog.PublicEvent{}
	for i, event := range data.Events {
		if i >= 2 {
			continue
		}
		if event.State != "draft" {
			return fmt.Errorf("refusing to edit existing publication")
		}
		event.OpensAt = ptr(now.Add(10 * time.Second).UnixMilli())
		event.FirstClosesAt = ptr(now.Add(time.Duration(4+i*4) * time.Minute).UnixMilli())
		event.CloseIntervalMS = ptr(60000)
		updated, e := svc.SaveEvent(ctx, event.ID, event.Version, event.EventContent)
		if e != nil {
			return e
		}
		published, e := svc.Publish(ctx, event.ID, updated.Version)
		if e != nil {
			return e
		}
		out = append(out, published)
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
