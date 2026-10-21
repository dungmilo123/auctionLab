package store

import (
	"auctionlab/internal/account"
	"auctionlab/internal/analytics"
	"auctionlab/internal/sqliteimport"
	"auctionlab/internal/testpg"
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"
)

type demoMedia bool

func (m demoMedia) Readable(string) bool { return bool(m) }
func demoFixture(t *testing.T) *Store {
	t.Helper()
	raw := testpg.NewURL(t)
	u, _ := url.Parse(raw)
	name := "auctionlab_demo_" + u.Query().Get("search_path")
	s, e := OpenConfig(Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: name})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		s.Close()
		control, e := Open(raw)
		if e != nil {
			t.Error(e)
			return
		}
		defer control.Close()
		if _, e = control.DB.Exec(`DROP SCHEMA IF EXISTS "` + name + `" CASCADE`); e != nil {
			t.Error(e)
		}
	})
	return s
}
func demoRows(prefix string) []account.Record {
	out := []account.Record{}
	for i := 0; i < 3; i++ {
		out = append(out, account.Record{Profile: account.Profile{ID: fmt.Sprint(prefix, i), Email: fmt.Sprintf("demo%d@example.test", i), DisplayName: "Demo", Role: "bidder", Demo: true}, Verifier: "fixture-hash", CreatedAt: 1})
	}
	return out
}
func prepareFixture(t *testing.T, s *Store) DemoEnvironment {
	t.Helper()
	ctx := context.Background()
	if _, found, e := s.DemoPrepared(ctx); e != nil || found {
		t.Fatal(found, e)
	}
	if e := s.Migrate(); e != nil {
		t.Fatal(e)
	}
	d, e := s.SeedDemo(ctx, false, "", time.UnixMilli(1000), demoRows("original"), demoMedia(true))
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestDemoPrepareResetAndReadOnlyStatus(t *testing.T) {
	s := demoFixture(t)
	ctx := context.Background()
	if _, e := s.DemoStatus(ctx); e == nil {
		t.Fatal("unmarked status")
	}
	var exists bool
	s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, s.SchemaName).Scan(&exists)
	if exists {
		t.Fatal("status initialized schema")
	}
	control, e := Open(testpg.NewURL(t))
	if e != nil {
		t.Fatal(e)
	}
	defer control.Close()
	mustSQL(t, control, `CREATE TABLE sentinel(value text);INSERT INTO sentinel VALUES('other-schema-data')`)
	mediaPath := t.TempDir() + "/media-sentinel"
	if e = os.WriteFile(mediaPath, []byte("unchanged-media"), 0600); e != nil {
		t.Fatal(e)
	}
	original := prepareFixture(t, s)
	var lots, events, orders int
	s.DB.QueryRow(`SELECT count(*) FROM catalog_lots WHERE state='draft'`).Scan(&lots)
	s.DB.QueryRow(`SELECT count(*) FROM catalog_events WHERE state='draft'`).Scan(&events)
	s.DB.QueryRow(`SELECT count(*) FROM commerce_orders`).Scan(&orders)
	if lots != 18 || events != 3 || orders != 0 {
		t.Fatal(lots, events, orders)
	}
	var offsets bool
	s.DB.QueryRow(`SELECT bool_and(opens_at=1000+substring(id from '[0-9]+$')::bigint*86400000 AND first_closes_at=opens_at+3600000 AND close_interval_ms=60000) FROM catalog_events`).Scan(&offsets)
	if !offsets {
		t.Fatal("changed seed offsets")
	}
	repeat, found, e := s.DemoPrepared(ctx)
	if e != nil || !found || repeat.Generation != original.Generation {
		t.Fatal(repeat, found, e)
	}
	mustSQL(t, s, `INSERT INTO server_config VALUES('admin_key','fixture-capability'); INSERT INTO commerce_sessions VALUES('session','original0',1,9999999999999,NULL)`)
	if _, e := OpenConfig(Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: s.SchemaName}); e == nil {
		t.Fatal("active writer accepted")
	}
	if _, e = s.SeedDemo(ctx, true, "wrong", time.UnixMilli(2000), demoRows("new"), demoMedia(true)); e == nil {
		t.Fatal("confirmation bypass")
	}
	// Media validation after clearing must roll back the clear, seeds and generation.
	if _, e = s.SeedDemo(ctx, true, s.SchemaName, time.UnixMilli(2000), demoRows("new"), demoMedia(false)); e == nil {
		t.Fatal("partial reseed accepted")
	}
	after, e := s.DemoStatus(ctx)
	if e != nil || !reflect.DeepEqual(after, original) {
		t.Fatal("generation changed on rollback", e)
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM commerce_sessions WHERE token_digest='session'`).Scan(&n)
	if n != 1 {
		t.Fatal("old session lost on rollback")
	}
	replacement, e := s.SeedDemo(ctx, true, s.SchemaName, time.UnixMilli(2000), demoRows("new"), demoMedia(true))
	if e != nil || replacement.Generation == original.Generation {
		t.Fatal(replacement, e)
	}
	// Treat the mutation response as lost; inspect durable status rather than reset again.
	durable, e := s.DemoStatus(ctx)
	if e != nil || durable.Generation != replacement.Generation || durable.SeedReference != 2000 {
		t.Fatal(durable, e)
	}
	var ok bool
	s.DB.QueryRow(`SELECT (SELECT count(*)=0 FROM commerce_sessions) AND (SELECT count(*)=0 FROM commerce_accounts WHERE id LIKE 'original%') AND (SELECT count(*)=3 FROM commerce_accounts) AND (SELECT value='fixture-capability' FROM server_config) AND (SELECT count(*)=5 FROM catalog_categories) AND (SELECT max(version)=6 FROM schema_migrations)`).Scan(&ok)
	if !ok {
		t.Fatal("reset scope")
	}
	var controlValue string
	if e = control.DB.QueryRow(`SELECT value FROM sentinel`).Scan(&controlValue); e != nil || controlValue != "other-schema-data" {
		t.Fatal("control schema changed", e)
	}
	mediaAfter, e := os.ReadFile(mediaPath)
	if e != nil || string(mediaAfter) != "unchanged-media" {
		t.Fatal("media changed", e)
	}
	// Reopen independent read-only path and establish no schema/ledger initialization.
	read, e := OpenConfig(Config{URL: os.Getenv("TEST_DATABASE_URL"), Schema: s.SchemaName, ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	defer read.Close()
	if _, e = read.DemoStatus(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestDemoRefusesUnsafeLayouts(t *testing.T) {
	for _, layout := range []string{"unknown", "sequence", "view", "trigger", "reference", "import", "occupied"} {
		t.Run(layout, func(t *testing.T) {
			s := demoFixture(t)
			ctx := context.Background()
			d := prepareFixture(t, s)
			switch layout {
			case "unknown":
				mustSQL(t, s, `CREATE TABLE alien(sentinel text);INSERT INTO alien VALUES('untouched')`)
			case "sequence":
				mustSQL(t, s, `CREATE SEQUENCE unrelated_sequence`)
			case "view":
				mustSQL(t, s, `CREATE VIEW unrelated_view AS SELECT id FROM catalog_lots`)
			case "trigger":
				mustSQL(t, s, `CREATE FUNCTION custom_trigger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;CREATE TRIGGER custom_trigger BEFORE INSERT ON catalog_lots FOR EACH ROW EXECUTE FUNCTION custom_trigger()`)
			case "reference":
				// A separate owned control schema with a dependency must never be cascaded.
				control, e := Open(testpg.NewURL(t))
				if e != nil {
					t.Fatal(e)
				}
				defer control.Close()
				mustSQL(t, control, `CREATE TABLE control(id text REFERENCES "`+s.SchemaName+`".catalog_lots(id));INSERT INTO control VALUES('demo-lot-01')`)
			case "import":
				mustSQL(t, s, `INSERT INTO sqlite_import_runs VALUES('fixture',4,'{}',1)`)
			case "occupied":
				mustSQL(t, s, `DELETE FROM demo_environments`)
			}
			if _, e := s.SeedDemo(ctx, true, s.SchemaName, time.UnixMilli(2000), demoRows("new"), demoMedia(true)); e == nil {
				t.Fatal("unsafe reset accepted")
			}
			if _, _, e := s.DemoPrepared(ctx); e == nil {
				t.Fatal("unsafe preparation accepted")
			}
			var count int
			s.DB.QueryRow(`SELECT count(*) FROM catalog_lots`).Scan(&count)
			if count != 18 {
				t.Fatal("unsafe reset erased lots")
			}
			_ = d
		})
	}
}
func TestDemoRejectsUnmarkedAndImportOccupancy(t *testing.T) {
	s := demoFixture(t)
	ctx := context.Background()
	if e := s.Migrate(); e != nil {
		t.Fatal(e)
	}
	mustSQL(t, s, `INSERT INTO catalog_lots(id,slug,title,created_at,updated_at) VALUES('existing','existing','Existing',1,1)`)
	if _, _, e := s.DemoPrepared(ctx); e == nil {
		t.Fatal("adopted occupied target")
	}
	mustSQL(t, s, `DELETE FROM catalog_lots`)
	mustSQL(t, s, `INSERT INTO demo_environments VALUES(1,$1,1,$2,1000,1000)`, s.SchemaName, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	sourcePath := sourceFixture(t)
	before, _ := os.ReadFile(sourcePath)
	source, e := sqliteimport.Read(ctx, sourcePath)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ImportSQLite(ctx, source); e == nil {
		t.Fatal("demo marker ignored by importer")
	}
	after, _ := os.ReadFile(sourcePath)
	if string(before) != string(after) {
		t.Fatal("source modified")
	}
	for _, bad := range []string{"auctionlab", "public", "auctionlab_demo_", "auctionlab_demo_Bad", "auctionlab_demo_a;drop"} {
		if ValidDemoSchema(bad) {
			t.Fatal(bad)
		}
	}
}

func TestDemoInteractionResetCoverage(t *testing.T) {
	s := demoFixture(t)
	ctx := context.Background()
	prepareFixture(t, s)
	mustSQL(t, s, `UPDATE catalog_events SET state='published' WHERE id='demo-event-1';UPDATE catalog_lots SET state='published' WHERE id='demo-lot-01';INSERT INTO commerce_interactions VALUES('00000000000000000000000000000001','lot_view','demo-lot-01','demo-event-1',NULL,1000,1000,true,true,NULL,'observed','00000000-0000-0000-0000-000000000001');UPDATE commerce_interaction_coverage SET collection_started_at=1`)
	if _, e := s.SeedDemo(ctx, true, s.SchemaName, time.UnixMilli(2000), demoRows("new"), demoMedia(false)); e == nil {
		t.Fatal("failed reset accepted")
	}
	var ok bool
	if e := s.DB.QueryRow(`SELECT (SELECT count(*)=1 FROM commerce_interactions) AND (SELECT collection_started_at=1 FROM commerce_interaction_coverage)`).Scan(&ok); e != nil || !ok {
		t.Fatal("failed reset erased observations", e)
	}
	d, e := s.SeedDemo(ctx, true, s.SchemaName, time.UnixMilli(2000), demoRows("new"), demoMedia(true))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(`SELECT (SELECT count(*)=0 FROM commerce_interactions) AND (SELECT collection_started_at=$1 FROM commerce_interaction_coverage) AND (SELECT count(*)=0 FROM catalog_lots WHERE state='published')`, d.PreparedAt).Scan(&ok); e != nil || !ok {
		t.Fatal("reset generation coverage", e)
	}
	report, e := (AnalyticsRepository{s}).Report(ctx, analytics.Filters{Event: "all", Dataset: "all"})
	if e != nil || report.Data.Summary.Lots != "0" || len(report.Data.Interactions) != 0 {
		t.Fatal(report, e)
	}
}
