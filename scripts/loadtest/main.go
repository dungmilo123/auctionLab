// Command loadtest is the real-protocol harness behind `make loadtest`: a pair
// of independent player bots that join a live /ws and validate the frozen
// stream contract on the bytes that actually arrive.
//
// Each bot dials, sends `join {role:"player", nickname}`, then reads until its
// own lobby shows both bots. Every frame is checked: `seq` must be contiguous
// from 1 — a duplicate or a gap fails the run, because a gap is the
// client-visible signal of a dropped message (D-21) — and the `rev` carried by
// successive state events must be non-decreasing and contiguous (D-22).
//
// Scope (D-43): two bots, the smallest pool that proves two connections really
// see each other through the hub. The 20-bot rapid-fire harness and the
// stalled-bot convergence drill stay in Phase 7 (REH-01); this is the smoke test
// that runs before every rehearsal.
//
// It lives inside the main module (`go run ./scripts/loadtest`) and uses the
// same gorilla DefaultDialer the Go tests use, so `make loadtest` needs no
// separate module, no lockfile and no install step (A4).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// defaultTarget matches `make run` (the binary on its default port).
	defaultTarget = "ws://127.0.0.1:8080/ws"
	// bots is D-43's Phase 2 pool: two independent players, so each bot can
	// assert that the other one is visible in its own lobby.
	bots = 2
	// maxFrames bounds every bot's read loop. A run that needs more frames than
	// this to see both players is already broken, and an unbounded loop would
	// hang the gate on a server that keeps talking.
	maxFrames = 8
	// ioTimeout bounds every read and write so a hung server fails the run
	// instead of blocking it.
	ioTimeout = 5 * time.Second
)

// envelope is one server→client frame, decoded with the exact five frozen
// fields (D-21). An unknown field would be ignored here on purpose: D-24's
// freeze is additive, and the Go test suite owns the exact-key-set assertion.
type envelope struct {
	Type string          `json:"type"`
	Seq  int64           `json:"seq"`
	Rev  int64           `json:"rev"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`
}

// helloPayload is D-23's private handshake answer. A player's hello is the only
// frame in the protocol that ever carries a token, and only its own (D-26/D-29).
type helloPayload struct {
	Role     string `json:"role"`
	PlayerID string `json:"player"`
	Nickname string `json:"nickname"`
	Token    string `json:"token"`
	Rev      int64  `json:"rev"`
}

// snapshotPayload is the part of the player projection the lobby contract needs
// (D-25): who this bot is, and who else is in the room.
type snapshotPayload struct {
	Phase    string `json:"phase"`
	Revision int64  `json:"revision"`
	Me       struct {
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
		Budget   int64  `json:"budget"`
	} `json:"me"`
	Lobby []lobbyEntry `json:"lobby"`
}

// lobbyEntry is one lobby row: a nickname and its live presence (D-25).
type lobbyEntry struct {
	Nickname  string `json:"nickname"`
	Connected bool   `json:"connected"`
}

// botSpec is one bot's identity and the peers it must be able to see.
type botSpec struct {
	index    int
	nickname string
	peers    []string
}

// result is one bot's outcome, kept in bot order for printing.
type result struct {
	bot  int
	seen int   // frames read
	rev  int64 // the last state-event revision, -1 before the first
	errs []string
}

// ok reports whether this bot met the contract.
func (r result) ok() bool { return len(r.errs) == 0 }

// stream is one bot's view of its own socket: the next `seq` it must see, the
// last `rev` a state event carried, and the lobby names it has seen so far. It
// is deliberately separate from the socket, so the whole D-21/D-22 contract is
// testable without a server.
type stream struct {
	role    string // the role this stream validates: player, screen or admin
	want    int64  // the seq the next frame must carry; 1 at hello
	lastRev int64  // the last revision a state event carried; -1 before the first
	seen    map[string]bool
}

// newStream starts a stream at its first expected frame, validating the player
// role — the shape every Phase 2 case and both smoke bots use.
func newStream() *stream { return newStreamAs("player") }

// newStreamAs starts a stream for one connection role: the same D-21/D-22
// contract, with the role-specific parts of the handshake and the snapshot
// (`me` exists only for a player; the admin and screen snapshots have no such
// block, and their hello carries no player id or token).
func newStreamAs(role string) *stream {
	return &stream{role: role, want: 1, lastRev: -1, seen: make(map[string]bool)}
}

