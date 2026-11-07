import { Clock } from 'lucide-react'
import type { ParticipationStatus } from '@/api/auction'
import { useAuctionCopy } from '@/lib/auction-copy'
import { countdownLabel } from '@/lib/countdown'
import { standingIsLive, standingStyle } from '@/lib/standing'

/** Coloured banner for the bidder's standing; `compact` drops the help sentence for list rows. */
export function Standing({ status, closesAt, now, compact = false }: { status: ParticipationStatus; closesAt: number | null; now: number; compact?: boolean }) {
  const { b } = useAuctionCopy()
  const style = standingStyle[status]
  if (!style) return <p>{b(status)}</p>
  const live = standingIsLive(status) && closesAt !== null && closesAt > now
  const help = compact ? null : status === 'outbid' ? b('outbidHelp') : status === 'won' ? b('wonHelp') : null
  return <div data-standing={status} className={`flex flex-col gap-1.5 border-l-4 ${compact ? 'px-3 py-2.5' : 'px-4 py-3.5'} ${style.box}`}>
    <div className={`flex flex-wrap items-center gap-x-5 gap-y-1.5 ${style.text}`}>
      <p className={`flex items-center gap-2.5 font-semibold ${compact ? 'text-base' : 'text-lg'}`}><style.Icon aria-hidden="true" className="size-5 shrink-0" />{b(status)}</p>
      {live && <p className="flex items-center gap-2 font-mono text-sm numeric"><Clock aria-hidden="true" className="size-4 shrink-0" /><span className="text-[0.6875rem] font-medium tracking-[0.12em] uppercase">{b('remaining')}</span><span className="text-base font-semibold">{countdownLabel(closesAt, now)}</span></p>}
    </div>
    {help && <p className="text-sm text-ink">{help}</p>}
  </div>
}
