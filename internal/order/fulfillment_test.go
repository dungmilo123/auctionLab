package order

import (
	"strings"
	"testing"
)

func TestShipmentBounds(t *testing.T) {
	for _, tc := range []struct {
		c, r string
		ok   bool
	}{{strings.Repeat("航", 120), strings.Repeat("参", 200), true}, {strings.Repeat("航", 121), "r", false}, {"c", strings.Repeat("参", 201), false}, {"c\n", "r", false}, {"c", "r\x00", false}, {" ", "r", false}, {"c", " ", false}, {" <b>carrier</b> ", " https://example.test/ref ", true}} {
		v, e := NormalizeTransition(Transition{ExpectedVersion: 2, NextStatus: "shipped", Shipment: &Shipment{tc.c, tc.r}})
		if (e == nil) != tc.ok {
			t.Fatal(tc, e)
		}
		if e == nil && v.Shipment.Carrier != strings.TrimSpace(tc.c) {
			t.Fatal(v)
		}
	}
	if _, e := NormalizeTransition(Transition{ExpectedVersion: 1, NextStatus: "preparing", Shipment: &Shipment{Carrier: "c", TrackingReference: "r"}}); e == nil {
		t.Fatal("unexpected shipment")
	}
}
