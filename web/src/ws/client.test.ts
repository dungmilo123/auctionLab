/**
 * The socket lifecycle: the per-connection stream account (D-21 — closes
 * GAP 2) and the reconnect policy (D-130…D-134).
 *
 * `client.ts` is the one module that touches `window`, `document`,
 * `localStorage` and `WebSocket`, and this suite still runs in Vitest's `node`
 * environment: those four globals are stubbed narrowly for the duration of a
 * test and restored afterwards, so no jsdom and no new dependency is involved.
 * The policy's own numbers are pinned purely in `reconnect.test.ts`; this file
 * pins what the wiring does with them — one scheduler, one current socket, and
 * a replacement that always starts from its own stream position.
 *
 * The regression the stream account pins: `connect()` kept ONE `GateState` for
 * the whole `openPlayerSocket` lifetime, although D-21 and `gate.ts` say the
 * gate is one per connection. A replacement socket's server-side stream starts
 * at seq 1 again, so with the dead connection's high-water mark still in place,
 * its `hello` and every non-snapshot frame at or below that mark were
 * classified `duplicate` and silently dropped. Snapshots survived because they
 * are rev-gated rather than seq-gated, which is exactly why the loss was
 * invisible for identity and budget and only showed up as a frozen stream.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initialGateState } from './gate'
import { openPlayerSocket, readStoredToken, resetStream } from './client'
import { openAdminSocket, type AdminConnection } from './admin'
import { openScreenSocket, type ScreenConnection } from './screen'
import {
  readAdminSnapshot,
  readBidAccepted,
  readFinale,
  readPlayerSnapshot,
  readPriceTick,
  readScreenSnapshot,
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
    join_urls: ['http://192.168.1.10:8080'],
    selected_join_url: 'http://192.168.1.10:8080',
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

/** The screen projection the hub sends a role=screen connection (D-87/D-88). */
function screenSnapshotData(): Record<string, unknown> {
  return {
    phase: 'LOBBY',
    revision: 1,
    server_ts: WS_TS,
    join_url: 'http://192.168.1.10:8080',
    lobby: [{ nickname: 'Milo', connected: true }],
    leaderboard: [{ rank: 1, nickname: 'Milo', total_value: 0, budget: 1_000_000 }],
    item: null,
    recent_bids: [],
    sealed_progress: null,
  }
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
    // Queued, it is a no-op for the caller's deliberate teardown — `client.ts`
    // detaches onclose before it closes a socket it is replacing — but if a
    // future edit moves that detach, the spurious onclose becomes a visible
    // state change instead of a silent pass. For the STALE force-close the
    // event is the whole point: it must flow through the ordinary close path.
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
  function fire(scope: 'document' | 'window', type: string, event?: unknown): void {
    for (const listener of [...(listeners[scope].get(type) ?? [])]) {
      ;(listener as (event?: unknown) => void)(event)
    }
  }

  const fakeDocument = {
    visibilityState: 'visible',
    addEventListener: add('document'),
    removeEventListener: remove('document'),
  }
  // `pageshow` carries `persisted` (D-133): the bfcache restore is the ONLY
  // pageshow the socket acts on, and bfcache fires no visibilitychange.
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
    firePageshow(persisted = false): void {
      fire('window', 'pageshow', { persisted })
    },
  }
}

type SocketEnvironment = ReturnType<typeof installSocketEnvironment>

/** The socket the environment constructed last — the replacement under test. */
function lastSocket(env: SocketEnvironment): FakeSocket {
  const socket = env.sockets.at(-1)
  expect(socket).toBeDefined()
  if (socket === undefined) throw new Error('no socket was constructed')
  return socket
}

/** The player's stored token, as the server handed it over in a `hello`. */
function hello(seq: number, token: string): string {
  return envelope('hello', seq, { role: 'player', player: 'p1', nickname: 'Milo', token, rev: 0 })
}

