package main

import (
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWinnerStartupProcess(t *testing.T) {
	if os.Getenv("AUCTIONLAB_CHECKOUT_CHILD") != "1" {
		return
	}
	log.SetOutput(io.Discard)
	flag.CommandLine = flag.NewFlagSet("checkout-startup", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = []string{"checkout-startup"}
	switch os.Getenv("AUCTIONLAB_CHECKOUT_MODE") {
	case "accounts":
		os.Args = append(os.Args, "--seed-accounts")
	case "catalog":
		os.Args = append(os.Args, "--seed-catalog", "--seed-at", "2030-01-01T00:00:00Z")
	}
	e := run()
	if os.Getenv("AUCTIONLAB_CHECKOUT_MODE") == "normal" {
		if e == nil {
			os.Exit(3)
		}
	} else if e != nil {
		os.Exit(4)
	}
	os.Exit(0)
}
func TestWinnerStartupAdoptionFailureAndMaintenance(t *testing.T) {
	for _, mode := range []string{"accounts", "catalog", "normal"} {
		t.Run(mode, func(t *testing.T) {
			raw := testpg.NewURL(t)
			st, e := store.Open(raw)
			if e != nil {
				t.Fatal(e)
			}
			if e = st.Migrate(); e != nil {
				t.Fatal(e)
			}
			_, e = st.DB.Exec(`INSERT INTO commerce_accounts VALUES('winner','winner@example.test','Winner','fixture','bidder',false,0,0);
 INSERT INTO commerce_account_seed_runs VALUES(1,1);
 INSERT INTO catalog_lots(id,slug,title,state,created_at,updated_at) VALUES('historic','historic','Historic winner','published',0,0);
 INSERT INTO catalog_events(id,slug,title,state,created_at,updated_at) VALUES('historic','historic','Historic event','published',0,0);
 INSERT INTO commerce_lot_auctions(lot_id,event_id,opens_at,closes_at,starting_price_minor,bid_increment_minor) VALUES('historic','historic',1,10,0,1);
 INSERT INTO commerce_bids(id,lot_id,account_id,sequence,amount_minor,accepted_at,request_id,bidder_number) VALUES(1,'historic','winner',1,0,2,'00000000-0000-0000-0000-000000000001',1);
 UPDATE commerce_lot_auctions SET state='sold',bid_count=1,leading_bid_id=1;
 INSERT INTO commerce_auction_results VALUES('historic','sold',10,11,1,'winner',0);`)
			if e != nil {
				t.Fatal(e)
			}
			schema := st.SchemaName
			cfg := store.Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: schema}
			st.Close()
			child := exec.Command(os.Args[0], "-test.run=^TestWinnerStartupProcess$")
			env := []string{}
			for _, v := range os.Environ() {
				if !strings.HasPrefix(v, "DATABASE_URL=") && !strings.HasPrefix(v, "DATABASE_SCHEMA=") && !strings.HasPrefix(v, "DB_PATH=") {
					env = append(env, v)
				}
			}
			child.Env = append(env, "AUCTIONLAB_CHECKOUT_CHILD=1", "AUCTIONLAB_CHECKOUT_MODE="+mode, "DATABASE_URL="+cfg.URL, "DATABASE_SCHEMA="+schema)
			if e = child.Run(); e != nil {
				t.Fatal("startup boundary failed", e)
			}
			reopened, e := store.OpenConfig(cfg)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			var ok bool
			e = reopened.DB.QueryRowContext(context.Background(), `SELECT (SELECT count(*)=0 FROM commerce_orders) AND (SELECT count(*)=0 FROM commerce_demo_payments) AND (SELECT count(*)=1 FROM commerce_auction_results) AND (SELECT winning_account_id='winner' AND recorded_at=11 FROM commerce_auction_results)`).Scan(&ok)
			if e != nil || !ok {
				t.Fatal(ok, e)
			}
		})
	}
}
