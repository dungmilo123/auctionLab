package store

import (
	"auctionlab/internal/account"
	"auctionlab/internal/catalog"
	"context"
	"database/sql"
)

func (r AccountRepository) Membership(parent context.Context, digest, id string, saved *bool, now int64) (bool, error) {
	var result bool
	read := func(ctx context.Context, tx *sql.Tx) error {
		p, e := sessionProfile(ctx, tx, digest, now)
		if e != nil {
			return e
		}
		if saved != nil && !*saved {
			_, e = tx.ExecContext(ctx, `DELETE FROM commerce_watchlist WHERE account_id=$1 AND lot_id=$2`, p.ID, id)
			return e
		}
		var published bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_lots l JOIN catalog_event_lots m ON m.lot_id=l.id JOIN catalog_events e ON e.id=m.event_id WHERE l.id=$1 AND l.state='published' AND e.state='published')`, id).Scan(&published)
		if e != nil {
			return e
		}
		if !published {
			return account.Fail("not_found")
		}
		if saved != nil {
			_, e = tx.ExecContext(ctx, `INSERT INTO commerce_watchlist(account_id,lot_id,saved_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, p.ID, id, now)
			result = true
			return e
		}
		return tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_watchlist WHERE account_id=$1 AND lot_id=$2)`, p.ID, id).Scan(&result)
	}
	var e error
	if saved != nil {
		e = r.write(parent, read)
	} else {
		ctx, cancel := context.WithTimeout(parent, OperationTimeout)
		defer cancel()
		tx, err := r.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
		if err != nil {
			return false, sanitize("watch read", err)
		}
		defer tx.Rollback()
		e = read(ctx, tx)
	}
	return result, sanitize("watch membership", e)
}
func (r AccountRepository) Watchlist(parent context.Context, digest string, now int64) ([]catalog.PublicLot, error) {
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	tx, e := r.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return nil, sanitize("watch read", e)
	}
	defer tx.Rollback()
	p, e := sessionProfile(ctx, tx, digest, now)
	if e != nil {
		return nil, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT lot_id FROM commerce_watchlist WHERE account_id=$1 ORDER BY saved_at DESC,lot_id COLLATE "C" ASC`, p.ID)
	if e != nil {
		return nil, sanitize("watch rows", e)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, sanitize("watch row", e)
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, sanitize("watch rows", e)
	}
	data, e := (&catalogTx{ctx, tx}).Load()
	if e != nil {
		return nil, e
	}
	byID := map[string]catalog.PublicLot{}
	for _, lot := range catalog.PublishedLots(data) {
		byID[lot.ID] = lot
	}
	out := []catalog.PublicLot{}
	for _, id := range ids {
		if lot, ok := byID[id]; ok {
			out = append(out, lot)
		}
	}
	return out, nil
}

var _ account.Repository = AccountRepository{}
