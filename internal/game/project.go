package game

import (
	"sort"
	"time"
)

// This file is the leak boundary (D-25/D-26): the ONLY types in the package with
// JSON tags, and the only way internal state can reach a socket.
//
// The three projections are structurally separate from `state` — no embedding,
// no aliasing, no `json:"-"` field "for debugging". A type that cannot express a
// secret cannot leak one, and project_test.go walks the marshaled JSON of every
// role to keep it that way through Phase 6.
//
// Field names are the wire contract: add fields, never rename or remove one
// (D-24).

// PlayerView is what one player may see: the public game state, the public
// standings, and their own identity. It deliberately has no field for another
// player's budget, for truth_value, for sealed bids or for any token (D-25).
type PlayerView struct {
	Phase       string        `json:"phase"`
	Revision    int64         `json:"revision"`
	ServerTS    int64         `json:"server_ts"`
	Me          PlayerMe      `json:"me"`
	Lobby       []LobbyEntry  `json:"lobby"`
	Leaderboard []LeaderEntry `json:"leaderboard"`
	Item        *PublicItem   `json:"item"` // null until an item is current (Phase 3)
}

// PlayerMe is the player's own row: the values they are allowed to know about
// themselves. `connected` is live presence, which is why it rides the current
// revision (D-37).
//
// Phase 3 (D-67, additive): `last_bid` is this player's most recent accepted bid
// in the CURRENT run — a pointer so "never bid" is the key's absence while a
// real 0 would still travel — and `is_top_bidder` is computed by the server so
// the outbid banner never has to (D-72).
type PlayerMe struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	Budget      int64  `json:"budget"`
	TotalValue  int64  `json:"total_value"`
	Connected   bool   `json:"connected"`
	LastBid     *int64 `json:"last_bid,omitempty"`
	IsTopBidder bool   `json:"is_top_bidder"`
}

// LobbyEntry is one row of the lobby list: a nickname and whether that player is
// connected. No budgets — the lobby is public (D-25).
type LobbyEntry struct {
	Nickname  string `json:"nickname"`
	Connected bool   `json:"connected"`
}

// LeaderEntry is one public standing. Budget is part of the public standings
// because the finale ranks by budget (PLAN.md §6 FINALE) — the value is revealed
// to the projector, not to a player's own view of another player's row.
type LeaderEntry struct {
	Rank       int    `json:"rank"`
	Nickname   string `json:"nickname"`
	TotalValue int64  `json:"total_value"`
	Budget     int64  `json:"budget"`
}

// PublicItem is the item information a player or the projector may see: every
// public parameter and the hint range, but never truth_value (D-26).
//
// Phase 3 (D-67, additive): the live fields are pointers with `omitempty` so the
// wire states are distinguishable — `current_price`/`min_next_bid`/`deadline_at`
// exist for a live English item, and `settle` EXISTS ONLY at SETTLED. The leak
// boundary is the presence of the whole `settle` object: `truth_value` never
// travels at the top level, only inside it, and only after the reveal.
type PublicItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AuctionType string `json:"auction_type"`
	HintMin     *int64 `json:"hint_min,omitempty"`
	HintMax     *int64 `json:"hint_max,omitempty"`
	StartPrice  int64  `json:"start_price"`
	Status      string `json:"status"`

	CurrentPrice   *int64      `json:"current_price,omitempty"`
	TopBidder      *Bidder     `json:"top_bidder,omitempty"`
	DeadlineAt     *int64      `json:"deadline_at,omitempty"`
	MinNextBid     *int64      `json:"min_next_bid,omitempty"`
	ExtensionsUsed *int        `json:"extensions_used,omitempty"`
	Settle         *SettleInfo `json:"settle,omitempty"`
}

// Bidder is the public identity of the leading (or winning) player: a nickname
// and nothing else — no id, no budget (D-67).
type Bidder struct {
	Nickname string `json:"nickname"`
}

// SettleInfo is D-67's reveal, shared by every role (D-68). `winner` is an
// explicit JSON null for an unsold item — the client treats null as "no winner";
// the UI omitting the row is a render rule, not a payload rule.
type SettleInfo struct {
	Outcome     string  `json:"outcome"` // "sold" | "unsold"
	Winner      *Bidder `json:"winner"`  // null when unsold
	Price       int64   `json:"price"`
	TruthValue  int64   `json:"truth_value"`
	BudgetDelta int64   `json:"budget_delta"` // truth_value − price for a sold item, 0 for unsold
}

