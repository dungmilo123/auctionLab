/**
 * The socket lifecycle (D-44/D-45 → D-130…D-134) — one role-neutral core plus
 * the player wrapper Phase 2 shipped.
 *
 * This module owns everything the pure modules cannot: the same-origin URL, the
 * open/close state, the reconciliation state per connection, the token in
 * storage, and every global the policy is applied through (`window`,
 * `document`, `WebSocket`, `localStorage`, `setTimeout`, `Math.random`). The
 * rules themselves live in `reconnect.ts` (the policy), `gate.ts` (the stream
 * account), `protocol.ts` (the frame shapes) and `view.ts` (the reduction), so
 * all four stay testable in the node environment.
 *
 * `openSocket` is the extracted lifecycle (H17); `openPlayerSocket` is the
 * player's thin wrapper over it, with its exported signature and token
 * behaviour unchanged. `admin.ts` and `screen.ts` build their connections on the
 * same core, which is what gives all three roles the same recovery (D-130).
 *
 * It is deliberately NOT a React hook: the React effect that calls
 * `openPlayerSocket` owns the lifetime, and cleanup closes it. A module-scope
 * socket would open twice under `<StrictMode>` and supersede itself, which is
 * the failure the UI-SPEC calls out.
 */

import {
  SOCKET_OPEN,
  initialGateState,
  nextOutcome,
  type GateState,
} from './gate'
import {
  HIDDEN_TEARDOWN_MS,
  STALE_CHECK_PERIOD_MS,
  STALE_SILENCE_MS,
  SUPERSEDED_CLOSE_CODE,
  deriveState,
  nextDelay,
  schedulerDecision,
  type ReconnectState,
} from './reconnect'
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

/**
 * Close code 4001: another connection took this identity (D-31). It is the ONE
 * terminal close (D-131): the token is kept and the scheduler stops for this
 * page instance, and the player's notice offers one deliberate rejoin. The
 * literal lives in `reconnect.ts` beside the terminal decision that reads it,
 * and is re-exported here because this module has always published it.
 */
export { SUPERSEDED_CLOSE_CODE }

/**
 * Close code 4002: the PRESENTER ended this session — D-78's `clear_players`, or
 * D-80's release of one row. The capability this device held is gone (the row
 * was deleted or its token released), so the role must return to its entry
 * state instead of rendering what it last saw. Like 4001 it is a deliberate
 * server decision, never a network drop — but it is NOT terminal: the reopened
 * socket is what lets the returned join form actually submit.
 */
export const SESSION_ENDED_CLOSE_CODE = 4002

/** The four connection states UI-SPEC allows — exactly these, no more. */
export type ConnectionStatus = ReconnectState

/** The role a socket joins as (D-23). It selects the snapshot decoder. */
export type SocketRole = 'player' | 'admin' | 'screen'

/**
 * The role-neutral socket contract (H17). The lifecycle — same-origin URL,
 * detach-then-replace, reset-per-connection gate, ONE scheduler per socket and
 * the heartbeat-driven staleness check — lives here once; the role owns its
 * join frame, its identity and its callbacks.
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
  /**
   * called when the server ends this session on purpose (D-78/D-80's close code
   * 4002): the identity is gone, so the role must drop everything it is showing
   * and return to its entry state (the player's join form).
   */
  onSessionEnded?: () => void
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
  /**
   * Reconnect immediately (D-130): reset the backoff, cancel the scheduled
   * attempt and connect now, with the carried detach-then-replace order. The
   * player's "Retry now" / "Use this device" and the admin's retry both reach
   * the socket through this one capability.
   */
  retry(): void
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
  /**
   * The presenter ended this identity's session (D-78's `clear_players`, D-80's
   * release): the server closed the socket with 4002, the stored token has been
   * dropped, and the route must go back to the join form.
   */
  onSessionEnded?(): void
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
  /** Connect now, resetting the backoff (D-130's manual retry). */
  retry(): void
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
 * per connection, the detach-then-replace order, D-130's backoff and liveness
 * policy, the status callback and the reduced-view callback. The role owns its
 * join frame, its identity storage and its callbacks; nothing role-specific
 * lives here.
 */
