package store

import (
	"auctionlab/internal/testpg"
	"testing"
	"time"
)

func TestSchema(t *testing.T) {
	s, err := Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 2; i++ {
		if err = s.Migrate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"players", "items", "bids", "sealed_bids", "auction_runtime", "game", "events", "server_config", "catalog_categories", "catalog_lots", "catalog_lot_images", "catalog_events", "catalog_event_lots", "catalog_seed_runs", "sqlite_import_runs", "schema_migrations"} {
		var ok bool
		if err = s.DB.QueryRow("SELECT to_regclass($1) IS NOT NULL", s.SchemaName+"."+table).Scan(&ok); err != nil || !ok {
			t.Fatal("missing table", table, err)
		}
	}
	var n int
	s.DB.QueryRow("SELECT count(*) FROM catalog_categories").Scan(&n)
	if n != 5 {
		t.Fatal(n)
	}
	if err = s.Bootstrap(time.UnixMilli(1700000000000)); err != nil {
		t.Fatal(err)
	}
	if err = s.Bootstrap(time.Now()); err != nil {
		t.Fatal(err)
	}
	g, err := s.LoadGame()
	if err != nil || g.UpdatedAt != 1700000000000 {
		t.Fatal(g, err)
	}
	if _, err = s.DB.Exec("INSERT INTO schema_migrations VALUES(2,0)"); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(); err == nil {
		t.Fatal("newer ledger accepted")
	}
}
func TestMigrationRollback(t *testing.T) {
	s, err := Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.DB.Exec("CREATE TABLE catalog_lots(blocker INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(); err == nil {
		t.Fatal("invalid migration committed")
	}
	var table *string
	if err = s.DB.QueryRow("SELECT to_regclass('players')::text").Scan(&table); err != nil || table != nil {
		t.Fatal("partial migration", err)
	}
	if _, err = s.DB.Exec("DROP TABLE catalog_lots"); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
}
