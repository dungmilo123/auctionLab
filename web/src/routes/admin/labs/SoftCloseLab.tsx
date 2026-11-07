import { useEffect, useState } from 'react'
import { Gavel, RotateCcw, TimerReset } from 'lucide-react'
import { cn } from 'cn'
import { Icon } from '@/components/site/Icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { applySoftClose, formatUsd, type SoftClose } from '@/lib/labs'
import { useLabsCopy } from '@/lib/labs-copy'
import { Choice, Field, LabHeader, LotStage, Rules } from './shared'
import { SEED_LOTS } from '@/lib/labs'

const WINDOWS = ['60000', '120000', '300000'] as const
const CAPS = ['300000', '600000', '1800000'] as const
const mmss = (ms: number) => {
  const s = Math.max(0, Math.ceil(ms / 1000))
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}
const minutes = (ms: string) => `${Number(ms) / 60000} min`

function fresh(windowMs: number, capMs: number): { lot: SoftClose; price: number; bids: number } {
  const close = Date.now() + 150_000
  return { lot: { closeAt: close, originalCloseAt: close, windowMs, capMs, extensions: [] }, price: 42_000, bids: 7 }
}

export default function SoftCloseLab() {
  const { l, lang } = useLabsCopy()
  const [windowMs, setWindowMs] = useState<(typeof WINDOWS)[number]>('120000')
  const [capMs, setCapMs] = useState<(typeof CAPS)[number]>('600000')
  const [state, setState] = useState(() => fresh(120_000, 600_000))
  const [offset, setOffset] = useState(0) // simulated clock skip
  const [now, setNow] = useState(() => Date.now())
  const [flash, setFlash] = useState(0)

  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 250)
    return () => window.clearInterval(t)
  }, [])
  const sim = now + offset
  const { lot } = state
  const remaining = lot.closeAt - sim
  const closed = remaining <= 0
  const inWindow = !closed && remaining < lot.windowMs
  const capTotal = lot.capMs
  const capUsed = Math.min(capTotal, lot.closeAt - lot.originalCloseAt)

  const reset = (w = Number(windowMs), c = Number(capMs)) => {
    setOffset(0)
    setState(fresh(w, c))
  }
  const bid = () => {
    if (closed) return
    const next = applySoftClose(lot, sim)
    if (next !== lot) setFlash((n) => n + 1)
    setState((s) => ({ lot: next, price: s.price + 1_000, bids: s.bids + 1 }))
  }
  const time = (t: number) => new Date(t).toLocaleTimeString(lang === 'vi' ? 'vi-VN' : 'en-GB')

  return (
    <section className="flex flex-col gap-8">
      <LabHeader title={l('scTitle')} intro={l('scIntro')} />

      <div className="flex flex-wrap items-end gap-6 border-y border-hairline py-4">
        <Field label={l('window')}>
          <Choice label={l('window')} value={windowMs} onChange={(v) => { setWindowMs(v); reset(Number(v), Number(capMs)) }}
            options={WINDOWS.map((w) => ({ value: w, label: minutes(w) }))} />
        </Field>
        <Field label={l('cap')}>
          <Choice label={l('cap')} value={capMs} onChange={(v) => { setCapMs(v); reset(Number(windowMs), Number(v)) }}
            options={CAPS.map((c) => ({ value: c, label: `+${minutes(c)}` }))} />
        </Field>
        <Button type="button" variant="ghost" onClick={() => reset()}><Icon icon={RotateCcw} size={16} />{l('reset')}</Button>
      </div>

      <LotStage
        lot={SEED_LOTS.turntable}
        badge={closed ? <Badge tone="sold">{l('closed')}</Badge>
          : inWindow ? <Badge tone="closing">{l('window')}</Badge>
            : <Badge tone="live">Live</Badge>}
      >
        <div className="grid gap-6 sm:grid-cols-[auto_1fr] sm:items-end">
          <div className="flex flex-col">
            <span className="mono-label">{closed ? l('closed') : l('closesIn')}</span>
            <span
              key={flash}
              aria-live="off"
              className={cn(
                'numeric font-heading text-7xl leading-none font-medium tracking-[-0.05em]',
                inWindow && 'text-[#8A6A12]',
                flash > 0 && 'animate-[confirmed-feedback_900ms_ease-out]',
              )}
            >
              {mmss(remaining)}
            </span>
          </div>
          <div className="flex flex-col sm:items-end">
            <span className="mono-label">{l('currentBid')}</span>
            <span className="numeric font-heading text-3xl font-medium tracking-[-0.03em]">{formatUsd(state.price)}</span>
            <span className="text-sm text-muted-foreground">{state.bids} {l('bids').toLowerCase()}</span>
          </div>
        </div>
        <p role="status" className={cn('min-h-12 rounded-[2px] p-3 text-sm', inWindow ? 'bg-(--standing-won-bg) text-(--standing-won)' : 'bg-muted text-muted-foreground')}>
          {lot.extensions.length > 0 && !closed
            ? `${l('extended')} ×${lot.extensions.length} · +${mmss(lot.closeAt - lot.originalCloseAt)}${inWindow ? ` — ${l('inWindow')}` : ''}`
            : inWindow ? l('inWindow') : `${l('window')}: ${minutes(windowMs)} · ${l('cap')}: +${minutes(capMs)}`}
        </p>
        <div className="flex flex-col gap-2">
          <div className="flex justify-between text-sm text-muted-foreground">
            <span>{l('capUsed')}</span>
            <span className="numeric">+{mmss(capUsed)} / +{mmss(capTotal)}</span>
          </div>
          <Progress value={(capUsed / capTotal) * 100} aria-label={l('capUsed')} className="h-2 rounded-[2px] [&>[data-slot=progress-indicator]]:bg-ink" />
        </div>
        <div className="mt-auto flex flex-wrap gap-3 border-t border-hairline pt-6">
          <Button type="button" size="lg" onClick={bid} disabled={closed}><Icon icon={Gavel} size={16} />{l('simulateBid')}</Button>
          <Button type="button" size="lg" variant="outline" disabled={closed || remaining <= 30_000}
            onClick={() => setOffset((o) => o + (remaining - 30_000))}>
            <Icon icon={TimerReset} size={16} />{l('skipAhead')}
          </Button>
        </div>
      </LotStage>

      <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader><CardTitle>{l('extensionLog')}</CardTitle></CardHeader>
            <CardContent>
              {lot.extensions.length === 0 ? (
                <p className="text-sm text-muted-foreground">{l('noExtensions')}</p>
              ) : (
                <ol className="numeric m-0 flex list-none flex-col gap-2 p-0 text-sm">
                  {[...lot.extensions].reverse().map((e) => (
                    <li key={e.at} className="flex justify-between gap-3 border-b border-hairline pb-2">
                      <span>{time(e.at)}</span>
                      <span className="text-muted-foreground">{mmss(e.from - e.at)} → {mmss(e.to - e.at)}</span>
                      <span>+{mmss(e.to - e.from)}</span>
                    </li>
                  ))}
                </ol>
              )}
            </CardContent>
          </Card>
          <Rules items={[l('scRule1'), l('scRule2'), l('scRule3'), l('scRule4')]} />
      </div>
    </section>
  )
}
