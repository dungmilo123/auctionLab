// dbfixture creates/drops only explicit disposable load-test schemas.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"os"
	"regexp"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	drop := flag.String("drop", "", "owned disposable schema to remove")
	flag.Parse()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		return fmt.Errorf("TEST_DATABASE_URL required; use a dedicated test database")
	}
	cfg, err := pgx.ParseConfig(raw)
	if err != nil {
		return fmt.Errorf("invalid TEST_DATABASE_URL")
	}
	cfg.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if *drop != "" {
		if !regexp.MustCompile(`^test_load_[a-f0-9]{32}$`).MatchString(*drop) {
			return fmt.Errorf("refusing non-fixture schema")
		}
		_, err = db.ExecContext(ctx, `DROP SCHEMA "`+*drop+`" CASCADE`)
	} else {
		var b [16]byte
		if _, err = rand.Read(b[:]); err != nil {
			return fmt.Errorf("fixture entropy unavailable")
		}
		schema := "test_load_" + hex.EncodeToString(b[:])
		_, err = db.ExecContext(ctx, `CREATE SCHEMA "`+schema+`"`)
		if err == nil {
			fmt.Println(schema)
		}
	}
	if err != nil {
		return fmt.Errorf("PostgreSQL fixture operation failed; check isolated test service")
	}
	return nil
}
