package store

import (
	"auctionlab/internal/order"
	"context"
	"database/sql"
	"reflect"
	"time"
)

func readProgress(ctx context.Context, tx *sql.Tx, o *order.Order) error {
	p := order.Progress{Status: "awaiting_payment", UpdatedAt: o.CreatedAt, Timeline: []order.Event{{Version: 0, FromStatus: "", ToStatus: "awaiting_payment", OccurredAt: o.CreatedAt, Source: "system"}}}
	if o.State == "awaiting_payment" {
		var occupied bool
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_order_fulfillment WHERE order_id=$1)`, o.ID).Scan(&occupied); e != nil {
			return e
		}
		if occupied {
			return order.Fail("unavailable")
		}
		o.Fulfillment = p
		return nil
	}
	var receipt, method string
	var carrier, reference sql.NullString
	e := tx.QueryRowContext(ctx, `SELECT receipt_id,method,status,version,updated_at,carrier,tracking_reference FROM commerce_order_fulfillment WHERE order_id=$1`, o.ID).Scan(&receipt, &method, &p.Status, &p.Version, &p.UpdatedAt, &carrier, &reference)
	if e != nil {
		return order.Fail("unavailable")
	}
	if receipt != o.Receipt.ID || method != o.Selection.FulfillmentMethod {
		return order.Fail("unavailable")
	}
	if carrier.Valid && reference.Valid {
		p.Shipment = &order.Shipment{Carrier: carrier.String, TrackingReference: reference.String}
	} else if carrier.Valid || reference.Valid {
		return order.Fail("unavailable")
	}
	rows, e := tx.QueryContext(ctx, `SELECT version,from_status,to_status,occurred_at,source,carrier,tracking_reference FROM commerce_order_fulfillment_events WHERE order_id=$1 ORDER BY version`, o.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	var shipment *order.Shipment
	previous := p.Timeline[0]
	for rows.Next() {
		var event order.Event
		var c, r sql.NullString
		if e = rows.Scan(&event.Version, &event.FromStatus, &event.ToStatus, &event.OccurredAt, &event.Source, &c, &r); e != nil {
			return e
		}
		if c.Valid && r.Valid {
			event.Shipment = &order.Shipment{Carrier: c.String, TrackingReference: r.String}
		} else if c.Valid || r.Valid {
			return order.Fail("unavailable")
		}
		expected := order.Next(method, previous.ToStatus)
		source := "operator"
		if event.Version == 1 {
			expected = "paid"
			source = "payment"
			if event.OccurredAt != o.Receipt.PaidAt {
				return order.Fail("unavailable")
			}
		}
		if event.Version != previous.Version+1 || event.FromStatus != previous.ToStatus || event.ToStatus != expected || event.Source != source || event.OccurredAt < previous.OccurredAt {
			return order.Fail("unavailable")
		}
		if event.ToStatus == "shipped" {
			normalized, err := order.NormalizeTransition(order.Transition{ExpectedVersion: 2, NextStatus: "shipped", Shipment: event.Shipment})
			if err != nil || !reflect.DeepEqual(normalized.Shipment, event.Shipment) {
				return order.Fail("unavailable")
			}
			shipment = event.Shipment
		} else if event.Shipment != nil {
			return order.Fail("unavailable")
		}
		p.Timeline = append(p.Timeline, event)
		previous = event
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if previous.Version != p.Version || previous.ToStatus != p.Status || previous.OccurredAt != p.UpdatedAt || !reflect.DeepEqual(shipment, p.Shipment) {
		return order.Fail("unavailable")
	}
	o.Fulfillment = p
	return nil
}
func initializeProgress(ctx context.Context, tx *sql.Tx, id, receipt, method string, now int64) error {
	if _, e := tx.ExecContext(ctx, `INSERT INTO commerce_order_fulfillment(order_id,receipt_id,method,status,version,updated_at) VALUES($1,$2,$3,'paid',1,$4)`, id, receipt, method, now); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO commerce_order_fulfillment_events(order_id,version,from_status,to_status,occurred_at,source) VALUES($1,1,'awaiting_payment','paid',$2,'payment')`, id, now)
	return e
}
func adminOrder(ctx context.Context, tx *sql.Tx, id string, lock bool) (order.Order, error) {
	var owner string
	if e := tx.QueryRowContext(ctx, `SELECT owner_id FROM commerce_orders WHERE id=$1`, id).Scan(&owner); e != nil {
		if e == sql.ErrNoRows {
			return order.Order{}, order.Fail("not_found")
		}
		return order.Order{}, e
	}
	return readOrder(ctx, tx, owner, id, lock)
}
func (r OrderRepository) AdminGet(ctx context.Context, id string) (order.Order, error) {
	var o order.Order
	e := (AuctionRepository{r.Store}).read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var e error
		o, e = adminOrder(ctx, tx, id, false)
		return e
	})
	return o, e
}
func (r OrderRepository) list(ctx context.Context, digest *string, filters order.Filters, now func() time.Time) ([]order.Summary, error) {
	out := []order.Summary{}
	if !filters.Valid() {
		return nil, order.Fail("validation")
	}
	e := (AuctionRepository{r.Store}).read(ctx, func(ctx context.Context, tx *sql.Tx) error {
		owner := ""
		if digest != nil {
			p, e := sessionProfile(ctx, tx, *digest, now().UnixMilli())
			if e != nil {
				return e
			}
			owner = p.ID
		}
		rows, e := tx.QueryContext(ctx, `SELECT o.id FROM commerce_orders o LEFT JOIN commerce_order_fulfillment f ON f.order_id=o.id WHERE ($1='' OR o.owner_id=$1) AND ($2='all' OR COALESCE(f.status,o.state)=$2) AND ($3='all' OR COALESCE(f.method,'unselected')=$3) ORDER BY o.recorded_at DESC,o.id COLLATE "C"`, owner, filters.Status, filters.Method)
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
			o, e := adminOrder(ctx, tx, id, false)
			if e != nil {
				return e
			}
			out = append(out, order.Summarize(o))
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func (r OrderRepository) History(ctx context.Context, digest string, now func() time.Time) ([]order.Summary, error) {
	return r.list(ctx, &digest, order.Filters{Status: "all", Method: "all"}, now)
}
func (r OrderRepository) AdminList(ctx context.Context, f order.Filters) ([]order.Summary, error) {
	return r.list(ctx, nil, f, time.Now)
}
func (r OrderRepository) Transition(ctx context.Context, id string, t order.Transition, now func() time.Time) (order.Order, error) {
	var out order.Order
	normalized, e := order.NormalizeTransition(t)
	if e != nil {
		return out, e
	}
	t = normalized
	e = (AuctionRepository{r.Store}).write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		o, e := adminOrder(ctx, tx, id, true)
		if e != nil {
			return e
		}
		if o.State != "paid" || o.Fulfillment.Version != t.ExpectedVersion || order.Next(o.Selection.FulfillmentMethod, o.Fulfillment.Status) != t.NextStatus {
			return order.Fail("transition_conflict")
		}
		instant := max(now().UnixMilli(), o.Fulfillment.UpdatedAt)
		var carrier, reference any
		if t.Shipment != nil {
			carrier = t.Shipment.Carrier
			reference = t.Shipment.TrackingReference
		} else if o.Fulfillment.Shipment != nil {
			carrier = o.Fulfillment.Shipment.Carrier
			reference = o.Fulfillment.Shipment.TrackingReference
		}
		if _, e = tx.ExecContext(ctx, `UPDATE commerce_order_fulfillment SET status=$2,version=version+1,updated_at=$3,carrier=$4,tracking_reference=$5 WHERE order_id=$1`, id, t.NextStatus, instant, carrier, reference); e != nil {
			return e
		}
		if t.Shipment == nil {
			carrier = nil
			reference = nil
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO commerce_order_fulfillment_events(order_id,version,from_status,to_status,occurred_at,source,carrier,tracking_reference) VALUES($1,$2,$3,$4,$5,'operator',$6,$7)`, id, t.ExpectedVersion+1, o.Fulfillment.Status, t.NextStatus, instant, carrier, reference); e != nil {
			return e
		}
		out, e = readOrder(ctx, tx, o.OwnerID, id, false)
		return e
	})
	if e != nil {
		return order.Order{}, e
	}
	return out, nil
}
