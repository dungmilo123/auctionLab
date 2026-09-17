import { useEffect, useState, type ReactNode } from 'react'
import { ConnectionLine } from '@/components/ConnectionLine'
import { QrPanel } from '@/components/QrPanel'
import { anchor, formatClock, remainingMs } from '@/lib/countdown'
import { anchorDutch, atFloor, dutchPrice, elapsedOf } from '@/lib/dutch'
import { formatVND } from '@/lib/format'
import { fill, useLanguage, type CopyKey } from '@/lib/i18n'
import {
  SCREEN_SCALE_CLASS,
  isDutchAuction,
  isEnglishAuction,
  isSealedAuction,
  joinUrlInvitable,
  podiumEntries,
} from '@/lib/screen'
import type { ConnectionStatus } from '@/ws/client'
import type { LeaderEntry, PublicItem, RecentBid, SealedProgress } from '@/ws/protocol'
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
// One number is the exception, and only in the Dutch live arm: a falling price
// has to move between frames, so `DutchWells` re-derives it with the SAME pure
// module the phone calls (`lib/dutch.ts`, D-105) from the item's public
// mechanics and the anchor the newest frame carried. Everything else here is a
// projection field, or the LIVE overlay seeded from one (D-60/D-63), so a reload
// mid-run paints the whole screen from the trailing snapshot alone — and no
// client-side sort ever reorders the server's standings (ECON-05).

// The operator's cue is the SHARED indicator (G-11), with the two cells this
// route owns passed in: the live square is near-black here — the screen spends
// its one accent fill on the datum the room is watching, so its connection
// square is never accent (§Color item 10) — and the word keeps the carried
// muted Caption role, which is what keeps the projector's operator chrome
// chrome. No live region is requested: a price that re-announces four times a
// second is unusable, and no retry callback is passed, so the route declares no
// handler and stays structurally read-only (G-3).

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

/**
 * The Dutch live arm's own clock and its two wells (D-105, 05-UI-SPEC §The arm
 * table amendment). It is the exact counterpart of `ScreenCountdown`: ONE
 * component owns the route's single interval for the arm that is on screen —
 * the countdown on an English run, this on a Dutch one, mutually exclusive by
 * item type — and a frame that repeats `started_at`+`server_ts` re-anchors it
 * during render, the immediate go-LIVE tick included (D-56/D-94), so the number
 * never blanks and never runs backwards.
 *
 * The price is NOT a projection field here: it is `lib/dutch.ts`'s derivation
 * from the item's own public mechanics — the one module the phone also calls
 * (D-105/D-91), so the room and the phones can never disagree about the falling
 * number. Every input is a public projection value: the component has no
 * access to `truth_value`, a sealed amount or any other secret, and it builds
 * no frame and mounts no control — the accept lives on the phones.
 */
