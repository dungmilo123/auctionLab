import type { ReactNode } from 'react'
import ItemImage from '@/components/ItemImage'
import { bidReasonKey } from '@/lib/bid'
import { formatUSD } from '@/lib/format'
import { useLanguage } from '@/lib/i18n'
import type { BidRejected, PlayerMe, PublicItem } from '@/ws/protocol'

// Screen E of the player route (03-UI-SPEC §Screen E — SETTLED).
//
// Body order: outcome heading → outcome rows → (winner-only) budget change →
// budget block → unsold explanation → waiting line. This is the ONE surface
// where `truth_value` ever reaches a player, and the server only sends it
// inside the `settle` object at SETTLED (D-67/D-68) — the decoder refuses a
// payload that carries one earlier, so the leak boundary is the object's
// presence rather than this component's restraint.
//
// Phase 5 adds ONE row and changes nothing else (D-97/05-UI-SPEC §The Dutch
// race loser's notice): the Dutch accept race loser's `already_sold` sentence,
// rendered here because the refusal necessarily arrives after the settle's own
// fan-out — the live body is already gone, so without this row the loser would
// read only the public reveal and could mistake a lost race for a phantom win.

/** One label/value row: label 14/700, value 16/400, both baseline-aligned. */
function RevealRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-2">
      <p className="text-sm leading-[1.5] font-bold">{label}</p>
      <p className="flex flex-wrap items-baseline gap-2 text-base leading-[1.5]">{children}</p>
    </div>
  )
}

type SettledRevealProps = {
  item: PublicItem | null
  me: PlayerMe | null
  /**
   * D-97's race-loss refusal for THIS item, or any other private refusal. Only
   * `already_sold` renders a notice: the reveal is not a second feedback slot
   * for the ladder's reasons, and the caller has already scoped it to the item
   * on screen (`PlayerView`'s `held`), so a stale refusal cannot appear here.
   */
  rejection: BidRejected | null
}

function SettledReveal({ item, me, rejection }: SettledRevealProps) {
  const { t } = useLanguage()
  const settle = item?.settle ?? null

  // Unreachable by construction: `readPlayerSnapshot` refuses a SETTLED payload
  // without its `settle` object and without the current item, and an unapplied
  // snapshot leaves the previous body on screen (UI-SPEC §Phase-body totality).
  if (item === null || settle === null || me === null) return null

  const sold = settle.outcome === 'sold'
  const iWon = sold && settle.winner !== null && settle.winner === me.nickname
  // The reason string selects the copy through the same map the live bodies use,
  // so the loser's sentence is the ONE `reason.alreadySold` row rather than a
  // second wording of it (D-97/D-106).
  const noticeKey = rejection === null ? null : bidReasonKey(rejection.reason)
  const alreadySold = noticeKey === 'reason.alreadySold'

  return (
    <>
      <div className="flex items-start gap-4">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em] break-words">
            {sold ? t('settle.soldHeading') : t('settle.unsoldHeading')}
          </h2>
          <p className="min-w-0 break-words text-base leading-[1.5]">{item.name}</p>
        </div>

        {/* D-01: the picture of the lot the reveal is about, beside the outcome
            heading. The reveal rows below are the ceremony's facts and keep
            their shipped positions; an unconfigured item renders no element. */}
        <ItemImage
          src={item.imageUrl}
          alt={item.name}
          className="size-20 shrink-0 border-2 border-border bg-card object-contain"
        />
      </div>

      {/* D-97: the first thing the loser reads — directly under the outcome and
          the item name, before the reveal rows. The carried rejection-row shape
          (destructive BORDER, near-black copy), because destructive-as-text
          measures below the 4.5:1 threshold on this surface. */}
      {alreadySold && (
        <p role="alert" className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]">
          {t('reason.alreadySold')}
        </p>
      )}
      <div className="flex flex-col gap-2">
        {/* Omitted entirely when there is no winner — which is exactly the
            unsold case (ECON-03): a blank winner slot would read as a missing
            value rather than as "nobody won". */}
        {settle.winner !== null && (
          <RevealRow label={t('settle.winnerLabel')}>
            <span className="min-w-0 break-words">{settle.winner}</span>
            {iWon && (
              <span className="text-sm leading-[1.5] font-bold text-muted-foreground">
                {t('lobby.you')}
              </span>
            )}
          </RevealRow>
        )}

        <RevealRow label={t('settle.priceLabel')}>
          <span>{formatUSD(settle.price)}</span>
        </RevealRow>

        <RevealRow label={t('settle.truthLabel')}>
          <span>{formatUSD(settle.truthValue)}</span>
        </RevealRow>
      </div>

      {/* D-122: the ONE type-conditional line on this body. A Vickrey winner
          pays the second-highest bid (or the start price when alone), and the
          wire carries no amount at SETTLED (D-114) — so the line is a RULE
          statement that reads identically for the winner and every loser. A
          plain sentence: no box, no fill, no heading, no `role="alert"`. */}
      {sold && item.auctionType === 'vickrey' && (
        <p className="text-base leading-[1.5]">{t('settle.vickreyNote')}</p>
      )}

      {iWon && <p className="text-base leading-[1.5]">{t('settle.youWon')}</p>}

      {/* ECON-02: only the winner's budget moved. `budget_delta` is an
          ITEM-scoped number — ONE value broadcast to every role (D-68) — so
          `!== 0` alone would show a non-winner a change to money that never
          moved, while `me.budget` below renders unchanged. The winner gate is
          therefore the load-bearing half; a "0" delta stays hidden as noise
          even for the winner, and `formatUSD` already carries the minus sign of
          a negative delta, so the plus is added only when the value is positive. */}
      {iWon && settle.budgetDelta !== 0 && (
        <RevealRow label={t('settle.deltaLabel')}>
          <span>
            {settle.budgetDelta > 0 ? '+' : ''}
            {formatUSD(settle.budgetDelta)}
          </span>
        </RevealRow>
      )}

      {/* The shipped Phase 2 budget block, showing the UPDATED budget: the
          settle's own snapshot arrives before this body renders. */}
      <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
        <p className="text-xl leading-[1.2] font-bold">{formatUSD(me.budget)}</p>
      </div>

      {/* ECON-03: the unsold branch states the outcome and that no budget moved,
          rather than leaving the reveal to be inferred from a missing winner. */}
      {!sold && <p className="text-base leading-[1.5]">{t('settle.unsoldBody')}</p>}

      <p className="text-base leading-[1.5]">{t('settle.waiting')}</p>
    </>
  )
}

export default SettledReveal
