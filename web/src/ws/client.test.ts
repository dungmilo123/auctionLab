/**
 * The player socket's per-connection stream accounting (D-21 — closes GAP 2).
 *
 * `client.ts` is the one module that touches `window`, `document`,
 * `localStorage` and `WebSocket`, and this suite still runs in Vitest's `node`
 * environment: those four globals are stubbed narrowly for the duration of a
 * test and restored afterwards, so no jsdom and no new dependency is involved.
 *
 * The regression this pins: `connect()` kept ONE `GateState` for the whole
 * `openPlayerSocket` lifetime, although D-21 and `gate.ts` say the gate is one
 * per connection. The D-45 resume's replacement socket starts its server-side
 * stream at seq 1 again, so with the dead connection's high-water mark still in
 * place, its `hello` and every non-snapshot frame at or below that mark were
 * classified `duplicate` and silently dropped. Snapshots survived because they
 * are rev-gated rather than seq-gated, which is exactly why the loss was
 * invisible for identity and budget and only showed up as a frozen stream.
 */

import { afterEach, describe, expect, it, vi } from 'vitest'
import { initialGateState } from './gate'
import { openPlayerSocket, readStoredToken, resetStream } from './client'
import { openAdminSocket, type AdminConnection } from './admin'
import {
  readAdminSnapshot,
  readBidAccepted,
  readPlayerSnapshot,
  readPriceTick,
  type BidRejected,
  type PlayerSnapshot,
} from './protocol'
import { initialViewState, reduceView, type ViewFrame, type ViewState } from './view'
import type { PlayerConnection } from './client'
import type { ErrorPayload } from './protocol'

/** One server frame, exactly as the wire carries it (D-21). */
function envelope(type: string, seq: number, data: unknown, rev = 0, ts = 1_757_934_000_000): string {
  return JSON.stringify({ type, seq, rev, ts, data })
}

/** The wire clock every Phase-3 fixture is anchored to. */
const WS_TS = 1_757_934_000_000

/** A live English snapshot payload, exactly as `internal/game/project.go` builds it. */
function liveSnapshotData(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    phase: 'LIVE',
    revision: 9,
    server_ts: WS_TS,
    me: {
      id: 'p1',
      nickname: 'Milo',
      budget: 1_000_000,
      total_value: 0,
      connected: true,
      last_bid: 120_000,
      is_top_bidder: true,
    },
    lobby: [{ nickname: 'Milo', connected: true }],
    leaderboard: [{ rank: 1, nickname: 'Milo', total_value: 0, budget: 1_000_000 }],
    item: {
      id: 'i1',
      name: 'Bức tranh',
      auction_type: 'english',
      start_price: 100_000,
      status: 'live',
      current_price: 120_000,
      top_bidder: { nickname: 'Milo' },
      deadline_at: WS_TS + 30_000,
      min_next_bid: 130_000,
      extensions_used: 0,
    },
    ...overrides,
  }
}

/** The admin projection the hub sends a role=admin connection. */
function adminSnapshotData(): Record<string, unknown> {
  return {
    phase: 'LOBBY',
    revision: 1,
    server_ts: WS_TS,
    config: { starting_budget: 1_000_000, currency: 'VND' },
    players: [],
    sealed_bids: [],
    lobby: [],
    leaderboard: [],
    item: null,
    items: [
      {
        id: 'i1',
        order_index: 0,
        name: 'Bức tranh',
        auction_type: 'english',
        truth_value: 200_000,
        start_price: 100_000,
        min_increment: 10_000,
        duration_ms: 30_000,
        anti_snipe_mode: 'extend',
        anti_snipe_window_ms: 10_000,
        max_extensions: 1,
        status: 'pending',
      },
    ],
  }
}

/** `reduceView`, with the "it applied" expectation made explicit. */
function reduceOrThrow(state: ViewState, frame: ViewFrame): ViewState {
  const next = reduceView(state, frame)
  expect(next).not.toBeNull()
  if (next === null) throw new Error('frame did not apply')
  return next
}

type Listener = () => void

/**
 * The recorded socket: just enough of `WebSocket` for `client.ts`, plus test
 * taps that play the server's half (the upgrade, a pushed frame, a drop).
 */
