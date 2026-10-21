package store

import (
	"auctionlab/internal/account"
	"auctionlab/internal/catalog"
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrDemoTarget = errors.New("demo target refused: require a matching marked disposable schema, known application objects and a stopped writer")

type DemoEnvironment struct {
	Status        string `json:"status"`
	Schema        string `json:"schema"`
	Format        int    `json:"format_version"`
	Generation    string `json:"generation"`
	SeedReference int64  `json:"seed_reference"`
	PreparedAt    int64  `json:"prepared_at"`
}

func ValidDemoSchema(name string) bool {
	return schemaPattern.MatchString(name) && strings.HasPrefix(name, "auctionlab_demo_") && len(name) > len("auctionlab_demo_")
}

var demoClearTables = []string{"commerce_interactions", "commerce_interaction_coverage", "commerce_order_fulfillment_events", "commerce_order_fulfillment", "commerce_demo_payments", "commerce_orders", "commerce_auction_results", "commerce_bids", "commerce_lot_auctions", "commerce_watchlist", "commerce_sessions", "commerce_accounts", "commerce_account_seed_runs", "catalog_lot_images", "catalog_event_lots", "catalog_lots", "catalog_events", "catalog_seed_runs", "auction_runtime", "sealed_bids", "bids", "events", "game", "players", "items", "sqlite_import_runs"}
var demoPreservedTables = []string{"schema_migrations", "server_config", "catalog_categories", "demo_environments"}

func readDemo(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, schema string) (DemoEnvironment, error) {
	d := DemoEnvironment{Schema: schema, Status: "prepared"}
	e := q.QueryRowContext(ctx, `SELECT schema_name,format_version,generation,seed_reference,prepared_at FROM "`+schema+`".demo_environments WHERE id=1`).Scan(&d.Schema, &d.Format, &d.Generation, &d.SeedReference, &d.PreparedAt)
	if e != nil || d.Schema != schema || d.Format != 1 {
		return DemoEnvironment{}, ErrDemoTarget
	}
	return d, nil
}
func (s *Store) DemoStatus(parent context.Context) (DemoEnvironment, error) {
	if !ValidDemoSchema(s.SchemaName) {
		return DemoEnvironment{}, ErrDemoTarget
	}
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return DemoEnvironment{}, sanitize("demo status", e)
	}
	defer tx.Rollback()
	return readDemo(ctx, tx, s.SchemaName)
}

