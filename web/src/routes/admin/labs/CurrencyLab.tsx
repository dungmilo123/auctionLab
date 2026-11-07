import { useState } from 'react'
import { ArrowRightLeft, Receipt } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  centsToDong, dongBidToCents, formatDongAmount, formatUsd,
  type DisplayCurrency, type FxRate,
} from '@/lib/labs'
import { useLabsCopy } from '@/lib/labs-copy'
import { Choice, Field, LabHeader, LotStage, Rules } from './shared'
import { SEED_LOTS, lotArt } from '@/lib/labs'

const DAY = 86_400_000
const NOW = Date.UTC(2026, 10, 7, 9)
const SEED_RATES: FxRate[] = [
  { id: 1, dongPerUsd: 25_120, source: 'seed', effectiveAt: NOW - 20 * DAY },
  { id: 2, dongPerUsd: 25_380, source: 'operator', effectiveAt: NOW - 1 * DAY },
]
// Fixture lot and a past order whose rate was frozen at creation.
const LOT = { ...SEED_LOTS.rangefinder, priceCents: 17_000, incrementCents: 500, bids: 6 }
const ORDER = { lot: SEED_LOTS.pocketWatch, hammer: 15_000, delivery: 1_500, rateId: 1 }

type Mode = DisplayCurrency | 'AUTO'

