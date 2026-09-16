import { describe, expect, it } from 'vitest'
import {
  canSubmitSealed,
  sealedFieldError,
  sealedInputValue,
  type SealedAmountInput,
  type SealedSubmitInput,
} from './sealed'

// The sealed amount field's pure rules (D-119/D-120/D-107), pinned in Vitest's
// node environment with frozen values — the same shape `lib/bid.test.ts` pins the
// English field with. Every assertion below is written against 06-UI-SPEC.md's
// §The submit/edit interaction and §Feedback slot, not against the module: the
// server stays the authority (D-52) and these rules exist so the player has a
// field to fix.

const START = 300_000
const BUDGET = 1_000_000

/** The field's own facts, with the two bounds the UI-SPEC fixes. */
function field(overrides: Partial<SealedAmountInput> = {}): SealedAmountInput {
  return { raw: '', startPrice: START, budget: BUDGET, ...overrides }
}

/** The field plus the window/socket facts the enabled predicate reads. */
function submit(overrides: Partial<SealedSubmitInput> = {}): SealedSubmitInput {
  return { ...field({ raw: '320000' }), status: 'open', remainingMs: 30_000, inFlight: false, ...overrides }
}

describe('the sealed field prefill (D-119)', () => {
  it("follows the snapshot's own my_bid while the player has not edited", () => {
    // A reload mid-window paints the amount the player submitted (D-63): the
    // field follows `my_bid` — not `min_next_bid` — until the player types.
    expect(sealedInputValue(false, '', 320_000)).toBe('320000')
    expect(sealedInputValue(false, '999999', 320_000)).toBe('320000')
  })

  it('starts empty for a player who has not submitted', () => {
    // `my_bid` absent is the wire's "not submitted" (D-115): the field is empty,
    // which is the initial state and never an error.
    expect(sealedInputValue(false, '', null)).toBe('')
  })

  it('never overwrites the value the player is typing (D-120)', () => {
    expect(sealedInputValue(true, '350000', 320_000)).toBe('350000')
    expect(sealedInputValue(true, '', 320_000)).toBe('')
  })
})

describe('the sealed field message (D-119/D-120)', () => {
  it('says nothing about an empty field — an unfilled field is not an error', () => {
    expect(sealedFieldError(field())).toBeNull()
  })

  it('names a non-digit value, a below-minimum value and an over-budget value', () => {
    expect(sealedFieldError(field({ raw: '1.000' }))).toBe('reason.invalidAmount')
    expect(sealedFieldError(field({ raw: 'abc' }))).toBe('reason.invalidAmount')
    expect(sealedFieldError(field({ raw: '299999' }))).toBe('reason.belowMinimum')
    expect(sealedFieldError(field({ raw: '1000001' }))).toBe('reason.exceedsBudget')
  })

  it('accepts both bounds inclusively (SEAL-02)', () => {
    expect(sealedFieldError(field({ raw: String(START) }))).toBeNull()
    expect(sealedFieldError(field({ raw: String(BUDGET) }))).toBeNull()
    expect(sealedFieldError(field({ raw: '320000' }))).toBeNull()
  })

  it("applies the server's own order when one value breaks two rules (D-112)", () => {
    // A start price above the budget is a configuration the presenter owns, but
    // the ORDER is the contract: `below_minimum` is answered before
    // `exceeds_budget`, exactly as the engine's sealed arm refuses.
    const impossible = field({ raw: '1500000', startPrice: 2_000_000, budget: 1_000_000 })
    expect(sealedFieldError(impossible)).toBe('reason.belowMinimum')
  })

  it('says nothing about the window — the closed sentence is its own slot', () => {
    expect(sealedFieldError(field({ raw: '320000' }))).toBeNull()
  })
})

describe('the sealed submit predicate (D-120)', () => {
  it('is enabled only with an open socket, an open window, nothing in flight and a usable amount', () => {
    expect(canSubmitSealed(submit())).toBe(true)
  })

  it('is enabled at both bounds and only at them', () => {
    expect(canSubmitSealed(submit({ raw: String(START) }))).toBe(true)
    expect(canSubmitSealed(submit({ raw: String(BUDGET) }))).toBe(true)
    expect(canSubmitSealed(submit({ raw: String(START - 1) }))).toBe(false)
    expect(canSubmitSealed(submit({ raw: String(BUDGET + 1) }))).toBe(false)
  })

  it('is disabled for an empty or unparseable field', () => {
    expect(canSubmitSealed(submit({ raw: '' }))).toBe(false)
    expect(canSubmitSealed(submit({ raw: '1.000' }))).toBe(false)
  })

  it('is disabled while the socket is not open, the window has closed, or a frame is in flight', () => {
    for (const status of ['connecting', 'lost', 'superseded'] as const) {
      expect(canSubmitSealed(submit({ status }))).toBe(false)
    }
    expect(canSubmitSealed(submit({ remainingMs: 0 }))).toBe(false)
    expect(canSubmitSealed(submit({ inFlight: true }))).toBe(false)
  })

  it('is disabled by the window and the flight even when the amount is valid', () => {
    // The two facts are independent of the field's own message: a refused value
    // keeps its sentence, and a closed window disables the CTA without one.
    expect(sealedFieldError(field({ raw: '320000' }))).toBeNull()
    expect(canSubmitSealed(submit({ raw: '320000', remainingMs: 0, inFlight: true }))).toBe(false)
  })
})
