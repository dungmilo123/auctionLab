/**
 * The client's reconciliation gate (D-22) — pure, DOM-free and socket-free, so
 * Vitest's `node` environment can pin every rule without a browser harness.
 *
 * The three rules, exactly as D-22 states them:
 *
 *   - `seq <= lastSeq` is a duplicate or a stale replay: drop it.
 *   - `seq > lastSeq + 1` is a gap: apply the newest message anyway (it IS
 *     newer) and ask for ONE coalesced resync. A message is never dropped
 *     because its `rev` is one we have seen — several messages may share a
 *     revision (a multi-part mutation, presence, a tick), and discarding a
 *     sibling is exactly the bug Pitfall 3 describes.
 *   - `state.snapshot` is the sync point: apply it when its `rev` is current or
 *     newer, and reset the expected stream position to it.
 *
 * `tunnel.status` and `heartbeat` are the two control-plane exceptions (D-24,
 * D-132): both are applied regardless of `seq` and must never be fed through
 * the revision gate. `tunnel.status` because a tunnel URL rotation whose
 * revision looked stale is exactly when the venue needs it; `heartbeat`
 * because it carries no game state at all — it exists so the client can see
 * that a quiet phase is still alive, and a beat that triggered a resync every
 * 25 seconds would be the noise it was added to prevent (Pitfall 4).
 */

import { HEARTBEAT, STATE_SNAPSHOT, TUNNEL_STATUS, type Envelope } from './protocol'

/**
 * WebSocket readyState values, spelled as literals on purpose: the `WebSocket`
 * global does not exist in the node test environment, so this module must not
 * reference it — not even for a constant.
 */
export const SOCKET_CONNECTING = 0
export const SOCKET_OPEN = 1
export const SOCKET_CLOSING = 2
export const SOCKET_CLOSED = 3

/** The client's stream state: one per connection (D-21/D-22). */
export type GateState = {
  /** the highest stream position accounted for on this connection */
  lastSeq: number
  /** the game revision currently reconciled */
  rev: number
  /** true while a resync request is outstanding (D-22's coalescing) */
  resyncPending: boolean
}

/** The client state before a single frame has arrived. */
export function initialGateState(): GateState {
  // hello starts the stream at seq 1 (D-21) and LOBBY sits at rev 0 until the
  // first mutation (D-35), so "nothing seen yet" is 0/0.
  return { lastSeq: 0, rev: 0, resyncPending: false }
}

/** What the caller should do with one frame. */
export type Outcome =
  | { kind: 'control' }
  | { kind: 'duplicate' }
  | { kind: 'snapshot'; apply: boolean }
  | { kind: 'apply'; requestResync: boolean }

/** One gate step: the outcome plus the stream state that follows it. */
export type GateStep = {
  state: GateState
  outcome: Outcome
}

/**
 * Feed one envelope through the gate.
 *
 * `requestResync` is true at most once per gap episode: while a resync is
 * outstanding a second gap is still applied, but the request is coalesced
 * (D-22's "send one resync, do not spam"), and any snapshot — applied or
 * discarded — ends the episode because it is the server's answer by
 * construction.
 */
export function nextOutcome(state: GateState, env: Envelope): GateStep {
  // The control plane first (D-24/D-132): tunnel.status and heartbeat are
  // applied as they arrive and are NEVER fed through the revision gate — a
  // rotated tunnel URL whose rev looked stale is exactly when the venue needs
  // it, and a beat carries no game state to gate. Their stream position still
  // counts, so the next game message is not misread as a gap.
  if (env.type === TUNNEL_STATUS || env.type === HEARTBEAT) {
    return {
      state: { ...state, lastSeq: Math.max(state.lastSeq, env.seq) },
      outcome: { kind: 'control' },
    }
  }

  // The sync point. A snapshot is self-sufficient (D-25), so applying one
  // resets both the reconciled revision and the expected stream position.
  if (env.type === STATE_SNAPSHOT) {
    const apply = env.rev >= state.rev
    return {
      state: {
        lastSeq: Math.max(state.lastSeq, env.seq),
        rev: apply ? env.rev : state.rev,
        // A snapshot frame is the answer to a pending request (D-42) or the
        // automatic repair after a drop (D-41), so the episode ends here either
        // way — a discarded (older) one included.
        resyncPending: false,
      },
      outcome: { kind: 'snapshot', apply },
    }
  }

  // Dedupe on seq ONLY. A message is never discarded because its rev has been
  // seen: several messages legitimately share one revision, and dropping the
  // siblings is the Pitfall 3 bug this gate exists to prevent.
  if (env.seq <= state.lastSeq) {
    return { state, outcome: { kind: 'duplicate' } }
  }

  const gap = env.seq > state.lastSeq + 1
  const requestResync = gap && !state.resyncPending
  return {
    state: {
      // The newest message is applied even across a gap — it IS newer.
      lastSeq: env.seq,
      rev: Math.max(state.rev, env.rev),
      resyncPending: state.resyncPending || requestResync,
    },
    outcome: { kind: 'apply', requestResync },
  }
}

