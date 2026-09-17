/**
 * The admin page's pure logic (D-79/D-81/D-82).
 *
 * Everything here is a selector: no DOM, no React, no socket, no clock. The
 * page renders what these return, and Vitest's node environment pins the rules
 * — which is the only way this phase can test the page at all (there is no DOM
 * harness; `vitest` runs `environment: node`).
 *
 * Two carried principles shape the file:
 *
 *   - The SERVER is the single source of truth (contract #1). Nothing here
 *     carries optimistic state: `validateDraft` mirrors D-66's own validation so
 *     the presenter has a field to fix, and `urlCandidates` only decides which
 *     rows to OFFER — the selected one still comes back in a snapshot (D-84).
 *   - A control that cannot deliver must not look available (D-82a). Every
 *     mutating control's `disabled` prop is `!mutationsEnabled(status)` or an
 *     in-flight flag derived from the snapshot it was sent against.
 */

import type { AdminCommand, AdminConfigInput, ResetMode } from '@/ws/admin'
import type { ConnectionStatus } from '@/ws/client'
import type { AdminItem, AdminPlayer, Phase, PublicItem } from '@/ws/protocol'
import { parseBidAmount } from './bid'
import { formatVND } from './format'
import type { CopyKey } from './i18n'
import { isSealedAuction } from './screen'

export const MAX_ITEM_NAME_RUNES = 80

/** The budget field's DOM id — the Setup card's only new field. */
export const STARTING_BUDGET_FIELD_ID = 'admin-field-starting-budget'

/** The editor's field ids: the DOM id each draft field renders under. */
export const FIELD_IDS = {
  name: 'admin-field-name',
  truthValue: 'admin-field-truth-value',
  hintMin: 'admin-field-hint-min',
  hintMax: 'admin-field-hint-max',
  startPrice: 'admin-field-start-price',
  minIncrement: 'admin-field-min-increment',
  durationMs: 'admin-field-duration-ms',
  antiSnipeWindowMs: 'admin-field-anti-snipe-window-ms',
  maxExtensions: 'admin-field-max-extensions',
  floorPrice: 'admin-field-floor-price',
  decrementStep: 'admin-field-decrement-step',
  decrementIntervalMs: 'admin-field-decrement-interval-ms',
} as const

export type NumberField = Exclude<keyof typeof FIELD_IDS, 'name'>

/** The whole editor draft: strings as typed, plus the fields the form never shows. */
export type DraftFields = {
  name: string
  auctionType: string
  antiSnipeMode: string
  truthValue: string
  hintMin: string
  hintMax: string
  startPrice: string
  minIncrement: string
  durationMs: string
  antiSnipeWindowMs: string
  maxExtensions: string
  floorPrice: string
  decrementStep: string
  decrementIntervalMs: string
}

/**
 * The editor's own object. `description` and `imageUrl` are NOT fields in Phase
 * 3 (CNT-01 is v2): they are carried so the form echoes the loaded item's values
 * back unchanged, and they are empty strings on create.
 */
export type EditorDraft = {
  /** '' means "create with a server-minted id" (H14) */
  id: string
  description: string
  imageUrl: string
  fields: DraftFields
}

export type DraftErrors = Partial<Record<keyof DraftFields, CopyKey>>

/** Which fields the matrix shows for this type — and therefore which are validated. */
export function visibleFields(
  type: string,
  antiSnipeMode: string,
): Record<keyof DraftFields, boolean> {
  const english = type === 'english'
  const dutch = type === 'dutch'
  const timed = english || type === 'sealed_first' || type === 'vickrey'
  return {
    name: true,
    auctionType: true,
    antiSnipeMode: english,
    truthValue: true,
    hintMin: true,
    hintMax: true,
    startPrice: true,
    minIncrement: english,
    durationMs: timed,
    antiSnipeWindowMs: english && antiSnipeMode === 'extend',
    maxExtensions: english,
    floorPrice: dutch,
    decrementStep: dutch,
    decrementIntervalMs: dutch,
  }
}

