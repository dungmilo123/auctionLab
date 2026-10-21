package store

import (
	"auctionlab/internal/analytics"
	"auctionlab/internal/catalog"
	"context"
	"database/sql"
	"errors"
	"regexp"
)

type AnalyticsRepository struct{ Store *Store }

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

const cohortSQL = `WITH cohort AS (SELECT l.id,l.demo OR e.demo AS demo,e.id AS event_id FROM catalog_lots l JOIN catalog_event_lots m ON m.lot_id=l.id JOIN catalog_events e ON e.id=m.event_id WHERE l.state='published' AND e.state='published' AND ($1='' OR e.id=$1) AND ($2='all' OR ($2='demo' AND (l.demo OR e.demo)) OR ($2='non-demo' AND NOT(l.demo OR e.demo)))) `

func readMetrics(ctx context.Context, tx *sql.Tx, event, dataset string) (analytics.Metrics, error) {
	var m analytics.Metrics
	// Independent scalar aggregates prevent bid x receipt multiplication. SUM(bigint)
	// stays PostgreSQL numeric and is scanned as text, without a per-order bound.
	e := tx.QueryRowContext(ctx, cohortSQL+`SELECT
 (SELECT count(*)::text FROM cohort),
 (SELECT count(*)::text FROM commerce_bids b JOIN cohort c ON c.id=b.lot_id),
 (SELECT count(DISTINCT b.account_id)::text FROM commerce_bids b JOIN cohort c ON c.id=b.lot_id),
 (SELECT count(*)::text FROM commerce_bids b JOIN cohort c ON c.id=b.lot_id JOIN commerce_accounts a ON a.id=b.account_id WHERE a.demo),
 (SELECT count(*)::text FROM commerce_bids b JOIN cohort c ON c.id=b.lot_id JOIN commerce_accounts a ON a.id=b.account_id WHERE NOT a.demo),
 (SELECT count(DISTINCT b.account_id)::text FROM commerce_bids b JOIN cohort c ON c.id=b.lot_id JOIN commerce_accounts a ON a.id=b.account_id WHERE a.demo),
 (SELECT count(DISTINCT b.account_id)::text FROM commerce_bids b JOIN cohort c ON c.id=b.lot_id JOIN commerce_accounts a ON a.id=b.account_id WHERE NOT a.demo),
 (SELECT count(*)::text FROM commerce_auction_results r JOIN cohort c ON c.id=r.lot_id WHERE r.outcome='sold'),
 (SELECT count(*)::text FROM commerce_auction_results r JOIN cohort c ON c.id=r.lot_id WHERE r.outcome='unsold'),
 (SELECT count(*)::text FROM commerce_auction_results r JOIN cohort c ON c.id=r.lot_id),
 (SELECT count(*)::text FROM cohort c WHERE NOT EXISTS(SELECT 1 FROM commerce_auction_results r WHERE r.lot_id=c.id)),
 (SELECT COALESCE(sum(r.winning_amount_minor),0)::text FROM commerce_auction_results r JOIN cohort c ON c.id=r.lot_id WHERE r.outcome='sold'),
 (SELECT count(*)::text FROM commerce_demo_payments p JOIN commerce_orders o ON o.id=p.order_id JOIN cohort c ON c.id=o.lot_id),
 (SELECT count(*)::text FROM commerce_orders o JOIN cohort c ON c.id=o.lot_id WHERE o.state='awaiting_payment'),
 (SELECT COALESCE(sum(o.winning_amount_minor),0)::text FROM commerce_orders o JOIN commerce_demo_payments p ON p.order_id=o.id JOIN cohort c ON c.id=o.lot_id),
 (SELECT COALESCE(sum(o.fulfillment_fee_minor),0)::text FROM commerce_orders o JOIN commerce_demo_payments p ON p.order_id=o.id JOIN cohort c ON c.id=o.lot_id),
 (SELECT COALESCE(sum(p.total_minor),0)::text FROM commerce_demo_payments p JOIN commerce_orders o ON o.id=p.order_id JOIN cohort c ON c.id=o.lot_id)`, event, dataset).Scan(&m.Lots, &m.Bids, &m.Bidders, &m.DemoBids, &m.OrdinaryBids, &m.DemoBidders, &m.OrdinaryBidders, &m.Sold, &m.Unsold, &m.Terminal, &m.Pending, &m.Winning, &m.PaidOrders, &m.Unpaid, &m.PaidWinning, &m.PaidFees, &m.PaidTotal)
	if e != nil {
		return m, e
	}
	for _, s := range []string{m.Lots, m.Bids, m.Bidders, m.DemoBids, m.OrdinaryBids, m.DemoBidders, m.OrdinaryBidders, m.Sold, m.Unsold, m.Terminal, m.Pending, m.Winning, m.PaidOrders, m.Unpaid, m.PaidWinning, m.PaidFees, m.PaidTotal} {
		if !decimal.MatchString(s) {
			return m, errors.New("invalid aggregate")
		}
	}
	if m.Terminal != "0" {
		m.SellThrough = &analytics.Ratio{Sold: m.Sold, Terminal: m.Terminal}
	}
	return m, nil
}
func (r AnalyticsRepository) Report(parent context.Context, f analytics.Filters) (analytics.Response, error) {
	out := analytics.Response{Data: analytics.Data{Events: []analytics.Event{}, Interactions: []analytics.InteractionCount{}, Definitions: analytics.Definitions}, Meta: analytics.Meta{Currency: "USD", Filters: f}}
	e := (AuctionRepository{r.Store}).read(parent, func(ctx context.Context, tx *sql.Tx) error {
		// The timestamp belongs to this repeatable-read transaction, never a later read.
		if e := tx.QueryRowContext(ctx, `SELECT (extract(epoch FROM transaction_timestamp())*1000)::bigint`).Scan(&out.Meta.AsOf); e != nil {
			return e
		}
		var coverage sql.NullInt64
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT collection_started_at FROM commerce_interaction_coverage WHERE id=1)`).Scan(&coverage); e != nil {
			return e
		}
		if coverage.Valid {
			out.Meta.CollectionStart = &coverage.Int64
		}
		rows, e := tx.QueryContext(ctx, `SELECT id,slug,title,demo FROM catalog_events WHERE state='published' AND ($1='all' OR slug=$1) ORDER BY slug COLLATE "C"`, f.Event)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			var ev analytics.Event
			if e = rows.Scan(&id, &ev.Slug, &ev.Title, &ev.Demo); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
			out.Data.Events = append(out.Data.Events, ev)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if f.Event != "all" && len(ids) == 0 {
			return catalog.Fail("not_found")
		}
		selection := ""
		if f.Event != "all" {
			selection = ids[0]
		}
		var corrupt bool
		e = tx.QueryRowContext(ctx, cohortSQL+`SELECT EXISTS(SELECT 1 FROM cohort c JOIN commerce_auction_results r ON r.lot_id=c.id LEFT JOIN commerce_orders o ON o.lot_id=r.lot_id LEFT JOIN commerce_demo_payments p ON p.order_id=o.id WHERE
   (r.outcome='sold' AND (o.id IS NULL OR o.winning_bid_id<>r.winning_bid_id OR o.owner_id<>r.winning_account_id OR o.winning_amount_minor<>r.winning_amount_minor OR o.closes_at<>r.closes_at OR o.recorded_at<>r.recorded_at OR o.currency<>'USD')) OR
   (r.outcome='unsold' AND o.id IS NOT NULL) OR
   (o.state='paid' AND (p.id IS NULL OR p.owner_id<>o.owner_id OR p.total_minor<>o.total_minor OR p.paid_at<>o.paid_at OR NOT p.simulated OR p.currency<>o.currency OR p.payment_method IS DISTINCT FROM o.selection->>'payment_method' OR o.total_minor<>o.winning_amount_minor+o.fulfillment_fee_minor OR o.selection->>'fulfillment_method' IS NULL OR o.selection->>'fulfillment_method' NOT IN ('shipping','pickup') OR o.fulfillment_fee_minor IS DISTINCT FROM CASE WHEN o.selection->>'fulfillment_method'='pickup' THEN 0 ELSE (o.snapshot->'fulfillment'->>'shipping_fee_minor')::bigint END)) OR
   (o.state='awaiting_payment' AND p.id IS NOT NULL))`, selection, f.Dataset).Scan(&corrupt)
		if e != nil {
			return e
		}
		if corrupt {
			return errors.New("inconsistent aggregate sources")
		}
		out.Data.Summary, e = readMetrics(ctx, tx, selection, f.Dataset)
		if e != nil {
			return e
		}
		for i, id := range ids {
			out.Data.Events[i].Metrics, e = readMetrics(ctx, tx, id, f.Dataset)
			if e != nil {
				return e
			}
		}
		rows, e = tx.QueryContext(ctx, cohortSQL+`SELECT i.kind,i.provenance,count(*)::text FROM commerce_interactions i JOIN cohort c ON c.id=i.lot_id GROUP BY i.kind,i.provenance ORDER BY i.kind COLLATE "C",i.provenance COLLATE "C"`, selection, f.Dataset)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var n analytics.InteractionCount
			if e = rows.Scan(&n.Kind, &n.Provenance, &n.Count); e != nil {
				return e
			}
			if !decimal.MatchString(n.Count) {
				return errors.New("invalid interaction aggregate")
			}
			out.Data.Interactions = append(out.Data.Interactions, n)
		}
		return rows.Err()
	})
	return out, sanitize("analytics", e)
}
