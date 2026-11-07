import { useMemo, useState, type ReactNode } from 'react'
import { ArrowDownRight, ArrowUpRight, Timer, TrendingUp, Wallet } from 'lucide-react'
import {
  Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, ComposedChart, LabelList, Line,
  ReferenceDot, ReferenceLine, XAxis, YAxis,
} from 'recharts'
import { cn } from 'cn'
import { Icon } from '@/components/site/Icon'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart'
import {
  SEED_LOTS, centsToDong, forecastRevenue, formatDongAmount, formatUsd, lotArt, predictHammer,
  type FxRate,
} from '@/lib/labs'
import { useLabsCopy, type LabsKey } from '@/lib/labs-copy'
import { Choice, LabHeader } from './shared'

/* Colour does one job per chart. Validated categorical slots (light, on
   #FBFAF8): blue = the measure / the emphasised mark, orange = the thing that
   needs attention, aqua = above estimate. De-emphasised marks are the same
   hue at reduced opacity, never a new hue. Every highlight also carries a
   text label, so meaning is never colour-alone. */
const BLUE = '#2a78d6'
const ORANGE = '#eb6834'
const AQUA = '#1baf7a'
const SOFT = 0.32
const OUTCOME: Record<'below' | 'within' | 'above', string> = { below: ORANGE, within: BLUE, above: AQUA }

// ---------- Fixtures (deterministic) ----------
const DAY = 86_400_000
const END = Date.UTC(2026, 10, 7)
function seeded(seed: number) {
  let s = seed
  return () => ((s = (s * 1_103_515_245 + 12_345) % 2_147_483_648) / 2_147_483_648)
}
const rnd = seeded(7)
const DAILY = Array.from({ length: 30 }, (_, n) => {
  const day = END - (29 - n) * DAY
  const weekend = [0, 6].includes(new Date(day).getUTCDay())
  return { day, bids: Math.round(4 + n * 0.45 + rnd() * 6 + (weekend ? 5 : 0)) }
})
const TIMING = [
  { bucket: '>60', bids: 18 }, { bucket: '30–60', bids: 9 }, { bucket: '10–30', bids: 14 },
  { bucket: '5–10', bids: 11 }, { bucket: '2–5', bids: 16 }, { bucket: '<2', bids: 41 },
]
const LOTS = [
  { lot: SEED_LOTS.rangefinder, low: 12_000, high: 18_000, hammer: 20_500 },
  { lot: SEED_LOTS.fieldWatch, low: 25_000, high: 35_000, hammer: 31_000 },
  { lot: SEED_LOTS.turntable, low: 8_000, high: 12_000, hammer: 7_200 },
  { lot: SEED_LOTS.twinLens, low: 9_000, high: 14_000, hammer: 13_800 },
  { lot: SEED_LOTS.jazzRecord, low: 3_000, high: 5_000, hammer: 6_100 },
  { lot: SEED_LOTS.receiver, low: 11_000, high: 16_000, hammer: 9_800 },
]
const CATEGORIES = [
  { category: 'Cameras', sold: 9, terminal: 11 },
  { category: 'Watches', sold: 6, terminal: 8 },
  { category: 'Vinyl', sold: 7, terminal: 12 },
  { category: 'Audio', sold: 3, terminal: 6 },
  { category: 'Décor', sold: 2, terminal: 5 },
]
const RATES: FxRate[] = [
  { id: 1, dongPerUsd: 25_120, source: 'seed', effectiveAt: END - 20 * DAY },
  { id: 2, dongPerUsd: 25_380, source: 'operator', effectiveAt: END - DAY },
]
const ORDERS = [
  { cents: 20_500, rate: RATES[0]! }, { cents: 31_000, rate: RATES[0]! },
  { cents: 13_800, rate: RATES[1]! }, { cents: 6_100, rate: RATES[1]! },
]
// Hammer revenue realised per day over the last 14 days (cents), oldest first.
const REALISED_DAILY = [0, 0, 12_000, 0, 20_500, 0, 8_000, 31_000, 0, 13_800, 0, 6_100, 18_000, 9_800]
// Lots still open: current bid and days until close.
const OPEN = [
  { lot: SEED_LOTS.mantelClock, currentCents: 14_000, closesInDays: 1 },
  { lot: SEED_LOTS.speakers, currentCents: 22_000, closesInDays: 2 },
  { lot: SEED_LOTS.pocketWatch, currentCents: 16_000, closesInDays: 3 },
  { lot: SEED_LOTS.jazzRecord, currentCents: 4_500, closesInDays: 4 },
  { lot: SEED_LOTS.receiver, currentCents: 12_500, closesInDays: 5 },
]
const UPLIFTS = { '20-30': [20, 30], '30-40': [30, 40], '40-50': [40, 50] } as const
type Uplift = keyof typeof UPLIFTS

