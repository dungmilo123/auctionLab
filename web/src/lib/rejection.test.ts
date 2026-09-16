import { describe, expect, it } from 'vitest'
import type { BidRejected, PlayerSnapshot } from '@/ws/protocol'
import { keepRejection, type HeldRejection } from './rejection'

// D-97's notice lifetime (WR-04), pinned where it lives. The phase's one
// deliberate change to the carried "a rejection clears on the next snapshot"
// rule is a pure `(prev, snapshot) → prev | null` function, so Vitest's node
// environment is its whole test surface (the convention lib/bid.ts and
// lib/screen.ts set). The plan's validation row (05-04-T2, "Race loss remains
// visible through same-item settled snapshots") names a case: `already_sold`
// must survive the reveal's own snapshots and die with a phase or item change —
// a regression here is the phase's "phantom win" failure, invisible to the
// suite until a whole UAT pass on the phones.

const TS = 1_757_934_000_000

/** One private refusal payload. Only `reason` matters to this rule. */
function rejected(reason: string): BidRejected {
  return { reason, minNextBid: null, budget: 900_000, amount: 70_000 }
}

/** The refusal the Dutch race loser holds, scoped to the item it lost on. */
function held(reason: string, itemId: string | null = 'i1'): HeldRejection {
  return { payload: rejected(reason), itemId }
}

/** One player snapshot; only `phase` and `item.id` matter to this rule. */
function snapshot(phase: PlayerSnapshot['phase'], itemId: string | null): PlayerSnapshot {
  return {
    phase,
    revision: 9,
    serverTs: TS,
    me: {
      id: 'p1',
      nickname: 'An',
      budget: 900_000,
      totalValue: 0,
      connected: true,
      lastBid: null,
      isTopBidder: false,
      rank: 0,
      // Phase 6 (D-115): only the sealed decode reads this key.
      myBid: null,
    },
    lobby: [],
    leaderboard: [],
    // Phase 6 (D-116): only a sealed item carries a counter.
    sealedProgress: null,
    item:
      itemId === null
        ? null
        : {
            id: itemId,
            name: 'Đồng hồ',
            description: '',
            auctionType: 'dutch',
            hintMin: null,
            hintMax: null,
            startPrice: 100_000,
            status: 'settled',
            currentPrice: 70_000,
            topBidder: null,
            deadlineAt: null,
            minNextBid: null,
            extensionsUsed: 0,
            startedAt: TS - 3_000,
            floorPrice: 40_000,
            decrementStep: 10_000,
            decrementIntervalMs: 1_000,
            settle: {
              outcome: 'sold',
              winner: 'An',
              price: 70_000,
              truthValue: 350_000,
              budgetDelta: 280_000,
            },
          },
  }
}

describe('the held refusal (D-53/D-97)', () => {
  it('keeps an already_sold notice through later SETTLED snapshots for the same item', () => {
    const prev = held('already_sold')
    // The winner's settle snapshot has already landed when the loser's refusal
    // arrives (and every later same-item snapshot — a join, a presence change,
    // a resync — must not take the notice away).
    const next = keepRejection(prev, snapshot('SETTLED', 'i1'))
    expect(next).toBe(prev)
  })

  it('clears an already_sold notice on a phase change', () => {
    const prev = held('already_sold')
    expect(keepRejection(prev, snapshot('ITEM_INTRO', 'i1'))).toBeNull()
    expect(keepRejection(prev, snapshot('LIVE', 'i1'))).toBeNull()
    expect(keepRejection(prev, snapshot('FINALE', null))).toBeNull()
  })

  it('clears an already_sold notice when another item is on screen', () => {
    expect(keepRejection(held('already_sold'), snapshot('SETTLED', 'i2'))).toBeNull()
    // The no-current-item snapshot clears it too: there is no body to hold it on.
    expect(keepRejection(held('already_sold'), snapshot('SETTLED', null))).toBeNull()
  })

  it('clears every other reason on the first snapshot, whatever the phase', () => {
    for (const reason of ['window_closed', 'exceeds_budget', 'below_minimum']) {
      const prev = held(reason)
      expect(keepRejection(prev, snapshot('SETTLED', 'i1'))).toBeNull()
      expect(keepRejection(prev, snapshot('LIVE', 'i1'))).toBeNull()
    }
    // An unknown reason is the generic sentence, not a held notice.
    expect(keepRejection(held('something_new'), snapshot('SETTLED', 'i1'))).toBeNull()
  })

  it('is the identity while no refusal is held', () => {
    expect(keepRejection(null, snapshot('SETTLED', 'i1'))).toBeNull()
  })
})
