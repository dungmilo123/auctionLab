package game

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxItemNameRunes is D-66's name cap. It counts RUNES, never bytes: 80
// Vietnamese characters are 160 bytes, so a byte check would reject a name the
// presenter's own keyboard counted as 80 characters (the same rule D-28 set for
// nicknames).
const MaxItemNameRunes = 80

// MaxAmount is H15's bound on every admin-supplied amount. JSON numbers are
// IEEE doubles in the browser, so anything above 2^53 silently loses precision;
// 1e12 keeps every value exactly representable and the settle arithmetic far
// from overflow.
const MaxAmount = int64(1_000_000_000_000)

// auctionTypes is PLAN.md §5's four item types (D-66). Dutch/sealed/vickrey
// fields are validated now even though their flows land in Phases 5–6.
var auctionTypes = []string{"english", "dutch", "sealed_first", "vickrey"}

// antiSnipeModes is the ENGL-04 switch (PLAN.md §5: disabled | extend).
var antiSnipeModes = []string{"disabled", "extend"}

// ItemInput is the `admin.item.upsert` payload (D-66): the full item object, so
// the admin editor round-trips exactly the row it was shown. An empty ID means
// "create with a server-minted id" (H14); a non-empty ID must name an existing
// item or the command is refused.
//
// Every nullable numeric is a pointer so "absent" and a legitimate 0 stay
// distinguishable: `max_extensions: null` means no cap (D-59).
type ItemInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`

	AuctionType string `json:"auction_type"`
	TruthValue  int64  `json:"truth_value"`

	HintMin    *int64 `json:"hint_min"`
	HintMax    *int64 `json:"hint_max"`
	StartPrice int64  `json:"start_price"`

	MinIncrement        *int64 `json:"min_increment"`
	FloorPrice          *int64 `json:"floor_price"`
	DecrementStep       *int64 `json:"decrement_step"`
	DecrementIntervalMS *int64 `json:"decrement_interval_ms"`
	DurationMS          *int64 `json:"duration_ms"`

	AntiSnipeMode     string `json:"anti_snipe_mode"`
	AntiSnipeWindowMS *int64 `json:"anti_snipe_window_ms"`
	MaxExtensions     *int64 `json:"max_extensions"`
}

// validateItemInput runs D-66's rules in order and returns ONE error naming the
// field at fault, so the presenter can act on it (Pitfall 12: the client mirrors
// these rules for copy, the server stays the authority).
func validateItemInput(in ItemInput) error {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxItemNameRunes {
		return fmt.Errorf("game: item input name must be 1..%d characters after trimming", MaxItemNameRunes)
	}
	if !slices.Contains(auctionTypes, in.AuctionType) {
		return fmt.Errorf("game: item input auction_type %q is not one of english|dutch|sealed_first|vickrey", in.AuctionType)
	}
	if in.TruthValue < 0 || in.TruthValue > MaxAmount {
		return fmt.Errorf("game: item input truth_value %d must be within [0, %d]", in.TruthValue, MaxAmount)
	}
	if in.StartPrice < 0 || in.StartPrice > MaxAmount {
		return fmt.Errorf("game: item input start_price %d must be within [0, %d]", in.StartPrice, MaxAmount)
	}
	if err := boundedAmount("hint_min", in.HintMin); err != nil {
		return err
	}
	if err := boundedAmount("hint_max", in.HintMax); err != nil {
		return err
	}
	if in.HintMin != nil && in.HintMax != nil && *in.HintMin > *in.HintMax {
		return fmt.Errorf("game: item input hint_min %d must not exceed hint_max %d", *in.HintMin, *in.HintMax)
	}

	// The shared optional mechanics, bounded wherever present. The order is
	// fixed so a doubly-invalid input always reports the same field first.
	for _, field := range []struct {
		name  string
		value *int64
	}{
		{"min_increment", in.MinIncrement},
		{"floor_price", in.FloorPrice},
		{"decrement_step", in.DecrementStep},
		{"decrement_interval_ms", in.DecrementIntervalMS},
		{"duration_ms", in.DurationMS},
		{"anti_snipe_window_ms", in.AntiSnipeWindowMS},
		{"max_extensions", in.MaxExtensions},
	} {
		if err := boundedAmount(field.name, field.value); err != nil {
			return err
		}
	}

	switch in.AuctionType {
	case "english":
		// ENGL-01/ENGL-04's contract: a live English item needs a minimum raise,
		// a duration and a coherent anti-snipe configuration.
		if in.MinIncrement == nil || *in.MinIncrement < 1 {
			return fmt.Errorf("game: item input min_increment must be a positive integer for an english item")
		}
		if in.DurationMS == nil || *in.DurationMS < 1 {
			return fmt.Errorf("game: item input duration_ms must be a positive integer for an english item")
		}
		if !slices.Contains(antiSnipeModes, in.AntiSnipeMode) {
			return fmt.Errorf("game: item input anti_snipe_mode %q is not one of disabled|extend", in.AntiSnipeMode)
		}
		if in.AntiSnipeMode == "extend" && (in.AntiSnipeWindowMS == nil || *in.AntiSnipeWindowMS < 1) {
			return fmt.Errorf("game: item input anti_snipe_window_ms must be a positive integer when anti_snipe_mode is extend")
		}
		if in.MaxExtensions != nil && *in.MaxExtensions < 1 {
			return fmt.Errorf("game: item input max_extensions %d must be at least 1 or null for no cap", *in.MaxExtensions)
		}
	case "dutch":
		// Phase 5 owns the flow. Every present field must make sense, and the
		// floor pair is the type's one cross-field rule (D-92): an ABSENT floor
		// is still legal — a Dutch row may be configured before its mechanics
		// are filled in — but a floor above the start price could never be
		// reached honestly, so it is refused at save time (where the presenter
		// can act on it) rather than at go-live time.
		if in.DecrementStep != nil && *in.DecrementStep < 1 {
			return fmt.Errorf("game: item input decrement_step must be a positive integer when present")
		}
		if in.DecrementIntervalMS != nil && *in.DecrementIntervalMS < 1 {
			return fmt.Errorf("game: item input decrement_interval_ms must be a positive integer when present")
		}
		if in.FloorPrice != nil && *in.FloorPrice > in.StartPrice {
			return fmt.Errorf("game: item input floor_price %d must not exceed start_price %d", *in.FloorPrice, in.StartPrice)
		}
	}
	return nil
}

// boundedAmount validates one nullable amount: nil is "absent", otherwise the
// value must lie inside [0, MaxAmount].
func boundedAmount(field string, v *int64) error {
	if v == nil {
		return nil
	}
	if *v < 0 || *v > MaxAmount {
		return fmt.Errorf("game: item input %s %d must be within [0, %d]", field, *v, MaxAmount)
	}
	return nil
}

// itemFromInput projects a validated input into the in-memory row. The nullable
// pointers normalize to 0 ("absent"), the same convention the store's row
// mapping inverts on the way out.
func itemFromInput(id string, in ItemInput, orderIndex int64, status string) *item {
	deref := func(v *int64) int64 {
		if v == nil {
			return 0
		}
		return *v
	}
	return &item{
		id: id, orderIndex: orderIndex, name: strings.TrimSpace(in.Name),
		description: in.Description, imageURL: in.ImageURL,
		auctionType: in.AuctionType, truthValue: in.TruthValue,
		hintMin: deref(in.HintMin), hintMax: deref(in.HintMax), startPrice: in.StartPrice,
		minIncrement: deref(in.MinIncrement), floorPrice: deref(in.FloorPrice),
		decrementStep: deref(in.DecrementStep), decrementIntervalMS: deref(in.DecrementIntervalMS),
		durationMS: deref(in.DurationMS), antiSnipeMode: in.AntiSnipeMode,
		antiSnipeWindowMS: deref(in.AntiSnipeWindowMS), maxExtensions: deref(in.MaxExtensions),
		status: status,
	}
}

// editableStatus is D-66's history rule: only a row that has not played yet may
// be rewritten or removed. A live or settled item is the record of what the room
// saw, and a skipped one has already had its turn.
func editableStatus(status string) bool { return status == "pending" || status == "intro" }

// nextOrderIndex appends after the current maximum, so a new item always lands
// at the end of the presentation order.
func nextOrderIndex(s *state) int64 {
	max := int64(-1)
	for _, id := range s.itemOrder {
		if it := s.items[id]; it != nil && it.orderIndex > max {
			max = it.orderIndex
		}
	}
	return max + 1
}

// nextPendingItemID is D-65's selection: the first item in presentation order
// that has never played. A skipped or settled item is never replayed.
func nextPendingItemID(s *state) string {
	for _, id := range s.itemOrder {
		if it := s.items[id]; it != nil && it.status == "pending" {
			return id
		}
	}
	return ""
}

// phaseChanged is the D-57 `phase.changed` payload: the new phase plus the item
// it applies to (null when none is current, e.g. FINALE), so the frame alone is
// enough for a client to patch its view.
//
// now is the mutation's own clock reading: the LIVE edge is packed with the run
// that just started, and a Dutch run's price is DERIVED at the instant of each
// frame (D-94), so the phase change has to describe the same moment as its
// sibling `item.live` frame.
func (s *state) phaseChanged(phase Phase, now int64) PhaseChanged {
	return PhaseChanged{Phase: string(phase), Item: s.currentPublicItem(now)}
}

// ItemUpsert creates (empty ID) or rewrites (known ID) one item (D-66). Only a
// pending or intro item may be rewritten: a live or settled row is history.
func (e *Engine) ItemUpsert(in ItemInput) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := validateItemInput(in); err != nil {
		return err
	}

	if in.ID == "" {
		id, err := newItemID()
		if err != nil {
			return err
		}
		return e.applyLocked(func(s *state) error {
			s.items[id] = itemFromInput(id, in, nextOrderIndex(s), "pending")
			s.itemOrder = append(s.itemOrder, id)
			// D-66: item CRUD ends with the authoritative snapshot — the item
			// list is an admin projection, and every role's view derives from
			// the same rows.
			s.events = append(s.events, Event{Type: EventSnapshot})
			return nil
		}, true, nil)
	}

	existing := e.state.items[in.ID]
	if existing == nil {
		// H14: an unknown id is refused rather than created — a client-chosen
		// primary key must not be able to invent rows.
		return fmt.Errorf("game: item.upsert %q does not exist; send an empty id to create", in.ID)
	}
	if !editableStatus(existing.status) {
		return fmt.Errorf("game: item %q is %s; only a pending or intro item can be edited", in.ID, existing.status)
	}

	return e.applyLocked(func(s *state) error {
		previous := s.items[in.ID]
		s.items[in.ID] = itemFromInput(in.ID, in, previous.orderIndex, previous.status)
		s.events = append(s.events, Event{Type: EventSnapshot})
		return nil
	}, true, nil)
}

// DeleteItem removes one pending or intro item. The current item cannot be
// deleted — the presenter skips it first, so the phase invariants (ITEM_INTRO
// always has a current item) stay intact for the next boot.
func (e *Engine) DeleteItem(itemID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	it := e.state.items[itemID]
	if it == nil {
		return fmt.Errorf("game: item.delete %q does not exist", itemID)
	}
	if !editableStatus(it.status) {
		return fmt.Errorf("game: item %q is %s; only a pending or intro item can be deleted", itemID, it.status)
	}
	if e.state.currentItemID == itemID {
		return fmt.Errorf("game: item %q is the current item; skip it before deleting", itemID)
	}

	return e.applyLocked(func(s *state) error {
		delete(s.items, itemID)
		s.itemOrder = slices.DeleteFunc(s.itemOrder, func(id string) bool { return id == itemID })
		s.events = append(s.events, Event{Type: EventSnapshot})
		return nil
	}, true, nil)
}

// ReorderItems applies a complete unique permutation of the existing item ids
// and rewrites order_index accordingly. Anything else — a duplicate, a missing
// id, an unknown one, a different length — is refused with NO partial write
// (D-66): the whole check runs against the committed state before the mutation.
func (e *Engine) ReorderItems(ids []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(ids) != len(e.state.items) {
		return fmt.Errorf("game: item.reorder must name all %d items", len(e.state.items))
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if _, ok := e.state.items[id]; !ok {
			return fmt.Errorf("game: item.reorder names unknown item %q", id)
		}
		if seen[id] {
			return fmt.Errorf("game: item.reorder names item %q twice", id)
		}
		seen[id] = true
	}

	return e.applyLocked(func(s *state) error {
		for i, id := range ids {
			s.items[id].orderIndex = int64(i)
		}
		s.itemOrder = append([]string(nil), ids...)
		s.events = append(s.events, Event{Type: EventSnapshot})
		return nil
	}, true, nil)
}

// StartItem is `admin.item.start {id}` (D-65):
//
//   - from LOBBY, SETTLED or FINALE it opens the item's intro (the item must
//     still be pending — a settled row must never replay);
//   - from ITEM_INTRO with the SAME id it goes LIVE, starting the clock and the
//     runtime row that anti-snipe and crash recovery depend on;
//   - anything else is refused through CanTransition with nothing changed.
func (e *Engine) StartItem(itemID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	it := e.state.items[itemID]
	if it == nil {
		return fmt.Errorf("game: item.start names unknown item %q", itemID)
	}
	now := e.now().UnixMilli()

	if e.state.phase == PhaseItemIntro {
		if itemID != e.state.currentItemID {
			// Accepting a stale id here would resurrect an item behind the
			// presenter's back (Pitfall 4).
			return fmt.Errorf("game: item.start %q while item %q is in intro", itemID, e.state.currentItemID)
		}
		// D-92: the LIVE edge is guarded BY TYPE. An english run needs its
		// wall-clock deadline, because the ticker's settle arm and the boot's
		// resume both require one; a dutch run needs its own mechanics and
		// deliberately carries NO deadline (D-93 — the column stays NULL and
		// nothing may settle Dutch on the clock); the sealed types are Phase 6's
		// and cannot go live yet. A row that fails its type's guard stays in
		// intro and the presenter skips it with `admin.item.next`, which is the
		// documented way out.
		switch it.auctionType {
		case "english":
			if it.durationMS <= 0 {
				return fmt.Errorf("game: item %q has duration_ms %d; a live run always needs a wall-clock deadline", itemID, it.durationMS)
			}
		case "dutch":
			if it.floorPrice <= 0 || it.floorPrice > it.startPrice {
				return fmt.Errorf("game: item %q has floor_price %d; a Dutch run needs 0 < floor_price <= start_price (%d)", itemID, it.floorPrice, it.startPrice)
			}
			if it.decrementStep < 1 {
				return fmt.Errorf("game: item %q has decrement_step %d; a Dutch run needs a positive decrement step", itemID, it.decrementStep)
			}
			if it.decrementIntervalMS < 1 {
				return fmt.Errorf("game: item %q has decrement_interval_ms %d; a Dutch run needs a positive decrement interval", itemID, it.decrementIntervalMS)
			}
		default:
			return fmt.Errorf("game: item %q is %s; that type cannot go live yet", itemID, it.auctionType)
		}
		if !CanTransition(PhaseItemIntro, PhaseLive) {
			return fmt.Errorf("game: the machine does not declare ITEM_INTRO → LIVE")
		}
		return e.applyLocked(func(s *state) error {
			live := s.items[itemID]
			live.status = "live"
			run := &auctionRun{itemID: itemID, startedAt: now}
			if live.durationMS > 0 {
				// The deadline is what an English run settles on. A Dutch run has
				// no duration_ms at all, so this stays 0 and the column stays
				// NULL (D-93) — the price is re-derived from started_at instead.
				run.deadlineAt = now + live.durationMS
			}
			s.currentAuction = run
			s.phase = PhaseLive
			s.events = append(s.events,
				Event{Type: EventPhaseChanged, Data: s.phaseChanged(PhaseLive, now)},
				Event{Type: EventItemLive, Data: s.currentPublicItem(now)},
				Event{Type: EventSnapshot},
			)
			return nil
		}, true, nil)
	}

	if it.status != "pending" {
		return fmt.Errorf("game: item %q is %s; only a pending item can be started", itemID, it.status)
	}
	if !CanTransition(e.state.phase, PhaseItemIntro) {
		return fmt.Errorf("game: the machine does not declare %s → ITEM_INTRO", e.state.phase)
	}
	return e.applyLocked(func(s *state) error {
		s.items[itemID].status = "intro"
		s.currentItemID = itemID
		s.currentAuction = nil
		s.phase = PhaseItemIntro
		s.events = append(s.events,
			Event{Type: EventPhaseChanged, Data: s.phaseChanged(PhaseItemIntro, now)},
			Event{Type: EventItemIntro, Data: s.currentPublicItem(now)},
			Event{Type: EventSnapshot},
		)
		return nil
	}, true, nil)
}

// NextItem is `admin.item.next {}` (D-65, which also IS ADMN-02's item.skip):
//
//   - in ITEM_INTRO the item on screen ends as `skipped` and the next pending
//     item opens its intro; with none left the game enters FINALE;
//   - in SETTLED the next pending item opens its intro; with none left, FINALE;
//   - FINALE clears current_item_id (H10: the "current item" concept is over) and
//     publishes the D-89 `finale` payload, so the ceremony is event-driven while
//     the snapshot stays the fallback a resync renders from;
//   - any other phase is refused with nothing changed.
func (e *Engine) NextItem() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch e.state.phase {
	case PhaseItemIntro, PhaseSettled:
	default:
		return fmt.Errorf("game: item.next is not a command for phase %s", e.state.phase)
	}
	skipping := e.state.phase == PhaseItemIntro

	return e.applyLocked(func(s *state) error {
		if skipping {
			if current := s.items[s.currentItemID]; current != nil {
				current.status = "skipped"
			}
		}

		nextID := nextPendingItemID(s)
		if nextID == "" {
			if !CanTransition(s.phase, PhaseFinale) {
				return fmt.Errorf("game: the machine does not declare %s → FINALE", s.phase)
			}
			s.currentItemID = ""
			s.currentAuction = nil
			s.phase = PhaseFinale
			s.events = append(s.events,
				Event{Type: EventPhaseChanged, Data: s.phaseChanged(PhaseFinale, e.now().UnixMilli())},
				// D-89: the reserved `finale` frame lands between the phase change
				// and the snapshot — the snapshot stays LAST because it is what a
				// client that missed everything else resyncs from (D-41), and the
				// finale is the event that carries the ceremony itself.
				Event{Type: EventFinale, Data: s.finaleReveal()},
				Event{Type: EventSnapshot},
			)
			return nil
		}

		if !CanTransition(s.phase, PhaseItemIntro) {
			return fmt.Errorf("game: the machine does not declare %s → ITEM_INTRO", s.phase)
		}
		s.items[nextID].status = "intro"
		s.currentItemID = nextID
		s.currentAuction = nil // a new item has no run until it goes LIVE
		s.phase = PhaseItemIntro
		s.events = append(s.events,
			Event{Type: EventPhaseChanged, Data: s.phaseChanged(PhaseItemIntro, e.now().UnixMilli())},
			Event{Type: EventItemIntro, Data: s.currentPublicItem(e.now().UnixMilli())},
			Event{Type: EventSnapshot},
		)
		return nil
	}, true, nil)
}
