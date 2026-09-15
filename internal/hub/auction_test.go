package hub

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This file owns the Phase 3 command surface at the transport boundary: the
// socket routes `bid.place` and `admin.item.*` into the engine, and the engine's
// committed frames come back through the same lock-held FIFO as every Phase 2
// mutation (D-52…D-58, D-64…D-66).
//
// The cases are written as a script of exact frame sequences rather than a
// "wait for something" poll: the ORDER of one mutation's siblings and the
// revision they share are both part of the contract (D-21/D-57), and a test that
// skipped a stray frame could not see a reordered fan-out.

// englishItemJSON is D-66's create payload for one playable English item.
const englishItemJSON = `{"name":"Phase 3 lot","auction_type":"english","truth_value":350000,` +
	`"start_price":100000,"min_increment":10000,"duration_ms":60000,"anti_snipe_mode":"disabled"}`

// joinPlayer joins one player, consumes the join's own fan-out frame (the
// mutation's snapshot, H2/D-33) and returns the connection plus the handshake
// snapshot — which is the self-sufficient frame a late joiner renders from.
func joinPlayer(t *testing.T, h *Hub, nickname string) (*websocket.Conn, frame) {
	t.Helper()

	conn := dialWS(t, h)
	_, snapshot := join(t, conn, fmt.Sprintf(`{"type":"join","role":"player","nickname":%q}`, nickname))
	expectFrames(t, conn, TypeStateSnapshot)
	return conn, snapshot
}

// joinRole joins one identity-less role (screen or admin). Such a join is not a
// mutation, so it fans nothing out to anybody — not even to itself.
func joinRole(t *testing.T, h *Hub, role string) *websocket.Conn {
	t.Helper()

	conn := dialWS(t, h)
	join(t, conn, fmt.Sprintf(`{"type":"join","role":%q}`, role))
	return conn
}

// expectAll reads the same frame sequence from each connection, in order.
func expectAll(t *testing.T, conns []*websocket.Conn, types ...string) [][]frame {
	t.Helper()

	all := make([][]frame, 0, len(conns))
	for _, conn := range conns {
		all = append(all, expectFrames(t, conn, types...))
	}
	return all
}

// createItem drives D-66's create over the socket and returns the id the admin
// projection reported.
func createItem(t *testing.T, admin *websocket.Conn) string {
	t.Helper()

	writeFrame(t, admin, `{"type":"admin.item.upsert","item":`+englishItemJSON+`}`)
	env := readType(t, admin, TypeStateSnapshot)

	var view struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &view); err != nil {
		t.Fatalf("the admin snapshot is not an admin view: %v (%s)", err, env.Data)
	}
	if len(view.Items) != 1 {
		t.Fatalf("admin items = %+v, want exactly the item just created", view.Items)
	}
	return view.Items[0].ID
}

// startItem sends one `admin.item.start {id}` (D-65).
func startItem(t *testing.T, admin *websocket.Conn, id string) {
	t.Helper()

	writeFrame(t, admin, fmt.Sprintf(`{"type":"admin.item.start","id":%q}`, id))
}

// placeBid sends one `bid.place {amount}` (D-52).
func placeBid(t *testing.T, conn *websocket.Conn, amount int64) {
	t.Helper()

	writeFrame(t, conn, fmt.Sprintf(`{"type":"bid.place","amount":%d}`, amount))
}

// dataObject decodes one nested object of a frame's data.
func dataObject(t *testing.T, env frame, key string) map[string]json.RawMessage {
	t.Helper()

	var object map[string]json.RawMessage
	if err := json.Unmarshal(dataField(t, env, key), &object); err != nil {
		t.Fatalf("data.%s is not a JSON object: %v", key, err)
	}
	return object
}

// hasKey reports whether a decoded object carries a key at all — the pointer +
// omitempty contract needs an ABSENCE assertion, not a zero-value one (D-67).
func hasKey(object map[string]json.RawMessage, key string) bool {
	_, ok := object[key]
	return ok
}

// objectInt reads one integer out of a decoded object.
func objectInt(t *testing.T, object map[string]json.RawMessage, key string) int64 {
	t.Helper()

	raw, ok := object[key]
	if !ok {
		t.Fatalf("object has no %q key: %v", key, object)
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("object.%s is not an integer: %v", key, err)
	}
	return value
}

