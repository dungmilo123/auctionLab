package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"time"
)

var ErrNewerSchema = errors.New("PostgreSQL schema version is newer than this build supports; use a matching application build")

const UserVersion = 5
const gameConfigJSON = `{"starting_budget":1000000,"currency":"USD"}`

//go:embed migrations/001_initial.sql
var initialDDL string

//go:embed migrations/002_accounts_and_watchlists.sql
var accountsDDL string

//go:embed migrations/003_timed_auctions.sql
var auctionsDDL string

//go:embed migrations/004_winner_checkout.sql
var ordersDDL string

//go:embed migrations/005_order_fulfillment.sql
var fulfillmentDDL string

var migrations = []string{initialDDL, accountsDDL, auctionsDDL, ordersDDL, fulfillmentDDL}

func (s *Store) Migrate() error {
	err := s.withWriter(func(ctx context.Context, c *sql.Conn) error {
		tx, err := c.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS "`+s.SchemaName+`"`); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at BIGINT NOT NULL)`); err != nil {
			return err
		}
		var version int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
			return err
		}
		if version > UserVersion {
			return ErrNewerSchema
		}
		for i := version; i < len(migrations); i++ {
			if _, err = tx.ExecContext(ctx, migrations[i]); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations VALUES($1,$2)`, i+1, time.Now().UnixMilli()); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if errors.Is(err, ErrNewerSchema) {
		return ErrNewerSchema
	}
	return sanitize("migration", err)
}
func (s *Store) Bootstrap(now time.Time) error {
	return sanitize("bootstrap", s.withWriter(func(ctx context.Context, c *sql.Conn) error {
		_, err := c.ExecContext(ctx, `INSERT INTO game(id,phase,current_item_id,revision,config_json,updated_at) VALUES(1,'LOBBY',NULL,0,$1,$2) ON CONFLICT(id) DO NOTHING`, gameConfigJSON, now.UnixMilli())
		return err
	}))
}
