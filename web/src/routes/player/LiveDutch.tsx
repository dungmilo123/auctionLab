import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { bidReasonKey, type BidReasonKey } from '@/lib/bid'
import {
  anchorDutch,
  atFloor,
  canAccept,
  dutchPrice,
  elapsedOf,
  type DutchAnchor,
} from '@/lib/dutch'
import { formatVND } from '@/lib/format'
import { fill, useLanguage } from '@/lib/i18n'
import type { ConnectionStatus } from '@/ws/client'
import type { BidRejected, PublicItem } from '@/ws/protocol'
import type { ViewState } from '@/ws/view'
import { PendingLive } from './LiveEnglish'

// Screen D of the player route, the Dutch arm (05-UI-SPEC §Player Route &
// Interaction Contract — the Dutch LIVE body, D-103/D-104).
//
// Body order: item name + type → price well → at-floor line → hint row → floor
// row → budget block → ACCEPT → one feedback sentence. Every rendered fact is a
// projection field or a value the server's own clock derives (D-63): the price
// is the ONE derivation in `lib/dutch.ts` re-anchored by every frame that
// carries `started_at`, so a reload mid-Dutch paints the whole screen from the
// snapshot alone and ticks are only acceleration.
//
// Three structural rules shape this module:
//
//   - ONE local clock, in the BODY (D-104/05-UI-SPEC §The falling price). The
//     English body hides its clock inside the countdown leaf; Dutch inverts
//     that on purpose, because the displayed price and the ACCEPT predicate
//     must never disagree — both read the same derived value from the same
//     render. This body is cheap to re-render: one number, four rows, a button.
//   - The ACCEPT is ONE TAP and decides nothing (D-104/T-05-01): no amount
//     field, no confirmation, no optimistic winner state. The server charges
//     its own derived price; the frame it sends is `{ type: 'dutch.accept' }`
//     with nothing in it.
//   - The disabled explanation is re-derived every render (D-104), so the
//     button re-enables by itself as the price falls past the budget — it is
//     never a snapshot-only decision.

/** The body's own cadence: ~4 Hz, and only this body re-renders at it. */
const DUTCH_INTERVAL_MS = 250

const FEEDBACK_ID = 'dutch-feedback'

/**
 * The local clock, advanced by an interval and by nothing else. The lazy
 * initialiser is the one place a clock reading is taken outside the interval.
 * (The private copy is deliberate: one clock per route body, as `ScreenView`
 * keeps its own — an imported hook would hide which body owns the interval.)
 */
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])

  return now
}

/** The hint range, or null when the item carries no hint at all. */
function hintRange(item: PublicItem): string | null {
  const { hintMin, hintMax } = item
  if (hintMin !== null && hintMax !== null) return `${formatVND(hintMin)} – ${formatVND(hintMax)}`
  if (hintMin !== null) return formatVND(hintMin)
  if (hintMax !== null) return formatVND(hintMax)
  return null
}

type LiveDutchProps = {
  view: ViewState
  status: ConnectionStatus
  /** D-53's private refusal, or null when there is none outstanding */
  rejection: BidRejected | null
  /** the player acted, so the refusal row goes away */
  onClearRejection(): void
  /** send `dutch.accept` — the caller owns the socket */
  onAccept(): void
}

