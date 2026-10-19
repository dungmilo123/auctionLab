// Package testpg provides isolated real PostgreSQL fixtures. It never falls back or skips.
package testpg

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

var fixtures sync.Map

func URL(t testing.TB, key string) string {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("TEST_DATABASE_URL is required: provision a dedicated PostgreSQL 18 test database")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	sum := sha256.Sum256([]byte(key))
	schema := fmt.Sprintf("test_%x", sum[:12])
	if _, loaded := fixtures.LoadOrStore(schema, true); !loaded {
		cfg, err := pgx.ParseConfig(raw)
		if err != nil {
			t.Fatal("invalid TEST_DATABASE_URL")
		}
		db := stdlib.OpenDB(*cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err = db.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
			db.Close()
			t.Fatal("PostgreSQL fixture unavailable: check TEST_DATABASE_URL and schema privileges")
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := db.ExecContext(ctx, `DROP SCHEMA "`+schema+`" CASCADE`)
			db.Close()
			fixtures.Delete(schema)
			if err != nil {
				t.Error("could not drop owned test schema")
			}
		})
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
func NewURL(t testing.TB) string { t.Helper(); return URL(t, t.TempDir()) }