export function openSocket(opts: SocketOptions): Socket {
  // The gate is per CONNECTION (D-21), but the view is the app's state: it
  // survives a reconnect, and the replacement connection's snapshot replaces it.
  let gate: GateState = initialGateState()
  let view: ViewState = initialViewState()
  let current: WebSocket | null = null
  let closed = false

  // D-130's machine. `attempts` counts consecutive failures since the last
  // successful open and is ALSO the backoff's exponent, so "the manual retry
  // resets the backoff" is one assignment rather than a second counter free to
  // drift from the first. `lastOpenAt` is never reset by anything: it is the
  // age half of the offline threshold, and a retry cannot un-age the page.
  let attempts = 0
  let lastOpenAt: number | null = null
  let lastCloseCode: number | null = null
  /** D-132: the arrival instant of the newest ADMITTED frame (control frames included). */
  let lastMessageAt = 0
  /** THE scheduled attempt. One scheduler per socket (T-07-09): every path that
   *  reschedules clears this first, so two retries can never race. */
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  /** D-133: when this tab went hidden, or null while it is visible. */
  let hiddenAt: number | null = null

  function now(): number {
    return Date.now()
  }

  function isOpen(): boolean {
    return current !== null && current.readyState === SOCKET_OPEN
  }

  /** The machine's current reading, as the UI declares it (D-130). */
  function status(): ConnectionStatus {
    return deriveState({ open: isOpen(), attempts, lastOpenAt, now: now(), lastCloseCode })
  }

  function publishStatus(): void {
    opts.onStatus(status())
  }

  function cancelScheduled(): void {
    if (retryTimer === null) return
    clearTimeout(retryTimer)
    retryTimer = null
  }

  function send(frame: unknown): void {
    const socket = current
    if (socket === null || socket.readyState !== SOCKET_OPEN) return
    socket.send(JSON.stringify(frame))
  }

  /**
   * D-130's schedule: one attempt, full jitter, capped — forever. `offline` is
   * a presentation threshold, not a stop (G-2), so the only state that refuses
   * to schedule is D-131's terminal supersede.
   */
  function scheduleRetry(): void {
    if (closed) return
    if (schedulerDecision(status()) === 'stop') return
    const delay = nextDelay(attempts, Math.random())
    retryTimer = setTimeout(() => {
      retryTimer = null
      connect()
    }, delay)
  }

  /**
   * Connect NOW (D-130's manual retry, and D-131's one deliberate rejoin):
   * reset the backoff, cancel the waiting attempt and run the ordinary
   * detach-then-replace connect — so a retry re-sends the stored token exactly
   * like any other connect, and a 4001 rejoin supersedes the other device in
   * turn.
   */
  function retryNow(): void {
    if (closed) return
    attempts = 0
    // A deliberate rejoin answers the terminal close, so the machine must stop
    // reading it as terminal — otherwise the page could never leave superseded.
    lastCloseCode = null
    connect()
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
    // the UI selects its copy from the dictionary by `error.code`. An unknown
    // code (D-138's `rate_limited` included) stays here and paints nothing.
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

    // D-132: EVERY admitted frame is liveness, control frames included. The
    // 25 s heartbeat exists precisely so a quiet LOBBY/INTRO/SETTLED stream is
    // not mistaken for a dead socket by the staleness check below.
    lastMessageAt = now()

    const step = nextOutcome(gate, envelope)
    gate = step.state

    switch (step.outcome.kind) {
      case 'duplicate':
        return
      case 'control':
        // The control plane is admitted and consumed by the pattern above: the
        // beat and the tunnel frame carry no game state to reduce.
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

    // The scheduled attempt (if any) IS this connect: clear it first so a
    // manual retry racing the timer cannot leave two timers alive.
    cancelScheduled()

    // D-21/GAP 2: `seq` is THIS connection's stream position and the server
    // restarts it at 1 on every accepted upgrade, so the gate is one per
    // connection. Reusing the dead socket's high-water mark would classify the
    // replacement's hello — and every later sibling at or below that mark — as
    // a duplicate and silently drop it: the resume would look alive while its
    // stream stayed frozen.
    gate = resetStream()

    // Detach and close the connection being replaced BEFORE opening its
    // successor, so a late event from the old socket can never be mistaken for
    // the new one's (the StrictMode double-invoke, the 4001 case, and D-133's
    // teardown-rejoin, which must never reuse the old socket).
    const previous = current
    current = null
    if (previous !== null) {
      previous.onopen = null
      previous.onmessage = null
      previous.onclose = null
      previous.onerror = null
      previous.close()
    }

    publishStatus()

    let socket: WebSocket
    try {
      socket = new WebSocket(socketURL())
    } catch {
      // The constructor throws on an unusable URL (e.g. a built index.html
      // opened from file://). Without this the status line would stay
      // "connecting" forever, which mid-demo reads as a hung server. It is a
      // failed attempt like any other: count it and let the policy schedule the
      // next one — auto-retry never stops (G-2).
      attempts += 1
      lastCloseCode = null
      publishStatus()
      scheduleRetry()
      return
    }
    current = socket
    lastMessageAt = now()

    socket.onopen = () => {
      if (current !== socket) return
      // A successful open resets the backoff and the age threshold at once
      // (D-130), so the state is `live` until the next close says otherwise.
      attempts = 0
      lastOpenAt = now()
      lastCloseCode = null
      lastMessageAt = now()
      publishStatus()
      // The role's join. The player sends the stored token when there is one
      // (D-30 (1)) and otherwise waits for the form; the admin and the screen
      // always join with their role — there is no identity to restore.
      const frame = opts.joinFrame(opts.readToken())
      if (frame !== null) send(frame)
    }

    socket.onmessage = (event: MessageEvent) => onFrame(socket, event.data)

    socket.onclose = (event: CloseEvent) => {
      if (current !== socket) return // a deliberate close already ran
      current = null
      lastCloseCode = event.code

      if (event.code === SESSION_ENDED_CLOSE_CODE) {
        // D-78/D-80: the presenter ended this session, so the identity this
        // device held is gone and the view it is showing is stale by
        // construction. Drop the reduced view here (the module's copy of it is
        // the app's state, so it must not outlive the identity it described)
        // and hand the role the two signals that return it to its entry state:
        // `onTokenInvalid` clears the stored token when this role has one, and
        // `onSessionEnded` resets the route.
        view = initialViewState()
        opts.onTokenInvalid?.()
        opts.onSessionEnded?.()
      }

      if (event.code === SUPERSEDED_CLOSE_CODE) {
        // D-131: terminal for this page instance. The token is KEPT — it is the
        // identity the notice's one action re-claims — and the scheduler is
        // stopped, which is what ends the page-supersedes-page fight (Pitfall 12).
        cancelScheduled()
        publishStatus()
        return
      }

      attempts += 1
      publishStatus()
      // 4002 is an ordinary close for the scheduler even though it cleared the
      // identity: the reopened socket is what makes the join form the route
      // just returned to actually submit.
      scheduleRetry()
    }

    socket.onerror = () => {
      // The close event carries the state; nothing to add here.
    }
  }

  /**
   * D-132's staleness check. A phone that was suspended can hold a socket whose
   * `readyState` still says OPEN while nothing will ever arrive on it, so the
   * liveness signal has to be the frames themselves. Only the CURRENT socket is
   * ever considered, and a hidden tab is skipped entirely: iOS throttles its
   * timers, so a hidden-tab tick is not evidence about the socket (D-133).
   */
  function checkStale(): void {
    if (closed) return
    const socket = current
    if (socket === null || socket.readyState !== SOCKET_OPEN) return
    if (document.visibilityState !== 'visible') return
    if (now() - lastMessageAt <= STALE_SILENCE_MS) return
    // Force-close through the ordinary close path: the ordinary transition, no
    // new state, no copy, no notice (D-132). `onclose` is still attached, so
    // the retry policy runs exactly as it does for a network drop.
    socket.close()
  }

  /**
   * D-133's Apple-shaped recovery. Returned from a tab that was hidden for more
   * than the teardown threshold: the old socket is never reused (it may be a
   * zombie whose readyState lies), so the ordinary connect replaces it and the
   * role rejoins with whatever identity it holds.
   */
  function onVisibilityChange(): void {
    if (document.visibilityState !== 'visible') {
      hiddenAt = now()
      return
    }
    const hiddenForMs = hiddenAt === null ? 0 : now() - hiddenAt
    hiddenAt = null
    if (hiddenForMs > HIDDEN_TEARDOWN_MS) retryNow()
  }

  /**
   * The bfcache restore. `event.persisted === true` is read EXPLICITLY because
   * a bfcache pageshow fires no visibilitychange at all — this is the only
   * signal that the page was resurrected rather than loaded.
   */
  function onPageShow(event: PageTransitionEvent): void {
    if (event.persisted !== true) return
    retryNow()
  }

  const staleTimer = setInterval(checkStale, STALE_CHECK_PERIOD_MS)

  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('pageshow', onPageShow)
  connect()

  return {
    send,
    retry: retryNow,
    close(): void {
      closed = true
      cancelScheduled()
      clearInterval(staleTimer)
      document.removeEventListener('visibilitychange', onVisibilityChange)
      window.removeEventListener('pageshow', onPageShow)
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
 * `tunnel.status`, `heartbeat`, later phases' frames — AND every decode
 * failure, so an invalid or phase-inconsistent payload can never reach `onView`
 * (T-03-09).
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
 * now a thin role wrapper over `openSocket`. The signature, the callbacks and
 * the token behaviour are unchanged: the player joins by name from the form, or
 * rejoins with the stored token the moment the socket opens.
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
    onSessionEnded: handlers.onSessionEnded,
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
    retry(): void {
      socket.retry()
    },
    close(): void {
      socket.close()
    },
  }
}
