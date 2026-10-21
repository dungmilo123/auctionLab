package store

import (
	"context"
	"testing"
)

func TestRecommendationRepositoryExclusionsAndSource(t *testing.T) {
	s, a := auctionFixture(t)
	ctx := context.Background()
	r := RecommendationRepository{s}
	out, e := r.Recommend(ctx, "", "", 100)
	if e != nil || len(out) != 2 {
		t.Fatal(out, e)
	}
	out, e = r.Recommend(ctx, "", "lot", 100)
	if e != nil || len(out) != 1 || out[0].Lot.ID != "other" {
		t.Fatal(out, e)
	}
	yes := true
	if _, e = (AccountRepository{s}).Membership(ctx, "a", "l", &yes, 100); e != nil {
		t.Fatal(e)
	}
	out, e = r.Recommend(ctx, "a", "", 100)
	if e != nil || len(out) != 1 || out[0].Lot.ID != "other" {
		t.Fatal(out, e)
	}
	reconcile(t, a, 200)
	out, e = r.Recommend(ctx, "", "", 150)
	if e != nil || len(out) != 0 {
		t.Fatal("terminal leak", out, e)
	}
	out, e = r.Recommend(ctx, "", "lot", 150)
	if e != nil || len(out) != 0 {
		t.Fatal("historical source", out, e)
	}
	if _, e = r.Recommend(ctx, "invalid", "", 100); e == nil {
		t.Fatal("invalid owner")
	}
}