function DutchWells({ item, live }: { item: PublicItem; live: LiveOverlay }) {
  const { t } = useLanguage()
  const now = useNow(COUNTDOWN_INTERVAL_MS)

  // `startedAt` is the Dutch anchor (D-94) and is always present on this arm:
  // the decoder's Dutch LIVE arm refuses a payload without it, and `overlayFrom`
  // admits the item on it. The key is the frame's own pair, so a tick that
  // omits the anchor does not re-anchor (D-94) — and the fallback to 0 exists
  // only so the anchor stays total for a value that cannot occur.
  const startedAt = live.startedAt ?? 0
  const frameKey = `${startedAt}:${live.serverTs}`
  const [frame, setFrame] = useState(() => ({
    key: frameKey,
    anchor: anchorDutch(startedAt, live.serverTs, Date.now()),
  }))

  // Adjusting state during render is `ScreenCountdown`'s own rule: the anchor
  // stays in step with the frame without a second pass.
  if (frame.key !== frameKey) {
    setFrame({ key: frameKey, anchor: anchorDutch(startedAt, live.serverTs, now) })
  }

  const mechanics = {
    startPrice: item.startPrice,
    floorPrice: item.floorPrice ?? 0,
    decrementStep: item.decrementStep ?? 0,
    decrementIntervalMs: item.decrementIntervalMs ?? 0,
  }
  const price = dutchPrice(mechanics, elapsedOf(frame.anchor, now))

  return (
    <>
      {/* Both wells keep the footprint the intro arm gave them: the transition
          into LIVE changes numbers, never layout (05-UI-SPEC §arm table). */}
      <div className="grid shrink-0 grid-cols-2 gap-4">
        {/* The one accent fill of this screen-state, and the route's only
            moving number: mono + `tabular-nums` so the digits cannot change
            width between ticks (D-90's typography rule). */}
        <div className="border-2 border-border bg-primary px-4 py-2 text-primary-foreground">
          <p className="text-sm leading-[1.5] font-bold">{t('live.priceLabel')}</p>
          <p className="font-mono text-display leading-[1.2] font-bold tabular-nums whitespace-nowrap">
            {formatVND(price)}
          </p>
          <p className="text-sm leading-[1.5]">{t('amount.caption')}</p>
        </div>

        {/* No clock, no timer role and no drop countdown: the floor is context,
            not the datum — and it is deliberately not accent. */}
        <div className="border-2 border-border bg-background px-4 py-2">
          <p className="text-sm leading-[1.5] font-bold">{t('dutch.floorLabel')}</p>
          <p className="text-xl leading-[1.2] font-bold">{formatVND(mechanics.floorPrice)}</p>
          <p className="text-sm leading-[1.5]">{t('amount.caption')}</p>
        </div>
      </div>

      {/* A fact about the price's own movement, not an error: the number has
          stopped, so the room is told rather than left to infer it. */}
      {atFloor(price, mechanics.floorPrice) && (
        <p className="shrink-0 text-base leading-[1.5]">{t('dutch.atFloor')}</p>
      )}
    </>
  )
}

/**
 * The sealed live arm's two wells (D-121, 06-UI-SPEC §The arm table amendment).
 *
 * A sealed run has no price to fill the first well and no bidder to name, so
 * the arm keeps the carried pair of wells and changes what is in them: the
 * count well takes the accent fill the price well spends on every other type
 * (the count is the only moving datum the room has), and the second well is the
 * carried countdown, re-anchored by every frame that carries timing. A sealed
 * run emits NO `price.tick` (D-116), so the clock interpolates from the newest
 * frame until the settle's snapshot replaces the body.
 *
 * Nothing else may be added here: no amount, no start/current price, no bidder
 * row and no feed ever reaches this arm (T-06-13), and the count is the
 * server's own number (`sealed_progress`, already reduced) rather than a
 * client-side length — the browser counts nothing.
 */
function SealedWells({ live, sealed }: { live: LiveOverlay | null; sealed: SealedProgress | null }) {
  const { t } = useLanguage()

  return (
    // Both wells keep the footprint the intro arm gave them, so the transition
    // into LIVE changes numbers, never layout.
    <div className="grid shrink-0 grid-cols-2 gap-4">
      {/* The one accent fill of this screen-state. No label and no caption row:
          the sentence labels itself (the carried `intro.waiting` well is the
          label-less precedent), and a label above it would restate it. */}
      <div className="border-2 border-border bg-primary px-4 py-2 text-primary-foreground">
        {/* The count is the server's own field and a sealed projection always
            carries it; a payload that reached here without one renders nothing
            rather than a fabricated count (the player body's own rule). */}
        {sealed !== null && (
          <p className="text-display leading-[1.2] font-bold whitespace-nowrap">
            {fill(t('screen.sealedProgress'), {
              submitted: String(sealed.submitted),
              total: String(sealed.total),
            })}
          </p>
        )}
      </div>

      {/* The carried countdown, never accent: the count is the datum and the
          clock is context (the Dutch floor well's own rule). */}
      <div className="border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('live.timeLabel')}</p>
        {/* Anchored on a frame that carried timing; a run whose deadline is
            absent shows no clock rather than a placeholder this phase never
            agreed on (the English arm's guard, verbatim). */}
        {live !== null && live.deadlineAt !== null && (
          <ScreenCountdown deadlineAt={live.deadlineAt} serverTs={live.serverTs} />
        )}
        <p className="text-sm leading-[1.5]">{t('amount.caption')}</p>
      </div>
    </div>
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

/** One label/value row on the projector: label 14/700, value 16/400. */
function RevealRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-2">
      <p className="text-sm leading-[1.5] font-bold">{label}</p>
      <p className="flex flex-wrap items-baseline gap-2 text-base leading-[1.5]">{children}</p>
    </div>
  )
}

