package store

import (
	"auctionlab/internal/testpg"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestConfiguration(t *testing.T) {
	for _, env := range []map[string]string{{}, {"DATABASE_URL": "file:old.db"}, {"DATABASE_URL": "postgres://user:secret@localhost/db?sslmode=broken"}, {"DATABASE_URL": "postgres://localhost/db", "DATABASE_SCHEMA": "public"}, {"DATABASE_URL": "postgres://localhost/db", "DATABASE_SCHEMA": "a,b"}, {"DATABASE_URL": "postgres://localhost/db", "DB_PATH": "old.db"}} {
		_, err := ConfigFromEnv(func(k string) string { return env[k] })
		if err == nil {
			t.Fatal("accepted invalid configuration")
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("credential leak")
		}
	}
	before := time.Now()
	s, err := OpenConfig(Config{URL: "postgres://user:secret@127.0.0.1:1/no?sslmode=disable", Schema: "app"})
	if err == nil || s != nil || time.Since(before) > 6*time.Second {
		t.Fatal("unreachable configuration accepted or unbounded")
	}
}
func TestPoolOwnershipAndFailure(t *testing.T) {
	raw := testpg.NewURL(t)
	s, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if second, err := Open(raw); err == nil {
		second.Close()
		t.Fatal("competing writer accepted")
	}
	conns := []*sql.Conn{}
	for i := 0; i < 3; i++ {
		c, err := s.DB.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		var schema string
		if err = c.QueryRowContext(context.Background(), "SELECT current_schema()").Scan(&schema); err != nil || schema != s.SchemaName {
			t.Fatal("schema not pinned on pooled connection")
		}
	}
	for _, c := range conns {
		c.Close()
	}
	if s.DB.Stats().MaxOpenConnections != 8 {
		t.Fatal("unbounded pool")
	}
	if _, err = s.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	var pid int
	s.writer.QueryRowContext(context.Background(), "SELECT pg_backend_pid()").Scan(&pid)
	if _, err = s.DB.Exec("SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	if err = s.Bootstrap(time.Now()); err == nil {
		t.Fatal("lost writer committed")
	}
	if _, err = s.Ready(context.Background()); err == nil {
		t.Fatal("lost writer ready")
	}
	if err = s.Bootstrap(time.Now()); err == nil {
		t.Fatal("writer transparently reacquired")
	}
}
