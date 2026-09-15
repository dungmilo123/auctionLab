import { describe, expect, it } from 'vitest'
import {
  CLIENT_MESSAGE_TYPES,
  ENVELOPE_FIELDS,
  ERROR_CODES,
  PHASES,
  ROLES,
  SERVER_MESSAGE_TYPES,
  parseEnvelope,
  readAdminSnapshot,
  readBidAccepted,
  readBidRejected,
  readError,
  readHello,
  readPhaseChanged,
  readPlayerSnapshot,
  readPriceTick,
  type Envelope,
} from './protocol'

// The drift lock. Every literal below is copied by hand from
// internal/hub/protocol.go and internal/game/project.go — NOT imported from
// this module — so a rename on either side of the wire fails here instead of
// inside a classroom client. The point of the test is that a later
// "simplification" of the protocol cannot pass review.
const FROZEN_CLIENT_TYPES = [
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
]

const FROZEN_SERVER_TYPES = [
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
]

const FROZEN_ERROR_CODES = [
  'nickname_invalid',
  'nickname_taken',
  'token_invalid',
  'not_joined',
  'bad_message',
  'unsupported',
  'not_authorized',
]

const CANONICAL_FRAME = '{"type":"hello","seq":1,"rev":0,"ts":1757934000000,"data":{"role":"player"}}'

const CANONICAL_SNAPSHOT = {
  phase: 'LOBBY',
  revision: 4,
  server_ts: 1757934000000,
  me: { id: 'p1', nickname: 'An', budget: 1000000, total_value: 0, connected: true },
  lobby: [
    { nickname: 'An', connected: true },
    { nickname: 'Bình', connected: false },
  ],
  leaderboard: [{ rank: 1, nickname: 'An', total_value: 0, budget: 1000000 }],
  item: null,
}

describe('the frozen catalog', () => {
  it('carries exactly the five envelope fields, in order', () => {
    expect([...ENVELOPE_FIELDS]).toEqual(['type', 'seq', 'rev', 'ts', 'data'])
  })

  it('matches the frozen client message catalog literally', () => {
    expect([...CLIENT_MESSAGE_TYPES]).toEqual(FROZEN_CLIENT_TYPES)
  })

  it('matches the frozen server message catalog literally', () => {
    expect([...SERVER_MESSAGE_TYPES]).toEqual(FROZEN_SERVER_TYPES)
  })

  it('matches the frozen error codes literally', () => {
    expect([...ERROR_CODES]).toEqual(FROZEN_ERROR_CODES)
  })

  it('names exactly the three roles', () => {
    expect([...ROLES]).toEqual(['player', 'screen', 'admin'])
  })
})

describe('parseEnvelope', () => {
  it('decodes a canonical frame into its five fields', () => {
    expect(parseEnvelope(CANONICAL_FRAME)).toEqual({
      type: 'hello',
      seq: 1,
      rev: 0,
      ts: 1757934000000,
      data: { role: 'player' },
    })
  })

  it('accepts an already-decoded frame', () => {
    const frame: Envelope = { type: 'error', seq: 2, rev: 1, ts: 5, data: null }
    expect(parseEnvelope(frame)).toEqual(frame)
  })

  const malformed: Array<[label: string, input: unknown]> = [
    ['a string that is not JSON', 'not json at all'],
    ['JSON that is not an object', '"hello"'],
    ['an array', '[1,2,3]'],
    ['null', null],
    ['a frame with no type', { seq: 1, rev: 0, ts: 0, data: null }],
    ['an empty type', { type: '', seq: 1, rev: 0, ts: 0, data: null }],
    ['a non-numeric seq', { type: 'hello', seq: '1', rev: 0, ts: 0, data: null }],
    ['a missing rev', { type: 'hello', seq: 1, ts: 0, data: null }],
    ['a non-numeric ts', { type: 'hello', seq: 1, rev: 0, ts: null, data: null }],
  ]

  it.each(malformed)('rejects %s', (_label, input) => {
    expect(parseEnvelope(input)).toBeNull()
  })
})

