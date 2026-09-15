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
 * `tunnel.status` is the one control-plane exception (D-24): it is applied
 * regardless of `seq` and must never be fed through the revision gate, because
 * a tunnel URL rotation whose revision looked stale is exactly when the venue
 * needs it.
 */

import {
  ERROR,
  HELLO,
  STATE_SNAPSHOT,
  TUNNEL_STATUS,
  type Envelope,
} from './protocol'

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
export function nextOutcome(_state: GateState, _env: Envelope): GateStep {
  // RED sentinel: the surface is declared, the gate's rules are not — every
  // frame is reported as a duplicate until the GREEN commit replaces this body.
  return { state: _state, outcome: { kind: 'duplicate' } }
}

/** What a visibility/pageshow event should do about a closed socket (D-45). */
export type ResumeDecision = 'reconnect' | 'none'

/**
 * D-45's one-shot resume decision: on `visibilitychange → visible` and on
 * `pageshow`, a socket that is not OPEN is reconnected once and rejoined with
 * the stored token — so a locked phone comes back to the same identity and
 * budget (criterion 2).
 *
 * Without a stored token there is no identity to restore, and the join form's
 * own CTA is the recovery path — so the decision is `none` rather than opening
 * a socket that has nothing to say. Backoff, jitter and stale-socket detection
 * are deliberately absent (RECN-01/02 is Phase 7).
 */
export function resumeOutcome(_readyState: number, _hasToken: boolean): ResumeDecision {
  // RED sentinel — see nextOutcome.
  return 'none'
}

// The message-type literals the gate branches on, kept in one place so the
// rules and the tests cannot drift from the catalog in protocol.ts.
export const GATE_TYPES = { HELLO, STATE_SNAPSHOT, ERROR, TUNNEL_STATUS } as const
