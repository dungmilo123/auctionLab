import { useEffect, useState } from 'react'
import { QrPanel } from '@/components/QrPanel'
import { anchor, formatClock, remainingMs } from '@/lib/countdown'
import { formatVND } from '@/lib/format'
import { fill, useLanguage, type CopyKey } from '@/lib/i18n'
import {
  SCREEN_SCALE_CLASS,
  isEnglishAuction,
  isSealedAuction,
  joinUrlInvitable,
} from '@/lib/screen'
import type { ConnectionStatus } from '@/ws/client'
import type { PublicItem, RecentBid, SealedProgress } from '@/ws/protocol'
import { openScreenSocket } from '@/ws/screen'
import type { LiveOverlay, ViewState } from '@/ws/view'

// The projector (04-UI-SPEC.md §Screen Route & Interaction Contract, D-87..D-90).
//
// `/screen` is a READ-ONLY display. The handle `openScreenSocket` returns has no
// `send` at all (the type is `{ close(): void }`), this file builds no command
// frame and mounts no control — read-only is a property of the API, not a
// promise in a comment.
//
// It is also the app's only 100dvh page: a projector has nobody to scroll it,
// so exactly two regions scroll (the lobby player grid and the standings block)
// and everything else is laid out inside the viewport.
//
// One socket, one reduced view, one body per phase:
//
//   LOBBY             → the QR plate, the join URL and the player grid
//   ITEM_INTRO / LIVE → one item body, two arms: the intro prices the item, the
//                       live arm moves the price and counts the window down
//   SETTLED / FINALE  → the public reveal, the podium and the standings
//
// Nothing here derives an auction fact. Every number is a projection field, or
// the LIVE overlay seeded from one (D-60/D-63), so a reload mid-run paints the
// whole screen from the trailing snapshot alone — and no client-side sort ever
// reorders the server's standings (ECON-05).

const STATUS_COPY: Record<ConnectionStatus, CopyKey> = {
  connecting: 'status.connecting',
  open: 'status.open',
  lost: 'status.lost',
  superseded: 'status.superseded',
}

// The operator's cue, deliberately NOT the player route's: the screen spends its
// one accent fill on the datum the room is watching, so `open` is near-black
// here (§Color item 10). The square never carries the state alone — the word is
// always beside it, and this route has no live region at all (a price that
// re-announces four times a second is unusable).
const STATUS_FILL: Record<ConnectionStatus, string> = {
  connecting: 'bg-muted-foreground',
  open: 'bg-foreground',
  lost: 'bg-destructive',
  superseded: 'bg-destructive',
}

// The four frozen auction types, mapped to their approved names. `ItemIntro`
// carries the same map for the player route; lifting it would touch a shipped
// Phase-3 file that this plan does not own, and the fallback (the server's own
// value) is identical on both routes.
const AUCTION_TYPE_KEYS: Readonly<Record<string, CopyKey>> = {
  english: 'auction.english',
  dutch: 'auction.dutch',
  sealed_first: 'auction.sealedFirst',
  vickrey: 'auction.vickrey',
}

/** D-88's feed is "up to five"; the server already caps it (the client re-caps). */
const MAX_FEED_ROWS = 5

/**
 * The countdown's own cadence: ~4 Hz, and only this component re-renders at it.
 * It is the projector's only local clock (D-60): the anchor is the server's own
 * remaining duration plus the local instant the frame landed, so a device whose
 * clock is minutes off still shows the right remainder.
 *
 * Not a live region — the audience is a room, and the settle arrives as a body
 * change rather than a timer event.
 */
const COUNTDOWN_INTERVAL_MS = 250

/** The local clock, advanced by an interval and by nothing else. */
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])

  return now
}

