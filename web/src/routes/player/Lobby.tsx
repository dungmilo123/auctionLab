import { useLanguage } from '@/lib/i18n'
import { formatVND } from '@/lib/format'
import type { PlayerSnapshot } from '@/ws/protocol'

// Screen B of the player route (02-UI-SPEC.md §Screen B — Lobby).
//
// Everything rendered here comes from the self-sufficient snapshot (D-25), so a
// dropped connection or a superseded session leaves the last known lobby on
// screen instead of blanking it. The list has no inner scroll on purpose: at 20
// rows the page itself scrolls, which is the correct phone behaviour.

type LobbyProps = {
  snapshot: PlayerSnapshot
}

function Lobby({ snapshot }: LobbyProps) {
  const { t, count } = useLanguage()
  const players = snapshot.lobby

  return (
    <>
      <div className="flex items-baseline justify-between gap-2">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('lobby.heading')}</h2>
        <p className="text-sm leading-[1.5] font-bold">{count(players.length)}</p>
      </div>

      <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
        <p className="text-xl leading-[1.2] font-bold">{formatVND(snapshot.me.budget)}</p>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('lobby.budgetCaption')}</p>
      </div>

      <ul className="bg-secondary">
        {players.map((entry) => {
          // The lobby row carries no id — but nicknames are unique by
          // construction (D-28's lowercase key is a UNIQUE index), so the row
          // whose nickname equals my own IS my row.
          const mine = entry.nickname === snapshot.me.nickname
          return (
            <li
              key={entry.nickname}
              className={`flex min-h-11 items-center gap-2 border-t-2 border-t-border border-l-2 px-2 first:border-t-0 ${
                mine ? 'border-l-primary' : 'border-l-transparent'
              }`}
            >
              <span
                aria-hidden="true"
                className={`size-3 shrink-0 border-2 border-border ${
                  entry.connected ? 'bg-foreground' : 'bg-transparent'
                }`}
              />
              {/* Presence is never carried by the square alone: the fill is
                  hidden from assistive technology and this text is the state. */}
              <span className="sr-only">
                {entry.connected ? t('presence.connected') : t('presence.disconnected')}
              </span>
              <span className="min-w-0 break-words text-base leading-[1.5]">{entry.nickname}</span>
              {mine && (
                <span className="text-sm leading-[1.5] font-bold text-muted-foreground">
                  {t('lobby.you')}
                </span>
              )}
            </li>
          )
        })}
      </ul>

      {/* The list is never empty — I am always in it — so the empty-ish state is
          "waiting for someone else", which names the next step rather than
          saying there is no data. */}
      {players.length === 1 && <p className="text-base leading-[1.5]">{t('lobby.waiting')}</p>}
    </>
  )
}

export default Lobby
