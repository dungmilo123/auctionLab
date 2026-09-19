import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  QUICK_BID_DELTAS,
  bidInputValue,
  bidReasonKey,
  isOutbid,
  isOutOfBudget,
  parseBidAmount,
  quickBidAmount,
  type BidReasonKey,
} from '@/lib/bid'
import { anchor, formatClock, remainingMs } from '@/lib/countdown'
import { formatUSD } from '@/lib/format'
import { fill, useLanguage, type CopyKey } from '@/lib/i18n'
import type { ConnectionStatus } from '@/ws/client'
import type { BidRejected, PublicItem } from '@/ws/protocol'
import type { LiveOverlay, ViewState } from '@/ws/view'

// Screen D of the player route (03-UI-SPEC §Screen D — LIVE, English).
//
// Body order: item name → stats grid → bidder line → next-valid-bid row →
// outbid banner → bid form → feedback row. The SNAPSHOT is the truth and the
// events are only acceleration (D-63): every value this body renders is either
// a projection field or an overlay field seeded from one, so a reload mid-LIVE
// paints the whole screen with no event history at all.
//
// The bid controls never decide anything the server would not also decide
// (D-52). `min_next_bid`, `budget`, `last_bid` and `is_top_bidder` all arrive on
// the wire; the client's only computations are the quick-button sums, the local
// pre-check (which renders the same copy key the server's refusal selects) and
// the elapsed-time countdown.
//
// One structural rule shapes this module: the local clock lives in exactly ONE
// place — the countdown's own interval — and nothing else in the body reads it.
// That keeps the display honest (D-60: only local elapsed time is ever
// subtracted from a server anchor), keeps the 4 Hz re-render inside the timer,
// and keeps every other value in this file a pure function of the frames.

/** The countdown's own cadence: ~4 Hz, and only this component re-renders at it. */
const COUNTDOWN_INTERVAL_MS = 250

const BID_INPUT_ID = 'live-bid-amount'
const NEXT_BID_ID = 'live-next-bid'
const FEEDBACK_ID = 'live-feedback'

/** The three quick steps, in the UI-SPEC's order, each with its approved label. */
const QUICK_LABEL_KEYS: Readonly<Record<number, CopyKey>> = {
  10_000: 'live.quick10k',
  50_000: 'live.quick50k',
  100_000: 'live.quick100k',
}

/**
 * The local clock, advanced by an interval and by nothing else. The lazy
 * initialiser is the one place a clock reading is taken outside the interval.
 */
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])

  return now
}

/**
 * The countdown (D-60/D-61/TIME-02). Its OWN component with its OWN ~250 ms
 * interval, so the field and the buttons beside it do not re-render four times
 * a second. Every frame that carries timing re-anchors it: the snapshot's
 * `server_ts` + the item's `deadline_at`, and every `price.tick`'s
 * `deadline_at` + envelope `ts`. Only LOCAL elapsed time is ever subtracted
 * from the anchor, so a phone whose clock is minutes off still shows the right
 * remainder — and reaching zero stops at `00:00` without inventing an end state.
 */
function Countdown({ deadlineAt, serverTs }: { deadlineAt: number; serverTs: number }) {
  const now = useNow(COUNTDOWN_INTERVAL_MS)
  const frameKey = `${deadlineAt}:${serverTs}`
  const [frame, setFrame] = useState(() => ({
    key: frameKey,
    // The arrival stamp is the phone's own reading when the frame landed; it is
    // never compared against the server's clock (D-60).
    anchor: anchor(deadlineAt, serverTs, Date.now()),
  }))

  // A frame carrying new timing re-anchors the display. Adjusting state during
  // render (rather than in an effect) is what keeps the anchor in step with the
  // frame without a second render pass, and the arrival stamp is the ticking
  // local reading rather than a clock call during render.
  if (frame.key !== frameKey) {
    setFrame({ key: frameKey, anchor: anchor(deadlineAt, serverTs, now) })
  }

  return (
    // Not a live region: a per-second announcement would be unusable, and the
    // settlement arrives as a body change, never as a timer event.
    <p
      role="timer"
      className="font-mono text-xl leading-[1.2] font-bold tabular-nums whitespace-nowrap"
    >
      {formatClock(remainingMs(frame.anchor, now))}
    </p>
  )
}