/**
 * The standings block, shared by SETTLED and FINALE — one component, one rule:
 * every entry renders, in the server's own order, and the block is the second of
 * the route's two declared scroll regions (20 players fill three columns at
 * 1920×1080 and need no scroll). The podium above it is a *prefix* of the same
 * list, never a second ranking.
 */
function Standings({ entries }: { entries: readonly LeaderEntry[] }) {
  const { t } = useLanguage()

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <h2 className="shrink-0 text-xl leading-[1.2] font-bold tracking-[-0.01em]">
        {t('screen.standingsHeading')}
      </h2>
      <div className="grid min-h-0 flex-1 grid-cols-2 gap-2 overflow-y-auto xl:grid-cols-3">
        {entries.map((entry) => (
          <div
            key={entry.nickname}
            className="flex min-h-11 items-center gap-2 border-2 border-border bg-card px-2 py-1"
          >
            <p className="text-sm leading-[1.5] font-bold">{entry.rank}</p>
            <p className="min-w-0 flex-1 break-words text-base leading-[1.5]">{entry.nickname}</p>
            <p className="whitespace-nowrap text-base leading-[1.5]">{formatVND(entry.budget)}</p>
            <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
          </div>
        ))}
      </div>
    </div>
  )
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
            1920×1080 and need no scroll — 7 rows × 76.04px + 6 × 11.34px of
            `gap-1` is 600.3px inside the 631px box (UI-review finding 2); a
            smaller canvas scrolls here and nowhere else on this half of the
            screen. */}
        <div className="grid min-h-0 grid-cols-2 gap-1 overflow-y-auto xl:grid-cols-3">
          {view.lobby.map((entry) => (
            <div
              key={entry.nickname}
              className="flex items-center gap-2 border-2 border-border bg-card px-2 py-0"
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
 * Body 2 — ITEM_INTRO and LIVE, one body with four arms (SCRN-02/D-105/D-121).
 * The item stays in the same geometry while the phase moves, so the projector
 * must not reflow at the moment the run opens: the arms differ in exactly the
 * cells the UI-SPEC's arm table names, and in nothing else. The English arm's
 * wells live here; the Dutch arm's live wells are `DutchWells` above, because a
 * falling price needs the one component that owns its clock; the sealed arm's
 * two wells are `SealedWells`, which swaps the count in where the price well
 * cannot fill a well for a run that has no price at all.
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
  // D-105's branch, through the pure selector rather than a literal: the Dutch
  // live arm is the one arm whose number is derived instead of projected.
  const dutchArm = liveArm && isDutchAuction(item.auctionType)
  // D-121's branch, through the same kind of selector: a sealed run has no
  // price well and no bidder row, so its arm cannot be expressed as cells of
  // the English default.
  const sealedArm = liveArm && isSealedAuction(item.auctionType)
  const typeKey = AUCTION_TYPE_KEYS[item.auctionType]
  const hint = hintRange(item)

  // The English LIVE price is the newest frame's number (snapshot, then ticks
  // and accepted bids in arrival order) — `current_price` is the top bid or the
  // start price before the first one. The Dutch and sealed arms render their own
  // wells instead, so this value is only ever read by the English live arm and
  // by the intro arm below it.
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

      {/* Both wells keep their footprint in every arm: the transition into LIVE
          changes numbers, never layout. The Dutch live arm swaps in its own
          pair (the derived price and the floor); a Dutch item can only reach
          the arms below with an overlay, because the decoder's Dutch LIVE arm
          requires all five numbers and `overlayFrom` admits the item on them.
          The sealed arm is branched on its TYPE alone, so a sealed run can
          never fall through to the price well — it renders the count the room
          watches and the carried clock, and nothing else. */}
      {dutchArm && live !== null ? (
        <DutchWells item={item} live={live} />
      ) : sealedArm ? (
        <SealedWells live={live} sealed={sealed} />
      ) : (
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
              // No label row, no number and no timer role: there is no clock
              // yet, so the well is the carried waiting sentence.
              <p className="text-base leading-[1.5]">{t('intro.waiting')}</p>
            )}
          </div>
        </div>
      )}

      {/* English only (D-105's arm-table amendment): a Dutch run has no bidder,
          so `live.noBidder` ("No bids yet") would be a false statement about it. */}
      {liveArm && isEnglishAuction(item.auctionType) && (
        <div className="flex shrink-0 flex-wrap items-baseline gap-2">
          <p className="text-sm leading-[1.5] font-bold">{t('live.topBidderLabel')}</p>
          {topBidder === null ? (
            <p className="text-base leading-[1.5]">{t('live.noBidder')}</p>
          ) : (
            <p className="min-w-0 break-words text-base leading-[1.5]">{topBidder}</p>
          )}
        </div>
      )}

      {liveArm && isEnglishAuction(item.auctionType) && <BidFeed bids={feed} />}
    </>
  )
}

