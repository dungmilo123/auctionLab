package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func identity(title string) (string, string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", Fail("entropy")
	}
	id := hex.EncodeToString(b[:])
	base := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(title), "-")
	base = strings.Trim(base, "-")
	if len(base) > 80 {
		base = strings.TrimRight(base[:80], "-")
	}
	if base == "" {
		base = "lot-" + id[:12]
	}
	return id, base, nil
}
func MediaPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\?#%") || strings.HasPrefix(value, "//") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path != value || path.Clean(value) != value {
		return false
	}
	if !strings.HasPrefix(value, "/images/") && !strings.HasPrefix(value, "/site-assets/catalog/") {
		return false
	}
	ext := strings.ToLower(path.Ext(value))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".svg"
}
func contentFields(title, slug string) map[string]string {
	f := map[string]string{}
	if strings.TrimSpace(title) == "" {
		f["title"] = "required"
	}
	if len(slug) > 96 || !slugPattern.MatchString(slug) {
		f["slug"] = "invalid"
	}
	return f
}
func validateLot(l LotContent, categories []Category, publish bool, media MediaResolver) error {
	f := contentFields(l.Title, l.Slug)
	if l.Currency != "USD" {
		f["currency"] = "usd_only"
	}
	if l.CategoryID != nil {
		found := false
		for _, c := range categories {
			if c.ID == *l.CategoryID {
				found = true
			}
		}
		if !found {
			f["category_id"] = "invalid"
		}
	}
	money := func(key string, n *int64, positive bool) {
		if n != nil && (*n < 0 || *n > MaxMoney || positive && *n == 0) {
			f[key] = "money"
		}
	}
	money("starting_price_minor", l.StartingPriceMinor, false)
	money("bid_increment_minor", l.BidIncrementMinor, true)
	money("fulfillment.shipping_fee_minor", l.Fulfillment.ShippingFeeMinor, false)
	for i, image := range l.Images {
		prefix := fmt.Sprintf("images.%d", i)
		if !MediaPath(image.Path) {
			f[prefix+".path"] = "media_path"
		} else if publish && (media == nil || !media.Readable(image.Path)) {
			f[prefix+".path"] = "media_missing"
		}
		if publish && strings.TrimSpace(image.Alt) == "" {
			f[prefix+".alt"] = "required"
		}
	}
	for i, spec := range l.Specifications {
		if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Value) == "" {
			f[fmt.Sprintf("specifications.%d", i)] = "required"
		}
	}
	if publish {
		if l.CategoryID == nil {
			f["category_id"] = "required"
		}
		if strings.TrimSpace(l.Description) == "" {
			f["description"] = "required"
		}
		if strings.TrimSpace(l.Condition) == "" {
			f["condition"] = "required"
		}
		if len(l.Images) == 0 {
			f["images"] = "required"
		}
		if l.StartingPriceMinor == nil {
			f["starting_price_minor"] = "required"
		}
		if l.BidIncrementMinor == nil {
			f["bid_increment_minor"] = "required"
		}
		fulfillment := l.Fulfillment
		if !fulfillment.ShippingEnabled && !fulfillment.PickupEnabled {
			f["fulfillment"] = "required"
		}
		if fulfillment.ShippingEnabled {
			if fulfillment.ShippingFeeMinor == nil {
				f["fulfillment.shipping_fee_minor"] = "required"
			}
			if strings.TrimSpace(fulfillment.ShippingInstructions) == "" {
				f["fulfillment.shipping_instructions"] = "required"
			}
		}
		if fulfillment.PickupEnabled && strings.TrimSpace(fulfillment.PickupInstructions) == "" {
			f["fulfillment.pickup_instructions"] = "required"
		}
	}
	return invalid(f)
}
func Schedule(e Event) ([]Slot, error) {
	f := map[string]string{}
	for key, p := range map[string]*int64{"opens_at": e.OpensAt, "first_closes_at": e.FirstClosesAt, "close_interval_ms": e.CloseIntervalMS} {
		if p != nil && (*p < 0 || *p > MaxInstant || key == "close_interval_ms" && *p == 0) {
			f[key] = "schedule"
		}
	}
	if e.OpensAt != nil && e.FirstClosesAt != nil && *e.FirstClosesAt <= *e.OpensAt {
		f["first_closes_at"] = "schedule"
	}
	if len(f) > 0 {
		return nil, invalid(f)
	}
	if e.OpensAt == nil || e.FirstClosesAt == nil || e.CloseIntervalMS == nil {
		return []Slot{}, nil
	}
	result := make([]Slot, 0, len(e.LotIDs))
	for i, id := range e.LotIDs {
		if int64(i) > (MaxInstant-*e.FirstClosesAt) / *e.CloseIntervalMS {
			return nil, invalid(map[string]string{"close_interval_ms": "schedule_overflow"})
		}
		result = append(result, Slot{id, i, *e.OpensAt, *e.FirstClosesAt + int64(i)**e.CloseIntervalMS})
	}
	return result, nil
}
func validateEvent(e Event, publish bool, now int64, media MediaResolver) error {
	f := contentFields(e.Title, e.Slug)
	if e.CoverPath != "" && !MediaPath(e.CoverPath) {
		f["cover_path"] = "media_path"
	}
	if _, err := Schedule(e); err != nil {
		for k, v := range err.(*Error).Fields {
			f[k] = v
		}
	}
	if publish {
		for k, v := range map[string]string{"theme": e.Theme, "description": e.Description, "cover_path": e.CoverPath, "cover_alt": e.CoverAlt} {
			if strings.TrimSpace(v) == "" {
				f[k] = "required"
			}
		}
		if e.CoverPath != "" && (media == nil || !media.Readable(e.CoverPath)) {
			f["cover_path"] = "media_missing"
		}
		if len(e.LotIDs) == 0 {
			f["lot_ids"] = "required"
		}
		for k, p := range map[string]*int64{"opens_at": e.OpensAt, "first_closes_at": e.FirstClosesAt, "close_interval_ms": e.CloseIntervalMS} {
			if p == nil {
				f[k] = "required"
			}
		}
		if e.OpensAt != nil && *e.OpensAt <= now {
			f["opens_at"] = "future"
		}
	}
	return invalid(f)
}