// BidAccepted is the D-54 broadcast: the accepted bid plus every authoritative
// number a client would otherwise have to compute (D-54/D-72 — min_next_bid in
// particular is never client-side maths).
type BidAccepted struct {
	PlayerID       string `json:"player_id"`
	Nickname       string `json:"nickname"`
	Amount         int64  `json:"amount"`
	CurrentPrice   int64  `json:"current_price"`
	MinNextBid     int64  `json:"min_next_bid"`
	DeadlineAt     *int64 `json:"deadline_at,omitempty"`
	ExtensionsUsed int    `json:"extensions_used"`
}

// BidRejection is D-53's private payload: the structured reason plus the numbers
// that make the refusal actionable. It travels to the submitting player only,
// and it never bumps the revision.
type BidRejection struct {
	Reason     string `json:"reason"`
	MinNextBid *int64 `json:"min_next_bid,omitempty"`
	Budget     int64  `json:"budget"`
	Amount     int64  `json:"amount"`
}

// PhaseChanged is the D-57 payload of `phase.changed`: the new phase plus the
// current public item — an explicit null when there is none, as in FINALE — so
// the frame alone is enough for a client to patch its view.
type PhaseChanged struct {
	Phase string      `json:"phase"`
	Item  *PublicItem `json:"item"`
}

// ScreenView is the projector's view: public state only. Before SETTLED it
// carries no truth_value, and it never carries an identity or a token (D-25).
type ScreenView struct {
	Phase       string        `json:"phase"`
	Revision    int64         `json:"revision"`
	ServerTS    int64         `json:"server_ts"`
	Lobby       []LobbyEntry  `json:"lobby"`
	Leaderboard []LeaderEntry `json:"leaderboard"`
	Item        *PublicItem   `json:"item"`
}

// AdminView is the control panel's view. Phase 2 delivers it secret-capable even
// though the ADMIN_KEY check lands in Phase 4 (D-24): writing it any other way
// would mean reshaping it later, and the leak tests need a positive control that
// proves the fixture really carries secrets (D-26).
type AdminView struct {
	Phase       string           `json:"phase"`
	Revision    int64            `json:"revision"`
	ServerTS    int64            `json:"server_ts"`
	Config      AdminConfig      `json:"config"`
	Players     []AdminPlayer    `json:"players"`
	Items       []AdminItem      `json:"items"`
	SealedBids  []AdminSealedBid `json:"sealed_bids"`
	Lobby       []LobbyEntry     `json:"lobby"`
	Leaderboard []LeaderEntry    `json:"leaderboard"`
	Item        *PublicItem      `json:"item"`
}

// AdminConfig is the persisted configuration the admin panel edits (Phase 4).
type AdminConfig struct {
	StartingBudget int64  `json:"starting_budget"`
	Currency       string `json:"currency"`
}

// AdminPlayer is the full player row, including the token — the admin panel is
// the only view allowed to see one (D-29).
type AdminPlayer struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	NicknameKey string `json:"nickname_key"`
	Token       string `json:"token"`
	Budget      int64  `json:"budget"`
	TotalValue  int64  `json:"total_value"`
	JoinedAt    int64  `json:"joined_at"`
	LastSeenAt  int64  `json:"last_seen_at"`
	Connected   bool   `json:"connected"`
}