/**
 * Body 3 — SETTLED (SCRN-03): the public reveal, with the True value block as
 * the state's one accent fill. The reveal rows share the player route's shape;
 * the standings underneath are the same component FINALE uses.
 */
function SettledBody({
  item,
  standings,
}: {
  item: PublicItem
  standings: readonly LeaderEntry[]
}) {
  const { t } = useLanguage()
  const settle = item.settle

  // Unreachable by construction: the decoder's totality table refuses a SETTLED
  // payload without its reveal, and an unapplied snapshot leaves the previous
  // body on screen. No "missing data" copy exists because the state does not.
  if (settle === null) return null

  const sold = settle.outcome === 'sold'

  return (
    <>
      <div className="flex shrink-0 flex-col gap-1">
        <h2 className="text-display leading-[1.2] font-bold tracking-[-0.01em]">
          {sold ? t('settle.soldHeading') : t('settle.unsoldHeading')}
        </h2>
        <p className="min-w-0 break-words text-base leading-[1.5]">{item.name}</p>
      </div>

      <div className="flex shrink-0 flex-col gap-2">
        {/* Omitted entirely when there is no winner — which is exactly the
            unsold case: a blank winner slot would read as a missing value
            rather than as "nobody won" (ECON-03). */}
        {settle.winner !== null && (
          <RevealRow label={t('settle.winnerLabel')}>
            <span className="min-w-0 break-words">{settle.winner}</span>
          </RevealRow>
        )}

        <RevealRow label={t('settle.priceLabel')}>
          <span>{formatVND(settle.price)}</span>
          <span className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</span>
        </RevealRow>

        {/* The one accent fill of this state: the teaching moment of the whole
            app (§Color item 8). */}
        <div className="flex flex-wrap items-baseline justify-between gap-2 border-2 border-border bg-primary px-4 py-2 text-primary-foreground">
          <p className="text-sm leading-[1.5] font-bold">{t('settle.truthLabel')}</p>
          <p className="flex flex-wrap items-baseline gap-2 text-base leading-[1.5]">
            <span>{formatVND(settle.truthValue)}</span>
            <span className="text-sm leading-[1.5]">{t('amount.caption')}</span>
          </p>
        </div>

        {/* `budget_delta` is the winner's net and is item-scoped, so it belongs
            to the reveal rather than to any one viewer; only an unsold item
            omits it. The plus is presentation — `formatVND` already carries a
            minus sign when the value has one. */}
        {sold && (
          <RevealRow label={t('settle.deltaLabel')}>
            <span>
              {settle.budgetDelta > 0 ? '+' : ''}
              {formatVND(settle.budgetDelta)}
            </span>
            <span className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</span>
          </RevealRow>
        )}
      </div>

      {!sold && <p className="shrink-0 text-base leading-[1.5]">{t('settle.unsoldBody')}</p>}

      <Standings entries={standings} />
    </>
  )
}

/**
 * The podium (SCRN-03/D-89): one to three blocks in rank order, the winner in
 * the accent fill and the tallest footprint. The blocks come from the standings'
 * own prefix, so a reload at FINALE — the trailing snapshot path — paints the
 * identical ceremony with no event history, and a zero-player list paints
 * nothing rather than a "3rd place: nobody" frame.
 */