function LiveDutch({ view, status, rejection, onClearRejection, onAccept }: LiveDutchProps) {
  const { t } = useLanguage()
  const item = view.item
  const me = view.me
  const live = view.live

  // The ONE clock on this body. It is read before the defensive gate so the
  // hooks below are unconditional; nothing after the gate re-reads it.
  const now = useNow(DUTCH_INTERVAL_MS)

  // A frame carrying the Dutch clock re-anchors the falling price. Adjusting
  // state during render (rather than in an effect) is what keeps the anchor in
  // step with the frame without a second render pass, and the arrival stamp is
  // the ticking local reading rather than a clock call during render. Only a
  // frame that actually repeats `started_at`+`server_ts` re-anchors: a tick
  // that omits the anchor must never blank or reset the price (D-94).
  const startedAt = live?.startedAt ?? null
  const serverTs = live?.serverTs ?? 0
  const frameKey = `${startedAt}:${serverTs}`
  const [frame, setFrame] = useState<{ key: string; anchor: DutchAnchor }>(() => ({
    key: frameKey,
    anchor: anchorDutch(startedAt ?? 0, serverTs, Date.now()),
  }))
  if (frame.key !== frameKey) {
    setFrame({ key: frameKey, anchor: anchorDutch(startedAt ?? 0, serverTs, now) })
  }

  // D-71's in-flight flag, derived from the signal it was sent against: the
  // socket's status and the newest timing frame's server clock. The next frame
  // (a 250 ms beat included), a refusal, the settle's snapshot and a status
  // that is no longer `open` each change the signal, so the CTA can never be
  // left permanently disabled on `Accepting…` — and no effect has to chase any
  // of those four (05-UI-SPEC §ACCEPT, research Pitfall 14).
  const [submitted, setSubmitted] = useState<string | null>(null)

  // Defensive gate: `PlayerView` routes a Dutch item here, and every admitted
  // Dutch LIVE payload carries its mechanics — a snapshot through
  // `itemMatchesPhase`, and an `item.*` event through the same predicate in
  // `reduceView` (WR-03). The gate names those fields anyway, so this body
  // renders the sealed types' pending sentence rather than a price it could not
  // derive — the `?? 0` fallbacks below are gone because a fallback would print
  // a number the server is not charging and disable the CTA behind it.
  if (
    item === null ||
    live === null ||
    me === null ||
    item.auctionType !== 'dutch' ||
    item.currentPrice === null ||
    item.startedAt === null ||
    item.floorPrice === null ||
    item.decrementStep === null ||
    item.decrementIntervalMs === null
  ) {
    return <PendingLive item={item} live={live} />
  }

  const mechanics = {
    startPrice: item.startPrice,
    floorPrice: item.floorPrice,
    decrementStep: item.decrementStep,
    decrementIntervalMs: item.decrementIntervalMs,
  }
  // The player's own row, read once: the gate above proved it is there.
  const budget = me.budget
  // The SAME derived value feeds the price well and the CTA's predicate.
  const price = dutchPrice(mechanics, elapsedOf(frame.anchor, now))
  const flightSignal = `${status}|${live.serverTs}`
  // A refusal is an answer, so it releases the in-flight label on the spot —
  // the fourth releaser beside the next tick, the settle's snapshot and a
  // socket that left `open`. Without it the CTA would keep saying "Accepting…"
  // for the up-to-250 ms until the next beat, over a sentence that already
  // explains what happened.
  const accepting = submitted !== null && submitted === flightSignal && rejection === null
  const affordable = canAccept({ status, inFlight: accepting, price, budget })
  const pinned = atFloor(price, mechanics.floorPrice)
  const hint = hintRange(item)

  // One slot, never stacked (05-UI-SPEC §Feedback slot): a refusal is the more
  // recent fact and wins it; the too-expensive sentence is the disabled
  // control's explanation, and the budget block above already shows the number
  // it interpolates — so the sentence renders alone, with no box and no repeat.
  const refusalKey: BidReasonKey | null = rejection === null ? null : bidReasonKey(rejection.reason)
  const showTooExpensive = refusalKey === null && budget < price

  function accept() {
    if (!affordable) return
    setSubmitted(flightSignal)
    onClearRejection()
    onAccept()
  }

  function refusalText(key: BidReasonKey): string {
    // The server's own budget travels back with the refusal, so the sentence
    // names the number it was refused against rather than a stale render's.
    return key === 'reason.exceedsBudget'
      ? fill(t(key), { budget: formatVND(rejection?.budget ?? budget) })
      : t(key)
  }

  return (
    <>
      <div className="flex flex-col gap-1">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em] break-words">
          {item.name}
        </h2>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('auction.dutch')}</p>
      </div>

      {/* The body's one moving number, and its first block: mono + tabular so
          the digits cannot change width between ticks. */}
      <div className="border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('live.priceLabel')}</p>
        <p className="font-mono text-xl leading-[1.2] font-bold tabular-nums whitespace-nowrap">
          {formatVND(price)}
        </p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
      </div>

      {/* A fact about the price's own movement, not an error: it says the number
          has stopped instead of implying it can still fall (D-103). */}
      {pinned && <p className="text-base leading-[1.5]">{t('dutch.atFloor')}</p>}

      <div className="flex flex-wrap items-baseline gap-2">
        <p className="text-sm leading-[1.5] font-bold">{t('intro.hintLabel')}</p>
        {hint === null ? (
          // The both-null case is a fact about the item, never an error.
          <p className="text-base leading-[1.5]">{t('intro.hintNone')}</p>
        ) : (
          <>
            <p className="text-base leading-[1.5]">{hint}</p>
            <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
          </>
        )}
      </div>

      {/* The honest floor of the fall: the same number the price is clamped to. */}
      <div className="flex flex-wrap items-baseline gap-2">
        <p className="text-sm leading-[1.5] font-bold">{t('dutch.floorLabel')}</p>
        <p className="text-base leading-[1.5]">{formatVND(mechanics.floorPrice)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
      </div>

      {/* The shipped Phase-2 budget block, reused verbatim — and rendered
          PERMANENTLY on this body (D-103), because the ACCEPT is spending it. */}
      <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
        <p className="text-xl leading-[1.2] font-bold">{formatVND(budget)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('lobby.budgetCaption')}</p>
      </div>

      {/* One tap, no confirmation (D-104): the accent marks the enabled accept,
          and a disabled or in-flight control renders as an outline. */}
      <Button
        type="button"
        variant={affordable ? 'default' : 'outline'}
        disabled={!affordable}
        className="w-full"
        onClick={accept}
      >
        {accepting ? t('dutch.accepting') : t('dutch.accept')}
      </Button>

      {showTooExpensive && (
        <p className="text-base leading-[1.5]">
          {fill(t('dutch.tooExpensive'), { budget: formatVND(me.budget) })}
        </p>
      )}

      {refusalKey !== null && (
        // The Phase-2 error row, reused: the destructive cue is the border and
        // the copy stays near-black. `code`/`reason` select the sentence — the
        // server's `message` is never rendered (T-05-05).
        <p
          id={FEEDBACK_ID}
          role="alert"
          aria-live="polite"
          className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]"
        >
          {refusalText(refusalKey)}
        </p>
      )}
    </>
  )
}

export default LiveDutch
