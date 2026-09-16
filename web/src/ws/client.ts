/**
 * The socket lifecycle (D-44/D-45) — one role-neutral core plus the player
 * wrapper Phase 2 shipped.
 *
 * This module owns everything the pure gate cannot: the same-origin URL, the
 * open/close state, the reconciliation state per connection, the token in
 * storage, and D-45's one-shot resume. The rules themselves live in `gate.ts`,
 * the frame shapes in `protocol.ts` and the reduction in `view.ts`, so all
 * three stay testable in the node environment.
 *
 * `openSocket` is the extracted lifecycle (H17); `openPlayerSocket` is the
 * player's thin wrapper over it, with its exported signature, callbacks and
 * token behaviour unchanged. `admin.ts` builds the presenter's connection on
 * the same core.
 *
 * It is deliberately NOT a React hook: the React effect that calls
 * `openPlayerSocket` owns the lifetime, and cleanup closes it. A module-scope
 * socket would open twice under `<StrictMode>` and supersede itself, which is
 * the failure the UI-SPEC calls out.
 */

import {
  SOCKET_CLOSED,
  SOCKET_OPEN,
  initialGateState,
  nextOutcome,
  resumeOutcome,
  type GateState,
} from './gate'
import {
  BID_ACCEPTED,
  BID_REJECTED,
  CODE_TOKEN_INVALID,
  ERROR,
  FINALE,
  HELLO,
  ITEM_INTRO,
  ITEM_LIVE,
  ITEM_SETTLED,
  LEADERBOARD,
  PHASE_CHANGED,
  PRICE_TICK,
  STATE_SNAPSHOT,
  parseEnvelope,
  readAdminSnapshot,
  readBidAccepted,
  readBidRejected,
  readError,
  readFinale,
  readHello,
  readLeaderboard,
  readPhaseChanged,
  readPlayerSnapshot,
  readPriceTick,
  readPublicItem,
  readScreenSnapshot,
  type BidRejected,
  type Envelope,
  type ErrorPayload,
  type Hello,
  type PlayerSnapshot,
} from './protocol'
import { initialViewState, reduceView, type ViewFrame, type ViewState } from './view'

/** D-29's storage key (UI-SPEC §Resume, supersede and storage). */
export const TOKEN_KEY = 'auctionlab.player_token'

/** Close code 4001: another connection took this identity (D-31). */
export const SUPERSEDED_CLOSE_CODE = 4001

/** The four connection states UI-SPEC allows — exactly these, no more. */
export type ConnectionStatus = 'connecting' | 'open' | 'lost' | 'superseded'

/** The role a socket joins as (D-23). It selects the snapshot decoder. */
export type SocketRole = 'player' | 'admin' | 'screen'

/**
 * The role-neutral socket contract (H17). The lifecycle — same-origin URL,
 * detach-then-replace, reset-per-connection gate, ONE resume attempt per
 * visibility/pageshow event — lives here once; the role owns its join frame,
 * its identity and its callbacks.
 */
export type SocketOptions = {
  role: SocketRole
  /** the frame to send once the upgrade completes; null = this role joins later */
  joinFrame: (token: string) => unknown | null
  /** the stored identity to resume with — '' means there is nothing to restore */
  readToken: () => string
  /** called with a `hello` addressed to this socket (the player stores its token) */
  onHello?: (hello: Hello) => void
  /** called when the server refuses the stored identity (D-30 (3)) */
  onTokenInvalid?: () => void
  onStatus: (status: ConnectionStatus) => void
  /** one reduced view per admitted frame (D-57/D-70); invalid frames never arrive */
  onView?: (view: ViewState) => void
  /** the player route's Phase-2 snapshot callback, kept for the lobby screen */
  onSnapshot?: (snapshot: PlayerSnapshot) => void
  /** D-53's private refusal, for the submitter's feedback row */
  onBidRejected?: (rejection: BidRejected) => void
  onError?: (error: ErrorPayload) => void
}

/** What every role's socket handle can do. */
export type Socket = {
  /** send one already-built frame (a role command); a no-op while not open */
  send(frame: unknown): void
  /** detach and close. Safe to call twice. */
  close(): void
}

