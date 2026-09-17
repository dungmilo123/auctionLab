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
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// defaultTarget matches `make run` (the binary on its default port).
	defaultTarget = "ws://127.0.0.1:8080/ws"
	// bots is D-43's Phase 2 pool: two independent players, so each bot can
	// assert that the other one is visible in its own lobby. D-135's `--bots`
	// flag raises it; the smoke default stays two.
	bots = 2
	// maxFrames is the floor of every bot's frame budget: D-135's `--bots N`
	// runs scale it with the pool (frameBudget), and the two-bot smoke keeps
	// exactly this bound.
	maxFrames = 8
	// ioTimeout bounds every read and write so a hung server fails the run
	// instead of blocking it.
	ioTimeout = 5 * time.Second

	// typeHeartbeat and typeTunnelStatus are D-132/D-123's control-plane
	// frames: each takes a real seq on the connection's FIFO, carries the
	// CURRENT revision without moving it, and means nothing to a game run. The
	// harness must count them and keep going — a rapid-fire room is full of
	// them, and treating one as "unexpected with no game running" would fail a
	// perfectly healthy run.
	typeHeartbeat    = "heartbeat"
	typeTunnelStatus = "tunnel.status"
)

// frameBudget is how many frames ONE bot may read before the run gives up: the
// handshake pair, one fan-out snapshot per peer, and headroom for a round's
// frames. It scales with the pool because a 20-bot room fans out twenty times
// as much as the smoke pair.
func frameBudget(n int) int {
	if n < bots {
		n = bots
	}
	return max(maxFrames, 2+3*n)
}

// peakCommandRate is the largest number of commands seen in any one-second
// window of the recorded instants. It is the evidence D-138's bucket sizing is
// judged from (research Pattern 5 item 6): a false `rate_limited` on a
// legitimate rapid-fire bid is a correctness bug, so the harness must print what
// it actually asked of the limiter.
func peakCommandRate(at []time.Time) float64 {
	if len(at) == 0 {
		return 0
	}
	peak := 0
	for i := range at {
		seen := 0
		for j := i; j < len(at); j++ {
			if at[j].Sub(at[i]) > time.Second {
				break
			}
			seen++
		}
		peak = max(peak, seen)
	}
	return float64(peak)
}

// converged is D-135's end-of-run assertion: every bot's last state revision
// must equal the server's final revision. It returns a run-stopping error naming
// the bots that disagree, so a partially-converged room can never be certified.
func converged(revs []int64, want int64) error {
	if len(revs) == 0 {
		return fmt.Errorf("no bots reported a revision")
	}
	var lagging []string
	for i, rev := range revs {
		if rev != want {
			lagging = append(lagging, fmt.Sprintf("bot %d at rev %d", i, rev))
		}
	}
	if len(lagging) > 0 {
		return fmt.Errorf("%d/%d bots did not converge on the final revision %d: %s",
			len(lagging), len(revs), want, strings.Join(lagging, ", "))
	}
	return nil
}

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

	// rapid is D-135's repair-tolerant mode: under rapid fire a game-frame seq
	// gap is the CLIENT-VISIBLE SIGNAL OF A DROP, and D-41 repairs it with one
	// recovery snapshot. In this mode — and only this mode — a gap is legal
	// when the frame that reveals it is that snapshot. gaps and repairs record
	// the positive evidence a drill must show (D-136): a run that never gaped
	// proves nothing about the repair path.
	rapid   bool
	gaps    int
	repairs int

	// snapshots counts the state.snapshot frames this stream consumed. The
	// N-bot run's drain barrier polls it: a resync reply is a snapshot, and a
	// bot's own reply cannot arrive before its burst frames were processed.
	snapshots int
}

// newStream starts a stream at its first expected frame, validating the player
// role — the shape every Phase 2 case and both smoke bots use.
func newStream() *stream { return newStreamAs("player") }

