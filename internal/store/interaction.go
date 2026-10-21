package store

import (
	"auctionlab/internal/interaction"
	"context"
	"database/sql"
	"time"
)

// insertInteraction runs only in the parent's existing leased writer transaction.
// Source keys bind accepted bids/receipts, fresh watch transitions and page UUIDs.
// Replays exit before this helper. Failure rolls back the authoritative action.
func insertInteraction(ctx context.Context, tx *sql.Tx, kind, lot, actor, key string, now int64) error {
	id, e := purchaseID()
	if e != nil {
		return e
	}
	result, e := tx.ExecContext(ctx, `INSERT INTO commerce_interactions(id,kind,lot_id,event_id,actor_id,occurred_at,recorded_at,lot_demo,event_demo,actor_demo,provenance,source_key)
 SELECT $1,$2,l.id,e.id,a.id,$6,$6,l.demo,e.demo,a.demo,'observed',$5 FROM catalog_lots l JOIN catalog_event_lots m ON m.lot_id=l.id JOIN catalog_events e ON e.id=m.event_id LEFT JOIN commerce_accounts a ON a.id=NULLIF($4,'') WHERE l.id=$3 AND l.state='published' AND e.state='published' ON CONFLICT(kind,source_key) DO NOTHING`, id, kind, lot, actor, key, now)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		var published bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_lots l JOIN catalog_event_lots m ON m.lot_id=l.id JOIN catalog_events e ON e.id=m.event_id WHERE l.id=$1 AND l.state='published' AND e.state='published')`, lot).Scan(&published); e != nil {
			return e
		}
		if !published {
			return interaction.ErrNotFound
		}
		var bound string
		if e = tx.QueryRowContext(ctx, `SELECT lot_id FROM commerce_interactions WHERE kind=$1 AND source_key=$2`, kind, key).Scan(&bound); e == sql.ErrNoRows {
			return interaction.ErrNotFound
		}
		if e != nil {
			return e
		}
		if bound != lot {
			return interaction.ErrConflict
		}
	}
	return nil
}
func initializeCoverage(ctx context.Context, tx *sql.Tx, now int64) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO commerce_interaction_coverage VALUES(1,$1) ON CONFLICT(id) DO NOTHING`, now)
	return e
}
func (s *Store) InitializeInteractionCoverage(ctx context.Context, now time.Time) error {
	return (AccountRepository{s}).write(ctx, func(ctx context.Context, tx *sql.Tx) error { return initializeCoverage(ctx, tx, now.UnixMilli()) })
}

type InteractionRepository struct{ Store *Store }

func (r InteractionRepository) Observe(ctx context.Context, lot, id string, now int64) error {
	return (AccountRepository{r.Store}).write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return insertInteraction(ctx, tx, "lot_view", lot, "", id, now)
	})
}