/**
 * A required field is one the server's own validation would refuse as absent.
 *
 * Phase 6 (D-107): `durationMs` is required for the two SEALED types as well —
 * the field is already visible for a sealed row (`visibleFields`' `timed` flag)
 * and the server's sealed arm refuses `duration_ms < 1`, so an empty box must
 * report its own message and disable Save rather than round-trip into the
 * generic `admin.actionRejected` row. `minIncrement` stays English-only: the
 * form does not render it for a sealed row, so a rule there could only strand an
 * invisible error.
 */
function isRequired(key: NumberField, fields: DraftFields): boolean {
  switch (key) {
    case 'truthValue':
    case 'startPrice':
      return true
    case 'minIncrement':
      return fields.auctionType === 'english'
    case 'durationMs':
      return fields.auctionType === 'english' || isSealedAuction(fields.auctionType)
    case 'antiSnipeWindowMs':
      return fields.auctionType === 'english' && fields.antiSnipeMode === 'extend'
    default:
      return false
  }
}

/**
 * The UI-SPEC's validation table, applied to the VISIBLE fields only. The server
 * stays the authority (D-66): the client mirrors the same rules so the presenter
 * has a field to fix, because the server's refusal carries only the frozen
 * `bad_message` code and its `message` is log-only.
 *
 * There is deliberately NO `touched` gating (D-82b): the function takes the
 * draft and nothing else, so a freshly opened editor reports its empty required
 * fields on the first render and `Save item` is disabled from the same call.
 */
export function validateDraft(draft: EditorDraft): DraftErrors {
  const fields = draft.fields
  const visible = visibleFields(fields.auctionType, fields.antiSnipeMode)
  const errors: DraftErrors = {}

  const name = fields.name.trim()
  if (name.length === 0 || [...name].length > MAX_ITEM_NAME_RUNES) errors.name = 'admin.err.name'

  const check = (key: NumberField, floor: 0 | 1) => {
    if (!visible[key]) return
    const raw = fields[key]
    if (raw === '') {
      if (isRequired(key, fields)) {
        errors[key] = floor === 1 ? 'admin.err.positive' : 'admin.err.notNegative'
      }
      return
    }
    // One parse rule for both routes: a raw digit string, exactly what the bid
    // field accepts (D-71). Grouped amounts are a display form only.
    const parsed = parseBidAmount(raw)
    if (parsed === null || parsed < floor) {
      errors[key] = floor === 1 ? 'admin.err.positive' : 'admin.err.notNegative'
    }
  }

  for (const key of ['truthValue', 'hintMin', 'hintMax'] as const) check(key, 0)
  for (const key of [
    'startPrice',
    'minIncrement',
    'durationMs',
    'antiSnipeWindowMs',
    'maxExtensions',
    'floorPrice',
    'decrementStep',
    'decrementIntervalMs',
  ] as const) {
    check(key, 1)
  }

  const min = fields.hintMin === '' ? null : parseBidAmount(fields.hintMin)
  const max = fields.hintMax === '' ? null : parseBidAmount(fields.hintMax)
  if (min !== null && max !== null && min > max) errors.hintMin = 'admin.err.hintOrder'

  // D-92's Dutch pair rule, the phase's second cross-field check: a floor above
  // the start price could never be reached honestly, and the server refuses it
  // at save time (items.go's validateItemInput, inside its `dutch` arm — an
  // absent floor stays legal for a row still being configured). The rule is
  // scoped to the type that renders the floor field, so it can never disable
  // Save behind a message the presenter cannot see.
  if (visible.floorPrice) {
    const floor = fields.floorPrice === '' ? null : parseBidAmount(fields.floorPrice)
    const start = fields.startPrice === '' ? null : parseBidAmount(fields.startPrice)
    if (floor !== null && start !== null && floor > start) {
      errors.floorPrice = 'admin.err.floorOrder'
    }
  }

  return errors
}

/**
 * `Save item`'s `disabled` prop, derived from the same `validateDraft` call that
 * renders the field messages: an enabled-but-inert Save (the 03-UI-REVIEW
 * warning D-82b repairs) cannot come back, because there is no second rule.
 */
export function draftValid(draft: EditorDraft): boolean {
  return Object.keys(validateDraft(draft)).length === 0
}

/**
 * The `admin.item.upsert` payload (D-66): the FULL object, so the editor
 * round-trips exactly the row it was shown. Every numeric field is sent as the
 * integer its raw digit string parses to, or null when it is empty — including
 * the fields the current type hides, which keeps their loaded values instead of
 * discarding them when the presenter visits another type on the way to Save.
 */
