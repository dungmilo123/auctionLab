package catalog

import (
	"context"
	"errors"
	"strings"
)

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	var out Snapshot
	err := s.Repo.Read(ctx, func(r Reader) error { var err error; out, err = r.Load(); return err })
	return out, err
}
func findLot(data Snapshot, id string) (Lot, error) {
	for _, l := range data.Lots {
		if l.ID == id {
			return l, nil
		}
	}
	return Lot{}, Fail("not_found")
}
func findEvent(data Snapshot, id string) (Event, error) {
	for _, e := range data.Events {
		if e.ID == id {
			return e, nil
		}
	}
	return Event{}, Fail("not_found")
}
func (s *Service) Lot(ctx context.Context, id string) (Lot, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return Lot{}, err
	}
	return findLot(data, id)
}
func (s *Service) Event(ctx context.Context, id string) (Event, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return Event{}, err
	}
	return findEvent(data, id)
}
func slugExists(data Snapshot, kind, slug, id string) bool {
	if kind == "lot" {
		for _, l := range data.Lots {
			if l.Slug == slug && l.ID != id {
				return true
			}
		}
	} else {
		for _, e := range data.Events {
			if e.Slug == slug && e.ID != id {
				return true
			}
		}
	}
	return false
}
func (s *Service) SaveLot(ctx context.Context, id string, expected int64, content LotContent) (Lot, error) {
	var out Lot
	err := s.Repo.Write(ctx, func(tx Tx) error {
		data, err := tx.Load()
		if err != nil {
			return err
		}
		if id == "" {
			id, content.Slug, err = createIdentity(data, "lot", content.Title, content.Slug)
			if err != nil {
				return err
			}
			out = Lot{ID: id, State: "draft", Version: 1, CreatedAt: s.Now().UnixMilli()}
			if content.Currency == "" {
				content.Currency = "USD"
			}
		} else {
			out, err = findLot(data, id)
			if err != nil {
				return err
			}
			if out.State != "draft" {
				return Fail("published")
			}
			if out.Version != expected {
				return Fail("stale_version")
			}
			out.Version++
		}
		if slugExists(data, "lot", content.Slug, id) {
			return Fail("duplicate_slug")
		}
		if err = validateLot(content, data.Categories, false, s.Media); err != nil {
			return err
		}
		out.LotContent = content
		out.UpdatedAt = s.Now().UnixMilli()
		if out.Images == nil {
			out.Images = []Image{}
		}
		if out.Specifications == nil {
			out.Specifications = []Specification{}
		}
		if err = tx.PutLot(out, expected); err != nil {
			return err
		}
		if out.EventID != nil {
			event, err := findEvent(data, *out.EventID)
			if err != nil {
				return err
			}
			if event.State != "draft" {
				return Fail("published")
			}
			old := event.Version
			event.Version++
			event.UpdatedAt = out.UpdatedAt
			return tx.PutEvent(event, old)
		}
		return nil
	})
	return out, err
}
func createIdentity(data Snapshot, kind, title, slug string) (string, string, error) {
	id, generated, err := identity(title)
	if err != nil {
		return "", "", err
	}
	if slug == "" {
		slug = generated
		if slugExists(data, kind, slug, id) {
			slug += "-" + id[:8]
		}
	}
	return id, slug, nil
}
func (s *Service) SaveEvent(ctx context.Context, id string, expected int64, content EventContent) (Event, error) {
	var out Event
	err := s.Repo.Write(ctx, func(tx Tx) error {
		data, err := tx.Load()
		if err != nil {
			return err
		}
		if id == "" {
			id, content.Slug, err = createIdentity(data, "event", content.Title, content.Slug)
			if err != nil {
				return err
			}
			out = Event{ID: id, State: "draft", Version: 1, CreatedAt: s.Now().UnixMilli(), LotIDs: []string{}}
		} else {
			out, err = findEvent(data, id)
			if err != nil {
				return err
			}
			if out.State != "draft" {
				return Fail("published")
			}
			if out.Version != expected {
				return Fail("stale_version")
			}
			out.Version++
		}
		if slugExists(data, "event", content.Slug, id) {
			return Fail("duplicate_slug")
		}
		out.EventContent = content
		out.UpdatedAt = s.Now().UnixMilli()
		if err = validateEvent(out, false, 0, s.Media); err != nil {
			return err
		}
		out.Schedule, _ = Schedule(out)
		return tx.PutEvent(out, expected)
	})
	return out, err
}
func (s *Service) Members(ctx context.Context, id string, expected int64, ids []string) (Event, error) {
	var out Event
	err := s.Repo.Write(ctx, func(tx Tx) error {
		data, err := tx.Load()
		if err != nil {
			return err
		}
		out, err = findEvent(data, id)
		if err != nil {
			return err
		}
		if out.State != "draft" {
			return Fail("published")
		}
		if out.Version != expected {
			return Fail("stale_version")
		}
		seen := map[string]bool{}
		affected := map[string]bool{}
		for _, old := range out.LotIDs {
			affected[old] = true
		}
		for _, lotID := range ids {
			if seen[lotID] {
				return Fail("duplicate_lot")
			}
			seen[lotID] = true
			l, err := findLot(data, lotID)
			if err != nil {
				return Fail("missing_lot")
			}
			if l.State != "draft" {
				return Fail("published")
			}
			if l.EventID != nil && *l.EventID != id {
				return Fail("already_assigned")
			}
			affected[lotID] = true
		}
		out.LotIDs = append([]string{}, ids...)
		if err = validateEvent(out, false, 0, s.Media); err != nil {
			return err
		}
		if err = tx.ReplaceMembers(id, out.LotIDs); err != nil {
			return err
		}
		now := s.Now().UnixMilli()
		for lotID := range affected {
			l, err := findLot(data, lotID)
			if err != nil {
				return err
			}
			if l.State != "draft" {
				return Fail("published")
			}
			old := l.Version
			l.Version++
			l.UpdatedAt = now
			if err = tx.PutLot(l, old); err != nil {
				return err
			}
		}
		out.Version++
		out.UpdatedAt = now
		out.Schedule, _ = Schedule(out)
		return tx.PutEvent(out, expected)
	})
	return out, err
}
func (s *Service) Publish(ctx context.Context, id string, expected int64) (PublicEvent, error) {
	var out PublicEvent
	err := s.Repo.Write(ctx, func(tx Tx) error {
		data, err := tx.Load()
		if err != nil {
			return err
		}
		event, err := findEvent(data, id)
		if err != nil {
			return err
		}
		if event.State == "published" {
			out = projectEvent(data, event)
			return nil
		}
		if event.Version != expected {
			return Fail("stale_version")
		}
		fields := map[string]string{}
		merge := func(prefix string, err error) {
			var v *Error
			if errors.As(err, &v) {
				for k, val := range v.Fields {
					fields[prefix+k] = val
				}
			}
		}
		merge("", validateEvent(event, true, s.Now().UnixMilli(), s.Media))
		for _, id := range event.LotIDs {
			l, err := findLot(data, id)
			if err != nil {
				return err
			}
			if l.State != "draft" {
				return Fail("published")
			}
			merge("lots."+id+".", validateLot(l.LotContent, data.Categories, true, s.Media))
		}
		if len(fields) > 0 {
			return invalid(fields)
		}
		now := s.Now().UnixMilli()
		for i := range data.Lots {
			l := &data.Lots[i]
			if l.EventID != nil && *l.EventID == event.ID {
				old := l.Version
				l.State = "published"
				l.Version++
				l.UpdatedAt = now
				if err = tx.PutLot(*l, old); err != nil {
					return err
				}
			}
		}
		old := event.Version
		event.State = "published"
		event.Version++
		event.UpdatedAt = now
		if err = tx.PutEvent(event, old); err != nil {
			return err
		}
		out = projectEvent(data, event)
		return nil
	})
	return out, err
}
func (s *Service) Preview(ctx context.Context, id string) (PublicEvent, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return PublicEvent{}, err
	}
	event, err := findEvent(data, id)
	if err != nil {
		return PublicEvent{}, err
	}
	return projectEvent(data, event), nil
}
func optional(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}
