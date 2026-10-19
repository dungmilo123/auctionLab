package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func ptr(n int64) *int64 { return &n }
func TestScheduleArithmetic(t *testing.T) {
	event := Event{EventContent: EventContent{OpensAt: ptr(1000), FirstClosesAt: ptr(2000), CloseIntervalMS: ptr(60000)}, LotIDs: []string{"C", "A", "B"}}
	slots, err := Schedule(event)
	if err != nil || len(slots) != 3 || slots[2].ClosesAt != 122000 || slots[0].OpensAt != 1000 {
		t.Fatal(slots, err)
	}
	for _, e := range []Event{{EventContent: EventContent{OpensAt: ptr(-1)}}, {EventContent: EventContent{OpensAt: ptr(2), FirstClosesAt: ptr(1)}}, {EventContent: EventContent{CloseIntervalMS: ptr(0)}}, {EventContent: EventContent{OpensAt: ptr(0), FirstClosesAt: ptr(MaxInstant), CloseIntervalMS: ptr(MaxInstant)}, LotIDs: []string{"a", "b"}}} {
		if _, err := Schedule(e); err == nil {
			t.Fatal("invalid schedule accepted", e)
		}
	}
	if slots, err = Schedule(Event{}); err != nil || len(slots) != 0 {
		t.Fatal("incomplete draft fabricated time")
	}
}
func TestMediaConfinement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(root, "ok.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600)
	os.WriteFile(filepath.Join(outside, "secret.svg"), []byte("<svg/>"), 0600)
	os.Symlink(filepath.Join(outside, "secret.svg"), filepath.Join(root, "escape.svg"))
	os.WriteFile(filepath.Join(root, "fake.png"), []byte("not an image"), 0600)
	media := LocalMedia{ImagesDir: root, Embedded: fstest.MapFS{"site-assets/catalog/demo.svg": &fstest.MapFile{Data: []byte("<svg/>")}}}
	for _, good := range []string{"/images/ok.svg", "/site-assets/catalog/demo.svg"} {
		if !media.Readable(good) {
			t.Fatal(good)
		}
	}
	for _, bad := range []string{"https://remote/image.svg", "//remote/image.svg", "/images/../secret.svg", "/images/%2e%2e/secret.svg", "/images/%252e%252e/secret.svg", "/images/a%2fb.svg", "/images/ok.svg?x=1", "/images/ok.svg#x", "/images/escape.svg", "/images/fake.png", "/images/missing.svg", "/images/a\\b.svg"} {
		if media.Readable(bad) {
			t.Fatal("unsafe media accepted", bad)
		}
	}
}
func TestMoneyAndDraftValidation(t *testing.T) {
	l := LotContent{Title: "Camera", Slug: "camera", Currency: "USD"}
	if err := validateLot(l, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{-1, MaxMoney + 1} {
		l.StartingPriceMinor = ptr(n)
		if err := validateLot(l, nil, false, nil); err == nil {
			t.Fatal(n)
		}
	}
	l.StartingPriceMinor = ptr(0)
	l.BidIncrementMinor = ptr(0)
	if err := validateLot(l, nil, false, nil); err == nil {
		t.Fatal("zero increment")
	}
	l.BidIncrementMinor = ptr(1)
	l.Fulfillment.ShippingFeeMinor = ptr(0)
	if err := validateLot(l, nil, false, nil); err != nil {
		t.Fatal(err)
	}
	l.Currency = "VND"
	if err := validateLot(l, nil, false, nil); err == nil {
		t.Fatal("alternate currency")
	}
}
