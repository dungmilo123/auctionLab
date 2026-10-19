// Package sqliteimport reads stopped, checkpointed SQLite-v4 snapshots only.
// It is the sole package importing the SQLite driver; it never writes a source.
package sqliteimport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

type Table struct {
	Name    string
	Columns []string
	Integer map[string]bool
	Order   string
	Rows    [][]any
	Digest  string
}
type Snapshot struct {
	Tables      []Table
	HighWater   map[string]int64
	Fingerprint string
}
type Summary struct {
	Count  int    `json:"count"`
	Digest string `json:"digest"`
}
type Report struct {
	Status        string             `json:"status"`
	SourceVersion int                `json:"source_version"`
	Fingerprint   string             `json:"fingerprint"`
	Tables        map[string]Summary `json:"tables,omitempty"`
	IdentityReady *bool              `json:"identity_ready,omitempty"`
}

func Definitions() []Table {
	specs := []struct{ name, columns, integers, order string }{
		{"players", "id nickname nickname_key token budget total_value joined_at last_seen_at", "budget total_value joined_at last_seen_at", "id"},
		{"items", "id order_index name description image_url auction_type truth_value hint_min hint_max start_price min_increment floor_price decrement_step decrement_interval_ms duration_ms anti_snipe_mode anti_snipe_window_ms max_extensions status", "order_index truth_value hint_min hint_max start_price min_increment floor_price decrement_step decrement_interval_ms duration_ms anti_snipe_window_ms max_extensions", "id"},
		{"sealed_bids", "item_id player_id amount submitted_at updated_at", "amount submitted_at updated_at", "item_id,player_id"},
		{"bids", "id item_id player_id amount kind created_at", "id amount created_at", "id"},
		{"auction_runtime", "item_id started_at deadline_at settled_at revision extensions_used", "started_at deadline_at settled_at revision extensions_used", "item_id"},
		{"game", "id phase current_item_id revision config_json updated_at", "id revision updated_at", "id"},
		{"events", "id type payload_json created_at", "id created_at", "id"},
		{"server_config", "key value", "", "key"},
	}
	out := make([]Table, 0, len(specs))
	for _, s := range specs {
		ints := map[string]bool{}
		for _, c := range strings.Fields(s.integers) {
			ints[c] = true
		}
		out = append(out, Table{Name: s.name, Columns: strings.Fields(s.columns), Integer: ints, Order: s.order})
	}
	return out
}
func fileDigest(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, errors.New("source snapshot cannot be read")
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return [32]byte{}, errors.New("source snapshot read failed")
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}
func Read(ctx context.Context, path string) (*Snapshot, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("invalid snapshot path")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err = os.Stat(absolute + suffix); err == nil {
			return nil, errors.New("source has sidecars: stop writers, checkpoint and prepare a standalone consistent backup first")
		} else if !os.IsNotExist(err) {
			return nil, errors.New("source sidecar preflight failed")
		}
	}
	before, err := fileDigest(absolute)
	if err != nil {
		return nil, err
	}
	// Immutable is safe only for this standalone offline snapshot with no sidecars.
	// It prevents a WAL-mode header from creating new sidecars during read-only access.
	u := url.URL{Scheme: "file", Path: absolute}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("immutable", "1")
	q.Add("_pragma", "query_only(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, errors.New("cannot open source read-only")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, errors.New("cannot begin source snapshot")
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		return nil, errors.New("supported source is SQLite schema v4; upgrade a separate working copy with the prior compatible binary")
	}
	var integrity string
	if err = tx.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return nil, errors.New("source integrity check failed")
	}
	refs, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, errors.New("source reference check failed")
	}
	invalid := refs.Next()
	err = refs.Err()
	refs.Close()
	if invalid || err != nil {
		return nil, errors.New("source contains invalid references")
	}
	snap := &Snapshot{Tables: Definitions(), HighWater: map[string]int64{}}
	for i := range snap.Tables {
		t := &snap.Tables[i]
		// Explicit projections reject missing/incompatible columns instead of normalizing them.
		rows, err := tx.QueryContext(ctx, "SELECT "+strings.Join(t.Columns, ",")+" FROM "+t.Name+" ORDER BY "+t.Order)
		if err != nil {
			return nil, fmt.Errorf("source %s columns incompatible", t.Name)
		}
		for rows.Next() {
			vals := make([]any, len(t.Columns))
			dest := make([]any, len(vals))
			for j := range vals {
				dest[j] = &vals[j]
			}
			if err = rows.Scan(dest...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("source %s row unreadable", t.Name)
			}
			for j, v := range vals {
				if v == nil {
					nullable := map[string]string{"items": " description image_url hint_min hint_max min_increment floor_price decrement_step decrement_interval_ms duration_ms anti_snipe_mode anti_snipe_window_ms max_extensions ", "auction_runtime": " deadline_at settled_at ", "game": " current_item_id ", "events": " payload_json "}
					if !strings.Contains(nullable[t.Name], " "+t.Columns[j]+" ") {
						rows.Close()
						return nil, fmt.Errorf("source %s.%s must not be null", t.Name, t.Columns[j])
					}
					continue
				}
				if t.Integer[t.Columns[j]] {
					if _, ok := v.(int64); !ok {
						rows.Close()
						return nil, fmt.Errorf("source %s.%s must be an exact integer", t.Name, t.Columns[j])
					}
				} else {
					s, ok := v.(string)
					if !ok || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
						rows.Close()
						return nil, fmt.Errorf("source %s.%s invalid text", t.Name, t.Columns[j])
					}
				}
			}
			t.Rows = append(t.Rows, vals)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("source %s read failed", t.Name)
		}
		t.Digest = Digest(t.Rows)
	}
	for _, table := range []string{"bids", "events"} {
		var maximum, seq sql.NullInt64
		if err = tx.QueryRowContext(ctx, "SELECT MAX(id) FROM "+table).Scan(&maximum); err != nil {
			return nil, errors.New("source identity probe failed")
		}
		if err = tx.QueryRowContext(ctx, "SELECT seq FROM sqlite_sequence WHERE name=?", table).Scan(&seq); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("source sequence history invalid")
		}
		n := max(maximum.Int64, seq.Int64, 0)
		if n == math.MaxInt64 {
			return nil, errors.New("source identity range exhausted")
		}
		snap.HighWater[table] = n
	}
	h := sha256.New()
	for _, t := range snap.Tables {
		writeText(h, t.Name)
		writeText(h, t.Digest)
	}
	for _, name := range []string{"bids", "events"} {
		writeText(h, name)
		writeInt(h, snap.HighWater[name])
	}
	snap.Fingerprint = hex.EncodeToString(h.Sum(nil))
	after, err := fileDigest(absolute)
	if err != nil || before != after {
		return nil, errors.New("source changed during read: prepare a stopped consistent snapshot")
	}
	return snap, nil
}
func writeInt(h hash.Hash, n int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n))
	h.Write(b[:])
}
func writeText(h hash.Hash, s string) { writeInt(h, int64(len(s))); h.Write([]byte(s)) }
func Digest(rows [][]any) string {
	h := sha256.New()
	for _, r := range rows {
		h.Write([]byte{'R'})
		for _, v := range r {
			switch x := v.(type) {
			case nil:
				h.Write([]byte{'N'})
			case int64:
				h.Write([]byte{'I'})
				writeInt(h, x)
			case string:
				h.Write([]byte{'T'})
				writeText(h, x)
			default:
				panic("non-canonical legacy type")
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Snapshot) Report(status string) Report {
	ready := true
	r := Report{Status: status, SourceVersion: 4, Fingerprint: s.Fingerprint, Tables: map[string]Summary{}, IdentityReady: &ready}
	for _, t := range s.Tables {
		r.Tables[t.Name] = Summary{len(t.Rows), t.Digest}
	}
	return r
}
