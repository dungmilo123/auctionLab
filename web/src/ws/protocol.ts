/**
 * The browser half of the frozen wire protocol (D-21/D-24).
 *
 * Every name and key here mirrors `internal/hub/protocol.go` and the role
 * projections in `internal/game/project.go` exactly. The protocol is frozen at
 * the end of Phase 2: later phases add message types and optional fields, and
 * never rename, repurpose or remove one — a rename on either side of the wire
 * has to fail in `protocol.test.ts`, not in front of a classroom.
 *
 * `seq` is THIS connection's stream position (1 at hello) and `rev` is the game
 * revision. They are two numbers on purpose: one mutation legitimately produces
 * several messages, and presence/tick messages carry the current rev without
 * moving it. Dedupe on `seq` only, and never discard a message because its `rev`
 * looks familiar (research Pitfall 3).
 */

// `isSealedAuction` is the app's ONE definition of the two sealed types (the
// player body branch, the screen arm, the admin gate and this decoder's LIVE
// totality arm all read it — UI-SPEC §Requirement Impact 7). The import is one
// way at runtime: `lib/screen.ts` imports only TYPES from this module, which
// `verbatimModuleSyntax` erases.
import { isSealedAuction } from '@/lib/screen'

/** The five frozen envelope keys — the contract, not the field names. */
export const ENVELOPE_FIELDS = ['type', 'seq', 'rev', 'ts', 'data'] as const

/** One server→client frame (D-21). */
export type Envelope = {
  type: string
  /** per-connection stream sequence, 1 at hello */
  seq: number
  /** game revision: +1 per successful persisted mutation (D-35) */
  rev: number
  /** server clock, milliseconds */
  ts: number
  /** the message payload */
  data: unknown
}

/** One connection role (D-23); chosen once, in the join. */
export type Role = 'player' | 'screen' | 'admin'

export const ROLES: readonly Role[] = ['player', 'screen', 'admin']

/**
 * The complete client→server catalog (D-24). Commands whose behaviour lands in
 * a later phase are answered `unsupported` today, so a client can bind to the
 * names now.
 */
export const CLIENT_MESSAGE_TYPES = [
  'join',
  'resync',
  'bid.place',
  'dutch.accept',
  'sealed.submit',
  'admin.item.upsert',
  'admin.item.delete',
  'admin.item.reorder',
  'admin.item.start',
  'admin.item.force_end',
  'admin.item.next',
  'admin.game.reset',
  'admin.config.update',
  'admin.player.release',
] as const

/** The complete server→client catalog (D-24). */
export const SERVER_MESSAGE_TYPES = [
  'hello',
  'state.snapshot',
  'phase.changed',
  'item.intro',
  'item.live',
  'price.tick',
  'bid.accepted',
  'bid.rejected',
  'sealed.received',
  'item.settled',
  'leaderboard',
  'finale',
  'tunnel.status',
  // Phase 7 (D-132): the application-level heartbeat, appended inside the
  // server→client group beside the other control-plane frame — the same
  // deliberate two-list edit internal/hub/protocol.go made.
  'heartbeat',
  'error',
] as const

/** The frozen rejection codes; `not_authorized` is wired in Phase 4 (D-24). */
export const ERROR_CODES = [
  'nickname_invalid',
  'nickname_taken',
  'token_invalid',
  'not_joined',
  'bad_message',
  'unsupported',
  'not_authorized',
  // Phase 7 (D-138): the inbound command bucket's informational refusal. It
  // never closes the socket and never moves state; the UI renders nothing for
  // it, so it must not be mapped to a player-actionable error row.
  'rate_limited',
] as const

export type ErrorCode = (typeof ERROR_CODES)[number]

/** Message-type literals the client routes on today. */
export const HELLO = 'hello'
export const STATE_SNAPSHOT = 'state.snapshot'
export const ERROR = 'error'
/** Control plane: applied regardless of seq, never gated on rev (D-24). */
export const TUNNEL_STATUS = 'tunnel.status'
/**
 * The application-level liveness beat (D-132) — the second control-plane frame,
 * applied at any seq and never through the revision gate. It is the only
 * message a quiet LOBBY/INTRO/SETTLED stream carries, so the client's staleness
 * detector depends on it; its payload is `{ts}` and nothing else.
 */
export const HEARTBEAT = 'heartbeat'
/** The Phase-3 state events the view reducer consumes (D-57). */
export const PHASE_CHANGED = 'phase.changed'
export const ITEM_INTRO = 'item.intro'
export const ITEM_LIVE = 'item.live'
export const ITEM_SETTLED = 'item.settled'
export const PRICE_TICK = 'price.tick'
export const BID_ACCEPTED = 'bid.accepted'
export const BID_REJECTED = 'bid.rejected'
export const LEADERBOARD = 'leaderboard'
/** D-89's reserved ceremony frame: the podium and the complete standings. */
export const FINALE = 'finale'

/** `error` payload (D-24). `message` is for logs — the UI renders `code`. */
export type ErrorPayload = {
  code: string
  message: string
}

/** `hello` payload (D-23). The only frame that ever carries a token, and only */
/** the token of the player it is addressed to (D-26/D-29). */
export type Hello = {
  role: Role
  playerId: string
  nickname: string
  token: string
  rev: number
}

