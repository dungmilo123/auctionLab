package store

import (
	"auctionlab/internal/order"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type OrderRepository struct{ Store *Store }

func purchaseID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}

// insertWinnerOrder runs inside the closure/adoption transaction, never a nested writer.
func insertWinnerOrder(ctx context.Context, tx *sql.Tx, lotID string, now int64) error {
	o := order.Order{Currency: "USD"}
	var fulfillment []byte
	e := tx.QueryRowContext(ctx, `SELECT r.lot_id,a.event_id,l.slug,l.title,l.condition,l.fulfillment,l.demo,e.demo,c.demo,r.winning_bid_id,r.winning_account_id,r.winning_amount_minor,r.closes_at,r.recorded_at FROM commerce_auction_results r JOIN commerce_lot_auctions a ON a.lot_id=r.lot_id JOIN catalog_lots l ON l.id=r.lot_id JOIN catalog_events e ON e.id=a.event_id JOIN commerce_accounts c ON c.id=r.winning_account_id WHERE r.lot_id=$1 AND r.outcome='sold'`, lotID).Scan(&o.Snapshot.LotID, &o.Snapshot.EventID, &o.Snapshot.Slug, &o.Snapshot.Title, &o.Snapshot.Condition, &fulfillment, &o.Snapshot.LotDemo, &o.Snapshot.EventDemo, &o.Snapshot.AccountDemo, &o.WinningBidID, &o.OwnerID, &o.WinningAmountMinor, &o.ClosesAt, &o.RecordedAt)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(fulfillment, &o.Snapshot.Fulfillment); e != nil {
		return e
	}
	e = tx.QueryRowContext(ctx, `SELECT path,alt FROM catalog_lot_images WHERE lot_id=$1 ORDER BY position LIMIT 1`, lotID).Scan(&o.Snapshot.Image.Path, &o.Snapshot.Image.Alt)
	if e != nil {
		return e
	}
	if o.Snapshot.Title == "" || o.Snapshot.Condition == "" || o.Snapshot.Image.Path == "" || o.Snapshot.Image.Alt == "" {
		return order.Fail("unavailable")
	}
	if _, e = order.Options(o); e != nil {
		return e
	}
	id, e := purchaseID()
	if e != nil {
		return e
	}
	raw, e := json.Marshal(o.Snapshot)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO commerce_orders(id,lot_id,winning_bid_id,owner_id,winning_amount_minor,currency,snapshot,closes_at,recorded_at,created_at) VALUES($1,$2,$3,$4,$5,'USD',$6,$7,$8,$9) ON CONFLICT(lot_id) DO NOTHING`, id, lotID, o.WinningBidID, o.OwnerID, o.WinningAmountMinor, string(raw), o.ClosesAt, o.RecordedAt, now)
	return e
}
func readOrder(ctx context.Context, tx *sql.Tx, owner, id string, lock bool) (order.Order, error) {
	var o order.Order
	var snap, selection []byte
	var storedFee, storedTotal, storedPaid sql.NullInt64
	q := `SELECT id,owner_id,winning_bid_id,winning_amount_minor,currency,snapshot,closes_at,recorded_at,created_at,state,selection,fulfillment_fee_minor,total_minor,paid_at FROM commerce_orders WHERE owner_id=$1 AND id=$2`
	if lock {
		q += ` FOR UPDATE`
	}
	e := tx.QueryRowContext(ctx, q, owner, id).Scan(&o.ID, &o.OwnerID, &o.WinningBidID, &o.WinningAmountMinor, &o.Currency, &snap, &o.ClosesAt, &o.RecordedAt, &o.CreatedAt, &o.State, &selection, &storedFee, &storedTotal, &storedPaid)
	if errors.Is(e, sql.ErrNoRows) {
		return o, order.Fail("not_found")
	}
	if e != nil {
		return o, e
	}
	if e = json.Unmarshal(snap, &o.Snapshot); e != nil {
		return o, e
	}
	o.Options, e = order.Options(o)
	if e != nil {
		return o, e
	}
	if o.State == "paid" {
		o.Selection = &order.Selection{}
		if e = json.Unmarshal(selection, o.Selection); e != nil {
			return o, e
		}
		t, e := order.Price(o, o.Selection.FulfillmentMethod)
		if e != nil {
			return o, e
		}
		o.Total = &t
		p := order.Receipt{}
		e = tx.QueryRowContext(ctx, `SELECT id,order_id,owner_id,payment_method,currency,total_minor,paid_at,simulated FROM commerce_demo_payments WHERE order_id=$1 AND owner_id=$2`, id, owner).Scan(&p.ID, &p.OrderID, &p.OwnerID, &p.PaymentMethod, &p.Currency, &p.TotalMinor, &p.PaidAt, &p.Simulated)
		if e != nil {
			return o, e
		}
		o.Receipt = &p
		if p.TotalMinor != t.TotalMinor || p.PaymentMethod != o.Selection.PaymentMethod || !storedFee.Valid || !storedTotal.Valid || !storedPaid.Valid || storedFee.Int64 != t.FulfillmentFeeMinor || storedTotal.Int64 != t.TotalMinor || storedPaid.Int64 != p.PaidAt {
			return o, order.Fail("unavailable")
		}
	}
	if e = readProgress(ctx, tx, &o); e != nil {
		return o, e
	}
	return o, nil
}
func (r OrderRepository) Get(ctx context.Context, digest, id string, now func() time.Time) (order.Order, error) {
	var o order.Order
	e := (AuctionRepository{r.Store}).read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		p, e := sessionProfile(ctx, tx, digest, now().UnixMilli())
		if e != nil {
			return e
		}
		o, e = readOrder(ctx, tx, p.ID, id, false)
		return e
	})
	return o, e
}
func (r OrderRepository) Wins(ctx context.Context, digest string, now func() time.Time) ([]order.Order, error) {
	out := []order.Order{}
	e := (AuctionRepository{r.Store}).read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		p, e := sessionProfile(ctx, tx, digest, now().UnixMilli())
		if e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, `SELECT id FROM commerce_orders WHERE owner_id=$1 AND state='awaiting_payment' ORDER BY recorded_at DESC,id COLLATE "C"`, p.ID)
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
			o, e := readOrder(ctx, tx, p.ID, id, false)
			if e != nil {
				return e
			}
			out = append(out, o)
		}
		return nil
	})
	return out, e
}
func (r OrderRepository) Confirm(parent context.Context, digest, id string, i order.Intent, now func() time.Time) (order.Confirmation, error) {
	var out order.Confirmation
	if !order.ValidRequestID(i.RequestID) {
		return out, &order.Error{Code: "validation", Fields: map[string]string{"request_id": "invalid"}}
	}
	// Normalize once before hashing; rejected intents reserve no request ID.
	n, validationErr := order.Normalize(i.Selection)
	raw, e := json.Marshal(struct {
		OrderID   string          `json:"order_id"`
		Selection order.Selection `json:"selection"`
	}{id, n})
	if e != nil {
		return out, e
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	e = (AuctionRepository{r.Store}).write(parent, func(ctx context.Context, tx *sql.Tx) error {
		p, e := sessionProfile(ctx, tx, digest, now().UnixMilli())
		if e != nil {
			return e
		}
		o, e := readOrder(ctx, tx, p.ID, id, true)
		if e != nil {
			return e
		}
		instant := now().UnixMilli()
		if _, e = sessionProfile(ctx, tx, digest, instant); e != nil {
			return e
		}
		var accepted string
		e = tx.QueryRowContext(ctx, `SELECT intent_digest FROM commerce_demo_payments WHERE owner_id=$1 AND request_id=$2`, p.ID, i.RequestID).Scan(&accepted)
		if e == nil {
			if accepted != hash {
				return order.Fail("request_conflict")
			}
			out = order.Confirmation{Order: o, Replay: true}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if o.State == "paid" {
			out = order.Confirmation{Order: o, AlreadyPaid: true}
			return nil
		}
		if validationErr != nil {
			return validationErr
		}
		review, e := order.MakeReview(o, n)
		if e != nil {
			return e
		}
		selection, e := json.Marshal(review.Selection)
		if e != nil {
			return e
		}
		receiptID, e := purchaseID()
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO commerce_demo_payments(id,order_id,owner_id,request_id,intent_digest,payment_method,currency,total_minor,paid_at,simulated) VALUES($1,$2,$3,$4,$5,$6,'USD',$7,$8,true)`, receiptID, id, p.ID, i.RequestID, hash, n.PaymentMethod, review.Total.TotalMinor, instant)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE commerce_orders SET state='paid',selection=$2,fulfillment_fee_minor=$3,total_minor=$4,paid_at=$5 WHERE id=$1`, id, string(selection), review.Total.FulfillmentFeeMinor, review.Total.TotalMinor, instant)
		if e != nil {
			return e
		}
		if e = insertInteraction(ctx, tx, "payment_confirmed", o.Snapshot.LotID, p.ID, receiptID, instant); e != nil {
			return e
		}
		if e = initializeProgress(ctx, tx, id, receiptID, n.FulfillmentMethod, instant); e != nil {
			return e
		}
		o, e = readOrder(ctx, tx, p.ID, id, false)
		out = order.Confirmation{Order: o}
		return e
	})
	if e != nil {
		return order.Confirmation{}, e
	}
	return out, nil
}