class FakeSocket {
  readonly url: string
  readyState = 0
  sent: string[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: { data: unknown }) => void) | null = null
  onclose: ((event: { code: number }) => void) | null = null
  onerror: (() => void) | null = null

  constructor(url: string) {
    this.url = url
  }

  send(data: string): void {
    // The browser throws InvalidStateError outside OPEN; `client.ts` guards on
    // readyState before every send, so a future path that forgets the guard
    // fails here instead of recording a frame the browser would never send.
    if (this.readyState !== 1) throw new Error('InvalidStateError: still CONNECTING or CLOSED')
    this.sent.push(data)
  }

  close(code = 1000): void {
    this.readyState = 3
    // The browser queues the close event rather than firing it inside close().
    // Queued, it is a no-op for today's callers — `client.ts` nulls onclose
    // before it closes a socket — but if a future edit moves that detach, the
    // spurious onclose becomes a visible `lost` status instead of a silent
    // pass.
    queueMicrotask(() => this.onclose?.({ code }))
  }

  /** Server side: the upgrade completed. */
  accept(): void {
    this.readyState = 1
    this.onopen?.()
  }

  /** Server side: one frame arrived. */
  deliver(raw: string): void {
    this.onmessage?.({ data: raw })
  }

  /** Server side: the socket died (network drop, 4001 supersede, ...). */
  drop(code: number): void {
    this.readyState = 3
    this.onclose?.({ code })
  }
}

/**
 * Install the four globals `client.ts` needs — and nothing else. The fakes
 * record what the module constructed and let the test fire the events the
 * browser would; `afterEach` restores the real (absent) globals.
 */
function installSocketEnvironment() {
  const sockets: FakeSocket[] = []
  const storage = new Map<string, string>()
  const listeners: Record<'document' | 'window', Map<string, Listener[]>> = {
    document: new Map(),
    window: new Map(),
  }

  function add(scope: 'document' | 'window') {
    return (type: string, listener: Listener): void => {
      const registered = listeners[scope].get(type) ?? []
      registered.push(listener)
      listeners[scope].set(type, registered)
    }
  }
  function remove(scope: 'document' | 'window') {
    return (type: string, listener: Listener): void => {
      const registered = listeners[scope].get(type) ?? []
      listeners[scope].set(
        type,
        registered.filter((entry) => entry !== listener),
      )
    }
  }
  function fire(scope: 'document' | 'window', type: string): void {
    for (const listener of [...(listeners[scope].get(type) ?? [])]) listener()
  }

  const fakeDocument = {
    visibilityState: 'visible',
    addEventListener: add('document'),
    removeEventListener: remove('document'),
  }
  const fakeWindow = {
    location: { protocol: 'http:', host: 'localhost:5173' },
    localStorage: {
      getItem: (key: string): string | null => storage.get(key) ?? null,
      setItem: (key: string, value: string): void => void storage.set(key, value),
      removeItem: (key: string): void => void storage.delete(key),
    },
    addEventListener: add('window'),
    removeEventListener: remove('window'),
  }

  class StubWebSocket extends FakeSocket {
    constructor(url: string) {
      super(url)
      sockets.push(this)
    }
  }

  vi.stubGlobal('document', fakeDocument)
  vi.stubGlobal('window', fakeWindow)
  vi.stubGlobal('WebSocket', StubWebSocket)

  return {
    sockets,
    storage,
    fireVisibility(state: 'visible' | 'hidden'): void {
      fakeDocument.visibilityState = state
      fire('document', 'visibilitychange')
    },
    firePageshow(): void {
      fire('window', 'pageshow')
    },
  }
}

