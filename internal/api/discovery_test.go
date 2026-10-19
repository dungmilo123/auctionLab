package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"auctionlab/internal/catalog"
	"auctionlab/internal/store"
	"auctionlab/internal/testpg"
)

func TestPublicCatalogClockAndPrivacy(t *testing.T) {
	db, err := store.Open(testpg.NewURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err = db.Bootstrap(time.UnixMilli(1000)); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var now atomic.Int64
	now.Store(1000)
	s := catalog.New(store.CatalogRepository{Store: db}, func() time.Time { return time.UnixMilli(now.Load()) }, catalog.LocalMedia{Embedded: os.DirFS("../../web/public")})
	if _, err = s.Seed(ctx, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	data, _ := s.Snapshot(ctx)
	e := data.Events[0]
	l := data.Lots[0]
	l.AdminNotes = "PRIVATE_SENTINEL"
	if _, err = s.SaveLot(ctx, l.ID, l.Version, l.LotContent); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterCatalog(mux, "private-test-key", s)
	server := httptest.NewServer(mux)
	defer server.Close()
	for _, path := range []string{"events/" + e.Slug, "lots/" + l.Slug} {
		status, draft := call(t, server, "GET", "/api/catalog/"+path, "", false, nil)
		_, missing := call(t, server, "GET", "/api/catalog/"+strings.Split(path, "/")[0]+"/missing", "", false, nil)
		if status != 404 || draft != missing {
			t.Fatal("draft existence disclosed", draft, missing)
		}
	}
	_, empty := call(t, server, "GET", "/api/catalog/events", "", false, nil)
	if !strings.Contains(empty, `"data":[]`) {
		t.Fatal(empty)
	}
	e, err = s.Event(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, e.ID, e.Version); err != nil {
		t.Fatal(err)
	}
	var revisionBefore int64
	if err = db.DB.QueryRow("SELECT revision FROM game WHERE id=1").Scan(&revisionBefore); err != nil {
		t.Fatal(err)
	}
	paths := []string{"categories", "events", "events/" + e.Slug, "lots", "lots/" + l.Slug}
	for _, path := range paths {
		var previous json.RawMessage
		for _, at := range []int64{1000, 2000} {
			now.Store(at)
			status, raw := call(t, server, "GET", "/api/catalog/"+path, "", false, nil)
			var envelope struct {
				Data json.RawMessage `json:"data"`
				Meta struct {
					Time int64 `json:"server_time"`
				} `json:"meta"`
			}
			if err = json.Unmarshal([]byte(raw), &envelope); err != nil {
				t.Fatal(err)
			}
			if status != 200 || envelope.Meta.Time != at || (previous != nil && string(previous) != string(envelope.Data)) {
				t.Fatal(status, raw)
			}
			for _, secret := range []string{"PRIVATE_SENTINEL", "admin_notes", "version", "private-test-key", "sealed_bids", "true_value"} {
				if strings.Contains(raw, secret) {
					t.Fatal("private field", secret)
				}
			}
			previous = envelope.Data
		}
	}
	var revisionAfter int64
	if err = db.DB.QueryRow("SELECT revision FROM game WHERE id=1").Scan(&revisionAfter); err != nil {
		t.Fatal(err)
	}
	if revisionBefore != revisionAfter {
		t.Fatal("read changed game revision")
	}
	db.Close()
	for _, path := range paths {
		status, raw := call(t, server, "GET", "/api/catalog/"+path, "", false, nil)
		if status < 500 || strings.Contains(raw, "postgres") || strings.Contains(raw, "PRIVATE") || strings.Contains(raw, `"meta"`) {
			t.Fatal(status, raw)
		}
	}
}