describe('readHello', () => {
  it('reads the identity a player hello carries', () => {
    expect(
      readHello({ role: 'player', player: 'p1', nickname: 'An', token: 'ab'.repeat(32), rev: 3 }),
    ).toEqual({ role: 'player', playerId: 'p1', nickname: 'An', token: 'ab'.repeat(32), rev: 3 })
  })

  it('reads a screen hello that carries no identity', () => {
    expect(readHello({ role: 'screen', rev: 7 })).toEqual({
      role: 'screen',
      playerId: '',
      nickname: '',
      token: '',
      rev: 7,
    })
  })

  const malformed: Array<[label: string, input: unknown]> = [
    ['a null payload', null],
    ['a string payload', 'hello'],
    ['an unknown role', { role: 'host', rev: 0 }],
    ['a missing role', { rev: 0 }],
    ['a missing rev', { role: 'player' }],
  ]

  it.each(malformed)('rejects %s', (_label, input) => {
    expect(readHello(input)).toBeNull()
  })
})

describe('readError', () => {
  it('reads the typed error the server sends', () => {
    expect(readError({ code: 'nickname_taken', message: 'that nickname is already taken' })).toEqual({
      code: 'nickname_taken',
      message: 'that nickname is already taken',
    })
  })

  it('keeps an unknown code rather than dropping the answer', () => {
    expect(readError({ code: 'something_new', message: '' })?.code).toBe('something_new')
  })

  it('rejects a payload without a code', () => {
    expect(readError({ message: 'no code here' })).toBeNull()
    expect(readError(null)).toBeNull()
  })
})