describe('the per-connection stream account (D-21 — GAP 2)', () => {
  let connection: PlayerConnection | null = null

  afterEach(() => {
    connection?.close()
    connection = null
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('returns a fresh initial gate state from the reset seam', () => {
    expect(resetStream()).toEqual({ lastSeq: 0, rev: 0, resyncPending: false })
    expect(resetStream()).toEqual(initialGateState())
  })

  it('lets the replacement socket of a resume start from its own seq 1', () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    const errors: ErrorPayload[] = []
    vi.spyOn(console, 'warn').mockImplementation(() => {})

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: (error) => errors.push(error),
    })

    // ---- Connection one: the join form opens it, the player joins, and the
    // server's hello hands the identity token to storage (D-29).
    const first = env.sockets[0]
    expect(first).toBeDefined()
    if (first === undefined) return
    expect(first.url).toBe('ws://localhost:5173/ws') // same-origin by construction
    expect(statuses.at(-1)).toBe('connecting')
    first.accept()
    expect(statuses.at(-1)).toBe('open')
    expect(first.sent).toEqual([]) // no stored token yet — nothing to resume

    connection.join('Milo')
    expect(JSON.parse(first.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      nickname: 'Milo',
    })

    first.deliver(
      envelope('hello', 1, { role: 'player', player: 'p1', nickname: 'Milo', token: 'first-token', rev: 0 }),
    )
    expect(readStoredToken()).toBe('first-token')

    // Frames that push this connection's stream position well past 1 — the
    // exact state the replacement socket must not inherit.
    first.deliver(envelope('phase.changed', 7, null, 3))

    // ---- The socket dies (a locked phone), and D-45's visible/pageshow path
    // reopens it with the stored token.
    first.drop(1006)
    expect(statuses.at(-1)).toBe('lost')

    env.fireVisibility('visible')
    expect(env.sockets).toHaveLength(2)
    const second = env.sockets[1]
    expect(second).toBeDefined()
    if (second === undefined) return
    second.accept()
    expect(JSON.parse(second.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      token: 'first-token',
    })

    // ---- The replacement server stream starts over at seq 1 (D-21).
    second.deliver(
      envelope('hello', 1, {
        role: 'player',
        player: 'p1',
        nickname: 'Milo',
        token: 'second-token',
        rev: 0,
      }),
    )
    // With the dead connection's lastSeq still in place this hello classified
    // as `duplicate` and the replacement token never reached storage.
    expect(readStoredToken()).toBe('second-token')

    // A following NON-snapshot frame reaches its callback: the reset is a
    // stream reset, not snapshot-only recovery.
    second.deliver(envelope('error', 2, { code: 'not_joined', message: 'join first' }))
    expect(errors).toEqual([{ code: 'not_joined', message: 'join first' }])

    // ---- D-45 stays ONE attempt per visibility/pageshow event: an already
    // open socket is left alone by the next event.
    env.fireVisibility('visible')
    env.firePageshow()
    expect(env.sockets).toHaveLength(2)
  })
})

