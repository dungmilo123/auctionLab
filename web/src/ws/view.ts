/**
 * The client's view reducer (RESEARCH Q3/Q4, D-70).
 *
 * One pure `state + decoded frame → state | null` function, in the gate.ts
 * style: no DOM, no socket, no clock, so Vitest's node environment pins every
 * rule. `null` means "this frame changes nothing for this view" — the socket
 * never forwards it to `onView`, so an incompatible frame cannot re-render or
 * corrupt what is on screen.
 *
 * The rules, stated once:
 *
 *   - A `state.snapshot` REPLACES the whole view, so a resync heals with no
 *     special path and no merge step (Q3).
 *   - Phase-3 events PATCH it, and only in a state where their body would be
 *     valid: `item.intro` in ITEM_INTRO, `item.live` in LIVE, `item.settled`
 *     while the view is still on that live item (the settle's own snapshot is
 *     what moves the phase — Q3: `renderBody` switches on the phase the
 *     snapshot seeds and `phase.changed` keeps current).
 *   - The LIVE overlay is discarded when the phase leaves LIVE or the item id
 *     changes, and reseeded from the new item's own live fields (UI-SPEC §LIVE).
 *   - `me.budget` moves ONLY through a snapshot (Q4): the settle's snapshot
 *     always follows, and no client arithmetic decides what a player can spend.
 *   - D-88's public feed is seeded whole by a screen snapshot and, while the
 *     phase is LIVE, prepended by the same `bid.accepted` frame that moves the
 *     price — a bid produces no snapshot, so the live path cannot wait for one.
 */

import type {
  AdminConfig,
  AdminItem,
  AdminPlayer,
  AdminSnapshot,
  BidAccepted,
  FinaleReveal,
  LeaderEntry,
  LobbyEntry,
  Phase,
  PhaseChanged,
  PlayerMe,
  PlayerSnapshot,
  PriceTick,
  PublicItem,
  RecentBid,
  ScreenSnapshot,
  SealedProgress,
} from './protocol'
import { isPhase, itemMatchesPhase } from './protocol'

/** The LIVE overlay (D-60/D-67): the values the bid controls are rendered from. */
export type LiveOverlay = {
  itemId: string
  currentPrice: number
  minNextBid: number
  deadlineAt: number | null
  topBidder: string | null
  /** the server clock this overlay was last anchored to (D-60) */
  serverTs: number
  /**
   * Phase 5 (D-94): the Dutch price clock's anchor, carried from the item's own
   * live field and re-anchored by every frame that repeats it. null for an
   * English run, whose anchor is `deadlineAt`/`serverTs` instead.
   */
  startedAt: number | null
}

/** Everything one route renders, reduced from snapshots and events. */
export type ViewState = {
  phase: Phase
  /** the player's own row; null for the admin and screen roles (D-25) */
  me: PlayerMe | null
  lobby: LobbyEntry[]
  leaderboard: LeaderEntry[]
  item: PublicItem | null
  /** the admin catalog; empty for the player and screen roles */
  items: AdminItem[]
  live: LiveOverlay | null
  /**
   * The URL the QR is built from (D-84/D-85): the screen's `join_url`, or the
   * admin's `selected_join_url`. Empty for the player role, which never
   * receives one.
   */
  joinUrl: string
  /**
   * D-88's public feed, newest first. A screen snapshot seeds it whole and an
   * accepted bid prepends to it in place (capped at `MAX_RECENT_BIDS`); only the
   * screen renders it, and the player/admin snapshots reset it to empty.
   */
  recentBids: RecentBid[]
  /** D-88's count-only sealed indicator; null = not a sealed item / not the screen. */
  sealedProgress: SealedProgress | null
  /** the presenter's persisted config (D-81); null for every other role */
  config: AdminConfig | null
  /** the presenter's token-free player rows (D-80); empty for every other role */
  players: AdminPlayer[]
  /** the presenter's join-URL candidates (D-83); empty for every other role */
  joinUrls: string[]
}

/** The item events that carry the current item. */
export type ItemEventType = 'item.intro' | 'item.live' | 'item.settled'

/** One decoded frame, ready to reduce. `ts` is the envelope's server clock. */
export type ViewFrame =
  | { kind: 'player-snapshot'; snapshot: PlayerSnapshot }
  | { kind: 'admin-snapshot'; snapshot: AdminSnapshot }
  | { kind: 'screen-snapshot'; snapshot: ScreenSnapshot }
  | { kind: 'phase.changed'; payload: PhaseChanged; ts: number }
  | { kind: 'item'; type: ItemEventType; item: PublicItem; ts: number }
  | { kind: 'price.tick'; tick: PriceTick; ts: number }
  | { kind: 'bid.accepted'; bid: BidAccepted; ts: number }
  | { kind: 'leaderboard'; entries: LeaderEntry[]; ts: number }
  | { kind: 'finale'; payload: FinaleReveal; ts: number }

