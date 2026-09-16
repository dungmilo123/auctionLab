/**
 * The projector's socket (D-87).
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
 * The join is a passive role: the server answers with the role projection on
 * every mutation, and an `error` frame is logged by `openSocket` and otherwise
 * ignored — the projector has no error copy to render (UI-SPEC §Screen Route).
 */

import { openSocket, type ConnectionStatus, type Socket } from './client'
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

/**
 * Open the screen connection. The socket is created immediately; the join frame
 * goes out when the upgrade completes, and every admitted frame is decoded,
 * reduced and handed to `onView`.
 */
export function openScreenSocket(handlers: ScreenSocketHandlers): ScreenConnection {
  const socket: Socket = openSocket({
    role: 'screen',
    // No identity to restore (D-29/D-87): `readToken` is always empty, so
    // D-45's resume decision is always `none` and nothing is ever persisted.
    readToken: () => '',
    joinFrame: () => ({ type: 'join', role: 'screen' }),
    onStatus: handlers.onStatus,
    onView: handlers.onView,
  })

  return {
    close(): void {
      socket.close()
    },
  }
}
