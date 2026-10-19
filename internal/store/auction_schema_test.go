package store

import (
	"auctionlab/internal/testpg"
	"context"
	"testing"
)

func TestAuctionMigrationConstraints(t *testing.T) {
	s, e := Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
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
			t.Fatal("invalid auction row accepted:", q)
		}
	}
	exec(`INSERT INTO catalog_lots(id,slug,title,created_at,updated_at) VALUES('l','l','L',0,0),('other','other','Other',0,0);
 INSERT INTO catalog_events(id,slug,title,created_at,updated_at) VALUES('e','e','E',0,0);
 INSERT INTO commerce_accounts VALUES('a','a@example.test','Private','verifier','bidder',false,0,0),('b','b@example.test','Private B','verifier','bidder',false,0,0);
 INSERT INTO commerce_lot_auctions(lot_id,event_id,opens_at,closes_at,starting_price_minor,bid_increment_minor) VALUES('l','e',1,10,0,5),('other','e',1,20,0,5)`)
	for _, q := range []string{
		`UPDATE commerce_lot_auctions SET closes_at=1 WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET state='closing' WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET starting_price_minor=-1 WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET bid_increment_minor=0 WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET currency='VND' WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET revision=0 WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET bid_count=1 WHERE lot_id='l'`,
		`UPDATE commerce_lot_auctions SET leading_bid_id=99,bid_count=1 WHERE lot_id='l'`,
		`INSERT INTO commerce_bids(lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES('l','a',1,1000000000001,2,'00000000-0000-0000-0000-000000000001',1)`,
		`INSERT INTO commerce_bids(lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES('l','a',0,0,2,'00000000-0000-0000-0000-000000000001',1)`,
		`INSERT INTO commerce_bids(lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES('l','a',1,0,2,'invalid',1)`,
	} {
		reject(q)
	}
	exec(`INSERT INTO commerce_bids(id,lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES(100,'l','a',1,0,2,'00000000-0000-0000-0000-000000000001',1)`)
	reject(`INSERT INTO commerce_bids(id,lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES(101,'l','b',1,5,2,'00000000-0000-0000-0000-000000000002',2)`)
	reject(`INSERT INTO commerce_bids(id,lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES(102,'other','a',1,5,2,'00000000-0000-0000-0000-000000000001',1)`)
	reject(`UPDATE commerce_lot_auctions SET leading_bid_id=100,bid_count=1 WHERE lot_id='other'`)
	exec(`UPDATE commerce_lot_auctions SET leading_bid_id=100,bid_count=1 WHERE lot_id='l'`)
	reject(`INSERT INTO commerce_auction_results VALUES('l','sold',10,10,100,'b',0)`)
	reject(`INSERT INTO commerce_auction_results VALUES('l','sold',10,10,100,'a',1)`)
	reject(`INSERT INTO commerce_auction_results VALUES('l','unsold',10,10,100,'a',0)`)
	reject(`INSERT INTO commerce_auction_results VALUES('l','sold',10,9,100,'a',0)`)
	exec(`INSERT INTO commerce_auction_results VALUES('l','sold',10,10,100,'a',0)`)
	reject(`INSERT INTO commerce_auction_results VALUES('l','sold',10,11,100,'a',0)`)
	exec(`INSERT INTO commerce_auction_results VALUES('other','unsold',20,21,NULL,NULL,NULL)`)
	if e = s.Migrate(); e != nil {
		t.Fatal(e)
	}
	v, e := s.Ready(context.Background())
	if e != nil || v != 3 {
		t.Fatal(v, e)
	}
}

func TestUpgradeFromVersionTwo(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run(map[bool]string{false: "upgrade", true: "rollback"}[block], func(t *testing.T) {
			s, e := Open(testpg.NewURL(t))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			_, e = s.DB.Exec(initialDDL + accountsDDL + `; CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at BIGINT NOT NULL); INSERT INTO schema_migrations VALUES(1,1),(2,2);
 INSERT INTO players VALUES('p','Legacy','legacy','token',123,7,1,2);
 INSERT INTO game VALUES(1,'LOBBY',NULL,42,'{}',1);
 INSERT INTO server_config VALUES('admin_key','capability');
 INSERT INTO catalog_lots(id,slug,title,state,created_at,updated_at) VALUES('l','l','L','published',1,1);
 INSERT INTO catalog_events(id,slug,title,state,created_at,updated_at) VALUES('e','e','E','published',1,1);
 INSERT INTO catalog_event_lots VALUES('e','l',0);
 INSERT INTO commerce_accounts VALUES('a','a@example.test','A','verifier','bidder',false,1,1);
 INSERT INTO commerce_sessions VALUES('digest','a',1,9999999999999,NULL);
 INSERT INTO commerce_watchlist VALUES('a','l',2);
 INSERT INTO commerce_account_seed_runs VALUES(1,3);`)
			if e != nil {
				t.Fatal(e)
			}
			if block {
				if _, e = s.DB.Exec(`CREATE TABLE commerce_bids(blocker INTEGER)`); e != nil {
					t.Fatal(e)
				}
				if s.Migrate() == nil {
					t.Fatal("partial migration accepted")
				}
				var ok bool
				e = s.DB.QueryRow(`SELECT (SELECT max(version)=2 FROM schema_migrations) AND to_regclass('commerce_lot_auctions') IS NULL`).Scan(&ok)
				if e != nil || !ok {
					t.Fatal(ok, e)
				}
				if _, e = s.DB.Exec(`DROP TABLE commerce_bids`); e != nil {
					t.Fatal(e)
				}
			}
			for i := 0; i < 2; i++ {
				if e = s.Migrate(); e != nil {
					t.Fatal(e)
				}
			}
			var ok bool
			e = s.DB.QueryRow(`SELECT (SELECT max(version)=3 FROM schema_migrations) AND (SELECT revision=42 FROM game) AND (SELECT budget=123 AND token='token' FROM players) AND (SELECT value='capability' FROM server_config) AND (SELECT state='published' AND version=1 FROM catalog_events) AND (SELECT state='published' AND version=1 FROM catalog_lots) AND (SELECT count(*)=1 FROM catalog_event_lots) AND (SELECT password_verifier='verifier' FROM commerce_accounts) AND (SELECT token_digest='digest' AND revoked_at IS NULL FROM commerce_sessions) AND (SELECT saved_at=2 FROM commerce_watchlist) AND (SELECT completed_at=3 FROM commerce_account_seed_runs) AND (SELECT count(*)=0 FROM commerce_bids) AND (SELECT count(*)=0 FROM commerce_auction_results)`).Scan(&ok)
			if e != nil || !ok {
				t.Fatal("upgrade preservation", ok, e)
			}
		})
	}
}