/**
 * The view before a single snapshot has arrived. Nothing is invented here: a
 * player who has not joined renders the join form from this state, not a body.
 */
export function initialViewState(): ViewState {
  return {
    phase: 'LOBBY',
    me: null,
    lobby: [],
    leaderboard: [],
    item: null,
    items: [],
    live: null,
    joinUrl: '',
    recentBids: [],
    sealedProgress: null,
    config: null,
    players: [],
    joinUrls: [],
  }
}

/**
 * The phase an item event belongs to. `item.settled` is the odd one: the server
 * has already committed SETTLED when it builds the frame, but the client's view
 * is still LIVE until the settle's own snapshot arrives in the same fan-out —
 * so LIVE is the state this event is compatible with, and the phase moves with
 * the snapshot, never with the event (Q3).
 */
const ITEM_EVENT_PHASE: Readonly<Record<ItemEventType, Phase>> = {
  'item.intro': 'ITEM_INTRO',
  'item.live': 'LIVE',
  'item.settled': 'LIVE',
}

/**
 * The phase whose §Phase-body totality rules an item event's PAYLOAD must
 * satisfy — `itemMatchesPhase`, the very predicate a snapshot is admitted
 * through. It differs from `ITEM_EVENT_PHASE` only for `item.settled`, whose
 * payload is already the settled item the reveal belongs to while the view that
 * receives it is still LIVE.
 *
 * Without this check, "an event can never patch the view into a body a snapshot
 * would have been rejected for" was an assumption: Phase 5 is the first phase
 * whose body DERIVES a number from the payload (the Dutch price clock), and an
 * `item.live` frame carrying only some of those fields would pin the phone's
 * price and its ACCEPT predicate on a number the server is not charging
 * (WR-03). The decoder has already dropped everything it can prove invalid —
 * this is the phase-aware half only the reducer can decide.
 */
const ITEM_EVENT_PAYLOAD_PHASE: Readonly<Record<ItemEventType, Phase>> = {
  'item.intro': 'ITEM_INTRO',
  'item.live': 'LIVE',
  'item.settled': 'SETTLED',
}

/**
 * D-88's feed cap, in the one place the client can enforce it: the server sends
 * at most five entries, and every local prepend is trimmed to the same five so
 * the projector's feed is bounded by construction rather than by the server's
 * list happening to be short.
 */
const MAX_RECENT_BIDS = 5

/**
 * Build the LIVE overlay from a projection's own live fields, or null when it
 * carries none — a non-English item with no timing renders its pending-type
 * body instead (H13), and there is nothing for a bid control to read.
 *
 * Phase 5 (D-94): a Dutch run has no ladder, so the gate admits it on its own
 * clock instead — `startedAt` where an English item has `minNextBid`. The
 * ladder minimum then reads as 0 (the wire's own "no ladder" value: the frozen
 * `min_next_bid` field of a Dutch tick carries its zero because it has no
 * `omitempty`), and nothing on the Dutch body reads it.
 */
function overlayFrom(item: PublicItem | null, serverTs: number): LiveOverlay | null {
  // Only a LIVE item has an overlay: a settled or intro item carries no window
  // to render, and the phase-level rule ("discarded when the phase leaves LIVE")
  // is then structural here as well.
  if (item === null || item.status !== 'live' || item.currentPrice === null) return null
  if (item.minNextBid === null && item.startedAt === null) return null
  return {
    itemId: item.id,
    currentPrice: item.currentPrice,
    minNextBid: item.minNextBid ?? 0,
    deadlineAt: item.deadlineAt,
    topBidder: item.topBidder,
    serverTs,
    startedAt: item.startedAt,
  }
}

/**
 * The overlay after a state-bearing frame: kept while the phase is LIVE and the
 * item has not changed (a tick or an accepted bid patches it in place), reseeded
 * from an item that just entered LIVE, discarded the moment the phase leaves LIVE
 * or the item id moves.
 */
function transitionOverlay(
  prev: LiveOverlay | null,
  phase: Phase,
  item: PublicItem | null,
  ts: number,
): LiveOverlay | null {
  if (phase !== 'LIVE') return null
  if (prev !== null && item !== null && prev.itemId === item.id) return prev
  return overlayFrom(item, ts)
}

/**
 * Reduce one decoded frame. Returns the next view, or `null` when the frame is
 * not applicable to the current state — an invalid or incompatible frame never
 * reaches the UI (T-03-09).
 */
