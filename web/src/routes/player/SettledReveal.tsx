import type { ReactNode } from 'react'
import { formatVND } from '@/lib/format'
import { useLanguage } from '@/lib/i18n'
import type { PlayerMe, PublicItem } from '@/ws/protocol'

// Screen E of the player route (03-UI-SPEC §Screen E — SETTLED).
//
// Body order: outcome heading → outcome rows → (winner-only) budget change →
// budget block → unsold explanation → waiting line. This is the ONE surface
// where `truth_value` ever reaches a player, and the server only sends it
// inside the `settle` object at SETTLED (D-67/D-68) — the decoder refuses a
// payload that carries one earlier, so the leak boundary is the object's
// presence rather than this component's restraint.

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
}

function SettledReveal({ item, me }: SettledRevealProps) {
  const { t } = useLanguage()
  const settle = item?.settle ?? null

  // Unreachable by construction: `readPlayerSnapshot` refuses a SETTLED payload
  // without its `settle` object and without the current item, and an unapplied
  // snapshot leaves the previous body on screen (UI-SPEC §Phase-body totality).
  if (item === null || settle === null || me === null) return null

  const sold = settle.outcome === 'sold'
  const iWon = sold && settle.winner !== null && settle.winner === me.nickname

  return (
    <>
      <div className="flex flex-col gap-1">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
          {sold ? t('settle.soldHeading') : t('settle.unsoldHeading')}
        </h2>
        <p className="min-w-0 break-words text-base leading-[1.5]">{item.name}</p>
      </div>

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
          <span>{formatVND(settle.price)}</span>
          <span className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</span>
        </RevealRow>

        <RevealRow label={t('settle.truthLabel')}>
          <span>{formatVND(settle.truthValue)}</span>
          <span className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</span>
        </RevealRow>
      </div>

      {iWon && <p className="text-base leading-[1.5]">{t('settle.youWon')}</p>}

      {/* Only the winner's budget moved (ECON-02), and a "0" row would be noise
          for everybody else: the delta row renders only for a non-zero change.
          `formatVND` already carries the minus sign of a negative delta, so the
          plus is added only when the value is positive. */}
      {settle.budgetDelta !== 0 && (
        <RevealRow label={t('settle.deltaLabel')}>
          <span>
            {settle.budgetDelta > 0 ? '+' : ''}
            {formatVND(settle.budgetDelta)}
          </span>
          <span className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</span>
        </RevealRow>
      )}

      {/* The shipped Phase 2 budget block, showing the UPDATED budget: the
          settle's own snapshot arrives before this body renders. */}
      <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
        <p className="text-xl leading-[1.2] font-bold">{formatVND(me.budget)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('lobby.budgetCaption')}</p>
      </div>

      {/* ECON-03: the unsold branch states the outcome and that no budget moved,
          rather than leaving the reveal to be inferred from a missing winner. */}
      {!sold && <p className="text-base leading-[1.5]">{t('settle.unsoldBody')}</p>}

      <p className="text-base leading-[1.5]">{t('settle.waiting')}</p>
    </>
  )
}

export default SettledReveal
