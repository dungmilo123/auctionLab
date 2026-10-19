package store

import (
	"auctionlab/internal/catalog"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type CatalogRepository struct{ Store *Store }
type catalogTx struct {
	ctx context.Context
	tx  *sql.Tx
}

func (r CatalogRepository) Read(parent context.Context, fn func(catalog.Reader) error) error {
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	tx, err := r.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return sanitize("catalog read", err)
	}
	defer tx.Rollback()
	return fn(&catalogTx{ctx, tx})
}
func (r CatalogRepository) Write(parent context.Context, fn func(catalog.Tx) error) error {
	if err := r.Store.withWriter(func(ctx context.Context, c *sql.Conn) error { return c.PingContext(ctx) }); err != nil {
		return sanitize("catalog ownership", err)
	}
	ctx, cancel := context.WithTimeout(parent, OperationTimeout)
	defer cancel()
	tx, err := r.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return sanitize("catalog transaction", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_database() || ':' || $1 || ':catalog',0))`, r.Store.SchemaName); err != nil {
		return sanitize("catalog lock", err)
	}
	if err = fn(&catalogTx{ctx, tx}); err != nil {
		return err
	}
	return sanitize("catalog commit", tx.Commit())
}
func (t *catalogTx) Load() (catalog.Snapshot, error) {
	out := catalog.Snapshot{Categories: []catalog.Category{}, Lots: []catalog.Lot{}, Events: []catalog.Event{}}
	rows, err := t.tx.QueryContext(t.ctx, "SELECT id,slug,name_en,name_vi,sort_order FROM catalog_categories ORDER BY sort_order")
	if err != nil {
		return out, sanitize("categories", err)
	}
	for rows.Next() {
		var c catalog.Category
		if err = rows.Scan(&c.ID, &c.Slug, &c.NameEN, &c.NameVI, &c.SortOrder); err != nil {
			rows.Close()
			return out, sanitize("category", err)
		}
		out.Categories = append(out.Categories, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, sanitize("categories", err)
	}
	rows, err = t.tx.QueryContext(t.ctx, `SELECT id,slug,title,category_id,description,brand,era,story,condition,specifications,fulfillment,starting_price_minor,bid_increment_minor,currency,state,admin_notes,version,created_at,updated_at,demo FROM catalog_lots ORDER BY created_at,id COLLATE "C"`)
	if err != nil {
		return out, sanitize("lots", err)
	}
	indexes := map[string]int{}
	for rows.Next() {
		var l catalog.Lot
		var specs, fulfill []byte
		if err = rows.Scan(&l.ID, &l.Slug, &l.Title, &l.CategoryID, &l.Description, &l.Brand, &l.Era, &l.Story, &l.Condition, &specs, &fulfill, &l.StartingPriceMinor, &l.BidIncrementMinor, &l.Currency, &l.State, &l.AdminNotes, &l.Version, &l.CreatedAt, &l.UpdatedAt, &l.Demo); err != nil {
			rows.Close()
			return out, sanitize("lot", err)
		}
		if err = json.Unmarshal(specs, &l.Specifications); err == nil {
			err = json.Unmarshal(fulfill, &l.Fulfillment)
		}
		if err != nil {
			rows.Close()
			return out, sanitize("lot JSON", err)
		}
		l.Images = []catalog.Image{}
		indexes[l.ID] = len(out.Lots)
		out.Lots = append(out.Lots, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, sanitize("lots", err)
	}
	rows, err = t.tx.QueryContext(t.ctx, "SELECT lot_id,path,alt FROM catalog_lot_images ORDER BY lot_id,position")
	if err != nil {
		return out, sanitize("images", err)
	}
	for rows.Next() {
		var id string
		var image catalog.Image
		if err = rows.Scan(&id, &image.Path, &image.Alt); err != nil {
			rows.Close()
			return out, sanitize("image", err)
		}
		i, ok := indexes[id]
		if !ok {
			rows.Close()
			return out, catalog.Fail("storage")
		}
		out.Lots[i].Images = append(out.Lots[i].Images, image)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, sanitize("images", err)
	}
	rows, err = t.tx.QueryContext(t.ctx, `SELECT id,slug,title,theme,description,cover_path,cover_alt,opens_at,first_closes_at,close_interval_ms,state,admin_notes,version,created_at,updated_at,demo FROM catalog_events ORDER BY created_at,id COLLATE "C"`)
	if err != nil {
		return out, sanitize("events", err)
	}
	eventIndexes := map[string]int{}
	for rows.Next() {
		var e catalog.Event
		if err = rows.Scan(&e.ID, &e.Slug, &e.Title, &e.Theme, &e.Description, &e.CoverPath, &e.CoverAlt, &e.OpensAt, &e.FirstClosesAt, &e.CloseIntervalMS, &e.State, &e.AdminNotes, &e.Version, &e.CreatedAt, &e.UpdatedAt, &e.Demo); err != nil {
			rows.Close()
			return out, sanitize("event", err)
		}
		e.LotIDs = []string{}
		eventIndexes[e.ID] = len(out.Events)
		out.Events = append(out.Events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, sanitize("events", err)
	}
	rows, err = t.tx.QueryContext(t.ctx, "SELECT event_id,lot_id FROM catalog_event_lots ORDER BY event_id,position")
	if err != nil {
		return out, sanitize("members", err)
	}
	for rows.Next() {
		var eventID, lotID string
		if err = rows.Scan(&eventID, &lotID); err != nil {
			rows.Close()
			return out, sanitize("member", err)
		}
		ei, ok := eventIndexes[eventID]
		li, ok2 := indexes[lotID]
		if !ok || !ok2 {
			rows.Close()
			return out, catalog.Fail("storage")
		}
		out.Events[ei].LotIDs = append(out.Events[ei].LotIDs, lotID)
		v := eventID
		out.Lots[li].EventID = &v
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, sanitize("members", err)
	}
	for i := range out.Events {
		out.Events[i].Schedule, err = catalog.Schedule(out.Events[i])
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (t *catalogTx) put(table string, columns []string, args []any, expected int64) error {
	params := make([]string, len(args))
	for i := range args {
		params[i] = fmt.Sprintf("$%d", i+1)
	}
	query := "INSERT INTO " + table + " (" + strings.Join(columns, ",") + ") VALUES (" + strings.Join(params, ",") + ")"
	if expected != 0 {
		sets := []string{}
		for i, c := range columns {
			if i > 0 {
				sets = append(sets, c+"="+params[i])
			}
		}
		args = append(args, expected)
		query = "UPDATE " + table + " SET " + strings.Join(sets, ",") + fmt.Sprintf(" WHERE id=$1 AND version=$%d", len(args))
	}
	result, err := t.tx.ExecContext(t.ctx, query, args...)
	if err != nil {
		return sanitize("catalog write", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return sanitize("catalog result", err)
	}
	if n != 1 {
		return catalog.Fail("stale_version")
	}
	return nil
}
func (t *catalogTx) PutLot(l catalog.Lot, expected int64) error {
	specs, err := json.Marshal(l.Specifications)
	if err != nil {
		return err
	}
	fulfill, err := json.Marshal(l.Fulfillment)
	if err != nil {
		return err
	}
	cols := strings.Fields("id slug title category_id description brand era story condition specifications fulfillment starting_price_minor bid_increment_minor currency state admin_notes version created_at updated_at demo")
	if err = t.put("catalog_lots", cols, []any{l.ID, l.Slug, l.Title, l.CategoryID, l.Description, l.Brand, l.Era, l.Story, l.Condition, string(specs), string(fulfill), l.StartingPriceMinor, l.BidIncrementMinor, l.Currency, l.State, l.AdminNotes, l.Version, l.CreatedAt, l.UpdatedAt, l.Demo}, expected); err != nil {
		return err
	}
	if _, err = t.tx.ExecContext(t.ctx, "DELETE FROM catalog_lot_images WHERE lot_id=$1", l.ID); err != nil {
		return sanitize("images replace", err)
	}
	for i, image := range l.Images {
		if _, err = t.tx.ExecContext(t.ctx, "INSERT INTO catalog_lot_images VALUES($1,$2,$3,$4)", l.ID, i, image.Path, image.Alt); err != nil {
			return sanitize("image write", err)
		}
	}
	return nil
}
func (t *catalogTx) PutEvent(e catalog.Event, expected int64) error {
	return t.put("catalog_events", strings.Fields("id slug title theme description cover_path cover_alt opens_at first_closes_at close_interval_ms state admin_notes version created_at updated_at demo"), []any{e.ID, e.Slug, e.Title, e.Theme, e.Description, e.CoverPath, e.CoverAlt, e.OpensAt, e.FirstClosesAt, e.CloseIntervalMS, e.State, e.AdminNotes, e.Version, e.CreatedAt, e.UpdatedAt, e.Demo}, expected)
}
func (t *catalogTx) ReplaceMembers(id string, ids []string) error {
	if _, err := t.tx.ExecContext(t.ctx, "DELETE FROM catalog_event_lots WHERE event_id=$1", id); err != nil {
		return sanitize("membership replace", err)
	}
	for i, lot := range ids {
		if _, err := t.tx.ExecContext(t.ctx, "INSERT INTO catalog_event_lots VALUES($1,$2,$3)", id, lot, i); err != nil {
			return sanitize("membership write", err)
		}
	}
	return nil
}
func (t *catalogTx) Seeded(version int) (bool, error) {
	var n int
	err := t.tx.QueryRowContext(t.ctx, "SELECT count(*) FROM catalog_seed_runs WHERE version=$1", version).Scan(&n)
	return n > 0, sanitize("seed marker", err)
}
func (t *catalogTx) MarkSeed(version int, reference int64) error {
	_, err := t.tx.ExecContext(t.ctx, "INSERT INTO catalog_seed_runs VALUES($1,$2)", version, reference)
	return sanitize("seed marker", err)
}

var _ catalog.Repository = CatalogRepository{}