/** The Display-role clock of the LIVE arm, re-anchored by every timing frame. */
function ScreenCountdown({ deadlineAt, serverTs }: { deadlineAt: number; serverTs: number }) {
  const now = useNow(COUNTDOWN_INTERVAL_MS)
  const frameKey = `${deadlineAt}:${serverTs}`
  const [frame, setFrame] = useState(() => ({
    key: frameKey,
    anchor: anchor(deadlineAt, serverTs, Date.now()),
  }))

  // Adjusting state during render is what keeps the anchor in step with the
  // frame without a second pass; the arrival stamp is the ticking local reading,
  // never a clock call compared against the server's (D-60).
  if (frame.key !== frameKey) {
    setFrame({ key: frameKey, anchor: anchor(deadlineAt, serverTs, now) })
  }

  return (
    <p
      role="timer"
      className="font-mono text-display leading-[1.2] font-bold tabular-nums whitespace-nowrap"
    >
      {formatClock(remainingMs(frame.anchor, now))}
    </p>
  )
}

/** The hint range, or null when the item carries no hint at all (a fact, not an error). */
function hintRange(item: PublicItem): string | null {
  const { hintMin, hintMax } = item
  if (hintMin !== null && hintMax !== null) return `${formatVND(hintMin)} – ${formatVND(hintMax)}`
  if (hintMin !== null) return formatVND(hintMin)
  if (hintMax !== null) return formatVND(hintMax)
  return null
}