// TestLiveProjectionCarriesEveryLiveField pins D-67's additive live fields on
// the frames a real socket receives — for a player who was there and for a late
// joiner whose only source is the snapshot (SESS-04/D-63).
func TestLiveProjectionCarriesEveryLiveField(t *testing.T) {
	h, _ := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

	admin := joinRole(t, h, RoleAdmin)
	screen := joinRole(t, h, RoleScreen)
	player, _ := joinPlayer(t, h, "An")
	expectAll(t, []*websocket.Conn{admin, screen}, TypeStateSnapshot) // An's join fan-out

	id := createItem(t, admin)
	expectAll(t, []*websocket.Conn{admin, screen, player}, TypeStateSnapshot)

	// start → ITEM_INTRO: the phase frame and the intro item, one revision.
	startItem(t, admin, id)
	intro := expectFrames(t, player, TypePhaseChanged, TypeItemIntro, TypeStateSnapshot)
	if got := frameString(t, frameData(t, intro[0]), "phase"); got != "ITEM_INTRO" {
		t.Errorf("phase.changed phase = %q, want ITEM_INTRO", got)
	}
	introItem := frameData(t, intro[1])
	if got := frameString(t, introItem, "status"); got != "intro" {
		t.Errorf("item.intro status = %q, want intro", got)
	}
	if hasKey(introItem, "current_price") || hasKey(introItem, "settle") {
		t.Error("an intro item carries live or settle fields: the item is not running yet (D-67)")
	}
	expectAll(t, []*websocket.Conn{admin, screen}, TypePhaseChanged, TypeItemIntro, TypeStateSnapshot)

	// start again → LIVE: phase.changed + item.live + snapshot + the D-56 tick.
	startItem(t, admin, id)
	live := expectFrames(t, player, TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)
	expectAll(t, []*websocket.Conn{admin, screen}, TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)

	if got := frameString(t, frameData(t, live[0]), "phase"); got != "LIVE" {
		t.Errorf("phase.changed phase = %q, want LIVE", got)
	}
	item := frameData(t, live[1])
	if got := frameString(t, item, "status"); got != "live" {
		t.Errorf("item.live status = %q, want live", got)
	}
	if got := objectInt(t, item, "current_price"); got != 100_000 {
		t.Errorf("item.live current_price = %d, want the start price 100000", got)
	}
	if got := objectInt(t, item, "min_next_bid"); got != 100_000 {
		t.Errorf("item.live min_next_bid = %d, want the start price before any bid", got)
	}
	if !hasKey(item, "deadline_at") {
		t.Error("item.live carries no deadline_at: the countdown cannot anchor (TIME-02)")
	}
	if got := objectInt(t, item, "extensions_used"); got != 0 {
		t.Errorf("item.live extensions_used = %d, want 0 before any extend", got)
	}
	if hasKey(item, "top_bidder") {
		t.Error("item.live carries top_bidder before any bid was accepted")
	}
	if hasKey(item, "settle") {
		t.Error("item.live carries the settle object before SETTLED (D-67's leak boundary)")
	}

	tick := frameData(t, live[3])
	if got := objectInt(t, tick, "current_price"); got != 100_000 {
		t.Errorf("price.tick current_price = %d, want 100000", got)
	}
	if got := objectInt(t, tick, "remaining_ms"); got <= 0 || got > 60_000 {
		t.Errorf("price.tick remaining_ms = %d, want it inside (0, 60000]", got)
	}

	// A late joiner at LIVE renders from its own snapshot alone.
	_, late := joinPlayer(t, h, "Binh")
	if got := frameString(t, frameData(t, late), "phase"); got != "LIVE" {
		t.Fatalf("late-join snapshot phase = %q, want LIVE", got)
	}
	lateItem := dataObject(t, late, "item")
	if got := objectInt(t, lateItem, "current_price"); got != 100_000 {
		t.Errorf("late-join current_price = %d, want 100000 (SESS-04)", got)
	}
	if got := objectInt(t, lateItem, "min_next_bid"); got != 100_000 {
		t.Errorf("late-join min_next_bid = %d, want 100000", got)
	}
	if !hasKey(lateItem, "deadline_at") {
		t.Error("late-join item carries no deadline_at: a reload mid-LIVE must render")
	}
}

