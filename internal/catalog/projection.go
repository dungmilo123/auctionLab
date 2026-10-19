package catalog

import "context"

// Public DTOs deliberately do not embed private storage content.
type PublicLot struct {
	ID                 string          `json:"id"`
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
	EventID            string          `json:"event_id"`
	EventSlug          string          `json:"event_slug"`
	Position           int             `json:"position"`
	OpensAt            *int64          `json:"opens_at"`
	ClosesAt           *int64          `json:"closes_at"`
	State              string          `json:"state"`
	Demo               bool            `json:"demo"`
}
type PublicEvent struct {
	ID              string      `json:"id"`
	Slug            string      `json:"slug"`
	Title           string      `json:"title"`
	Theme           string      `json:"theme"`
	Description     string      `json:"description"`
	CoverPath       string      `json:"cover_path"`
	CoverAlt        string      `json:"cover_alt"`
	OpensAt         *int64      `json:"opens_at"`
	FirstClosesAt   *int64      `json:"first_closes_at"`
	CloseIntervalMS *int64      `json:"close_interval_ms"`
	PlannedEnd      *int64      `json:"planned_end"`
	State           string      `json:"state"`
	Demo            bool        `json:"demo"`
	Lots            []PublicLot `json:"lots"`
}

func projectEvent(data Snapshot, e Event) PublicEvent {
	p := PublicEvent{ID: e.ID, Slug: e.Slug, Title: e.Title, Theme: e.Theme, Description: e.Description, CoverPath: e.CoverPath, CoverAlt: e.CoverAlt, OpensAt: e.OpensAt, FirstClosesAt: e.FirstClosesAt, CloseIntervalMS: e.CloseIntervalMS, State: e.State, Demo: e.Demo, Lots: []PublicLot{}}
	slots, _ := Schedule(e)
	for i, id := range e.LotIDs {
		l, err := findLot(data, id)
		if err != nil {
			continue
		}
		if e.State == "published" && l.State != "published" {
			continue
		}
		v := PublicLot{ID: l.ID, Slug: l.Slug, Title: l.Title, CategoryID: l.CategoryID, Description: l.Description, Brand: optional(l.Brand), Era: optional(l.Era), Story: optional(l.Story), Condition: l.Condition, Images: l.Images, Specifications: l.Specifications, Fulfillment: l.Fulfillment, StartingPriceMinor: l.StartingPriceMinor, BidIncrementMinor: l.BidIncrementMinor, Currency: l.Currency, EventID: e.ID, EventSlug: e.Slug, Position: i, State: l.State, Demo: l.Demo}
		if i < len(slots) {
			o, c := slots[i].OpensAt, slots[i].ClosesAt
			v.OpensAt = &o
			v.ClosesAt = &c
			p.PlannedEnd = &c
		}
		p.Lots = append(p.Lots, v)
	}
	return p
}
func (s *Service) PublicEvents(ctx context.Context) ([]PublicEvent, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	out := []PublicEvent{}
	for _, e := range data.Events {
		if e.State == "published" {
			out = append(out, projectEvent(data, e))
		}
	}
	return out, nil
}
func (s *Service) PublicEvent(ctx context.Context, slug string) (PublicEvent, error) {
	all, err := s.PublicEvents(ctx)
	if err != nil {
		return PublicEvent{}, err
	}
	for _, e := range all {
		if e.Slug == slug {
			return e, nil
		}
	}
	return PublicEvent{}, Fail("not_found")
}
func (s *Service) PublicLots(ctx context.Context) ([]PublicLot, error) {
	events, err := s.PublicEvents(ctx)
	if err != nil {
		return nil, err
	}
	out := []PublicLot{}
	for _, e := range events {
		for _, l := range e.Lots {
			if l.State == "published" {
				out = append(out, l)
			}
		}
	}
	return out, nil
}
func (s *Service) PublicLot(ctx context.Context, slug string) (PublicLot, error) {
	all, err := s.PublicLots(ctx)
	if err != nil {
		return PublicLot{}, err
	}
	for _, l := range all {
		if l.Slug == slug {
			return l, nil
		}
	}
	return PublicLot{}, Fail("not_found")
}

// PublishedLots projects an already coherent repository snapshot. Private readers
// reuse this boundary without issuing a separate public-catalog transaction.
func PublishedLots(data Snapshot) []PublicLot {
	out := []PublicLot{}
	for _, e := range data.Events {
		if e.State == "published" {
			out = append(out, projectEvent(data, e).Lots...)
		}
	}
	return out
}
