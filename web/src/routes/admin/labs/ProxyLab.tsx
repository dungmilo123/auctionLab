import { useState } from 'react'
import { Bot, RotateCcw, UserRound } from 'lucide-react'
import { cn } from 'cn'
import { Icon } from '@/components/site/Icon'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatUsd, minimumNext, placeProxy, type ProxyState } from '@/lib/labs'
import { useLabsCopy } from '@/lib/labs-copy'
import { Field, LabHeader, LotStage, Rules } from './shared'
import { SEED_LOTS } from '@/lib/labs'

const START: ProxyState = { price: null, start: 20_000, increment: 1_000, leader: null, history: [] }
const toCents = (v: string) => {
  const n = Number(v.replace(/[,\s$]/g, ''))
  return Number.isFinite(n) && n > 0 ? Math.round(n * 100) : null
}

export default function ProxyLab() {
  const { l } = useLabsCopy()
  const [state, setState] = useState(START)
  const [myMax, setMyMax] = useState('')
  const [rivalMax, setRivalMax] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [clock, setClock] = useState(1)
  const [myCeiling, setMyCeiling] = useState<number | null>(null)

  const place = (bidder: 'you' | 'rival', raw: string) => {
    const max = toCents(raw)
    if (max === null) return setError(l('invalidAmount'))
    const next = placeProxy(state, { bidder, max, placedAt: clock })
    setClock((c) => c + 1)
    if ('error' in next) return setError(l('tooLow'))
    setError(null)
    setState(next)
    if (bidder === 'you') { setMyCeiling(max); setMyMax('') } else setRivalMax('')
  }
  const leading = state.leader?.bidder === 'you'
  const involved = myCeiling !== null
  const nextMin = minimumNext(state)
  const who = (b: string) => (b === 'you' ? l('you') : l('rival'))

  return (
    <section className="flex flex-col gap-8">
      <LabHeader title={l('pxTitle')} intro={l('pxIntro')} />

      <LotStage
        lot={SEED_LOTS.fieldWatch}
        badge={
          <Button type="button" size="sm" variant="ghost" onClick={() => { setState(START); setMyCeiling(null); setError(null) }}>
            <Icon icon={RotateCcw} size={14} />{l('reset')}
          </Button>
        }
      >
            <div className="flex flex-wrap items-end justify-between gap-4">
              <div className="flex flex-col">
                <span className="mono-label">{state.price === null ? `${l('noBids')} · ${l('minimum')}` : l('currentBid')}</span>
                <span className="numeric font-heading text-6xl font-medium tracking-[-0.04em]">
                  {state.price === null ? <span className="text-muted-foreground">{formatUsd(state.start)}</span> : formatUsd(state.price)}
                </span>
                <span className="numeric text-sm text-muted-foreground">
                  {l('nextMin')}: {formatUsd(nextMin)} · {l('increment')}: {formatUsd(state.increment)}
                </span>
              </div>
              {involved && (
                <div
                  role="status"
                  className={cn(
                    'flex flex-col gap-0.5 rounded-[2px] border px-3 py-2 text-sm',
                    leading
                      ? 'border-(--standing-leading) bg-(--standing-leading-bg) text-(--standing-leading)'
                      : 'border-(--standing-outbid) bg-(--standing-outbid-bg) text-(--standing-outbid)',
                  )}
                >
                  <span className="font-mono text-xs font-medium tracking-[0.1em] uppercase">{leading ? l('leading') : l('outbid')}</span>
                  {leading && state.leader && (
                    <span className="numeric">{l('autoUpTo')} {formatUsd(state.leader.max)}</span>
                  )}
                </div>
              )}
            </div>

            <div className="grid gap-4 border-t border-hairline pt-6 sm:grid-cols-2">
              <form className="flex flex-col gap-2" onSubmit={(e) => { e.preventDefault(); place('you', myMax) }}>
                <Field label={l('yourMax')}>
                  <Input inputMode="decimal" value={myMax} onChange={(e) => setMyMax(e.target.value)} placeholder={(Math.max(nextMin, 35_000) / 100).toFixed(2)} />
                </Field>
                <Button type="submit" size="lg"><Icon icon={UserRound} size={16} />{l('setMax')}</Button>
              </form>
              <form className="flex flex-col gap-2" onSubmit={(e) => { e.preventDefault(); place('rival', rivalMax) }}>
                <Field label={l('rivalMax')}>
                  <Input inputMode="decimal" value={rivalMax} onChange={(e) => setRivalMax(e.target.value)} placeholder={(Math.max(nextMin, 28_000) / 100).toFixed(2)} />
                </Field>
                <Button type="submit" size="lg" variant="outline"><Icon icon={Bot} size={16} />{l('rivalBids')}</Button>
              </form>
            </div>
            <p role="alert" className="min-h-5 text-sm text-destructive">{error}</p>
      </LotStage>

      <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader><CardTitle>{l('bidHistory')}</CardTitle></CardHeader>
            <CardContent>
              {state.history.length === 0 ? (
                <p className="text-sm text-muted-foreground">{l('noBids')}</p>
              ) : (
                <ol className="numeric m-0 flex list-none flex-col gap-2 p-0 text-sm">
                  {[...state.history].reverse().map((h, n) => (
                    <li key={state.history.length - n} className={cn('flex items-center justify-between gap-3 border-b border-hairline pb-2', n === 0 && 'font-medium')}>
                      <span>{who(h.bidder)}</span>
                      <span className="flex items-center gap-2">
                        {h.auto && <span className="rounded-[2px] border border-hairline px-1.5 font-mono text-[0.625rem] tracking-[0.1em] text-muted-foreground uppercase">{l('auto')}</span>}
                        {formatUsd(h.amount)}
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </CardContent>
          </Card>
          <Rules items={[l('pxRule1'), l('pxRule2'), l('pxRule3'), l('pxRule4')]} />
      </div>
    </section>
  )
}
