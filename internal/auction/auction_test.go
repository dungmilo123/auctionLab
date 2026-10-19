package auction

import (
	"auctionlab/internal/catalog"
	"testing"
)

func TestMinimumAndIntent(t *testing.T) {
	for _, v := range []struct {
		opening, increment int64
		top                *int64
		want               *int64
		limit              bool
	}{
		{0, 1, nil, ptr(0), false}, {12550, 500, nil, ptr(12550), false}, {12550, 500, ptr(14001), ptr(14501), false}, {0, 1, ptr(catalog.MaxMoney), nil, true},
	} {
		got, limit := Minimum(v.opening, v.increment, v.top)
		if limit != v.limit || (got == nil) != (v.want == nil) || got != nil && *got != *v.want {
			t.Fatal(got, limit, v)
		}
	}
	valid := Intent{LotID: "l", AmountMinor: ptr(0), RequestID: "00000000-0000-0000-0000-000000000001"}
	if Validate(valid) != nil {
		t.Fatal("zero rejected")
	}
	for _, amount := range []*int64{nil, ptr(-1), ptr(catalog.MaxMoney + 1)} {
		v := valid
		v.AmountMinor = amount
		if Validate(v) == nil {
			t.Fatal(v)
		}
	}
	valid.RequestID = "invalid"
	if Validate(valid) == nil {
		t.Fatal("request id accepted")
	}
}
func ptr(v int64) *int64 { return &v }
