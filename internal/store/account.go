package store

import (
	"auctionlab/internal/account"
	"context"
	"database/sql"
	"errors"
)

type AccountRepository struct{ Store *Store }
type rowScanner interface{ Scan(...any) error }

func scanProfile(row rowScanner) (account.Profile, error) {
	var p account.Profile
	e := row.Scan(&p.ID, &p.Email, &p.DisplayName, &p.Role, &p.Demo)
	if errors.Is(e, sql.ErrNoRows) {
		return p, account.Fail("access_required")
	}
	return p, sanitize("account read", e)
}

const profileColumns = "a.id,a.email,a.display_name,a.role,a.demo"

func sessionProfile(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, digest string, now int64) (account.Profile, error) {
	return scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM commerce_accounts a JOIN commerce_sessions s ON s.account_id=a.id WHERE s.token_digest=$1 AND s.revoked_at IS NULL AND s.expires_at>$2`, digest, now))
}
func (r AccountRepository) write(parent context.Context, fn func(context.Context, *sql.Tx) error) error {
	if err := parent.Err(); err != nil {
		return sanitize("account write", err)
	}
	return sanitize("account write", r.Store.withWriter(func(lease context.Context, c *sql.Conn) error {
		ctx, cancel := context.WithTimeout(parent, OperationTimeout)
		defer cancel()
		stop := context.AfterFunc(lease, cancel)
		defer stop()
		if e := c.PingContext(ctx); e != nil {
			return e
		}
		tx, e := c.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = fn(ctx, tx); e != nil {
			return e
		}
		return sanitize("account commit", tx.Commit())
	}, parent))
}
func (r AccountRepository) Credentials(parent context.Context, email string) (account.Record, error) {
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	var row account.Record
	e := r.Store.DB.QueryRowContext(ctx, `SELECT id,email,display_name,role,demo,password_verifier,created_at FROM commerce_accounts WHERE email=$1`, email).Scan(&row.ID, &row.Email, &row.DisplayName, &row.Role, &row.Demo, &row.Verifier, &row.CreatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return row, account.Fail("not_found")
	}
	return row, sanitize("account credentials", e)
}
func insertAccount(ctx context.Context, tx *sql.Tx, row account.Record) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO commerce_accounts(id,email,display_name,role,demo,password_verifier,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7)`, row.ID, row.Email, row.DisplayName, row.Role, row.Demo, row.Verifier, row.CreatedAt)
	if SQLState(e) == "23505" {
		return account.Fail("duplicate_email")
	}
	return e
}
func issueSession(ctx context.Context, tx *sql.Tx, id, digest, old string, now int64) error {
	if _, e := tx.ExecContext(ctx, `UPDATE commerce_sessions SET revoked_at=$2 WHERE token_digest=$1 AND revoked_at IS NULL AND expires_at>$2`, old, now); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `DELETE FROM commerce_sessions WHERE token_digest IN (SELECT token_digest FROM commerce_sessions WHERE expires_at<=$1 ORDER BY expires_at LIMIT 100)`, now); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO commerce_sessions(token_digest,account_id,created_at,expires_at) VALUES($1,$2,$3,$4)`, digest, id, now, now+account.SessionLifetime.Milliseconds())
	return e
}
func (r AccountRepository) Register(ctx context.Context, row account.Record, digest, old string, now int64) (account.Profile, error) {
	row.Role = "bidder"
	row.Demo = false
	e := r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if e := insertAccount(ctx, tx, row); e != nil {
			return e
		}
		return issueSession(ctx, tx, row.ID, digest, old, now)
	})
	if e != nil {
		return account.Profile{}, e
	}
	return row.Profile, nil
}
func (r AccountRepository) Login(ctx context.Context, id, digest, old string, now int64) (account.Profile, error) {
	var p account.Profile
	e := r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var e error
		p, e = scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileColumns+` FROM commerce_accounts a WHERE a.id=$1`, id))
		if e != nil {
			return e
		}
		return issueSession(ctx, tx, id, digest, old, now)
	})
	return p, e
}
func (r AccountRepository) Profile(parent context.Context, digest string, now int64) (account.Profile, error) {
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	return sessionProfile(ctx, r.Store.DB, digest, now)
}
func (r AccountRepository) Update(ctx context.Context, digest, name string, now int64) (account.Profile, error) {
	var p account.Profile
	e := r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var e error
		p, e = sessionProfile(ctx, tx, digest, now)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE commerce_accounts SET display_name=$1,updated_at=$2 WHERE id=$3`, name, now, p.ID)
		p.DisplayName = name
		return e
	})
	return p, e
}
func (r AccountRepository) Logout(ctx context.Context, digest string, now int64) error {
	return r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE commerce_sessions SET revoked_at=$2 WHERE token_digest=$1 AND revoked_at IS NULL`, digest, now)
		return e
	})
}
func (r AccountRepository) Seeded(parent context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	var done bool
	e := r.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_account_seed_runs WHERE version=1)`).Scan(&done)
	return done, sanitize("account seed read", e)
}
func (r AccountRepository) Seed(ctx context.Context, rows []account.Record, now int64) (bool, error) {
	already := false
	e := r.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_account_seed_runs WHERE version=1)`).Scan(&already); e != nil || already {
			return e
		}
		for _, row := range rows {
			if e := insertAccount(ctx, tx, row); e != nil {
				return e
			}
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO commerce_account_seed_runs VALUES(1,$1)`, now)
		return e
	})
	return already, e
}
