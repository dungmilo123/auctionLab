// Generates disposable v4 fixtures. Never accepts or reads an operational source.
package main

import (
	_ "auctionlab/internal/sqliteimport"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		panic("supply a NEW fixture directory")
	}
	dir := os.Args[1]
	if err := os.Mkdir(dir, 0700); err != nil {
		panic("fixture directory must not exist")
	}
	path := filepath.Join(dir, "snapshot.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		panic("fixture open")
	}
	ddl, err := os.ReadFile("internal/sqliteimport/testdata/v4.sql")
	if err != nil {
		panic("fixture schema")
	}
	if _, err = db.Exec(string(ddl)); err != nil {
		panic("fixture DDL")
	}
	now := time.Now().UnixMilli()
	start := now - 60000
	deadline := now - 30000
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			panic("fixture insert")
		}
	}
	exec("INSERT INTO players VALUES(?,?,?,?,?,?,?,?)", "legacy-player", "Bình", "bình", strings.Repeat("a", 64), int64(1000000), int64(0), start, start)
	exec(`INSERT INTO items(id,order_index,name,description,image_url,auction_type,truth_value,hint_min,hint_max,start_price,min_increment,duration_ms,anti_snipe_mode,status) VALUES('legacy-english',0,'Fixture camera',NULL,'','english',350000,NULL,0,100000,10000,30000,'disabled','live')`)
	exec(`INSERT INTO items(id,order_index,name,auction_type,truth_value,start_price,duration_ms,anti_snipe_mode,status) VALUES('legacy-sealed',1,'Fixture sealed object','sealed_first',456789,100000,30000,'disabled','pending')`)
	exec("INSERT INTO bids(id,item_id,player_id,amount,kind,created_at) VALUES(900,'legacy-english','legacy-player',100000,'bid',?)", start+1)
	exec("INSERT INTO bids(id,item_id,player_id,amount,kind,created_at) VALUES(901,'legacy-english','legacy-player',110000,'bid',?)", start+2)
	exec("DELETE FROM bids WHERE id=901")
	exec("INSERT INTO sealed_bids VALUES('legacy-sealed','legacy-player',123456,?,?)", start+3, start+4)
	exec("INSERT INTO auction_runtime VALUES('legacy-english',?,?,NULL,4,0)", start, deadline)
	exec("INSERT INTO game VALUES(1,'LIVE','legacy-english',4,?,?)", `{ "starting_budget":1000000, "currency":"USD", "join_url":"", "custom_note":"preserve whitespace" }`, start+5)
	exec("INSERT INTO events(id,type,payload_json,created_at) VALUES(700,'fixture',NULL,?)", start)
	exec("DELETE FROM events")
	exec("INSERT INTO server_config VALUES('admin_key',?)", os.Getenv("ACCEPTANCE_ADMIN_KEY"))
	if err = db.Close(); err != nil {
		panic("fixture close")
	}
	fmt.Println("created standalone disposable v4 fixture")
}