// AdminItem is the full item row, including the truth value (D-26's positive
// control) and every mechanics field the Phase 3 editor round-trips (D-66).
type AdminItem struct {
	ID          string `json:"id"`
	OrderIndex  int64  `json:"order_index"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	AuctionType string `json:"auction_type"`
	TruthValue  int64  `json:"truth_value"`

	HintMin             *int64 `json:"hint_min,omitempty"`
	HintMax             *int64 `json:"hint_max,omitempty"`
	StartPrice          int64  `json:"start_price"`
	MinIncrement        *int64 `json:"min_increment,omitempty"`
	FloorPrice          *int64 `json:"floor_price,omitempty"`
	DecrementStep       *int64 `json:"decrement_step,omitempty"`
	DecrementIntervalMS *int64 `json:"decrement_interval_ms,omitempty"`
	DurationMS          *int64 `json:"duration_ms,omitempty"`
	AntiSnipeMode       string `json:"anti_snipe_mode,omitempty"`
	AntiSnipeWindowMS   *int64 `json:"anti_snipe_window_ms,omitempty"`
	MaxExtensions       *int64 `json:"max_extensions,omitempty"`

	Status string `json:"status"`
}

// AdminSealedBid is one effective sealed bid (PLAN.md §5). Phase 6 owns the
// flow; the projection exists now so the secret has a home that is not a player
// view.
type AdminSealedBid struct {
	ItemID      string `json:"item_id"`
	PlayerID    string `json:"player_id"`
	Amount      int64  `json:"amount"`
	SubmittedAt int64  `json:"submitted_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// playerView builds one player's projection. An id the engine does not know
// yields a view whose `me` is zero: the hub only ever asks for a registered
// player, and inventing an error here would make the fan-out path fail in the
// middle of a critical section.
func (s *state) playerView(now time.Time, playerID string) PlayerView {
	view := PlayerView{
		Phase:       string(s.phase),
		Revision:    s.revision,
		ServerTS:    now.UnixMilli(),
		Lobby:       s.lobby(),
		Leaderboard: s.leaderboard(),
		Item:        s.currentPublicItem(),
	}
	if p := s.players[playerID]; p != nil {
		view.Me = PlayerMe{
			ID:         p.id,
			Nickname:   p.nickname,
			Budget:     p.budget,
			TotalValue: p.totalValue,
			Connected:  s.connected[p.id],
		}
		// D-67/D-72: the outbid banner is derived from these two server-computed
		// values, never from client arithmetic — `last_bid` is scoped to the
		// current run, so the banner clears on the next item by construction.
		if last, ok := s.lastBidOf(p.id); ok {
			amount := last.amount
			view.Me.LastBid = &amount
		}
		if top, ok := s.topBid(); ok && top.playerID == p.id {
			view.Me.IsTopBidder = true
		}
	}
	return view
}

// screenView builds the projector's projection.
func (s *state) screenView(now time.Time) ScreenView {
	return ScreenView{
		Phase:       string(s.phase),
		Revision:    s.revision,
		ServerTS:    now.UnixMilli(),
		Lobby:       s.lobby(),
		Leaderboard: s.leaderboard(),
		Item:        s.currentPublicItem(),
	}
}

// adminView builds the control panel's projection.
func (s *state) adminView(now time.Time) AdminView {
	view := AdminView{
		Phase:       string(s.phase),
		Revision:    s.revision,
		ServerTS:    now.UnixMilli(),
		Config:      AdminConfig{StartingBudget: s.config.StartingBudget, Currency: s.config.Currency},
		Players:     make([]AdminPlayer, 0, len(s.players)),
		Items:       make([]AdminItem, 0, len(s.items)),
		SealedBids:  make([]AdminSealedBid, 0, len(s.sealedBids)),
		Lobby:       s.lobby(),
		Leaderboard: s.leaderboard(),
		Item:        s.currentPublicItem(),
	}

	for _, id := range s.order {
		p := s.players[id]
		view.Players = append(view.Players, AdminPlayer{
			ID:          p.id,
			Nickname:    p.nickname,
			NicknameKey: p.nicknameKey,
			Token:       p.token,
			Budget:      p.budget,
			TotalValue:  p.totalValue,
			JoinedAt:    p.joinedAt,
			LastSeenAt:  p.lastSeenAt,
			Connected:   s.connected[p.id],
		})
	}

	for _, id := range s.itemOrder {
		view.Items = append(view.Items, adminItem(s.items[id]))
	}
	for _, bid := range s.sealedBids {
		view.SealedBids = append(view.SealedBids, AdminSealedBid{
			ItemID:      bid.itemID,
			PlayerID:    bid.playerID,
			Amount:      bid.amount,
			SubmittedAt: bid.submittedAt,
			UpdatedAt:   bid.updatedAt,
		})
	}
	return view
}

// lobby lists every player in join order with its live presence.
func (s *state) lobby() []LobbyEntry {
	entries := make([]LobbyEntry, 0, len(s.order))
	for _, id := range s.order {
		p := s.players[id]
		entries = append(entries, LobbyEntry{Nickname: p.nickname, Connected: s.connected[id]})
	}
	return entries
}

// leaderboard is the public standing: budget first, then total value, then join
// order — the PLAN.md §5 tie-break chain, applied consistently so the lobby, the
// live dashboard and the podium never disagree.
func (s *state) leaderboard() []LeaderEntry {
	ids := append([]string(nil), s.order...)
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := s.players[ids[i]], s.players[ids[j]]
		switch {
		case a.budget != b.budget:
			return a.budget > b.budget
		case a.totalValue != b.totalValue:
			return a.totalValue > b.totalValue
		default:
			return a.joinedAt < b.joinedAt
		}
	})

	entries := make([]LeaderEntry, 0, len(ids))
	for i, id := range ids {
		p := s.players[id]
		entries = append(entries, LeaderEntry{
			Rank:       i + 1,
			Nickname:   p.nickname,
			TotalValue: p.totalValue,
			Budget:     p.budget,
		})
	}
	return entries
}