// newRapidStream is newStream in D-135's rapid-fire mode: identical strict
// accounting, plus the one D-41 repair shape. It backs `--rapid-fire`, never the
// two-bot smoke.
func newRapidStream() *stream {
	s := newStream()
	s.rapid = true
	return s
}

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
	if env.TS <= 0 {
		return fmt.Errorf("frame %d: ts = %d, want a positive server millisecond clock (D-21)", s.want, env.TS)
	}

	// D-132/D-123's control plane comes FIRST, before any game-run handling:
	// a heartbeat or a tunnel.status is applied at ANY seq (the browser gate
	// does exactly that, and a room full of them is the normal rapid-fire
	// state), it carries the current revision without moving it, and its
	// payload means nothing here. A stale copy has already been counted, so it
	// is ignored rather than failed as a duplicate.
	if env.Type == typeHeartbeat || env.Type == typeTunnelStatus {
		if env.Seq >= s.want {
			s.want = env.Seq + 1
		}
		return nil
	}

	if env.Seq < s.want {
		return fmt.Errorf("frame %d: duplicate seq %d — the stream already carried it (D-21)", s.want, env.Seq)
	}
	if env.Seq > s.want {
		// D-41's ONE legal gap: under rapid fire a fenced connection loses a
		// burst and is repaired by a snapshot, so the frame that reveals the
		// gap must BE that snapshot. Anything else is a silent gap — the exact
		// failure the two-bot smoke exists to catch, and one the relaxed mode
		// must never wave through (D-135).
		if !s.rapid {
			return fmt.Errorf("frame %d: seq gap — the server sent %d, so a message was dropped (D-21)", s.want, env.Seq)
		}
		s.gaps++
		if env.Type != "state.snapshot" {
			return fmt.Errorf("frame %d: silent gap — seq jumped to %d and the frame is %q, not the recovery snapshot (D-41/D-135)",
				s.want, env.Seq, env.Type)
		}
		s.repairs++
		s.lastRev = env.Rev
		s.want = env.Seq + 1
		s.snapshots++
		for _, entry := range lobbyEntries(env.Data) {
			s.seen[entry] = true
		}
		return nil
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
		s.snapshots++
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

// lobbyEntries reads a snapshot payload's lobby nicknames. The repair arm needs
// it because a recovery snapshot is the frame that re-establishes the room after
// a drop; a payload that is not a snapshot yields nothing.
func lobbyEntries(data json.RawMessage) []string {
	var snap snapshotPayload
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil
	}
	names := make([]string, 0, len(snap.Lobby))
	for _, entry := range snap.Lobby {
		names = append(names, entry.Nickname)
	}
	return names
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

// D-135/D-136's N-bot modes. The defaults are the shipped smoke shape: two
// bots, no burst, nobody stalled — so `make loadtest` is the same run it has
// always been, and every new mode is opt-in.
var (
	botsFlag = flag.Int("bots", bots,
		"player bots to dial (default 2: the smoke pair)")
	rapidFlag = flag.Bool("rapid-fire", false,
		"blast --burst bid commands from every bot, then assert the room converged (D-135)")
	burstFlag = flag.Int("burst", 30,
		"bids each bot sends per rapid-fire round (bounded; stays inside burst 40, D-138)")
	roundsFlag = flag.Int("rounds", 1,
		"burst rounds to run while the stalled bot stays paused (--stalled-index; D-136's fence needs volume)")
	stalledFlag = flag.Int("stalled-index", -1,
		"bot index that stops reading during the burst so the fence and its repair are exercised (D-136; -1: none)")
)

func main() {
	target := resolveTarget()
	adminKey := *adminKeyFlag
	start := time.Now()

	// One run, one namespace: the nicknames carry a per-run suffix, so a second
	// `make loadtest` against the same dev database does not collide with the
	// players the previous run left behind (D-28's uniqueness is server-wide).
	run := fmt.Sprintf("lt-%06d", time.Now().UnixNano()%1_000_000)

	// D-135/D-136's N-bot modes take a different path from the smoke pair: the
	// smoke's assertions are about two connections seeing each other, while the
	// fleet's are about twenty streams staying ordered and converging. The
	// shipped shape is unchanged — `make loadtest` still runs exactly what it
	// ran before this phase.
	if *rapidFlag || *botsFlag != bots || *stalledFlag >= 0 {
		os.Exit(runFleetMain(target, run, fleetConfig{
			bots:    *botsFlag,
			rapid:   *rapidFlag,
			burst:   *burstFlag,
			rounds:  *roundsFlag,
			stalled: *stalledFlag,
		}, start))
	}

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
		fmt.Fprintf(out, "usage: go run ./scripts/loadtest [ws-url] [--admin-key KEY] [--bots N] [--rapid-fire] [--burst K] [--rounds R] [--stalled-index I]\n\n")
		fmt.Fprintf(out, "Dials %d player bots against a live /ws, asserts the real protocol's\n", bots)
		fmt.Fprintf(out, "seq/rev contract, then plays one bounded English round (an admin bot\n")
		fmt.Fprintf(out, "creates and drives an item, two players bid, and the round ends on the\n")
		fmt.Fprintf(out, "reveal). It exits non-zero when any bot cannot join, the round cannot\n")
		fmt.Fprintf(out, "complete, or a frame breaks the stream contract. The URL may also come\n")
		fmt.Fprintf(out, "from WS_URL.\n\n")
		fmt.Fprintf(out, "--bots N raises the pool; --rapid-fire makes every bot blast --burst\n")
		fmt.Fprintf(out, "bids and then asserts the whole room converged (D-135); --stalled-index\n")
		fmt.Fprintf(out, "names the one bot that stops reading during the burst, so the fence and\n")
		fmt.Fprintf(out, "its repair are exercised for real (D-136); --rounds keeps bursting while\n")
		fmt.Fprintf(out, "it stays paused, because the fence needs volume, not intent. The\n")
		fmt.Fprintf(out, "two-bot default is the smoke run: no burst, nobody stalled, the stream\n")
		fmt.Fprintf(out, "still strict.\n\n")
		fmt.Fprintf(out, "The admin bot is authorized (ADMN-04): --admin-key, or ADMIN_KEY in the\n")
		fmt.Fprintf(out, "environment, must carry the key the server printed at boot. --rapid-fire\n")
		fmt.Fprintf(out, "needs it too: the burst bids against an item the fleet's own admin starts.\n\n")
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
	for res.seen < frameBudget(len(spec.peers)) {
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

// --- Phase 7: the N-bot fleet (D-135/D-136) --------------------------------
//
// The smoke pair above proves two connections see each other. This section
// proves what a REAL room does: twenty connections, every stream validated as it
// arrives, a bounded burst of real bids, one connection deliberately not
// reading, and D-135's single strongest assertion at the end — every bot on the
// same final revision.
//
// The assertions are the point, not the traffic. A harness that generated twenty
// sockets and printed a number would certify nothing; the cases in main_test.go
// pin each rule here, including the ones that must FAIL.

// fleetConfig is one N-bot run's whole shape.
type fleetConfig struct {
	bots    int  // how many player bots dial (D-135's --bots)
	rapid   bool // blast a bounded burst and assert convergence (--rapid-fire)
	burst   int  // bids per bot per round (--burst)
	rounds  int  // burst rounds while the stalled bot stays paused (--rounds)
	stalled int  // the bot that stops reading during the burst (-1: none)
}

// fleetReport is what an N-bot run proved, in the order it happened. It mirrors
// roundReport: one line per proved fact, one entry per failure, and an exit code
// that follows the failures.
type fleetReport struct {
	lines []string
	errs  []string
}

func (r *fleetReport) ok(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *fleetReport) fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func (r *fleetReport) passed() bool { return len(r.errs) == 0 }

// burstTimeout bounds the burst and the drain that follows it. A race-built
// server under a race-built harness is slow on purpose, so the ceiling is
// generous — but it exists: a run that hangs must fail the gate, not hold it.
const burstTimeout = 60 * time.Second

// commandSpacing paces a bot's burst so the ROOM interleaves. Without it each
// bot's commands arrive back to back on its own socket, the first acceptance
// makes that bot the leader, and every later command is refused as
// already_leading — which produces almost no fan-out, and a drill whose stalled
// peer never fills its FIFO proves nothing (D-136). The spacing costs tens of
// milliseconds and is what makes the burst a room-wide burst instead of twenty
// private monologues.
const commandSpacing = 2 * time.Millisecond

// constrainedLink dials the stalled bot's socket the way a phone on a weak link
// looks to the server: a 2 KiB receive buffer AND a deliberately small MSS.
// Both matter, and neither is cosmetic:
//
//   - The receive buffer bounds what the local kernel can absorb before the
//     server's writes stop making progress.
//   - The MSS is the larger lever by far. Linux sizes the sender's transmit
//     buffer as 2×cwnd×MSS, and loopback's MSS is ~64 KiB, so the SERVER will
//     quietly buffer megabytes before its writer blocks — and a drill that
//     needs megabytes of fan-out never overflows the hub's FIFO, which is the
//     exact "drill that never stalls proves nothing" failure D-136 names.
//     Measured on this machine: ~2.1 MiB absorbed at the default MSS versus
//     ~14–32 KiB at 256–536 bytes.
//
// The seam is the one internal/hub/ordering_test.go documents, inlined because
// the harness is package main and cannot import the hub's test helpers.
func constrainedLink(network, addr string) (net.Conn, error) {
	d := net.Dialer{Control: func(_, _ string, c syscall.RawConn) error {
		var sockErr error
		if err := c.Control(func(fd uintptr) {
			if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 2048); err != nil {
				sockErr = err
				return
			}
			sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_MAXSEG, stalledMSS)
		}); err != nil {
			return err
		}
		return sockErr
	}}
	return d.Dial(network, addr)
}

// stalledMSS is the stalled bot's advertised segment size. 256 bytes is
// deliberately below the 536-byte IPv4 minimum reassembly size: the fence must
// be crossed by the fan-out a burst actually produces (hundreds of frames), not
// by a lucky megabyte of kernel buffering.
const stalledMSS = 256

// burstSpacing paces a burst so the D-138 bucket never runs dry: the bucket
// lets CommandBurst commands through at once and refills at CommandRate per
// second, so a burst of N needs a window of at least (N - CommandBurst) /
// CommandRate seconds. Without this, a burst past the allowance is answered
// with rate_limited refusals — legitimate traffic by the bucket's definition,
// but useless as fan-out for the fence the stalled bot has to cross.
//
// The 2 ms floor is the shipped smoke cadence: a 30-command burst is already
// inside the allowance, and spacing it wider would only slow the drill down.
func burstSpacing(burst int) time.Duration {
	const (
		allowance = 40 // hub.DefaultPolicy.CommandBurst
		rate      = 20 // hub.DefaultPolicy.CommandRate
		// The theoretical window is (burst-allowance)/rate seconds. It is
		// doubled here because the arithmetic assumes perfect pacing: the join
		// and resync frames come out of the same bucket, and a scheduling hiccup
		// that bunches two commands together would otherwise refuse the last
		// one — and a refusal is a real error frame, not a retryable blip.
		safety = 2
	)
	if burst <= allowance {
		return commandSpacing
	}
	window := time.Duration(float64(burst-allowance) / rate * safety * float64(time.Second))
	// The window is spread over the whole burst, and never below the floor.
	if paced := window / time.Duration(burst); paced > commandSpacing {
		return paced
	}
	return commandSpacing
}

// burstRefill is how long the bucket needs to hand back what one burst spent.
// A burst of N commands at the pacing floor empties the bucket outright — the
// join and the resync come out of the same allowance — so the wait is the time
// to refill the whole allowance, plus a margin. Waiting less is not enough: a
// round that starts on an empty bucket is a wall of refusals, and a refusal is
// a real error frame that fails the drill.
func burstRefill(burst int) time.Duration {
	const (
		allowance = 40
		rate      = 20
	)
	refill := allowance
	if burst > refill {
		refill = burst
	}
	return time.Duration(float64(refill) / rate * 1.2 * float64(time.Second)) // 20% margin
}

// fleetBot is one bot inside an N-bot run: its socket, its stream, and the
// accounting the run reports. The reader goroutine owns the socket and the
// stream; every field the run's goroutine reads is mutex-guarded, because a
// stream read from two goroutines is a data race the race-built gate exists to
// catch.
type fleetBot struct {
	spec botSpec
	conn *websocket.Conn

	// paused is D-136's stall: the reader stops pulling frames off the socket,
	// so the server's writer blocks on the tiny receive buffer and the FIFO
	// behind it overflows. stopping is the run's own teardown, which must not
	// be reported as a stream failure.
	paused   atomic.Bool
	stopping atomic.Bool

	mu     sync.Mutex
	s      *stream
	frames int
	bytes  int64 // payload bytes this bot actually pulled off the socket
	err    error
	at     []time.Time // when each command was sent (the measured rate)
}

// dialFleet connects one bot and starts validating its stream from seq 1. A
// stalled bot dials through the small receive buffer; a rapid-fire run validates
// with the repair-tolerant stream, and the smoke shape with the strict one.
func dialFleet(target string, spec botSpec, rapid, stalled bool) (*fleetBot, error) {
	dialer := websocket.DefaultDialer
	if stalled {
		dialer = &websocket.Dialer{NetDial: constrainedLink}
	}

	conn, resp, err := dialer.Dial(target, nil)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("dial %s as %s: %v (%s)", target, spec.nickname, err, status)
	}

	s := newStream()
	if rapid {
		s = newRapidStream()
	}
	b := &fleetBot{spec: spec, conn: conn, s: s}
	go b.read()
	return b, nil
}

