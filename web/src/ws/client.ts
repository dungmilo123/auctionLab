/**
 * The player's socket lifecycle (D-44/D-45).
 *
 * This module owns everything the pure gate cannot: the same-origin URL, the
 * open/close state, the reconciliation state per connection, the token in
 * storage, and D-45's one-shot resume. The rules themselves live in `gate.ts`
 * and the frame shapes in `protocol.ts`, so both stay testable in the node
 * environment.
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
  CODE_TOKEN_INVALID,
  ERROR,
  HELLO,
  parseEnvelope,
  readError,
  readHello,
  readPlayerSnapshot,
  type ErrorPayload,
  type PlayerSnapshot,
} from './protocol'

/** D-29's storage key (UI-SPEC §Resume, supersede and storage). */
export const TOKEN_KEY = 'auctionlab.player_token'

/** Close code 4001: another connection took this identity (D-31). */
export const SUPERSEDED_CLOSE_CODE = 4001

/** The four connection states UI-SPEC allows — exactly these, no more. */
export type ConnectionStatus = 'connecting' | 'open' | 'lost' | 'superseded'

export type PlayerSocketHandlers = {
  onStatus(status: ConnectionStatus): void
  onSnapshot(snapshot: PlayerSnapshot): void
  /** The rejection the player must act on. `code` selects the copy; the /
   * server's `message` is never rendered (UI-SPEC §Join/mutation states). */
  onError(error: ErrorPayload): void
}

export type PlayerConnection = {
  /** Send the nickname join (D-23). Valid after `open`. */
  join(nickname: string): void
  /** Ask the server for a fresh self-sufficient snapshot (D-42). */
  resync(): void
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
 * Open the player connection. Returns the handle the effect keeps for the
 * lifetime of the screen; `close()` detaches every listener it installed.
 */
export function openPlayerSocket(handlers: PlayerSocketHandlers): PlayerConnection {
  let gate: GateState = initialGateState()
  let current: WebSocket | null = null
  let closed = false

  function send(frame: unknown): void {
    const socket = current
    if (socket === null || socket.readyState !== SOCKET_OPEN) return
    socket.send(JSON.stringify(frame))
  }

  function applyHello(data: unknown): void {
    const hello = readHello(data)
    if (hello === null || hello.role !== 'player') return
    // D-29: the token is persisted ONLY from a player hello, and only the token
    // this connection was addressed with. It is never displayed and never
    // broadcast — storage is the whole of its lifetime (T-02-07).
    if (hello.token !== '') storeToken(hello.token)
  }

  function applyError(data: unknown): void {
    const error = readError(data)
    if (error === null) return
    if (error.code === CODE_TOKEN_INVALID) {
      // D-30 (3): the stored token is dead, so it must not be offered again —
      // otherwise every reload repeats the same rejected resume.
      clearStoredToken()
    }
    // The server's message is log-only (UI-SPEC §Join/mutation states, T-02-01):
    // the UI selects its copy from the dictionary by `error.code`.
    console.warn('[ws] server error', error.code, error.message)
    handlers.onError(error)
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
        const snapshot = readPlayerSnapshot(envelope.data)
        if (snapshot !== null) handlers.onSnapshot(snapshot)
        return
      }
      case 'apply':
        if (envelope.type === HELLO) applyHello(envelope.data)
        if (envelope.type === ERROR) applyError(envelope.data)
        if (step.outcome.requestResync) send({ type: 'resync' })
        return
    }
  }

  function connect(): void {
    if (closed) return

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

    handlers.onStatus('connecting')

    let socket: WebSocket
    try {
      socket = new WebSocket(socketURL())
    } catch {
      // The constructor throws on an unusable URL (e.g. a built index.html
      // opened from file://). Without this the status line would stay
      // "connecting" forever, which mid-demo reads as a hung server.
      handlers.onStatus('lost')
      return
    }
    current = socket

    socket.onopen = () => {
      if (current !== socket) return
      handlers.onStatus('open')
      // A stored token resumes the identity immediately (D-30 (1)): the player
      // never sees the form again after a reload or a phone unlock.
      const token = readStoredToken()
      if (token !== '') send({ type: 'join', role: 'player', token })
    }

    socket.onmessage = (event: MessageEvent) => onFrame(socket, event.data)

    socket.onclose = (event: CloseEvent) => {
      if (current !== socket) return // a deliberate close already ran
      current = null
      handlers.onStatus(event.code === SUPERSEDED_CLOSE_CODE ? 'superseded' : 'lost')
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
    // RECN-01/02 (Phase 7) and are deliberately absent.
    if (resumeOutcome(readyState, readStoredToken() !== '') !== 'reconnect') return
    connect()
  }

  function onVisibilityChange(): void {
    if (document.visibilityState === 'visible') resume()
  }

  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('pageshow', resume)
  connect()

  return {
    join(nickname: string): void {
      send({ type: 'join', role: 'player', nickname })
    },
    resync(): void {
      send({ type: 'resync' })
    },
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
