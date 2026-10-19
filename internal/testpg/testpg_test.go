package testpg

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"testing"
)

func TestParallelIsolation(t *testing.T) {
	for i := 0; i < 3; i++ {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			t.Parallel()
			cfg, err := pgx.ParseConfig(NewURL(t))
			if err != nil {
				t.Fatal(err)
			}
			db := stdlib.OpenDB(*cfg)
			defer db.Close()
			if _, err = db.ExecContext(context.Background(), "CREATE TABLE private_fixture(value TEXT)"); err != nil {
				t.Fatal("fixtures overlap", err)
			}
			if _, err = db.Exec("INSERT INTO private_fixture VALUES('owned')"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