describe('the per-connection stream account (D-21 — GAP 2)', () => {
  let connection: PlayerConnection | null = null

  beforeEach(() => {
    // The replacement is scheduled by D-130's policy now, so the retry is a
    // timer this suite must own rather than race.
    vi.useFakeTimers()
  })

  afterEach(() => {
    connection?.close()
    connection = null
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('returns a fresh initial gate state from the reset seam', () => {
    expect(resetStream()).toEqual({ lastSeq: 0, rev: 0, resyncPending: false })
    expect(resetStream()).toEqual(initialGateState())
  })

  it('lets the replacement socket of a reconnect start from its own seq 1', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    const errors: ErrorPayload[] = []
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: (error) => errors.push(error),
    })

    // ---- Connection one: the join form opens it, the player joins, and the
    // server's hello hands the identity token to storage (D-29).
    const first = lastSocket(env)
    expect(first.url).toBe('ws://localhost:5173/ws') // same-origin by construction
    expect(statuses.at(-1)).toBe('reconnecting') // no open socket, an attempt running
    first.accept()
    expect(statuses.at(-1)).toBe('live')
    expect(first.sent).toEqual([]) // no stored token yet — nothing to resume

    connection.join('Milo')
    expect(JSON.parse(first.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      nickname: 'Milo',
    })

    first.deliver(hello(1, 'first-token'))
    expect(readStoredToken()).toBe('first-token')

    // Frames that push this connection's stream position well past 1 — the
    // exact state the replacement socket must not inherit.
    first.deliver(envelope('phase.changed', 7, null, 3))

    // ---- The socket dies (a locked phone), and D-130's policy schedules the
    // replacement. The delay is the policy's own (attempt 1, sample 0 → 0 ms),
    // so one advance is enough — and it proves the close scheduled exactly one
    // attempt rather than replacing the socket in place.
    first.drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')
    expect(env.sockets).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(env.sockets).toHaveLength(2)

    const second = lastSocket(env)
    second.accept()
    expect(statuses.at(-1)).toBe('live')
    expect(JSON.parse(second.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      token: 'first-token',
    })

    // ---- The replacement server stream starts over at seq 1 (D-21).
    second.deliver(hello(1, 'second-token'))
    // With the dead connection's lastSeq still in place this hello classified
    // as `duplicate` and the replacement token never reached storage.
    expect(readStoredToken()).toBe('second-token')

    // A following NON-snapshot frame reaches its callback: the reset is a
    // stream reset, not snapshot-only recovery.
    second.deliver(envelope('error', 2, { code: 'not_joined', message: 'join first' }))
    expect(errors).toEqual([{ code: 'not_joined', message: 'join first' }])

    // ---- A visible tab that was never hidden and a non-bfcache pageshow are
    // not verdicts about a healthy socket: no second replacement is built.
    env.fireVisibility('visible')
    env.firePageshow(false)
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
    socket.deliver(hello(1, 'tok'))

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

describe('the deliberate session end (D-78/D-80 close code 4002)', () => {
  let connection: PlayerConnection | null = null

  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    connection?.close()
    connection = null
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('drops the stale identity and signals the route on 4002, not on an ordinary drop', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    let ended = 0
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
      onSessionEnded: () => {
        ended += 1
      },
    })

    const socket = lastSocket(env)
    socket.accept()
    connection.join('Milo')
    socket.deliver(hello(1, 'tok'))
    expect(readStoredToken()).toBe('tok')

    // A network drop is NOT a session end: the identity is kept so the next
    // socket can restore it, and the route is left where it is.
    socket.drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')
    expect(ended).toBe(0)
    expect(readStoredToken()).toBe('tok')

    // The stored token is what the replacement rejoins with…
    await vi.advanceTimersByTimeAsync(1)
    const second = lastSocket(env)
    second.accept()
    expect(JSON.parse(second.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      token: 'tok',
    })

    // …and the presenter then clears the room / releases this row: the server
    // ends the session with its dedicated code. The device must not stay on a
    // lobby whose identity is gone, so the token is dropped and the route is
    // told to return to the join form.
    second.drop(4002)
    expect(ended).toBe(1)
    expect(readStoredToken()).toBe('')
    expect(statuses.at(-1)).toBe('reconnecting')

    // The 4002 socket is an ordinary close for the SCHEDULER (only 4001 is
    // terminal): the replacement carries no identity, so the join form the
    // route just returned to has a live socket to submit through rather than
    // silently doing nothing.
    await vi.advanceTimersByTimeAsync(1)
    const third = lastSocket(env)
    expect(env.sockets).toHaveLength(3)
    third.accept()
    expect(third.sent).toEqual([]) // nothing left to resume with
  })

  it('keeps the identity on 4001, which is a supersede and not a session end', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    let ended = 0
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
      onSessionEnded: () => {
        ended += 1
      },
    })

    const socket = lastSocket(env)
    socket.accept()
    connection.join('Milo')
    socket.deliver(hello(1, 'tok'))

    socket.drop(4001)
    expect(statuses.at(-1)).toBe('superseded')
    expect(ended).toBe(0)
    expect(readStoredToken()).toBe('tok')
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
    expect(statuses.at(-1)).toBe('reconnecting')

    socket.accept()
    expect(statuses.at(-1)).toBe('live')
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
    // identity, so a replacement socket sends no identity frame.
    expect(env.storage.size).toBe(0)
    socket.drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')

    // The presenter's one recovery control (D-130) is part of the handle.
    expect(typeof admin.retry).toBe('function')
  })
})

