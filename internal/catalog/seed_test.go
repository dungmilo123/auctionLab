package catalog_test

import (
	"auctionlab/internal/catalog"
	"context"
	"os"
	"testing"
	"time"
)

func TestSeedRepeatAndAssets(t *testing.T) {
	ctx := context.Background()
	db, s, _ := fixture(t)
	_ = db
	public := os.DirFS("../../web/public")
	s.Media = catalog.LocalMedia{Embedded: public}
	at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	status, err := s.Seed(ctx, at)
	if err != nil || status != "seeded" {
		t.Fatal(status, err)
	}
	data, err := s.Snapshot(ctx)
	if err != nil || len(data.Lots) != 18 || len(data.Events) != 3 || len(data.Categories) != 5 {
		t.Fatal(data, err)
	}
	categories := map[string]bool{}
	for _, lot := range data.Lots {
		categories[*lot.CategoryID] = true
		for _, image := range lot.Images {
			if !s.Media.Readable(image.Path) {
				t.Fatal("bundled image unreadable", image.Path)
			}
		}
	}
	if len(categories) != 5 {
		t.Fatal(categories)
	}
	for i, e := range data.Events {
		if len(e.LotIDs) != 6 || *e.OpensAt != at.Add(time.Duration(i+1)*24*time.Hour).UnixMilli() || len(e.Schedule) != 6 {
			t.Fatal(e)
		}
	}
	l := data.Lots[0]
	l.Title = "Operator edit"
	if _, err = s.SaveLot(ctx, l.ID, l.Version, l.LotContent); err != nil {
		t.Fatal(err)
	}
	status, err = s.Seed(ctx, at.Add(30*24*time.Hour))
	if err != nil || status != "already_applied" {
		t.Fatal(status, err)
	}
	edited, err := s.Lot(ctx, l.ID)
	if err != nil || edited.Title != "Operator edit" {
		t.Fatal(edited, err)
	}
	e, _ := s.Event(ctx, data.Events[0].ID)
	if _, err = s.Publish(ctx, e.ID, e.Version); err != nil {
		t.Fatal("normal publication of bundled media", err)
	}
}
func TestSeedCollisionAndRollback(t *testing.T) {
	ctx := context.Background()
	db, s, _ := fixture(t)
	s.Media = media(true)
	at := time.UnixMilli(1000)
	if _, err := s.SaveLot(ctx, "", 0, catalog.LotContent{Title: "Walnut bookshelf speakers"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Seed(ctx, at); code(err) != "seed_collision" {
		t.Fatal(err)
	}
	data, _ := s.Snapshot(ctx)
	if len(data.Lots) != 1 || len(data.Events) != 0 {
		t.Fatal("partial seed")
	}
	// Separate target, failure after all content was written but before marker commit.
	db2, s2, _ := fixture(t)
	s2.Media = media(true)
	if _, err := db2.DB.Exec("ALTER TABLE catalog_seed_runs ADD CONSTRAINT seed_fault CHECK(version<0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Seed(ctx, at); err == nil {
		t.Fatal("injected seed failure succeeded")
	}
	data, _ = s2.Snapshot(ctx)
	if len(data.Lots) != 0 || len(data.Events) != 0 {
		t.Fatal("seed rollback lost")
	}
	_ = db
}
