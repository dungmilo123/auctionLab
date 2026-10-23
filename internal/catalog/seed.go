package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed seed_data.json
var seedData []byte

func number(n int64) *int64 { return &n }
func (s *Service) Seed(ctx context.Context, at time.Time) (string, error) {
	status := "seeded"
	err := s.Repo.Write(ctx, func(tx Tx) error { var e error; status, e = s.SeedTx(tx, at); return e })
	return status, err
}

// SeedTx participates in the caller transaction; it never acquires writer ownership.
func (s *Service) SeedTx(tx Tx, at time.Time) (string, error) {
	status := "seeded"
	err := func() error {
		applied, err := tx.Seeded(1)
		if err != nil {
			return err
		}
		if applied {
			status = "already_applied"
			return nil
		}
		reference := at.UnixMilli()
		if reference < 0 || reference > MaxInstant-4*24*60*60*1000 {
			return invalid(map[string]string{"seed_at": "schedule"})
		}
		data, err := tx.Load()
		if err != nil {
			return err
		}
		var records []struct {
			Title      string `json:"title"`
			CategoryID string `json:"category_id"`
		}
		if err = json.Unmarshal(seedData, &records); err != nil {
			return err
		}
		for group, title := range []string{"Analog Memories", "Objects of Time", "The Listening Room"} {
			eventID := fmt.Sprintf("demo-event-%d", group+1)
			_, slug, err := identity(title)
			if err != nil {
				return err
			}
			for _, e := range data.Events {
				if e.ID == eventID || e.Slug == slug {
					return Fail("seed_collision")
				}
			}
			opens := reference + int64(group+1)*24*60*60*1000
			event := Event{EventContent: EventContent{Slug: slug, Title: title, Theme: "A collection of vintage objects", Description: "Explore objects, materials and details from another time. Provenance information is unavailable.", CoverPath: fmt.Sprintf("/site-assets/catalog/event-%d.svg", group+1), CoverAlt: title + " collection", OpensAt: number(opens), FirstClosesAt: number(opens + 60*60*1000), CloseIntervalMS: number(60000)}, ID: eventID, State: "draft", Version: 1, CreatedAt: reference, UpdatedAt: reference, Demo: true, LotIDs: []string{}}
			for i := 0; i < 6; i++ {
				index := group*6 + i
				record := records[index]
				id := fmt.Sprintf("demo-lot-%02d", index+1)
				_, slug, err := identity(record.Title)
				if err != nil {
					return err
				}
				for _, l := range data.Lots {
					if l.ID == id || l.Slug == slug {
						return Fail("seed_collision")
					}
				}
				story := "Provenance information is unavailable."
				lot := Lot{LotContent: LotContent{Slug: slug, Title: record.Title, CategoryID: &record.CategoryID, Description: "A vintage object selected for the collection.", Story: &story, Condition: "Condition and operation have not been assessed.", Images: []Image{{Path: fmt.Sprintf("/site-assets/catalog/lot-%02d.svg", index+1), Alt: record.Title}, {Path: fmt.Sprintf("/site-assets/catalog/lot-%02d-detail.svg", index+1), Alt: record.Title + ", detail"}}, Specifications: []Specification{{Name: "Representation", Value: "Object study"}, {Name: "Authentication", Value: "Not independently verified"}}, Fulfillment: Fulfillment{ShippingEnabled: true, PickupEnabled: true, ShippingFeeMinor: number(1250), ShippingInstructions: "Protective packaging; delivery fee disclosed in USD.", PickupInstructions: "Pickup by arrangement; collection address is not provided."}, StartingPriceMinor: number(12550 + int64(index)*725), BidIncrementMinor: number(500), Currency: "USD"}, ID: id, State: "draft", Version: 1, CreatedAt: reference, UpdatedAt: reference, Demo: true}
				if err = validateLot(lot.LotContent, data.Categories, true, s.Media); err != nil {
					return err
				}
				if err = tx.PutLot(lot, 0); err != nil {
					return err
				}
				event.LotIDs = append(event.LotIDs, id)
			}
			// Completeness is checked without imposing publication's future-now rule.
			if err = validateEvent(event, true, reference, s.Media); err != nil {
				return err
			}
			if err = tx.PutEvent(event, 0); err != nil {
				return err
			}
			if err = tx.ReplaceMembers(eventID, event.LotIDs); err != nil {
				return err
			}
		}
		return tx.MarkSeed(1, reference)
	}()
	return status, err
}