export type PlayerSocketHandlers = {
  onStatus(status: ConnectionStatus): void
  onSnapshot(snapshot: PlayerSnapshot): void
  /** The rejection the player must act on. `code` selects the copy; the /
   * server's `message` is never rendered (UI-SPEC §Join/mutation states). */
  onError(error: ErrorPayload): void
  /** The Phase-3 reduced view (D-70). Optional so Phase 2's call site is unchanged. */
  onView?(view: ViewState): void
  /** D-53's private bid refusal, rendered as the feedback row. */
  onBidRejected?(rejection: BidRejected): void
}

export type PlayerConnection = {
  /** Send the nickname join (D-23). Valid after `open`. */
  join(nickname: string): void
  /** Ask the server for a fresh self-sufficient snapshot (D-42). */
  resync(): void
  /**
   * Send one player command built by the caller (`bid.place` is the Phase-3
   * one, D-52/D-71); a no-op while the socket is not open. It is the player's
   * counterpart to the admin connection's `send`, so both roles drive their
   * commands through the same shared core.
   */
  send(frame: unknown): void
  /** Detach and close. Safe to call twice. */
  close(): void
}

/**
 * Same-origin by construction: scheme and host come from the page that loaded
 * the app, so dev goes through the Vite proxy and the built binary answers
 * directly. Never an absolute or foreign URL (T-05-01).
 */
