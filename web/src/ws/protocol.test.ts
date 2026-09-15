import { describe, expect, it } from 'vitest'
import {
  CLIENT_MESSAGE_TYPES,
  ENVELOPE_FIELDS,
  ERROR_CODES,
  ROLES,
  SERVER_MESSAGE_TYPES,
  parseEnvelope,
  readError,
  readHello,
  readPlayerSnapshot,
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
      me: { id: 'p1', nickname: 'An', budget: 1000000, totalValue: 0, connected: true },
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
