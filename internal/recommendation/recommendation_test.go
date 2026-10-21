package recommendation

import (
	"auctionlab/internal/catalog"
	"testing"
)

func ptr[T any](v T) *T { return &v }
func lot(id, cat, era string, close int64) catalog.PublicLot {
	return catalog.PublicLot{ID: id, Slug: id, CategoryID: ptr(cat), Era: ptr(era), ClosesAt: &close}
}
func TestRulesRanking(t *testing.T) {
	source := lot("source", "c", "  1970S  ", 100)
	lots := []catalog.PublicLot{source, lot("six", "c", "1970s", 90), lot("four", "c", "1980s", 80), lot("two", "d", "1970s", 70), lot("zero-b", "d", "1980s", 60), lot("zero-a", "d", "1980s", 60), lot("expired", "c", "1970s", 1), lot("terminal", "c", "1970s", 90), lot("saved", "c", "1970s", 90)}
	saved := map[string]bool{"saved": true}
	out := Rank(lots, &source, saved, map[string]bool{"terminal": true}, 1)
	want := []string{"six", "four", "two", "zero-a", "zero-b"}
	reasons := []string{"category_and_era", "category", "era", "closing_soon", "closing_soon"}
	if len(out) != len(want) {
		t.Fatal(out)
	}
	for i, v := range out {
		if v.Lot.ID != want[i] || v.Reason != reasons[i] {
			t.Fatal(i, v)
		}
	}
	out = Rank(lots, nil, saved, map[string]bool{}, 1)
	if out[0].Reason != "watchlist_category_and_era" || len(out) != 6 {
		t.Fatal(out)
	}
	if Era(ptr(" \t ")) != "" || Era(nil) != "" {
		t.Fatal("blank era")
	}
	out = Rank([]catalog.PublicLot{lot("a", "", "", 100)}, nil, nil, nil, 1)
	if len(out) != 1 || out[0].Reason != "closing_soon" {
		t.Fatal("empty features matched")
	}
	if len(Rank(nil, nil, nil, nil, 0)) != 0 {
		t.Fatal("empty")
	}
}

func TestSourcePrecedenceAndRepeatedPreferenceSets(t *testing.T) {
	source := lot("source", "c", "1970s", 100)
	saved1 := lot("saved-1", "d", "1990s", 100)
	saved2 := lot("saved-2", "d", "1990s", 100)
	lots := []catalog.PublicLot{source, saved1, saved2, lot("category-d", "d", "1980s", 80), lot("category-c", "c", "1980s", 90), lot("era-d", "z", "1990s", 70)}
	saved := map[string]bool{"saved-1": true, "saved-2": true}
	out := Rank(lots, &source, saved, nil, 1)
	if len(out) != 3 || out[0].Lot.ID != "category-c" || out[0].Reason != "category" {
		t.Fatal("source lost precedence", out)
	}
	out = Rank(lots, nil, saved, nil, 1)
	if out[0].Lot.ID != "category-d" || out[0].Reason != "watchlist_category" || out[1].Lot.ID != "era-d" || out[1].Reason != "watchlist_era" {
		t.Fatal("preference scores accumulated", out)
	}
}
