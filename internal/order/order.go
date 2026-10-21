// Package order implements private single-winner checkout and local simulation.
package order

import (
	"auctionlab/internal/catalog"
	"context"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxTotal int64 = 2 * catalog.MaxMoney

type Snapshot struct {
	LotID       string              `json:"lot_id"`
	EventID     string              `json:"event_id"`
	Slug        string              `json:"slug"`
	Title       string              `json:"title"`
	Condition   string              `json:"condition"`
	Image       catalog.Image       `json:"image"`
	Fulfillment catalog.Fulfillment `json:"fulfillment"`
	LotDemo     bool                `json:"lot_demo"`
	EventDemo   bool                `json:"event_demo"`
	AccountDemo bool                `json:"account_demo"`
}
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	Locality   string `json:"locality"`
	Region     string `json:"region"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}
type Selection struct {
	Contact           Contact  `json:"contact"`
	FulfillmentMethod string   `json:"fulfillment_method"`
	ShippingAddress   *Address `json:"shipping_address,omitempty"`
	PaymentMethod     string   `json:"payment_method"`
}
type Intent struct {
	Selection
	RequestID string `json:"request_id"`
}
type Total struct {
	FulfillmentMethod   string `json:"fulfillment_method"`
	Instructions        string `json:"instructions"`
	WinningAmountMinor  int64  `json:"winning_amount_minor"`
	FulfillmentFeeMinor int64  `json:"fulfillment_fee_minor"`
	TotalMinor          int64  `json:"total_minor"`
}
type Receipt struct {
	ID            string `json:"id"`
	OrderID       string `json:"order_id"`
	OwnerID       string `json:"owner_id"`
	PaymentMethod string `json:"payment_method"`
	Currency      string `json:"currency"`
	TotalMinor    int64  `json:"total_minor"`
	PaidAt        int64  `json:"paid_at"`
	Simulated     bool   `json:"simulated"`
}
type Order struct {
	ID                 string     `json:"id"`
	OwnerID            string     `json:"owner_id"`
	WinningBidID       int64      `json:"winning_bid_id"`
	WinningAmountMinor int64      `json:"winning_amount_minor"`
	Currency           string     `json:"currency"`
	Snapshot           Snapshot   `json:"snapshot"`
	ClosesAt           int64      `json:"closes_at"`
	RecordedAt         int64      `json:"recorded_at"`
	CreatedAt          int64      `json:"created_at"`
	State              string     `json:"state"`
	Options            []Total    `json:"options"`
	Selection          *Selection `json:"selection"`
	Total              *Total     `json:"total"`
	Receipt            *Receipt   `json:"receipt"`
	Fulfillment        Progress   `json:"fulfillment"`
}
type Review struct {
	Order     Order      `json:"order"`
	Selection *Selection `json:"selection"`
	Total     *Total     `json:"total"`
}
type Confirmation struct {
	Order       Order `json:"order"`
	Replay      bool  `json:"replay"`
	AlreadyPaid bool  `json:"already_paid"`
}
type Error struct {
	Code   string
	Fields map[string]string
}

func (e *Error) Error() string { return "order: " + e.Code }
func Fail(code string) error   { return &Error{Code: code} }

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var country = regexp.MustCompile(`^[A-Z]{2}$`)

func ValidRequestID(s string) bool { return uuid.MatchString(s) }
func Price(o Order, method string) (Total, error) {
	t := Total{FulfillmentMethod: method, WinningAmountMinor: o.WinningAmountMinor}
	f := o.Snapshot.Fulfillment
	switch method {
	case "shipping":
		if !f.ShippingEnabled {
			return t, &Error{Code: "validation", Fields: map[string]string{"fulfillment_method": "unavailable"}}
		}
		if f.ShippingFeeMinor == nil || strings.TrimSpace(f.ShippingInstructions) == "" {
			return t, Fail("unavailable")
		}
		t.FulfillmentFeeMinor = *f.ShippingFeeMinor
		t.Instructions = f.ShippingInstructions
	case "pickup":
		if !f.PickupEnabled {
			return t, &Error{Code: "validation", Fields: map[string]string{"fulfillment_method": "unavailable"}}
		}
		if strings.TrimSpace(f.PickupInstructions) == "" {
			return t, Fail("unavailable")
		}
		t.Instructions = f.PickupInstructions
	default:
		return t, &Error{Code: "validation", Fields: map[string]string{"fulfillment_method": "invalid"}}
	}
	if o.Currency != "USD" || t.WinningAmountMinor < 0 || t.WinningAmountMinor > catalog.MaxMoney || t.FulfillmentFeeMinor < 0 || t.FulfillmentFeeMinor > catalog.MaxMoney {
		return t, Fail("unavailable")
	}
	t.TotalMinor = t.WinningAmountMinor + t.FulfillmentFeeMinor
	return t, nil
}
func Options(o Order) ([]Total, error) {
	out := []Total{}
	for _, m := range []string{"shipping", "pickup"} {
		if m == "shipping" && !o.Snapshot.Fulfillment.ShippingEnabled || m == "pickup" && !o.Snapshot.Fulfillment.PickupEnabled {
			continue
		}
		t, e := Price(o, m)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, Fail("unavailable")
	}
	return out, nil
}
func Normalize(s Selection) (Selection, error) {
	fields := map[string]string{}
	clean := func(key string, p *string, limit int, required bool) {
		hasControl := strings.IndexFunc(*p, unicode.IsControl) >= 0
		*p = strings.TrimSpace(*p)
		if !utf8.ValidString(*p) || hasControl || utf8.RuneCountInString(*p) > limit {
			fields[key] = "invalid"
		} else if required && *p == "" {
			fields[key] = "required"
		}
	}
	clean("contact.name", &s.Contact.Name, 120, true)
	clean("contact.email", &s.Contact.Email, 254, true)
	s.Contact.Email = strings.ToLower(s.Contact.Email)
	a, e := mail.ParseAddress(s.Contact.Email)
	if e != nil || a.Address != s.Contact.Email || !strings.Contains(s.Contact.Email, "@") {
		fields["contact.email"] = "invalid"
	}
	clean("contact.phone", &s.Contact.Phone, 40, true)
	if s.PaymentMethod != "demo-card" && s.PaymentMethod != "demo-bank-transfer" {
		fields["payment_method"] = "invalid"
	}
	if s.FulfillmentMethod == "pickup" {
		s.ShippingAddress = nil
	} else if s.FulfillmentMethod == "shipping" {
		a := Address{}
		if s.ShippingAddress != nil {
			a = *s.ShippingAddress
		}
		s.ShippingAddress = &a
		clean("shipping_address.line1", &a.Line1, 200, true)
		clean("shipping_address.line2", &a.Line2, 200, false)
		clean("shipping_address.locality", &a.Locality, 120, true)
		clean("shipping_address.region", &a.Region, 120, false)
		clean("shipping_address.postal_code", &a.PostalCode, 32, false)
		clean("shipping_address.country", &a.Country, 2, true)
		a.Country = strings.ToUpper(a.Country)
		if !country.MatchString(a.Country) {
			fields["shipping_address.country"] = "invalid"
		}
	} else {
		fields["fulfillment_method"] = "invalid"
	}
	if len(fields) > 0 {
		return s, &Error{Code: "validation", Fields: fields}
	}
	return s, nil
}
func MakeReview(o Order, s Selection) (Review, error) {
	r := Review{Order: o}
	if o.State == "paid" {
		r.Selection = o.Selection
		r.Total = o.Total
		return r, nil
	}
	n, e := Normalize(s)
	if e != nil {
		return r, e
	}
	t, e := Price(o, n.FulfillmentMethod)
	if e != nil {
		return r, e
	}
	r.Selection = &n
	r.Total = &t
	return r, nil
}

type Repository interface {
	Wins(context.Context, string, func() time.Time) ([]Order, error)
	History(context.Context, string, func() time.Time) ([]Summary, error)
	AdminList(context.Context, Filters) ([]Summary, error)
	AdminGet(context.Context, string) (Order, error)
	Transition(context.Context, string, Transition, func() time.Time) (Order, error)
	Get(context.Context, string, string, func() time.Time) (Order, error)
	Confirm(context.Context, string, string, Intent, func() time.Time) (Confirmation, error)
}
type Service struct {
	Repo Repository
	Now  func() time.Time
}

func New(r Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{r, now}
}
