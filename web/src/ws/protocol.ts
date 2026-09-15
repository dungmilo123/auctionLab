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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readString(value: unknown): string | null {
  return typeof value === 'string' ? value : null
}

function readInt(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

/**
 * Decode one frame off the socket. Returns null for anything that is not a
 * well-formed envelope, so no malformed input can reach the reconciliation
 * gate or the UI (T-02-01).
 */
export function parseEnvelope(_raw: unknown): Envelope | null {
  // RED sentinel: the surface is declared, the decode rules are not — every
  // frame decodes to null until the GREEN commit replaces this body.
  return null
}

/** Decode a `hello` payload. */
export function readHello(_data: unknown): Hello | null {
  // RED sentinel — see parseEnvelope.
  return null
}

/** Decode an `error` payload. */
export function readError(_data: unknown): ErrorPayload | null {
  // RED sentinel — see parseEnvelope.
  return null
}

/** Decode the player `state.snapshot` payload. */
export function readPlayerSnapshot(_data: unknown): PlayerSnapshot | null {
  // RED sentinel — see parseEnvelope.
  return null
}

// Referenced by the decoders once they are implemented; kept here so the
// module's surface (and its imports) do not change between RED and GREEN.
void isRecord
void readString
void readInt
