/**
 * The admin page's pure logic (D-79/D-81/D-82) — RED DECLARATIONS.
 *
 * Every export below is a sentinel: the types and the names are final, the
 * bodies are not. `admin.test.ts` is the failing suite that pins the behaviour
 * (D-82's budget rule, D-82b's immediate draft validation, D-82a's socket gate
 * and D-86's candidate rows), and the GREEN commit replaces each body with the
 * implementation — 04-04's and 04-06's declared-sentinel pattern, so the suite
 * COMPILES and fails on assertions instead of on a missing identifier (#3770).
 *
 * Everything this module will own is a selector: no DOM, no React, no socket,
 * no clock — which is the only way the page's rules are testable in this phase
 * (`vitest` runs `environment: node`; there is no DOM harness).
 */

import type { AdminCommand, AdminConfigInput } from '@/ws/admin'
import type { ConnectionStatus } from '@/ws/client'
import type { AdminItem } from '@/ws/protocol'
import type { CopyKey } from './i18n'

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
  _type: string,
  _antiSnipeMode: string,
): Record<keyof DraftFields, boolean> {
  return {
    name: true,
    auctionType: true,
    antiSnipeMode: true,
    truthValue: true,
    hintMin: true,
    hintMax: true,
    startPrice: true,
    minIncrement: true,
    durationMs: true,
    antiSnipeWindowMs: true,
    maxExtensions: true,
    floorPrice: true,
    decrementStep: true,
    decrementIntervalMs: true,
  }
}

/** The UI-SPEC's validation table, applied to the VISIBLE fields only (D-66). */
export function validateDraft(_draft: EditorDraft): DraftErrors {
  return {}
}

/** `Save item`'s `disabled` prop: one `validateDraft` call, never a second rule. */
export function draftValid(_draft: EditorDraft): boolean {
  return true
}

/** The `admin.item.upsert` payload (D-66): the FULL object. */
export function draftPayload(_draft: EditorDraft): Record<string, unknown> {
  return {}
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
export function draftFromItem(_item: AdminItem): EditorDraft {
  return blankDraft()
}

/** D-81's starting-budget rule: a whole number of 1 or more, or the field's own key. */
export function validateStartingBudget(_raw: string): CopyKey | undefined {
  return undefined
}

/** D-82a: every mutating control is disabled unless the socket is open. */
export function mutationsEnabled(_status: ConnectionStatus): boolean {
  return true
}

/** The one Setup command both the budget field and the URL rows send (D-81/D-84). */
export function configFrame(_config: AdminConfigInput): AdminCommand {
  return { type: 'admin.config.update', config: {} }
}

/**
 * D-86's candidate list: the server's rows plus the client-origin fallback,
 * appended once and only when it is not already a candidate.
 */
export function urlCandidates(joinUrls: readonly string[], _pageOrigin: string): string[] {
  return [...joinUrls]
}
