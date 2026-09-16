import { describe, expect, it } from 'vitest'
import type {
  AdminSnapshot,
  BidAccepted,
  PhaseChanged,
  PlayerMe,
  PlayerSnapshot,
  PriceTick,
  PublicItem,
  RecentBid,
  ScreenSnapshot,
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
    startedAt: null,
    floorPrice: null,
    decrementStep: null,
    decrementIntervalMs: null,
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
    rank: 0,
    // Phase 6 (D-115): an absent `my_bid` is "not submitted".
    myBid: null,
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
    // Phase 6 (D-116): the player's own count-only counter, an explicit null for
    // every non-sealed item.
    sealedProgress: null,
    ...overrides,
  }
}

/** The sealed LIVE item the decoder admits: a deadline and no ladder at all. */
function sealedItem(overrides: Partial<PublicItem> = {}): PublicItem {
  return publicItem({
    id: 'i1',
    name: 'Tranh kín',
    auctionType: 'sealed_first',
    currentPrice: null,
    topBidder: null,
    deadlineAt: TS + 45_000,
    minNextBid: null,
    extensionsUsed: null,
    ...overrides,
  })
}

/** The `/screen` projection (D-87/D-88). */
function screenSnapshot(overrides: Partial<ScreenSnapshot> = {}): ScreenSnapshot {
  return {
    phase: 'LOBBY',
    revision: 4,
    serverTs: TS,
    joinUrl: 'http://192.168.1.10:8080',
    lobby: [{ nickname: 'An', connected: true }],
    leaderboard: [{ rank: 1, nickname: 'An', totalValue: 0, budget: 1_000_000 }],
    item: null,
    recentBids: [],
    sealedProgress: null,
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
      startedAt: null,
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
      joinUrl: '',
      recentBids: [],
      sealedProgress: null,
      sealedBids: [],
      config: null,
      players: [],
      joinUrls: [],
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
      config: { startingBudget: 1_000_000, currency: 'VND' },
      players: [
        {
          id: 'p1',
          nickname: 'An',
          budget: 1_000_000,
          totalValue: 0,
          joinedAt: TS - 60_000,
          lastSeenAt: TS,
          connected: true,
        },
      ],
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
      joinUrls: ['http://192.168.1.10:8080'],
      selectedJoinUrl: 'http://192.168.1.10:8080',
      sealedBids: [],
      sealedProgress: null,
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
      // An English run's anchor is the deadline pair; no Dutch clock rides along.
      startedAt: null,
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

describe('the Dutch live overlay (D-94)', () => {
  const DUTCH_STARTED_AT = TS - 4_000

  /** The Dutch LIVE item the decoder admits: a price, its clock, no ladder. */
  function dutchItem(overrides: Partial<PublicItem> = {}): PublicItem {
    return publicItem({
      auctionType: 'dutch',
      currentPrice: 280_000,
      minNextBid: null,
      deadlineAt: null,
      topBidder: null,
      startedAt: DUTCH_STARTED_AT,
      floorPrice: 100_000,
      decrementStep: 20_000,
      decrementIntervalMs: 1_000,
      ...overrides,
    })
  }

  /** The view after one Dutch snapshot. */
  function dutch(overrides: Partial<PublicItem> = {}) {
    return applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ item: dutchItem(overrides) }),
    })
  }

  /** The server's Dutch beat: the derived price plus the clock it derived from. */
  function dutchTick(overrides: Partial<PriceTick> = {}): ViewFrame {
    return {
      kind: 'price.tick',
      ts: TS + 250,
      tick: {
        itemId: 'i1',
        currentPrice: 270_000,
        minNextBid: 0,
        deadlineAt: null,
        remainingMs: null,
        startedAt: DUTCH_STARTED_AT,
        ...overrides,
      },
    }
  }

  it('seeds a Dutch overlay from the snapshot alone — the price AND its anchor', () => {
    // A reload mid-Dutch paints the falling price from the snapshot alone
    // (D-63): the item's own clock plus the snapshot's server instant are the
    // whole anchor, and the ladder minimum a Dutch run never has reads as 0
    // (the wire's own "no ladder" value — nothing on this body reads it).
    expect(dutch().live).toEqual({
      itemId: 'i1',
      currentPrice: 280_000,
      minNextBid: 0,
      deadlineAt: null,
      topBidder: null,
      serverTs: TS,
      startedAt: DUTCH_STARTED_AT,
    })
  })

  it('re-anchors on a Dutch tick and never invents a ladder minimum', () => {
    const state = applied(dutch(), dutchTick())
    expect(state.live?.currentPrice).toBe(270_000)
    expect(state.live?.minNextBid).toBe(0)
    expect(state.live?.startedAt).toBe(DUTCH_STARTED_AT)
    expect(state.live?.serverTs).toBe(TS + 250)
  })

  it('keeps the anchor it holds when a tick omits started_at — the price never blanks', () => {
    // The carried rule for a frame with no timing in it (05-UI-SPEC §partial):
    // the body carries on interpolating from the anchor it already has.
    const carried = applied(
      dutch(),
      dutchTick({ currentPrice: 260_000, startedAt: null }),
    )
    expect(carried.live?.startedAt).toBe(DUTCH_STARTED_AT)
    expect(carried.live?.currentPrice).toBe(260_000)
  })

  it('admits no Dutch overlay without a price, and none without an anchor', () => {
    expect(dutch({ currentPrice: null }).live).toBeNull()
    expect(dutch({ startedAt: null }).live).toBeNull()
  })

  it('discards the Dutch overlay the moment the phase leaves LIVE', () => {
    const settled = applied(dutch(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({
        phase: 'SETTLED',
        item: dutchItem({
          status: 'settled',
          currentPrice: 230_000,
          settle: { outcome: 'sold', winner: 'An', price: 230_000, truthValue: 400_000, budgetDelta: 170_000 },
        }),
      }),
    })
    expect(settled.live).toBeNull()
    expect(settled.item?.settle?.price).toBe(230_000)
  })
})