// currentPublicItem projects the current item, or nil when none is current
// (LOBBY, and between items). It is the ONE builder behind all three roles, so a
// live field is either visible to everyone or to no one by construction —
// including the settle object, which reaches `/screen` for free (D-69).
func (s *state) currentPublicItem() *PublicItem {
	if s.currentItemID == "" {
		return nil
	}
	it := s.items[s.currentItemID]
	if it == nil {
		return nil
	}
	view := &PublicItem{
		ID:          it.id,
		Name:        it.name,
		Description: it.description,
		AuctionType: it.auctionType,
		StartPrice:  it.startPrice,
		Status:      it.status,
	}
	if it.hintMax > 0 {
		min, max := it.hintMin, it.hintMax
		view.HintMin, view.HintMax = &min, &max
	}
	s.addLiveFields(view, it)
	return view
}

// addLiveFields is D-67's extension: every field is a pointer with `omitempty`,
// so the wire distinguishes "not applicable" from a real 0, and `settle` exists
// ONLY at SETTLED — the leak boundary is the presence of the whole object, not
// of any single field inside it.
func (s *state) addLiveFields(view *PublicItem, it *item) {
	run := s.currentAuction
	if run == nil || run.itemID != it.id {
		return
	}

	if price, ok := s.currentPrice(); ok {
		view.CurrentPrice = &price
	}
	if nickname, ok := s.topBidderNickname(); ok {
		view.TopBidder = &Bidder{Nickname: nickname}
	}
	if run.deadlineAt > 0 {
		deadline := run.deadlineAt
		view.DeadlineAt = &deadline
	}
	if s.phase == PhaseLive {
		// Only an open window advertises a next bid: after SETTLED there is
		// nothing to bid on, and a stale minimum would invite a bid the ladder
		// refuses (D-52 #3).
		if min, ok := s.minNextBid(); ok {
			view.MinNextBid = &min
		}
	}
	extensionsUsed := run.extensionsUsed
	view.ExtensionsUsed = &extensionsUsed

	if s.phase == PhaseSettled && run.settle != nil {
		reveal := *run.settle
		view.Settle = &reveal
	}
}

// adminItem projects the full item row, truth value included, plus every
// mechanics field the editor round-trips (D-66: `admin.item.upsert` takes the
// same object shape back).
func adminItem(it *item) AdminItem {
	view := AdminItem{
		ID:          it.id,
		OrderIndex:  it.orderIndex,
		Name:        it.name,
		Description: it.description,
		ImageURL:    it.imageURL,
		AuctionType: it.auctionType,
		TruthValue:  it.truthValue,
		StartPrice:  it.startPrice,
		Status:      it.status,
	}
	if it.hintMax > 0 {
		min, max := it.hintMin, it.hintMax
		view.HintMin, view.HintMax = &min, &max
	}
	if it.minIncrement > 0 {
		v := it.minIncrement
		view.MinIncrement = &v
	}
	if it.floorPrice > 0 {
		v := it.floorPrice
		view.FloorPrice = &v
	}
	if it.decrementStep > 0 {
		v := it.decrementStep
		view.DecrementStep = &v
	}
	if it.decrementIntervalMS > 0 {
		v := it.decrementIntervalMS
		view.DecrementIntervalMS = &v
	}
	if it.durationMS > 0 {
		v := it.durationMS
		view.DurationMS = &v
	}
	if it.antiSnipeMode != "" {
		view.AntiSnipeMode = it.antiSnipeMode
	}
	if it.antiSnipeWindowMS > 0 {
		v := it.antiSnipeWindowMS
		view.AntiSnipeWindowMS = &v
	}
	if it.maxExtensions > 0 {
		v := it.maxExtensions
		view.MaxExtensions = &v
	}
	return view
}