// observe validates one frame against D-21/D-22/D-23 and the auction payload
// shapes, and folds it in. It returns nil only for a frame that fits the stream
// exactly.
func (s *stream) observe(env envelope) error {
	if env.Type == "" {
		return fmt.Errorf("frame %d: envelope has no type", s.want)
	}
	if env.Seq < s.want {
		return fmt.Errorf("frame %d: duplicate seq %d — the stream already carried it (D-21)", s.want, env.Seq)
	}
	if env.Seq > s.want {
		return fmt.Errorf("frame %d: seq gap — the server sent %d, so a message was dropped (D-21)", s.want, env.Seq)
	}
	if env.TS <= 0 {
		return fmt.Errorf("frame %d: ts = %d, want a positive server millisecond clock (D-21)", s.want, env.TS)
	}

	switch env.Type {
	case "hello":
		if env.Seq != 1 {
			return fmt.Errorf("frame %d: hello arrived at seq %d, want seq 1 (D-23)", s.want, env.Seq)
		}
		var hello helloPayload
		if err := json.Unmarshal(env.Data, &hello); err != nil {
			return fmt.Errorf("frame %d: hello payload is not JSON: %v (%s)", s.want, err, env.Data)
		}
		if hello.Role != s.role {
			return fmt.Errorf("frame %d: hello role = %q, want %q", s.want, hello.Role, s.role)
		}
		if s.role == "player" {
			if hello.PlayerID == "" {
				return fmt.Errorf("frame %d: hello carries no player id (D-23)", s.want)
			}
			if hello.Token == "" {
				return fmt.Errorf("frame %d: hello carries no token (D-29)", s.want)
			}
		}
		if hello.Rev != env.Rev {
			return fmt.Errorf("frame %d: hello rev %d disagrees with the envelope rev %d", s.want, hello.Rev, env.Rev)
		}
	case "state.snapshot":
		var snap snapshotPayload
		if err := json.Unmarshal(env.Data, &snap); err != nil {
			return fmt.Errorf("frame %d: state.snapshot payload is not JSON: %v (%s)", s.want, err, env.Data)
		}
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
		if s.role == "player" && snap.Me.Nickname == "" {
			return fmt.Errorf("frame %d: snapshot carries no `me` row, so it is not self-sufficient (D-25)", s.want)
		}
		for _, entry := range snap.Lobby {
			s.seen[entry.Nickname] = true
		}
	case "phase.changed":
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
		item, err := itemObj(env.Type, env.Data)
		if err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
		if item != nil {
			if _, err := stringKey(item, "status"); err != nil {
				return fmt.Errorf("frame %d: phase.changed item is malformed: %v", s.want, err)
			}
		}
	case "item.intro":
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
		if _, err := itemObj(env.Type, env.Data); err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
	case "item.live":
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
		item, err := itemObj(env.Type, env.Data)
		if err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
		if err := liveItemShape(item); err != nil {
			return fmt.Errorf("frame %d: item.live %v", s.want, err)
		}
	case "item.settled":
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
		item, err := itemObj(env.Type, env.Data)
		if err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
		reveal, err := settleOf(item)
		if err != nil {
			return fmt.Errorf("frame %d: item.settled %v", s.want, err)
		}
		if reveal.Outcome != "sold" && reveal.Outcome != "unsold" {
			return fmt.Errorf("frame %d: settle outcome = %q, want sold or unsold", s.want, reveal.Outcome)
		}
	case "leaderboard":
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
	case "bid.accepted":
		if err := s.acceptRevision(env.Rev); err != nil {
			return err
		}
		if err := acceptedShape(env.Data); err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
	case "price.tick":
		// D-58: a tick carries the CURRENT revision and never moves it, so it
		// deliberately does not go through acceptRevision.
		if err := tickShape(env.Data); err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
	case "bid.rejected":
		// D-53: the private refusal is not a state event either.
		if err := rejectedShape(env.Data); err != nil {
			return fmt.Errorf("frame %d: %v", s.want, err)
		}
	case "error":
		var problem struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(env.Data, &problem)
		return fmt.Errorf("frame %d: the server answered error %q — this bot never joined", s.want, problem.Code)
	default:
		return fmt.Errorf("frame %d: unexpected message type %q with no game running", s.want, env.Type)
	}

	s.want++
	return nil
}

// acceptRevision applies D-22's rule to one state event: a revision may repeat
// (sibling messages legitimately share one), but it must never regress or skip.
// A skip means a state event was never delivered, and only a fresh snapshot can
// repair that — which is precisely the failure this harness exists to catch.
func (s *stream) acceptRevision(rev int64) error {
	if s.lastRev >= 0 && rev != s.lastRev && rev != s.lastRev+1 {
		return fmt.Errorf("state event rev %d does not follow rev %d — revisions must be contiguous and never regress (D-22)",
			rev, s.lastRev)
	}
	s.lastRev = rev
	return nil
}

