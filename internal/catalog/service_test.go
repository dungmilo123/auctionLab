package catalog_test

import (
	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type media bool

func (m media) Readable(string) bool { return bool(m) }
func n(v int64) *int64               { return &v }
func text(v string) *string          { return &v }
func fixture(t *testing.T) (*store.Store, *catalog.Service, string) {
	t.Helper()
	raw := testpg.NewURL(t)
	s, err := store.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s, catalog.New(store.CatalogRepository{Store: s}, func() time.Time { return time.UnixMilli(1000) }, media(true)), raw
}
func code(err error) string {
	var e *catalog.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
func completeLot(title string) catalog.LotContent {
	return catalog.LotContent{Title: title, CategoryID: text("film-cameras"), Description: "Illustrative object, no authenticity claim", Condition: "Known wear; illustration only", Images: []catalog.Image{{Path: "/images/front.svg", Alt: "Front"}, {Path: "/images/back.svg", Alt: "Back"}}, Specifications: []catalog.Specification{{Name: "Medium", Value: "Illustration"}, {Name: "Color", Value: "Black"}}, StartingPriceMinor: n(12550), BidIncrementMinor: n(500), Currency: "USD", AdminNotes: "PRIVATE-NOTE-SENTINEL", Fulfillment: catalog.Fulfillment{ShippingEnabled: true, ShippingFeeMinor: n(0), ShippingInstructions: "Packed carefully"}}
}
func completeEvent(title string) catalog.EventContent {
	return catalog.EventContent{Title: title, Theme: "Objects", Description: "Illustrative preparation", CoverPath: "/images/cover.svg", CoverAlt: "Cover", OpensAt: n(10000), FirstClosesAt: n(20000), CloseIntervalMS: n(60000), AdminNotes: "EVENT-PRIVATE-SENTINEL"}
}
func prepare(t *testing.T, s *catalog.Service) (catalog.Event, []catalog.Lot) {
	t.Helper()
	ctx := context.Background()
	lots := []catalog.Lot{}
	for _, name := range []string{"Camera A", "Camera B", "Camera C"} {
		l, err := s.SaveLot(ctx, "", 0, completeLot(name))
		if err != nil {
			t.Fatal(err)
		}
		lots = append(lots, l)
	}
	e, err := s.SaveEvent(ctx, "", 0, completeEvent("Collection"))
	if err != nil {
		t.Fatal(err)
	}
	e, err = s.Members(ctx, e.ID, e.Version, []string{lots[2].ID, lots[0].ID, lots[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	return e, lots
}
func TestDraftsMembershipPublicationAndRestart(t *testing.T) {
	ctx := context.Background()
	db, s, raw := fixture(t)
	draft, err := s.SaveLot(ctx, "", 0, catalog.LotContent{Title: "Đồng hồ"})
	if err != nil || draft.Slug == "" || draft.Currency != "USD" {
		t.Fatal(draft, err)
	}
	c := draft.LotContent
	c.Title = "New title"
	draft, err = s.SaveLot(ctx, draft.ID, draft.Version, c)
	if err != nil || draft.Slug != c.Slug {
		t.Fatal("slug regenerated", err)
	}
	if _, err = s.SaveLot(ctx, draft.ID, 1, c); code(err) != "stale_version" {
		t.Fatal(err)
	}
	c.CategoryID = text("missing")
	if _, err = s.SaveLot(ctx, draft.ID, draft.Version, c); code(err) != "validation" {
		t.Fatal(err)
	}
	e, lots := prepare(t, s)
	if e.Schedule[0].LotID != lots[2].ID || e.Schedule[2].ClosesAt != 140000 {
		t.Fatal(e.Schedule)
	}
	other, err := s.SaveEvent(ctx, "", 0, catalog.EventContent{Title: "Overlapping"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Members(ctx, other.ID, other.Version, []string{lots[0].ID}); code(err) != "already_assigned" {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{lots[0].ID, lots[0].ID}, {lots[0].ID, "missing"}} {
		if _, err = s.Members(ctx, e.ID, e.Version, ids); err == nil {
			t.Fatal("partial membership accepted")
		}
	}
	oldVersion := e.Version
	l, err := s.Lot(ctx, lots[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	c = l.LotContent
	c.Title = "Edited"
	l, err = s.SaveLot(ctx, l.ID, l.Version, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, e.ID, oldVersion); code(err) != "stale_version" {
		t.Fatal("member edit did not invalidate event", err)
	}
	e, _ = s.Event(ctx, e.ID)
	preview, err := s.Preview(ctx, e.ID)
	if err != nil || len(preview.Lots) != 3 {
		t.Fatal(err)
	}
	if all, err := s.PublicEvents(ctx); err != nil || len(all) != 0 {
		t.Fatal("draft leaked", err)
	}
	if _, err = s.PublicLot(ctx, l.Slug); code(err) != "not_found" {
		t.Fatal(err)
	}
	published, err := s.Publish(ctx, e.ID, e.Version)
	if err != nil || published.State != "published" {
		t.Fatal(published, err)
	}
	encoded, _ := json.Marshal(published)
	for _, bad := range []string{"PRIVATE", "admin_notes", "version", "truth_value", "token", "winner", "current_bid", "live", "ended"} {
		if strings.Contains(string(encoded), bad) {
			t.Fatal("projection leak", bad)
		}
	}
	s.Now = func() time.Time { return time.UnixMilli(999999999) }
	repeated, err := s.Publish(ctx, e.ID, 1)
	encoded2, _ := json.Marshal(repeated)
	if err != nil || string(encoded) != string(encoded2) {
		t.Fatal("repeat changed aggregate", err)
	}
	if _, err = s.SaveEvent(ctx, e.ID, e.Version, e.EventContent); code(err) != "published" {
		t.Fatal(err)
	}
	l, _ = s.Lot(ctx, l.ID)
	if _, err = s.SaveLot(ctx, l.ID, l.Version, l.LotContent); code(err) != "published" {
		t.Fatal(err)
	}
	if _, err = s.Members(ctx, e.ID, e.Version, nil); code(err) != "published" {
		t.Fatal(err)
	}
	if err = db.Bootstrap(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = db.SetServerConfig("admin_key", "KEY-SENTINEL"); err != nil {
		t.Fatal(err)
	}
	before, _ := db.LoadGame()
	if err = db.Commit(store.Mutation{Phase: "LOBBY", Revision: before.Revision + 1, ClearBids: true, ClearSealedBids: true, DeleteRuntime: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.PublicEvents(ctx); err != nil || len(got) != 1 {
		t.Fatal("game reset changed catalog", err)
	}
	db.Close()
	reopened, err := store.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	fresh := catalog.New(store.CatalogRepository{Store: reopened}, s.Now, media(true))
	got, err := fresh.PublicEvent(ctx, e.Slug)
	encoded2, _ = json.Marshal(got)
	if err != nil || string(encoded) != string(encoded2) {
		t.Fatal("reconnect changed data", err)
	}
}
func TestAtomicFailureAndConcurrency(t *testing.T) {
	ctx := context.Background()
	db, s, _ := fixture(t)
	e, lots := prepare(t, s)
	// Fail after lot transitions, before event commit; none may become visible.
	if _, err := db.DB.Exec("ALTER TABLE catalog_events ADD CONSTRAINT publication_fault CHECK(state='draft')"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, e.ID, e.Version); err == nil {
		t.Fatal("storage failure accepted")
	}
	all, _ := s.PublicLots(ctx)
	if len(all) != 0 {
		t.Fatal("partial publication")
	}
	for _, l := range lots {
		got, _ := s.Lot(ctx, l.ID)
		if got.State != "draft" {
			t.Fatal("lot transition escaped rollback")
		}
	}
	db.DB.Exec("ALTER TABLE catalog_events DROP CONSTRAINT publication_fault")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.SaveEvent(ctx, e.ID, e.Version, e.EventContent); errs <- err }()
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if code(err) == "stale_version" {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	e, _ = s.Event(ctx, e.ID)
	lot, _ := s.Lot(ctx, lots[0].ID)
	errs = make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, err := s.Publish(ctx, e.ID, e.Version); errs <- err }()
	go func() { defer wg.Done(); _, err := s.SaveLot(ctx, lot.ID, lot.Version, lot.LotContent); errs <- err }()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && code(err) != "stale_version" && code(err) != "published" {
			t.Fatal(err)
		}
	}
	e, _ = s.Event(ctx, e.ID)
	if e.State == "draft" {
		if _, err := s.Publish(ctx, e.ID, e.Version); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPublicationValidationAndDetach(t *testing.T) {
	ctx := context.Background()
	_, s, _ := fixture(t)
	e, err := s.SaveEvent(ctx, "", 0, catalog.EventContent{Title: "Incomplete"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, e.ID, e.Version); code(err) != "validation" {
		t.Fatal(err)
	}
	l, _ := s.SaveLot(ctx, "", 0, catalog.LotContent{Title: "Title only"})
	e, err = s.Members(ctx, e.ID, e.Version, []string{l.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, e.ID, e.Version); code(err) != "validation" {
		t.Fatal(err)
	}
	e, err = s.Members(ctx, e.ID, e.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := s.SaveEvent(ctx, "", 0, catalog.EventContent{Title: "Other"})
	if _, err = s.Members(ctx, other.ID, other.Version, []string{l.ID}); err != nil {
		t.Fatal("detach reattach", err)
	}
	e, lots := prepare(t, s)
	s.Now = func() time.Time { return time.UnixMilli(10000) }
	if _, err = s.Publish(ctx, e.ID, e.Version); code(err) != "validation" {
		t.Fatal("expired publication accepted", err)
	}
	_ = lots
}
