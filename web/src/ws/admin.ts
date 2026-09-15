/**
 * The admin route's socket (D-64/H17) — RED stub.
 *
 * The admin connection shares the role-neutral lifecycle extracted from
 * `client.ts` (`openSocket`), but owns its own join frame
 * (`{type:"join", role:"admin"}`), its own reduced view, its command API and
 * NO localStorage path — the admin role has no token identity at all (D-29).
 *
 * The connection is implemented in the GREEN commit; this is the RED contract.
 */

import type { ConnectionStatus } from './client'
import type { ErrorPayload } from './protocol'
import type { ViewState } from './view'

export type AdminSocketHandlers = {
  /** the same four states the player route renders (UI-SPEC §Admin status line) */
  onStatus(status: ConnectionStatus): void
  /** one reduced view per admitted admin or event frame */
  onView(view: ViewState): void
  onError?(error: ErrorPayload): void
}

/** The presenter's handle: the D-65 command frames go out through `send`. */
export type AdminConnection = {
  send(frame: unknown): void
  close(): void
}

/** Open the admin connection. RED stub: no socket is opened yet. */
export function openAdminSocket(handlers: AdminSocketHandlers): AdminConnection {
  void handlers
  return { send: () => {}, close: () => {} }
}