// complete reports whether every peer has appeared in some snapshot's lobby.
func (s *stream) complete(peers []string) bool {
	for _, peer := range peers {
		if !s.seen[peer] {
			return false
		}
	}
	return true
}

// decodeObject unmarshals a payload into a raw-key object.
func decodeObject(data json.RawMessage) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("payload is not a JSON object: %v (%s)", err, data)
	}
	return object, nil
}

// stringKey reads one required string field.
func stringKey(object map[string]json.RawMessage, key string) (string, error) {
	raw, ok := object[key]
	if !ok {
		return "", fmt.Errorf("payload has no %q key", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("payload.%s is not a string: %v", key, err)
	}
	return value, nil
}

// intKey reads one required integer field.
func intKey(object map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := object[key]
	if !ok {
		return 0, fmt.Errorf("payload has no %q key", key)
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("payload.%s is not an integer: %v", key, err)
	}
	return value, nil
}

// optIntKey reads an optional integer field; an absent key and an explicit null
// both report nil.
func optIntKey(object map[string]json.RawMessage, key string) (*int64, error) {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("payload.%s is not an integer: %v", key, err)
	}
	return &value, nil
}

// itemObj extracts the item object a frame carries, whatever its nesting:
// `phase.changed` and `state.snapshot` nest it under `item` (an explicit null
// when there is none), while `item.intro`/`item.live`/`item.settled` carry it
// directly. It returns (nil, nil) for an explicit null item and an error for a
// frame that carries no item at all.
func itemObj(typ string, data json.RawMessage) (map[string]json.RawMessage, error) {
	switch typ {
	case "phase.changed", "state.snapshot":
	case "item.intro", "item.live", "item.settled":
	default:
		return nil, fmt.Errorf("a %s frame carries no item", typ)
	}

	raw := data
	if typ == "phase.changed" || typ == "state.snapshot" {
		var payload struct {
			Item *json.RawMessage `json:"item"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, fmt.Errorf("%s payload is not JSON: %v", typ, err)
		}
		if payload.Item == nil {
			return nil, fmt.Errorf("%s carries no item key", typ)
		}
		raw = *payload.Item
	}
	if string(raw) == "null" {
		return nil, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("item is not a JSON object: %v (%s)", err, raw)
	}
	return object, nil
}

// settleOf decodes the reveal a settled item carries. A missing key is an ERROR
// here on purpose: D-67's leak boundary is the presence of the whole object, so
// an item that claims to be settled without one is malformed, never a
// legitimate "no reveal".
func settleOf(item map[string]json.RawMessage) (*settleInfo, error) {
	raw, ok := item["settle"]
	if !ok {
		return nil, fmt.Errorf("carries no settle object (D-67's boundary)")
	}
	var reveal settleInfo
	if err := json.Unmarshal(raw, &reveal); err != nil {
		return nil, fmt.Errorf("settle object is not a reveal: %v", err)
	}
	return &reveal, nil
}

// liveItemShape validates the D-67 numbers a LIVE item must carry: a live
// English item without a price would leave the bid form with nothing to show.
func liveItemShape(item map[string]json.RawMessage) error {
	if _, err := stringKey(item, "id"); err != nil {
		return err
	}
	status, err := stringKey(item, "status")
	if err != nil {
		return err
	}
	if status != "live" {
		return fmt.Errorf("status = %q, want live", status)
	}
	if _, err := intKey(item, "current_price"); err != nil {
		return err
	}
	if _, err := intKey(item, "min_next_bid"); err != nil {
		return err
	}
	return nil
}

// acceptedShape validates D-54's broadcast: the accepted amount plus the
// server-computed numbers a client renders.
func acceptedShape(data json.RawMessage) error {
	object, err := decodeObject(data)
	if err != nil {
		return err
	}
	for _, key := range []string{"player_id", "nickname"} {
		if _, err := stringKey(object, key); err != nil {
			return err
		}
	}
	for _, key := range []string{"amount", "current_price", "min_next_bid"} {
		if _, err := intKey(object, key); err != nil {
			return err
		}
	}
	return nil
}

// rejectionReasons is D-52's frozen reason vocabulary (D-53 carries it as a
// payload-level string).
var rejectionReasons = []string{
	"invalid_amount", "window_closed", "below_minimum", "exceeds_budget", "already_leading",
}

// rejectedShape validates D-53's private payload.
func rejectedShape(data json.RawMessage) error {
	object, err := decodeObject(data)
	if err != nil {
		return err
	}
	reason, err := stringKey(object, "reason")
	if err != nil {
		return err
	}
	if !slices.Contains(rejectionReasons, reason) {
		return fmt.Errorf("bid.rejected reason %q is not one of the frozen reasons", reason)
	}
	for _, key := range []string{"budget", "amount"} {
		if _, err := intKey(object, key); err != nil {
			return err
		}
	}
	return nil
}

// tickShape validates D-56's timing frame: the price, the server-computed
// minimum, and the deadline/remaining pair that must agree with each other —
// remaining_ms without a deadline has nothing to anchor on, and a negative
// window would end the auction on the phone before the server settled it.
func tickShape(data json.RawMessage) error {
	object, err := decodeObject(data)
	if err != nil {
		return err
	}
	if _, err := stringKey(object, "item_id"); err != nil {
		return err
	}
	if _, err := intKey(object, "current_price"); err != nil {
		return err
	}
	if _, err := intKey(object, "min_next_bid"); err != nil {
		return err
	}
	deadline, err := optIntKey(object, "deadline_at")
	if err != nil {
		return err
	}
	remaining, err := optIntKey(object, "remaining_ms")
	if err != nil {
		return err
	}
	if deadline == nil && remaining == nil {
		return nil // a run without a deadline carries neither
	}
	if deadline == nil || remaining == nil {
		return fmt.Errorf("price.tick must carry deadline_at and remaining_ms together")
	}
	if *remaining < 0 {
		return fmt.Errorf("price.tick remaining_ms = %d, want a non-negative window", *remaining)
	}
	return nil
}

// itemIDFromSnapshot reads the id of the last item an admin snapshot listed —
// the round's way of learning the id the server minted for a create (H14).
func itemIDFromSnapshot(data json.RawMessage) (string, error) {
	var view struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &view); err != nil {
		return "", fmt.Errorf("admin snapshot is not JSON: %v", err)
	}
	if len(view.Items) == 0 {
		return "", fmt.Errorf("admin snapshot lists no items")
	}
	return view.Items[len(view.Items)-1].ID, nil
}

// adminKeyFlag is D-76's script/probe capability for the round's admin bot. It
// is registered at package level so it exists before the shared flag set is
// parsed (resolveTarget owns the parse), and it defaults to ADMIN_KEY because
// that is the variable a server started with an operator-pinned key was given.
// A server that minted and persisted its key (D-74) printed it once, in the
// `/admin?k=…` boot line; a human passes that value here.
var adminKeyFlag = flag.String("admin-key", os.Getenv("ADMIN_KEY"),
	"admin capability for the round's admin bot (default: $ADMIN_KEY)")

func main() {
	target := resolveTarget()
	adminKey := *adminKeyFlag
	start := time.Now()

	// One run, one namespace: the nicknames carry a per-run suffix, so a second
	// `make loadtest` against the same dev database does not collide with the
	// players the previous run left behind (D-28's uniqueness is server-wide).
	run := fmt.Sprintf("lt-%06d", time.Now().UnixNano()%1_000_000)
	nicknames := make([]string, bots)
	specs := make([]botSpec, bots)
	for i := range specs {
		nicknames[i] = fmt.Sprintf("%s-%d", run, i)
	}
	for i := range specs {
		specs[i] = botSpec{index: i, nickname: nicknames[i], peers: nicknames}
	}

	results := make([]result, bots)
	var wg sync.WaitGroup
	for i, spec := range specs {
		wg.Add(1)
		go func(i int, spec botSpec) {
			defer wg.Done()
			results[i] = runBot(target, spec)
		}(i, spec)
	}
	wg.Wait()

	okBots, failed := 0, false
	for _, res := range results {
		if res.ok() {
			okBots++
			fmt.Printf("bot %d: ok — %d frames, last state rev %d, saw every player\n", res.bot, res.seen, res.rev)
			continue
		}
		failed = true
		fmt.Printf("bot %d: FAILED — %d frames read\n", res.bot, res.seen)
		for _, e := range res.errs {
			fmt.Printf("  %s\n", e)
		}
	}

	// Phase 3: one bounded English round through the same protocol — an admin
	// bot creates the item and drives the phase changes, two player bots bid
	// (one accepted, one refused privately), and the round ends on the reveal.
	round := runRound(target, run, adminKey)
	for _, line := range round.lines {
		fmt.Printf("round: %s\n", line)
	}
	for _, problem := range round.errs {
		fmt.Printf("round: FAILED — %s\n", problem)
	}
	roundVerdict := "completed"
	if !round.passed() {
		roundVerdict = "FAILED"
		failed = true
	}

	fmt.Printf("loadtest: %s — %d/%d bots joined and saw each other, the English round %s, in %s\n",
		target, okBots, bots, roundVerdict, time.Since(start).Round(time.Millisecond))

	if failed {
		os.Exit(1)
	}
}

// settleInfo is D-67's reveal as the harness reads it off the wire.
type settleInfo struct {
	Outcome string `json:"outcome"`
	Winner  *struct {
		Nickname string `json:"nickname"`
	} `json:"winner"`
	Price       int64 `json:"price"`
	TruthValue  int64 `json:"truth_value"`
	BudgetDelta int64 `json:"budget_delta"`
}

// --- Phase 3: the bounded English round ------------------------------------

// roundIO bounds every read and write of the round, so a stalled server fails
// the gate instead of hanging it (the same rule the Phase 2 bots follow).
const roundIO = 10 * time.Second

// roundBot is one live connection inside the round: its socket, the stream
// contract every frame it receives is validated against, and how many frames it
// has read.
type roundBot struct {
	label  string
	role   string
	conn   *websocket.Conn
	s      *stream
	frames int
}

// dialRound connects one role and completes its handshake. A player's join is a
// mutation, so its own fan-out snapshot follows the handshake pair (H2) — it is
// consumed here so every bot starts the round with nothing queued.
//
// The admin round is authorized (D-76/ADMN-04): this harness IS the script
// channel the decision deliberately keeps, so the capability travels in the join
// frame's `key` field. A missing capability fails here with an actionable
// message instead of surfacing as a not_authorized frame from the server.
func dialRound(target, label, role, nickname, adminKey string) (*roundBot, error) {
	if role == "admin" && adminKey == "" {
		return nil, fmt.Errorf("%s: no admin capability — set ADMIN_KEY (or pass --admin-key) to the key the server printed at boot", label)
	}

	conn, resp, err := websocket.DefaultDialer.Dial(target, nil)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("dial %s as %s: %v (%s)", target, label, err, status)
	}

	b := &roundBot{label: label, role: role, conn: conn, s: newStreamAs(role)}

	join := map[string]string{"type": "join", "role": role}
	if nickname != "" {
		join["nickname"] = nickname
	}
	if role == "admin" {
		join["key"] = adminKey
	}
	frame, err := json.Marshal(join)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("%s: marshaling the join: %v", label, err)
	}
	if err := b.send(string(frame)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if _, err := b.await(roundIO, func(env envelope) (bool, error) { return env.Type == "hello", nil }); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("%s: waiting for the handshake: %w", label, err)
	}
	if _, err := b.awaitTypes(roundIO, "state.snapshot"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("%s: waiting for the handshake: %w", label, err)
	}
	if role == "player" {
		if _, err := b.awaitTypes(roundIO, "state.snapshot"); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("%s: waiting for its own join fan-out: %w", label, err)
		}
	}
	return b, nil
}

func (b *roundBot) close() { _ = b.conn.Close() }

// send writes one command frame.
func (b *roundBot) send(frame string) error {
	_ = b.conn.SetWriteDeadline(time.Now().Add(roundIO))
	if err := b.conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		return fmt.Errorf("%s: sending: %w", b.label, err)
	}
	return nil
}

// start sends one `admin.item.start {id}` (D-65).
func (b *roundBot) start(itemID string) error {
	return b.send(fmt.Sprintf(`{"type":"admin.item.start","id":%q}`, itemID))
}

// await reads until want matches, validating EVERY frame against the stream
// contract on the way. want may also return an error to fail on a frame that
// must not appear here — a private rejection reaching the wrong socket.
func (b *roundBot) await(budget time.Duration, want func(envelope) (bool, error)) (envelope, error) {
	deadline := time.Now().Add(budget)
	for {
		if time.Now().After(deadline) {
			return envelope{}, fmt.Errorf("%s: no expected frame within %s (read %d frames)", b.label, budget, b.frames)
		}
		_ = b.conn.SetReadDeadline(deadline)
		_, raw, err := b.conn.ReadMessage()
		if err != nil {
			return envelope{}, fmt.Errorf("%s: reading frame %d: %w", b.label, b.frames+1, err)
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return envelope{}, fmt.Errorf("%s: frame %d is not JSON: %v (%s)", b.label, b.frames+1, err, raw)
		}
		if err := b.s.observe(env); err != nil {
			return envelope{}, fmt.Errorf("%s: %w", b.label, err)
		}
		b.frames++

		match, err := want(env)
		if err != nil {
			return envelope{}, fmt.Errorf("%s: %w", b.label, err)
		}
		if match {
			return env, nil
		}
	}
}

// awaitTypes reads exactly the named frame types, in order. A different type
// where one of these was due is a failure — including a frame that should never
// have reached this connection.
func (b *roundBot) awaitTypes(budget time.Duration, types ...string) ([]envelope, error) {
	frames := make([]envelope, 0, len(types))
	for i, want := range types {
		env, err := b.await(budget, func(env envelope) (bool, error) { return env.Type == want, nil })
		if err != nil {
			return nil, err
		}
		if env.Type != want {
			return nil, fmt.Errorf("%s: frame %d = %q, want %q (the fan-out order is the contract)", b.label, i, env.Type, want)
		}
		frames = append(frames, env)
	}
	return frames, nil
}

// awaitItemID reads forward to the CRUD snapshot that carries the new item and
// returns the id the server minted (H14).
func (b *roundBot) awaitItemID(budget time.Duration) (string, error) {
	deadline := time.Now().Add(budget)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s: no admin snapshot carried the item within %s", b.label, budget)
		}
		env, err := b.await(time.Until(deadline), func(env envelope) (bool, error) {
			return env.Type == "state.snapshot", nil
		})
		if err != nil {
			return "", err
		}
		id, err := itemIDFromSnapshot(env.Data)
		if err == nil {
			return id, nil
		}
	}
}

// roundReport is what the round proved, in the order it happened.
type roundReport struct {
	lines []string
	errs  []string
}

func (r *roundReport) ok(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *roundReport) fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

// passed reports whether the round completed every step.
func (r *roundReport) passed() bool { return len(r.errs) == 0 }

// runRound plays one bounded English round against a live server and validates
// every frame every bot receives: the admin creates an item and starts it twice,
// two players bid (one accepted, one refused PRIVATELY), and the admin force-ends
// it — so the round's whole path, from a real socket command to the committed
// reveal, is what the exit code reflects (D-43's end-to-end gate).
//
// adminKey is D-76's capability for the admin bot; the players need nothing.
func runRound(target, run, adminKey string) roundReport {
	var report roundReport

	admin, err := dialRound(target, "admin", "admin", "", adminKey)
	if err != nil {
		report.fail("%v", err)
		return report
	}
	defer admin.close()

	a, err := dialRound(target, "player A", "player", run+"-a", "")
	if err != nil {
		report.fail("%v", err)
		return report
	}
	defer a.close()

	b, err := dialRound(target, "player B", "player", run+"-b", "")
	if err != nil {
		report.fail("%v", err)
		return report
	}
	defer b.close()

	// The bots that were already connected saw the later joins fan out: a join
	// has no typed event of its own, so the snapshot is the frame (H2).
	if _, err := admin.awaitTypes(roundIO, "state.snapshot", "state.snapshot"); err != nil {
		report.fail("consuming the join fan-out: %v", err)
		return report
	}
	if _, err := a.awaitTypes(roundIO, "state.snapshot"); err != nil {
		report.fail("consuming the join fan-out: %v", err)
		return report
	}

	// 1. Create the English item (D-66).
	item := fmt.Sprintf(`{"name":%q,"auction_type":"english","truth_value":350000,"start_price":100000,`+
		`"min_increment":10000,"duration_ms":60000,"anti_snipe_mode":"disabled"}`, run+" lot")
	if err := admin.send(`{"type":"admin.item.upsert","item":` + item + `}`); err != nil {
		report.fail("%v", err)
		return report
	}
	itemID, err := admin.awaitItemID(roundIO)
	if err != nil {
		report.fail("admin.item.upsert: %v", err)
		return report
	}
	for _, bot := range []*roundBot{a, b} {
		if _, err := bot.awaitTypes(roundIO, "state.snapshot"); err != nil {
			report.fail("the item.upsert fan-out: %v", err)
			return report
		}
	}
	report.ok("admin created item %s (D-66)", itemID)

	// 2. start → ITEM_INTRO, then start → LIVE with the immediate tick (D-65).
	if err := admin.start(itemID); err != nil {
		report.fail("%v", err)
		return report
	}
	for _, bot := range []*roundBot{admin, a, b} {
		if _, err := bot.awaitTypes(roundIO, "phase.changed", "item.intro", "state.snapshot"); err != nil {
			report.fail("item.start → ITEM_INTRO: %v", err)
			return report
		}
	}
	report.ok("item.start reached ITEM_INTRO on every socket")

	if err := admin.start(itemID); err != nil {
		report.fail("%v", err)
		return report
	}
	var live []envelope
	for i, bot := range []*roundBot{admin, a, b} {
		frames, err := bot.awaitTypes(roundIO, "phase.changed", "item.live", "state.snapshot", "price.tick")
		if err != nil {
			report.fail("item.start → LIVE: %v", err)
			return report
		}
		if i == 1 {
			live = frames
		}
	}
	liveItem, err := itemObj("item.live", live[1].Data)
	if err != nil {
		report.fail("item.live: %v", err)
		return report
	}
	if err := liveItemShape(liveItem); err != nil {
		report.fail("item.live: %v", err)
		return report
	}
	if deadline, err := optIntKey(liveItem, "deadline_at"); err != nil || deadline == nil {
		report.fail("item.live carries no deadline_at: the countdown has nothing to anchor on (TIME-02)")
		return report
	}
	tick, err := decodeObject(live[3].Data)
	if err != nil {
		report.fail("price.tick: %v", err)
		return report
	}
	if price, err := intKey(tick, "current_price"); err != nil || price != 100000 {
		report.fail("the immediate tick's current_price = %d (err %v), want the start price 100000", price, err)
		return report
	}
	if remaining, err := optIntKey(tick, "remaining_ms"); err != nil || remaining == nil || *remaining <= 0 {
		report.fail("the immediate tick's remaining_ms = %v (err %v), want a positive window", remaining, err)
		return report
	}
	report.ok("item.start reached LIVE with the immediate price.tick on every socket")

	// 3. Player A bids the minimum: accepted everywhere at one revision, with
	// its own immediate tick.
	if err := a.send(`{"type":"bid.place","amount":100000}`); err != nil {
		report.fail("%v", err)
		return report
	}
	var accepted envelope
	var rev int64
	for i, bot := range []*roundBot{admin, a, b} {
		frames, err := bot.awaitTypes(roundIO, "bid.accepted", "price.tick")
		if err != nil {
			report.fail("bid.place: %v", err)
			return report
		}
		if i == 1 {
			accepted = frames[0]
			rev = frames[0].Rev
		}
	}
	acceptedPayload, err := decodeObject(accepted.Data)
	if err != nil {
		report.fail("bid.accepted: %v", err)
		return report
	}
	if err := acceptedShape(accepted.Data); err != nil {
		report.fail("bid.accepted: %v", err)
		return report
	}
	if nickname, _ := stringKey(acceptedPayload, "nickname"); nickname != run+"-a" {
		report.fail("bid.accepted nickname = %q, want %q", nickname, run+"-a")
		return report
	}
	if min, err := intKey(acceptedPayload, "min_next_bid"); err != nil || min != 110000 {
		report.fail("bid.accepted min_next_bid = %d (err %v), want 110000", min, err)
		return report
	}
	report.ok("the minimum bid was accepted on every socket at rev %d", rev)

	// 4. Player B bids below the new minimum: refused, and PRIVATELY — A and the
	// admin only ever see the round continue (D-53).
	if err := b.send(`{"type":"bid.place","amount":100000}`); err != nil {
		report.fail("%v", err)
		return report
	}
	refused, err := b.await(roundIO, func(env envelope) (bool, error) { return env.Type == "bid.rejected", nil })
	if err != nil {
		report.fail("bid.rejected: %v", err)
		return report
	}
	var rejection struct {
		Reason     string `json:"reason"`
		MinNextBid int64  `json:"min_next_bid"`
	}
	if err := json.Unmarshal(refused.Data, &rejection); err != nil {
		report.fail("bid.rejected payload: %v", err)
		return report
	}
	if rejection.Reason != "below_minimum" {
		report.fail("bid.rejected reason = %q, want below_minimum", rejection.Reason)
		return report
	}
	if rejection.MinNextBid != 110000 {
		report.fail("bid.rejected min_next_bid = %d, want 110000", rejection.MinNextBid)
		return report
	}
	if b.s.lastRev != rev {
		report.fail("the refused bid moved the revision to %d, want the unchanged %d (D-53)", b.s.lastRev, rev)
		return report
	}
	report.ok("a below-minimum bid was refused privately and moved no revision")

	// 5. Force end: the reveal, the standings and the SETTLED snapshot reach
	// every role, and no player payload carries a top-level truth_value.
	if err := admin.send(`{"type":"admin.item.force_end"}`); err != nil {
		report.fail("%v", err)
		return report
	}
	for _, bot := range []*roundBot{admin, a, b} {
		frames, err := bot.awaitTypes(roundIO, "item.settled", "leaderboard", "state.snapshot")
		if err != nil {
			report.fail("admin.item.force_end: %v", err)
			return report
		}
		settled, err := itemObj("item.settled", frames[0].Data)
		if err != nil {
			report.fail("item.settled: %v", err)
			return report
		}
		reveal, err := settleOf(settled)
		if err != nil {
			report.fail("item.settled: %v", err)
			return report
		}
		if reveal.Outcome != "sold" || reveal.Price != 100000 {
			report.fail("the reveal = %s at %d, want sold at the accepted bid", reveal.Outcome, reveal.Price)
			return report
		}
		if reveal.Winner == nil || reveal.Winner.Nickname != run+"-a" {
			report.fail("the reveal's winner = %+v, want %q", reveal.Winner, run+"-a")
			return report
		}
		if _, leak := settled["truth_value"]; leak {
			report.fail("%s: item.settled carried a top-level truth_value", bot.label)
			return report
		}
		snapshotItem, err := itemObj("state.snapshot", frames[2].Data)
		if err != nil {
			report.fail("the SETTLED snapshot: %v", err)
			return report
		}
		if _, err := settleOf(snapshotItem); err != nil {
			report.fail("the SETTLED snapshot's item carries no reveal (D-63): %v", err)
			return report
		}
	}
	report.ok("the force-end reveal, leaderboard and SETTLED snapshot reached every role")

	return report
}

// resolveTarget picks the first of: positional argument, WS_URL, the default.
func resolveTarget() string {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: go run ./scripts/loadtest [ws-url] [--admin-key KEY]\n\n")
		fmt.Fprintf(out, "Dials %d player bots against a live /ws, asserts the real protocol's\n", bots)
		fmt.Fprintf(out, "seq/rev contract, then plays one bounded English round (an admin bot\n")
		fmt.Fprintf(out, "creates and drives an item, two players bid, and the round ends on the\n")
		fmt.Fprintf(out, "reveal). It exits non-zero when any bot cannot join, the round cannot\n")
		fmt.Fprintf(out, "complete, or a frame breaks the stream contract. The URL may also come\n")
		fmt.Fprintf(out, "from WS_URL.\n\n")
		fmt.Fprintf(out, "The admin bot is authorized (ADMN-04): --admin-key, or ADMIN_KEY in the\n")
		fmt.Fprintf(out, "environment, must carry the key the server printed at boot.\n\n")
		fmt.Fprintf(out, "  default: %s\n", defaultTarget)
	}
	flag.Parse()

	if arg := flag.Arg(0); arg != "" {
		// Go's flag package stops at the first non-flag argument, and this
		// harness takes the URL positionally — so `loadtest ws://… --admin-key K`
		// would otherwise drop the flag silently and fail the round with a
		// confusing "no admin capability". The remainder is parsed here, so both
		// orders mean the same thing.
		if rest := flag.Args()[1:]; len(rest) > 0 {
			_ = flag.CommandLine.Parse(rest) // ExitOnError: a bad flag exits here
		}
		return arg
	}
	if env := os.Getenv("WS_URL"); env != "" {
		return env
	}
	return defaultTarget
}

// runBot dials one connection, joins as a player, and reads until its own lobby
// shows every peer. It never returns an error: failures are collected in the
// result so one bot's trouble cannot hide another's.
func runBot(target string, spec botSpec) result {
	res := result{bot: spec.index, rev: -1}

	conn, resp, err := websocket.DefaultDialer.Dial(target, nil)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
			_ = resp.Body.Close()
		}
		res.errs = append(res.errs, fmt.Sprintf("dial %s: %v (%s)", target, err, status))
		return res
	}
	defer func() { _ = conn.Close() }()

	join, err := json.Marshal(map[string]string{
		"type":     "join",
		"role":     "player",
		"nickname": spec.nickname,
	})
	if err != nil {
		res.errs = append(res.errs, fmt.Sprintf("marshaling the join: %v", err))
		return res
	}
	_ = conn.SetWriteDeadline(time.Now().Add(ioTimeout))
	if err := conn.WriteMessage(websocket.TextMessage, join); err != nil {
		res.errs = append(res.errs, fmt.Sprintf("sending the join: %v", err))
		return res
	}

	s := newStream()
	for res.seen < maxFrames {
		_ = conn.SetReadDeadline(time.Now().Add(ioTimeout))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: read: %v", s.want, err))
			return res
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: envelope is not JSON: %v (%s)", s.want, err, raw))
			return res
		}
		if err := s.observe(env); err != nil {
			res.errs = append(res.errs, err.Error())
			return res
		}

		res.seen = int(s.want) - 1
		res.rev = s.lastRev
		if s.complete(spec.peers) {
			return res
		}
	}

	res.errs = append(res.errs, fmt.Sprintf(
		"no snapshot showed every player (%s) in %d frames — the other bot never became visible",
		strings.Join(spec.peers, ", "), res.seen))
	return res
}