const outcome = (l: (typeof LOTS)[number]) => (l.hammer < l.low ? 'below' : l.hammer > l.high ? 'above' : 'within')
const axisUsd = (v: number) => `$${Math.round(v / 100).toLocaleString('en-US')}`
const dayLabel = (t: number, locale: string) => new Date(t).toLocaleDateString(locale, { day: '2-digit', month: 'short', timeZone: 'UTC' })

function forecastSeries(lo: number, hi: number, locale: string) {
  let cum = 0
  const past = REALISED_DAILY.map((v, n) => {
    cum += v
    const label = dayLabel(END - (REALISED_DAILY.length - 1 - n) * DAY, locale)
    const today = n === REALISED_DAILY.length - 1
    return { label, actual: cum, ...(today ? { band: [cum, cum] as [number, number], mid: cum } : {}) }
  })
  const ahead = Array.from({ length: 5 }, (_, i) => {
    const d = i + 1
    const f = forecastRevenue(cum, OPEN.filter((o) => o.closesInDays <= d), lo, hi)
    return { label: dayLabel(END + d * DAY, locale), band: [f.low, f.high] as [number, number], mid: f.mid }
  })
  return [...past, ...ahead]
}

export default function AnalyticsLab() {
  const { l, lang } = useLabsCopy()
  const [range, setRange] = useState<'7' | '30'>('30')
  const [cur, setCur] = useState<'USD' | 'VND'>('USD')
  const [uplift, setUplift] = useState<Uplift>('30-40')
  const locale = lang === 'vi' ? 'vi-VN' : 'en-GB'

  // ---- Revenue forecast ----
  const [lo, hi] = UPLIFTS[uplift]
  const realised = REALISED_DAILY.reduce((t, v) => t + v, 0)
  const projection = forecastRevenue(realised, OPEN, lo, hi)
  const todayLabel = dayLabel(END, locale)
  const forecastData = useMemo(() => forecastSeries(lo, hi, locale), [lo, hi, locale])
  const last = forecastData[forecastData.length - 1]!

  // ---- Bids over time ----
  const daily = useMemo(() => DAILY.slice(-Number(range)).map((d) => ({ ...d, label: dayLabel(d.day, locale) })), [range, locale])
  const peak = daily.reduce((p, d) => (d.bids > p.bids ? d : p), daily[0]!)
  const last7 = DAILY.slice(-7).reduce((t, d) => t + d.bids, 0)
  const prev7 = DAILY.slice(-14, -7).reduce((t, d) => t + d.bids, 0)
  const delta = Math.round(((last7 - prev7) / prev7) * 1000) / 10

  // ---- Timing / categories / money ----
  const timingTotal = TIMING.reduce((t, b) => t + b.bids, 0)
  const snipe = TIMING[TIMING.length - 1]!
  const snipePct = Math.round((snipe.bids / timingTotal) * 1000) / 10
  const categories = CATEGORIES.map((c) => ({ ...c, pct: Math.round((c.sold / c.terminal) * 1000) / 10 }))
    .sort((a, b) => b.pct - a.pct)
  const avgPct = Math.round((CATEGORIES.reduce((t, c) => t + c.sold, 0) / CATEGORIES.reduce((t, c) => t + c.terminal, 0)) * 1000) / 10
  const usd = ORDERS.reduce((t, o) => t + o.cents, 0)
  const vnd = ORDERS.reduce((t, o) => t + centsToDong(o.cents, o.rate), 0n)
  const scaleMax = Math.max(...LOTS.map((x) => Math.max(x.high, x.hammer))) * 1.08
  const pct = (v: number) => `${(v / scaleMax) * 100}%`

  const forecastConfig = {
    actual: { label: l('realised'), color: BLUE },
    mid: { label: l('forecast'), color: BLUE },
  } satisfies ChartConfig
  const bidsConfig = { bids: { label: l('bids'), color: BLUE } } satisfies ChartConfig
  const stConfig = { pct: { label: l('byCategory'), color: BLUE } } satisfies ChartConfig

  return (
    <section className="flex flex-col gap-8">
      <LabHeader title={l('avTitle')} intro={l('avIntro')} />

      {/* Headline row: each tile carries its chart's accent. */}
      <dl className="m-0 grid grid-cols-[repeat(auto-fit,minmax(min(100%,14rem),1fr))] gap-4">
        <Kpi accent={BLUE} icon={Wallet} label={l('realised')} value={formatUsd(realised)}
          hint={`${REALISED_DAILY.filter(Boolean).length} lots · 14 ${l('day')}`} />
        <Kpi accent={BLUE} tint icon={TrendingUp} label={l('projected')}
          value={`${formatUsd(projection.low)} – ${formatUsd(projection.high)}`}
          hint={`${l('forecast')} ${formatUsd(projection.mid)} · +${lo}–${hi}%`} small />
        <Kpi accent={delta >= 0 ? 'var(--standing-leading)' : 'var(--standing-outbid)'} icon={delta >= 0 ? ArrowUpRight : ArrowDownRight}
          label={l('bids7')} value={String(last7)}
          hint={<span className={delta >= 0 ? 'text-(--standing-leading)' : 'text-(--standing-outbid)'}>{delta >= 0 ? '▲' : '▼'} {Math.abs(delta)}% {l('vsPrev')}</span>} />
        <Kpi accent={ORANGE} icon={Timer} label={l('snipeShare')} value={`${snipePct}%`}
          hint={`${snipe.bids}/${timingTotal} ${l('snipeHint')}`} />
      </dl>

      {/* Revenue forecast */}
      <Card>
        <CardHeader>
          <CardTitle>{l('revenueForecast')}</CardTitle>
          <CardDescription className="max-w-[46rem]">{l('forecastNote')}</CardDescription>
          <CardAction>
            <Choice<Uplift> label={l('uplift')} value={uplift} onChange={setUplift}
              options={(Object.keys(UPLIFTS) as Uplift[]).map((k) => ({ value: k, label: `+${k.replace('-', '–')}%` }))} />
          </CardAction>
        </CardHeader>
        <CardContent className="grid gap-8 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
          <div className="flex flex-col gap-3">
            <ul className="m-0 flex list-none flex-wrap gap-x-5 gap-y-1 p-0 text-xs text-muted-foreground">
              <li className="flex items-center gap-2"><span aria-hidden="true" className="h-0.5 w-5" style={{ background: BLUE }} />{l('realised')}</li>
              <li className="flex items-center gap-2"><span aria-hidden="true" className="h-0 w-5 border-t-2 border-dashed" style={{ borderColor: BLUE }} />{l('forecast')} (+{(lo + hi) / 2}%)</li>
              <li className="flex items-center gap-2"><span aria-hidden="true" className="h-3 w-5 rounded-[2px]" style={{ background: BLUE, opacity: 0.18 }} />+{lo}–{hi}%</li>
            </ul>
            <ChartContainer config={forecastConfig} className="aspect-auto h-72 w-full" role="img"
              aria-label={`${l('revenueForecast')}: ${formatUsd(realised)} → ${formatUsd(projection.low)}–${formatUsd(projection.high)}`}>
              <ComposedChart data={forecastData} margin={{ top: 16, right: 96, left: 8 }}>
                <defs>
                  <linearGradient id="labs-rev" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={BLUE} stopOpacity={0.28} />
                    <stop offset="100%" stopColor={BLUE} stopOpacity={0.02} />
                  </linearGradient>
                </defs>
                <CartesianGrid vertical={false} strokeDasharray="2 4" />
                <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={28} />
                <YAxis width={56} tickLine={false} axisLine={false} tickFormatter={axisUsd} />
                <ChartTooltip cursor={{ stroke: 'var(--border)' }} content={<ForecastTip />} />
                <Area dataKey="band" stroke="none" fill={BLUE} fillOpacity={0.16} isAnimationActive={false} />
                <Area dataKey="actual" type="monotone" stroke={BLUE} strokeWidth={2.5} fill="url(#labs-rev)" />
                <Line dataKey="mid" type="monotone" stroke={BLUE} strokeWidth={2} strokeDasharray="6 5" dot={false} />
                <ReferenceLine x={todayLabel} stroke="var(--ink)" strokeOpacity={0.4} strokeDasharray="3 3"
                  label={{ value: l('today'), position: 'insideTopLeft', fill: 'var(--muted-foreground)', fontSize: 11 }} />
                <ReferenceDot x={last.label} y={last.mid} r={5} fill={BLUE} stroke="var(--paper)" strokeWidth={2}
                  label={{ value: formatUsd(last.mid ?? 0), position: 'right', fill: 'var(--foreground)', fontSize: 12 }} />
              </ComposedChart>
            </ChartContainer>
          </div>
          <div className="flex flex-col gap-3">
            <p className="mono-label">{l('openLots')}</p>
            <ul className="m-0 flex list-none flex-col gap-3 p-0">
              {OPEN.map((o) => {
                const p = predictHammer(o.currentCents, lo, hi)
                return (
                  <li key={o.lot.n} className="grid grid-cols-[2.75rem_minmax(0,1fr)_auto] items-center gap-3 border-b border-hairline pb-3 text-sm">
                    <img src={lotArt(o.lot)} alt="" aria-hidden="true" className="size-11 rounded-[2px] border border-hairline bg-muted object-contain p-0.5 [filter:grayscale(1)]" />
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate">{o.lot.title}</span>
                      <span className="numeric text-xs text-muted-foreground">{formatUsd(o.currentCents)} · {l('closesInDays')} {o.closesInDays}{l('day')}</span>
                    </span>
                    <span className="numeric flex flex-col items-end">
                      <span className="font-medium" style={{ color: BLUE }}>{formatUsd(p.mid)}</span>
                      <span className="text-xs text-muted-foreground">{formatUsd(p.low)}–{formatUsd(p.high)}</span>
                    </span>
                  </li>
                )
              })}
            </ul>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle>{l('overTime')}</CardTitle>
            <CardDescription className="numeric">
              {l('peak')}: <span className="text-foreground">{peak.bids}</span> · {peak.label} <Needs text="date_trunc('day', bid.created_at)" />
            </CardDescription>
            <CardAction>
              <Choice<'7' | '30'> label={l('overTime')} value={range} onChange={setRange} options={[{ value: '7', label: l('days7') }, { value: '30', label: l('days30') }]} />
            </CardAction>
          </CardHeader>
          <CardContent>
            <ChartContainer config={bidsConfig} className="aspect-auto h-60 w-full" role="img" aria-label={l('overTime')}>
              <AreaChart data={daily} margin={{ left: 0, right: 16, top: 20 }}>
                <defs>
                  <linearGradient id="labs-bids" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={BLUE} stopOpacity={0.3} />
                    <stop offset="100%" stopColor={BLUE} stopOpacity={0.02} />
                  </linearGradient>
                </defs>
                <CartesianGrid vertical={false} strokeDasharray="2 4" />
                <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={24} />
                <YAxis width={28} tickLine={false} axisLine={false} allowDecimals={false} />
                <ChartTooltip cursor={{ stroke: 'var(--border)' }} content={<ChartTooltipContent />} />
                <Area dataKey="bids" type="monotone" stroke={BLUE} strokeWidth={2} fill="url(#labs-bids)" />
                <ReferenceDot x={peak.label} y={peak.bids} r={5} fill={BLUE} stroke="var(--paper)" strokeWidth={2}
                  label={{ value: `${l('peak')} ${peak.bids}`, position: 'top', fill: 'var(--foreground)', fontSize: 11 }} />
              </AreaChart>
            </ChartContainer>
          </CardContent>
        </Card>

        <Card className="relative overflow-hidden">
          <span aria-hidden="true" className="absolute inset-x-0 top-0 h-1" style={{ background: BLUE }} />
          <CardHeader>
            <CardTitle>{l('winningIn')}</CardTitle>
            <CardAction>
              <Choice<'USD' | 'VND'> label={l('winningIn')} value={cur} onChange={setCur} options={[{ value: 'USD', label: '$' }, { value: 'VND', label: '₫' }]} />
            </CardAction>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            <span className="numeric font-heading text-4xl font-medium tracking-[-0.04em]">
              {cur === 'USD' ? `${formatUsd(usd)}` : `≈ ${formatDongAmount(vnd)}`}
            </span>
            <span className="text-sm text-muted-foreground">{cur === 'USD' ? `${ORDERS.length} orders · USD` : l('snapshotRates')}</span>
            <ul className="numeric m-0 mt-2 flex list-none flex-col gap-1 p-0 text-xs text-muted-foreground">
              {ORDERS.map((o, n) => (
                <li key={n} className="flex justify-between gap-3">
                  <span>{formatUsd(o.cents)}</span>
                  <span>× {formatDongAmount(BigInt(o.rate.dongPerUsd))}</span>
                  <span className="text-foreground">{formatDongAmount(centsToDong(o.cents, o.rate))}</span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{l('bidTiming')}</CardTitle>
            <CardDescription>{l('bidTimingNote')} <Needs text="close_at − bid.created_at" /></CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <Legend items={[[BLUE, l('bids'), SOFT], [ORANGE, `${l('sniping')} · ${snipePct}%`, 1]]} />
            <ChartContainer config={bidsConfig} className="aspect-auto h-56 w-full" role="img" aria-label={`${l('bidTiming')}. ${l('sniping')}: ${snipePct}%`}>
              <BarChart data={TIMING} margin={{ top: 22, left: 0, right: 8 }}>
                <CartesianGrid vertical={false} strokeDasharray="2 4" />
                <XAxis dataKey="bucket" tickLine={false} axisLine={false} unit=" min" />
                <YAxis hide />
                <ChartTooltip cursor={{ fill: 'var(--muted)' }} content={<ChartTooltipContent />} />
                <Bar dataKey="bids" radius={[4, 4, 0, 0]}>
                  {TIMING.map((b) => (
                    <Cell key={b.bucket} fill={b === snipe ? ORANGE : BLUE} fillOpacity={b === snipe ? 1 : SOFT} />
                  ))}
                  <LabelList dataKey="bids" position="top" className="numeric fill-foreground" />
                </Bar>
              </BarChart>
            </ChartContainer>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{l('byCategory')}</CardTitle>
            <CardDescription>{l('average')} {avgPct}% <Needs text="GROUP BY catalog_lots.category" /></CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <Legend items={[[BLUE, l('best'), 1], [BLUE, `≥ ${l('average').toLowerCase()}`, SOFT], [ORANGE, l('belowAvg'), 0.7]]} />
            <ChartContainer config={stConfig} className="aspect-auto h-56 w-full" role="img" aria-label={l('byCategory')}>
              <BarChart data={categories} layout="vertical" margin={{ top: 16, left: 0, right: 88 }}>
                <XAxis type="number" hide domain={[0, 100]} />
                <YAxis type="category" dataKey="category" width={72} tickLine={false} axisLine={false} />
                <ChartTooltip cursor={{ fill: 'var(--muted)' }} content={<ChartTooltipContent hideLabel formatter={(_v, _n, item) => (
                  <span className="numeric flex w-full justify-between gap-4"><span>{item.payload.category}</span><span>{item.payload.pct}% ({item.payload.sold}/{item.payload.terminal})</span></span>
                )} />} />
                <Bar dataKey="pct" radius={[0, 4, 4, 0]} barSize={18}>
                  {categories.map((c, n) => (
                    <Cell key={c.category} fill={c.pct < avgPct ? ORANGE : BLUE} fillOpacity={n === 0 ? 1 : c.pct < avgPct ? 0.7 : SOFT} />
                  ))}
                  <LabelList position="right" className="numeric fill-foreground" content={(p) => {
                    const { x = 0, y = 0, width = 0, height = 0, index = 0 } = p as { x?: number; y?: number; width?: number; height?: number; index?: number }
                    const c = categories[index]!
                    return <text x={Number(x) + Number(width) + 8} y={Number(y) + Number(height) / 2} dy="0.35em" className={cn('numeric fill-foreground text-xs', index === 0 && 'font-semibold')}>{c.pct}% · {c.sold}/{c.terminal}</text>
                  }} />
                </Bar>
                <ReferenceLine x={avgPct} stroke="var(--ink)" strokeOpacity={0.5} strokeDasharray="3 3"
                  label={{ value: `${l('average')} ${avgPct}%`, position: 'top', fill: 'var(--muted-foreground)', fontSize: 11 }} />
              </BarChart>
            </ChartContainer>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{l('vsEstimate')}</CardTitle>
          <CardDescription>{l('vsEstimateNote')} <Needs text="estimate_low_minor, estimate_high_minor" /></CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <ul className="m-0 flex list-none flex-wrap gap-x-5 gap-y-1 p-0 text-xs text-muted-foreground">
            {(['below', 'within', 'above'] as const).map((k) => (
              <li key={k} className="flex items-center gap-2">
                <span aria-hidden="true" className="size-2.5 rounded-full ring-2 ring-paper" style={{ background: OUTCOME[k] }} />
                {l(k as LabsKey)} ({LOTS.filter((x) => outcome(x) === k).length})
              </li>
            ))}
          </ul>
          <ul className="m-0 flex list-none flex-col gap-3 p-0">
            {LOTS.map((x) => {
              const o = outcome(x)
              return (
                <li key={x.lot.n} className="grid grid-cols-[minmax(10rem,15rem)_minmax(0,1fr)_auto] items-center gap-4 text-sm">
                  <span className="flex min-w-0 items-center gap-3">
                    <img src={lotArt(x.lot)} alt="" aria-hidden="true" className="size-11 shrink-0 rounded-[2px] border border-hairline bg-muted object-contain p-0.5 [filter:grayscale(1)]" />
                    <span className="truncate">{x.lot.title}</span>
                  </span>
                  <div className="relative h-8" aria-hidden="true">
                    <div className="absolute inset-x-0 top-1/2 h-px bg-hairline" />
                    <div className="absolute top-1/2 h-3 -translate-y-1/2 rounded-[2px]" style={{ left: pct(x.low), width: `calc(${pct(x.high)} - ${pct(x.low)})`, background: BLUE, opacity: 0.18 }} />
                    <div className="absolute top-1/2 size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full ring-2 ring-paper" style={{ left: pct(x.hammer), background: OUTCOME[o] }} />
                  </div>
                  <span className="numeric text-end text-xs">
                    <span className="text-foreground">{formatUsd(x.hammer)}</span>
                    <span className="block text-muted-foreground">{formatUsd(x.low)}–{formatUsd(x.high)} · {l(o)}</span>
                  </span>
                </li>
              )
            })}
          </ul>
        </CardContent>
      </Card>
    </section>
  )
}

function Kpi({ accent, tint = false, small = false, icon, label, value, hint }: {
  accent: string; tint?: boolean; small?: boolean
  icon: Parameters<typeof Icon>[0]['icon']; label: string; value: string; hint: ReactNode
}) {
  return (
    <Card size="sm" className="relative gap-2 overflow-hidden px-5">
      <span aria-hidden="true" className="absolute inset-x-0 top-0 h-1" style={{ background: accent }} />
      {tint && <span aria-hidden="true" className="absolute inset-0" style={{ background: accent, opacity: 0.06 }} />}
      <dt className="relative flex items-center justify-between gap-2">
        <span className="mono-label">{label}</span>
        <span style={{ color: accent }}><Icon icon={icon} size={16} /></span>
      </dt>
      <dd className="relative m-0 flex flex-col">
        <span className={cn('numeric font-heading leading-tight font-medium tracking-[-0.03em]', small ? 'text-xl' : 'text-[1.75rem]')}>{value}</span>
        <span className="text-xs text-muted-foreground">{hint}</span>
      </dd>
    </Card>
  )
}

function Legend({ items }: { items: [color: string, label: string, opacity: number][] }) {
  return (
    <ul className="m-0 flex list-none flex-wrap gap-x-5 gap-y-1 p-0 text-xs text-muted-foreground">
      {items.map(([c, label, o]) => (
        <li key={label} className="flex items-center gap-2">
          <span aria-hidden="true" className="size-2.5 rounded-[2px]" style={{ background: c, opacity: o }} />{label}
        </li>
      ))}
    </ul>
  )
}

type TipRow = { label: string; actual?: number; band?: [number, number]; mid?: number }
function ForecastTip({ active, payload }: { active?: boolean; payload?: readonly { payload: TipRow }[] }) {
  const { l } = useLabsCopy()
  const row = payload?.[0]?.payload
  if (!active || !row) return null
  return (
    <div className="grid min-w-44 gap-1 rounded-[2px] border border-hairline bg-paper px-3 py-2 text-xs shadow-sm">
      <span className="font-medium">{row.label}</span>
      {row.actual !== undefined && (
        <span className="numeric flex justify-between gap-4"><span className="text-muted-foreground">{l('realised')}</span>{formatUsd(row.actual)}</span>
      )}
      {row.mid !== undefined && row.band && row.band[0] !== row.band[1] && (
        <>
          <span className="numeric flex justify-between gap-4"><span className="text-muted-foreground">{l('forecast')}</span><span style={{ color: BLUE }}>{formatUsd(row.mid)}</span></span>
          <span className="numeric flex justify-between gap-4 text-muted-foreground"><span>{l('uplift')}</span>{formatUsd(row.band[0])}–{formatUsd(row.band[1])}</span>
        </>
      )}
    </div>
  )
}

function Needs({ text }: { text: string }) {
  const { l } = useLabsCopy()
  return <span className="font-mono text-[0.6875rem] text-muted-foreground">· {l('needs')}: <code>{text}</code></span>
}