/** The two headline blocks: current price and time left, two columns at every width. */
function LiveStats({ live }: { live: LiveOverlay }) {
  const { t } = useLanguage()

  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('live.priceLabel')}</p>
        {/* Phase 6 (D-116) made the overlay's price nullable for a sealed run —
            which never reaches this body — so the value slot renders only when
            there is a number, exactly as the deadline below it does. */}
        {live.currentPrice !== null && (
          <p className="text-xl leading-[1.2] font-bold whitespace-nowrap">
            {formatUSD(live.currentPrice)}
          </p>
        )}
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
      </div>

      <div className="border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('live.timeLabel')}</p>
        {/* An English item always carries a deadline (the decoder requires the
            live fields together); a run whose deadline is absent shows no clock
            rather than a placeholder this phase never agreed on. */}
        {live.deadlineAt !== null && (
          <Countdown deadlineAt={live.deadlineAt} serverTs={live.serverTs} />
        )}
      </div>
    </div>
  )
}

/**
 * The defensive body for a LIVE item this phase cannot play: a non-English type
 * (Phases 5–6 own the real bodies), or a projection the decoder would have
 * refused. It keeps the item name and any timing the server sent, and says what
 * is missing instead of rendering a blank screen.
 *
 * Exported because it is the whole family's fallback, not this arm's: the Dutch
 * body (05-04) renders it for the types it cannot play too, and `PlayerView`
 * keeps routing those to `LiveEnglish`, so there is exactly one "not yet"
 * sentence in the app.
 */
export function PendingLive({ item, live }: { item: PublicItem | null; live: LiveOverlay | null }) {
  const { t } = useLanguage()

  return (
    <>
      {item !== null && (
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em] break-words">
          {item.name}
        </h2>
      )}
      {live !== null && <LiveStats live={live} />}
      <p className="text-base leading-[1.5]">{t('live.typePending')}</p>
    </>
  )
}

type LiveEnglishProps = {
  view: ViewState
  status: ConnectionStatus
  /** D-53's private refusal, or null when there is none outstanding */
  rejection: BidRejected | null
  /** the player edited the field, so the refusal row goes away */
  onClearRejection(): void
  /** send `bid.place {amount}` — the caller owns the socket */
  onPlaceBid(amount: number): void
}