export function draftPayload(draft: EditorDraft): Record<string, unknown> {
  const fields = draft.fields
  const num = (key: NumberField): number | null => {
    const raw = fields[key]
    return raw === '' ? null : parseBidAmount(raw)
  }

  return {
    id: draft.id,
    name: fields.name.trim(),
    description: draft.description,
    image_url: draft.imageUrl,
    auction_type: fields.auctionType,
    truth_value: num('truthValue') ?? 0,
    hint_min: num('hintMin'),
    hint_max: num('hintMax'),
    start_price: num('startPrice') ?? 0,
    min_increment: num('minIncrement'),
    floor_price: num('floorPrice'),
    decrement_step: num('decrementStep'),
    decrement_interval_ms: num('decrementIntervalMs'),
    // D-93's client mirror: a Dutch run has no wall-clock window, so the
    // duration is omitted from a Dutch payload rather than echoed from a since-
    // hidden field (a draft that was English when the value was typed, or one
    // loaded from another type). The server refuses the field outright; every
    // other type keeps the value it was shown (the sealed types are timed too).
    duration_ms: fields.auctionType === 'dutch' ? null : num('durationMs'),
    // D-107's mirror, the sealed twin of the line above: the anti-snipe control
    // is invisible for a sealed row (`visibleFields` keeps it English-only), so
    // a leftover `extend` — a draft loaded from an English item, or a type switch
    // in the editor — would be an unfixable refusal. The payload emits the value
    // the type can have; the server's sealed arm requires exactly `disabled`.
    anti_snipe_mode: isSealedAuction(fields.auctionType) ? 'disabled' : fields.antiSnipeMode,
    anti_snipe_window_ms: num('antiSnipeWindowMs'),
    max_extensions: num('maxExtensions'),
  }
}

export function blankDraft(): EditorDraft {
  return {
    id: '',
    description: '',
    imageUrl: '',
    fields: {
      name: '',
      auctionType: 'english',
      antiSnipeMode: 'disabled',
      truthValue: '',
      hintMin: '',
      hintMax: '',
      startPrice: '',
      minIncrement: '',
      durationMs: '',
      antiSnipeWindowMs: '',
      maxExtensions: '',
      floorPrice: '',
      decrementStep: '',
      decrementIntervalMs: '',
    },
  }
}

/** Load one catalog row into the form, keeping every value it was shown. */
export function draftFromItem(item: AdminItem): EditorDraft {
  const text = (value: number | null): string => (value === null ? '' : String(value))
  return {
    id: item.id,
    description: item.description,
    imageUrl: item.imageUrl,
    fields: {
      name: item.name,
      auctionType: item.auctionType,
      antiSnipeMode: item.antiSnipeMode === '' ? 'disabled' : item.antiSnipeMode,
      truthValue: String(item.truthValue),
      hintMin: text(item.hintMin),
      hintMax: text(item.hintMax),
      startPrice: String(item.startPrice),
      minIncrement: text(item.minIncrement),
      durationMs: text(item.durationMs),
      antiSnipeWindowMs: text(item.antiSnipeWindowMs),
      maxExtensions: text(item.maxExtensions),
      floorPrice: text(item.floorPrice),
      decrementStep: text(item.decrementStep),
      decrementIntervalMs: text(item.decrementIntervalMs),
    },
  }
}

/**
 * D-81's starting-budget rule, applied to the one field the Setup tab adds: a
 * whole number ≥ 1 (`admin.err.positive` is the field's own sentence). The
 * returned key drives BOTH the message under the field and `Save budget`'s
 * `disabled` prop, so an enabled-but-inert Save cannot exist here either.
 */
export function validateStartingBudget(raw: string): CopyKey | undefined {
  const parsed = parseBidAmount(raw)
  return parsed === null || parsed < 1 ? 'admin.err.positive' : undefined
}

/**
 * D-82a generalised: the socket is the presenter's only channel, so every
 * mutating control on all three tabs derives its `disabled` prop from this one
 * predicate. A control that cannot deliver must not look available.
 */
export function mutationsEnabled(status: ConnectionStatus): boolean {
  return status === 'live'
}

