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
type PlayerMe struct {
	ID         string `json:"id"`
	Nickname   string `json:"nickname"`
	Budget     int64  `json:"budget"`
	TotalValue int64  `json:"total_value"`
	Connected  bool   `json:"connected"`
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
type PublicItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AuctionType string `json:"auction_type"`
	HintMin     *int64 `json:"hint_min,omitempty"`
	HintMax     *int64 `json:"hint_max,omitempty"`
	StartPrice  int64  `json:"start_price"`
	Status      string `json:"status"`
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
// control).
type AdminItem struct {
	ID          string `json:"id"`
	OrderIndex  int64  `json:"order_index"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AuctionType string `json:"auction_type"`
	TruthValue  int64  `json:"truth_value"`
	HintMin     *int64 `json:"hint_min,omitempty"`
	HintMax     *int64 `json:"hint_max,omitempty"`
	StartPrice  int64  `json:"start_price"`
	Status      string `json:"status"`
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
// (LOBBY, and between items).
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
	return view
}

// adminItem projects the full item row, truth value included.
func adminItem(it *item) AdminItem {
	view := AdminItem{
		ID:          it.id,
		OrderIndex:  it.orderIndex,
		Name:        it.name,
		Description: it.description,
		AuctionType: it.auctionType,
		TruthValue:  it.truthValue,
		StartPrice:  it.startPrice,
		Status:      it.status,
	}
	if it.hintMax > 0 {
		min, max := it.hintMin, it.hintMax
		view.HintMin, view.HintMax = &min, &max
	}
	return view
}
