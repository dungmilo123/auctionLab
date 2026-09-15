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
import type { PlayerConnection } from './client'
import type { ErrorPayload } from './protocol'

/** One server frame, exactly as the wire carries it (D-21). */
function envelope(type: string, seq: number, data: unknown, rev = 0): string {
  return JSON.stringify({ type, seq, rev, ts: 1_757_934_000_000, data })
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
    this.sent.push(data)
  }

  close(): void {
    this.readyState = 3
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
