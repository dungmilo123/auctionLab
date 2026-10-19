package store

import (
	"auctionlab/internal/sqliteimport"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ImportSQLite never bootstraps game/key state. Migrations must precede it.
func (s *Store) ImportSQLite(ctx context.Context, source *sqliteimport.Snapshot) (sqliteimport.Report, error) {
	report := source.Report("imported")
	err := s.withWriter(func(ctx context.Context, c *sql.Conn) error {
		tx, err := c.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var fingerprint string
		err = tx.QueryRowContext(ctx, "SELECT fingerprint FROM sqlite_import_runs LIMIT 1").Scan(&fingerprint)
		if err == nil {
			if fingerprint != source.Fingerprint {
				return errors.New("different source already imported")
			}
			report.Status = "already_imported"
			report.IdentityReady = nil
			report.Tables = nil
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		for _, table := range []string{"commerce_accounts", "commerce_sessions", "commerce_watchlist", "commerce_account_seed_runs", "players", "items", "sealed_bids", "bids", "auction_runtime", "game", "events", "server_config", "catalog_lots", "catalog_lot_images", "catalog_events", "catalog_event_lots", "catalog_seed_runs"} {
			var n int64
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				return errors.New("target is occupied; use an initialized unused schema before normal startup")
			}
		}
		// Dependency order differs from canonical report order.
		for _, name := range []string{"players", "items", "sealed_bids", "bids", "auction_runtime", "game", "events", "server_config"} {
			for _, table := range source.Tables {
				if table.Name != name {
					continue
				}
				args := make([]string, len(table.Columns))
				for i := range args {
					args[i] = fmt.Sprintf("$%d", i+1)
				}
				q := "INSERT INTO " + name + " (" + strings.Join(table.Columns, ",") + ") VALUES (" + strings.Join(args, ",") + ")"
				for _, row := range table.Rows {
					if _, err = tx.ExecContext(ctx, q, row...); err != nil {
						return sanitize("import "+name, err)
					}
				}
			}
		}
		for _, name := range []string{"bids", "events"} {
			next := source.HighWater[name] + 1
			if _, err = tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN id RESTART WITH %d`, name, next)); err != nil {
				return err
			}
		}
		if err = compareSQLite(ctx, tx, source, s.SchemaName); err != nil {
			return err
		}
		raw, _ := json.Marshal(report)
		if _, err = tx.ExecContext(ctx, "INSERT INTO sqlite_import_runs VALUES($1,4,$2,$3)", source.Fingerprint, string(raw), time.Now().UnixMilli()); err != nil {
			return err
		}
		return tx.Commit()
	})
	return report, err
}

type importReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func compareSQLite(ctx context.Context, db importReader, source *sqliteimport.Snapshot, schema string) error {
	for _, table := range source.Tables {
		rows, err := db.QueryContext(ctx, "SELECT "+strings.Join(table.Columns, ",")+" FROM "+table.Name+" ORDER BY "+table.Order)
		if err != nil {
			return sanitize("verify "+table.Name, err)
		}
		all := [][]any{}
		for rows.Next() {
			vals := make([]any, len(table.Columns))
			dest := make([]any, len(vals))
			for i := range vals {
				dest[i] = &vals[i]
			}
			if err = rows.Scan(dest...); err != nil {
				rows.Close()
				return sanitize("verify "+table.Name, err)
			}
			all = append(all, vals)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return sanitize("verify "+table.Name, err)
		}
		if len(all) != len(table.Rows) || sqliteimport.Digest(all) != table.Digest {
			return fmt.Errorf("verification mismatch: %s row count/digest", table.Name)
		}
	}
	for _, name := range []string{"bids", "events"} {
		var last int64
		var called bool
		if err := db.QueryRowContext(ctx, `SELECT last_value,is_called FROM "`+schema+`".`+name+`_id_seq`).Scan(&last, &called); err != nil {
			return sanitize("identity verification", err)
		}
		if called {
			if last >= 9223372036854775807 {
				return errors.New("identity range exhausted")
			}
			last++
		}
		if last <= source.HighWater[name] {
			return fmt.Errorf("verification mismatch: %s identity readiness", name)
		}
	}
	return nil
}
func (s *Store) VerifySQLite(ctx context.Context, source *sqliteimport.Snapshot) (sqliteimport.Report, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return sqliteimport.Report{}, sanitize("verify begin", err)
	}
	defer tx.Rollback()
	if err = compareSQLite(ctx, tx, source, s.SchemaName); err != nil {
		return sqliteimport.Report{}, err
	}
	return source.Report("verified"), nil
}