function socketURL(): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${window.location.host}/ws`
}

/**
 * Read the stored token. A throwing or unavailable storage degrades to "no
 * token" with no error copy shown: the player joins as a new identity instead
 * of being blocked by a browser setting (UI-SPEC §Resume, supersede and
 * storage).
 */
export function readStoredToken(): string {
  try {
    return window.localStorage.getItem(TOKEN_KEY) ?? ''
  } catch {
    return ''
  }
}

function storeToken(token: string): void {
  try {
    window.localStorage.setItem(TOKEN_KEY, token)
  } catch {
    // Degrade to a session that cannot resume, never to a broken screen.
  }
}

function clearStoredToken(): void {
  try {
    window.localStorage.removeItem(TOKEN_KEY)
  } catch {
    // Nothing to clear.
  }
}

/**
 * The stream account a socket starts with. D-21 says `seq` is THIS
 * connection's position and the server restarts it at 1 on every accepted
 * upgrade, so the gate is one per connection — a replacement socket must never
 * inherit the dead connection's high-water mark.
 */
export function resetStream(): GateState {
  return initialGateState()
}

/**
 * The extracted role-neutral lifecycle (H17): same-origin URL, the gate reset
 * per connection, the detach-then-replace order, the D-45 resume, the status
 * callback and the reduced-view callback. The role owns its join frame, its
 * identity storage and its callbacks; nothing role-specific lives here.
 *
 * Everything the pure gate cannot own is here: one `GateState` per connection,
 * the open/close state, the stored-identity read, and D-45's one-shot resume.
 * The rules themselves live in `gate.ts`, the frame shapes in `protocol.ts` and
 * the reduction in `view.ts`, so all three stay testable in the node
 * environment with this module's four stubbed globals.
 */
export function openSocket(opts: SocketOptions): Socket {
  // The gate is per CONNECTION (D-21), but the view is the app's state: it
  // survives a resume, and the replacement connection's snapshot replaces it.
  let gate: GateState = initialGateState()
  let view: ViewState = initialViewState()
  let current: WebSocket | null = null
  let closed = false

  function send(frame: unknown): void {
    const socket = current
    if (socket === null || socket.readyState !== SOCKET_OPEN) return
    socket.send(JSON.stringify(frame))
  }

  function applyHello(data: unknown): void {
    const hello = readHello(data)
    if (hello === null || hello.role !== opts.role) return
    opts.onHello?.(hello)
  }

  function applyError(data: unknown): void {
    const error = readError(data)
    if (error === null) return
    if (error.code === CODE_TOKEN_INVALID) {
      // D-30 (3): the stored token is dead, so it must not be offered again —
      // otherwise every reload repeats the same rejected resume.
      opts.onTokenInvalid?.()
    }
    // The server's message is log-only (UI-SPEC §Join/mutation states, T-02-01):
    // the UI selects its copy from the dictionary by `error.code`.
    console.warn('[ws] server error', error.code, error.message)
    opts.onError?.(error)
  }

  function pushView(frame: ViewFrame): void {
    const next = reduceView(view, frame)
    if (next === null) return
    view = next
    opts.onView?.(view)
  }

  function onFrame(socket: WebSocket, raw: unknown): void {
    if (current !== socket) return
    const envelope = parseEnvelope(raw)
    if (envelope === null) return

    const step = nextOutcome(gate, envelope)
    gate = step.state

    switch (step.outcome.kind) {
      case 'duplicate':
        return
      case 'control':
        // Phase 7 owns tunnel.status behaviour. The gate's exception is the
        // only thing Phase 2 needs: never mistaken for a game event.
        return
      case 'snapshot': {
        if (!step.outcome.apply) return
        const frame = decodeFrame(opts.role, envelope)
        if (frame === null) return
        // The Phase-2 lobby callback and the reduced view are the same frame:
        // one decode, two consumers.
        if (frame.kind === 'player-snapshot') opts.onSnapshot?.(frame.snapshot)
        pushView(frame)
        return
      }
      case 'apply': {
        if (envelope.type === HELLO) applyHello(envelope.data)
        if (envelope.type === ERROR) applyError(envelope.data)
        if (envelope.type === BID_REJECTED) {
          const rejection = readBidRejected(envelope.data)
          if (rejection !== null) opts.onBidRejected?.(rejection)
        }
        const frame = decodeFrame(opts.role, envelope)
        if (frame !== null) pushView(frame)
        if (step.outcome.requestResync) send({ type: 'resync' })
        return
      }
    }
  }

  function connect(): void {
    if (closed) return

    // D-21/GAP 2: `seq` is THIS connection's stream position and the server
    // restarts it at 1 on every accepted upgrade, so the gate is one per
    // connection. Reusing the dead socket's high-water mark would classify the
    // replacement's hello — and every later sibling at or below that mark — as
    // a duplicate and silently drop it: the resume would look alive while its
    // stream stayed frozen.
    gate = resetStream()

    // Detach and close the connection being replaced BEFORE opening its
    // successor, so a late event from the old socket can never be mistaken for
    // the new one's (the StrictMode double-invoke and the 4001 case).
    const previous = current
    current = null
    if (previous !== null) {
      previous.onopen = null
      previous.onmessage = null
      previous.onclose = null
      previous.onerror = null
      previous.close()
    }

    opts.onStatus('connecting')

    let socket: WebSocket
    try {
      socket = new WebSocket(socketURL())
    } catch {
      // The constructor throws on an unusable URL (e.g. a built index.html
      // opened from file://). Without this the status line would stay
      // "connecting" forever, which mid-demo reads as a hung server.
      opts.onStatus('lost')
      return
    }
    current = socket

    socket.onopen = () => {
      if (current !== socket) return
      opts.onStatus('open')
      // The role's join. The player sends the stored token when there is one
      // (D-30 (1)) and otherwise waits for the form; the admin always joins as
      // admin — there is no identity to restore.
      const frame = opts.joinFrame(opts.readToken())
      if (frame !== null) send(frame)
    }

    socket.onmessage = (event: MessageEvent) => onFrame(socket, event.data)

    socket.onclose = (event: CloseEvent) => {
      if (current !== socket) return // a deliberate close already ran
      current = null
      // `superseded` is a status, not a terminal state: the token is kept, so
      // the next visibility/pageshow runs D-45's resume and reopens with it —
      // which means two tabs of one identity take turns superseding each other,
      // each time a backgrounded one comes forward. No token leaks and no data
      // is lost; making the superseded state terminal is Phase 7's reconnect
      // policy (RECN-01/02).
      opts.onStatus(event.code === SUPERSEDED_CLOSE_CODE ? 'superseded' : 'lost')
    }

    socket.onerror = () => {
      // The close event carries the state; nothing to add here.
    }
  }

  function resume(): void {
    if (closed) return
    const readyState = current === null ? SOCKET_CLOSED : current.readyState
    // D-45: ONE attempt per visibility/pageshow event, and only when there is
    // an identity to restore. Backoff, jitter and stale-socket detection are
    // RECN-01/02 (Phase 7) and are deliberately absent — and a role with no
    // stored identity (the admin) simply never resumes.
    if (resumeOutcome(readyState, opts.readToken() !== '') !== 'reconnect') return
    connect()
  }

  function onVisibilityChange(): void {
    if (document.visibilityState === 'visible') resume()
  }

  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('pageshow', resume)
  connect()

  return {
    send,
    close(): void {
      closed = true
      document.removeEventListener('visibilitychange', onVisibilityChange)
      window.removeEventListener('pageshow', resume)
      const socket = current
      current = null
      if (socket !== null) {
        socket.onopen = null
        socket.onmessage = null
        socket.onclose = null
        socket.onerror = null
        socket.close()
      }
    },
  }
}

/**
 * Decode one admitted envelope into the frame the reducer understands. `null`
 * covers everything this socket does not render — `hello`, `error`,
 * `tunnel.status`, later phases' frames — AND every decode failure, so an
 * invalid or phase-inconsistent payload can never reach `onView` (T-03-09).
 */
function decodeFrame(role: SocketRole, envelope: Envelope): ViewFrame | null {
  const { type, data, ts } = envelope
  switch (type) {
    case STATE_SNAPSHOT: {
      if (role === 'admin') {
        const snapshot = readAdminSnapshot(data)
        return snapshot === null ? null : { kind: 'admin-snapshot', snapshot }
      }
      if (role === 'screen') {
        const snapshot = readScreenSnapshot(data)
        return snapshot === null ? null : { kind: 'screen-snapshot', snapshot }
      }
      const snapshot = readPlayerSnapshot(data)
      return snapshot === null ? null : { kind: 'player-snapshot', snapshot }
    }
    case FINALE: {
      const payload = readFinale(data)
      return payload === null ? null : { kind: 'finale', payload, ts }
    }
    case PHASE_CHANGED: {
      const payload = readPhaseChanged(data)
      return payload === null ? null : { kind: 'phase.changed', payload, ts }
    }
    case ITEM_INTRO:
    case ITEM_LIVE:
    case ITEM_SETTLED: {
      const item = readPublicItem(data)
      return item === null ? null : { kind: 'item', type, item, ts }
    }
    case PRICE_TICK: {
      const tick = readPriceTick(data)
      return tick === null ? null : { kind: 'price.tick', tick, ts }
    }
    case BID_ACCEPTED: {
      const bid = readBidAccepted(data)
      return bid === null ? null : { kind: 'bid.accepted', bid, ts }
    }
    case LEADERBOARD:
      return { kind: 'leaderboard', entries: readLeaderboard(data), ts }
    default:
      return null
  }
}

/**
 * Open the player connection (D-44/D-45) — the exported API Phase 2 shipped,
 * now a thin role wrapper over `openSocket`. The signature, the callbacks, the
 * token behaviour and the resume rule are unchanged: the player joins by name
 * from the form, or resumes with the stored token the moment the socket opens.
 */
export function openPlayerSocket(handlers: PlayerSocketHandlers): PlayerConnection {
  const socket = openSocket({
    role: 'player',
    readToken: readStoredToken,
    // A stored token resumes the identity immediately (D-30 (1)): the player
    // never sees the form again after a reload or a phone unlock. Without one,
    // nothing is sent and the form's join is the path.
    joinFrame: (token) => (token !== '' ? { type: 'join', role: 'player', token } : null),
    onHello: (hello) => {
      // D-29: the token is persisted ONLY from a player hello, and only the
      // token this connection was addressed with. It is never displayed and
      // never broadcast — storage is the whole of its lifetime (T-02-07).
      if (hello.token !== '') storeToken(hello.token)
    },
    onTokenInvalid: clearStoredToken,
    onStatus: handlers.onStatus,
    onView: handlers.onView,
    onSnapshot: handlers.onSnapshot,
    onBidRejected: handlers.onBidRejected,
    onError: handlers.onError,
  })

  return {
    join(nickname: string): void {
      socket.send({ type: 'join', role: 'player', nickname })
    },
    resync(): void {
      socket.send({ type: 'resync' })
    },
    send(frame: unknown): void {
      socket.send(frame)
    },
    close(): void {
      socket.close()
    },
  }
}
