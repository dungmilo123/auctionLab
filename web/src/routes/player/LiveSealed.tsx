import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { bidReasonKey, parseBidAmount, type BidReasonKey } from '@/lib/bid'
import { anchor, formatClock, remainingMs, type TimingAnchor } from '@/lib/countdown'
import { formatVND } from '@/lib/format'
import { fill, useLanguage } from '@/lib/i18n'
import { isSealedAuction } from '@/lib/screen'
import { canSubmitSealed, sealedFieldError, sealedInputValue } from '@/lib/sealed'
import type { ConnectionStatus } from '@/ws/client'
import type { BidRejected, PublicItem } from '@/ws/protocol'
import type { ViewState } from '@/ws/view'
import { PendingLive } from './LiveEnglish'

// Screen D of the player route, the sealed arm (06-UI-SPEC §Player Route &
// Interaction Contract — the sealed LIVE body, D-119/D-120).
//
// Body order: item name + type → close-clock well + own-bid well → the shared
// count line → hint row → start row → budget block → amount field + CTA → one
// feedback sentence. Every rendered fact is a projection field (D-63): the own
// amount is `my_bid`, the count is the server's `sealed_progress`, and a reload
// mid-window paints the whole form from the snapshot alone.
//
// Three structural rules shape this module:
//
//   - ONE local clock, in the BODY (D-119/D-120, 06-UI-SPEC §The close
//     countdown). A sealed run emits NO `price.tick` (D-116), so the countdown
//     interpolates from the newest frame's `(deadline_at, server_ts)` anchor.
//     The displayed clock and the field's enabled predicate read the SAME
//     derived value from the same render — the Dutch inversion — so D-120's
//     "window over → form disabled, from the countdown reaching 0" is one fact
//     instead of two.
//   - ONE TAP, no confirmation (D-120). The button sends `sealed.submit
//     {amount}` immediately; the bid is editable until close, so there is
//     nothing irreversible to confirm.
//   - The field follows the SNAPSHOT until the player edits it (D-119's prefill
//     with `my_bid` in place of `min_next_bid`), and a refused typed value is
//     never clobbered — the player fixes the number rather than retyping it.

/** The body's own cadence: ~4 Hz, and only this body re-renders at it. */
const SEALED_INTERVAL_MS = 250

const SEALED_INPUT_ID = 'sealed-bid-amount'
const SEALED_START_ID = 'sealed-start-price'
const FEEDBACK_ID = 'sealed-feedback'

/**
 * The local clock, advanced by an interval and by nothing else. The lazy
 * initialiser is the one place a clock reading is taken outside the interval.
 * (The private copy is deliberate: one clock per route body, as `LiveDutch`
 * and `ScreenView` keep their own — an imported hook would hide which body owns
 * the interval.)
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

type LiveSealedProps = {
  view: ViewState
  status: ConnectionStatus
  /** D-53's private refusal, or null when there is none outstanding */
  rejection: BidRejected | null
  /** the player acted, so the refusal row goes away */
  onClearRejection(): void
  /** send `sealed.submit {amount}` — the caller owns the socket */
  onSubmit(amount: number): void
}