/** The player's own row in the lobby snapshot. */
export type PlayerMe = {
  id: string
  nickname: string
  budget: number
  totalValue: number
  connected: boolean
  /** this player's most recent accepted bid in the CURRENT run, or null (D-67) */
  lastBid: number | null
  /** server-computed so the outbid banner never has to be (D-72) */
  isTopBidder: boolean
  /**
   * D-89/ECON-05: the player's 1-based place in the ONE server standing, or 0
   * for "no standing" — the finale body omits the block rather than printing
   * "rank 0". Server-computed for the same reason `isTopBidder` is: a public
   * LeaderEntry carries no id to match a nickname against (research Pitfall 1).
   */
  rank: number
  /**
   * Phase 6 (D-115, additive): THIS player's own effective sealed amount while
   * the current item is a sealed run that is still LIVE, or null. The key is
   * `my_bid` — never a `sealed_bid*` spelling, which D-26's leak lock rejects at
   * any depth — and an absent key is the wire's "not submitted". Another
   * player's amount exists nowhere in a player payload (D-118).
   */
  myBid: number | null
}

/** One lobby row: a nickname and its live presence. No budgets — public (D-25). */
export type LobbyEntry = {
  nickname: string
  connected: boolean
}

/** One public standing. */
export type LeaderEntry = {
  rank: number
  nickname: string
  totalValue: number
  budget: number
}

/** The public item parameters (null until an item is current — Phase 3). */
export type PublicItem = {
  id: string
  name: string
  description: string
  auctionType: string
  hintMin: number | null
  hintMax: number | null
  startPrice: number
  status: string
  /** live fields, present while the run is open (D-67) */
  currentPrice: number | null
  topBidder: string | null
  deadlineAt: number | null
  minNextBid: number | null
  extensionsUsed: number | null
  /**
   * Phase 5 (D-94, additive): the Dutch price clock — the instant the server's
   * clock started plus the three mechanics a client re-derives the falling
   * number from. They are emitted ONLY for a `dutch` item, so an English item's
   * key set is byte-identical to Phase 4's; a Dutch item has no `min_next_bid`
   * and no `deadline_at` at all (D-93/D-98).
   */
  startedAt: number | null
  floorPrice: number | null
  decrementStep: number | null
  decrementIntervalMs: number | null
  /** present ONLY at SETTLED — the leak boundary is the object itself (D-67/D-68) */
  settle: SettleInfo | null
}

/** The `state.snapshot` payload a player receives (D-25): self-sufficient. */
export type PlayerSnapshot = {
  phase: Phase
  revision: number
  serverTs: number
  me: PlayerMe
  lobby: LobbyEntry[]
  leaderboard: LeaderEntry[]
  item: PublicItem | null
  /**
   * Phase 6 (D-116, additive): the SAME count-only sealed counter the projector
   * and the presenter carry — the one sealed fact that crosses people, and an
   * explicit null for every non-sealed item (the server always marshals the key).
   */
  sealedProgress: SealedProgress | null
}

/**
 * The one rejection code the client acts on by itself: D-30's third rung —
 * an unknown token with no nickname — clears the stored token, so the next
 * attempt is a fresh join instead of a loop of rejected resumes.
 */
export const CODE_TOKEN_INVALID = 'token_invalid'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readString(value: unknown): string | null {
  return typeof value === 'string' ? value : null
}