// TestBidRejectionIsPrivateAndDoesNotBumpRev pins D-53 on real sockets: the
// refusal reaches the bidder alone with its structured reason and the numbers
// that make it actionable, and the room's revision does not move.
func TestBidRejectionIsPrivateAndDoesNotBumpRev(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

	admin := joinRole(t, h, RoleAdmin)
	screen := joinRole(t, h, RoleScreen)
	an, _ := joinPlayer(t, h, "An")
	expectAll(t, []*websocket.Conn{admin, screen}, TypeStateSnapshot) // An joined
	binh, _ := joinPlayer(t, h, "Binh")
	expectAll(t, []*websocket.Conn{admin, screen, an}, TypeStateSnapshot) // Binh joined

	id := createItem(t, admin)
	expectAll(t, []*websocket.Conn{admin, screen, an, binh}, TypeStateSnapshot)

	startItem(t, admin, id)
	expectAll(t, []*websocket.Conn{admin, screen, an, binh}, TypePhaseChanged, TypeItemIntro, TypeStateSnapshot)

	startItem(t, admin, id)
	expectAll(t, []*websocket.Conn{admin, screen, an, binh}, TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)

	// An bids the minimum: accepted everywhere, one revision, one immediate tick.
	placeBid(t, an, 100_000)
	accepted := expectFrames(t, an, TypeBidAccepted, TypePriceTick)
	expectAll(t, []*websocket.Conn{admin, screen, binh}, TypeBidAccepted, TypePriceTick)

	if got := frameString(t, frameData(t, accepted[0]), "nickname"); got != "An" {
		t.Errorf("bid.accepted nickname = %q, want An", got)
	}
	if got := objectInt(t, frameData(t, accepted[0]), "current_price"); got != 100_000 {
		t.Errorf("bid.accepted current_price = %d, want the accepted amount", got)
	}
	if got := objectInt(t, frameData(t, accepted[0]), "min_next_bid"); got != 110_000 {
		t.Errorf("bid.accepted min_next_bid = %d, want 110000 (current + increment)", got)
	}

	revAfterBid := engine.Rev()

	// Binh bids the price that was just outbid: below the new minimum.
	placeBid(t, binh, 100_000)
	rejected := readType(t, binh, TypeBidRejected)

	var payload struct {
		Reason     string `json:"reason"`
		MinNextBid int64  `json:"min_next_bid"`
		Budget     int64  `json:"budget"`
		Amount     int64  `json:"amount"`
	}
	if err := json.Unmarshal(rejected.Data, &payload); err != nil {
		t.Fatalf("bid.rejected payload is not a rejection: %v (%s)", err, rejected.Data)
	}
	if payload.Reason != "below_minimum" {
		t.Errorf("bid.rejected reason = %q, want below_minimum", payload.Reason)
	}
	if payload.MinNextBid != 110_000 {
		t.Errorf("bid.rejected min_next_bid = %d, want 110000", payload.MinNextBid)
	}
	if payload.Budget != 1_000_000 {
		t.Errorf("bid.rejected budget = %d, want the starting budget", payload.Budget)
	}
	if payload.Amount != 100_000 {
		t.Errorf("bid.rejected amount = %d, want the refused amount", payload.Amount)
	}
	if got := engine.Rev(); got != revAfterBid {
		t.Errorf("revision after a refused bid = %d, want the unchanged %d (D-53)", got, revAfterBid)
	}

	// Nobody else received anything: the refusal is private, and because it is
	// not a state event the room's stream carries no frame for it at all.
	for name, conn := range map[string]*websocket.Conn{"An": an, "screen": screen, "admin": admin} {
		if frames := drainFrames(t, conn, 250*time.Millisecond); len(frames) != 0 {
			t.Errorf("%s received %d frame(s) after a private rejection: %s", name, len(frames), frameSummary(frames))
		}
	}
}

