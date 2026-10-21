package store

import (
	"auctionlab/internal/catalog"
	"auctionlab/internal/recommendation"
	"context"
	"database/sql"
)

type RecommendationRepository struct{ Store *Store }

func (r RecommendationRepository) Recommend(ctx context.Context, digest, source string, now int64) ([]recommendation.Item, error) {
	out := []recommendation.Item{}
	e := (AuctionRepository{r.Store}).read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		saved := map[string]bool{}
		if digest != "" {
			p, e := sessionProfile(ctx, tx, digest, now)
			if e != nil {
				return e
			}
			rows, e := tx.QueryContext(ctx, `SELECT lot_id FROM commerce_watchlist WHERE account_id=$1`, p.ID)
			if e != nil {
				return e
			}
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				saved[id] = true
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		data, e := (&catalogTx{ctx, tx}).Load()
		if e != nil {
			return e
		}
		lots := catalog.PublishedLots(data)
		var src *catalog.PublicLot
		if source != "" {
			for _, l := range lots {
				if l.Slug == source {
					v := l
					src = &v
					break
				}
			}
			if src == nil {
				return catalog.Fail("not_found")
			}
		}
		rows, e := tx.QueryContext(ctx, `SELECT lot_id FROM commerce_auction_results`)
		if e != nil {
			return e
		}
		terminal := map[string]bool{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			terminal[id] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		out = recommendation.Rank(lots, src, saved, terminal, now)
		return nil
	})
	return out, sanitize("recommendations", e)
}
