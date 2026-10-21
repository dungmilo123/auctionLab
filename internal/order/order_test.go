package order

import (
	"auctionlab/internal/catalog"
	"strings"
	"testing"
)

func testOrder() Order {
	fee := int64(1500)
	return Order{Currency: "USD", WinningAmountMinor: 12550, Snapshot: Snapshot{Fulfillment: catalog.Fulfillment{ShippingEnabled: true, PickupEnabled: true, ShippingFeeMinor: &fee, ShippingInstructions: "Ship", PickupInstructions: "Pickup"}}}
}
func valid() Selection {
	return Selection{Contact: Contact{" Winner ", "BUYER@example.test", " +84 123 "}, FulfillmentMethod: "shipping", ShippingAddress: &Address{Line1: " 1 Street ", Locality: "Hà Nội", Country: "vn"}, PaymentMethod: "demo-card"}
}
func TestNormalizationAndLimits(t *testing.T) {
	s, e := Normalize(valid())
	if e != nil || s.Contact.Email != "buyer@example.test" || s.ShippingAddress.Country != "VN" || s.ShippingAddress.Line1 != "1 Street" {
		t.Fatal(s, e)
	}
	for _, tc := range []struct {
		field string
		limit int
	}{{"name", 120}, {"email", 254}, {"phone", 40}, {"line1", 200}, {"line2", 200}, {"locality", 120}, {"region", 120}, {"postal", 32}} {
		for _, extra := range []int{0, 1} {
			v := valid()
			x := strings.Repeat("ấ", tc.limit+extra)
			switch tc.field {
			case "name":
				v.Contact.Name = x
			case "email":
				v.Contact.Email = strings.Repeat("x", tc.limit+extra-13) + "@example.test"
			case "phone":
				v.Contact.Phone = x
			case "line1":
				v.ShippingAddress.Line1 = x
			case "line2":
				v.ShippingAddress.Line2 = x
			case "locality":
				v.ShippingAddress.Locality = x
			case "region":
				v.ShippingAddress.Region = x
			case "postal":
				v.ShippingAddress.PostalCode = x
			}
			_, e := Normalize(v)
			if (e != nil) != (extra == 1) {
				t.Fatalf("%s %d: %v", tc.field, extra, e)
			}
		}
	}
	for _, change := range []func(*Selection){func(s *Selection) { s.Contact.Name = "\ninside\nname" }, func(s *Selection) { s.Contact.Email = "name <x@example.test>" }, func(s *Selection) { s.ShippingAddress.Country = "V1" }, func(s *Selection) { s.Contact.Phone = "" }, func(s *Selection) { s.PaymentMethod = "card" }, func(s *Selection) { s.ShippingAddress = nil }} {
		s := valid()
		change(&s)
		if _, e := Normalize(s); e == nil {
			t.Fatal("invalid accepted")
		}
	}
	s = valid()
	s.FulfillmentMethod = "pickup"
	s.ShippingAddress.Country = "invalid"
	n, e := Normalize(s)
	if e != nil || n.ShippingAddress != nil {
		t.Fatal(n, e)
	}
}
func TestExactReview(t *testing.T) {
	o := testOrder()
	for _, tc := range []struct {
		method string
		want   int64
	}{{"shipping", 14050}, {"pickup", 12550}} {
		s := valid()
		s.FulfillmentMethod = tc.method
		r, e := MakeReview(o, s)
		if e != nil || r.Total.TotalMinor != tc.want {
			t.Fatal(r, e)
		}
	}
	o.WinningAmountMinor = catalog.MaxMoney
	f := catalog.MaxMoney
	o.Snapshot.Fulfillment.ShippingFeeMinor = &f
	r, e := MakeReview(o, valid())
	if e != nil || r.Total.TotalMinor != MaxTotal {
		t.Fatal(r, e)
	}
	o.WinningAmountMinor = 0
	f = 0
	r, e = MakeReview(o, valid())
	if e != nil || r.Total.TotalMinor != 0 {
		t.Fatal(r, e)
	}
	o.Snapshot.Fulfillment.ShippingFeeMinor = nil
	if _, e = MakeReview(o, valid()); e == nil {
		t.Fatal("missing fee became zero")
	}
	o.Snapshot.Fulfillment.ShippingEnabled = false
	if _, e = MakeReview(o, valid()); e == nil {
		t.Fatal("disabled shipping")
	}
	o.Snapshot.Fulfillment.PickupEnabled = false
	if _, e = Options(o); e == nil {
		t.Fatal("no enabled method")
	}
}