// TestSettleRevealReachesPlayerAndScreen pins D-55/D-67/D-68 end to end: the
// force-end is one mutation whose three frames carry the reveal, the standings
// and the authoritative snapshot, and the winner's budget moves exactly once.
func TestSettleRevealReachesPlayerAndScreen(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

	admin := joinRole(t, h, RoleAdmin)
	screen := joinRole(t, h, RoleScreen)
	an, _ := joinPlayer(t, h, "An")
	expectAll(t, []*websocket.Conn{admin, screen}, TypeStateSnapshot)

	id := createItem(t, admin)
	expectAll(t, []*websocket.Conn{admin, screen, an}, TypeStateSnapshot)

	startItem(t, admin, id)
	expectAll(t, []*websocket.Conn{admin, screen, an}, TypePhaseChanged, TypeItemIntro, TypeStateSnapshot)

	startItem(t, admin, id)
	expectAll(t, []*websocket.Conn{admin, screen, an}, TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)

	placeBid(t, an, 100_000)
	expectAll(t, []*websocket.Conn{admin, screen, an}, TypeBidAccepted, TypePriceTick)

	// Force end: LIVE → SETTLED through the one settle path (D-55b/D-65).
	writeFrame(t, admin, `{"type":"admin.item.force_end"}`)
	settled := expectAll(t, []*websocket.Conn{admin, screen, an}, TypeItemSettled, TypeLeaderboard, TypeStateSnapshot)

	var reveal struct {
		Status string `json:"status"`
		Settle *struct {
			Outcome string `json:"outcome"`
			Winner  *struct {
				Nickname string `json:"nickname"`
			} `json:"winner"`
			Price       int64 `json:"price"`
			TruthValue  int64 `json:"truth_value"`
			BudgetDelta int64 `json:"budget_delta"`
		} `json:"settle"`
	}
	if err := json.Unmarshal(settled[1][0].Data, &reveal); err != nil {
		t.Fatalf("item.settled payload is not a public item: %v", err)
	}
	if reveal.Status != "settled" || reveal.Settle == nil {
		t.Fatalf("item.settled = %s, want a settled item carrying its reveal", settled[1][0].Data)
	}
	if reveal.Settle.Outcome != "sold" {
		t.Errorf("settle outcome = %q, want sold", reveal.Settle.Outcome)
	}
	if reveal.Settle.Winner == nil || reveal.Settle.Winner.Nickname != "An" {
		t.Errorf("settle winner = %+v, want An", reveal.Settle.Winner)
	}
	if reveal.Settle.Price != 100_000 {
		t.Errorf("settle price = %d, want the accepted bid", reveal.Settle.Price)
	}
	if reveal.Settle.TruthValue != 350_000 {
		t.Errorf("settle truth_value = %d, want the item's truth value (the reveal moment)", reveal.Settle.TruthValue)
	}
	if reveal.Settle.BudgetDelta != 250_000 {
		t.Errorf("settle budget_delta = %d, want truth_value − price", reveal.Settle.BudgetDelta)
	}

	// The reveal is ONE object: the same numbers ride the SETTLED snapshot a
	// late joiner or a resync reads (D-63/D-68).
	if snapshotItem := dataObject(t, settled[1][2], "item"); !hasKey(snapshotItem, "settle") {
		t.Error("the SETTLED snapshot's item carries no settle object: a late joiner could not render the reveal")
	}

	// The winner's own SETTLED snapshot carries the post-settle budget: the one
	// place a client's money may move (Pitfall 13).
	me := dataObject(t, settled[2][2], "me")
	if got := objectInt(t, me, "budget"); got != 1_250_000 {
		t.Errorf("winner budget after the settle = %d, want starting − price + truth_value = 1250000", got)
	}

	if got := string(engine.Phase()); got != "SETTLED" {
		t.Errorf("phase = %s, want SETTLED", got)
	}
}

// TestPhaseChangedSharesRevision pins D-21/D-57 for the phase flow: the phase
// frame, the item frame and the snapshot of one mutation share one revision,
// and the next mutation is exactly one revision later.
func TestPhaseChangedSharesRevision(t *testing.T) {
	h, _ := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

	admin := joinRole(t, h, RoleAdmin)
	screen := joinRole(t, h, RoleScreen)

	id := createItem(t, admin)
	crud := expectAll(t, []*websocket.Conn{admin, screen}, TypeStateSnapshot)
	crudRev := crud[1][0].Rev

	startItem(t, admin, id)
	intro := expectFrames(t, screen, TypePhaseChanged, TypeItemIntro, TypeStateSnapshot)
	for i, env := range intro {
		if env.Rev != intro[0].Rev {
			t.Errorf("ITEM_INTRO sibling %d (%s) rev = %d, want the mutation's %d", i, env.Type, env.Rev, intro[0].Rev)
		}
	}
	if intro[0].Rev != crudRev+1 {
		t.Errorf("ITEM_INTRO rev = %d, want exactly one past the CRUD mutation's %d", intro[0].Rev, crudRev)
	}

	startItem(t, admin, id)
	live := expectFrames(t, screen, TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)
	for i, env := range live {
		if env.Rev != live[0].Rev {
			t.Errorf("LIVE sibling %d (%s) rev = %d, want the mutation's %d", i, env.Type, env.Rev, live[0].Rev)
		}
	}
	if live[0].Rev != intro[0].Rev+1 {
		t.Errorf("LIVE rev = %d, want exactly one past ITEM_INTRO's %d", live[0].Rev, intro[0].Rev)
	}
}