function readInt(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function readRole(value: unknown): Role | null {
  return value === 'player' || value === 'screen' || value === 'admin' ? value : null
}

function parseJSON(raw: string): unknown {
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

/**
 * Decode one frame off the socket. Returns null for anything that is not a
 * well-formed envelope, so no malformed input can reach the reconciliation
 * gate or the UI (T-02-01). Unknown extra keys are kept in `data`, never
 * rejected: D-24's freeze is additive.
 */
export function parseEnvelope(raw: unknown): Envelope | null {
  const value = typeof raw === 'string' ? parseJSON(raw) : raw
  if (!isRecord(value)) return null

  const type = readString(value.type)
  const seq = readInt(value.seq)
  const rev = readInt(value.rev)
  const ts = readInt(value.ts)
  if (type === null || type === '' || seq === null || rev === null || ts === null) return null

  return { type, seq, rev, ts, data: value.data ?? null }
}

/** Decode a `hello` payload. */
export function readHello(data: unknown): Hello | null {
  if (!isRecord(data)) return null
  const role = readRole(data.role)
  const rev = readInt(data.rev)
  if (role === null || rev === null) return null

  return {
    role,
    playerId: readString(data.player) ?? '',
    nickname: readString(data.nickname) ?? '',
    token: readString(data.token) ?? '',
    rev,
  }
}

/** Decode an `error` payload. */
export function readError(data: unknown): ErrorPayload | null {
  if (!isRecord(data)) return null
  const code = readString(data.code)
  if (code === null || code === '') return null
  return { code, message: readString(data.message) ?? '' }
}

/**
 * `tunnel.status` payload (D-123): the two display facts the presenter's Join
 * URLs panel reads. The URL is a string and `healthy` is a boolean; the empty
 * URL with `healthy:false` is a DECLARED state (tunnel off or restarting), not
 * a missing one.
 */
export type TunnelStatus = {
  url: string
  healthy: boolean
}

/**
 * Decode a `tunnel.status` control frame.
 *
 * Display-only data, so this is deliberately NOT a totality rule: a malformed
 * frame returns null and is ignored by the caller, exactly like a malformed
 * `recent_bids` entry — a tunnel read-out must never reject a snapshot or
 * freeze the panel it decorates (UI-SPEC §Requirement Impact 4).
 */
export function readTunnelStatus(data: unknown): TunnelStatus | null {
  if (!isRecord(data)) return null
  const url = readString(data.url)
  const healthy = data.healthy
  if (url === null || typeof healthy !== 'boolean') return null
  return { url, healthy }
}

/**
 * Decode the player `state.snapshot` payload (D-25/D-63).
 *
 * A payload with a missing or ill-typed required field decodes to null rather
 * than to a half-populated lobby: an unapplied snapshot leaves the last known
 * good state on screen, which is exactly what UI-SPEC requires when the
 * connection is lost or superseded.
 *
 * Phase 3 (UI-SPEC §Phase-body totality): the phase↔item invariant is enforced
 * HERE, so the body switch downstream is total — an admitted snapshot always
 * carries what its body renders, and a payload that breaks the table leaves the
 * previous body on screen instead of rendering blank.
 */
export function readPlayerSnapshot(data: unknown): PlayerSnapshot | null {
  if (!isRecord(data)) return null

  const phase = readPhase(data.phase)
  const revision = readInt(data.revision)
  const serverTs = readInt(data.server_ts)
  const me = readPlayerMe(data.me)
  const lobby = readLobby(data.lobby)
  if (phase === null || revision === null || serverTs === null || me === null || lobby === null) {
    return null
  }

  const item = readPublicItem(data.item)
  if (!itemMatchesPhase(phase, item)) return null

  return {
    phase,
    revision,
    serverTs,
    me,
    lobby,
    leaderboard: readLeaderboard(data.leaderboard),
    item,
    // Phase 6 (D-116): the player's own copy of the count-only counter, decoded
    // with the projector's shipped helper — one reader for one wire shape. An
    // explicit null outside a sealed item; a malformed counter reads as absent
    // (this field is never authoritative for money).
    sealedProgress: readSealedProgress(data.sealed_progress),
  }
}

function readPlayerMe(value: unknown): PlayerMe | null {
  if (!isRecord(value)) return null
  const id = readString(value.id)
  const nickname = readString(value.nickname)
  const budget = readInt(value.budget)
  const totalValue = readInt(value.total_value)
  const connected = typeof value.connected === 'boolean' ? value.connected : null
  if (id === null || nickname === null || budget === null || totalValue === null || connected === null) {
    return null
  }
  // `last_bid`/`is_top_bidder` are Phase 3's additive keys (D-67): a row that
  // predates them reads as "never bid", never as a rejection. `rank` (D-89) is
  // the same shape of additive key.
  const rank = readRank(value.rank)
  if (rank === null) return null
  return {
    id,
    nickname,
    budget,
    totalValue,
    connected,
    lastBid: readInt(value.last_bid),
    isTopBidder: value.is_top_bidder === true,
    rank,
    // Phase 6 (D-115): the same additive shape — an absent `my_bid` is "not
    // submitted" (the `last_bid` precedent), and the server emits the key only
    // for the owner of a sealed run that is still open.
    myBid: readInt(value.my_bid),
  }
}

/**
 * Read `me.rank` (D-89/ECON-05). The key is always present on the Phase-4 wire
 * as a plain int, so a row that predates it reads as 0 — "no standing", the
 * value the finale body omits the block for — exactly the way a missing
 * `last_bid` reads as "never bid". A rank that IS present but is not a
 * non-negative integer (a string, a negative, a fraction, a null, a
 * non-finite) is a malformed projection, so the whole `me` row is rejected
 * rather than silently reporting "unranked" for a ranked player.
 */
function readRank(value: unknown): number | null {
  if (value === undefined) return 0
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 0) return null
  return value
}

function readLobby(value: unknown): LobbyEntry[] | null {
  if (!Array.isArray(value)) return null
  const entries: LobbyEntry[] = []
  for (const raw of value) {
    if (!isRecord(raw)) return null
    const nickname = readString(raw.nickname)
    const connected = typeof raw.connected === 'boolean' ? raw.connected : null
    if (nickname === null || connected === null) return null
    entries.push({ nickname, connected })
  }
  return entries
}

// The standings and the current item are not rendered by the Phase 2 lobby,
// but they are part of the self-sufficient snapshot (D-25), so they are decoded
// the moment they arrive rather than re-derived from events later.
export function readLeaderboard(value: unknown): LeaderEntry[] {
  if (!Array.isArray(value)) return []
  const entries: LeaderEntry[] = []
  for (const raw of value) {
    if (!isRecord(raw)) continue
    const rank = readInt(raw.rank)
    const nickname = readString(raw.nickname)
    const totalValue = readInt(raw.total_value)
    const budget = readInt(raw.budget)
    if (rank === null || nickname === null || totalValue === null || budget === null) continue
    entries.push({ rank, nickname, totalValue, budget })
  }
  return entries
}

/**
 * Decode one public item payload (the `item.intro` / `item.live` / `item.settled`
 * frames and the `item` key of a snapshot). Shape only: which phases a given
 * item may appear in is the caller's rule, because the caller is the one that
 * knows the phase (`itemMatchesPhase` for snapshots, the event type for events).
 */
export function readPublicItem(value: unknown): PublicItem | null {
  if (!isRecord(value)) return null
  const id = readString(value.id)
  const name = readString(value.name)
  const auctionType = readString(value.auction_type)
  const startPrice = readInt(value.start_price)
  const status = readString(value.status)
  if (id === null || name === null || auctionType === null || startPrice === null || status === null) {
    return null
  }
  return {
    id,
    name,
    description: readString(value.description) ?? '',
    auctionType,
    hintMin: readInt(value.hint_min),
    hintMax: readInt(value.hint_max),
    startPrice,
    status,
    currentPrice: readInt(value.current_price),
    topBidder: readBidder(value.top_bidder),
    deadlineAt: readInt(value.deadline_at),
    minNextBid: readInt(value.min_next_bid),
    extensionsUsed: readInt(value.extensions_used),
    // Phase 5 (D-94): the Dutch price clock. `readInt` maps an absent key to
    // null — an English item simply has none — while a present-but-ill-typed
    // value lands as null too, which the Dutch arm below then refuses (T-05-01).
    startedAt: readInt(value.started_at),
    floorPrice: readInt(value.floor_price),
    decrementStep: readInt(value.decrement_step),
    decrementIntervalMs: readInt(value.decrement_interval_ms),
    settle: readSettle(value.settle),
  }
}

/** The public identity the wire nests under `top_bidder`/`settle.winner`. */
function readBidder(value: unknown): string | null {
  if (!isRecord(value)) return null
  return readString(value.nickname)
}

/**
 * Decode the D-67 reveal. The object is internally consistent or it is not a
 * reveal at all: `sold` names its winner, `unsold` carries an explicit null —
 * the SETTLED body omits the winner row on exactly the second case (UI-SPEC
 * §Screen E), so admitting a sold item with no winner would render a hole.
 */
function readSettle(value: unknown): SettleInfo | null {
  if (!isRecord(value)) return null
  const outcome = readString(value.outcome)
  const price = readInt(value.price)
  const truthValue = readInt(value.truth_value)
  const budgetDelta = readInt(value.budget_delta)
  if (outcome !== 'sold' && outcome !== 'unsold') return null
  if (price === null || truthValue === null || budgetDelta === null) return null

  const winner = readBidder(value.winner)
  if (outcome === 'sold' && winner === null) return null
  if (outcome === 'unsold' && winner !== null) return null
  return { outcome, winner, price, truthValue, budgetDelta }
}

/** One of the five frozen phase names, or null. */
function readPhase(value: unknown): Phase | null {
  return isPhase(value) ? value : null
}

/**
 * The UI-SPEC §Phase-body totality table, as one predicate: an admitted
 * snapshot's phase always carries what its body renders. `settle` may exist
 * only at SETTLED — a settle object on any other phase is the leak boundary
 * itself (D-67), so it is rejected rather than ignored.
 *
 * Exported because `reduceView` applies the SAME predicate to every `item.*`
 * event's payload, so an event can never patch the view into a body a snapshot
 * would have been rejected for (WR-03). A caller that knows the phase whose
 * totality an item must satisfy is the only kind of caller this has.
 */
export function itemMatchesPhase(phase: Phase, item: PublicItem | null): boolean {
  switch (phase) {
    case 'LOBBY':
    case 'FINALE':
      return item === null || item.settle === null
    case 'ITEM_INTRO':
      return item !== null && item.status === 'intro' && item.settle === null
    case 'LIVE':
      if (item === null || item.status !== 'live' || item.settle !== null) return false
      // Phase 5 (D-92/D-93/D-94): a Dutch LIVE item renders from a price clock
      // the snapshot itself must carry — the derived price plus the anchor and
      // the three mechanics the client re-derives from — and it may not carry a
      // deadline it can never have (a Dutch run is deadline-less by type, D-93).
      // A snapshot that breaks that totality is refused here rather than
      // painting a body with holes in it (T-05-01).
      if (item.auctionType === 'dutch') {
        return (
          item.currentPrice !== null &&
          item.floorPrice !== null &&
          item.decrementStep !== null &&
          item.decrementIntervalMs !== null &&
          item.startedAt !== null &&
          item.deadlineAt === null
        )
      }
      // H13: an english LIVE item must carry the numbers its bid controls
      // render. Phase 6 (D-116) narrows this for the two sealed types: the sealed
      // LIVE body renders its close countdown from `deadline_at`, so an admitted
      // sealed item must carry one — while `current_price` and `min_next_bid`
      // stay optional, because a sealed run has no ladder to describe. H13's
      // leniency now survives only for a type this build does not know at all
      // (which renders `live.typePending` rather than freezing the screen).
      if (isSealedAuction(item.auctionType)) return item.deadlineAt !== null
      if (item.auctionType !== 'english') return true
      return item.currentPrice !== null && item.minNextBid !== null && item.deadlineAt !== null
    case 'SETTLED':
      return item !== null && item.status === 'settled' && item.settle !== null
  }
}

// ---------------------------------------------------------------------------
// Phase 3 (D-54/D-56/D-57/D-64/D-67/D-68). The wire payloads are declared by
// `internal/game/project.go`; every reader below maps that JSON onto the
// camelCase the UI renders, and every one of them refuses an inconsistent
// payload rather than half-populating state (T-03-09).
// ---------------------------------------------------------------------------

/** The five PLAN §6 phases, in order. The body switch is total over exactly these. */
export const PHASES = ['LOBBY', 'ITEM_INTRO', 'LIVE', 'SETTLED', 'FINALE'] as const

export type Phase = (typeof PHASES)[number]

/** True for one of the five frozen phase names. */
export function isPhase(value: unknown): value is Phase {
  return typeof value === 'string' && (PHASES as readonly string[]).includes(value)
}

/** The D-67 reveal, present ONLY at SETTLED — the leak boundary is the object. */
export type SettleInfo = {
  outcome: string
  winner: string | null
  price: number
  truthValue: number
  budgetDelta: number
}

/** The D-56 timing frame. */
export type PriceTick = {
  itemId: string
  currentPrice: number
  minNextBid: number
  deadlineAt: number | null
  remainingMs: number | null
  /**
   * Phase 5 (D-94, additive): the Dutch clock's anchor. A Dutch beat carries it
   * so every tick re-anchors the falling price; an English beat has no such key
   * (the deadline pair is its anchor), and a tick that omits it must NOT clear
   * the anchor the body already holds (05-UI-SPEC §partial).
   */
  startedAt: number | null
}

/** The D-54 broadcast. */
export type BidAccepted = {
  playerId: string
  nickname: string
  amount: number
  currentPrice: number
  minNextBid: number
  deadlineAt: number | null
  extensionsUsed: number
}

/** The D-53 private refusal. */
export type BidRejected = {
  reason: string
  minNextBid: number | null
  budget: number
  amount: number
}

/** The D-57 `phase.changed` payload. */
export type PhaseChanged = {
  phase: Phase
  item: PublicItem | null
}

/** One admin catalog row (D-66). */
export type AdminItem = {
  id: string
  orderIndex: number
  name: string
  description: string
  imageUrl: string
  auctionType: string
  truthValue: number
  hintMin: number | null
  hintMax: number | null
  startPrice: number
  minIncrement: number | null
  floorPrice: number | null
  decrementStep: number | null
  decrementIntervalMs: number | null
  durationMs: number | null
  antiSnipeMode: string
  antiSnipeWindowMs: number | null
  maxExtensions: number | null
  status: string
}

/** The persisted configuration the presenter's Setup tab edits (D-81). */
export type AdminConfig = {
  startingBudget: number
  currency: string
}

/**
 * One admin player row — the Players tab's row (D-80). The token is
 * deliberately NOT a field: the server sends one, and a typed model that cannot
 * express a token cannot leak one (T-04-04). `nickname_key` is left out for the
 * same reason — the panel renders neither.
 */
export type AdminPlayer = {
  id: string
  nickname: string
  budget: number
  totalValue: number
  joinedAt: number
  lastSeenAt: number
  connected: boolean
}

/**
 * One effective sealed bid (D-117) — the presenter's private ledger row, and the
 * ONE client model that expresses a sealed amount. It is decoded only from the
 * admin snapshot; the server scopes the rows to the current item, sends them in
 * arrival order (`submitted_at` ASC → player id ASC, the tie-break's own fields)
 * and carries the nickname (the `recent_bids` precedent), so the browser never
 * filters, sorts, joins or counts. `updatedAt` is the one time the card renders
 * — only an edit moves it — while `submittedAt` is the tie-break truth and is
 * deliberately not displayed (VID-04 is v2).
 */
export type AdminSealedBid = {
  itemId: string
  playerId: string
  nickname: string
  amount: number
  submittedAt: number
  updatedAt: number
}

/** The admin projection the `/admin` page renders. */
export type AdminSnapshot = {
  phase: Phase
  revision: number
  serverTs: number
  /** D-81: the persisted config the Setup tab edits */
  config: AdminConfig
  /** D-80: the named, token-free player rows */
  players: AdminPlayer[]
  items: AdminItem[]
  /** D-117: the current item's effective sealed rows, in arrival order */
  sealedBids: AdminSealedBid[]
  /** D-117: the server's own sealed count — never `rows.length` recomputed here */
  sealedProgress: SealedProgress | null
  item: PublicItem | null
  lobby: LobbyEntry[]
  leaderboard: LeaderEntry[]
  /** D-83: the boot-time join-URL candidates, in rank order — never an absence */
  joinUrls: string[]
  /** D-84: the URL every QR is built from */
  selectedJoinUrl: string
}

/**
 * Decode a `price.tick` payload (D-56). The three numbers the frame exists for
 * are required; the deadline pair is optional because a non-English run may
 * carry no deadline at all (H13). `remaining_ms` is decoded for completeness
 * and never used as the countdown's authority — the client interpolates from
 * `(deadline_at, ts)` (D-60/D-61).
 */
export function readPriceTick(data: unknown): PriceTick | null {
  if (!isRecord(data)) return null
  const itemId = readString(data.item_id)
  const currentPrice = readInt(data.current_price)
  const minNextBid = readInt(data.min_next_bid)
  if (itemId === null || itemId === '' || currentPrice === null || minNextBid === null) return null
  return {
    itemId,
    currentPrice,
    minNextBid,
    deadlineAt: readInt(data.deadline_at),
    remainingMs: readInt(data.remaining_ms),
    // Phase 5 (D-94): a Dutch beat's re-anchor. The key is optional, so an
    // English beat (or a Dutch frame that omits it) reads as null — and the
    // reducer keeps the anchor it holds rather than clearing it.
    startedAt: readInt(data.started_at),
  }
}

/** Decode a `bid.accepted` payload (D-54). */
export function readBidAccepted(data: unknown): BidAccepted | null {
  if (!isRecord(data)) return null
  const playerId = readString(data.player_id)
  const nickname = readString(data.nickname)
  const amount = readInt(data.amount)
  const currentPrice = readInt(data.current_price)
  const minNextBid = readInt(data.min_next_bid)
  const extensionsUsed = readInt(data.extensions_used)
  if (
    playerId === null ||
    nickname === null ||
    amount === null ||
    currentPrice === null ||
    minNextBid === null ||
    extensionsUsed === null
  ) {
    return null
  }
  return {
    playerId,
    nickname,
    amount,
    currentPrice,
    minNextBid,
    extensionsUsed,
    deadlineAt: readInt(data.deadline_at),
  }
}

/** Decode a `bid.rejected` payload (D-53). */
export function readBidRejected(data: unknown): BidRejected | null {
  if (!isRecord(data)) return null
  const reason = readString(data.reason)
  const budget = readInt(data.budget)
  const amount = readInt(data.amount)
  if (reason === null || reason === '' || budget === null || amount === null) return null
  return { reason, minNextBid: readInt(data.min_next_bid), budget, amount }
}

/**
 * Decode a `phase.changed` payload (D-57): the new phase plus the item that
 * comes with it — an explicit null in LOBBY/FINALE. The phase↔item invariant
 * applies here too, so an event can never patch the view into a body a snapshot
 * would have been rejected for.
 */
export function readPhaseChanged(data: unknown): PhaseChanged | null {
  if (!isRecord(data)) return null
  const phase = readPhase(data.phase)
  if (phase === null) return null
  const item = readPublicItem(data.item)
  if (!itemMatchesPhase(phase, item)) return null
  return { phase, item }
}

/**
 * Decode the admin `state.snapshot` (D-64/D-66/D-80/D-81/D-83): the catalog the
 * presenter edits, their own row set, the persisted config, the two halves of
 * the Join URLs panel, the current item and the shared public lists.
 *
 * Phase 6 (D-117) decodes the sealed ledger here — a DELIBERATE reversal of the
 * Phase-3/4 claim that "a typed model that cannot express a sealed amount cannot
 * leak one". The guarantee moved from this model to the wire: the role
 * projections never send an amount to a player or the projector, and D-118's
 * leak suite (`internal/hub/leak_wire_test.go`, over a POPULATED sealed window)
 * is the authority. Decoding is as strict as `players`/`items`: ONE ill-typed
 * row rejects the whole snapshot, and an absent (or explicit null) list reads as
 * "no rows" rather than as a rejection.
 */
export function readAdminSnapshot(data: unknown): AdminSnapshot | null {
  if (!isRecord(data)) return null
  const phase = readPhase(data.phase)
  const revision = readInt(data.revision)
  const serverTs = readInt(data.server_ts)
  const items = readAdminItems(data.items)
  const lobby = readLobby(data.lobby)
  const config = readAdminConfig(data.config)
  const players = readAdminPlayers(data.players)
  const sealedBids = readAdminSealedBids(data.sealed_bids)
  const joinUrls = readJoinUrls(data.join_urls)
  const selectedJoinUrl = readString(data.selected_join_url)
  if (
    phase === null ||
    revision === null ||
    serverTs === null ||
    items === null ||
    lobby === null ||
    config === null ||
    players === null ||
    sealedBids === null ||
    joinUrls === null ||
    selectedJoinUrl === null
  ) {
    return null
  }
  const item = readPublicItem(data.item)
  if (!itemMatchesPhase(phase, item)) return null
  return {
    phase,
    revision,
    serverTs,
    items,
    sealedBids,
    sealedProgress: readSealedProgress(data.sealed_progress),
    item,
    lobby,
    leaderboard: readLeaderboard(data.leaderboard),
    config,
    players,
    joinUrls,
    selectedJoinUrl,
  }
}

/**
 * Decode the presenter's sealed ledger (D-117). The rows are the server's own —
 * scoped to the current item, in arrival order, nickname included — and this
 * reader neither reorders nor filters them. ONE ill-typed row rejects the
 * snapshot (the `players`/`items` rule): the card renders the list as a whole,
 * and a half-decoded ledger is one the presenter cannot trust. An absent or null
 * key is "this build sends no rows" — the additive-key rule D-24 fixes.
 */
function readAdminSealedBids(value: unknown): AdminSealedBid[] | null {
  if (value === undefined || value === null) return []
  if (!Array.isArray(value)) return null
  const rows: AdminSealedBid[] = []
  for (const raw of value) {
    const row = readAdminSealedBid(raw)
    if (row === null) return null
    rows.push(row)
  }
  return rows
}

function readAdminSealedBid(value: unknown): AdminSealedBid | null {
  if (!isRecord(value)) return null
  const itemId = readString(value.item_id)
  const playerId = readString(value.player_id)
  const nickname = readString(value.nickname)
  const amount = readInt(value.amount)
  const submittedAt = readInt(value.submitted_at)
  const updatedAt = readInt(value.updated_at)
  if (
    itemId === null ||
    playerId === null ||
    nickname === null ||
    amount === null ||
    submittedAt === null ||
    updatedAt === null
  ) {
    return null
  }
  return { itemId, playerId, nickname, amount, submittedAt, updatedAt }
}

/** Decode the persisted config the Setup tab edits (D-81). */
function readAdminConfig(value: unknown): AdminConfig | null {
  if (!isRecord(value)) return null
  const startingBudget = readInt(value.starting_budget)
  const currency = readString(value.currency)
  if (startingBudget === null || currency === null) return null
  return { startingBudget, currency }
}

/**
 * Decode the presenter's player rows, narrow by construction: `token` and
 * `nickname_key` are on the wire and are simply not fields of this model, so
 * the Players tab cannot render, copy or spread a secret it never received
 * (T-04-04). ONE ill-typed row rejects the snapshot, the same rule the catalog
 * follows: the panel renders the list as a whole, and the server's rows are
 * always well-formed.
 */
function readAdminPlayers(value: unknown): AdminPlayer[] | null {
  if (!Array.isArray(value)) return null
  const players: AdminPlayer[] = []
  for (const raw of value) {
    const player = readAdminPlayer(raw)
    if (player === null) return null
    players.push(player)
  }
  return players
}

function readAdminPlayer(value: unknown): AdminPlayer | null {
  if (!isRecord(value)) return null
  const id = readString(value.id)
  const nickname = readString(value.nickname)
  const budget = readInt(value.budget)
  const totalValue = readInt(value.total_value)
  const joinedAt = readInt(value.joined_at)
  const lastSeenAt = readInt(value.last_seen_at)
  const connected = typeof value.connected === 'boolean' ? value.connected : null
  if (
    id === null ||
    nickname === null ||
    budget === null ||
    totalValue === null ||
    joinedAt === null ||
    lastSeenAt === null ||
    connected === null
  ) {
    return null
  }
  return { id, nickname, budget, totalValue, joinedAt, lastSeenAt, connected }
}

/**
 * Decode the boot-time join-URL candidates (D-83). Every entry is a string; the
 * list may be empty (the "this page's URL" fallback is a client-side row), and
 * the client never re-ranks it — the wire order is the rank order.
 */
function readJoinUrls(value: unknown): string[] | null {
  if (!Array.isArray(value)) return null
  const urls: string[] = []
  for (const raw of value) {
    const url = readString(raw)
    if (url === null) return null
    urls.push(url)
  }
  return urls
}

/**
 * Decode the catalog. ONE ill-typed row rejects the snapshot: the list is
 * rendered as a whole, so a half-decoded catalog would be a list the presenter
 * cannot trust — and the server's rows are always well-formed.
 */
function readAdminItems(value: unknown): AdminItem[] | null {
  if (!Array.isArray(value)) return null
  const items: AdminItem[] = []
  for (const raw of value) {
    const item = readAdminItem(raw)
    if (item === null) return null
    items.push(item)
  }
  return items
}

function readAdminItem(value: unknown): AdminItem | null {
  if (!isRecord(value)) return null
  const id = readString(value.id)
  const orderIndex = readInt(value.order_index)
  const name = readString(value.name)
  const auctionType = readString(value.auction_type)
  const truthValue = readInt(value.truth_value)
  const startPrice = readInt(value.start_price)
  const status = readString(value.status)
  if (
    id === null ||
    orderIndex === null ||
    name === null ||
    auctionType === null ||
    truthValue === null ||
    startPrice === null ||
    status === null
  ) {
    return null
  }
  return {
    id,
    orderIndex,
    name,
    description: readString(value.description) ?? '',
    imageUrl: readString(value.image_url) ?? '',
    auctionType,
    truthValue,
    hintMin: readInt(value.hint_min),
    hintMax: readInt(value.hint_max),
    startPrice,
    minIncrement: readInt(value.min_increment),
    floorPrice: readInt(value.floor_price),
    decrementStep: readInt(value.decrement_step),
    decrementIntervalMs: readInt(value.decrement_interval_ms),
    durationMs: readInt(value.duration_ms),
    antiSnipeMode: readString(value.anti_snipe_mode) ?? '',
    antiSnipeWindowMs: readInt(value.anti_snipe_window_ms),
    maxExtensions: readInt(value.max_extensions),
    status,
  }
}

// ---------------------------------------------------------------------------
// Phase 4 (D-87/D-88/D-89). The wire payloads are declared by
// `internal/game/project.go`; every reader below maps that JSON onto the
// camelCase the UI renders, and the projector's snapshot keeps the same
// fail-closed rule the player and admin snapshots follow.
// ---------------------------------------------------------------------------

/** One entry of D-88's public bid feed: a nickname, an amount and the bid's instant. */
export type RecentBid = {
  nickname: string
  amount: number
  ts: number
}

/**
 * D-88's count-only sealed indicator. The type has no amount field at all, so
 * "the projector may only know how many sheets arrived" is structural rather
 * than a convention.
 */
export type SealedProgress = {
  submitted: number
  total: number
}

/**
 * The `/screen` `state.snapshot` (D-87/D-88): public state only, plus the two
 * additive extras the projector renders and the ONE join URL every QR is built
 * from (D-84/D-85) — never the candidate list.
 */
export type ScreenSnapshot = {
  phase: Phase
  revision: number
  serverTs: number
  joinUrl: string
  lobby: LobbyEntry[]
  leaderboard: LeaderEntry[]
  item: PublicItem | null
  /** newest five accepted bids of the CURRENT run, newest first; never null */
  recentBids: RecentBid[]
  /** {submitted,total} for a sealed item, an explicit null for every other type */
  sealedProgress: SealedProgress | null
}

/** D-89's `finale` payload: the podium is a PREFIX of the standings. */
export type FinaleReveal = {
  podium: LeaderEntry[]
  standings: LeaderEntry[]
}

/**
 * D-88's public feed. The projector must never freeze on a convenience field
 * (UI-SPEC §Data contract): an entry with a missing or ill-typed field is
 * DROPPED and the snapshot still applies — the price, the countdown and the
 * bidder line are the authoritative numbers. An absent key is an empty feed;
 * the server always sends an array.
 */
function readRecentBids(value: unknown): RecentBid[] {
  if (!Array.isArray(value)) return []
  const entries: RecentBid[] = []
  for (const raw of value) {
    if (!isRecord(raw)) continue
    const nickname = readString(raw.nickname)
    const amount = readInt(raw.amount)
    const ts = readInt(raw.ts)
    if (nickname === null || amount === null || ts === null) continue
    entries.push({ nickname, amount, ts })
  }
  return entries
}

/**
 * D-88's sealed counter, decoded as optional-null: a record that carries both
 * counts is the counter, anything else (an explicit null, a missing or
 * ill-typed payload) is "not a sealed item" and the line simply does not
 * render. The amounts live only on the admin projection.
 */
function readSealedProgress(value: unknown): SealedProgress | null {
  if (!isRecord(value)) return null
  const submitted = readInt(value.submitted)
  const total = readInt(value.total)
  if (submitted === null || total === null) return null
  return { submitted, total }
}

/**
 * Decode the screen `state.snapshot` (D-87/D-88). Mandatory data is
 * fail-closed: the phase↔item totality table is enforced HERE, exactly as the
 * player and admin snapshots enforce it, so the projector's body switch stays
 * total — an admitted snapshot always carries what its body renders, and a
 * payload that breaks the table leaves the previous body on screen instead of
 * rendering blank (T-04-05).
 *
 * The two D-88 extras are the declared exception: a malformed feed entry is
 * dropped and a malformed counter is read as absent, because neither is
 * authoritative for the price or the reveal.
 */
export function readScreenSnapshot(data: unknown): ScreenSnapshot | null {
  if (!isRecord(data)) return null

  const phase = readPhase(data.phase)
  const revision = readInt(data.revision)
  const serverTs = readInt(data.server_ts)
  // The join URL is required and may be the empty string (the projector then
  // renders `screen.noJoinUrl`); a missing or non-string key is a payload this
  // build cannot render, so it is rejected rather than half-applied.
  const joinUrl = readString(data.join_url)
  const lobby = readLobby(data.lobby)
  if (
    phase === null ||
    revision === null ||
    serverTs === null ||
    joinUrl === null ||
    lobby === null
  ) {
    return null
  }

  const item = readPublicItem(data.item)
  if (!itemMatchesPhase(phase, item)) return null

  return {
    phase,
    revision,
    serverTs,
    joinUrl,
    lobby,
    leaderboard: readLeaderboard(data.leaderboard),
    item,
    recentBids: readRecentBids(data.recent_bids),
    sealedProgress: readSealedProgress(data.sealed_progress),
  }
}

/**
 * Decode a `finale` payload (D-89): the podium, a ≤3 prefix of the standings,
 * plus the complete standings in the server's ONE leaderboard order. Both
 * lists go through `readLeaderboard`, so there is exactly one place a standing
 * row is built and the client never re-sorts: the order on the wire is the
 * order on the projector. A payload that is not the two-list ceremony at all
 * is rejected; a malformed row inside a list is skipped the same way the
 * `leaderboard` frame skips one.
 */
export function readFinale(data: unknown): FinaleReveal | null {
  if (!isRecord(data)) return null
  if (!Array.isArray(data.podium) || !Array.isArray(data.standings)) return null
  return { podium: readLeaderboard(data.podium), standings: readLeaderboard(data.standings) }
}
