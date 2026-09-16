import { describe, expect, it } from 'vitest'
import type { AdminItem, AdminPlayer, PublicItem } from '@/ws/protocol'
import {
  blankDraft,
  configFrame,
  draftFromItem,
  draftPayload,
  draftValid,
  mutationsEnabled,
  presenterSettle,
  releaseFrame,
  resetConfirmKey,
  resetFrame,
  sortPlayersByJoinedAt,
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
    // min_increment/duration_ms are english-only, so the same blanks that fail
    // an english draft are simply not in the dutch draft's matrix.
    const dutch = filled({ auctionType: 'dutch', minIncrement: '', durationMs: '' })
    expect(validateDraft(dutch)).toEqual({})
    expect(draftValid(dutch)).toBe(true)
    expect(
      Object.keys(validateDraft(filled({ minIncrement: '', durationMs: '' }))).sort(),
    ).toEqual(['durationMs', 'minIncrement'])
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

  it("refuses a Dutch floor above the start price with the field's own key (D-92)", () => {
    const above = filled({ auctionType: 'dutch', floorPrice: '200000', startPrice: '100000' })
    expect(validateDraft(above).floorPrice).toBe('admin.err.floorOrder')
    expect(draftValid(above)).toBe(false)
  })

  it('keeps a Dutch floor at or below the start valid, and the pair rule Dutch-only', () => {
    expect(validateDraft(filled({ auctionType: 'dutch', floorPrice: '100000' }))).toEqual({})
    // The floor field is hidden on every other type, so the rule cannot strand
    // an invisible error under a field the form never renders — the server
    // scopes the same rule to `dutch` (items.go's validateItemInput).
    expect(validateDraft(filled({ floorPrice: '200000' })).floorPrice).toBeUndefined()
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

  it('omits duration_ms from a Dutch payload, and only from a Dutch one (D-93)', () => {
    // A Dutch run has no wall-clock window, so the hidden field must not reach
    // the server, which refuses it outright (items.go's validateItemInput) —
    // and the presenter cannot see the field to clear it on this type.
    const dutch = draftPayload(filled({ auctionType: 'dutch', durationMs: '30000' }))
    expect(dutch.auction_type).toBe('dutch')
    expect(dutch.duration_ms).toBeNull()

    // Every other type keeps the value it was shown: the sealed types are timed
    // as well, so discarding their duration would lose the presenter's input.
    const sealed = draftPayload(filled({ auctionType: 'sealed_first', minIncrement: '', durationMs: '45000' }))
    expect(sealed.duration_ms).toBe(45_000)
  })

  it('requires a duration for both sealed types, exactly as the server does (D-107)', () => {
    // The sealed arm of `validateItemInput` refuses `duration_ms < 1`, and the
    // field IS visible for a sealed row (`visibleFields`' `timed` flag) — so an
    // empty box must report its own message and disable Save rather than
    // round-trip into the generic `admin.actionRejected` row.
    for (const auctionType of ['sealed_first', 'vickrey'] as const) {
      const empty = filled({ auctionType, minIncrement: '', durationMs: '' })
      expect(validateDraft(empty).durationMs).toBe('admin.err.positive')
      expect(draftValid(empty)).toBe(false)
      // The same row with a duration is valid: the rule is the duration alone.
      expect(draftValid(filled({ auctionType, minIncrement: '', durationMs: '45000' }))).toBe(true)
    }
  })

  it('sends anti_snipe_mode disabled for a sealed payload, whatever the draft carries (D-107)', () => {
    // D-93's Dutch mirror, applied to the other type-specific field: the
    // anti-snipe control is invisible for a sealed row (`visibleFields` keeps it
    // English-only), so an inherited `extend` would be an unfixable refusal —
    // the payload emits the value the type can have.
    const sealed = draftPayload(
      filled({ auctionType: 'sealed_first', minIncrement: '', antiSnipeMode: 'extend', durationMs: '45000' }),
    )
    expect(sealed.anti_snipe_mode).toBe('disabled')
    const vickrey = draftPayload(
      filled({ auctionType: 'vickrey', minIncrement: '', antiSnipeMode: 'extend', durationMs: '45000' }),
    )
    expect(vickrey.anti_snipe_mode).toBe('disabled')

    // Every other type still sends what the presenter set (the English control
    // is the one that renders).
    expect(draftPayload(filled({ antiSnipeMode: 'extend' })).anti_snipe_mode).toBe('extend')
    expect(draftPayload(filled({ antiSnipeMode: 'disabled' })).anti_snipe_mode).toBe('disabled')
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

/** A presenter row, in the shape the admin snapshot decodes. */
function player(id: string, joinedAt: number, overrides: Partial<AdminPlayer> = {}): AdminPlayer {
  return {
    id,
    nickname: id.toUpperCase(),
    budget: 5_000_000,
    totalValue: 0,
    joinedAt,
    lastSeenAt: joinedAt,
    connected: true,
    ...overrides,
  }
}

/** The public item at SETTLED, with the reveal this tab is allowed to read. */
function settledItem(overrides: Partial<PublicItem> = {}): PublicItem {
  return {
    id: 'i1',
    name: 'Đèn bàn',
    description: '',
    auctionType: 'english',
    hintMin: null,
    hintMax: null,
    startPrice: 100_000,
    status: 'settled',
    currentPrice: 300_000,
    topBidder: 'An',
    deadlineAt: null,
    minNextBid: null,
    extensionsUsed: 0,
    startedAt: null,
    floorPrice: null,
    decrementStep: null,
    decrementIntervalMs: null,
    settle: { outcome: 'sold', winner: 'An', price: 300_000, truthValue: 350_000, budgetDelta: 50_000 },
    ...overrides,
  }
}

describe('the reset modes (D-79)', () => {
  it('arms each mode with its own consequence sentence', () => {
    expect(resetConfirmKey('keep_players')).toBe('admin.resetKeepConfirm')
    expect(resetConfirmKey('clear_players')).toBe('admin.resetClearConfirm')
  })

  it('sends exactly the armed mode on the second click', () => {
    expect(resetFrame('keep_players')).toEqual({ type: 'admin.game.reset', mode: 'keep_players' })
    expect(resetFrame('clear_players')).toEqual({ type: 'admin.game.reset', mode: 'clear_players' })
  })
})

describe('the release command (D-80)', () => {
  it('names the player the row belongs to and nothing else', () => {
    expect(releaseFrame('p1')).toEqual({ type: 'admin.player.release', id: 'p1' })
  })
})

describe('the Players rows (D-80)', () => {
  it('orders the rows by joined_at, oldest first', () => {
    const players = [player('c', 3000), player('a', 1000), player('b', 2000)]
    expect(sortPlayersByJoinedAt(players).map((p) => p.id)).toEqual(['a', 'b', 'c'])
  })

  it('keeps the server order for two players who joined in the same millisecond', () => {
    const tied = [player('second', 1000), player('first', 1000)]
    expect(sortPlayersByJoinedAt(tied).map((p) => p.id)).toEqual(['second', 'first'])
  })

  it('does not order the snapshot list in place', () => {
    const players = [player('b', 2000), player('a', 1000)]
    sortPlayersByJoinedAt(players)
    expect(players.map((p) => p.id)).toEqual(['b', 'a'])
  })
})

describe('the presenter settle card (D-81/ADMN-07)', () => {
  it('is absent outside SETTLED, and when there is no reveal to read', () => {
    const players = [player('an', 1000, { nickname: 'An' })]
    expect(presenterSettle('LIVE', settledItem(), players)).toBeNull()
    expect(presenterSettle('SETTLED', null, players)).toBeNull()
    expect(presenterSettle('SETTLED', settledItem({ settle: null }), players)).toBeNull()
  })

  it('reads the winner, the price, the hidden truth value and the delta from the snapshot', () => {
    const card = presenterSettle('SETTLED', settledItem(), [player('an', 1000, { nickname: 'An' })])
    expect(card).not.toBeNull()
    expect(card).toMatchObject({
      outcome: 'sold',
      headingKey: 'settle.soldHeading',
      winner: 'An',
      price: 300_000,
      truthValue: 350_000,
      budgetDelta: 50_000,
      deltaText: '+50.000',
    })
  })

  it("reads the winner's current budget from the presenter rows, matched by nickname", () => {
    const card = presenterSettle('SETTLED', settledItem(), [
      player('an', 1000, { nickname: 'An', budget: 4_700_000 }),
      player('binh', 2000, { nickname: 'Bình', budget: 5_000_000 }),
    ])
    expect(card?.winnerBudget).toBe(4_700_000)
  })

  it('leaves the winner rows out of an unsold reveal instead of printing zeros', () => {
    const unsold = settledItem({
      settle: { outcome: 'unsold', winner: null, price: 0, truthValue: 350_000, budgetDelta: 0 },
    })
    const card = presenterSettle('SETTLED', unsold, [player('an', 1000, { nickname: 'An' })])
    expect(card).toMatchObject({
      outcome: 'unsold',
      headingKey: 'settle.unsoldHeading',
      winner: null,
      truthValue: 350_000,
      deltaText: '0',
      winnerBudget: null,
    })
  })
})