export default function CurrencyLab() {
  const { l, lang } = useLabsCopy()
  const [mode, setMode] = useState<Mode>('AUTO')
  const [rates, setRates] = useState(SEED_RATES)
  const [draftRate, setDraftRate] = useState('25380')
  const [bid, setBid] = useState('')
  const rate = rates[rates.length - 1]!
  const currency: DisplayCurrency = mode === 'AUTO' ? (lang === 'vi' ? 'VND' : 'USD') : mode
  const date = (t: number) => new Date(t).toLocaleDateString(lang === 'vi' ? 'vi-VN' : 'en-GB', { day: '2-digit', month: 'short', year: 'numeric' })
  const dong = (cents: number, r = rate, round = 1000) => formatDongAmount(centsToDong(cents, r, round))
  const minimum = LOT.priceCents + LOT.incrementCents

  // Bid entry is in the display currency; the wire is always USD cents.
  const typed = Number(bid.replace(/[.,\s]/g, ''))
  const submitCents = bid === '' ? null
    : currency === 'VND' ? dongBidToCents(typed, rate, minimum, LOT.incrementCents)
      : Number.isFinite(Number(bid)) && Number(bid) > 0
        ? Math.max(minimum, minimum + Math.ceil((Math.round(Number(bid) * 100) - minimum) / LOT.incrementCents) * LOT.incrementCents)
        : null

  const publish = () => {
    const n = Number(draftRate)
    if (!Number.isSafeInteger(n) || n < 1000 || n > 1_000_000) return
    setRates((r) => [...r, { id: r.length + 1, dongPerUsd: n, source: 'operator', effectiveAt: Date.now() }])
  }
  const orderRate = rates.find((r) => r.id === ORDER.rateId)!
  const total = ORDER.hammer + ORDER.delivery

  return (
    <section className="flex flex-col gap-8">
      <LabHeader title={l('fxTitle')} intro={l('fxIntro')} />

      <div className="flex flex-wrap items-end justify-between gap-4 border-y border-hairline py-4">
        <Field label={l('displayCurrency')}>
          <Choice<Mode>
            label={l('displayCurrency')}
            value={mode}
            onChange={setMode}
            options={[
              { value: 'AUTO', label: `${l('followLanguage')} (${lang === 'vi' ? 'VND' : 'USD'})` },
              { value: 'USD', label: '$ USD' },
              { value: 'VND', label: '₫ VND' },
            ]}
          />
        </Field>
        <p className="numeric text-sm text-muted-foreground">
          1 USD = <span className="text-foreground">{formatDongAmount(BigInt(rate.dongPerUsd))}</span> · {rate.source} · {date(rate.effectiveAt)}
        </p>
      </div>

      <LotStage lot={LOT} badge={<Badge tone="live">Live</Badge>}>
        {/* Price block: display currency first, settlement currency beneath. */}
        <div className="flex flex-col gap-1">
          <span className="mono-label">{l('currentBid')} · {LOT.bids} {l('bids').toLowerCase()}</span>
          {currency === 'USD' ? (
            <>
              <span className="numeric font-heading text-5xl font-medium tracking-[-0.04em]">{formatUsd(LOT.priceCents)}</span>
              <span className="numeric text-base text-muted-foreground">≈ {dong(LOT.priceCents)}</span>
            </>
          ) : (
            <>
              <span className="numeric font-heading text-5xl font-medium tracking-[-0.04em]">
                <abbr title={l('approx')} className="no-underline">≈</abbr> {dong(LOT.priceCents)}
              </span>
              <span className="numeric text-base text-muted-foreground">{l('chargedAs')} {formatUsd(LOT.priceCents)} USD</span>
            </>
          )}
        </div>

        <div className="flex flex-col gap-3 border-t border-hairline pt-6">
          <div className="flex items-center justify-between gap-3">
            <span className="flex items-center gap-2 font-medium"><Icon icon={ArrowRightLeft} size={18} />{l('yourBid')}</span>
            <span className="numeric text-sm text-muted-foreground">
              {l('minimum')} {currency === 'USD' ? formatUsd(minimum) : dong(minimum)} · {l('increment')} {formatUsd(LOT.incrementCents)}
            </span>
          </div>
          <div className="flex flex-wrap gap-3">
            <div className="relative min-w-48 flex-1">
              <Input
                inputMode={currency === 'VND' ? 'numeric' : 'decimal'}
                value={bid}
                onChange={(e) => setBid(e.target.value)}
                placeholder={currency === 'VND' ? '4.400.000' : '176.00'}
                aria-label={`${l('yourBid')} (${currency})`}
                className="min-h-12 pr-14 text-lg"
              />
              <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 font-mono text-xs text-muted-foreground">{currency === 'VND' ? '₫' : 'USD'}</span>
            </div>
            <Button type="button" size="lg" disabled={submitCents === null}>{l('yourBid')}</Button>
          </div>
          <div aria-live="polite" className="min-h-20 rounded-[2px] bg-muted p-4 text-sm">
            {bid === '' ? (
              <span className="text-muted-foreground">{l('roundedUp')}.</span>
            ) : submitCents === null ? (
              <span className="text-destructive">{l('invalidAmount')}</span>
            ) : (
              <div className="flex flex-col gap-1">
                <span className="mono-label">{l('willSubmit')}</span>
                <span className="numeric text-xl">{formatUsd(submitCents)} USD <span className="text-sm text-muted-foreground">≈ {dong(submitCents)}</span></span>
                <span className="text-xs text-muted-foreground">{l('roundedUp')}</span>
              </div>
            )}
          </div>
        </div>
      </LotStage>

      <div className="grid items-start gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardDescription className="mono-label">{l('receipt')}</CardDescription>
            <CardTitle className="flex items-center gap-2"><Icon icon={Receipt} size={18} />#A41C…9F02</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center gap-4">
              <img src={lotArt(ORDER.lot)} alt="" aria-hidden="true" className="size-16 rounded-[2px] border border-hairline bg-muted object-contain p-1 [filter:grayscale(1)]" />
              <div className="flex flex-col">
                <span className="font-medium">{ORDER.lot.title}</span>
                <span className="text-sm text-muted-foreground">{ORDER.lot.event}</span>
              </div>
            </div>
            <dl className="numeric m-0 grid grid-cols-[1fr_auto] gap-x-4 gap-y-1.5">
              <dt className="text-muted-foreground">{l('hammer')}</dt><dd className="m-0 text-end">{formatUsd(ORDER.hammer)}</dd>
              <dt className="text-muted-foreground">{l('delivery')}</dt><dd className="m-0 text-end">{formatUsd(ORDER.delivery)}</dd>
              <dt className="border-t border-hairline pt-2 font-medium">{l('totalCharged')}</dt>
              <dd className="m-0 border-t border-hairline pt-2 text-end text-lg font-medium">{formatUsd(total)} USD</dd>
              <dt className="text-muted-foreground">{l('shownAs')}</dt>
              <dd className="m-0 text-end">≈ {formatDongAmount(centsToDong(total, orderRate))}</dd>
            </dl>
            <p className="text-sm text-muted-foreground">
              {l('atRate')} {formatDongAmount(BigInt(orderRate.dongPerUsd))}/$ · {l('rateOf')} {date(orderRate.effectiveAt)}. {l('receiptNote')}
            </p>
          </CardContent>
        </Card>

        <div className="flex flex-col gap-6">
          <Card>
            <CardHeader>
              <CardTitle>{l('rateCard')}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-5">
              <div className="flex items-end gap-3">
                <div className="flex-1">
                  <Field label={l('dongPerUsd')}>
                    <Input inputMode="numeric" value={draftRate} onChange={(e) => setDraftRate(e.target.value.replace(/\D/g, ''))} />
                  </Field>
                </div>
                <Button type="button" onClick={publish}>{l('publishRate')}</Button>
              </div>
              <div>
                <p className="mono-label mb-2">{l('rateHistory')}</p>
                <ul className="numeric m-0 flex list-none flex-col p-0 text-sm">
                  {[...rates].reverse().map((r, n) => (
                    <li key={r.id} className="grid grid-cols-[1fr_auto_4.5rem] items-center gap-3 border-b border-hairline py-2">
                      <span>{formatDongAmount(BigInt(r.dongPerUsd))}/$</span>
                      <span className="text-muted-foreground">{r.source} · {date(r.effectiveAt)}</span>
                      <span className="justify-self-end">{n === 0 ? <Badge tone="live">live</Badge> : r.id === ORDER.rateId ? <Badge tone="ended">order</Badge> : null}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </CardContent>
          </Card>
          <Rules
            items={[
              'USD cents stay the only stored and charged amount (`*_minor`).',
              'Display currency is a user preference, independent of language.',
              'VND rounds to whole đồng (1.000 ₫ on prices) and is always marked ≈.',
              'VND bids round UP to the next valid USD increment, then confirm.',
              'Orders snapshot `fx_rate_id`; receipts and analytics use it, never today’s rate.',
              'No rate published → show USD only; never fall back to a constant.',
            ]}
          />
        </div>
      </div>
    </section>
  )
}
