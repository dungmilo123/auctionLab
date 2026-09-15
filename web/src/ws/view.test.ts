import { describe, expect, it } from 'vitest'
import type {
  AdminSnapshot,
  BidAccepted,
  PhaseChanged,
  PlayerMe,
  PlayerSnapshot,
  PriceTick,
  PublicItem,
} from './protocol'
import { initialViewState, reduceView, type ViewFrame } from './view'

// The reducer is pure, so every case is a decoded frame in and a whole view
// out — the rules RESEARCH Q3/Q4 names (snapshot replaces, events patch, the
// overlay is discarded when the phase leaves LIVE or the item changes, and
// `me.budget` only ever moves through a snapshot).
const TS = 1_757_934_000_000

function publicItem(overrides: Partial<PublicItem> = {}): PublicItem {
  return {
    id: 'i1',
    name: 'Bức tranh',
    description: '',
    auctionType: 'english',
    hintMin: null,
    hintMax: null,
    startPrice: 100_000,
    status: 'live',
    currentPrice: 120_000,
    topBidder: 'An',
    deadlineAt: TS + 30_000,
    minNextBid: 130_000,
    extensionsUsed: 0,
    settle: null,
    ...overrides,
  }
}

function me(overrides: Partial<PlayerMe> = {}): PlayerMe {
  return {
    id: 'p1',
    nickname: 'An',
    budget: 1_000_000,
    totalValue: 0,
    connected: true,
    lastBid: 120_000,
    isTopBidder: true,
    ...overrides,
  }
}

function playerSnapshot(overrides: Partial<PlayerSnapshot> = {}): PlayerSnapshot {
  return {
    phase: 'LIVE',
    revision: 9,
    serverTs: TS,
    me: me(),
    lobby: [{ nickname: 'An', connected: true }],
    leaderboard: [{ rank: 1, nickname: 'An', totalValue: 0, budget: 1_000_000 }],
    item: publicItem(),
    ...overrides,
  }
}

function tick(overrides: Partial<PriceTick> = {}): ViewFrame {
  return {
    kind: 'price.tick',
    ts: TS + 250,
    tick: {
      itemId: 'i1',
      currentPrice: 130_000,
      minNextBid: 140_000,
      deadlineAt: TS + 30_000,
      remainingMs: 29_750,
      ...overrides,
    },
  }
}

function accepted(overrides: Partial<BidAccepted> = {}): ViewFrame {
  return {
    kind: 'bid.accepted',
    ts: TS + 500,
    bid: {
      playerId: 'p1',
      nickname: 'An',
      amount: 130_000,
      currentPrice: 130_000,
      minNextBid: 140_000,
      deadlineAt: TS + 30_000,
      extensionsUsed: 0,
      ...overrides,
    },
  }
}

/** The view after one applied frame, or the state itself if `null` came back. */
function applied(state = initialViewState(), frame: ViewFrame) {
  const next = reduceView(state, frame)
  expect(next).not.toBeNull()
  return next ?? state
}

describe('initialViewState', () => {
  it('is the empty LOBBY view before a single snapshot arrives', () => {
    expect(initialViewState()).toEqual({
      phase: 'LOBBY',
      me: null,
      lobby: [],
      leaderboard: [],
      item: null,
      items: [],
      live: null,
    })
  })
})

