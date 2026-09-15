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
  auctionType: string
  startPrice: number
  status: string
}

/** The `state.snapshot` payload a player receives (D-25): self-sufficient. */
export type PlayerSnapshot = {
  phase: string
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
 * Decode the player `state.snapshot` payload (D-25).
 *
 * A payload with a missing or ill-typed required field decodes to null rather
 * than to a half-populated lobby: an unapplied snapshot leaves the last known
 * good state on screen, which is exactly what UI-SPEC requires when the
 * connection is lost or superseded.
 */
export function readPlayerSnapshot(data: unknown): PlayerSnapshot | null {
  if (!isRecord(data)) return null

  const phase = readString(data.phase)
  const revision = readInt(data.revision)
  const serverTs = readInt(data.server_ts)
  const me = readPlayerMe(data.me)
  const lobby = readLobby(data.lobby)
  if (phase === null || revision === null || serverTs === null || me === null || lobby === null) {
    return null
  }

  return {
    phase,
    revision,
    serverTs,
    me,
    lobby,
    leaderboard: readLeaderboard(data.leaderboard),
    item: readPublicItem(data.item),
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
  return { id, nickname, budget, totalValue, connected }
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
function readLeaderboard(value: unknown): LeaderEntry[] {
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

function readPublicItem(value: unknown): PublicItem | null {
  if (!isRecord(value)) return null
  const id = readString(value.id)
  const name = readString(value.name)
  const auctionType = readString(value.auction_type)
  const startPrice = readInt(value.start_price)
  const status = readString(value.status)
  if (id === null || name === null || auctionType === null || startPrice === null || status === null) {
    return null
  }
  return { id, name, auctionType, startPrice, status }
}
