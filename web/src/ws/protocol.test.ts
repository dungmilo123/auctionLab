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
  readFinale,
  readHello,
  readPhaseChanged,
  readPlayerSnapshot,
  readPriceTick,
  readScreenSnapshot,
  readTunnelStatus,
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
  'admin.player.release',
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
  // Phase 7 (D-132): the application-level heartbeat, appended INSIDE the
  // server→client group beside the other control-plane frame — the same
  // deliberate two-list edit internal/hub/protocol.go made.
  'heartbeat',
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
  // Phase 7 (D-138): the inbound command bucket's informational refusal.
  'rate_limited',
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

describe('readTunnelStatus (D-123)', () => {
  it('reads the two display facts a tunnel.status frame carries', () => {
    expect(readTunnelStatus({ url: 'https://abc.trycloudflare.com', healthy: true })).toEqual({
      url: 'https://abc.trycloudflare.com',
      healthy: true,
    })
  })

  it('keeps the declared off/restarting state instead of reading it as absent', () => {
    // `{url:"", healthy:false}` is a valid status (the tunnel is off or coming
    // back), so it decodes to the pair — the panel renders its waiting word
    // from it rather than inventing an empty state (UI-SPEC §Copywriting).
    expect(readTunnelStatus({ url: '', healthy: false })).toEqual({ url: '', healthy: false })
  })

  it('ignores a malformed frame — display data is never a rejection', () => {
    expect(readTunnelStatus(null)).toBeNull()
    expect(readTunnelStatus({})).toBeNull()
    expect(readTunnelStatus({ url: 1, healthy: true })).toBeNull()
    expect(readTunnelStatus({ url: '', healthy: 'yes' })).toBeNull()
    expect(readTunnelStatus({ healthy: true })).toBeNull()
    expect(readTunnelStatus('tunnel.status')).toBeNull()
    expect(readTunnelStatus([{ url: '', healthy: true }])).toBeNull()
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
        // me row arrives without them (D-67), and Phase 4's `rank` defaults to
        // 0 = "no standing" for the same reason (D-89).
        lastBid: null,
        isTopBidder: false,
        rank: 0,
        // Phase 6 (D-115): the same additive shape — an absent `my_bid` is the
        // wire's "not submitted", never a rejection.
        myBid: null,
      },
      lobby: [
        { nickname: 'An', connected: true },
        { nickname: 'Bình', connected: false },
      ],
      leaderboard: [{ rank: 1, nickname: 'An', totalValue: 0, budget: 1000000 }],
      item: null,
      // Phase 6 (D-116): the player's own copy of the count-only counter; an
      // absent key is "not a sealed item / this build does not send one".
      sealedProgress: null,
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

// Phase 5 (D-92/D-94): the Dutch LIVE item, hand-copied from
// internal/game/project.go's PublicItem — the five numbers its clock needs and
// `min_next_bid`/`deadline_at` absent entirely (a Dutch run has no ladder and no
// deadline; an absent key is how the client tells that, D-98).
const LIVE_DUTCH_ITEM = {
  id: 'i4',
  name: 'Đồng hồ',
  auction_type: 'dutch',
  start_price: 300_000,
  status: 'live',
  current_price: 280_000,
  started_at: SERVER_TS - 4_000,
  floor_price: 100_000,
  decrement_step: 20_000,
  decrement_interval_ms: 1_000,
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

// Phase 6 (D-115/D-116/D-117): the sealed wire literals, hand-copied from
// internal/game/project.go. A sealed LIVE item carries its deadline and NO
// ladder field at all — `current_price`, `min_next_bid`, `top_bidder` and
// `extensions_used` are absent by type (`omitempty` server-side), which is what
// the sealed arm of the totality table must tolerate.
const LIVE_SEALED_ITEM = {
  id: 'i6',
  name: 'Tranh kín',
  description: '',
  auction_type: 'sealed_first',
  hint_min: 200_000,
  hint_max: 500_000,
  start_price: 300_000,
  status: 'live',
  deadline_at: SERVER_TS + 45_000,
}

/** D-117's presenter ledger rows, in the server's own arrival order. */
const WIRE_SEALED_BIDS = [
  {
    item_id: 'i6',
    player_id: 'p1',
    nickname: 'An',
    amount: 320_000,
    submitted_at: SERVER_TS - 3_000,
    updated_at: SERVER_TS - 3_000,
  },
  {
    item_id: 'i6',
    player_id: 'p2',
    nickname: 'Bình',
    amount: 350_000,
    submitted_at: SERVER_TS - 2_000,
    updated_at: SERVER_TS - 1_000,
  },
]

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
      // Phase 5's Dutch clock is absent on an English item — the keys are
      // decoded as null, never invented (D-94).
      startedAt: null,
      floorPrice: null,
      decrementStep: null,
      decrementIntervalMs: null,
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
    // D-89's rank is the same shape of additive key: absent means 0, not a
    // rejection.
    expect(snapshot?.me.rank).toBe(0)
    // Phase 6 (D-115): and `my_bid` is the third — absent means "not submitted",
    // which is exactly what a player who has not opened the sealed form has.
    expect(snapshot?.me.myBid).toBeNull()
  })

  it('decodes the server-computed rank when the row carries one (D-89/ECON-05)', () => {
    const snapshot = readPlayerSnapshot({
      ...liveSnapshot(),
      me: {
        id: 'p1',
        nickname: 'An',
        budget: 1_000_000,
        total_value: 0,
        connected: true,
        rank: 2,
      },
    })
    expect(snapshot?.me.rank).toBe(2)
  })

  const malformedRanks: Array<[label: string, rank: unknown]> = [
    ['a string rank', 'two'],
    ['a negative rank', -1],
    ['a fractional rank', 1.5],
    ['a null rank', null],
    ['a non-finite rank', Number.POSITIVE_INFINITY],
  ]

  it.each(malformedRanks)('rejects a me row whose rank is %s', (_label, rank) => {
    const payload = {
      ...liveSnapshot(),
      me: {
        id: 'p1',
        nickname: 'An',
        budget: 1_000_000,
        total_value: 0,
        connected: true,
        rank,
      },
    }
    expect(readPlayerSnapshot(payload)).toBeNull()
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

  it('requires the deadline a sealed LIVE body renders (D-116)', () => {
    // Phase 6 retires H13's leniency for the two sealed types: their LIVE body
    // renders a clock from `deadline_at`, so an admitted sealed snapshot must
    // carry it. `current_price` and `min_next_bid` stay optional — a sealed run
    // has no ladder. Dutch is stricter still (its five clock numbers above), and
    // the leniency survives only for a type this build does not know at all.
    expect(readPlayerSnapshot(liveSnapshot(LIVE_SEALED_ITEM))?.item?.auctionType).toBe('sealed_first')

    const { deadline_at: _deadline, ...noDeadline } = LIVE_SEALED_ITEM
    expect(readPlayerSnapshot({ ...liveSnapshot(noDeadline), item: noDeadline })).toBeNull()
  })

  it('decodes my_bid and the player sealed counter, and reads both absences as null (D-115/D-116)', () => {
    const withBid = {
      ...liveSnapshot(LIVE_SEALED_ITEM),
      sealed_progress: { submitted: 1, total: 3 },
      me: {
        id: 'p1',
        nickname: 'An',
        budget: 1_000_000,
        total_value: 0,
        connected: true,
        my_bid: 320_000,
      },
    }
    const snapshot = readPlayerSnapshot(withBid)
    // The owner reads exactly their OWN effective amount; the count is the one
    // sealed fact that travels to everyone (D-115/D-116).
    expect(snapshot?.me.myBid).toBe(320_000)
    expect(snapshot?.sealedProgress).toEqual({ submitted: 1, total: 3 })

    // A player who never submitted has no `my_bid` at all, and a non-sealed item
    // carries an explicit null counter — neither is a rejection (D-24 additive).
    const plain = readPlayerSnapshot(liveSnapshot(LIVE_SEALED_ITEM))
    expect(plain?.me.myBid).toBeNull()
    expect(plain?.sealedProgress).toBeNull()
  })

  const MISSING_CLOCK_FIELDS: Array<[label: string, field: string]> = [
    ['a Dutch LIVE item without a current price', 'current_price'],
    ['a Dutch LIVE item without a floor price', 'floor_price'],
    ['a Dutch LIVE item without a decrement step', 'decrement_step'],
    ['a Dutch LIVE item without a decrement interval', 'decrement_interval_ms'],
    ['a Dutch LIVE item without its clock anchor', 'started_at'],
  ]

  it.each(MISSING_CLOCK_FIELDS)('refuses %s', (_label, field) => {
    const partial = { ...LIVE_DUTCH_ITEM } as Record<string, unknown>
    delete partial[field]
    expect(readPlayerSnapshot({ ...liveSnapshot(partial), item: partial })).toBeNull()
  })

  it('refuses a Dutch LIVE item that carries a deadline it cannot have (D-93)', () => {
    const withDeadline = { ...LIVE_DUTCH_ITEM, deadline_at: SERVER_TS + 30_000 }
    expect(readPlayerSnapshot({ ...liveSnapshot(withDeadline), item: withDeadline })).toBeNull()
  })

  it('admits the complete Dutch LIVE item, clock and mechanics decoded (D-94)', () => {
    expect(readPlayerSnapshot(liveSnapshot(LIVE_DUTCH_ITEM))?.item).toEqual({
      id: 'i4',
      name: 'Đồng hồ',
      description: '',
      auctionType: 'dutch',
      hintMin: null,
      hintMax: null,
      startPrice: 300_000,
      status: 'live',
      currentPrice: 280_000,
      // A Dutch item advertises no ladder and no deadline at all (D-98): the
      // keys are absent on the wire, so both read as null.
      topBidder: null,
      deadlineAt: null,
      minNextBid: null,
      extensionsUsed: null,
      startedAt: SERVER_TS - 4_000,
      floorPrice: 100_000,
      decrementStep: 20_000,
      decrementIntervalMs: 1_000,
      settle: null,
    })
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
    [
      'a sealed LIVE item without the deadline its body renders (D-116)',
      'LIVE',
      (() => {
        const { deadline_at: _deadline, ...rest } = LIVE_SEALED_ITEM
        return rest
      })(),
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
      // An English beat has no Dutch anchor; the key is absent on the wire.
      startedAt: null,
    })
  })

  it('reads a tick without a deadline (the non-English shape) as nulls', () => {
    const tick = readPriceTick({ item_id: 'i1', current_price: 300_000, min_next_bid: 300_000 })
    expect(tick?.deadlineAt).toBeNull()
    expect(tick?.remainingMs).toBeNull()
    expect(tick?.startedAt).toBeNull()
  })

  it('carries the Dutch anchor a falling price re-anchors on (D-94)', () => {
    // The server's Dutch beat: current_price derived at the beat, started_at
    // for the re-anchor, no deadline and no remaining (D-91/D-93/D-94). The
    // ladder minimum rides along as its zero value because the frozen Phase-3
    // field has no `omitempty` — a Dutch body never reads it.
    const tick = readPriceTick({
      item_id: 'i4',
      current_price: 280_000,
      min_next_bid: 0,
      started_at: SERVER_TS - 4_000,
    })
    expect(tick).toEqual({
      itemId: 'i4',
      currentPrice: 280_000,
      minNextBid: 0,
      deadlineAt: null,
      remainingMs: null,
      startedAt: SERVER_TS - 4_000,
    })
  })

  it('reads a tick whose started_at is present but malformed as "no anchor"', () => {
    const tick = readPriceTick({ item_id: 'i4', current_price: 1, min_next_bid: 0, started_at: 'soon' })
    expect(tick?.startedAt).toBeNull()
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
        startedAt: null,
        floorPrice: null,
        decrementStep: null,
        decrementIntervalMs: null,
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

  // D-80's full row, hand-copied from internal/game/project.go — token included
  // on the wire, deliberately NOT decoded (T-04-04).
  const ADMIN_PLAYER = {
    id: 'p1',
    nickname: 'An',
    nickname_key: 'an',
    token: 'ab'.repeat(32),
    budget: 1_000_000,
    total_value: 0,
    joined_at: SERVER_TS - 60_000,
    last_seen_at: SERVER_TS,
    connected: true,
  }

  /** The admin projection as the hub sends it. */
  function adminSnapshotPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      phase: 'LOBBY',
      revision: 2,
      server_ts: SERVER_TS,
      config: { starting_budget: 1_000_000, currency: 'VND' },
      players: [ADMIN_PLAYER],
      sealed_bids: [],
      lobby: [{ nickname: 'An', connected: true }],
      leaderboard: [],
      item: null,
      items: [ADMIN_ITEM],
      join_urls: ['http://192.168.1.10:8080', 'http://10.0.0.5:8080'],
      selected_join_url: 'http://192.168.1.10:8080',
      ...overrides,
    }
  }

  it('decodes the catalog row the editor round-trips, truth_value included', () => {
    const snapshot = readAdminSnapshot(adminSnapshotPayload())
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

  it('decodes the presenter rows, the config and both halves of the URL panel', () => {
    const snapshot = readAdminSnapshot(adminSnapshotPayload())
    expect(snapshot?.config).toEqual({ startingBudget: 1_000_000, currency: 'VND' })
    expect(snapshot?.players).toEqual([
      {
        id: 'p1',
        nickname: 'An',
        budget: 1_000_000,
        totalValue: 0,
        joinedAt: SERVER_TS - 60_000,
        lastSeenAt: SERVER_TS,
        connected: true,
      },
    ])
    // T-04-04: the narrow row cannot express a token, so the panel can never
    // render (or spread) one.
    expect(Object.keys(snapshot?.players[0] ?? {}).sort()).toEqual([
      'budget',
      'connected',
      'id',
      'joinedAt',
      'lastSeenAt',
      'nickname',
      'totalValue',
    ])
    expect(snapshot?.joinUrls).toEqual(['http://192.168.1.10:8080', 'http://10.0.0.5:8080'])
    expect(snapshot?.selectedJoinUrl).toBe('http://192.168.1.10:8080')
  })

  it('decodes the presenter sealed ledger strictly and reads an absent list as empty (D-117)', () => {
    const snapshot = readAdminSnapshot(
      adminSnapshotPayload({
        phase: 'LIVE',
        item: LIVE_SEALED_ITEM,
        sealed_bids: WIRE_SEALED_BIDS,
        sealed_progress: { submitted: 2, total: 3 },
      }),
    )
    // The rows arrive in the server's own order and are decoded as sent: the
    // browser filters, sorts and counts nothing (D-117).
    expect(snapshot?.sealedBids).toEqual([
      {
        itemId: 'i6',
        playerId: 'p1',
        nickname: 'An',
        amount: 320_000,
        submittedAt: SERVER_TS - 3_000,
        updatedAt: SERVER_TS - 3_000,
      },
      {
        itemId: 'i6',
        playerId: 'p2',
        nickname: 'Bình',
        amount: 350_000,
        submittedAt: SERVER_TS - 2_000,
        updatedAt: SERVER_TS - 1_000,
      },
    ])
    expect(snapshot?.sealedProgress).toEqual({ submitted: 2, total: 3 })

    // An absent (or explicit null) list is "no ledger rows", never a rejection —
    // the additive-key rule every other optional key follows (D-24).
    const none = readAdminSnapshot(
      adminSnapshotPayload({ sealed_bids: undefined, sealed_progress: undefined }),
    )
    expect(none?.sealedBids).toEqual([])
    expect(none?.sealedProgress).toBeNull()
    expect(readAdminSnapshot(adminSnapshotPayload({ sealed_bids: null }))?.sealedBids).toEqual([])

    // ONE ill-typed row rejects the whole snapshot — the players/items rule: the
    // card renders the list as a whole, and a half-decoded ledger is one the
    // presenter cannot trust.
    expect(
      readAdminSnapshot(
        adminSnapshotPayload({ sealed_bids: [{ ...WIRE_SEALED_BIDS[0], amount: '320000' }] }),
      ),
    ).toBeNull()
    expect(
      readAdminSnapshot(
        adminSnapshotPayload({ sealed_bids: [{ ...WIRE_SEALED_BIDS[0], nickname: undefined }] }),
      ),
    ).toBeNull()
  })

  it('applies the same phase↔item invariant to the admin projection', () => {
    const payload = adminSnapshotPayload({
      phase: 'LIVE',
      item: { ...LIVE_ITEM, deadline_at: undefined },
    })
    expect(readAdminSnapshot(payload)).toBeNull()
  })

  const malformed: Array<[label: string, input: unknown]> = [
    ['a payload with no items array', adminSnapshotPayload({ items: undefined })],
    ['a payload with no config', adminSnapshotPayload({ config: undefined })],
    ['a config without a currency', adminSnapshotPayload({ config: { starting_budget: 1 } })],
    ['a payload with no players array', adminSnapshotPayload({ players: undefined })],
    [
      'a player row without a budget',
      adminSnapshotPayload({ players: [{ id: 'p1', nickname: 'An' }] }),
    ],
    ['a payload with no join URLs', adminSnapshotPayload({ join_urls: undefined })],
    ['a join URL that is not a string', adminSnapshotPayload({ join_urls: [1] })],
    ['a selected join URL that is not a string', adminSnapshotPayload({ selected_join_url: 7 })],
  ]

  it.each(malformed)('rejects %s', (_label, input) => {
    expect(readAdminSnapshot(input)).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// Phase 4: the screen projection and the reserved finale frame (D-87/D-88/D-89).
// Every literal here is copied by hand from internal/game/project.go.
// ---------------------------------------------------------------------------

/** The `/screen` projection as the hub sends it (D-87/D-88). */
function screenSnapshot(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    phase: 'LOBBY',
    revision: 4,
    server_ts: SERVER_TS,
    join_url: 'http://192.168.1.10:8080',
    lobby: [
      { nickname: 'An', connected: true },
      { nickname: 'Bình', connected: false },
    ],
    leaderboard: [{ rank: 1, nickname: 'An', total_value: 0, budget: 1_000_000 }],
    item: null,
    recent_bids: [],
    sealed_progress: null,
    ...overrides,
  }
}

describe('readScreenSnapshot (D-87/D-88)', () => {
  it('decodes the public snapshot, the selected join URL and the two extras', () => {
    expect(readScreenSnapshot(screenSnapshot())).toEqual({
      phase: 'LOBBY',
      revision: 4,
      serverTs: SERVER_TS,
      joinUrl: 'http://192.168.1.10:8080',
      lobby: [
        { nickname: 'An', connected: true },
        { nickname: 'Bình', connected: false },
      ],
      leaderboard: [{ rank: 1, nickname: 'An', totalValue: 0, budget: 1_000_000 }],
      item: null,
      recentBids: [],
      sealedProgress: null,
    })
  })

  it('decodes the feed in wire order and a count-only sealed counter', () => {
    const snapshot = readScreenSnapshot(
      screenSnapshot({
        phase: 'LIVE',
        item: LIVE_ITEM,
        recent_bids: [
          { nickname: 'Bình', amount: 140_000, ts: SERVER_TS + 900 },
          { nickname: 'An', amount: 130_000, ts: SERVER_TS + 500 },
        ],
        sealed_progress: { submitted: 3, total: 12 },
      }),
    )
    expect(snapshot?.recentBids).toEqual([
      { nickname: 'Bình', amount: 140_000, ts: SERVER_TS + 900 },
      { nickname: 'An', amount: 130_000, ts: SERVER_TS + 500 },
    ])
    expect(snapshot?.sealedProgress).toEqual({ submitted: 3, total: 12 })
  })

  it('drops a malformed feed entry and a malformed counter, and still applies the snapshot', () => {
    // The projector must never freeze on a convenience field (UI-SPEC): the
    // price, the countdown and the bidder line are the authoritative numbers.
    const snapshot = readScreenSnapshot(
      screenSnapshot({
        recent_bids: [
          { nickname: 'An', amount: 130_000, ts: SERVER_TS },
          { nickname: 'Bình', amount: '140000', ts: SERVER_TS },
          { amount: 150_000, ts: SERVER_TS },
        ],
        sealed_progress: { submitted: '3' },
      }),
    )
    expect(snapshot).not.toBeNull()
    expect(snapshot?.recentBids).toEqual([{ nickname: 'An', amount: 130_000, ts: SERVER_TS }])
    expect(snapshot?.sealedProgress).toBeNull()
  })

  it('reads an absent feed or counter as "this build does not send one"', () => {
    const snapshot = readScreenSnapshot(
      screenSnapshot({ recent_bids: undefined, sealed_progress: undefined }),
    )
    expect(snapshot?.recentBids).toEqual([])
    expect(snapshot?.sealedProgress).toBeNull()
  })

  const violations: Array<[label: string, input: unknown]> = [
    ['a null payload', null],
    ['a snapshot with no join URL', screenSnapshot({ join_url: undefined })],
    ['a join URL that is not a string', screenSnapshot({ join_url: null })],
    ['a snapshot with no lobby', screenSnapshot({ lobby: undefined })],
    ['a revision that is not a number', screenSnapshot({ revision: 'four' })],
    [
      'an english LIVE item without a deadline',
      screenSnapshot({ phase: 'LIVE', item: { ...LIVE_ITEM, deadline_at: undefined } }),
    ],
    [
      'a SETTLED snapshot without the settle object',
      screenSnapshot({
        phase: 'SETTLED',
        item: (() => {
          const { settle: _settle, ...rest } = SETTLED_ITEM
          return rest
        })(),
      }),
    ],
    [
      'a settle object before SETTLED (the leak boundary, D-67)',
      screenSnapshot({ phase: 'LIVE', item: { ...LIVE_ITEM, settle: SETTLED_ITEM.settle } }),
    ],
  ]

  it.each(violations)('rejects %s', (_label, input) => {
    expect(readScreenSnapshot(input)).toBeNull()
  })

  it('accepts a FINALE screen snapshot exactly like the player route does', () => {
    expect(readScreenSnapshot(screenSnapshot({ phase: 'FINALE', item: null }))?.phase).toBe('FINALE')
  })
})

describe('readFinale (D-89)', () => {
  // The wire literals (hand-copied from internal/game/project.go) and their
  // decoded camelCase shape — the row decoder is readLeaderboard's, so the two
  // differ only in snake_case/camelCase.
  const WIRE_STANDINGS = [
    { rank: 1, nickname: 'Bình', total_value: 200_000, budget: 900_000 },
    { rank: 2, nickname: 'An', total_value: 0, budget: 1_000_000 },
    { rank: 3, nickname: 'Cường', total_value: 0, budget: 800_000 },
    { rank: 4, nickname: 'Hạnh', total_value: 0, budget: 700_000 },
  ]
  const STANDINGS = [
    { rank: 1, nickname: 'Bình', totalValue: 200_000, budget: 900_000 },
    { rank: 2, nickname: 'An', totalValue: 0, budget: 1_000_000 },
    { rank: 3, nickname: 'Cường', totalValue: 0, budget: 800_000 },
    { rank: 4, nickname: 'Hạnh', totalValue: 0, budget: 700_000 },
  ]

  it('decodes the podium prefix and the complete standings in server order', () => {
    const payload = readFinale({
      podium: WIRE_STANDINGS.slice(0, 3),
      standings: WIRE_STANDINGS,
    })
    expect(payload?.podium).toEqual(STANDINGS.slice(0, 3))
    expect(payload?.standings).toEqual(STANDINGS)
  })

  it('reads a zero-player finale as two empty lists, never null', () => {
    expect(readFinale({ podium: [], standings: [] })).toEqual({ podium: [], standings: [] })
  })

  it('rejects an incomplete payload rather than inventing half a ceremony', () => {
    expect(readFinale({ podium: [], standings: undefined })).toBeNull()
    expect(readFinale({ podium: 'first' })).toBeNull()
    expect(readFinale(null)).toBeNull()
  })
})