/** Body 1 — LOBBY (SCRN-01): the QR plate is the focal point, the grid stays quiet. */
function LobbyBody({ view }: { view: ViewState }) {
  const { t, count } = useLanguage()
  const invitable = joinUrlInvitable(view.joinUrl)

  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-8 md:grid-cols-2">
      <div className="flex min-h-0 flex-col gap-4">
        <p className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('screen.scanNote')}</p>

        {/* The one state where the projector cannot invite anyone: the copy names
            the fix rather than showing a plate that encodes nothing. */}
        {invitable ? (
          <>
            <QrPanel url={view.joinUrl} size={240} className="w-[40vmin] h-auto" />
            <p className="font-mono break-all text-base leading-[1.5]">{view.joinUrl}</p>
          </>
        ) : (
          <p className="text-base leading-[1.5]">{t('screen.noJoinUrl')}</p>
        )}
      </div>

      <div className="flex min-h-0 flex-col gap-2">
        {/* The Display node of this state. Zero players is not an error: the
            count reads `0 players` and the grid renders nothing. */}
        <p className="text-display leading-[1.2] font-bold">{count(view.lobby.length)}</p>

        {/* Declared scroll region #1: 20 players fill three columns at
            1920×1080 and need no scroll; a smaller canvas scrolls here and
            nowhere else on this half of the screen. */}
        <div className="grid min-h-0 grid-cols-2 gap-2 overflow-y-auto xl:grid-cols-3">
          {view.lobby.map((entry) => (
            <div
              key={entry.nickname}
              className="flex min-h-11 items-center gap-2 border-2 border-border bg-card px-2 py-1"
            >
              <span
                aria-hidden="true"
                className={`size-3 shrink-0 border-2 border-border ${
                  entry.connected ? 'bg-foreground' : 'bg-transparent'
                }`}
              />
              {/* Presence is never the square alone, here either: the fill is
                  hidden from assistive technology and this text is the state. */}
              <span className="sr-only">
                {entry.connected ? t('presence.connected') : t('presence.disconnected')}
              </span>
              <span className="min-w-0 break-words text-base leading-[1.5]">{entry.nickname}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

/** D-88's newest-first feed: public nicknames and amounts, the newest at 700. */
function BidFeed({ bids }: { bids: readonly RecentBid[] }) {
  const { t } = useLanguage()

  if (bids.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('screen.bidFeedHeading')}</p>
        <p className="text-base leading-[1.5]">{t('live.noBidder')}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm leading-[1.5] text-muted-foreground">{t('screen.bidFeedHeading')}</p>
      {bids.slice(0, MAX_FEED_ROWS).map((bid, index) => {
        // The newest row is the one the room just watched land; the rest are
        // history, so only it carries 700.
        const weight = index === 0 ? 'font-bold' : ''
        return (
          <div
            key={`${bid.ts}-${bid.nickname}-${index}`}
            className="flex items-center gap-2 border-2 border-border bg-card px-2 py-1"
          >
            <p className={`min-w-0 flex-1 break-words text-base leading-[1.5] ${weight}`}>
              {bid.nickname}
            </p>
            <p className={`whitespace-nowrap text-base leading-[1.5] ${weight}`}>
              {formatVND(bid.amount)}
            </p>
            <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
          </div>
        )
      })}
    </div>
  )
}

/**
 * Body 2 — ITEM_INTRO and LIVE, one body with two arms (SCRN-02). The item stays
 * in the same geometry while the phase moves, so the projector must not reflow
 * at the moment the run opens: the two arms differ in exactly the cells the
 * UI-SPEC's arm table names, and in nothing else.
 */
function ItemBody({
  item,
  live,
  sealed,
  feed,
}: {
  item: PublicItem
  live: LiveOverlay | null
  /** D-88's count-only counter, already reduced to "absent" when malformed */
  sealed: SealedProgress | null
  /** D-88's newest-first feed, already in wire order */
  feed: readonly RecentBid[]
}) {
  const { t } = useLanguage()
  const liveArm = item.status === 'live'
  const typeKey = AUCTION_TYPE_KEYS[item.auctionType]
  const hint = hintRange(item)

  // The LIVE price is the newest frame's number (snapshot, then ticks and
  // accepted bids in arrival order); a non-English run still carries the price
  // its engine computed — `current_price` is the top bid or the start price
  // before the first one.
  const price = liveArm ? (live?.currentPrice ?? item.currentPrice) : item.startPrice
  const topBidder = live?.topBidder ?? item.topBidder

  return (
    <>
      <div className="flex shrink-0 flex-col gap-1">
        {/* Item names run to 80 runes: the heading wraps, it never truncates. */}
        <h2 className="min-w-0 break-words text-xl leading-[1.2] font-bold tracking-[-0.01em]">
          {item.name}
        </h2>
        <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          <p className="text-sm leading-[1.5] font-bold">{t('intro.typeLabel')}</p>
          <p className="text-sm leading-[1.5] text-muted-foreground">
            {typeKey === undefined ? item.auctionType : t(typeKey)}
          </p>
          <p className="text-sm leading-[1.5] font-bold">{t('intro.hintLabel')}</p>
          {hint === null ? (
            // The both-null case is a fact about the item, not an error, so it
            // gets the carried sentence rather than an empty value slot.
            <p className="text-sm leading-[1.5] text-muted-foreground">{t('intro.hintNone')}</p>
          ) : (
            <p className="text-sm leading-[1.5] text-muted-foreground">{hint}</p>
          )}
        </div>
      </div>

      {/* Both wells keep their footprint in both arms: the transition into LIVE
          changes numbers, never layout. */}
      <div className="grid shrink-0 grid-cols-2 gap-4">
        <div
          className={`border-2 border-border px-4 py-2 ${
            liveArm ? 'bg-primary text-primary-foreground' : 'bg-background'
          }`}
        >
          <p className="text-sm leading-[1.5] font-bold">
            {liveArm ? t('live.priceLabel') : t('intro.startLabel')}
          </p>
          {price !== null && (
            <p className="text-display leading-[1.2] font-bold whitespace-nowrap">
              {formatVND(price)}
            </p>
          )}
          <p className="text-sm leading-[1.5]">{t('amount.caption')}</p>
        </div>

        <div className="border-2 border-border bg-background px-4 py-2">
          {liveArm ? (
            <>
              <p className="text-sm leading-[1.5] font-bold">{t('live.timeLabel')}</p>
              {/* The clock is anchored on a frame that carried timing; a run
                  whose deadline is absent shows no clock rather than a
                  placeholder this phase never agreed on. */}
              {live !== null && live.deadlineAt !== null && (
                <ScreenCountdown deadlineAt={live.deadlineAt} serverTs={live.serverTs} />
              )}
              <p className="text-sm leading-[1.5]">{t('amount.caption')}</p>
            </>
          ) : (
            // No label row, no number and no `role="timer"`: there is no clock
            // yet, so the well is the carried waiting sentence.
            <p className="text-base leading-[1.5]">{t('intro.waiting')}</p>
          )}
        </div>
      </div>

      {liveArm && (
        <div className="flex shrink-0 flex-wrap items-baseline gap-2">
          <p className="text-sm leading-[1.5] font-bold">{t('live.topBidderLabel')}</p>
          {topBidder === null ? (
            <p className="text-base leading-[1.5]">{t('live.noBidder')}</p>
          ) : (
            <p className="min-w-0 break-words text-base leading-[1.5]">{topBidder}</p>
          )}
        </div>
      )}

      {/* Phase 4 ships the shell; Phase 6 is what makes the numbers move. The
          line is count-only by type — the amounts live on the admin projection. */}
      {liveArm && isSealedAuction(item.auctionType) && sealed !== null && (
        <p className="shrink-0 text-xl leading-[1.2] font-bold">
          {fill(t('screen.sealedProgress'), {
            submitted: String(sealed.submitted),
            total: String(sealed.total),
          })}
        </p>
      )}

      {liveArm && isEnglishAuction(item.auctionType) && <BidFeed bids={feed} />}
    </>
  )
}

function ScreenView() {
  const { t } = useLanguage()
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  // `null` until the first admitted snapshot: before it the route shows the
  // header row and one line, and nothing else (no QR, no skeleton).
  const [view, setView] = useState<ViewState | null>(null)

  // The socket is created INSIDE the effect and closed in its cleanup — at
  // module scope it would open twice under StrictMode and supersede itself.
  useEffect(() => {
    const connection = openScreenSocket({
      onStatus: setStatus,
      onView: (next) => setView(next),
    })

    return () => connection.close()
  }, [])

  // The route's one global side effect (D-90): the projector base. Adding the
  // class to the document root is idempotent under StrictMode's double-invoke,
  // and the cleanup removes exactly what this route added.
  useEffect(() => {
    const root = document.documentElement
    root.classList.add(SCREEN_SCALE_CLASS)
    return () => root.classList.remove(SCREEN_SCALE_CLASS)
  }, [])

  function renderBody(current: ViewState) {
    switch (current.phase) {
      case 'LOBBY':
        return <LobbyBody view={current} />
      case 'ITEM_INTRO':
      case 'LIVE':
        // The decoder's totality table guarantees an item here; the null arm is
        // unreachable and exists only to keep this switch total.
        if (current.item === null) return null
        return (
          <ItemBody
            item={current.item}
            live={current.live}
            sealed={current.sealedProgress}
            feed={current.recentBids}
          />
        )
      default:
        // SETTLED and FINALE land in the next task of this plan.
        return null
    }
  }

  return (
    // The app's only 100dvh page: nothing scrolls except the two declared
    // regions, because nobody is standing at the projector.
    <main className="flex h-dvh flex-col gap-6 p-8">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('screen.brand')}</h1>

        {/* Operator chrome, not content: no live region on this route, and the
            word always carries the state beside the square. */}
        <div className="flex items-center gap-2">
          <span
            aria-hidden="true"
            className={`inline-block size-3 border-2 border-border ${STATUS_FILL[status]}`}
          />
          <p className="text-sm leading-[1.5] text-muted-foreground">{t(STATUS_COPY[status])}</p>
        </div>
      </div>

      {/* The last known body deliberately stays on screen through a `lost`
          connection: the room keeps seeing the last real auction state and the
          presenter reloads when the socket is back. */}
      <div className="flex min-h-0 flex-1 flex-col gap-6">
        {view === null ? (
          <p className="text-base leading-[1.5]">{t('screen.starting')}</p>
        ) : (
          renderBody(view)
        )}
      </div>
    </main>
  )
}

export default ScreenView