describe('readPlayerSnapshot', () => {
  it('maps the snake_case wire keys onto camelCase fields', () => {
    expect(readPlayerSnapshot(CANONICAL_SNAPSHOT)).toEqual({
      phase: 'LOBBY',
      revision: 4,
      serverTs: 1757934000000,
      me: {
        id: 'p1',
        nickname: 'An',
        budget: 1000000,
        totalValue: 0,
        connected: true,
        // Phase 3's additive keys default to "never bid" when a Phase-2-shaped
        // me row arrives without them (D-67).
        lastBid: null,
        isTopBidder: false,
      },
      lobby: [
        { nickname: 'An', connected: true },
        { nickname: 'Bình', connected: false },
      ],
      leaderboard: [{ rank: 1, nickname: 'An', totalValue: 0, budget: 1000000 }],
      item: null,
    })
  })

  it('ignores extra fields a later phase adds (D-24 is additive)', () => {
    const withFutureField = { ...CANONICAL_SNAPSHOT, something_new: { deep: true } }
    expect(readPlayerSnapshot(withFutureField)?.revision).toBe(4)
  })

  const malformed: Array<[label: string, input: unknown]> = [
    ['a null payload', null],
    ['a snapshot with no me', { ...CANONICAL_SNAPSHOT, me: undefined }],
    ['a snapshot with no lobby', { ...CANONICAL_SNAPSHOT, lobby: undefined }],
    ['a me without a budget', { ...CANONICAL_SNAPSHOT, me: { id: 'p1', nickname: 'An' } }],
    ['a lobby that is not a list', { ...CANONICAL_SNAPSHOT, lobby: { nickname: 'An' } }],
    ['a revision that is not a number', { ...CANONICAL_SNAPSHOT, revision: 'four' }],
  ]

  it.each(malformed)('rejects %s', (_label, input) => {
    expect(readPlayerSnapshot(input)).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// Phase 3: the live fields, the reveal, the event payloads and the admin
// projection (D-54/D-56/D-57/D-67/D-68). Every literal here is copied by hand
// from internal/game/project.go.
// ---------------------------------------------------------------------------

const SERVER_TS = 1_757_934_000_000

const LIVE_ITEM = {
  id: 'i1',
  name: 'Bức tranh',
  description: 'Sơn dầu',
  auction_type: 'english',
  hint_min: 200_000,
  hint_max: 500_000,
  start_price: 100_000,
  status: 'live',
  current_price: 120_000,
  top_bidder: { nickname: 'An' },
  deadline_at: SERVER_TS + 30_000,
  min_next_bid: 130_000,
  extensions_used: 0,
}

const SETTLED_ITEM = {
  id: 'i1',
  name: 'Bức tranh',
  auction_type: 'english',
  start_price: 100_000,
  status: 'settled',
  current_price: 120_000,
  top_bidder: { nickname: 'An' },
  deadline_at: SERVER_TS + 30_000,
  extensions_used: 0,
  settle: {
    outcome: 'sold',
    winner: { nickname: 'An' },
    price: 120_000,
    truth_value: 200_000,
    budget_delta: 80_000,
  },
}

const UNSOLD_ITEM = {
  id: 'i2',
  name: 'Bình gốm',
  auction_type: 'english',
  start_price: 50_000,
  status: 'settled',
  extensions_used: 0,
  settle: { outcome: 'unsold', winner: null, price: 0, truth_value: 75_000, budget_delta: 0 },
}

/** A LIVE snapshot around one item, with the me/lobby the invariant needs. */
function liveSnapshot(item: unknown = LIVE_ITEM): Record<string, unknown> {
  return {
    ...CANONICAL_SNAPSHOT,
    phase: 'LIVE',
    revision: 9,
    server_ts: SERVER_TS,
    me: { id: 'p1', nickname: 'An', budget: 1_000_000, total_value: 0, connected: true },
    item,
  }
}

describe('PHASES', () => {
  it('names exactly the five PLAN §6 phases in order', () => {
    expect([...PHASES]).toEqual(['LOBBY', 'ITEM_INTRO', 'LIVE', 'SETTLED', 'FINALE'])
  })
})

describe('readPlayerSnapshot: the phase↔item invariant (UI-SPEC §Phase-body totality)', () => {
  it('decodes a LIVE english item with every live field', () => {
    expect(readPlayerSnapshot(liveSnapshot())?.item).toEqual({
      id: 'i1',
      name: 'Bức tranh',
      description: 'Sơn dầu',
      auctionType: 'english',
      hintMin: 200_000,
      hintMax: 500_000,
      startPrice: 100_000,
      status: 'live',
      currentPrice: 120_000,
      topBidder: 'An',
      deadlineAt: SERVER_TS + 30_000,
      minNextBid: 130_000,
      extensionsUsed: 0,
      settle: null,
    })
  })

  it('carries me.last_bid and me.is_top_bidder (D-67, additive)', () => {
    const snapshot = readPlayerSnapshot({
      ...liveSnapshot(),
      me: {
        id: 'p1',
        nickname: 'An',
        budget: 1_000_000,
        total_value: 0,
        connected: true,
        last_bid: 120_000,
        is_top_bidder: true,
      },
    })
    expect(snapshot?.me.lastBid).toBe(120_000)
    expect(snapshot?.me.isTopBidder).toBe(true)
  })

  it('reads a Phase-2 me row that predates those keys as "never bid"', () => {
    const snapshot = readPlayerSnapshot(CANONICAL_SNAPSHOT)
    expect(snapshot?.me.lastBid).toBeNull()
    expect(snapshot?.me.isTopBidder).toBe(false)
  })

  it('decodes the SETTLED reveal, winner and all (D-67/D-68)', () => {
    const snapshot = readPlayerSnapshot({
      ...liveSnapshot(SETTLED_ITEM),
      phase: 'SETTLED',
      item: SETTLED_ITEM,
    })
    expect(snapshot?.item?.settle).toEqual({
      outcome: 'sold',
      winner: 'An',
      price: 120_000,
      truthValue: 200_000,
      budgetDelta: 80_000,
    })
    // The minimum is not advertised after the window closed.
    expect(snapshot?.item?.minNextBid).toBeNull()
  })

  it('keeps winner null for an unsold item (ECON-03)', () => {
    const snapshot = readPlayerSnapshot({
      ...liveSnapshot(UNSOLD_ITEM),
      phase: 'SETTLED',
      item: UNSOLD_ITEM,
    })
    expect(snapshot?.item?.settle).toEqual({
      outcome: 'unsold',
      winner: null,
      price: 0,
      truthValue: 75_000,
      budgetDelta: 0,
    })
  })

  it('decodes a non-English LIVE item for its pending-type body (H13)', () => {
    // A dutch item that somehow reaches LIVE: the server still projects the
    // price fields it has, and the client must render `live.typePending`
    // rather than reject the whole snapshot and freeze the screen.
    const dutch = {
      id: 'i3',
      name: 'Đồng hồ',
      auction_type: 'dutch',
      start_price: 300_000,
      status: 'live',
      current_price: 300_000,
      min_next_bid: 300_000,
    }
    expect(readPlayerSnapshot({ ...liveSnapshot(dutch), item: dutch })?.item?.auctionType).toBe(
      'dutch',
    )
    // No deadline on the wire at all: still decodable.
    expect(readPlayerSnapshot({ ...liveSnapshot(dutch), item: { ...dutch, deadline_at: null } })).not.toBeNull()
  })

  const violations: Array<[label: string, phase: string, item: unknown]> = [
    [
      'an english LIVE item without a deadline',
      'LIVE',
      { ...LIVE_ITEM, deadline_at: undefined },
    ],
    [
      'an english LIVE item without a current price',
      'LIVE',
      { ...LIVE_ITEM, current_price: undefined },
    ],
    [
      'an english LIVE item without a next valid bid',
      'LIVE',
      { ...LIVE_ITEM, min_next_bid: undefined },
    ],
    ['a LIVE snapshot with no item at all', 'LIVE', null],
    ['an item whose status contradicts LIVE', 'LIVE', { ...LIVE_ITEM, status: 'intro' }],
    [
      'a SETTLED snapshot without the settle object',
      'SETTLED',
      (() => {
        const { settle: _settle, ...rest } = SETTLED_ITEM
        return rest
      })(),
    ],
    ['a SETTLED snapshot whose item is still live', 'SETTLED', { ...SETTLED_ITEM, status: 'live' }],
    ['an ITEM_INTRO snapshot with no item', 'ITEM_INTRO', null],
    ['an item whose status contradicts ITEM_INTRO', 'ITEM_INTRO', { ...LIVE_ITEM, status: 'pending' }],
    ['an unknown phase', 'PAUSED', LIVE_ITEM],
    ['a settle object before SETTLED (the leak boundary, D-67)', 'LIVE', { ...LIVE_ITEM, settle: SETTLED_ITEM.settle }],
    ['a LOBBY snapshot whose phase is missing', undefined as unknown as string, null],
  ]

  it.each(violations)('rejects %s', (_label, phase, item) => {
    const payload: Record<string, unknown> = liveSnapshot(item)
    if (phase === undefined) delete payload.phase
    else payload.phase = phase
    expect(readPlayerSnapshot(payload)).toBeNull()
  })

  it('accepts a LOBBY and a FINALE snapshot with a null item', () => {
    expect(readPlayerSnapshot({ ...CANONICAL_SNAPSHOT, phase: 'LOBBY' })?.phase).toBe('LOBBY')
    expect(
      readPlayerSnapshot({ ...CANONICAL_SNAPSHOT, phase: 'FINALE', item: null })?.item,
    ).toBeNull()
  })
})

describe('readPriceTick (D-56)', () => {
  it('decodes the timing frame both anchors need', () => {
    expect(
      readPriceTick({
        item_id: 'i1',
        current_price: 120_000,
        min_next_bid: 130_000,
        deadline_at: SERVER_TS + 30_000,
        remaining_ms: 29_750,
      }),
    ).toEqual({
      itemId: 'i1',
      currentPrice: 120_000,
      minNextBid: 130_000,
      deadlineAt: SERVER_TS + 30_000,
      remainingMs: 29_750,
    })
  })

  it('reads a tick without a deadline (the non-English shape) as nulls', () => {
    const tick = readPriceTick({ item_id: 'i1', current_price: 300_000, min_next_bid: 300_000 })
    expect(tick?.deadlineAt).toBeNull()
    expect(tick?.remainingMs).toBeNull()
  })

  const malformed: Array<[label: string, input: unknown]> = [
    ['a null payload', null],
    ['a tick without an item id', { current_price: 1, min_next_bid: 2 }],
    ['a tick without a current price', { item_id: 'i1', min_next_bid: 2 }],
    ['a tick without a next valid bid', { item_id: 'i1', current_price: 1 }],
    ['a stringly current price', { item_id: 'i1', current_price: '1', min_next_bid: 2 }],
  ]

  it.each(malformed)('rejects %s', (_label, input) => {
    expect(readPriceTick(input)).toBeNull()
  })
})

describe('readBidAccepted (D-54)', () => {
  it('decodes every number the client would otherwise compute', () => {
    expect(
      readBidAccepted({
        player_id: 'p1',
        nickname: 'An',
        amount: 130_000,
        current_price: 130_000,
        min_next_bid: 140_000,
        deadline_at: SERVER_TS + 30_000,
        extensions_used: 1,
      }),
    ).toEqual({
      playerId: 'p1',
      nickname: 'An',
      amount: 130_000,
      currentPrice: 130_000,
      minNextBid: 140_000,
      deadlineAt: SERVER_TS + 30_000,
      extensionsUsed: 1,
    })
  })

  it('rejects a bid frame without the numbers the UI renders', () => {
    expect(readBidAccepted({ nickname: 'An', amount: 1 })).toBeNull()
    expect(readBidAccepted(null)).toBeNull()
  })
})

describe('readBidRejected (D-53)', () => {
  it('decodes the private refusal', () => {
    expect(
      readBidRejected({ reason: 'below_minimum', min_next_bid: 130_000, budget: 1_000_000, amount: 120_000 }),
    ).toEqual({ reason: 'below_minimum', minNextBid: 130_000, budget: 1_000_000, amount: 120_000 })
  })

  it('reads a refusal with no minimum as null and rejects one with no reason', () => {
    expect(readBidRejected({ reason: 'window_closed', budget: 1_000_000, amount: 1 })?.minNextBid).toBeNull()
    expect(readBidRejected({ budget: 1_000_000, amount: 1 })).toBeNull()
  })
})

describe('readPhaseChanged (D-57)', () => {
  it('decodes the new phase together with its item', () => {
    expect(readPhaseChanged({ phase: 'LIVE', item: LIVE_ITEM })).toEqual({
      phase: 'LIVE',
      item: {
        id: 'i1',
        name: 'Bức tranh',
        description: 'Sơn dầu',
        auctionType: 'english',
        hintMin: 200_000,
        hintMax: 500_000,
        startPrice: 100_000,
        status: 'live',
        currentPrice: 120_000,
        topBidder: 'An',
        deadlineAt: SERVER_TS + 30_000,
        minNextBid: 130_000,
        extensionsUsed: 0,
        settle: null,
      },
    })
  })

  it('decodes an explicit null item (FINALE, D-65/H10)', () => {
    expect(readPhaseChanged({ phase: 'FINALE', item: null })).toEqual({ phase: 'FINALE', item: null })
  })

  it('rejects an unknown phase and a payload without one', () => {
    expect(readPhaseChanged({ phase: 'PAUSED', item: null })).toBeNull()
    expect(readPhaseChanged({ item: null })).toBeNull()
  })
})

describe('readAdminSnapshot (D-64/D-66)', () => {
  const ADMIN_ITEM = {
    id: 'i1',
    order_index: 0,
    name: 'Bức tranh',
    description: 'Sơn dầu',
    image_url: '',
    auction_type: 'english',
    truth_value: 200_000,
    hint_min: 200_000,
    hint_max: 500_000,
    start_price: 100_000,
    min_increment: 10_000,
    duration_ms: 30_000,
    anti_snipe_mode: 'extend',
    anti_snipe_window_ms: 10_000,
    max_extensions: 1,
    status: 'pending',
  }

  it('decodes the catalog row the editor round-trips, truth_value included', () => {
    const snapshot = readAdminSnapshot({
      phase: 'LOBBY',
      revision: 2,
      server_ts: SERVER_TS,
      config: { starting_budget: 1_000_000, currency: 'VND' },
      players: [],
      sealed_bids: [],
      lobby: [{ nickname: 'An', connected: true }],
      leaderboard: [],
      item: null,
      items: [ADMIN_ITEM],
    })
    expect(snapshot?.items).toEqual([
      {
        id: 'i1',
        orderIndex: 0,
        name: 'Bức tranh',
        description: 'Sơn dầu',
        imageUrl: '',
        auctionType: 'english',
        truthValue: 200_000,
        hintMin: 200_000,
        hintMax: 500_000,
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
    ])
    expect(snapshot?.serverTs).toBe(SERVER_TS)
  })

  it('applies the same phase↔item invariant to the admin projection', () => {
    const payload = {
      phase: 'LIVE',
      revision: 2,
      server_ts: SERVER_TS,
      lobby: [],
      leaderboard: [],
      items: [ADMIN_ITEM],
      item: { ...LIVE_ITEM, deadline_at: undefined },
    }
    expect(readAdminSnapshot(payload)).toBeNull()
  })

  it('rejects a payload with no items array', () => {
    expect(
      readAdminSnapshot({ phase: 'LOBBY', revision: 1, server_ts: SERVER_TS, lobby: [], leaderboard: [] }),
    ).toBeNull()
  })
})