/**
 * D-86's candidate rows: the server's boot-time list (D-83) plus the
 * client-origin fallback the server itself cannot know (the admin page may be
 * open on a hostname the enumeration did not rank). The fallback is appended
 * ONCE and only when it is absent, so choosing it twice cannot produce two rows
 * that mean the same URL, and the panel is never empty while the page has an
 * origin. An empty origin is ignored rather than offered as a blank row.
 */
export function urlCandidates(joinUrls: readonly string[], pageOrigin: string): string[] {
  const rows = [...joinUrls]
  if (pageOrigin !== '' && !rows.includes(pageOrigin)) rows.push(pageOrigin)
  return rows
}

/** The one Setup command both the budget field and the URL rows send (D-81/D-84). */
export function configFrame(config: AdminConfigInput): AdminCommand {
  return { type: 'admin.config.update', config }
}

/** D-79's per-mode confirm sentence — the two strips name their own consequence. */
export function resetConfirmKey(mode: ResetMode): CopyKey {
  return mode === 'keep_players' ? 'admin.resetKeepConfirm' : 'admin.resetClearConfirm'
}

/**
 * The reset frame: the armed mode IS the wire mode (D-78's closed vocabulary),
 * so the second click sends exactly what the strip described and nothing else.
 */
export function resetFrame(mode: ResetMode): AdminCommand {
  return { type: 'admin.game.reset', mode }
}

/** D-80's release command: the row's player, and no other field. */
export function releaseFrame(id: string): AdminCommand {
  return { type: 'admin.player.release', id }
}

/**
 * D-80's row order: `joined_at` ascending. The sort is stable, so two players
 * who joined in the same millisecond keep the order the snapshot carried (the
 * server's own joined_at-then-id order, ECON-05's one standing), and the
 * snapshot array is copied rather than reordered in place.
 */
export function sortPlayersByJoinedAt(players: readonly AdminPlayer[]): AdminPlayer[] {
  return [...players].sort((a, b) => a.joinedAt - b.joinedAt)
}

/** D-81's presenter read-out — the settled presenter card's whole model. */
export type PresenterSettle = {
  outcome: 'sold' | 'unsold'
  headingKey: CopyKey
  /** the winner's nickname, or null when unsold */
  winner: string | null
  price: number
  /** the presenter's secret view (D-81/ADMN-07) */
  truthValue: number
  /** the server's own net for the winner (ECON-02/ECON-03) */
  budgetDelta: number
  /** `formatVND(budgetDelta)`, with a leading `+` only when positive */
  deltaText: string
  /** the winner's budget NOW, from the presenter rows; null when unsold */
  winnerBudget: number | null
}

/**
 * D-81/ADMN-07's presenter card, derived from the snapshot and nothing else:
 * the reveal object the settle commit built (`item.settle`) and the presenter's
 * own player rows. Nothing here recomputes an outcome — the winner, the price,
 * the truth value and the net are all the server's numbers, and the "winner's
 * budget now" is the row the same snapshot carries (matched by nickname, the
 * one public identity a leader entry has).
 *
 * Returns null outside SETTLED and when there is no reveal to read, which is
 * what keeps the card off every other phase rather than rendering a blank one.
 */
export function presenterSettle(
  phase: Phase,
  item: PublicItem | null,
  players: readonly AdminPlayer[],
): PresenterSettle | null {
  if (phase !== 'SETTLED' || item === null || item.settle === null) return null

  const settle = item.settle
  const sold = settle.outcome === 'sold'
  const winner = sold ? settle.winner : null
  return {
    outcome: sold ? 'sold' : 'unsold',
    headingKey: sold ? 'settle.soldHeading' : 'settle.unsoldHeading',
    winner,
    price: settle.price,
    truthValue: settle.truthValue,
    budgetDelta: settle.budgetDelta,
    // A `+` is presentation, never a second number: the delta is the server's
    // own and `formatVND` already carries a minus sign when it has one.
    deltaText:
      settle.budgetDelta > 0 ? `+${formatVND(settle.budgetDelta)}` : formatVND(settle.budgetDelta),
    winnerBudget:
      winner === null ? null : (players.find((player) => player.nickname === winner)?.budget ?? null),
  }
}
