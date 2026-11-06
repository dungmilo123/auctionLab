import { useCallback, useEffect, useState } from 'react'
import { ArrowUpRight, BarChart3, Gavel, Plus, RefreshCw, Truck } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { catalogAPI, type AuctionEvent, type Category, type Lot } from '@/api/catalog'
import { AccountError } from '@/api/account'
import { fulfillmentAPI, STATUSES, type Summary } from '@/api/fulfillment'
import { insightsAPI, aggregateCount, aggregateMoney, sellThrough, type Analytics } from '@/api/insights'
import { formatOrderMoney } from '@/lib/order-money'
import { useFulfillmentCopy } from '@/lib/fulfillment-copy'
import { useSiteCopy } from '@/lib/site-copy'
import { useLanguage } from '@/lib/i18n'

type Overview = {
  analytics: Analytics
  events: AuctionEvent[]
  lots: Lot[]
  categories: Category[]
  orders: Summary[]
}
type Read =
  | { status: 'loading' }
  | { status: 'error'; error: AccountError }
  | { status: 'ready'; data: Overview }

const DAY = 86_400_000

function gateURL() {
  const gate = new URL('/admin', window.location.href)
  if (import.meta.env.DEV) gate.port = '8080'
  return gate.href
}

function when(ms: number, lang: string) {
  return new Intl.DateTimeFormat(lang === 'vi' ? 'vi-VN' : 'en-US', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(ms)
}

function relative(ms: number, now: number) {
  const delta = ms - now
  const abs = Math.abs(delta)
  if (abs < 60_000) return 'now'
  if (abs < 3_600_000) return `${Math.round(abs / 60_000)}m`
  if (abs < DAY) return `${Math.round(abs / 3_600_000)}h`
  return `${Math.round(abs / DAY)}d`
}

function Metric({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <Card size="sm" className="gap-2 px-5 py-4">
      <p className="mono-label">{label}</p>
      <p className="font-mono text-2xl leading-none tracking-[-0.02em] numeric">{value}</p>
      {note && <p className="text-xs text-muted-foreground">{note}</p>}
    </Card>
  )
}

function AccessNotice({ error, retry }: { error: AccountError; retry: () => void }) {
  const { t } = useSiteCopy()
  const denied = error.status === 401 || error.code === 'access_required'
  return (
    <Card className="max-w-2xl border-t-ink">
      <CardHeader>
        <CardTitle>{denied ? t('admin') : t('loading')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <p className="text-muted-foreground">{denied ? t('adminAccessNote') : t('unavailable')}</p>
        <div className="flex flex-wrap gap-3">
          {denied ? (
            <Button asChild>
              <a href={gateURL()} target="_blank" rel="noreferrer">
                {t('admin')}
                <Icon icon={ArrowUpRight} />
              </a>
            </Button>
          ) : (
            <Button variant="outline" onClick={retry}>
              <Icon icon={RefreshCw} />
              {t('adminRefresh')}
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

export default function AdminDashboard() {
  const { t } = useSiteCopy()
  const { lang } = useLanguage()
  const { f } = useFulfillmentCopy()
  const [read, setRead] = useState<Read>({ status: 'loading' })
  const [attempt, setAttempt] = useState(0)
  const load = useCallback(
    (signal: AbortSignal) =>
      Promise.all([
        insightsAPI.analytics({ event: 'all', dataset: 'all' }, signal),
        catalogAPI.events(),
        catalogAPI.lots(),
        catalogAPI.categories(),
        fulfillmentAPI.list({ status: 'all', method: 'all' }, signal),
      ]).then(([analytics, events, lots, categories, orders]) => ({ analytics, events, lots, categories, orders })),
    [],
  )
  const retry = () => {
    setRead({ status: 'loading' })
    setAttempt((n) => n + 1)
  }
  useEffect(() => {
    const controller = new AbortController()
    let active = true
    load(controller.signal).then(
      (data) => { if (active) setRead({ status: 'ready', data }) },
      (error) => {
        if (active)
          setRead({
            status: 'error',
            error: error instanceof AccountError ? error : new AccountError('unavailable'),
          })
      },
    )
    return () => {
      active = false
      controller.abort()
    }
  }, [load, attempt])

  if (read.status === 'loading')
    return (
      <div className="flex flex-col gap-10">
        <AdminHeader />
        <p role="status" className="mono-label">{t('loading')}</p>
      </div>
    )
  if (read.status === 'error')
    return (
      <div className="flex flex-col gap-10">
        <AdminHeader />
        <AccessNotice error={read.error} retry={retry} />
      </div>
    )

  const { analytics, events, lots, categories, orders } = read.data
  const summary = analytics.data.summary
  const now = analytics.meta.as_of
  const scheduled = [...events]
    .filter((event) => event.first_closes_at !== null)
    .sort((a, b) => (a.first_closes_at ?? 0) - (b.first_closes_at ?? 0))
    .slice(0, 5)
  const published = lots.filter((lot) => lot.state === 'published').length
  const drafts = lots.length - published
  const byCategory = categories
    .map((category) => ({
      category,
      count: lots.filter((lot) => lot.category_id === category.id).length,
    }))
    .filter((row) => row.count > 0)
    .sort((a, b) => b.count - a.count)
  const queue = STATUSES.map((status) => ({
    status,
    rows: orders.filter((order) => order.status === status),
  }))
  const recent = orders.slice(0, 6)

  return (
    <div className="flex flex-col gap-10">
      <AdminHeader onRefresh={retry} />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label={t('adminRevenueSim')} value={aggregateMoney(summary.simulated_paid_minor, lang)} note={f('paid')} />
        <Metric label={t('adminSoldRate')} value={sellThrough(summary)} note={`${summary.sold} / ${summary.terminal}`} />
        <Metric label={t('lots')} value={aggregateCount(summary.lots, lang)} note={`${published} ${t('adminPublished').toLowerCase()} · ${drafts} ${t('adminDrafts').toLowerCase()}`} />
        <Metric label={t('adminBids')} value={aggregateCount(summary.bids, lang)} note={`${summary.bidders} ${t('bidders')}`} />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
        <Card>
          <CardHeader className="flex-row items-baseline justify-between">
            <CardTitle>{t('adminClosingNext')}</CardTitle>
            <Button asChild variant="link">
              <a href="/admin/auctions">
                {t('adminAuctions')}
                <Icon icon={ArrowUpRight} />
              </a>
            </Button>
          </CardHeader>
          <CardContent>
            {scheduled.length === 0 ? (
              <p className="text-muted-foreground">{t('adminNoSchedule')}</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('adminAuctions')}</TableHead>
                    <TableHead>{t('adminFirstClose')}</TableHead>
                    <TableHead className="text-right">{t('lots')}</TableHead>
                    <TableHead>{t('overview')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {scheduled.map((event) => {
                    const closes = event.first_closes_at ?? 0
                    const open = event.opens_at !== null && event.opens_at <= now
                    const live = open && closes > now
                    return (
                      <TableRow key={event.id}>
                        <TableCell>
                          <a href={`/admin/auctions?edit=${encodeURIComponent(event.id)}`} className="font-medium">
                            {event.title}
                          </a>
                          <p className="text-xs text-muted-foreground">{event.theme}</p>
                        </TableCell>
                        <TableCell className="font-mono text-xs numeric">
                          <time dateTime={new Date(closes).toISOString()}>{when(closes, lang)}</time>
                          <p className="text-muted-foreground">{relative(closes, now)}</p>
                        </TableCell>
                        <TableCell className="text-right font-mono numeric">{event.lot_ids.length}</TableCell>
                        <TableCell>
                          <Badge tone={live ? 'live' : open ? 'closing' : 'upcoming'}>
                            {live ? t('live') : open ? t('adminOpenNow') : t('upcoming')}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-baseline justify-between">
            <CardTitle>{t('adminQueue')}</CardTitle>
            <Button asChild variant="link">
              <a href="/admin/orders">
                <Icon icon={Truck} />
                {t('orders')}
              </a>
            </Button>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {orders.length === 0 ? (
              <p className="text-muted-foreground">{t('adminNoOrders')}</p>
            ) : (
              queue.map(({ status, rows }) => (
                <div key={status} className="flex items-center justify-between gap-3">
                  <a href={`/admin/orders?status=${status}`} className="text-sm">
                    {f(status)}
                  </a>
                  <span className="font-mono text-sm numeric">{rows.length}</span>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)]">
        <Card>
          <CardHeader className="flex-row items-baseline justify-between">
            <CardTitle>{t('adminInventory')}</CardTitle>
            <Button asChild variant="link">
              <a href="/admin/lots">
                <Icon icon={Gavel} />
                {t('lots')}
              </a>
            </Button>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex gap-6">
              <div>
                <p className="mono-label">{t('adminPublished')}</p>
                <p className="font-mono text-xl numeric">{published}</p>
              </div>
              <div>
                <p className="mono-label">{t('adminDrafts')}</p>
                <p className="font-mono text-xl numeric">{drafts}</p>
              </div>
            </div>
            <Separator />
            {byCategory.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('adminNoSchedule')}</p>
            ) : (
              byCategory.map(({ category, count }) => (
                <div key={category.id} className="flex flex-col gap-1">
                  <div className="flex items-baseline justify-between gap-3">
                    <span className="text-sm">{lang === 'vi' ? category.name_vi : category.name_en}</span>
                    <span className="font-mono text-xs numeric">{count}</span>
                  </div>
                  <div className="h-1.5 w-full bg-ink/10">
                    <div
                      className="h-full bg-ink"
                      style={{ width: `${Math.round((count / Math.max(1, lots.length)) * 100)}%` }}
                    />
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex-row items-baseline justify-between">
            <CardTitle>{t('adminRecentOrders')}</CardTitle>
            <Button asChild variant="link">
              <a href="/admin/analytics">
                <Icon icon={BarChart3} />
                {t('analytics')}
              </a>
            </Button>
          </CardHeader>
          <CardContent>
            {recent.length === 0 ? (
              <p className="text-muted-foreground">{t('adminNoOrders')}</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('orders')}</TableHead>
                    <TableHead>{t('adminQueue')}</TableHead>
                    <TableHead className="text-right">{t('total')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recent.map((order) => (
                    <TableRow key={order.id}>
                      <TableCell>
                        <a href={`/admin/orders?view=${encodeURIComponent(order.id)}`} className="font-medium">
                          {order.title}
                        </a>
                        <p className="text-xs text-muted-foreground">
                          <time dateTime={new Date(order.recorded_at).toISOString()}>
                            {when(order.recorded_at, lang)}
                          </time>
                        </p>
                      </TableCell>
                      <TableCell>
                        <Badge tone={order.state === 'paid' ? 'live' : 'ended'}>{f(order.status)}</Badge>
                      </TableCell>
                      <TableCell className="text-right font-mono text-sm numeric">
                        {formatOrderMoney(order.total_minor ?? order.winning_amount_minor)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function AdminHeader({ onRefresh }: { onRefresh?: () => void }) {
  const { t } = useSiteCopy()
  return (
    <header className="flex flex-wrap items-end justify-between gap-6">
      <div className="flex flex-col gap-2">
        <p className="mono-label">{t('concept')}</p>
        <h1 className="font-display text-[clamp(2rem,4vw,3rem)] leading-none font-medium tracking-[-0.045em]">
          {t('admin')}
        </h1>
        <p className="max-w-[36rem] text-muted-foreground">{t('adminDashboardIntro')}</p>
      </div>
      <div className="flex flex-wrap items-start gap-3">
        <Button asChild>
          <a href="/admin/auctions?new=1">
            <Icon icon={Plus} />
            {t('adminNewEvent')}
          </a>
        </Button>
        <Button asChild variant="outline">
          <a href="/admin/lots?new=1">
            <Icon icon={Plus} />
            {t('adminNewLot')}
          </a>
        </Button>
        {onRefresh && (
          <Button variant="outline" size="icon" aria-label={t('adminRefresh')} onClick={onRefresh}>
            <Icon icon={RefreshCw} />
          </Button>
        )}
      </div>
    </header>
  )
}
