/**
 * The admin route's socket (D-64/H17).
 *
 * The presenter's connection shares the role-neutral lifecycle extracted from
 * `client.ts` (`openSocket`) — the same-origin URL, the per-connection gate,
 * the detach-then-replace order and the four-state status callback — but it is
 * a deliberately different role:
 *
 *   - it joins as `{type:"join", role:"admin"}` the moment the socket opens,
 *     with NO key in this phase (ADMN-04 is Phase 4, D-64);
 *   - it has NO localStorage path and NO identity: a drop is never resumed by
 *     D-45's visibility/pageshow rule, because there is nothing to restore —
 *     the presenter reloads the page;
 *   - its state arrives through the same reduced `ViewState`, so the panel
 *     renders server snapshots only and never optimistic state (contract #1);
 *   - it exposes the D-65 command frames through one `send` API.
 */

import { openSocket, type ConnectionStatus, type Socket } from './client'
import type { ErrorPayload } from './protocol'
import type { ViewState } from './view'

export type AdminSocketHandlers = {
  /** the same four states the player route renders (UI-SPEC §Admin status line) */
  onStatus(status: ConnectionStatus): void
  /** one reduced view per admitted admin or event frame */
  onView(view: ViewState): void
  /** `bad_message`/`not_authorized` and every other typed refusal */
  onError?(error: ErrorPayload): void
}

/** The presenter's handle: the D-65 command frames go out through `send`. */
export type AdminConnection = {
  send(frame: unknown): void
  close(): void
}

/**
 * Open the admin connection. The socket is created immediately; the join frame
 * goes out when the upgrade completes, and every admitted frame is decoded,
 * reduced and handed to `onView`.
 */
export function openAdminSocket(handlers: AdminSocketHandlers): AdminConnection {
  const socket: Socket = openSocket({
    role: 'admin',
    // The admin role has no stored identity (D-29): `readToken` is always empty,
    // so D-45's resume decision is always `none` and nothing is ever persisted.
    readToken: () => '',
    joinFrame: () => ({ type: 'join', role: 'admin' }),
    onStatus: handlers.onStatus,
    onView: handlers.onView,
    onError: handlers.onError,
  })

  return {
    send(frame: unknown): void {
      socket.send(frame)
    },
    close(): void {
      socket.close()
    },
  }
}