describe('the snapshot rule (RESEARCH Q3/Q4)', () => {
  it('replaces the whole view, stale overlay included', () => {
    const stale = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ item: publicItem({ id: 'i2', name: 'Cũ' }) }),
    })
    expect(stale.live?.itemId).toBe('i2')

    const replaced = applied(stale, { kind: 'player-snapshot', snapshot: playerSnapshot() })
    expect(replaced.item?.id).toBe('i1')
    expect(replaced.live?.itemId).toBe('i1')
    expect(replaced.me?.budget).toBe(1_000_000)
  })

  it('is the only frame that moves me.budget (Q4)', () => {
    const afterSnapshot = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot(),
    })
    expect(afterSnapshot.me?.budget).toBe(1_000_000)

    // A tick and an accepted bid move the price and the minimum, never the
    // budget: only the settle's own snapshot may do that.
    const afterEvents = applied(applied(afterSnapshot, tick()), accepted())
    expect(afterEvents.me?.budget).toBe(1_000_000)

    const afterSettle = applied(afterEvents, {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({
        phase: 'SETTLED',
        me: me({ budget: 1_080_000, isTopBidder: true, lastBid: 120_000 }),
        item: publicItem({ status: 'settled', minNextBid: null, settle: { outcome: 'sold', winner: 'An', price: 120_000, truthValue: 200_000, budgetDelta: 80_000 } }),
      }),
    })
    expect(afterSettle.me?.budget).toBe(1_080_000)
    expect(afterSettle.live).toBeNull()
    expect(afterSettle.item?.settle?.truthValue).toBe(200_000)
  })

  it('heals a resync: the outbid state is replaced, not merged', () => {
    const outbid = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ me: me({ lastBid: 120_000, isTopBidder: false }) }),
    })
    expect(outbid.me?.isTopBidder).toBe(false)

    const healed = applied(outbid, {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ me: me({ lastBid: null, isTopBidder: true }) }),
    })
    expect(healed.me?.lastBid).toBeNull()
    expect(healed.me?.isTopBidder).toBe(true)
  })

  it('seeds no overlay for a LIVE item that carries no live fields', () => {
    const dutch = publicItem({
      auctionType: 'dutch',
      currentPrice: null,
      minNextBid: null,
      deadlineAt: null,
      topBidder: null,
    })
    const state = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ item: dutch }),
    })
    expect(state.item?.auctionType).toBe('dutch')
    expect(state.live).toBeNull()
  })

  it('takes an admin snapshot into the same view without inventing a me row', () => {
    const admin: AdminSnapshot = {
      phase: 'LOBBY',
      revision: 2,
      serverTs: TS,
      items: [
        {
          id: 'i1',
          orderIndex: 0,
          name: 'Bức tranh',
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
          durationMs: 30_000,
          antiSnipeMode: 'extend',
          antiSnipeWindowMs: 10_000,
          maxExtensions: 1,
          status: 'pending',
        },
      ],
      item: null,
      lobby: [{ nickname: 'An', connected: true }],
      leaderboard: [],
    }
    const state = applied(initialViewState(), { kind: 'admin-snapshot', snapshot: admin })
    expect(state.me).toBeNull()
    expect(state.items[0]?.truthValue).toBe(200_000)
    expect(state.phase).toBe('LOBBY')
  })
})

describe('the LIVE overlay (D-60/D-67)', () => {
  const live = () => applied(initialViewState(), { kind: 'player-snapshot', snapshot: playerSnapshot() })

  it('re-anchors on a tick that belongs to the current item', () => {
    const state = applied(live(), tick())
    expect(state.live).toEqual({
      itemId: 'i1',
      currentPrice: 130_000,
      minNextBid: 140_000,
      deadlineAt: TS + 30_000,
      topBidder: 'An',
      serverTs: TS + 250,
    })
  })

  it('ignores a tick for another item', () => {
    expect(reduceView(live(), tick({ itemId: 'i2' }))).toBeNull()
  })

  it('ignores a tick once the phase left LIVE', () => {
    const settled = applied(live(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ phase: 'SETTLED', item: publicItem({ status: 'settled', minNextBid: null }) }),
    })
    expect(reduceView(settled, tick())).toBeNull()
  })

  it('applies my accepted bid to the price, the bidder and my own row', () => {
    const mine = applied(live(), accepted())
    expect(mine.live?.currentPrice).toBe(130_000)
    expect(mine.live?.minNextBid).toBe(140_000)
    expect(mine.live?.topBidder).toBe('An')
    expect(mine.live?.serverTs).toBe(TS + 500)
    expect(mine.me?.lastBid).toBe(130_000)
    expect(mine.me?.isTopBidder).toBe(true)
  })

  it("keeps my last bid while another player's bid takes the lead", () => {
    const theirs = applied(live(), accepted({ playerId: 'p2', nickname: 'Bình', amount: 140_000 }))
    expect(theirs.live?.topBidder).toBe('Bình')
    expect(theirs.me?.lastBid).toBe(120_000) // unchanged: mine, from the run
    expect(theirs.me?.isTopBidder).toBe(false)
  })

  it('re-anchors the deadline when an accepted bid extended it (D-59)', () => {
    const extended = applied(live(), accepted({ deadlineAt: TS + 40_500 }))
    expect(extended.live?.deadlineAt).toBe(TS + 40_500)
    expect(extended.live?.serverTs).toBe(TS + 500)
  })

  it('ignores an accepted bid after the phase left LIVE', () => {
    const intro = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ phase: 'ITEM_INTRO', item: publicItem({ status: 'intro', currentPrice: null, minNextBid: null, deadlineAt: null, topBidder: null }) }),
    })
    expect(reduceView(intro, accepted())).toBeNull()
  })
})