describe('the sealed live overlay (D-116)', () => {
  /** The view after one sealed snapshot. */
  function sealed(overrides: Partial<PublicItem> = {}) {
    return applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ item: sealedItem(overrides) }),
    })
  }

  it('seeds a sealed overlay from the deadline alone — no price, no ladder (D-116)', () => {
    // A sealed run has no price and no minimum (D-116); the only number its body
    // renders is the countdown anchored on `deadline_at` + the frame's clock. The
    // overlay is what carries that anchor to the player body and the projector.
    expect(sealed().live).toEqual({
      itemId: 'i1',
      currentPrice: null,
      minNextBid: 0,
      deadlineAt: TS + 45_000,
      topBidder: null,
      serverTs: TS,
      startedAt: null,
    })
  })

  it('admits no sealed overlay without the deadline its body renders', () => {
    // The decoder already refuses this shape; the reducer is airtight too, so no
    // future path can paint a sealed body with a blank clock.
    expect(sealed({ deadlineAt: null }).live).toBeNull()
  })

  it('re-anchors a sealed overlay from the submit fan-out and discards it at SETTLED', () => {
    const reSnapshotted = applied(sealed(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({
        serverTs: TS + 2_000,
        item: sealedItem(),
        sealedProgress: { submitted: 1, total: 3 },
      }),
    })
    expect(reSnapshotted.live?.serverTs).toBe(TS + 2_000)
    expect(reSnapshotted.live?.deadlineAt).toBe(TS + 45_000)

    const settled = applied(reSnapshotted, {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({
        phase: 'SETTLED',
        item: sealedItem({
          status: 'settled',
          settle: { outcome: 'sold', winner: 'An', price: 320_000, truthValue: 400_000, budgetDelta: 80_000 },
        }),
      }),
    })
    expect(settled.live).toBeNull()
    expect(settled.item?.settle?.price).toBe(320_000)
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

  it('refuses an item.live payload a snapshot would have been refused for (WR-03)', () => {
    // A complete Dutch LIVE payload — the only shape the decoder and the
    // projector ever admit — seeds the view.
    const dutch = publicItem({
      auctionType: 'dutch',
      currentPrice: 100_000,
      topBidder: null,
      minNextBid: null,
      deadlineAt: null,
      startedAt: TS,
      floorPrice: 40_000,
      decrementStep: 10_000,
      decrementIntervalMs: 1_000,
    })
    const state = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ item: dutch }),
    })

    // The event's payload lost the mechanics the Dutch body derives its price
    // and its ACCEPT predicate from: reducing it would fall back to `?? 0` and
    // pin the phone on a number the server is not charging.
    const broken = publicItem({
      ...dutch,
      floorPrice: null,
      decrementStep: null,
      decrementIntervalMs: null,
    })
    expect(reduceView(state, { kind: 'item', type: 'item.live', item: broken, ts: TS })).toBeNull()

    // Nor may an event hand a Dutch item the deadline the type cannot have —
    // CR-01's freeze arrived through exactly this shape.
    const withDeadline = publicItem({ ...dutch, deadlineAt: TS + 30_000 })
    expect(reduceView(state, { kind: 'item', type: 'item.live', item: withDeadline, ts: TS })).toBeNull()

    // The gate is not a blanket refusal: the complete payload still patches.
    const patched = applied(state, { kind: 'item', type: 'item.live', item: dutch, ts: TS })
    expect(patched.item?.floorPrice).toBe(40_000)
  })

  it('refuses an item.settled payload that carries no reveal', () => {
    // SETTLED is the phase whose totality the payload must satisfy: a settled
    // item with no reveal would paint a reveal body with a hole in it.
    const noReveal = publicItem({ status: 'settled', minNextBid: null, settle: null })
    expect(reduceView(live(), { kind: 'item', type: 'item.settled', item: noReveal, ts: TS })).toBeNull()
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

describe('the projector view (D-87/D-88/D-89)', () => {
  it('replaces the whole view from one screen snapshot, extras included', () => {
    const feed = [
      { nickname: 'Bình', amount: 140_000, ts: TS + 900 },
      { nickname: 'An', amount: 130_000, ts: TS + 500 },
    ]
    const state = applied(initialViewState(), {
      kind: 'screen-snapshot',
      snapshot: screenSnapshot({
        phase: 'LIVE',
        item: publicItem(),
        recentBids: feed,
        sealedProgress: { submitted: 2, total: 3 },
      }),
    })
    expect(state.phase).toBe('LIVE')
    expect(state.me).toBeNull()
    expect(state.items).toEqual([])
    expect(state.item?.id).toBe('i1')
    expect(state.joinUrl).toBe('http://192.168.1.10:8080')
    expect(state.recentBids).toEqual(feed)
    expect(state.sealedProgress).toEqual({ submitted: 2, total: 3 })
    expect(state.live?.itemId).toBe('i1')
  })

  // UI-review finding 1: a bid produces no snapshot, so a projector that only
  // wrote `recentBids` in the screen-snapshot arm showed a live price beside a
  // feed frozen at the last snapshot. The live path is the same frame that
  // patches the price (D-88's "additive" rule).
  it('prepends an accepted bid to the feed while the projector runs (finding 1)', () => {
    const view = applied(initialViewState(), {
      kind: 'screen-snapshot',
      snapshot: screenSnapshot({
        phase: 'LIVE',
        item: publicItem(),
        recentBids: [{ nickname: 'An', amount: 120_000, ts: TS }],
      }),
    })

    const patched = applied(
      view,
      accepted({ nickname: 'Bình', amount: 140_000, currentPrice: 140_000, minNextBid: 150_000 }),
    )

    // The overlay moved with the same frame...
    expect(patched.live?.currentPrice).toBe(140_000)
    expect(patched.live?.minNextBid).toBe(150_000)
    expect(patched.live?.topBidder).toBe('Bình')
    // ...and the feed's newest row is the bid that just landed, stamped with the
    // envelope's server clock (the same instant the server's snapshot feed uses).
    expect(patched.recentBids).toEqual([
      { nickname: 'Bình', amount: 140_000, ts: TS + 500 },
      { nickname: 'An', amount: 120_000, ts: TS },
    ])
  })

  it('keeps the feed newest-first and capped at five, and the snapshot still replaces it (finding 1)', () => {
    const seed: RecentBid[] = [
      { nickname: 'S4', amount: 120_000, ts: TS + 400 },
      { nickname: 'S3', amount: 119_000, ts: TS + 300 },
      { nickname: 'S2', amount: 118_000, ts: TS + 200 },
      { nickname: 'S1', amount: 117_000, ts: TS + 100 },
    ]
    let view = applied(initialViewState(), {
      kind: 'screen-snapshot',
      snapshot: screenSnapshot({ phase: 'LIVE', item: publicItem(), recentBids: seed }),
    })

    for (let n = 1; n <= 3; n += 1) {
      view = applied(view, {
        kind: 'bid.accepted',
        ts: TS + 1_000 * n,
        bid: {
          playerId: `p${n}`,
          nickname: `Mới ${n}`,
          amount: 120_000 + n,
          currentPrice: 120_000 + n,
          minNextBid: 130_000 + n,
          deadlineAt: TS + 30_000,
          extensionsUsed: 0,
        },
      })
    }

    expect(view.recentBids.map((bid) => bid.nickname)).toEqual([
      'Mới 3',
      'Mới 2',
      'Mới 1',
      'S4',
      'S3',
    ])
    expect(view.recentBids.map((bid) => bid.ts)).toEqual([
      TS + 3_000,
      TS + 2_000,
      TS + 1_000,
      TS + 400,
      TS + 300,
    ])

    // A reload mid-LIVE paints the feed from the server's own list, never merged
    // with what the client patched.
    const reloaded = applied(view, {
      kind: 'screen-snapshot',
      snapshot: screenSnapshot({
        phase: 'LIVE',
        item: publicItem(),
        recentBids: [{ nickname: 'An', amount: 120_000, ts: TS }],
      }),
    })
    expect(reloaded.recentBids).toEqual([{ nickname: 'An', amount: 120_000, ts: TS }])
  })

  it('resets the role extras when the next snapshot belongs to another role', () => {
    const screenView = applied(initialViewState(), {
      kind: 'screen-snapshot',
      snapshot: screenSnapshot({
        phase: 'LIVE',
        item: publicItem(),
        recentBids: [{ nickname: 'An', amount: 120_000, ts: TS }],
        sealedProgress: { submitted: 1, total: 3 },
      }),
    })
    expect(screenView.joinUrl).not.toBe('')

    const player = applied(screenView, {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ sealedProgress: { submitted: 1, total: 3 } }),
    })
    expect(player.me?.id).toBe('p1')
    // The screen-only extras reset...
    expect(player.joinUrl).toBe('')
    expect(player.recentBids).toEqual([])
    // ...and the presenter's ledger is the presenter's alone (D-117): a player
    // snapshot never carries one.
    expect(player.sealedBids).toEqual([])
    // D-116 is the exception: the count is the ONE sealed fact every role reads,
    // so the player branch carries the server's own counter rather than nulling
    // it (the deliberate move of the Phase-3/4 rule that forced it to null).
    expect(player.sealedProgress).toEqual({ submitted: 1, total: 3 })
  })

  it('patches the standings from a finale frame, in server order', () => {
    const standings = [
      { rank: 1, nickname: 'Bình', totalValue: 200_000, budget: 900_000 },
      { rank: 2, nickname: 'An', totalValue: 0, budget: 1_000_000 },
      { rank: 3, nickname: 'Cường', totalValue: 0, budget: 800_000 },
      { rank: 4, nickname: 'Hạnh', totalValue: 0, budget: 700_000 },
    ]
    const state = applied(initialViewState(), {
      kind: 'finale',
      payload: { podium: standings.slice(0, 3), standings },
      ts: TS + 5_000,
    })
    expect(state.leaderboard).toEqual(standings)
  })

  it('keeps the snapshot authoritative after a finale frame', () => {
    const patched = applied(initialViewState(), {
      kind: 'finale',
      payload: {
        podium: [{ rank: 1, nickname: 'Bình', totalValue: 200_000, budget: 900_000 }],
        standings: [{ rank: 1, nickname: 'Bình', totalValue: 200_000, budget: 900_000 }],
      },
      ts: TS,
    })
    const replaced = applied(patched, {
      kind: 'screen-snapshot',
      snapshot: screenSnapshot({ phase: 'FINALE' }),
    })
    expect(replaced.leaderboard).toEqual([
      { rank: 1, nickname: 'An', totalValue: 0, budget: 1_000_000 },
    ])
  })
})

