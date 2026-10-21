package store

import (
	"auctionlab/internal/auction"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type AuctionRepository struct{ Store *Store }

func (r AuctionRepository) write(parent context.Context, fn func(context.Context, *sql.Tx) error) error {
	return sanitize("auction write", r.Store.withWriter(func(ctx context.Context, c *sql.Conn) error {
		if e := c.PingContext(ctx); e != nil {
			return e
		}
		tx, e := c.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = fn(ctx, tx); e != nil {
			return e
		}
		return sanitize("auction commit", tx.Commit())
	}, parent))
}

const publishedSchedule = `SELECT l.id,e.id,e.opens_at,e.first_closes_at+m.position*e.close_interval_ms,l.starting_price_minor,l.bid_increment_minor FROM catalog_lots l JOIN catalog_event_lots m ON m.lot_id=l.id JOIN catalog_events e ON e.id=m.event_id WHERE l.state='published' AND e.state='published'`
const initializeAuction = `INSERT INTO commerce_lot_auctions(lot_id,event_id,opens_at,closes_at,starting_price_minor,bid_increment_minor) `

func (r AuctionRepository) Reconcile(ctx context.Context, now func() time.Time) (int, error) {
	n := 0
	e := r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, initializeAuction+publishedSchedule+` AND NOT EXISTS(SELECT 1 FROM commerce_lot_auctions a WHERE a.lot_id=l.id) ORDER BY l.id LIMIT 100 ON CONFLICT(lot_id) DO NOTHING`)
		if e != nil {
			return e
		}
		c, e := res.RowsAffected()
		if e != nil {
			return e
		}
		n += int(c)
		rows, e := tx.QueryContext(ctx, `SELECT lot_id FROM commerce_lot_auctions WHERE (state='scheduled' AND opens_at<=$1) OR (state='open' AND closes_at<=$1) ORDER BY closes_at,lot_id LIMIT 100 FOR UPDATE`, now().UnixMilli())
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			if e = advanceAuction(ctx, tx, id, now().UnixMilli()); e != nil {
				return e
			}
			n++
		}
		rows, e = tx.QueryContext(ctx, `SELECT r.lot_id FROM commerce_auction_results r WHERE r.outcome='sold' AND NOT EXISTS(SELECT 1 FROM commerce_orders o WHERE o.lot_id=r.lot_id) ORDER BY r.recorded_at,r.lot_id LIMIT 100`)
		if e != nil {
			return e
		}
		ids = nil
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			if e = insertWinnerOrder(ctx, tx, id, now().UnixMilli()); e != nil {
				return e
			}
			n++
		}
		return nil
	})
	return n, e
}
func advanceAuction(ctx context.Context, tx *sql.Tx, id string, now int64) error {
	// The caller holds the auction row lock (or just inserted the row).
	var state string
	var opens, closes int64
	var leader sql.NullInt64
	if e := tx.QueryRowContext(ctx, `SELECT state,opens_at,closes_at,leading_bid_id FROM commerce_lot_auctions WHERE lot_id=$1`, id).Scan(&state, &opens, &closes, &leader); e != nil {
		return e
	}
	if state == "sold" || state == "unsold" {
		return nil
	}
	if now >= closes {
		outcome := "unsold"
		if leader.Valid {
			outcome = "sold"
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO commerce_auction_results(lot_id,outcome,closes_at,recorded_at,winning_bid_id,winning_account_id,winning_amount_minor) SELECT a.lot_id,$2,a.closes_at,$3,b.id,b.account_id,b.amount_minor FROM commerce_lot_auctions a LEFT JOIN commerce_bids b ON b.id=a.leading_bid_id WHERE a.lot_id=$1`, id, outcome, now)
		if e != nil {
			return e
		}
		if outcome == "sold" {
			if e = insertWinnerOrder(ctx, tx, id, now); e != nil {
				return e
			}
		}
		_, e = tx.ExecContext(ctx, `UPDATE commerce_lot_auctions SET state=$2,revision=revision+1 WHERE lot_id=$1`, id, outcome)
		return e
	}
	if state == "scheduled" && now >= opens {
		_, e := tx.ExecContext(ctx, `UPDATE commerce_lot_auctions SET state='open',revision=revision+1 WHERE lot_id=$1`, id)
		return e
	}
	return nil
}
func (r AuctionRepository) Place(ctx context.Context, digest string, i auction.Intent, now func() time.Time) (auction.Receipt, error) {
	var receipt auction.Receipt
	if e := auction.Validate(i); e != nil {
		return receipt, e
	}
	e := r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		p, e := sessionProfile(ctx, tx, digest, now().UnixMilli())
		if e != nil {
			return e
		}
		var published bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(`+publishedSchedule+` AND l.id=$1)`, i.LotID).Scan(&published)
		if e != nil {
			return e
		}
		if !published {
			return auction.Fail("not_found")
		}
		var oldLot string
		var old auction.OwnBid
		e = tx.QueryRowContext(ctx, `SELECT lot_id,sequence,amount_minor,accepted_at,request_id FROM commerce_bids WHERE account_id=$1 AND request_id=$2`, p.ID, i.RequestID).Scan(&oldLot, &old.Sequence, &old.AmountMinor, &old.AcceptedAt, &old.RequestID)
		if e == nil {
			if oldLot != i.LotID || old.AmountMinor != *i.AmountMinor {
				return auction.Fail("request_conflict")
			}
			receipt = auction.Receipt{LotID: oldLot, Bid: old, Replay: true}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if _, e = tx.ExecContext(ctx, initializeAuction+publishedSchedule+` AND l.id=$1 ON CONFLICT(lot_id) DO NOTHING`, i.LotID); e != nil {
			return e
		}
		var opens, closes, opening, increment, count int64
		var state string
		var leaderAccount sql.NullString
		var top sql.NullInt64
		e = tx.QueryRowContext(ctx, `SELECT a.opens_at,a.closes_at,a.starting_price_minor,a.bid_increment_minor,a.bid_count,a.state,b.account_id,b.amount_minor FROM commerce_lot_auctions a LEFT JOIN commerce_bids b ON b.id=a.leading_bid_id WHERE a.lot_id=$1 FOR UPDATE OF a`, i.LotID).Scan(&opens, &closes, &opening, &increment, &count, &state, &leaderAccount, &top)
		if e != nil {
			return e
		}
		instant := now().UnixMilli()
		if _, e = sessionProfile(ctx, tx, digest, instant); e != nil {
			return e
		}
		if instant < opens {
			return auction.Fail("not_open")
		}
		if instant >= closes || state == "sold" || state == "unsold" {
			return auction.Fail("closed")
		}
		if leaderAccount.Valid && leaderAccount.String == p.ID {
			return auction.Fail("already_leading")
		}
		var current *int64
		if top.Valid {
			current = &top.Int64
		}
		minimum, exhausted := auction.Minimum(opening, increment, current)
		if exhausted {
			return auction.Fail("amount_limit_reached")
		}
		if *i.AmountMinor < *minimum {
			return auction.Fail("minimum_not_met")
		}
		if e = advanceAuction(ctx, tx, i.LotID, instant); e != nil {
			return e
		}
		var alias int64
		e = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT bidder_number FROM commerce_bids WHERE lot_id=$1 AND account_id=$2 LIMIT 1),(SELECT COALESCE(max(bidder_number),0)+1 FROM commerce_bids WHERE lot_id=$1))`, i.LotID, p.ID).Scan(&alias)
		if e != nil {
			return e
		}
		var id int64
		own := auction.OwnBid{Sequence: count + 1, AmountMinor: *i.AmountMinor, AcceptedAt: instant, RequestID: i.RequestID}
		e = tx.QueryRowContext(ctx, `INSERT INTO commerce_bids(lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, i.LotID, p.ID, own.Sequence, own.AmountMinor, instant, i.RequestID, alias).Scan(&id)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE commerce_lot_auctions SET leading_bid_id=$2,bid_count=bid_count+1,revision=revision+1 WHERE lot_id=$1`, i.LotID, id)
		if e != nil {
			return e
		}
		receipt = auction.Receipt{LotID: i.LotID, Bid: own}
		return nil
	})
	if e != nil {
		return auction.Receipt{}, e
	}
	return receipt, nil
}
func (r AuctionRepository) read(parent context.Context, fn func(context.Context, *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	tx, e := r.Store.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return sanitize("auction read", e)
	}
	defer tx.Rollback()
	if e = fn(ctx, tx); e != nil {
		return sanitize("auction read", e)
	}
	return sanitize("auction read", tx.Commit())
}
func publicHistory(ctx context.Context, tx *sql.Tx, id string, before int64) (auction.History, error) {
	h := auction.History{Bids: []auction.Bid{}}
	rows, e := tx.QueryContext(ctx, `SELECT sequence,amount_minor,accepted_at,bidder_number FROM commerce_bids WHERE lot_id=$1 AND ($2::bigint=0 OR sequence<$2) ORDER BY sequence DESC LIMIT 51`, id, before)
	if e != nil {
		return h, e
	}
	defer rows.Close()
	for rows.Next() {
		var b auction.Bid
		var alias int64
		if e = rows.Scan(&b.Sequence, &b.AmountMinor, &b.AcceptedAt, &alias); e != nil {
			return h, e
		}
		b.Alias = fmt.Sprintf("Bidder %d", alias)
		h.Bids = append(h.Bids, b)
	}
	if e = rows.Err(); e != nil {
		return h, e
	}
	if len(h.Bids) > auction.HistoryLimit {
		h.Bids = h.Bids[:auction.HistoryLimit]
		h.HasMore = true
		v := h.Bids[len(h.Bids)-1].Sequence
		h.BeforeSequence = &v
	}
	return h, nil
}
func readSnapshot(ctx context.Context, tx *sql.Tx, id string, now func() time.Time) (auction.Snapshot, error) {
	s := auction.Snapshot{LotID: id, Currency: "USD", History: []auction.Bid{}}
	var state sql.NullString
	var leaderID sql.NullInt64
	e := tx.QueryRowContext(ctx, `SELECT e.opens_at,e.first_closes_at+m.position*e.close_interval_ms,l.starting_price_minor,l.bid_increment_minor,a.state,COALESCE(a.revision,0),COALESCE(a.bid_count,0),a.leading_bid_id FROM catalog_lots l JOIN catalog_event_lots m ON m.lot_id=l.id JOIN catalog_events e ON e.id=m.event_id LEFT JOIN commerce_lot_auctions a ON a.lot_id=l.id WHERE l.id=$1 AND l.state='published' AND e.state='published'`, id).Scan(&s.OpensAt, &s.ClosesAt, &s.StartingPriceMinor, &s.BidIncrementMinor, &state, &s.Revision, &s.BidCount, &leaderID)
	if errors.Is(e, sql.ErrNoRows) {
		return s, auction.Fail("not_found")
	}
	if e != nil {
		return s, e
	}
	s.ServerTime = now().UnixMilli()
	s.State = "pending"
	if state.Valid {
		s.State = state.String
		if s.State == "open" || s.State == "scheduled" {
			if s.ServerTime >= s.ClosesAt {
				s.State = "closing"
			} else if s.State == "scheduled" && s.ServerTime >= s.OpensAt {
				s.State = "opening"
			}
		}
	}
	if leaderID.Valid {
		var b auction.Bid
		var alias int64
		e = tx.QueryRowContext(ctx, `SELECT sequence,amount_minor,accepted_at,bidder_number FROM commerce_bids WHERE id=$1`, leaderID.Int64).Scan(&b.Sequence, &b.AmountMinor, &b.AcceptedAt, &alias)
		if e != nil {
			return s, e
		}
		b.Alias = fmt.Sprintf("Bidder %d", alias)
		s.Leader = &b
		s.CurrentPriceMinor = &b.AmountMinor
	}
	s.NextMinimumMinor, s.AmountLimitReached = auction.Minimum(s.StartingPriceMinor, s.BidIncrementMinor, s.CurrentPriceMinor)
	h, e := publicHistory(ctx, tx, id, 0)
	if e != nil {
		return s, e
	}
	s.History = h.Bids
	s.HistoryHasMore = h.HasMore
	if s.State == "sold" || s.State == "unsold" {
		var result auction.Result
		e = tx.QueryRowContext(ctx, `SELECT outcome,closes_at,recorded_at FROM commerce_auction_results WHERE lot_id=$1`, id).Scan(&result.Outcome, &result.ClosesAt, &result.RecordedAt)
		if e != nil {
			return s, e
		}
		if result.Outcome == "sold" {
			result.Winner = s.Leader
		}
		s.Result = &result
	}
	return s, nil
}
func (r AuctionRepository) Snapshot(ctx context.Context, id string, now func() time.Time) (auction.Snapshot, error) {
	var s auction.Snapshot
	e := r.read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var e error
		s, e = readSnapshot(ctx, tx, id, now)
		return e
	})
	return s, e
}
func (r AuctionRepository) History(ctx context.Context, id string, before int64) (auction.History, error) {
	var h auction.History
	e := r.read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, e := readSnapshot(ctx, tx, id, time.Now); e != nil {
			return e
		}
		var e error
		h, e = publicHistory(ctx, tx, id, before)
		return e
	})
	return h, e
}
func (r AuctionRepository) Participation(ctx context.Context, digest, id string, now func() time.Time) ([]auction.Participation, error) {
	out := []auction.Participation{}
	e := r.read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		p, e := sessionProfile(ctx, tx, digest, now().UnixMilli())
		if e != nil {
			return e
		}
		ids := []string{}
		if id != "" {
			ids = append(ids, id)
		} else {
			rows, e := tx.QueryContext(ctx, `SELECT lot_id FROM commerce_bids WHERE account_id=$1 GROUP BY lot_id ORDER BY max(accepted_at) DESC,lot_id COLLATE "C"`, p.ID)
			if e != nil {
				return e
			}
			for rows.Next() {
				var v string
				if e = rows.Scan(&v); e != nil {
					rows.Close()
					return e
				}
				ids = append(ids, v)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		for _, lotID := range ids {
			s, e := readSnapshot(ctx, tx, lotID, now)
			if e != nil {
				return e
			}
			row := auction.Participation{LotID: lotID, Status: "not_participating", Revision: s.Revision, CurrentPriceMinor: s.CurrentPriceMinor, Bids: []auction.OwnBid{}}
			if e = tx.QueryRowContext(ctx, `SELECT slug,title FROM catalog_lots WHERE id=$1`, lotID).Scan(&row.Slug, &row.Title); e != nil {
				return e
			}
			rows, e := tx.QueryContext(ctx, `SELECT sequence,amount_minor,accepted_at,request_id FROM commerce_bids WHERE lot_id=$1 AND account_id=$2 ORDER BY sequence DESC`, lotID, p.ID)
			if e != nil {
				return e
			}
			for rows.Next() {
				var b auction.OwnBid
				if e = rows.Scan(&b.Sequence, &b.AmountMinor, &b.AcceptedAt, &b.RequestID); e != nil {
					rows.Close()
					return e
				}
				row.Bids = append(row.Bids, b)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			if len(row.Bids) > 0 {
				row.Latest = &row.Bids[0]
				row.Status = "outbid"
				if s.State == "closing" {
					row.Status = "closing"
				} else if s.State == "sold" || s.State == "unsold" {
					row.Status = "lost"
					if s.Result != nil && s.Result.Winner != nil && s.Result.Winner.Sequence == row.Latest.Sequence {
						row.Status = "won"
					}
				} else if s.Leader != nil && s.Leader.Sequence == row.Latest.Sequence {
					row.Status = "leading"
				}
			}
			if row.Status == "won" {
				var orderID, orderState string
				e = tx.QueryRowContext(ctx, `SELECT id,state FROM commerce_orders WHERE lot_id=$1 AND owner_id=$2`, lotID, p.ID).Scan(&orderID, &orderState)
				if e == nil {
					row.OrderID = &orderID
					row.OrderState = &orderState
					row.CheckoutAvailable = orderState == "awaiting_payment"
				} else if !errors.Is(e, sql.ErrNoRows) {
					return e
				}
			}
			out = append(out, row)
		}
		return nil
	})
	return out, e
}
