/**
 * The client's view reducer (RESEARCH Q3/Q4, D-70) — RED stub.
 *
 * One pure `state + decoded frame → state | null` function, in the gate.ts
 * style: no DOM, no socket, no clock, so Vitest's node environment pins every
 * rule. A snapshot REPLACES the whole view; Phase-3 events patch it; the LIVE
 * overlay is discarded when the phase leaves LIVE or the item changes; and
 * `me.budget` moves only through a snapshot.
 *
 * The rules are implemented in the GREEN commit; this is the RED contract.
 */

import type {
  AdminItem,
  AdminSnapshot,
  BidAccepted,
  LeaderEntry,
  LobbyEntry,
  Phase,
  PhaseChanged,
  PlayerMe,
  PlayerSnapshot,
  PriceTick,
  PublicItem,
} from './protocol'

/** The LIVE overlay (D-60/D-67): the values the bid controls are rendered from. */
export type LiveOverlay = {
  itemId: string
  currentPrice: number
  minNextBid: number
  deadlineAt: number | null
  topBidder: string | null
  serverTs: number
}

/** Everything one route renders, reduced from snapshots and events. */
export type ViewState = {
  phase: Phase
  me: PlayerMe | null
  lobby: LobbyEntry[]
  leaderboard: LeaderEntry[]
  item: PublicItem | null
  items: AdminItem[]
  live: LiveOverlay | null
}

/** One decoded frame, ready to reduce. */
export type ViewFrame =
  | { kind: 'player-snapshot'; snapshot: PlayerSnapshot }
  | { kind: 'admin-snapshot'; snapshot: AdminSnapshot }
  | { kind: 'phase.changed'; payload: PhaseChanged; ts: number }
  | { kind: 'item'; type: ItemEventType; item: PublicItem; ts: number }
  | { kind: 'price.tick'; tick: PriceTick; ts: number }
  | { kind: 'bid.accepted'; bid: BidAccepted; ts: number }
  | { kind: 'leaderboard'; entries: LeaderEntry[]; ts: number }

/** The item events that carry the current item. */
export type ItemEventType = 'item.intro' | 'item.live' | 'item.settled'

/** The view before a single snapshot has arrived. */
export function initialViewState(): ViewState {
  return { phase: 'LOBBY', me: null, lobby: [], leaderboard: [], item: null, items: [], live: null }
}

/** Reduce one decoded frame. RED stub: nothing is applied yet. */
export function reduceView(state: ViewState, frame: ViewFrame): ViewState | null {
  void state
  void frame
  return null
}
