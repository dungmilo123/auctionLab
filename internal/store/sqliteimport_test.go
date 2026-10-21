package store

import (
	"auctionlab/internal/sqliteimport"
	"auctionlab/internal/testpg"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sourceFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("../sqliteimport/testdata/v4.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(ddl)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO players VALUES('p','Bình','bình','token-sentinel',1000123,7,1,2);
 INSERT INTO items(id,order_index,name,description,image_url,auction_type,truth_value,start_price) VALUES('i',0,'Camera',NULL,'','english',456789,125);
 INSERT INTO sealed_bids VALUES('i','p',123,5,6);
 INSERT INTO bids(id,item_id,player_id,amount,kind,created_at) VALUES(90,'i','p',10,'bid',3);
 DELETE FROM bids;
 INSERT INTO bids(item_id,player_id,amount,kind,created_at) VALUES('i','p',20,'bid',4);
 INSERT INTO auction_runtime VALUES('i',1,99,NULL,3,2);
 INSERT INTO game VALUES(1,'LIVE','i',3,'{ "starting_budget":1000000, "currency":"VND", "custom": 2 }',7);
 INSERT INTO events(id,type,payload_json,created_at) VALUES(500,'audit',NULL,2);
 DELETE FROM events;
 INSERT INTO server_config VALUES('admin_key','capability-sentinel');`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestSQLiteImportAndIndependentVerification(t *testing.T) {
	ctx := context.Background()
	path := sourceFixture(t)
	before, _ := os.ReadFile(path)
	source, err := sqliteimport.Read(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	raw := testpg.NewURL(t)
	s, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	report, err := s.ImportSQLite(ctx, source)
	if err != nil || report.Status != "imported" {
		t.Fatal(report, err)
	}
	if _, err = s.VerifySQLite(ctx, source); err != nil {
		t.Fatal(err)
	}
	var value string
	s.DB.QueryRow("SELECT config_json FROM game").Scan(&value)
	if !strings.Contains(value, `"currency":"VND"`) {
		t.Fatal("legacy JSON/currency changed")
	}
	var next int64
	if err = s.DB.QueryRow("INSERT INTO events(type,created_at) VALUES('new',1) RETURNING id").Scan(&next); err != nil || next != 501 {
		t.Fatal("deleted identity high-water lost", next, err)
	}
	s.DB.Exec("DELETE FROM events")
	report, err = s.ImportSQLite(ctx, source)
	if err != nil || report.Status != "already_imported" || report.IdentityReady != nil || report.Tables != nil {
		t.Fatal(report, err)
	}
	if _, err = s.DB.Exec("UPDATE players SET budget=budget+1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.VerifySQLite(ctx, source); err == nil || !strings.Contains(err.Error(), "players") {
		t.Fatal("verification trusted marker", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("source modified")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err = os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatal("sidecar created")
		}
	}
}
func TestSQLiteImportRollbackAndOccupied(t *testing.T) {
	ctx := context.Background()
	source, err := sqliteimport.Read(ctx, sourceFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("ALTER TABLE events ADD CONSTRAINT injected CHECK(type='never')"); err != nil {
		t.Fatal(err)
	}
	// Nonempty events makes the injected constraint fail after prior table inserts.
	for i := range source.Tables {
		if source.Tables[i].Name == "events" {
			source.Tables[i].Rows = [][]any{{int64(1), "audit", nil, int64(2)}}
			source.Tables[i].Digest = sqliteimport.Digest(source.Tables[i].Rows)
		}
	}
	if _, err = s.ImportSQLite(ctx, source); err == nil {
		t.Fatal("injected failure accepted")
	}
	for _, table := range []string{"players", "items", "bids", "game", "sqlite_import_runs"} {
		var n int
		s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n)
		if n != 0 {
			t.Fatal("partial import", table)
		}
	}
	var last int64
	var called bool
	s.DB.QueryRow("SELECT last_value,is_called FROM bids_id_seq").Scan(&last, &called)
	if last != 1 || called {
		t.Fatal("sequence not rolled back", last, called)
	}
	s.DB.Exec("ALTER TABLE events DROP CONSTRAINT injected")
	if err = s.Bootstrap(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportSQLite(ctx, source); err == nil {
		t.Fatal("occupied target accepted")
	}
}
func TestSQLiteSourceRejectsIncompatible(t *testing.T) {
	for _, query := range []string{"PRAGMA user_version=3", "UPDATE players SET nickname=char(0)", "UPDATE players SET budget=1.25", "PRAGMA foreign_keys=off; UPDATE sealed_bids SET player_id='missing'"} {
		t.Run(query, func(t *testing.T) {
			path := sourceFixture(t)
			db, _ := sql.Open("sqlite", path)
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if _, err := sqliteimport.Read(context.Background(), path); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	path := sourceFixture(t)
	os.WriteFile(path+"-wal", []byte("uncheckpointed"), 0600)
	if _, err := sqliteimport.Read(context.Background(), path); err == nil {
		t.Fatal("WAL source accepted")
	}
}
func TestSQLiteIdentityRestartRollbackAndOverflow(t *testing.T) {
	ctx := context.Background()
	path := sourceFixture(t)
	source, err := sqliteimport.Read(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("ALTER TABLE sqlite_import_runs ADD CONSTRAINT marker_fault CHECK(source_version=0)"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportSQLite(ctx, source); err == nil {
		t.Fatal("marker failure accepted")
	}
	for _, name := range []string{"bids", "events"} {
		var value int64
		var called bool
		if err = s.DB.QueryRow("SELECT last_value,is_called FROM "+name+"_id_seq").Scan(&value, &called); err != nil || value != 1 || called {
			t.Fatal("RESTART did not roll back", name, value, called, err)
		}
	}
	s.DB.Exec("ALTER TABLE sqlite_import_runs DROP CONSTRAINT marker_fault")
	if _, err = s.ImportSQLite(ctx, source); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec("UPDATE players SET budget=99")
	report, err := s.ImportSQLite(ctx, source)
	if err != nil || report.Status != "already_imported" || report.IdentityReady != nil || report.Tables != nil {
		t.Fatal(report, err)
	}
	var budget int64
	s.DB.QueryRow("SELECT budget FROM players").Scan(&budget)
	if budget != 99 {
		t.Fatal("repeat overwrote edit")
	}
	source.Fingerprint = "different"
	if _, err = s.ImportSQLite(ctx, source); err == nil {
		t.Fatal("different source accepted")
	}
	sqlite, _ := sql.Open("sqlite", path)
	sqlite.Exec("UPDATE sqlite_sequence SET seq=9223372036854775807 WHERE name='events'")
	sqlite.Close()
	if _, err = sqliteimport.Read(ctx, path); err == nil {
		t.Fatal("exhausted identity accepted")
	}
}

func TestImportAccountOccupied(t *testing.T) {
	source, e := sqliteimport.Read(context.Background(), sourceFixture(t))
	if e != nil {
		t.Fatal(e)
	}
	for _, seed := range []string{`INSERT INTO commerce_accounts VALUES('a','a@example.test','A','private','bidder',false,1,1)`, `INSERT INTO commerce_account_seed_runs VALUES(1,1)`} {
		s, e := Open(testpg.NewURL(t))
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { s.Close() })
		if e = s.Migrate(); e != nil {
			t.Fatal(e)
		}
		if _, e = s.DB.Exec(seed); e != nil {
			t.Fatal(e)
		}
		if _, e = s.ImportSQLite(context.Background(), source); e == nil || !strings.Contains(e.Error(), "occupied") {
			t.Fatal("occupied target accepted")
		}
		var n int
		s.DB.QueryRow(`SELECT count(*) FROM players`).Scan(&n)
		if n != 0 {
			t.Fatal("partial import")
		}
		s.DB.QueryRow(`SELECT count(*) FROM sqlite_import_runs`).Scan(&n)
		if n != 0 {
			t.Fatal("marker written")
		}
	}
}

func TestImportAuctionOccupied(t *testing.T) {
	ctx := context.Background()
	path := sourceFixture(t)
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	source, e := sqliteimport.Read(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	s, e := Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`INSERT INTO catalog_lots(id,slug,title,created_at,updated_at) VALUES('l','l','L',0,0);
 INSERT INTO catalog_events(id,slug,title,created_at,updated_at) VALUES('e','e','E',0,0);
 INSERT INTO commerce_lot_auctions(lot_id,event_id,opens_at,closes_at,starting_price_minor,bid_increment_minor) VALUES('l','e',0,10,0,1);
 INSERT INTO commerce_auction_results VALUES('l','unsold',10,11,NULL,NULL,NULL)`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportSQLite(ctx, source); e == nil || !strings.Contains(e.Error(), "occupied") {
		t.Fatal(e)
	}
	var clean bool
	e = s.DB.QueryRow(`SELECT (SELECT count(*)=0 FROM players) AND (SELECT count(*)=0 FROM sqlite_import_runs) AND (SELECT count(*)=1 FROM commerce_auction_results)`).Scan(&clean)
	if e != nil || !clean {
		t.Fatal(clean, e)
	}
	after, e := os.ReadFile(path)
	if e != nil || string(before) != string(after) {
		t.Fatal("source modified", e)
	}
}

func TestImportPurchaseOccupied(t *testing.T) {
	for _, paid := range []bool{false, true} {
		t.Run(map[bool]string{false: "order", true: "receipt"}[paid], func(t *testing.T) {
			s, r, id := soldOrder(t)
			ctx := context.Background()
			if paid {
				if _, e := r.Confirm(ctx, "a", id, checkoutIntent(1), at(300)); e != nil {
					t.Fatal(e)
				}
			}
			path := sourceFixture(t)
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			source, e := sqliteimport.Read(ctx, path)
			if e != nil {
				t.Fatal(e)
			}
			var orders, payments int
			s.DB.QueryRow(`SELECT count(*) FROM commerce_orders`).Scan(&orders)
			s.DB.QueryRow(`SELECT count(*) FROM commerce_demo_payments`).Scan(&payments)
			if _, e = s.ImportSQLite(ctx, source); e == nil || !strings.Contains(e.Error(), "occupied") {
				t.Fatal("occupied purchase accepted")
			}
			var ok bool
			e = s.DB.QueryRow(`SELECT (SELECT count(*)=$1 FROM commerce_orders) AND (SELECT count(*)=$2 FROM commerce_demo_payments) AND (SELECT count(*)=0 FROM players) AND (SELECT count(*)=0 FROM sqlite_import_runs)`, orders, payments).Scan(&ok)
			if e != nil || !ok {
				t.Fatal(ok, e)
			}
			after, e := os.ReadFile(path)
			if e != nil || string(after) != string(before) {
				t.Fatal("source modified")
			}
		})
	}
}
