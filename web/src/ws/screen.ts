/**
 * The projector's socket (D-87) — RED stub.
 *
 * The screen shares the role-neutral lifecycle `client.ts` extracted
 * (`openSocket`) — the same-origin URL, the per-connection gate, the
 * detach-then-replace order and the four-state status callback — but it is the
 * one role with NO command surface at all:
 *
 *   - it joins as `{type:"join", role:"screen"}` the moment the socket opens,
 *     with no key, no nickname and no token;
 *   - it has no localStorage path and no identity, so D-45's
 *     visibility/pageshow resume never reopens it (there is nothing to
 *     restore) — the same rule the admin role follows;
 *   - its handle is `{ close(): void }` ONLY. That is a structural read-only
 *     guarantee: no future edit can add a screen control by reaching for a
 *     `send` that exists.
 *
 * The connection is implemented in the GREEN commit; this is the RED contract.
 */

import type { ConnectionStatus } from './client'
import type { ViewState } from './view'

export type ScreenSocketHandlers = {
  /** the same four states the player route renders (UI-SPEC §Connection cue) */
  onStatus(status: ConnectionStatus): void
  /** one reduced view per admitted public frame */
  onView(view: ViewState): void
}

/** The projector's handle: `close` is the whole API — there is no `send` (D-87). */
export type ScreenConnection = {
  close(): void
}

/** Open the screen connection. RED stub: no socket is opened yet. */
export function openScreenSocket(handlers: ScreenSocketHandlers): ScreenConnection {
  void handlers
  return { close: () => {} }
}