describe('the presenter view (D-80/D-81/D-83)', () => {
  function presenterSnapshot(overrides: Partial<AdminSnapshot> = {}): AdminSnapshot {
    return {
      phase: 'LOBBY',
      revision: 3,
      serverTs: TS,
      config: { startingBudget: 2_000_000, currency: 'VND' },
      players: [
        {
          id: 'p1',
          nickname: 'An',
          budget: 1_000_000,
          totalValue: 0,
          joinedAt: TS - 60_000,
          lastSeenAt: TS,
          connected: true,
        },
      ],
      items: [],
      item: null,
      lobby: [{ nickname: 'An', connected: true }],
      leaderboard: [],
      joinUrls: ['http://192.168.1.10:8080', 'http://10.0.0.5:8080'],
      selectedJoinUrl: 'http://10.0.0.5:8080',
      sealedBids: [],
      sealedProgress: null,
      ...overrides,
    }
  }

  it('carries the config, the player rows and both halves of the URL panel', () => {
    const state = applied(initialViewState(), {
      kind: 'admin-snapshot',
      snapshot: presenterSnapshot(),
    })
    expect(state.config).toEqual({ startingBudget: 2_000_000, currency: 'VND' })
    expect(state.players.map((player) => player.nickname)).toEqual(['An'])
    expect(state.joinUrls).toEqual(['http://192.168.1.10:8080', 'http://10.0.0.5:8080'])
    // `joinUrl` is the SELECTED URL, whichever role supplied it: the QR takes
    // the admin's `selected_join_url` and the screen's `join_url`.
    expect(state.joinUrl).toBe('http://10.0.0.5:8080')
  })

  it('resets the presenter rows when another role’s snapshot arrives', () => {
    const adminView = applied(initialViewState(), {
      kind: 'admin-snapshot',
      snapshot: presenterSnapshot(),
    })
    const player = applied(adminView, { kind: 'player-snapshot', snapshot: playerSnapshot() })
    expect(player.config).toBeNull()
    expect(player.players).toEqual([])
    expect(player.joinUrls).toEqual([])
    expect(player.joinUrl).toBe('')
  })

  it('carries the server-computed rank through a player snapshot (D-89)', () => {
    const state = applied(initialViewState(), {
      kind: 'player-snapshot',
      snapshot: playerSnapshot({ me: me({ rank: 3 }) }),
    })
    expect(state.me?.rank).toBe(3)
  })

  it('carries the presenter sealed ledger and the server count through an admin snapshot (D-117)', () => {
    const ledger = [
      {
        itemId: 'i6',
        playerId: 'p1',
        nickname: 'An',
        amount: 320_000,
        submittedAt: TS - 3_000,
        updatedAt: TS - 3_000,
      },
      {
        itemId: 'i6',
        playerId: 'p2',
        nickname: 'Bình',
        amount: 350_000,
        submittedAt: TS - 2_000,
        updatedAt: TS - 1_000,
      },
    ]
    const state = applied(initialViewState(), {
      kind: 'admin-snapshot',
      snapshot: presenterSnapshot({
        sealedBids: ledger,
        sealedProgress: { submitted: 2, total: 3 },
      }),
    })
    // The rows travel in the server's own order and are carried as sent; the
    // count is the server's own number, never `rows.length` recomputed here.
    expect(state.sealedBids).toEqual(ledger)
    expect(state.sealedProgress).toEqual({ submitted: 2, total: 3 })
    // The ledger is the presenter's alone: no `me` is invented for the admin.
    expect(state.me).toBeNull()

    // A non-sealed item's admin snapshot carries the same empty/absent shape —
    // the ledger is scoped to the current item by the server (D-117).
    const plain = applied(state, { kind: 'admin-snapshot', snapshot: presenterSnapshot() })
    expect(plain.sealedBids).toEqual([])
    expect(plain.sealedProgress).toBeNull()
  })
})