// read validates every frame this bot receives until the socket ends or the
// stream breaks. It is the ONLY reader of this socket — which is exactly what
// gets paused to make the stall real.
func (b *fleetBot) read() {
	for {
		if b.stopping.Load() {
			return
		}
		if b.paused.Load() {
			time.Sleep(2 * time.Millisecond)
			continue
		}

		_ = b.conn.SetReadDeadline(time.Now().Add(ioTimeout))
		_, raw, err := b.conn.ReadMessage()
		if err != nil {
			if b.stopping.Load() {
				return
			}
			b.fail(fmt.Errorf("%s: reading: %w", b.spec.nickname, err))
			return
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			b.fail(fmt.Errorf("%s: frame is not JSON: %v (%s)", b.spec.nickname, err, raw))
			return
		}

		b.mu.Lock()
		observeErr := b.s.observe(env)
		b.frames++
		b.bytes += int64(len(raw))
		b.mu.Unlock()
		if observeErr != nil {
			b.fail(fmt.Errorf("%s: %w", b.spec.nickname, observeErr))
			return
		}
	}
}

// fail keeps the FIRST error, so a run reports the symptom that started the
// cascade instead of the cascade.
func (b *fleetBot) fail(err error) {
	b.mu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.mu.Unlock()
}

// close ends the reader without recording an error: a socket ending is how every
// one of these bots ends.
func (b *fleetBot) close() {
	b.stopping.Store(true)
	_ = b.conn.Close()
}