// TestLateJoinSnapshotIsSelfSufficientAtEveryPhase is SESS-04/D-63 at the
// socket: whoever joins at any moment receives a snapshot that renders that
// phase on its own, and a bid placed once the window is closed is refused
// instead of accepted (D-52 #3).
func TestLateJoinSnapshotIsSelfSufficientAtEveryPhase(t *testing.T) {
	h, _ := newTestHubWithPolicy(t, testPolicyWithBuffer(64))
	admin := joinRole(t, h, RoleAdmin)
	id := createItem(t, admin)

	// LOBBY: an item exists, but none is current.
	_, lobby := joinPlayer(t, h, "Lobby")
	if got := frameString(t, frameData(t, lobby), "phase"); got != "LOBBY" {
		t.Errorf("LOBBY snapshot phase = %q, want LOBBY", got)
	}
	if raw := dataField(t, lobby, "item"); string(raw) != "null" {
		t.Errorf("LOBBY snapshot item = %s, want an explicit null", raw)
	}

	// ITEM_INTRO: the public intro data, and no live timing yet.
	startItem(t, admin, id)
	_, intro := joinPlayer(t, h, "Intro")
	if got := frameString(t, frameData(t, intro), "phase"); got != "ITEM_INTRO" {
		t.Fatalf("ITEM_INTRO snapshot phase = %q, want ITEM_INTRO", got)
	}
	introItem := dataObject(t, intro, "item")
	if got := frameString(t, introItem, "status"); got != "intro" {
		t.Errorf("ITEM_INTRO item status = %q, want intro", got)
	}
	if got := frameString(t, introItem, "name"); got != "Phase 3 lot" {
		t.Errorf("ITEM_INTRO item name = %q, want the item the admin created", got)
	}
	if hasKey(introItem, "deadline_at") {
		t.Error("ITEM_INTRO item carries deadline_at: the clock has not started")
	}

	// LIVE: every field the bid form and the countdown need.
	startItem(t, admin, id)
	_, live := joinPlayer(t, h, "Live")
	if got := frameString(t, frameData(t, live), "phase"); got != "LIVE" {
		t.Fatalf("LIVE snapshot phase = %q, want LIVE", got)
	}
	liveItem := dataObject(t, live, "item")
	for _, key := range []string{"current_price", "min_next_bid", "deadline_at"} {
		if !hasKey(liveItem, key) {
			t.Errorf("LIVE snapshot item lacks %q: a late joiner cannot bid", key)
		}
	}

	// SETTLED with no bids: unsold is a first-class outcome, and the window is
	// closed for anyone who arrives now.
	writeFrame(t, admin, `{"type":"admin.item.force_end"}`)
	settledConn, settled := joinPlayer(t, h, "Settled")
	if got := frameString(t, frameData(t, settled), "phase"); got != "SETTLED" {
		t.Fatalf("SETTLED snapshot phase = %q, want SETTLED", got)
	}
	settledItem := dataObject(t, settled, "item")
	if !hasKey(settledItem, "settle") {
		t.Fatal("SETTLED snapshot item carries no settle object (D-63/D-68)")
	}
	var unsold struct {
		Outcome string `json:"outcome"`
		Winner  any    `json:"winner"`
		Price   int64  `json:"price"`
	}
	if err := json.Unmarshal(settledItem["settle"], &unsold); err != nil {
		t.Fatalf("settle object is not decodable: %v", err)
	}
	if unsold.Outcome != "unsold" || unsold.Winner != nil || unsold.Price != 0 {
		t.Errorf("unsold reveal = %+v, want outcome unsold, winner null and price 0 (ECON-03)", unsold)
	}

	placeBid(t, settledConn, 100_000)
	refused := readType(t, settledConn, TypeBidRejected)
	var reason struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(refused.Data, &reason); err != nil {
		t.Fatalf("bid.rejected payload: %v", err)
	}
	if reason.Reason != "window_closed" {
		t.Errorf("a late bid on a settled item was refused with %q, want window_closed", reason.Reason)
	}

	// FINALE: back to an explicit null item.
	writeFrame(t, admin, `{"type":"admin.item.next"}`)
	_, finale := joinPlayer(t, h, "Finale")
	if got := frameString(t, frameData(t, finale), "phase"); got != "FINALE" {
		t.Fatalf("FINALE snapshot phase = %q, want FINALE", got)
	}
	if raw := dataField(t, finale, "item"); string(raw) != "null" {
		t.Errorf("FINALE snapshot item = %s, want an explicit null", raw)
	}
}