describe('the screen connection (D-87)', () => {
  let screen: ScreenConnection | null = null

  afterEach(() => {
    screen?.close()
    screen = null
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('joins as screen, reduces its public snapshot and exposes no send', () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    const views: ViewState[] = []

    screen = openScreenSocket({
      onStatus: (status) => statuses.push(status),
      onView: (view) => views.push(view),
    })

    const socket = env.sockets[0]
    expect(socket).toBeDefined()
    if (socket === undefined) return
    expect(socket.url).toBe('ws://localhost:5173/ws')
    expect(statuses.at(-1)).toBe('reconnecting')

    socket.accept()
    expect(statuses.at(-1)).toBe('live')
    expect(JSON.parse(socket.sent.at(-1) ?? 'null')).toEqual({ type: 'join', role: 'screen' })
    // Read-only BY TYPE: `close` is the whole handle, so no screen control can
    // ever reach for a `send` or a `retry` that exists (D-87).
    expect(Object.keys(screen)).toEqual(['close'])

    socket.deliver(envelope('hello', 1, { role: 'screen', rev: 0 }))
    const payload = screenSnapshotData()
    socket.deliver(envelope('state.snapshot', 2, payload, 1))
    const snapshot = readScreenSnapshot(payload)
    expect(snapshot).not.toBeNull()
    if (snapshot === null) return
    expect(views.at(-1)).toEqual(
      reduceOrThrow(initialViewState(), { kind: 'screen-snapshot', snapshot }),
    )
    expect(views.at(-1)?.joinUrl).toBe('http://192.168.1.10:8080')

    // No identity, so nothing is persisted; the projector's own recovery is the
    // automatic scheduler, not a control (G-3).
    expect(env.storage.size).toBe(0)
    socket.drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')
  })

  it('applies the reserved finale frame to the same reduced view', () => {
    const env = installSocketEnvironment()
    const views: ViewState[] = []

    screen = openScreenSocket({ onStatus: () => {}, onView: (view) => views.push(view) })

    const socket = env.sockets[0]
    expect(socket).toBeDefined()
    if (socket === undefined) return
    socket.accept()

    const standings = [
      { rank: 1, nickname: 'Bình', total_value: 200_000, budget: 900_000 },
      { rank: 2, nickname: 'An', total_value: 0, budget: 1_000_000 },
    ]
    socket.deliver(envelope('finale', 1, { podium: standings, standings }, 12))
    const payload = readFinale({ podium: standings, standings })
    expect(payload).not.toBeNull()
    if (payload === null) return
    expect(views.at(-1)).toEqual(
      reduceOrThrow(initialViewState(), { kind: 'finale', payload, ts: WS_TS }),
    )
    expect(views.at(-1)?.leaderboard).toEqual([
      { rank: 1, nickname: 'Bình', totalValue: 200_000, budget: 900_000 },
      { rank: 2, nickname: 'An', totalValue: 0, budget: 1_000_000 },
    ])
  })
})

describe('the reconnect policy in the lifecycle (D-130…D-134)', () => {
  let connection: PlayerConnection | null = null

  beforeEach(() => {
    vi.useFakeTimers()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    connection?.close()
    connection = null
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('schedules ONE attempt per close, after the policy delay (D-130)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0.5)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
    })

    const first = lastSocket(env)
    first.accept()
    first.deliver(hello(1, 'tok'))
    first.drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')

    // One failed attempt, sample 0.5: the policy's ceiling is 500·2^1 = 1000 ms,
    // so the replacement exists at 500 ms and not one tick earlier.
    expect(env.sockets).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(499)
    expect(env.sockets).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(env.sockets).toHaveLength(2)
    lastSocket(env).accept()
    expect(statuses.at(-1)).toBe('live')
  })

  it('keeps attempting after the offline threshold instead of stopping (G-2)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
    })

    // Six consecutive failures on a page that never opened: the state turns
    // `offline` (the presentation threshold) and the seventh attempt still
    // exists — auto-retry never stops.
    for (let attempt = 0; attempt < 6; attempt += 1) {
      expect(statuses.at(-1)).toBe(attempt >= 6 ? 'offline' : 'reconnecting')
      lastSocket(env).drop(1006)
      await vi.advanceTimersByTimeAsync(1)
    }
    expect(statuses.at(-1)).toBe('offline')
    expect(env.sockets).toHaveLength(7)
  })

  it('connects immediately on the manual retry and resets the backoff (D-130)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
    })

    for (let attempt = 0; attempt < 6; attempt += 1) {
      lastSocket(env).drop(1006)
      await vi.advanceTimersByTimeAsync(1)
    }
    expect(statuses.at(-1)).toBe('offline')

    // The retry cancels the scheduled wait and connects now: no timer advance.
    const before = env.sockets.length
    connection.retry()
    expect(env.sockets).toHaveLength(before + 1)
    expect(statuses.at(-1)).toBe('reconnecting')

    // …and the counter really went back to the base: one more failure is not
    // the threshold again, so the affordance cannot get stuck in `offline`.
    lastSocket(env).drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')
  })

  it('treats 4001 as terminal, with one deliberate rejoin using the kept token (D-131)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
    })

    const first = lastSocket(env)
    first.accept()
    first.deliver(hello(1, 'tok'))
    first.drop(4001)
    expect(statuses.at(-1)).toBe('superseded')

    // No auto-reconnect ever, on this page instance — the supersede loop is the
    // failure this rule exists to stop (Pitfall 12).
    await vi.advanceTimersByTimeAsync(120_000)
    expect(env.sockets).toHaveLength(1)

    // The two lifecycle paths that connect DIRECTLY (D-133's long hide, the
    // bfcache restore) are bound by the same terminal rule. A backgrounded
    // phone that resumes while the notice is showing must not clear it and
    // re-claim the identity: that rejoin is silent (the status turns `live`, so
    // the notice vanishes) and it is the supersede loop reached through a path
    // the scheduler never touches.
    env.fireVisibility('hidden')
    await vi.advanceTimersByTimeAsync(6_000)
    env.fireVisibility('visible')
    expect(env.sockets).toHaveLength(1)
    env.firePageshow(true)
    expect(env.sockets).toHaveLength(1)
    expect(statuses.at(-1)).toBe('superseded')

    // The token is KEPT: the notice's action is one deliberate rejoin that
    // supersedes the other device in turn.
    expect(readStoredToken()).toBe('tok')
    connection.retry()
    expect(env.sockets).toHaveLength(2)
    const second = lastSocket(env)
    second.accept()
    expect(JSON.parse(second.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      token: 'tok',
    })
    expect(statuses.at(-1)).toBe('live')
  })

  it('force-closes a stale socket after 60 s of visible silence, refreshed by heartbeats (D-132)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
    })

    const socket = lastSocket(env)
    socket.accept()

    // Ten seconds of silence at a time, each broken by the server's beat: the
    // quiet phases (LOBBY/INTRO/SETTLED) carry nothing else.
    for (let beat = 0; beat < 5; beat += 1) {
      await vi.advanceTimersByTimeAsync(10_000)
      expect(socket.readyState).toBe(1)
      socket.deliver(envelope('heartbeat', beat + 2, { ts: WS_TS + (beat + 1) * 10_000 }))
    }

    // Exactly the budget of silence is not yet stale — the force-close is a
    // strict "> 60 s", so a beat that lands on the boundary still counts.
    await vi.advanceTimersByTimeAsync(60_000)
    expect(socket.readyState).toBe(1)

    await vi.advanceTimersByTimeAsync(5_000)
    expect(socket.readyState).toBe(3)
    // The force-close is an ORDINARY close for the policy — the same path a
    // network drop takes — so the next scheduled attempt builds a replacement.
    await vi.advanceTimersByTimeAsync(1)
    expect(env.sockets).toHaveLength(2)
    // The state it produces is the machine's own: the same silence also aged the
    // page past its 30 s offline threshold, so it reads `offline`. Invisible
    // except through that state, exactly as D-132 requires.
    expect(statuses.at(-1)).toBe('offline')
  })

  it('never tears a socket down from a hidden tab, and replaces it on a long return (D-133)', async () => {
    const env = installSocketEnvironment()
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: () => {},
      onSnapshot: () => {},
      onError: () => {},
    })

    const first = lastSocket(env)
    first.accept()
    first.deliver(hello(1, 'tok'))

    env.fireVisibility('hidden')
    // A hidden tab's timers are throttled by iOS, so they are not a liveness
    // input: 70 s of silence changes nothing while the tab is away.
    await vi.advanceTimersByTimeAsync(70_000)
    expect(first.readyState).toBe(1)

    env.fireVisibility('visible')
    // Back after more than five seconds: a NEW socket, never the old one, with
    // the carried detach-then-replace order.
    expect(first.readyState).toBe(3)
    expect(env.sockets).toHaveLength(2)
    const second = lastSocket(env)
    second.accept()
    expect(JSON.parse(second.sent.at(-1) ?? 'null')).toEqual({
      type: 'join',
      role: 'player',
      token: 'tok',
    })
  })

  it('replaces the socket only for a bfcache pageshow, and only after five hidden seconds', async () => {
    const env = installSocketEnvironment()
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: () => {},
      onSnapshot: () => {},
      onError: () => {},
    })

    const first = lastSocket(env)
    first.accept()

    // Exactly five seconds hidden is NOT past the threshold: a quick app switch
    // keeps its socket.
    env.fireVisibility('hidden')
    await vi.advanceTimersByTimeAsync(5_000)
    env.fireVisibility('visible')
    expect(env.sockets).toHaveLength(1)

    // A normal pageshow (a fresh load) is not a bfcache restore.
    env.firePageshow(false)
    expect(env.sockets).toHaveLength(1)

    // `event.persisted === true` is the explicit bfcache signal, and bfcache
    // fires no visibilitychange.
    env.firePageshow(true)
    expect(env.sockets).toHaveLength(2)
  })

  it('ignores a close event from a socket it has already replaced (one scheduler)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0)

    connection = openPlayerSocket({
      onStatus: (status) => statuses.push(status),
      onSnapshot: () => {},
      onError: () => {},
    })

    const first = lastSocket(env)
    first.accept()
    const staleClose = first.onclose
    connection.retry()
    expect(env.sockets).toHaveLength(2)
    lastSocket(env).accept()
    expect(statuses.at(-1)).toBe('live')

    // The dead socket's handler is detached, but a late event that still got
    // through must not stop or duplicate the replacement's own loop — a 4001
    // from the old connection is exactly the case that would strand the page.
    staleClose?.({ code: 4001 })
    expect(statuses.at(-1)).toBe('live')
    await vi.advanceTimersByTimeAsync(20_000)
    expect(env.sockets).toHaveLength(2)
  })

  it('self-heals the projector while its handle stays close-only (D-130/G-3)', async () => {
    const env = installSocketEnvironment()
    const statuses: string[] = []
    vi.spyOn(Math, 'random').mockReturnValue(0)

    const projector = openScreenSocket({
      onStatus: (status) => statuses.push(status),
      onView: () => {},
    })

    const first = lastSocket(env)
    first.accept()
    first.drop(1006)
    expect(statuses.at(-1)).toBe('reconnecting')
    await vi.advanceTimersByTimeAsync(1)
    expect(env.sockets).toHaveLength(2)
    lastSocket(env).accept()
    expect(statuses.at(-1)).toBe('live')

    // No control was added for it: the handle is still `close` alone.
    expect(Object.keys(projector)).toEqual(['close'])
    projector.close()
  })

  it('logs an informational code without painting a route error (D-138/T-07-11)', async () => {
    const env = installSocketEnvironment()
    const errors: ErrorPayload[] = []
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    connection = openPlayerSocket({
      onStatus: () => {},
      onSnapshot: () => {},
      onError: (error) => errors.push(error),
    })

    const socket = lastSocket(env)
    socket.accept()
    socket.deliver(hello(1, 'tok'))

    // The command bucket's answer: the socket stays open, nothing moved, and
    // the frame reaches the log — but no route may turn it into an error row.
    socket.deliver(envelope('error', 2, { code: 'rate_limited', message: 'slow down' }, 4))
    expect(errors).toEqual([])
    expect(warn).toHaveBeenCalledWith('[ws] server error', 'rate_limited', 'slow down')

    // Every other refusal still reaches the route, so the filter is one code
    // wide rather than a mute button.
    socket.deliver(envelope('error', 3, { code: 'bad_message', message: 'no such frame' }, 4))
    expect(errors).toEqual([{ code: 'bad_message', message: 'no such frame' }])
  })
})
