// Package catalog models editorial preparation independently of the legacy game.
package catalog

import (
	"context"
	"time"
)

const MaxMoney int64 = 1000000000000
const MaxInstant int64 = 8640000000000000

type Category struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	NameEN    string `json:"name_en"`
	NameVI    string `json:"name_vi"`
	SortOrder int    `json:"sort_order"`
}
type Image struct {
	Path string `json:"path"`
	Alt  string `json:"alt"`
}
type Specification struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Fulfillment struct {
	ShippingEnabled      bool   `json:"shipping_enabled"`
	PickupEnabled        bool   `json:"pickup_enabled"`
	ShippingFeeMinor     *int64 `json:"shipping_fee_minor"`
	ShippingInstructions string `json:"shipping_instructions"`
	PickupInstructions   string `json:"pickup_instructions"`
}
type LotContent struct {
	Slug               string          `json:"slug"`
	Title              string          `json:"title"`
	CategoryID         *string         `json:"category_id"`
	Description        string          `json:"description"`
	Brand              *string         `json:"brand"`
	Era                *string         `json:"era"`
	Story              *string         `json:"story"`
	Condition          string          `json:"condition"`
	Images             []Image         `json:"images"`
	Specifications     []Specification `json:"specifications"`
	Fulfillment        Fulfillment     `json:"fulfillment"`
	StartingPriceMinor *int64          `json:"starting_price_minor"`
	BidIncrementMinor  *int64          `json:"bid_increment_minor"`
	Currency           string          `json:"currency"`
	AdminNotes         string          `json:"admin_notes"`
}
type Lot struct {
	LotContent
	ID        string  `json:"id"`
	State     string  `json:"state"`
	Version   int64   `json:"version"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
	Demo      bool    `json:"demo"`
	EventID   *string `json:"event_id"`
}
type EventContent struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Theme           string `json:"theme"`
	Description     string `json:"description"`
	CoverPath       string `json:"cover_path"`
	CoverAlt        string `json:"cover_alt"`
	OpensAt         *int64 `json:"opens_at"`
	FirstClosesAt   *int64 `json:"first_closes_at"`
	CloseIntervalMS *int64 `json:"close_interval_ms"`
	AdminNotes      string `json:"admin_notes"`
}
type Event struct {
	EventContent
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Version   int64    `json:"version"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
	Demo      bool     `json:"demo"`
	LotIDs    []string `json:"lot_ids"`
	Schedule  []Slot   `json:"schedule"`
}
type Slot struct {
	LotID    string `json:"lot_id"`
	Position int    `json:"position"`
	OpensAt  int64  `json:"opens_at"`
	ClosesAt int64  `json:"closes_at"`
}
type Snapshot struct {
	Categories []Category
	Lots       []Lot
	Events     []Event
}
type Reader interface{ Load() (Snapshot, error) }
type Tx interface {
	Reader
	PutLot(Lot, int64) error
	PutEvent(Event, int64) error
	ReplaceMembers(string, []string) error
	Seeded(int) (bool, error)
	MarkSeed(int, int64) error
}
type Repository interface {
	Read(context.Context, func(Reader) error) error
	Write(context.Context, func(Tx) error) error
}
type MediaResolver interface{ Readable(string) bool }
type Service struct {
	Repo  Repository
	Now   func() time.Time
	Media MediaResolver
}

func New(repo Repository, now func() time.Time, media MediaResolver) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo, now, media}
}

type Error struct {
	Code   string            `json:"code"`
	Fields map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return "catalog: " + e.Code }
func Fail(code string) error   { return &Error{Code: code} }
func invalid(fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}
	return &Error{Code: "validation", Fields: fields}
}
