// Read-only acceptance audit; refuses non-fixture schemas and emits no PII.
package main

import (
	"auctionlab/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "fixture audit failed")
		os.Exit(1)
	}
}
func run() error {
	cfg, e := store.ConfigFromEnv(os.Getenv)
	if e != nil {
		return e
	}
	if !regexp.MustCompile(`^test_load_[a-f0-9]{32}$`).MatchString(cfg.Schema) {
		return fmt.Errorf("fixture required")
	}
	cfg.ReadOnly = true
	st, e := store.OpenConfig(cfg)
	if e != nil {
		return e
	}
	defer st.Close()
	out := map[string]any{}
	for _, q := range []struct{ name, sql string }{
		{"orders", `SELECT count(*) FROM commerce_orders`}, {"payments", `SELECT count(*) FROM commerce_demo_payments`}, {"paid_orders", `SELECT count(*) FROM commerce_orders WHERE state='paid'`}, {"unpaid_orders", `SELECT count(*) FROM commerce_orders WHERE state='awaiting_payment'`}, {"results", `SELECT count(*) FROM commerce_auction_results`}, {"game_revision", `SELECT revision FROM game`},
	} {
		var n int64
		if e = st.DB.QueryRowContext(context.Background(), q.sql).Scan(&n); e != nil {
			return e
		}
		out[q.name] = n
	}
	var valid bool
	e = st.DB.QueryRow(`SELECT NOT EXISTS(SELECT 1 FROM commerce_orders o LEFT JOIN commerce_demo_payments p ON p.order_id=o.id WHERE (o.state='paid')<>(p.id IS NOT NULL) OR (o.state='paid' AND (p.owner_id<>o.owner_id OR p.total_minor<>o.total_minor OR p.paid_at<>o.paid_at OR o.total_minor<>o.winning_amount_minor+o.fulfillment_fee_minor))) AND NOT EXISTS(SELECT 1 FROM commerce_orders WHERE selection->>'fulfillment_method'='pickup' AND selection ? 'shipping_address')`).Scan(&valid)
	if e != nil {
		return e
	}
	out["one_matching_receipt_per_paid_order_and_no_pickup_address"] = valid
	return json.NewEncoder(os.Stdout).Encode(out)
}