// inspectDemo refuses unknown tables, views, routines, triggers and dependencies.
// No schema drop, CASCADE or identity reset is used, even for marked targets.
func (s *Store) inspectDemo(ctx context.Context, tx *sql.Tx, empty bool) error {
	allowed := map[string]bool{}
	for _, name := range append(append([]string{}, demoClearTables...), demoPreservedTables...) {
		allowed[name] = true
	}
	rows, e := tx.QueryContext(ctx, `SELECT c.relname,c.relkind::text FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relkind NOT IN ('i')`, s.SchemaName)
	if e != nil {
		return e
	}
	tables := []string{}
	for rows.Next() {
		var name, kind string
		if e = rows.Scan(&name, &kind); e != nil {
			rows.Close()
			return e
		}
		if kind == "S" && (name == "bids_id_seq" || name == "events_id_seq" || name == "commerce_bids_id_seq") {
			continue
		}
		if kind != "r" || !allowed[name] {
			rows.Close()
			return ErrDemoTarget
		}
		tables = append(tables, name)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var unsafe bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND NOT t.tgisinternal) OR EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1) OR EXISTS(SELECT 1 FROM pg_constraint con JOIN pg_class a ON a.oid=con.conrelid JOIN pg_namespace na ON na.oid=a.relnamespace JOIN pg_class b ON b.oid=con.confrelid JOIN pg_namespace nb ON nb.oid=b.relnamespace WHERE con.contype='f' AND ((na.nspname=$1 AND nb.nspname<>$1) OR (nb.nspname=$1 AND na.nspname<>$1))) OR EXISTS(SELECT 1 FROM pg_depend d JOIN pg_rewrite r ON r.oid=d.objid JOIN pg_class v ON v.oid=r.ev_class JOIN pg_namespace vn ON vn.oid=v.relnamespace JOIN pg_class c ON c.oid=d.refobjid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND vn.nspname<>$1)`, s.SchemaName).Scan(&unsafe)
	if e != nil {
		return e
	}
	if unsafe {
		return ErrDemoTarget
	}
	for _, name := range tables {
		if !empty && name != "sqlite_import_runs" {
			continue
		}
		if name == "schema_migrations" || name == "catalog_categories" {
			continue
		}
		var occupied bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM "`+s.SchemaName+`"."`+name+`")`).Scan(&occupied); e != nil {
			return e
		}
		if occupied {
			return ErrDemoTarget
		}
	}
	return nil
}
func (s *Store) DemoPrepared(ctx context.Context) (DemoEnvironment, bool, error) {
	var d DemoEnvironment
	found := false
	if !ValidDemoSchema(s.SchemaName) {
		return d, false, ErrDemoTarget
	}
	e := (AuctionRepository{s}).write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Never initialize/adopt an occupied unmarked target.
		var exists bool
		if e := tx.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, s.SchemaName+".demo_environments").Scan(&exists); e != nil {
			return e
		}
		if exists {
			var count int
			if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM "`+s.SchemaName+`".demo_environments`).Scan(&count); e != nil {
				return e
			}
			if count > 0 {
				var e error
				d, e = readDemo(ctx, tx, s.SchemaName)
				if e != nil {
					return e
				}
				if e = s.inspectDemo(ctx, tx, false); e != nil {
					return e
				}
				found = true
				d.Status = "already_prepared"
				return nil
			}
		}
		return s.inspectDemo(ctx, tx, true)
	})
	return d, found, e
}
func (s *Store) SeedDemo(ctx context.Context, reset bool, confirmation string, reference time.Time, rows []account.Record, media catalog.MediaResolver) (DemoEnvironment, error) {
	var out DemoEnvironment
	if !ValidDemoSchema(s.SchemaName) || reset && confirmation != s.SchemaName || reference.UnixMilli() < 0 || reference.UnixMilli() > catalog.MaxInstant-4*86400000 || len(rows) != 3 {
		return out, ErrDemoTarget
	}
	generation, e := purchaseID()
	if e != nil {
		return out, sanitize("demo generation", e)
	}
	out = DemoEnvironment{Status: "prepared", Schema: s.SchemaName, Format: 1, Generation: generation, SeedReference: reference.UnixMilli(), PreparedAt: time.Now().UnixMilli()}
	if reset {
		out.Status = "reset"
	}
	e = (AuctionRepository{s}).write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if reset {
			if _, e := readDemo(ctx, tx, s.SchemaName); e != nil {
				return e
			}
			if e := s.inspectDemo(ctx, tx, false); e != nil {
				return e
			}
			// TRUNCATE checks every dependency even on empty referencing tables; no CASCADE.
			names := []string{}
			for _, name := range demoClearTables {
				names = append(names, `"`+s.SchemaName+`"."`+name+`"`)
			}
			if _, e := tx.ExecContext(ctx, `TRUNCATE TABLE `+strings.Join(names, ",")); e != nil {
				return e
			}
		} else if e := s.inspectDemo(ctx, tx, true); e != nil {
			return e
		}
		if _, e := seedAccountsTx(ctx, tx, rows, out.PreparedAt); e != nil {
			return e
		}
		svc := catalog.New(nil, time.Now, media)
		if _, e := svc.SeedTx(&catalogTx{ctx, tx}, reference); e != nil {
			return e
		}
		if e := initializeCoverage(ctx, tx, out.PreparedAt); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO demo_environments VALUES(1,$1,1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET generation=EXCLUDED.generation,seed_reference=EXCLUDED.seed_reference,prepared_at=EXCLUDED.prepared_at`, out.Schema, out.Generation, out.SeedReference, out.PreparedAt)
		return e
	})
	if e != nil {
		return DemoEnvironment{}, e
	}
	return out, nil
}
