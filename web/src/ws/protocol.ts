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
] as const

export type ErrorCode = (typeof ERROR_CODES)[number]

/** Message-type literals the client routes on today. */
export const HELLO = 'hello'
export const STATE_SNAPSHOT = 'state.snapshot'
export const ERROR = 'error'
/** Control plane: applied regardless of seq, never gated on rev (D-24). */
export const TUNNEL_STATUS = 'tunnel.status'
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
  // predates them reads as "never bid", never as a rejection.
  return {
    id,
    nickname,
    budget,
    totalValue,
    connected,
    lastBid: readInt(value.last_bid),
    isTopBidder: value.is_top_bidder === true,
  }
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
 */
function itemMatchesPhase(phase: Phase, item: PublicItem | null): boolean {
  switch (phase) {
    case 'LOBBY':
    case 'FINALE':
      return item === null || item.settle === null
    case 'ITEM_INTRO':
      return item !== null && item.status === 'intro' && item.settle === null
    case 'LIVE':
      if (item === null || item.status !== 'live' || item.settle !== null) return false
      // H13: an english LIVE item must carry the numbers its bid controls
      // render; any other type may reach LIVE with fewer fields and renders
      // `live.typePending` instead of the form.
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

/** The admin projection the `/admin` page renders. */
export type AdminSnapshot = {
  phase: Phase
  revision: number
  serverTs: number
  items: AdminItem[]
  item: PublicItem | null
  lobby: LobbyEntry[]
  leaderboard: LeaderEntry[]
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
 * Decode the admin `state.snapshot` (D-64/D-66): the catalog the presenter
 * edits, the current item and the shared public lists. The player/sealed-bid
 * rows are deliberately NOT decoded — the Phase-3 panel renders neither, and a
 * typed model that cannot express a token cannot leak one (T-03-10).
 */
export function readAdminSnapshot(data: unknown): AdminSnapshot | null {
  if (!isRecord(data)) return null
  const phase = readPhase(data.phase)
  const revision = readInt(data.revision)
  const serverTs = readInt(data.server_ts)
  const items = readAdminItems(data.items)
  const lobby = readLobby(data.lobby)
  if (phase === null || revision === null || serverTs === null || items === null || lobby === null) {
    return null
  }
  const item = readPublicItem(data.item)
  if (!itemMatchesPhase(phase, item)) return null
  return {
    phase,
    revision,
    serverTs,
    items,
    item,
    lobby,
    leaderboard: readLeaderboard(data.leaderboard),
  }
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
// `internal/game/project.go`; the readers below are RED stubs until the GREEN
// commit.
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

/** Decode the screen `state.snapshot` (D-87/D-88). RED stub: nothing decodes yet. */
export function readScreenSnapshot(data: unknown): ScreenSnapshot | null {
  void data
  return null
}

/** Decode a `finale` payload (D-89). RED stub: nothing decodes yet. */
export function readFinale(data: unknown): FinaleReveal | null {
  void data
  return null
}