describe('phase.changed and the item events (Q3)', () => {
  const live = () => applied(initialViewState(), { kind: 'player-snapshot', snapshot: playerSnapshot() })

  it('patches the phase and the item in one step', () => {
    const payload: PhaseChanged = { phase: 'SETTLED', item: publicItem({ status: 'settled', minNextBid: null }) }
    const state = applied(live(), { kind: 'phase.changed', payload, ts: TS + 1_000 })
    expect(state.phase).toBe('SETTLED')
    expect(state.item?.status).toBe('settled')
    expect(state.live).toBeNull()
  })

  it('discards the overlay when the item changes, and re-seeds from the new one', () => {
    const next = publicItem({ id: 'i2', name: 'Bình gốm' })
    const state = applied(live(), {
      kind: 'phase.changed',
      payload: { phase: 'LIVE', item: next },
      ts: TS + 2_000,
    })
    expect(state.item?.id).toBe('i2')
    expect(state.live?.itemId).toBe('i2')
    expect(state.live?.serverTs).toBe(TS + 2_000)
  })

  it('applies item.intro only while the phase is ITEM_INTRO', () => {
    const introItem = publicItem({ status: 'intro', currentPrice: null, minNextBid: null, deadlineAt: null, topBidder: null })
    expect(reduceView(live(), { kind: 'item', type: 'item.intro', item: introItem, ts: TS })).toBeNull()

    const intro = applied(initialViewState(), {
      kind: 'phase.changed',
      payload: { phase: 'ITEM_INTRO', item: introItem },
      ts: TS,
    })
    const patched = applied(intro, { kind: 'item', type: 'item.intro', item: { ...introItem, name: 'Mới' }, ts: TS })
    expect(patched.item?.name).toBe('Mới')
    expect(patched.item?.status).toBe('intro')
  })

  it('applies item.live only while the phase is LIVE', () => {
    const renamed = publicItem({ name: 'Bức tranh (live)' })
    const state = applied(live(), { kind: 'item', type: 'item.live', item: renamed, ts: TS })
    expect(state.item?.name).toBe('Bức tranh (live)')

    const intro = applied(initialViewState(), {
      kind: 'phase.changed',
      payload: { phase: 'ITEM_INTRO', item: publicItem({ status: 'intro' }) },
      ts: TS,
    })
    expect(reduceView(intro, { kind: 'item', type: 'item.live', item: renamed, ts: TS })).toBeNull()
  })

  it('patches item.settled without moving the phase itself', () => {
    const settledItem = publicItem({
      status: 'settled',
      minNextBid: null,
      settle: { outcome: 'sold', winner: 'An', price: 120_000, truthValue: 200_000, budgetDelta: 80_000 },
    })
    const state = applied(live(), { kind: 'item', type: 'item.settled', item: settledItem, ts: TS + 3_000 })
    // The settle's own snapshot (next in the same fan-out) is what moves the
    // body; the event itself only patches the item (Q3).
    expect(state.phase).toBe('LIVE')
    expect(state.item?.settle?.outcome).toBe('sold')
  })

  it('ignores an item event for a different item', () => {
    const other = publicItem({ id: 'i9' })
    expect(reduceView(live(), { kind: 'item', type: 'item.live', item: other, ts: TS })).toBeNull()
  })

  it('ignores a phase.changed that names no known phase', () => {
    expect(
      reduceView(live(), {
        kind: 'phase.changed',
        payload: { phase: 'PAUSED' as unknown as PhaseChanged['phase'], item: null },
        ts: TS,
      }),
    ).toBeNull()
  })

  it('replaces the standings on a leaderboard frame', () => {
    const entries = [{ rank: 1, nickname: 'Bình', totalValue: 200_000, budget: 900_000 }]
    const state = applied(live(), { kind: 'leaderboard', entries, ts: TS })
    expect(state.leaderboard).toEqual(entries)
  })
})