function LiveEnglish({ view, status, rejection, onClearRejection, onPlaceBid }: LiveEnglishProps) {
  const { t } = useLanguage()
  const item = view.item
  const me = view.me
  const live = view.live
  const itemId = item?.id ?? null
  const minNextBid = live?.minNextBid ?? 0

  const myLastBid = me?.lastBid ?? null
  const myTop = me?.isTopBidder ?? false

  // The field's value belongs to the item being looked at: carrying the item id
  // with the field state makes "a different item starts clean" a derived fact
  // (D-71) rather than a reset that has to be scheduled.
  const [field, setField] = useState<{ itemId: string | null; dirty: boolean; value: string }>({
    itemId,
    dirty: false,
    value: '',
  })
  const dirty = field.itemId === itemId ? field.dirty : false
  const userValue = field.itemId === itemId ? field.value : ''

  // D-71's in-flight flag, derived from the signal it was sent against: the
  // socket's status, the newest timing frame's server clock and the refusal in
  // force. It clears itself on the next accepted bid, the refusal, the next
  // snapshot (a new `serverTs`) or a status that is no longer `open` — so a
  // dropped socket can never leave the form permanently disabled, and no effect
  // has to chase any of those four.
  const [submitted, setSubmitted] = useState<string | null>(null)

  if (item === null || live === null || me === null || item.auctionType !== 'english') {
    return <PendingLive item={item} live={live} />
  }

  const flightSignal = `${status}|${live.serverTs}|${minNextBid}|${myTop}|${myLastBid}`
  const bidding = submitted !== null && submitted === flightSignal

  // The window as the newest frame described it — the server's own remaining,
  // with no local arrival stamp. Ticks arrive four times a second while the run
  // is LIVE and the settle lands within a tick of the deadline, so this is at
  // most one tick stale; the countdown above owns the local clock and the
  // clamped display (D-60).
  const remaining = Math.max(0, live.deadlineAt === null ? 0 : live.deadlineAt - live.serverTs)

  const rawValue = bidInputValue(dirty, userValue, live.minNextBid)
  const parsed = parseBidAmount(rawValue)
  const outOfBudget = isOutOfBudget(me.budget, live.minNextBid)
  const belowMinimum = parsed !== null && parsed < live.minNextBid
  const outbid = isOutbid({
    phase: view.phase,
    lastBid: myLastBid,
    isTopBidder: myTop,
    remainingMs: remaining,
  })
  // Enabled ⟺ the socket is open, the budget covers the minimum, nothing is in
  // flight, the window is open, and the value is a whole number at or above the
  // server's minimum. The server still decides (D-52).
  const canSubmit =
    status === 'live' &&
    !outOfBudget &&
    !bidding &&
    remaining > 0 &&
    parsed !== null &&
    parsed >= live.minNextBid
  const canRebid = status === 'live' && !outOfBudget && !bidding && remaining > 0

  // One slot, never stacked. A refusal is the most recent fact and wins — but
  // the local pre-check renders the same copy key from the SAME dictionary, so
  // when the minimum has risen above what was typed the fresher sentence takes
  // the refusal's place rather than sitting beside it (D-52/D-71).
  const showRejection = rejection !== null && !belowMinimum
  const showBudgetLow = !showRejection && outOfBudget
  const showBelowMinimum = !showRejection && !showBudgetLow && belowMinimum
  const hasErrorRow = showRejection || showBelowMinimum

  /** The refusal's copy: `reason` selects it, the server's `message` never renders. */
  const reasonText = (key: BidReasonKey): string => {
    switch (key) {
      case 'reason.belowMinimum':
        return fill(t(key), { min: formatUSD(rejection?.minNextBid ?? live.minNextBid) })
      case 'reason.exceedsBudget':
        return fill(t(key), { budget: formatUSD(rejection?.budget ?? me.budget) })
      default:
        return t(key)
    }
  }

  function send(amount: number) {
    setSubmitted(flightSignal)
    onClearRejection()
    onPlaceBid(amount)
  }

  function submit() {
    if (!canSubmit || parsed === null) return
    send(parsed)
  }

  function edit(next: string) {
    setField({ itemId, dirty: true, value: next })
    onClearRejection()
  }

  return (
    <>
      <div className="flex flex-col gap-1">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em] break-words">
          {item.name}
        </h2>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('auction.english')}</p>
      </div>

      <LiveStats live={live} />

      <div className="flex flex-wrap items-baseline gap-2">
        <p className="text-sm leading-[1.5] font-bold">{t('live.topBidderLabel')}</p>
        {live.topBidder === null ? (
          <p className="text-base leading-[1.5]">{t('live.noBidder')}</p>
        ) : (
          <>
            <p className="min-w-0 break-words text-base leading-[1.5]">{live.topBidder}</p>
            {myTop && (
              <span className="text-sm leading-[1.5] font-bold text-muted-foreground">
                {t('lobby.you')}
              </span>
            )}
          </>
        )}
      </div>

      {/* The minimum is always one glance above the field — and this row is the
          field's own description target. */}
      <div id={NEXT_BID_ID} className="flex flex-wrap items-baseline gap-2">
        <p className="text-sm leading-[1.5] font-bold">{t('live.nextBidLabel')}</p>
        <p className="text-base leading-[1.5]">{formatUSD(live.minNextBid)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
      </div>

      {/* D-72's banner is derived from state and nothing else: it disappears by
          itself when you are top again or the window closes, and it never
          re-appears on the next item. */}
      {outbid && (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-2 border-2 border-destructive bg-card px-2 py-2"
        >
          <p className="min-w-0 flex-1 text-base leading-[1.5]">{t('live.outbid')}</p>
          <Button
            type="button"
            // The accent marks the recommended action while the banner is
            // visible: the one-tap rebid. The form's CTA is deliberately
            // rendered as an outline at the same moment, so exactly one accent
            // fill is ever on screen.
            variant={canRebid ? 'default' : 'outline'}
            disabled={!canRebid}
            onClick={() => send(live.minNextBid)}
          >
            {fill(t('live.rebid'), { amount: formatUSD(live.minNextBid) })}
          </Button>
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor={BID_INPUT_ID}>{t('live.bidLabel')}</Label>
        <Input
          id={BID_INPUT_ID}
          className="bg-input"
          // A raw digit string, not a number field: the server reads a plain
          // integer and FRMT-01 (Phase 8) owns formatted inputs. Grouped
          // amounts are a display form only.
          type="text"
          inputMode="numeric"
          autoComplete="off"
          value={rawValue}
          disabled={bidding || outOfBudget}
          aria-invalid={hasErrorRow}
          aria-describedby={hasErrorRow ? `${NEXT_BID_ID} ${FEEDBACK_ID}` : NEXT_BID_ID}
          onChange={(event) => edit(event.target.value)}
          onKeyDown={(event) => {
            // Enter submits — there is no form element and no page reload.
            if (event.key === 'Enter') {
              event.preventDefault()
              submit()
            }
          }}
        />

        <div className="flex flex-wrap gap-2">
          {QUICK_BID_DELTAS.map((delta) => {
            const amount = quickBidAmount(live.minNextBid, delta)
            return (
              <Button
                key={delta}
                type="button"
                variant="outline"
                // A `flex-1`-only button gets `flex-basis: 0%`, so the row held
                // all three on one ever-shrinking line and its wrap never
                // engaged: at 320px the labels (85–93px) overflowed their 68px
                // boxes and collided (03-UI-REVIEW.md Pillar 2 BLOCKER). The
                // max-content floor makes a button at least as wide as its own
                // label — the declared wrap (03-UI-SPEC §Spacing) happens at
                // narrow widths, and rows with room stay single-line.
                className="flex-1 min-w-max"
                disabled={bidding || outOfBudget}
                onClick={() => edit(String(amount))}
              >
                <span>{t(QUICK_LABEL_KEYS[delta] ?? 'live.quick10k')}</span>
                <span aria-hidden="true">→</span>
                <span>{formatUSD(amount)}</span>
              </Button>
            )
          })}
        </div>

        <Button
          type="button"
          // The accent marks the one recommended action — and never while the
          // outbid banner owns it. A blocked CTA renders as an outline.
          variant={!outbid && canSubmit ? 'default' : 'outline'}
          disabled={!canSubmit}
          onClick={submit}
        >
          {bidding ? t('live.bidding') : t('live.placeBid')}
        </Button>
      </div>

      {/* The feedback slot: a refusal or the out-of-budget explanation, never
          both. The budget block is the explanation's own shape, reused. */}
      {showBudgetLow && (
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
            <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
            <p className="text-xl leading-[1.2] font-bold">{formatUSD(me.budget)}</p>
            <p className="text-sm leading-[1.5] text-muted-foreground">{t('lobby.budgetCaption')}</p>
          </div>
          <p className="text-base leading-[1.5]">
            {fill(t('live.budgetLow'), {
              budget: formatUSD(me.budget),
              min: formatUSD(live.minNextBid),
            })}
          </p>
        </div>
      )}

      {hasErrorRow && (
        // The Phase 2 error row, reused: the destructive cue is the border and
        // the copy stays near-black. `role="alert"` because it is something the
        // player must act on; the input points at it by id.
        <p
          id={FEEDBACK_ID}
          role="alert"
          className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]"
        >
          {reasonText(
            showRejection && rejection !== null
              ? bidReasonKey(rejection.reason)
              : 'reason.belowMinimum',
          )}
        </p>
      )}
    </>
  )
}

export default LiveEnglish