// send writes one command frame. Only this bot's own goroutine writes to its
// socket — gorilla allows exactly one concurrent writer.
func (b *fleetBot) send(frame string) error {
	_ = b.conn.SetWriteDeadline(time.Now().Add(ioTimeout))
	if err := b.conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		return fmt.Errorf("%s: sending: %w", b.spec.nickname, err)
	}
	return nil
}

// noteCommand records one command's send instant for the measured rate.
func (b *fleetBot) noteCommand() {
	b.mu.Lock()
	b.at = append(b.at, time.Now())
	b.mu.Unlock()
}

// join is the D-28 handshake: role player plus the run-scoped nickname.
func (b *fleetBot) join() error {
	frame, err := json.Marshal(map[string]string{"type": "join", "role": "player", "nickname": b.spec.nickname})
	if err != nil {
		return fmt.Errorf("%s: marshaling the join: %w", b.spec.nickname, err)
	}
	return b.send(string(frame))
}

// state copies this bot's observable stream state under the lock.
func (b *fleetBot) state() (frames, snapshots int, lastRev int64, gaps, repairs int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.frames, b.s.snapshots, b.s.lastRev, b.s.gaps, b.s.repairs, b.err
}

// awaitComplete waits until every peer has appeared in one of this bot's
// snapshots. The budget is the pool-scaled frame bound (D-135) plus a wall-clock
// ceiling, so a server that stops talking fails the run instead of hanging it.
func (b *fleetBot) awaitComplete(budget int) error {
	deadline := time.Now().Add(ioTimeout)
	for {
		b.mu.Lock()
		done := b.s.complete(b.spec.peers)
		frames, err := b.frames, b.err
		b.mu.Unlock()

		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if frames >= budget {
			return fmt.Errorf("%s: no snapshot showed every player (%s) in %d frames — a peer never became visible",
				b.spec.nickname, strings.Join(b.spec.peers, ", "), frames)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: the room was not fully visible within %s (%d frames read)",
				b.spec.nickname, ioTimeout, frames)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// awaitSnapshotAfter waits until this bot's stream has consumed a snapshot past
// `before`. It is the fleet's drain barrier: a bot's resync reply is a snapshot,
// and it cannot arrive before that bot's OWN commands were processed — so N
// replies prove the whole burst is done, whatever order the sockets raced in.
func (b *fleetBot) awaitSnapshotAfter(before int) error {
	deadline := time.Now().Add(ioTimeout)
	for {
		b.mu.Lock()
		now, err := b.s.snapshots, b.err
		b.mu.Unlock()

		if err != nil {
			return err
		}
		if now > before {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: no snapshot arrived within %s — the burst never drained", b.spec.nickname, ioTimeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// awaitRevision waits until this bot's stream has reached the given revision.
func (b *fleetBot) awaitRevision(want int64) error {
	deadline := time.Now().Add(burstTimeout)
	for {
		b.mu.Lock()
		rev, err := b.s.lastRev, b.err
		b.mu.Unlock()

		if err != nil {
			return err
		}
		if rev >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: never reached the room's revision %d within %s (last state rev %d)",
				b.spec.nickname, want, burstTimeout, rev)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// startFleetItem drives the admin path the round already owns — upsert, then
// start twice (D-65) — so the fleet's bids are real mutations rather than
// private refusals that generate no traffic. The returned connection is kept
// open for the run's duration: an admin that disconnected mid-drill would commit
// a presence mutation the convergence assertion cannot attribute.
//
// The item's min_increment is deliberately small (1000): the burst must be able
// to climb the ladder for the whole burst, which is what produces the fan-out
// volume that makes the stalled bot's overflow real (D-136).
func startFleetItem(target, run, adminKey string) (*roundBot, error) {
	if adminKey == "" {
		return nil, fmt.Errorf("no admin capability — set ADMIN_KEY (or pass --admin-key) to the key the server printed at boot")
	}

	admin, err := dialRound(target, "fleet admin", "admin", "", adminKey)
	if err != nil {
		return nil, err
	}

	item := fmt.Sprintf(`{"name":%q,"auction_type":"english","truth_value":350000,"start_price":100000,`+
		`"min_increment":1000,"duration_ms":300000,"anti_snipe_mode":"disabled"}`, run+" fleet lot")
	if err := admin.send(`{"type":"admin.item.upsert","item":` + item + `}`); err != nil {
		admin.close()
		return nil, err
	}
	itemID, err := admin.awaitItemID(roundIO)
	if err != nil {
		admin.close()
		return nil, fmt.Errorf("admin.item.upsert: %w", err)
	}

	if err := admin.start(itemID); err != nil {
		admin.close()
		return nil, err
	}
	if _, err := admin.awaitTypes(roundIO, "phase.changed", "item.intro", "state.snapshot"); err != nil {
		admin.close()
		return nil, fmt.Errorf("item.start → ITEM_INTRO: %w", err)
	}
	if err := admin.start(itemID); err != nil {
		admin.close()
		return nil, err
	}
	if _, err := admin.awaitTypes(roundIO, "phase.changed", "item.live", "state.snapshot", "price.tick"); err != nil {
		admin.close()
		return nil, fmt.Errorf("item.start → LIVE: %w", err)
	}
	return admin, nil
}

// runFleet is D-135's twenty-client drill against a live server, and D-136's
// wire half of the stalled-client proof. The order is the contract:
//
//  1. An admin bot starts one LIVE item, so a bid is a real mutation.
//  2. N bots dial, join, and read until every peer is visible. A bot's own
//     handshake pair is the proof ITS join was committed.
//  3. Every bot blasts `--burst` bids. The stalled bot is not reading while
//     this happens: its 2 KiB receive buffer fills, the writer blocks, the
//     64-deep FIFO overflows, and the connection is fenced (D-41).
//  4. Every bot asks for a snapshot and waits for one. Its own reply cannot
//     arrive before its own commands were processed, so N replies prove the
//     burst is drained.
//  5. One more mutation — a sentinel join — is the room's FINAL revision, and
//     D-135's convergence assertion runs on that number.
func runFleet(target, run string, cfg fleetConfig) fleetReport {
	var report fleetReport

	admin, err := startFleetItem(target, run, *adminKeyFlag)
	if err != nil {
		report.fail("%v", err)
		return report
	}
	defer admin.close()
	report.ok("the fleet's item is LIVE (min_increment 1000, so the burst can climb the ladder)")

	nicknames := make([]string, cfg.bots)
	for i := range nicknames {
		nicknames[i] = fmt.Sprintf("%s-%02d", run, i)
	}

	bots := make([]*fleetBot, cfg.bots)
	defer func() {
		for _, b := range bots {
			if b != nil {
				b.close()
			}
		}
	}()
	for i := range bots {
		spec := botSpec{index: i, nickname: nicknames[i], peers: nicknames}
		b, err := dialFleet(target, spec, cfg.rapid, i == cfg.stalled)
		if err != nil {
			report.fail("%v", err)
			return report
		}
		bots[i] = b
	}

	for _, b := range bots {
		if err := b.join(); err != nil {
			report.fail("%v", err)
			return report
		}
	}
	budget := frameBudget(cfg.bots)
	for _, b := range bots {
		if err := b.awaitComplete(budget); err != nil {
			report.fail("%v", err)
			return report
		}
	}
	report.ok("%d bots joined and saw every peer within %d frames", cfg.bots, budget)

	if !cfg.rapid {
		report.converge(target, run, bots)
		return report
	}

	// The burst is paced so it stays inside D-138's bucket: every command is
	// then legitimate traffic working the fence, not a rate_limited refusal.
	spacing := burstSpacing(cfg.burst)
	report.ok("the burst is paced at %s/command across %d commands per bot (inside D-138's 40-burst, 20/s bucket)",
		spacing.Round(100*time.Microsecond), cfg.burst)

	// The stall: the reader stops pulling frames off this socket for the whole
	// burst, so the fence and its repair are exercised for real (D-41/D-136).
	if cfg.stalled >= 0 {
		bots[cfg.stalled].paused.Store(true)
		report.ok("bot %02d is not reading across %d burst round(s): the fence and its repair are being exercised",
			cfg.stalled, cfg.rounds)
	}

	var wg sync.WaitGroup
	for round := 0; round < cfg.rounds; round++ {
		// Between rounds the bucket is allowed to refill, so the next round is
		// another burst of legitimate commands rather than a wall of refusals.
		// The stalled bot stays paused: the point of a second round is to keep
		// pushing fan-out at a socket that is provably not draining, until its
		// kernel buffers and the hub's 64-frame FIFO are both full.
		if round > 0 {
			time.Sleep(burstRefill(cfg.burst))
		}
		for i, b := range bots {
			wg.Add(1)
			go func(i int, b *fleetBot, round int) {
				defer wg.Done()
				for c := 0; c < cfg.burst; c++ {
					// The ladder continues across rounds: a repeated rung
					// would be refused as below the standing bid.
					rung := round*cfg.burst + c
					amount := int64(100_000 + (rung*cfg.bots+i+1)*1_000)
					if err := b.send(fmt.Sprintf(`{"type":"bid.place","amount":%d}`, amount)); err != nil {
						b.fail(err)
						return
					}
					b.noteCommand()
					if c < cfg.burst-1 {
						time.Sleep(spacing)
					}
				}
			}(i, b, round)
		}
		wg.Wait()
		if cfg.rounds > 1 {
			bots[0].mu.Lock()
			rev := bots[0].s.lastRev
			bots[0].mu.Unlock()
			report.ok("round %d/%d done: the room is at rev %d", round+1, cfg.rounds, rev)
		}
	}

	if cfg.stalled >= 0 {
		bots[cfg.stalled].paused.Store(false)
	}

	// The drain barrier. Each bot asks for a snapshot and waits for one; its own
	// reply is queued behind its own commands, so once all N replies are in, the
	// burst is provably done and the next mutation really is the last one.
	before := make([]int, len(bots))
	for i, b := range bots {
		b.mu.Lock()
		before[i] = b.s.snapshots
		b.mu.Unlock()
		if err := b.send(`{"type":"resync"}`); err != nil {
			report.fail("%v", err)
			return report
		}
	}
	for i, b := range bots {
		if err := b.awaitSnapshotAfter(before[i]); err != nil {
			report.fail("%v", err)
			return report
		}
	}
	report.ok("every bot's burst drained: each resync reply arrived after that bot's own commands")

	report.converge(target, run, bots)

	// D-136: the stalled bot must show POSITIVE repair evidence. A drill that
	// never stalled proves nothing about the mechanism, so it fails the run.
	if cfg.stalled >= 0 {
		frames, _, lastRev, gaps, repairs, err := bots[cfg.stalled].state()
		if err != nil {
			report.fail("%v", err)
		}
		if gaps == 0 || repairs == 0 {
			report.fail("bot %02d never showed a gap repaired by a snapshot (%d gaps, %d repairs in %d frames): "+
				"a drill that never stalls proves nothing (D-136)", cfg.stalled, gaps, repairs, frames)
		} else {
			report.ok("bot %02d stalled and was repaired: %d gap(s), %d recovery snapshot(s), at rev %d",
				cfg.stalled, gaps, repairs, lastRev)
		}
	}

	// The evidence D-138's bucket sizing is judged from: what each bot actually
	// asked of the limiter. Nothing below is a t.Log — this output IS the
	// acceptance record.
	for i, b := range bots {
		b.mu.Lock()
		commands, rate, frames, rev := len(b.at), peakCommandRate(b.at), b.frames, b.s.lastRev
		absorbed := b.bytes
		b.mu.Unlock()
		tag := ""
		if i == cfg.stalled {
			tag = " (stalled)"
		}
		report.ok("bot %02d%s: %d commands, peak %g cmd/s, %d frames (%d bytes), last state rev %d", i, tag, commands, rate, frames, absorbed, rev)
	}
	return report
}

// converge is D-135's single strongest assertion: one more mutation — a
// sentinel join, committed after the burst is provably drained — is the room's
// FINAL revision, and every bot must observe it. converged() then fails the run
// if any bot's last state revision differs, naming the ones that lagged.
func (r *fleetReport) converge(target, run string, bots []*fleetBot) {
	sentinel, err := dialRound(target, "sentinel", "player", run+"-sentinel", "")
	if err != nil {
		r.fail("the settling mutation: %v", err)
		return
	}
	defer sentinel.close()

	final := sentinel.s.lastRev
	if final <= 0 {
		r.fail("the sentinel join reported no revision (%d)", final)
		return
	}

	for _, b := range bots {
		if err := b.awaitRevision(final); err != nil {
			r.fail("%v", err)
		}
	}

	revs := make([]int64, len(bots))
	for i, b := range bots {
		_, _, rev, _, _, err := b.state()
		if err != nil {
			r.fail("%v", err)
		}
		revs[i] = rev
	}
	if err := converged(revs, final); err != nil {
		r.fail("%v", err)
		return
	}
	r.ok("all %d bots converged on the room's final revision %d", len(bots), final)
}

// runFleetMain is main()'s N-bot arm: validate the configuration, run the
// drill, print the per-bot evidence, and return the process's exit code.
func runFleetMain(target, run string, cfg fleetConfig, start time.Time) int {
	switch {
	case cfg.bots < 1:
		fmt.Printf("loadtest: --bots must be at least 1 (got %d)\n", cfg.bots)
		return 2
	case cfg.burst < 1:
		fmt.Printf("loadtest: --burst must be at least 1 (got %d)\n", cfg.burst)
		return 2
	case cfg.rounds < 1:
		fmt.Printf("loadtest: --rounds must be at least 1 (got %d)\n", cfg.rounds)
		return 2
	case cfg.stalled >= cfg.bots:
		fmt.Printf("loadtest: --stalled-index %d is outside the pool (0..%d)\n", cfg.stalled, cfg.bots-1)
		return 2
	case cfg.stalled >= 0 && !cfg.rapid:
		fmt.Printf("loadtest: --stalled-index needs --rapid-fire — the stall happens during the burst\n")
		return 2
	}

	report := runFleet(target, run, cfg)
	for _, line := range report.lines {
		fmt.Printf("fleet: %s\n", line)
	}
	for _, problem := range report.errs {
		fmt.Printf("fleet: FAILED — %s\n", problem)
	}

	mode := "join-only"
	if cfg.rapid {
		mode = fmt.Sprintf("rapid-fire burst=%d rounds=%d stalled=%d", cfg.burst, cfg.rounds, cfg.stalled)
	}
	verdict, code := "converged", 0
	if !report.passed() {
		verdict, code = "FAILED", 1
	}
	fmt.Printf("loadtest: %s — %d bots (%s), the fleet %s, in %s\n",
		target, cfg.bots, mode, verdict, time.Since(start).Round(time.Millisecond))
	return code
}