function Podium({ entries }: { entries: readonly LeaderEntry[] }) {
  const { t } = useLanguage()

  return (
    // `items-end`, not `items-stretch`: the podium is a STEPPED hierarchy, so
    // each block keeps its own height and sits on the same baseline — rank 1
    // (Display nickname, `py-6`) is genuinely the tallest, and #2/#3 stay
    // shorter (UI-review finding 4).
    <div className="flex shrink-0 flex-wrap items-end gap-4">
      {entries.map((entry, index) => {
        const first = index === 0
        return (
          <div
            key={entry.nickname}
            // `basis-0` + a `min-w-40` floor: the three blocks share one line
            // while they fit and stack below when they do not, always in rank
            // order in the DOM.
            className={`flex min-w-40 flex-1 basis-0 flex-col gap-1 border-2 border-border ${
              first ? 'bg-primary px-6 py-6 text-primary-foreground' : 'bg-card px-4 py-4'
            }`}
          >
            <p className={first ? 'text-xl leading-[1.2] font-bold' : 'text-sm leading-[1.5] font-bold'}>
              {entry.rank}
            </p>
            <p
              className={`min-w-0 break-words font-bold ${
                first
                  ? 'text-display leading-[1.2]'
                  : 'text-xl leading-[1.2] tracking-[-0.01em]'
              }`}
            >
              {entry.nickname}
            </p>
            <p
              className={`whitespace-nowrap ${
                first ? 'text-xl leading-[1.2] font-bold' : 'text-base leading-[1.5]'
              }`}
            >
              {formatVND(entry.budget)}
            </p>
            <p
              className={`text-sm leading-[1.5] ${first ? '' : 'text-muted-foreground'}`}
            >
              {t('amount.caption')}
            </p>
          </div>
        )
      })}
    </div>
  )
}

/** Body 4 — FINALE (SCRN-03/ECON-05/D-89): the podium, then the full record. */
function FinaleBody({ standings }: { standings: readonly LeaderEntry[] }) {
  const { t } = useLanguage()

  return (
    <>
      <h2 className="shrink-0 text-xl leading-[1.2] font-bold tracking-[-0.01em]">
        {t('finale.heading')}
      </h2>

      <Podium entries={podiumEntries(standings)} />

      {/* The podium is the celebration and this is the result: everyone renders,
          in the server's order, in the route's second scroll region. */}
      <Standings entries={standings} />
    </>
  )
}

function ScreenView() {
  const { t } = useLanguage()
  const [status, setStatus] = useState<ConnectionStatus>('reconnecting')
  // G-1: the machine's first-open boolean, mirrored for the shared indicator's
  // `Connecting…`/`Reconnecting…` split (the rule stays in `reconnect.ts`).
  const [lastOpenAt, setLastOpenAt] = useState<number | null>(null)
  // `null` until the first admitted snapshot: before it the route shows the
  // header row and one line, and nothing else (no QR, no skeleton).
  const [view, setView] = useState<ViewState | null>(null)

  // The socket is created INSIDE the effect and closed in its cleanup — at
  // module scope it would open twice under StrictMode and supersede itself. The
  // handle has `close()` and nothing else: read-only by API, not by promise.
  useEffect(() => {
    const connection = openScreenSocket({
      onStatus: (next) => {
        if (next === 'live') setLastOpenAt(Date.now())
        setStatus(next)
      },
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
      case 'SETTLED':
        if (current.item === null) return null
        return <SettledBody item={current.item} standings={current.leaderboard} />
      case 'FINALE':
        return <FinaleBody standings={current.leaderboard} />
    }

    // Unreachable: the switch above is total over the five frozen phases (the
    // decoder refuses any other name), and this line exists only so the function
    // returns a value on every path.
    return null
  }

  return (
    // The app's only 100dvh page: nothing scrolls except the two declared
    // regions, because nobody is standing at the projector.
    <main className="flex h-dvh flex-col gap-6 p-8">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('screen.brand')}</h1>

        {/* Operator chrome, not content: no live region on this route, and the
            shared indicator's word always carries the state beside the square. */}
        <ConnectionLine status={status} lastOpenAt={lastOpenAt} liveFill="bg-foreground" muted />
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
