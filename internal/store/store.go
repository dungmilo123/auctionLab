// Package store owns PostgreSQL persistence. SQLite is only an offline import source.
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

var ErrWriterLost = errors.New("PostgreSQL writer ownership lost; restart required")

const OperationTimeout = 5 * time.Second

var schemaPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

type Config struct {
	URL, Schema string
	ReadOnly    bool
}

func ConfigFromEnv(getenv func(string) string) (Config, error) {
	if getenv("DB_PATH") != "" {
		return Config{}, errors.New("DB_PATH is obsolete; use DATABASE_URL and --import-sqlite for a stopped v4 snapshot")
	}
	c := Config{URL: getenv("DATABASE_URL"), Schema: getenv("DATABASE_SCHEMA")}
	if c.Schema == "" {
		c.Schema = "auctionlab"
	}
	if _, err := parseConfig(c); err != nil {
		return Config{}, err
	}
	return c, nil
}
func parseConfig(c Config) (*pgx.ConnConfig, error) {
	if c.URL == "" {
		return nil, errors.New("DATABASE_URL is required (PostgreSQL 18)")
	}
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return nil, errors.New("invalid DATABASE_URL: expected a PostgreSQL URL")
	}
	if !schemaPattern.MatchString(c.Schema) || c.Schema == "public" || c.Schema == "pg_catalog" || len(c.Schema) > 3 && c.Schema[:3] == "pg_" {
		return nil, errors.New("invalid DATABASE_SCHEMA: use a dedicated simple lowercase identifier")
	}
	cfg, err := pgx.ParseConfig(c.URL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL: check PostgreSQL connection options")
	}
	cfg.ConnectTimeout = OperationTimeout
	cfg.RuntimeParams["search_path"] = `"` + c.Schema + `",pg_catalog`
	cfg.RuntimeParams["statement_timeout"] = "5000"
	cfg.RuntimeParams["lock_timeout"] = "5000"
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = "10000"
	return cfg, nil
}

// Open is a convenience for explicit URLs. Production uses OpenConfig with env validation.
func Open(raw string) (*Store, error) {
	c := Config{URL: raw, Schema: "auctionlab"}
	if u, e := url.Parse(raw); e == nil {
		if v := u.Query().Get("search_path"); v != "" {
			c.Schema = v
		}
	}
	return OpenConfig(c)
}

type Store struct {
	DB         *sql.DB
	SchemaName string
	writer     *sql.Conn
	writerMu   sync.Mutex
	lost       bool
	closeOnce  sync.Once
}

func OpenConfig(c Config) (*Store, error) {
	cfg, err := parseConfig(c)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), OperationTimeout)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.New("PostgreSQL unavailable: check DATABASE_URL and service readiness")
	}
	var version int
	if err = db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&version); err != nil || version/10000 != 18 {
		db.Close()
		return nil, errors.New("PostgreSQL 18 is required")
	}
	s := &Store{DB: db, SchemaName: c.Schema}
	if !c.ReadOnly {
		s.writer, err = db.Conn(ctx)
		if err == nil {
			var ok bool
			err = s.writer.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended(current_database() || ':' || $1 || ':writer',0))`, c.Schema).Scan(&ok)
			if err == nil && !ok {
				err = errors.New("owned")
			}
		}
		if err != nil {
			s.Close()
			return nil, errors.New("PostgreSQL writer ownership unavailable: stop the competing process before startup or maintenance")
		}
	}
	return s, nil
}

// withWriter never replaces the leased connection, even after it breaks.
func (s *Store) withWriter(fn func(context.Context, *sql.Conn) error, parents ...context.Context) error {
	var ctx context.Context
	var cancel context.CancelFunc
	if len(parents) > 0 {
		ctx, cancel = context.WithTimeout(parents[0], OperationTimeout)
		defer cancel()
		// Account requests may abandon the queue without touching the leased connection.
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for !s.writerMu.TryLock() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	} else {
		s.writerMu.Lock()
		ctx, cancel = context.WithTimeout(context.Background(), OperationTimeout)
		defer cancel()
	}
	defer s.writerMu.Unlock()
	if s.writer == nil || s.lost {
		return ErrWriterLost
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	err := fn(ctx, s.writer)
	if err != nil && (errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) || ctx.Err() != nil || SQLState(err) == "57P01" || SQLState(err) == "08006") {
		s.lost = true
	}
	return err
}
func (s *Store) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.writerMu.Lock()
		defer s.writerMu.Unlock()
		s.lost = true
		if s.writer != nil { // Physically close the session: returning a locked session to the pool is unsafe.
			_ = s.writer.Raw(func(c any) error {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				return c.(*stdlib.Conn).Conn().Close(ctx)
			})
			_ = s.writer.Close()
		}
		err = s.DB.Close()
	})
	return err
}
func SQLState(err error) string {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		return p.Code
	}
	return ""
}

// SafeError preserves classification without printing credentials or row values.
type safeError struct {
	op    string
	cause error
}

func (e safeError) Error() string { return "store: " + e.op + " failed" }
func (e safeError) Unwrap() error { return e.cause }
func sanitize(op string, err error) error {
	if err == nil {
		return nil
	}
	return safeError{op, err}
}
func (s *Store) Ready(ctx context.Context) (int, error) {
	if err := s.DB.PingContext(ctx); err != nil {
		return 0, errors.New("database unavailable")
	}
	s.writerMu.Lock()
	defer s.writerMu.Unlock()
	if s.writer == nil || s.lost {
		return 0, errors.New("writer unavailable")
	}
	if err := s.writer.PingContext(ctx); err != nil {
		s.lost = true
		return 0, errors.New("writer unavailable")
	}
	var version int
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM "`+s.SchemaName+`".schema_migrations`).Scan(&version)
	if err != nil || version != UserVersion {
		return 0, errors.New("schema unavailable")
	}
	return version, nil
}

func (e safeError) CommitUncertain() bool {
	var nested interface{ CommitUncertain() bool }
	if errors.As(e.cause, &nested) && nested.CommitUncertain() {
		return true
	}
	return (e.op == "catalog commit" || e.op == "account commit" || e.op == "auction commit") && SQLState(e.cause) != "40001" && SQLState(e.cause) != "40P01"
}

func (e safeError) Unavailable() bool {
	var network net.Error
	var connect *pgconn.ConnectError
	return errors.Is(e.cause, ErrWriterLost) || errors.Is(e.cause, driver.ErrBadConn) || errors.Is(e.cause, sql.ErrConnDone) || errors.As(e.cause, &network) || errors.As(e.cause, &connect)
}
