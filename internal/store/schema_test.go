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
	for _, table := range []string{"commerce_orders", "commerce_demo_payments", "commerce_lot_auctions", "commerce_bids", "commerce_auction_results", "commerce_accounts", "commerce_sessions", "commerce_watchlist", "commerce_account_seed_runs", "players", "items", "bids", "sealed_bids", "auction_runtime", "game", "events", "server_config", "catalog_categories", "catalog_lots", "catalog_lot_images", "catalog_events", "catalog_event_lots", "catalog_seed_runs", "sqlite_import_runs", "schema_migrations"} {
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
	if _, err = s.DB.Exec("INSERT INTO schema_migrations VALUES($1,0)", UserVersion+1); err != nil {
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

func TestCommerceConstraints(t *testing.T) {
	s, err := Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	exec := func(q string) {
		t.Helper()
		if _, e := s.DB.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	reject := func(q string) {
		t.Helper()
		if _, e := s.DB.Exec(q); e == nil {
			t.Fatal("constraint accepted invalid row")
		}
	}
	exec(`INSERT INTO commerce_accounts VALUES('a','a@example.test','A','private','bidder',false,1,1)`)
	reject(`INSERT INTO commerce_accounts VALUES('b','a@example.test','B','private','bidder',false,1,1)`)
	reject(`INSERT INTO commerce_accounts VALUES('b','b@example.test','B','private','root',false,1,1)`)
	reject(`INSERT INTO commerce_sessions VALUES('digest','missing',1,2,NULL)`)
	reject(`INSERT INTO commerce_sessions VALUES('digest','a',2,1,NULL)`)
	exec(`INSERT INTO commerce_sessions VALUES('digest','a',1,2,NULL)`)
	reject(`INSERT INTO commerce_sessions VALUES('digest','a',1,2,NULL)`)
	reject(`INSERT INTO commerce_watchlist VALUES('a','missing',1)`)
	exec(`INSERT INTO catalog_lots(id,slug,title,created_at,updated_at) VALUES('lot','lot','Lot',1,1)`)
	reject(`INSERT INTO commerce_watchlist VALUES('missing','lot',1)`)
	exec(`INSERT INTO commerce_watchlist VALUES('a','lot',1)`)
	reject(`INSERT INTO commerce_watchlist VALUES('a','lot',2)`)
	exec(`INSERT INTO commerce_account_seed_runs VALUES(1,1)`)
	reject(`INSERT INTO commerce_account_seed_runs VALUES(1,2)`)
}

func TestUpgradeFromVersionOne(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "upgrade", true: "rollback"}[fail], func(t *testing.T) {
			s, e := Open(testpg.NewURL(t))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			_, e = s.DB.Exec(initialDDL + `; CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at BIGINT NOT NULL); INSERT INTO schema_migrations VALUES(1,1);
 INSERT INTO players VALUES('p','Legacy','legacy','legacy-token',123,7,1,2);
 INSERT INTO game VALUES(1,'LOBBY',NULL,42,'{}',1);
 INSERT INTO server_config VALUES('admin_key','upgrade-capability');
 INSERT INTO catalog_lots(id,slug,title,state,created_at,updated_at) VALUES('lot','lot','Published','published',1,1);
 INSERT INTO catalog_events(id,slug,title,state,created_at,updated_at) VALUES('event','event','Published event','published',1,1);
 INSERT INTO catalog_event_lots VALUES('event','lot',0);`)
			if e != nil {
				t.Fatal(e)
			}
			if fail {
				_, e = s.DB.Exec(`CREATE TABLE commerce_sessions(blocker INTEGER)`)
				if e != nil {
					t.Fatal(e)
				}
				if s.Migrate() == nil {
					t.Fatal("failed migration accepted")
				}
				var v int
				s.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&v)
				if v != 1 {
					t.Fatal(v)
				}
				var present bool
				s.DB.QueryRow(`SELECT to_regclass('commerce_accounts') IS NOT NULL`).Scan(&present)
				if present {
					t.Fatal("partial DDL")
				}
				s.DB.Exec(`DROP TABLE commerce_sessions`)
			}
			if e = s.Migrate(); e != nil {
				t.Fatal(e)
			}
			var ok bool
			e = s.DB.QueryRow(`SELECT (SELECT budget=123 AND token='legacy-token' FROM players WHERE id='p') AND (SELECT revision=42 FROM game) AND (SELECT value='upgrade-capability' FROM server_config WHERE key='admin_key') AND (SELECT state='published' AND version=1 FROM catalog_lots WHERE id='lot') AND (SELECT state='published' AND version=1 FROM catalog_events WHERE id='event') AND (SELECT count(*)=1 FROM catalog_event_lots) AND (SELECT count(*)=0 FROM commerce_accounts) AND (SELECT count(*)=0 FROM commerce_watchlist) AND (SELECT max(version)=$1 FROM schema_migrations)`, UserVersion).Scan(&ok)
			if e != nil || !ok {
				t.Fatal("upgrade changed existing data", e)
			}
		})
	}
}
