// Package recommendation provides an explainable rules-v1 baseline, not ML.
package recommendation

import (
	"auctionlab/internal/catalog"
	"context"
	"sort"
	"strings"
)

type Item struct {
	Lot    catalog.PublicLot `json:"lot"`
	Reason string            `json:"reason"`
}
type Repository interface {
	Recommend(context.Context, string, string, int64) ([]Item, error)
}

func Era(s *string) string {
	if s == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*s))
}

// Rank counts each category/era feature once. Source context takes precedence
// over preference sets; zero-score entries fill with deterministic closing order.
func Rank(lots []catalog.PublicLot, source *catalog.PublicLot, saved map[string]bool, terminal map[string]bool, now int64) []Item {
	categories, eras := map[string]bool{}, map[string]bool{}
	prefix := ""
	add := func(l catalog.PublicLot) {
		if l.CategoryID != nil {
			categories[*l.CategoryID] = true
		}
		if e := Era(l.Era); e != "" {
			eras[e] = true
		}
	}
	if source != nil {
		add(*source)
	} else {
		prefix = "watchlist_"
		for _, l := range lots {
			if saved[l.ID] {
				add(l)
			}
		}
	}
	type scored struct {
		Item
		score int
	}
	all := []scored{}
	for _, l := range lots {
		if l.ClosesAt == nil || *l.ClosesAt <= now || terminal[l.ID] || saved[l.ID] || (source != nil && l.ID == source.ID) {
			continue
		}
		score := 0
		reason := "closing_soon"
		if l.CategoryID != nil && categories[*l.CategoryID] {
			score += 4
		}
		if e := Era(l.Era); e != "" && eras[e] {
			score += 2
		}
		switch score {
		case 6:
			reason = prefix + "category_and_era"
		case 4:
			reason = prefix + "category"
		case 2:
			reason = prefix + "era"
		}
		all = append(all, scored{Item{l, reason}, score})
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if *a.Lot.ClosesAt != *b.Lot.ClosesAt {
			return *a.Lot.ClosesAt < *b.Lot.ClosesAt
		}
		return a.Lot.Slug < b.Lot.Slug
	})
	if len(all) > 6 {
		all = all[:6]
	}
	out := []Item{}
	for _, a := range all {
		out = append(out, a.Item)
	}
	return out
}