describe('the reduced view contract (D-57/D-70)', () => {
  let connection: PlayerConnection | null = null

  afterEach(() => {
    connection?.close()
    connection = null
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('hands onView exactly what reduceView makes of the decoded frames', () => {
    const env = installSocketEnvironment()
    const views: ViewState[] = []
    const snapshots: PlayerSnapshot[] = []
    const rejections: BidRejected[] = []
    vi.spyOn(console, 'warn').mockImplementation(() => {})

    connection = openPlayerSocket({
      onStatus: () => {},
      onSnapshot: (snapshot) => snapshots.push(snapshot),
      onError: () => {},
      onView: (view) => views.push(view),
      onBidRejected: (rejection) => rejections.push(rejection),
    })

    const socket = env.sockets[0]
    expect(socket).toBeDefined()
    if (socket === undefined) return
    socket.accept()
    connection.join('Milo')
    socket.deliver(
      envelope('hello', 1, { role: 'player', player: 'p1', nickname: 'Milo', token: 'tok', rev: 0 }),
    )

    // ---- A snapshot seeds the view (and the Phase-2 callback still fires).
    const payload = liveSnapshotData()
    socket.deliver(envelope('state.snapshot', 2, payload, 9))
    const snapshot = readPlayerSnapshot(payload)
    expect(snapshot).not.toBeNull()
    if (snapshot === null) return
    const afterSnapshot = reduceOrThrow(initialViewState(), { kind: 'player-snapshot', snapshot })
    expect(views.at(-1)).toEqual(afterSnapshot)
    expect(snapshots).toEqual([snapshot])

    // ---- A tick re-anchors the countdown through the same contract (D-60).
    const tickData = {
      item_id: 'i1',
      current_price: 130_000,
      min_next_bid: 140_000,
      deadline_at: WS_TS + 30_000,
      remaining_ms: 29_500,
    }
    socket.deliver(envelope('price.tick', 3, tickData, 9, WS_TS + 500))
    const tick = readPriceTick(tickData)
    expect(tick).not.toBeNull()
    if (tick === null) return
    const afterTick = reduceOrThrow(afterSnapshot, { kind: 'price.tick', tick, ts: WS_TS + 500 })
    expect(views.at(-1)).toEqual(afterTick)
    expect(views.at(-1)?.live?.serverTs).toBe(WS_TS + 500)

    // ---- An accepted bid patches the price and my own row (D-67/D-72).
    const bidData = {
      player_id: 'p1',
      nickname: 'Milo',
      amount: 130_000,
      current_price: 130_000,
      min_next_bid: 140_000,
      deadline_at: WS_TS + 30_000,
      extensions_used: 0,
    }
    socket.deliver(envelope('bid.accepted', 4, bidData, 9, WS_TS + 800))
    const bid = readBidAccepted(bidData)
    expect(bid).not.toBeNull()
    if (bid === null) return
    const afterBid = reduceOrThrow(afterTick, { kind: 'bid.accepted', bid, ts: WS_TS + 800 })
    expect(views.at(-1)).toEqual(afterBid)
    expect(views.at(-1)?.me?.lastBid).toBe(130_000)
    expect(views.at(-1)?.me?.isTopBidder).toBe(true)
    // Events never masquerade as snapshots.
    expect(snapshots).toHaveLength(1)

    // ---- D-53's private refusal reaches its submitter as feedback, not state.
    socket.deliver(
      envelope('bid.rejected', 5, {
        reason: 'below_minimum',
        min_next_bid: 140_000,
        budget: 1_000_000,
        amount: 130_000,
      }),
    )
    expect(rejections).toEqual([
      { reason: 'below_minimum', minNextBid: 140_000, budget: 1_000_000, amount: 130_000 },
    ])
  })

  it('never hands onView a frame the decoder refused, or one from another phase', () => {
    const env = installSocketEnvironment()
    const views: ViewState[] = []
    vi.spyOn(console, 'warn').mockImplementation(() => {})

    connection = openPlayerSocket({
      onStatus: () => {},
      onSnapshot: () => {},
      onError: () => {},
      onView: (view) => views.push(view),
    })

    const socket = env.sockets[0]
    expect(socket).toBeDefined()
    if (socket === undefined) return
    socket.accept()

    // A LIVE snapshot whose english item carries no deadline: rejected by the
    // decoder, so the last known view (none yet) stays and onView never fires.
    socket.deliver(
      envelope(
        'state.snapshot',
        1,
        liveSnapshotData({
          item: {
            id: 'i1',
            name: 'Bức tranh',
            auction_type: 'english',
            start_price: 100_000,
            status: 'live',
            current_price: 120_000,
            min_next_bid: 130_000,
          },
        }),
        9,
      ),
    )
    expect(views).toHaveLength(0)

    // An unknown phase is refused at the decoder too.
    socket.deliver(envelope('phase.changed', 2, { phase: 'PAUSED', item: null }, 10))
    expect(views).toHaveLength(0)

    // A well-formed frame in a phase it does not belong to is refused by the
    // reducer: a tick while the view is still LOBBY.
    socket.deliver(
      envelope(
        'price.tick',
        3,
        { item_id: 'i1', current_price: 1, min_next_bid: 2, deadline_at: WS_TS },
        10,
        WS_TS,
      ),
    )
    expect(views).toHaveLength(0)
  })
})

describe('the admin connection (D-64/H17)', () => {
  let admin: AdminConnection | null = null

  afterEach(() => {
    admin?.close()
    admin = null
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('joins as admin, reduces its own projection and stores no token', () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    const views: ViewState[] = []

    admin = openAdminSocket({
      onStatus: (status) => statuses.push(status),
      onView: (view) => views.push(view),
      onError: () => {},
    })

    const socket = env.sockets[0]
    expect(socket).toBeDefined()
    if (socket === undefined) return
    expect(socket.url).toBe('ws://localhost:5173/ws')
    expect(statuses.at(-1)).toBe('connecting')

    socket.accept()
    expect(statuses.at(-1)).toBe('open')
    expect(JSON.parse(socket.sent.at(-1) ?? 'null')).toEqual({ type: 'join', role: 'admin' })

    socket.deliver(envelope('hello', 1, { role: 'admin', rev: 0 }))
    const payload = adminSnapshotData()
    socket.deliver(envelope('state.snapshot', 2, payload, 1))
    const snapshot = readAdminSnapshot(payload)
    expect(snapshot).not.toBeNull()
    if (snapshot === null) return
    expect(views.at(-1)).toEqual(
      reduceOrThrow(initialViewState(), { kind: 'admin-snapshot', snapshot }),
    )
    expect(views.at(-1)?.items[0]?.truthValue).toBe(200_000)
    expect(views.at(-1)?.me).toBeNull()

    // The presenter's command API sends the D-65 frames on the same socket.
    admin.send({ type: 'admin.item.start', id: 'i1' })
    expect(JSON.parse(socket.sent.at(-1) ?? 'null')).toEqual({ type: 'admin.item.start', id: 'i1' })

    // No localStorage path at all: nothing was written, and there is no
    // identity to resume with, so a drop is not re-opened by visibility.
    expect(env.storage.size).toBe(0)
    socket.drop(1006)
    expect(statuses.at(-1)).toBe('lost')
    env.fireVisibility('visible')
    env.firePageshow()
    expect(env.sockets).toHaveLength(1)
  })
})
