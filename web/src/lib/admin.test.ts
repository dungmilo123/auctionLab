import { describe, expect, it } from 'vitest'
import type { AdminItem } from '@/ws/protocol'
import {
  blankDraft,
  configFrame,
  draftFromItem,
  draftPayload,
  draftValid,
  mutationsEnabled,
  urlCandidates,
  validateDraft,
  validateStartingBudget,
  type DraftFields,
  type EditorDraft,
} from './admin'

// The admin page's pure rules (D-79/D-81/D-82), pinned in Vitest's node
// environment: there is no DOM harness in this phase, so the page's validation
// and confirmation behaviour is only testable where it lives — in selectors.
// Every assertion below is written against the UI-SPEC's own wording
// (04-UI-SPEC.md §Admin Route & Interaction Contract), not against the module.

const LAN = 'http://192.168.1.10:8080'
const PAGE = 'http://localhost:5173'

/** A stored catalog row the editor loads (the shape the decoder hands over). */
const ITEM: AdminItem = {
  id: 'i1',
  orderIndex: 0,
  name: 'Đèn bàn',
  description: '',
  imageUrl: '',
  auctionType: 'english',
  truthValue: 200_000,
  hintMin: null,
  hintMax: null,
  startPrice: 100_000,
  minIncrement: 10_000,
  floorPrice: null,
  decrementStep: null,
  decrementIntervalMs: null,
  durationMs: 60_000,
  antiSnipeMode: 'disabled',
  antiSnipeWindowMs: null,
  maxExtensions: null,
  status: 'pending',
}

/** A draft with every required english field filled, for the "valid" cases. */
function filled(overrides: Partial<DraftFields> = {}): EditorDraft {
  const base = blankDraft()
  return {
    ...base,
    fields: {
      ...base.fields,
      name: 'Đèn bàn',
      truthValue: '200000',
      startPrice: '100000',
      minIncrement: '10000',
      durationMs: '60000',
      ...overrides,
    },
  }
}

describe('the starting budget (D-81/D-82)', () => {
  it('accepts a whole number of 1 or more', () => {
    expect(validateStartingBudget('1')).toBeUndefined()
    expect(validateStartingBudget('5000000')).toBeUndefined()
  })

  it('rejects an empty, zero, negative, fractional or grouped value with the field key', () => {
    for (const raw of ['', '0', '-1', '1.5', '1.000', 'abc', ' 5 ']) {
      expect(validateStartingBudget(raw)).toBe('admin.err.positive')
    }
  })
})

describe('the item draft (D-82b)', () => {
  it('reports every empty required field of a brand-new english draft on the first render', () => {
    const errors = validateDraft(blankDraft())
    expect(errors.name).toBe('admin.err.name')
    // truth_value has a 0 floor; the rest are positive-only.
    expect(errors.truthValue).toBe('admin.err.notNegative')
    expect(errors.startPrice).toBe('admin.err.positive')
    expect(errors.minIncrement).toBe('admin.err.positive')
    expect(errors.durationMs).toBe('admin.err.positive')
    // Optional fields stay silent, and a field the current type hides is never
    // reported: anti-snipe is off in a blank draft.
    expect(errors.hintMin).toBeUndefined()
    expect(errors.hintMax).toBeUndefined()
    expect(errors.antiSnipeWindowMs).toBeUndefined()
  })

  it('disables Save from the same call that renders the messages', () => {
    expect(draftValid(blankDraft())).toBe(false)
    expect(draftValid(filled())).toBe(true)
  })

  it('validates the fields the current type shows, and only those', () => {
    const dutch = filled({ auctionType: 'dutch', minIncrement: '', durationMs: '' })
    // min_increment/duration_ms are english-only, so their blanks are not
    // errors; the dutch-only fields are optional on the wire (D-66).
    expect(Object.keys(validateDraft(dutch)).sort()).toEqual(['name', 'startPrice', 'truthValue'])
    expect(draftValid(dutch)).toBe(true)
  })

  it('refuses a value that is grouped, fractional or negative', () => {
    expect(validateDraft(filled({ startPrice: '1.000' })).startPrice).toBe('admin.err.positive')
    expect(validateDraft(filled({ truthValue: '-1' })).truthValue).toBe('admin.err.notNegative')
  })

  it("refuses a hint range whose minimum is above its maximum", () => {
    expect(validateDraft(filled({ hintMin: '500', hintMax: '100' })).hintMin).toBe(
      'admin.err.hintOrder',
    )
    expect(validateDraft(filled({ hintMin: '100', hintMax: '500' })).hintMin).toBeUndefined()
  })

  it('loads a stored row into the form unchanged, already valid', () => {
    const draft = draftFromItem(ITEM)
    expect(draft.id).toBe(ITEM.id)
    expect(draft.fields.name).toBe(ITEM.name)
    expect(draft.fields.truthValue).toBe('200000')
    // A blank optional field is the empty string, not the word "null".
    expect(draft.fields.floorPrice).toBe('')
    expect(draft.fields.hintMin).toBe('')
    expect(draftValid(draft)).toBe(true)
  })

  it('sends the whole object back with every numeric field parsed or null', () => {
    const payload = draftPayload(filled())
    expect(payload.id).toBe('')
    expect(payload.name).toBe('Đèn bàn')
    expect(payload.auction_type).toBe('english')
    expect(payload.truth_value).toBe(200_000)
    expect(payload.start_price).toBe(100_000)
    // Empty optional fields travel as an explicit null, never as 0.
    expect(payload.hint_min).toBeNull()
    expect(payload.hint_max).toBeNull()
    expect(payload.floor_price).toBeNull()
  })
})

describe('the socket gate (D-82a)', () => {
  it('allows a mutation only while the socket is open', () => {
    expect(mutationsEnabled('open')).toBe(true)
    expect(mutationsEnabled('connecting')).toBe(false)
    expect(mutationsEnabled('lost')).toBe(false)
    expect(mutationsEnabled('superseded')).toBe(false)
  })
})

describe('the join-URL candidates (D-86)', () => {
  it("appends this page's origin once, when the server list does not carry it", () => {
    expect(urlCandidates([LAN], PAGE)).toEqual([LAN, PAGE])
  })

  it('never duplicates a candidate the server already listed', () => {
    expect(urlCandidates([LAN, PAGE], PAGE)).toEqual([LAN, PAGE])
  })

  it('is never empty: with no server candidates the client row is the whole list', () => {
    expect(urlCandidates([], PAGE)).toEqual([PAGE])
  })

  it('ignores an unavailable origin instead of offering an empty row', () => {
    expect(urlCandidates([LAN], '')).toEqual([LAN])
  })

  it('leaves the snapshot list it was given untouched', () => {
    const server = [LAN]
    urlCandidates(server, PAGE)
    expect(server).toEqual([LAN])
  })
})

describe('the typed admin commands (D-81/D-84)', () => {
  it('builds one admin.config.update for both Setup actions', () => {
    expect(configFrame({ starting_budget: 5_000_000 })).toEqual({
      type: 'admin.config.update',
      config: { starting_budget: 5_000_000 },
    })
    expect(configFrame({ join_url: LAN })).toEqual({
      type: 'admin.config.update',
      config: { join_url: LAN },
    })
  })
})