export function reduceView(state: ViewState, frame: ViewFrame): ViewState | null {
  switch (frame.kind) {
    case 'player-snapshot': {
      const snapshot = frame.snapshot
      // Every role-specific extra resets to its empty value: a snapshot that
      // does not carry a field (a player snapshot has no join_url, no feed and
      // no sealed counter) means "this role does not have one", never "keep the
      // previous role's".
      return {
        ...initialViewState(),
        phase: snapshot.phase,
        me: snapshot.me,
        lobby: snapshot.lobby,
        leaderboard: snapshot.leaderboard,
        item: snapshot.item,
        live: overlayFrom(snapshot.item, snapshot.serverTs),
      }
    }

    case 'admin-snapshot': {
      const snapshot = frame.snapshot
      return {
        phase: snapshot.phase,
        me: null,
        lobby: snapshot.lobby,
        leaderboard: snapshot.leaderboard,
        item: snapshot.item,
        items: snapshot.items,
        live: overlayFrom(snapshot.item, snapshot.serverTs),
        // The QR is built from the SELECTED url, whichever role supplied it —
        // the admin's `selected_join_url` is the same fact as the screen's
        // `join_url` (D-84/D-85).
        joinUrl: snapshot.selectedJoinUrl,
        recentBids: [],
        sealedProgress: null,
        config: snapshot.config,
        players: snapshot.players,
        joinUrls: snapshot.joinUrls,
      }
    }

    case 'phase.changed': {
      const { phase, item } = frame.payload
      // Defence in depth: `readPhaseChanged` already refuses a phase outside
      // the frozen five, but "only compatible state is applied" is this
      // function's own contract — the body switch downstream is total over
      // exactly those five names, so nothing else may be admitted here.
      if (!isPhase(phase)) return null
      return {
        ...state,
        phase,
        item,
        live: transitionOverlay(state.live, phase, item, frame.ts),
      }
    }

    case 'item': {
      if (state.phase !== ITEM_EVENT_PHASE[frame.type]) return null
      if (state.item === null || state.item.id !== frame.item.id) return null
      // The payload goes through the SAME totality predicate a snapshot does:
      // the body this event is about to patch renders from fields the payload
      // must therefore carry (Phase 5's Dutch arm derives its price from them),
      // and a snapshot missing any of them would have been refused outright.
      if (!itemMatchesPhase(ITEM_EVENT_PAYLOAD_PHASE[frame.type], frame.item)) return null
      return {
        ...state,
        item: frame.item,
        live: transitionOverlay(state.live, state.phase, frame.item, frame.ts),
      }
    }

    case 'price.tick': {
      const live = state.live
      if (state.phase !== 'LIVE' || live === null || live.itemId !== frame.tick.itemId) return null
      return {
        ...state,
        live: {
          ...live,
          currentPrice: frame.tick.currentPrice,
          minNextBid: frame.tick.minNextBid,
          deadlineAt: frame.tick.deadlineAt ?? live.deadlineAt,
          // D-94: a beat that carries the Dutch anchor re-anchors the falling
          // price; one that omits it keeps the anchor the body already holds, so
          // the price neither blanks nor jumps backwards (05-UI-SPEC §partial).
          startedAt: frame.tick.startedAt ?? live.startedAt,
          serverTs: frame.ts,
        },
      }
    }

    case 'bid.accepted': {
      const live = state.live
      if (state.phase !== 'LIVE' || live === null) return null
      const bid = frame.bid
      const me = state.me
      return {
        ...state,
        live: {
          ...live,
          currentPrice: bid.currentPrice,
          minNextBid: bid.minNextBid,
          deadlineAt: bid.deadlineAt ?? live.deadlineAt,
          topBidder: bid.nickname,
          serverTs: frame.ts,
        },
        // D-88's feed is patched by the SAME frame that moves the price: a bid
        // produces no snapshot, so without this the projector shows a live price
        // beside a feed frozen at the last snapshot (UI-review finding 1). The
        // row's `ts` is the envelope's server clock — the instant the server's
        // own snapshot feed stamps — and the newest row is always index 0.
        recentBids: [
          { nickname: bid.nickname, amount: bid.amount, ts: frame.ts },
          ...state.recentBids,
        ].slice(0, MAX_RECENT_BIDS),
        // The bidder is the new top by definition; anybody else's bid clears my
        // `is_top_bidder` without touching my `last_bid` — which is exactly the
        // pair the D-72 banner is derived from (UI-SPEC §LIVE).
        me:
          me === null
            ? null
            : {
                ...me,
                lastBid: bid.playerId === me.id ? bid.amount : me.lastBid,
                isTopBidder: bid.playerId === me.id,
              },
      }
    }

    case 'leaderboard':
      return { ...state, leaderboard: frame.entries }

    case 'screen-snapshot': {
      const snapshot = frame.snapshot
      return {
        ...initialViewState(),
        phase: snapshot.phase,
        me: null,
        lobby: snapshot.lobby,
        leaderboard: snapshot.leaderboard,
        item: snapshot.item,
        joinUrl: snapshot.joinUrl,
        recentBids: snapshot.recentBids,
        sealedProgress: snapshot.sealedProgress,
        live: overlayFrom(snapshot.item, snapshot.serverTs),
      }
    }

    case 'finale':
      // D-89's ceremony frame patches the standings in the server's own order —
      // the podium is a PREFIX of them, so the two can never disagree — and the
      // trailing snapshot (D-41's resync anchor) remains the healing source that
      // replaces the whole view, standings included.
      return { ...state, leaderboard: frame.payload.standings }
  }
}
