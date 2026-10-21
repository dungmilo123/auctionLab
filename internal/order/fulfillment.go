package order

import (
	"auctionlab/internal/catalog"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Shipment struct {
	Carrier           string `json:"carrier"`
	TrackingReference string `json:"tracking_reference"`
}
type Event struct {
	Version    int64     `json:"version"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	OccurredAt int64     `json:"occurred_at"`
	Source     string    `json:"source"`
	Shipment   *Shipment `json:"shipment"`
}
type Progress struct {
	Status    string    `json:"status"`
	Version   int64     `json:"version"`
	UpdatedAt int64     `json:"updated_at"`
	Shipment  *Shipment `json:"shipment"`
	Timeline  []Event   `json:"timeline"`
}
type Summary struct {
	ID                 string        `json:"id"`
	OwnerID            string        `json:"owner_id"`
	LotID              string        `json:"lot_id"`
	EventID            string        `json:"event_id"`
	Slug               string        `json:"slug"`
	Title              string        `json:"title"`
	Image              catalog.Image `json:"image"`
	LotDemo            bool          `json:"lot_demo"`
	EventDemo          bool          `json:"event_demo"`
	AccountDemo        bool          `json:"account_demo"`
	WinningAmountMinor int64         `json:"winning_amount_minor"`
	Currency           string        `json:"currency"`
	ClosesAt           int64         `json:"closes_at"`
	RecordedAt         int64         `json:"recorded_at"`
	CreatedAt          int64         `json:"created_at"`
	State              string        `json:"state"`
	Status             string        `json:"status"`
	Version            int64         `json:"version"`
	Method             *string       `json:"method"`
	TotalMinor         *int64        `json:"total_minor"`
}

func Summarize(o Order) Summary {
	s := Summary{ID: o.ID, OwnerID: o.OwnerID, LotID: o.Snapshot.LotID, EventID: o.Snapshot.EventID, Slug: o.Snapshot.Slug, Title: o.Snapshot.Title, Image: o.Snapshot.Image, LotDemo: o.Snapshot.LotDemo, EventDemo: o.Snapshot.EventDemo, AccountDemo: o.Snapshot.AccountDemo, WinningAmountMinor: o.WinningAmountMinor, Currency: o.Currency, ClosesAt: o.ClosesAt, RecordedAt: o.RecordedAt, CreatedAt: o.CreatedAt, State: o.State, Status: o.Fulfillment.Status, Version: o.Fulfillment.Version}
	if o.State == "paid" {
		s.Method = &o.Selection.FulfillmentMethod
		s.TotalMinor = &o.Total.TotalMinor
	}
	return s
}

type Filters struct{ Status, Method string }

func (f Filters) Valid() bool {
	return validOne(f.Status, "all", "awaiting_payment", "paid", "preparing", "shipped", "ready_for_pickup", "completed") && validOne(f.Method, "all", "shipping", "pickup", "unselected")
}
func validOne(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func Next(method, status string) string {
	if status == "paid" {
		if method == "shipping" {
			return "preparing"
		}
		if method == "pickup" {
			return "ready_for_pickup"
		}
	}
	if method == "shipping" && status == "preparing" {
		return "shipped"
	}
	if method == "shipping" && status == "shipped" || method == "pickup" && status == "ready_for_pickup" {
		return "completed"
	}
	return ""
}

type Transition struct {
	ExpectedVersion int64     `json:"expected_version"`
	NextStatus      string    `json:"next_status"`
	Shipment        *Shipment `json:"shipment,omitempty"`
}

func NormalizeTransition(t Transition) (Transition, error) {
	fields := map[string]string{}
	if t.ExpectedVersion < 1 {
		fields["expected_version"] = "invalid"
	}
	if !validOne(t.NextStatus, "awaiting_payment", "paid", "preparing", "shipped", "ready_for_pickup", "completed") {
		fields["next_status"] = "invalid"
	}
	if t.NextStatus == "shipped" {
		if t.Shipment == nil {
			fields["shipment.carrier"] = "required"
			fields["shipment.tracking_reference"] = "required"
		} else {
			shipment := *t.Shipment
			t.Shipment = &shipment
			for _, f := range []struct {
				key   string
				p     *string
				limit int
			}{{"shipment.carrier", &shipment.Carrier, 120}, {"shipment.tracking_reference", &shipment.TrackingReference, 200}} {
				raw := *f.p
				*f.p = strings.TrimSpace(raw)
				if !utf8.ValidString(raw) || strings.IndexFunc(raw, unicode.IsControl) >= 0 || utf8.RuneCountInString(*f.p) > f.limit {
					fields[f.key] = "invalid"
				} else if *f.p == "" {
					fields[f.key] = "required"
				}
			}
		}
	} else if t.Shipment != nil {
		fields["shipment"] = "unexpected"
	}
	if len(fields) > 0 {
		return t, &Error{Code: "validation", Fields: fields}
	}
	return t, nil
}
