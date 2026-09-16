/**
 * The projector's pure logic (D-86/D-88/D-89/D-90).
 *
 * Everything here is a selector: no DOM, no React, no socket, no clock — the
 * convention `lib/admin.ts` and `lib/bid.ts` already follow, and the only way
 * this phase can test `/screen` at all (`vitest` runs `environment: node`, so
 * there is no DOM harness to mount a route in).
 *
 * Two rules shape the file:
 *
 *   - The SERVER is the single source of truth (contract #1). Nothing here
 *     re-derives a public number: the standings arrive in the server's own
 *     ECON-05 order and the projector renders that order verbatim, so a client
 *     sort can never disagree with the room's scoreboard.
 *   - A projector must never freeze or blank. Every predicate below is a fact
 *     about what the route can invite or render — never a guess about what the
 *     server meant.
 */

import type { LeaderEntry, PlayerMe } from '@/ws/protocol'

/**
 * The class `ScreenView` puts on `document.documentElement` for the route's
 * lifetime (D-90). The stylesheet's two projector rules are scoped to
 * `html.<this>`, and `screen.test.ts` reads the stylesheet through this
 * constant — so renaming the class here without the CSS (or the reverse) fails
 * a test instead of silently leaving a projected image at the 16px base.
 */
export const SCREEN_SCALE_CLASS = 'screen-scale'

/**
 * The lobby's invitation gate (D-86, UI-SPEC §Body 1): the projector may show a
 * QR only when the selected URL is one the server itself would accept — a
 * non-empty `http://` or `https://` string. An empty or non-http URL renders
 * `screen.noJoinUrl` in the plate's place.
 */
export function joinUrlInvitable(url: string): boolean {
  // The server's rule, character for character: a case-sensitive scheme prefix
  // and no trimming (internal/game/config.go's validateJoinURL). A second,
  // tighter rule here could hide a URL the admin legitimately selected — and a
  // looser one could put a URL in a QR that the server would refuse.
  return url.startsWith('http://') || url.startsWith('https://')
}

/**
 * The sealed gate for Body 2's progress line (D-88): `sealed_progress` is
 * populated for exactly the two sealed types, so the line renders only for
 * them. `english` and `dutch` never carry one.
 */
export function isSealedAuction(auctionType: string): boolean {
  return auctionType === 'sealed_first' || auctionType === 'vickrey'
}

/**
 * The feed gate for Body 2's bid feed (UI-SPEC §Body 2): the newest-five feed
 * belongs to an English run, whose bids are public as they land. A Dutch or
 * sealed run renders rows 1–2 (and, at LIVE, the bidder row) instead — never a
 * blank body and never an invented one.
 */
export function isEnglishAuction(auctionType: string): boolean {
  return auctionType === 'english'
}

/**
 * The podium (D-89): the standings' own first three, in the order the server
 * sent them. There is no sort here and no rank arithmetic — ECON-05's one order
 * lives on the server (`leaderboardOrder`), and the podium is a prefix of the
 * same list the standings block renders underneath it, so the two can never
 * disagree. A zero-player finale yields an empty podium rather than fabricated
 * placements, and the list is copied so a caller cannot reorder the view.
 */
export function podiumEntries(standings: readonly LeaderEntry[]): LeaderEntry[] {
  // RED sentinel: the declaration `screen.test.ts` compiles against.
  void standings
  return []
}

/**
 * The player's own standing for the finale body (D-89/ECON-05), or null when
 * the block must be omitted entirely.
 *
 * The rank is the SERVER's (`me.rank` — 04-06's `readRank`, where 0 means "no
 * standing") and the total is the length of the standings the same snapshot
 * carried. Matching a nickname against a public `LeaderEntry` (which carries no
 * id) would be a second ranking rule in the browser; `is_top_bidder` is
 * server-computed for exactly this reason (D-72).
 */
export function playerRank(
  me: PlayerMe | null,
  standings: readonly LeaderEntry[],
): { rank: number; total: number } | null {
  // RED sentinel: the declaration `screen.test.ts` compiles against.
  void me
  void standings
  return null
}
