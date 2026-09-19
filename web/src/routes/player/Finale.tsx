import { formatUSD } from '@/lib/format'
import { fill, useLanguage } from '@/lib/i18n'
import { playerRank, podiumEntries } from '@/lib/screen'
import type { ViewState } from '@/ws/view'

// Screen F of the player route (04-UI-SPEC §Player Finale Body, D-89/ECON-05):
// the Phase 3 placeholder's replacement.
//
// Body order: the finale heading → the top three rows → your own rank block →
// the carried closing line. The public half is the same standings the projector
// renders; the personal half is the SERVER's `me.rank` (0 means "no standing"),
// never a nickname matched against a public row that carries no id (research
// Pitfall 1 — `is_top_bidder` is server-computed for the same reason, D-72).
//
// The rank block is OMITTED entirely when the server sent no standing: an
// unranked player must not be shown a blank scaffold, and "rank 0" is not a
// place. Nothing here shows another player's budget beyond the top three, which
// the projector is already showing to the room.

function Finale({ view }: { view: ViewState }) {
  const { t } = useLanguage()
  const me = view.me
  const podium = podiumEntries(view.leaderboard)
  const rank = playerRank(me, view.leaderboard)

  return (
    <>
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('finale.heading')}</h2>

      <div className="flex flex-col gap-2">
        <p className="text-sm leading-[1.5] font-bold text-muted-foreground">
          {t('finale.podiumHeading')}
        </p>
        {/* The carried lobby-row shape. Fewer than three players renders fewer
            rows — never an empty frame, and never a "3rd place: nobody" line. */}
        <ul className="bg-secondary">
          {podium.map((entry) => (
            <li
              key={entry.nickname}
              className="flex min-h-11 flex-wrap items-center gap-2 border-t-2 border-t-border border-l-2 border-l-transparent px-2 py-1 first:border-t-0"
            >
              <p className="text-sm leading-[1.5] font-bold">{entry.rank}</p>
              <p className="min-w-40 flex-1 break-words text-base leading-[1.5]">
                {entry.nickname}
              </p>
              <p className="whitespace-nowrap text-base leading-[1.5]">
                {formatUSD(entry.budget)}
              </p>
              <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
            </li>
          ))}
        </ul>
      </div>

      {rank !== null && me !== null && (
        <div className="flex flex-col gap-2">
          {/* The own-row marker, exactly as the lobby renders it: a 2px accent
              left stripe, never a colour that carries the meaning alone. */}
          <div className="flex min-h-11 items-center gap-2 border-t-2 border-t-border border-l-2 border-l-primary bg-secondary px-2 py-1">
            <p className="min-w-0 flex-1 break-words text-sm leading-[1.5] font-bold">
              {fill(t('finale.yourRank'), { rank: String(rank.rank), total: String(rank.total) })}
            </p>
          </div>

          {/* The carried budget block, twice: what you can still spend, and what
              the items you won are worth (ECON-05's two numbers). */}
          <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
            <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
            <p className="text-xl leading-[1.2] font-bold">{formatUSD(me.budget)}</p>
            <p className="text-sm leading-[1.5] text-muted-foreground">
              {t('lobby.budgetCaption')}
            </p>
          </div>

          <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
            <p className="text-sm leading-[1.5] font-bold">{t('admin.playerValue')}</p>
            <p className="text-xl leading-[1.2] font-bold">{formatUSD(me.totalValue)}</p>
            <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
          </div>
        </div>
      )}

      <p className="text-base leading-[1.5]">{t('finale.body')}</p>
    </>
  )
}

export default Finale