function LiveSealed({ view, status, rejection, onClearRejection, onSubmit }: LiveSealedProps) {
  const { t } = useLanguage()
  const item = view.item
  const me = view.me
  const live = view.live
  const count = view.sealedProgress

  // The ONE clock on this body. It is read before the defensive gate so the
  // hooks below are unconditional; nothing after the gate re-reads it.
  const now = useNow(SEALED_INTERVAL_MS)

  // A frame carrying the sealed window's timing re-anchors the close clock.
  // Adjusting state during render (rather than in an effect) is what keeps the
  // anchor in step with the frame without a second render pass, and the arrival
  // stamp is the ticking local reading rather than a clock call during render.
  const deadlineAt = live?.deadlineAt ?? null
  const serverTs = live?.serverTs ?? 0
  const frameKey = `${deadlineAt}:${serverTs}`
  const [frame, setFrame] = useState<{ key: string; anchor: TimingAnchor }>(() => ({
    key: frameKey,
    anchor: anchor(deadlineAt ?? 0, serverTs, Date.now()),
  }))
  if (frame.key !== frameKey) {
    setFrame({ key: frameKey, anchor: anchor(deadlineAt ?? 0, serverTs, now) })
  }

  // D-120's in-flight flag, derived from the signal it was sent against: the
  // socket's status, the newest frame's server clock, the server count and the
  // owner's own amount. The submit's own fan-out moves the signal (a fresh
  // `server_ts`, and either `my_bid` or the count — D-110 commits an edit as a
  // full mutation), a refusal releases it, and so does a socket that leaves
  // `open`: the CTA can never remain permanently disabled on `Sending…`, and no
  // effect has to chase any of those.
  const [submitted, setSubmitted] = useState<string | null>(null)

  // The field's value belongs to the item being looked at: carrying the item id
  // with the field state makes "a different item starts clean" a derived fact
  // (D-71/D-119) rather than a reset that has to be scheduled.
  const itemId = item?.id ?? null
  const [field, setField] = useState<{ itemId: string | null; dirty: boolean; value: string }>({
    itemId,
    dirty: false,
    value: '',
  })

  // Defensive gate: `PlayerView` routes a sealed item here, and every admitted
  // sealed LIVE payload carries its deadline (the totality arm) and its own row.
  // This body renders the family's pending sentence rather than a form it could
  // not complete — the fallback `LiveEnglish` exports for the sealed types.
  if (
    item === null ||
    live === null ||
    me === null ||
    live.deadlineAt === null ||
    !isSealedAuction(item.auctionType)
  ) {
    return <PendingLive item={item} live={live} />
  }

  // The SAME derived value feeds the displayed clock and the field predicate,
  // so the two can never disagree (06-UI-SPEC §The close countdown).
  const remaining = remainingMs(frame.anchor, now)
  const closed = remaining <= 0

  const dirty = field.itemId === itemId ? field.dirty : false
  const userValue = field.itemId === itemId ? field.value : ''
  const myBid = me.myBid
  const budget = me.budget
  const startPrice = item.startPrice
  const rawValue = sealedInputValue(dirty, userValue, myBid)

  const flightSignal = `${status}|${live.serverTs}|${count?.submitted ?? 'null'}|${myBid ?? 'null'}`
  const sending = submitted !== null && submitted === flightSignal && rejection === null

  // One call decides both the sentence and the CTA (D-120/D-82b): an
  // enabled-but-refused submit has nowhere to come from. The server remains the
  // authority (D-52).
  const bounds = { raw: rawValue, startPrice, budget }
  const fieldError = sealedFieldError(bounds)
  const enabled = canSubmitSealed({
    ...bounds,
    status,
    remainingMs: remaining,
    inFlight: sending,
  })

  // One slot, never stacked (06-UI-SPEC §Feedback slot): a refusal is the more
  // recent fact and wins it; once the window has closed locally the disabled
  // form's explanation is the closed sentence; otherwise the typed value's own
  // message (if any) is the only thing to fix.
  const refusalKey: BidReasonKey | null = rejection === null ? null : bidReasonKey(rejection.reason)
  const showRefusal = refusalKey !== null
  const showClosed = !showRefusal && closed
  const showFieldError = !showRefusal && !showClosed && fieldError !== null
  const sentenceKey: BidReasonKey | null = showRefusal ? refusalKey : showFieldError ? fieldError : null
  const hasErrorRow = sentenceKey !== null
  const hint = hintRange(item)

  function reasonText(key: BidReasonKey): string {
    // The server's own numbers travel back with a refusal, so the sentence
    // names what it was refused against rather than a stale render's; the start
    // price is the sealed body's `{min}` when no refusal is in force (a sealed
    // run has no ladder minimum to fall back to).
    switch (key) {
      case 'reason.belowMinimum':
        return fill(t(key), { min: formatVND(rejection?.minNextBid ?? startPrice) })
      case 'reason.exceedsBudget':
        return fill(t(key), { budget: formatVND(rejection?.budget ?? budget) })
      default:
        return t(key)
    }
  }

  function edit(next: string) {
    setField({ itemId, dirty: true, value: next })
    onClearRejection()
  }

  function submit() {
    if (!enabled) return
    const amount = parseBidAmount(rawValue)
    if (amount === null) return
    setSubmitted(flightSignal)
    onClearRejection()
    onSubmit(amount)
  }

  return (
    <>
      <div className="flex flex-col gap-1">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em] break-words">
          {item.name}
        </h2>
        <p className="text-sm leading-[1.5] text-muted-foreground">
          {t(item.auctionType === 'vickrey' ? 'auction.vickrey' : 'auction.sealedFirst')}
        </p>
      </div>

      {/* The body's one moving number (mono + tabular, so the digits cannot
          change width between ticks) beside the player's own effective amount. */}
      <div className="grid grid-cols-2 gap-4">
        <div className="border-2 border-border bg-background px-4 py-2">
          <p className="text-sm leading-[1.5] font-bold">{t('live.timeLabel')}</p>
          <p
            role="timer"
            className="font-mono text-xl leading-[1.2] font-bold tabular-nums whitespace-nowrap"
          >
            {formatClock(remaining)}
          </p>
          <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
        </div>

        <div className="border-2 border-border bg-background px-4 py-2">
          <p className="text-sm leading-[1.5] font-bold">{t('sealed.submittedLabel')}</p>
          {/* The value slot is never blank: the amount while there is one, the
              not-submitted sentence otherwise. */}
          {myBid === null ? (
            <p className="text-base leading-[1.5]">{t('sealed.notSubmitted')}</p>
          ) : (
            <>
              <p className="text-xl leading-[1.2] font-bold whitespace-nowrap">
                {formatVND(myBid)}
              </p>
              <p className="text-sm leading-[1.5] text-muted-foreground">{t('sealed.editable')}</p>
            </>
          )}
        </div>
      </div>

      {/* The ONE shared sealed fact: the same sentence the projector shows. A
          projection that carries no counter renders no line rather than a
          fabricated count (the browser counts nothing — D-116). */}
      {count !== null && (
        <p className="text-base leading-[1.5]">
          {fill(t('screen.sealedProgress'), {
            submitted: String(count.submitted),
            total: String(count.total),
          })}
        </p>
      )}

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

      {/* The inclusive lower bound, always one glance above the field — and the
          field's own description target beside its feedback row. */}
      <div id={SEALED_START_ID} className="flex flex-wrap items-baseline gap-2">
        <p className="text-sm leading-[1.5] font-bold">{t('intro.startLabel')}</p>
        <p className="text-base leading-[1.5]">{formatVND(startPrice)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
      </div>

      {/* The shipped Phase-2 budget block, reused verbatim — and rendered
          PERMANENTLY on this body, because the field's ceiling is money and a
          ceiling inferred from elsewhere is the D-82b mistake in player form. */}
      <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
        <p className="text-xl leading-[1.2] font-bold">{formatVND(budget)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('lobby.budgetCaption')}</p>
      </div>

      {/* One tap, no confirmation (D-120): the accent marks the enabled submit,
          and a disabled or in-flight control renders as an outline. */}
      <div className="flex flex-col gap-2">
        <Label htmlFor={SEALED_INPUT_ID}>{t('sealed.bidLabel')}</Label>
        <Input
          id={SEALED_INPUT_ID}
          className="bg-input"
          // A raw digit string, not a number field: the server reads a plain
          // integer and FRMT-01 (Phase 8) owns formatted inputs (D-119).
          type="text"
          inputMode="numeric"
          autoComplete="off"
          value={rawValue}
          disabled={sending || closed}
          aria-invalid={hasErrorRow}
          aria-describedby={hasErrorRow ? `${SEALED_START_ID} ${FEEDBACK_ID}` : SEALED_START_ID}
          onChange={(event) => edit(event.target.value)}
          onKeyDown={(event) => {
            // Enter submits — there is no form element and no page reload.
            if (event.key === 'Enter') {
              event.preventDefault()
              submit()
            }
          }}
        />

        <Button
          type="button"
          variant={enabled ? 'default' : 'outline'}
          disabled={!enabled}
          className="w-full"
          onClick={submit}
        >
          {sending ? t('sealed.sending') : myBid !== null ? t('sealed.update') : t('sealed.submit')}
        </Button>
      </div>

      {hasErrorRow && sentenceKey !== null && (
        // The Phase-2 error row, reused: the destructive cue is the border and
        // the copy stays near-black. `code`/`reason` select the sentence — the
        // server's `message` is never rendered (T-06-12).
        <p
          id={FEEDBACK_ID}
          role="alert"
          className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]"
        >
          {reasonText(sentenceKey)}
        </p>
      )}

      {showClosed && (
        // The disabled form's explanation: a plain sentence, no box and no
        // border (06-UI-SPEC §Feedback slot) — the reveal follows as the
        // carried body switch once the settle's snapshot lands.
        <p className="text-base leading-[1.5]">{t('sealed.closed')}</p>
      )}
    </>
  )
}

export default LiveSealed
