// Prepares only owned disposable acceptance schemas with a controlled fixture clock.
package main

import (
	"auctionlab"
	"auctionlab/internal/account"
	"auctionlab/internal/auction"
	"auctionlab/internal/catalog"
	"auctionlab/internal/order"
	"auctionlab/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"
)

func ptr(n int64) *int64 { return &n }
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
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
	ctx := context.Background()
	base := time.Now().Add(-20 * time.Minute)
	clock := func() time.Time { return base }
	svc := catalog.New(store.CatalogRepository{Store: st}, clock, catalog.LocalMedia{Embedded: spa})
	if _, e = svc.Seed(ctx, base.Add(-24*time.Hour)); e != nil {
		return e
	}
	data, e := svc.Snapshot(ctx)
	if e != nil {
		return e
	}
	as := account.New(store.AccountRepository{Store: st})
	as.Now = clock
	_, ta, e := as.Register(ctx, "checkout-a@example.test", os.Getenv("FIXTURE_PASSWORD"), "Rehearsal A", "")
	if e != nil {
		return e
	}
	_, tb, e := as.Register(ctx, "checkout-b@example.test", os.Getenv("FIXTURE_PASSWORD"), "Rehearsal B", "")
	if e != nil {
		return e
	}
	a := auction.New(store.AuctionRepository{Store: st}, func() time.Time { return base.Add(90 * time.Second) })
	for idx, event := range data.Events {
		if idx >= 2 {
			continue
		}
		if event.State != "draft" {
			return fmt.Errorf("refusing published edit")
		}
		for j, id := range event.LotIDs {
			l, e := svc.Lot(ctx, id)
			if e != nil {
				return e
			}
			l.StartingPriceMinor = ptr(12545)
			l.BidIncrementMinor = ptr(5)
			l.Fulfillment.ShippingFeeMinor = ptr(1500)
			if j == 1 {
				l.Fulfillment.ShippingEnabled = false
				l.Fulfillment.ShippingFeeMinor = nil
			}
			if j == 2 {
				l.StartingPriceMinor = ptr(0)
				l.Fulfillment.ShippingFeeMinor = ptr(0)
			}
			if _, e = svc.SaveLot(ctx, l.ID, l.Version, l.LotContent); e != nil {
				return e
			}
		}
		// Lot edits can advance the parent version; read before saving.
		event, e = svc.Event(ctx, event.ID)
		if e != nil {
			return e
		}
		event.OpensAt = ptr(base.Add(time.Minute).UnixMilli())
		event.FirstClosesAt = ptr(base.Add(2 * time.Minute).UnixMilli())
		event.CloseIntervalMS = ptr(1000)
		event, e = svc.SaveEvent(ctx, event.ID, event.Version, event.EventContent)
		if e != nil {
			return e
		}
		if _, e = svc.Publish(ctx, event.ID, event.Version); e != nil {
			return e
		}
		for j, id := range event.LotIDs {
			request := fmt.Sprintf("00000000-0000-0000-0000-%012d", idx*100+j+1)
			if j != 2 {
				if _, e = a.Place(ctx, account.Digest(ta), auction.Intent{LotID: id, AmountMinor: ptr(12545), RequestID: request}); e != nil {
					return e
				}
			}
			amount := int64(12550)
			if j == 2 {
				amount = 0
			}
			if _, e = a.Place(ctx, account.Digest(tb), auction.Intent{LotID: id, AmountMinor: ptr(amount), RequestID: request}); e != nil {
				return e
			}
		}
	}
	a.Now = time.Now
	if e = a.Reconcile(ctx); e != nil {
		return e
	}
	wins, e := (store.OrderRepository{Store: st}).Wins(ctx, account.Digest(tb), time.Now)
	if e != nil {
		return e
	}
	out := []map[string]any{}
	for _, v := range wins {
		out = append(out, map[string]any{"id": v.ID, "lot_id": v.Snapshot.LotID, "slug": v.Snapshot.Slug, "amount": v.WinningAmountMinor, "methods": v.Options})
	}
	_ = order.MaxTotal
	return json.NewEncoder(os.Stdout).Encode(out)
}
