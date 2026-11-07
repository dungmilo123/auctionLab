import {
  Activity,
  BarChart3,
  Gavel,
  RefreshCw,
  SlidersHorizontal,
  Users,
  Wallet,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Bar, BarChart, CartesianGrid, LabelList, XAxis, YAxis } from "recharts";
import { cn } from "cn";
import { Icon } from "@/components/site/Icon";
import { PageHeading } from "@/components/site/Presentation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { Select } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  insightsAPI,
  metricKeys,
  aggregateCount,
  aggregateMoney,
  sellThrough,
  parseAnalyticsFilters,
  type Analytics,
  type Metrics,
  type MetricKey,
} from "@/api/insights";
import { useInsightsCopy, type InsightsKey } from "@/lib/insights-copy";
import { startPrivateRead } from "@/routes/account/usePrivateRead";

type Read =
  | { status: "loading" | "error" | "access" }
  | { status: "ready"; value: Analytics };

/* Categorical slots validated (light, surface #FBFAF8, all pairs): worst CVD
   ΔE 9.2, normal-vision ΔE 24. Slot 3 is < 3:1 on paper, so every segmented
   bar carries a text legend with exact values (relief rule). Single-series
   charts stay in ink. Colour follows the entity, never its rank. */
const SERIES = ["#2a78d6", "#eb6834", "#1baf7a"] as const;
const INK = "var(--ink)";

const STAGES = [
  "lot_view",
  "watchlist_add",
  "bid_accepted",
  "payment_confirmed",
] as const;

type EventMeasure = "winning_minor" | "bids" | "sell_through" | "lots";
const MEASURES: { key: EventMeasure; label: InsightsKey }[] = [
  { key: "winning_minor", label: "winningShort" },
  { key: "bids", label: "bidsShort" },
  { key: "sell_through", label: "sellThroughShort" },
  { key: "lots", label: "lots" },
];

const big = (s: string) => BigInt(s);
// Chart geometry only; every printed figure uses the exact BigInt formatters.
const ratio = (part: bigint, whole: bigint) =>
  whole === 0n ? 0 : Number((part * 10000n) / whole) / 100;
const sellPct = (m: Metrics) =>
  m.sell_through === null ? 0 : ratio(big(m.sold), big(m.terminal));

export default function AnalyticsPage() {
  return <AnalyticsWorkspace key={window.location.search} />;
}

function AnalyticsWorkspace() {
  const { i } = useInsightsCopy(),
    filters = parseAnalyticsFilters(window.location.search);
  const event = filters?.event ?? "all",
    dataset = filters?.dataset ?? "all",
    valid = filters !== null;
  const generation = useRef(0);
  const [read, setRead] = useState<Read>({ status: "loading" }),
    [attempt, retry] = useState(0);
  useEffect(() => {
    if (!valid) return;
    const current = ++generation.current;
    return startPrivateRead(
      (s) => insightsAPI.analytics({ event, dataset }, s),
      (v) => {
        if (generation.current === current)
          setRead({ status: "ready", value: v });
      },
      (e) => {
        if (generation.current === current)
          setRead({ status: e.status === 401 ? "access" : "error" });
      },
    );
  }, [event, dataset, attempt, valid]);
  useEffect(() => {
    const hide = () => {
        generation.current++;
        setRead({ status: "loading" });
      },
      show = () => {
        setRead({ status: "loading" });
        retry((n) => n + 1);
      };
    window.addEventListener("pagehide", hide);
    window.addEventListener("pageshow", show);
    return () => {
      window.removeEventListener("pagehide", hide);
      window.removeEventListener("pageshow", show);
    };
  }, []);
  const refresh = () => {
    setRead({ status: "loading" });
    retry((n) => n + 1);
  };

  return (
    <div className="flex flex-col gap-8">
      <PageHeading title={i("analytics")} intro={i("intro")} />

      {/* Filters: one row above every chart. */}
      <form
        method="get"
        action="/admin/analytics"
        className="flex flex-wrap items-end gap-4 border-y border-hairline py-4"
      >
        <label className="flex min-w-48 flex-1 flex-col gap-1.5">
          <span className="mono-label">{i("event")}</span>
          <Input
            name="event"
            defaultValue={event}
            list="analytics-events"
            required
            pattern="[a-z0-9]+(-[a-z0-9]+)*"
          />
          <datalist id="analytics-events">
            <option value="all">{i("all")}</option>
            {read.status === "ready" &&
              read.value.data.events.map((e) => (
                <option key={e.slug} value={e.slug}>
                  {e.title}
                </option>
              ))}
          </datalist>
        </label>
        <label className="flex min-w-48 flex-1 flex-col gap-1.5">
          <span className="mono-label">{i("dataset")}</span>
          <Select name="dataset" defaultValue={dataset}>
            <option value="all">{i("all")}</option>
            <option value="demo">{i("demo")}</option>
            <option value="non-demo">{i("nonDemo")}</option>
          </Select>
        </label>
        <Button type="submit">
          <Icon icon={SlidersHorizontal} />
          {i("apply")}
        </Button>
      </form>

      {!filters ? (
        <p role="alert">{i("error")}</p>
      ) : read.status !== "ready" ? (
        <StatusPanel read={read} onRetry={refresh} />
      ) : (
        <Dashboard value={read.value} onRefresh={refresh} />
      )}

      <section className="flex flex-col gap-2.5 border-t border-hairline pt-6 text-sm text-muted-foreground">
        <h2 className="text-foreground">{i("definitions")}</h2>
        <p>{i("cohort")}</p>
        <p>{i("formula")}</p>
        <p>{i("moneyDefinition")}</p>
        <p>{i("observations")}</p>
      </section>
    </div>
  );
}

function StatusPanel({
  read,
  onRetry,
}: {
  read: { status: "loading" | "error" | "access" };
  onRetry: () => void;
}) {
  const { i } = useInsightsCopy();
  if (read.status === "loading")
    return (
      <div role="status" aria-busy="true" className="flex flex-col gap-4">
        <span className="sr-only">{i("loading")}</span>
        <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,13rem),1fr))] gap-4">
          {[0, 1, 2, 3].map((n) => (
            <div key={n} className="h-32 animate-pulse rounded-[2px] bg-muted" />
          ))}
        </div>
        <div className="h-64 animate-pulse rounded-[2px] bg-muted" />
      </div>
    );
  return (
    <Card role="status">
      <CardContent className="flex flex-wrap items-center justify-between gap-4">
        <p>{i(read.status === "access" ? "access" : "error")}</p>
        <Button variant="outline" onClick={onRetry}>
          <Icon icon={RefreshCw} />
          {i("retry")}
        </Button>
      </CardContent>
    </Card>
  );
}

function Dashboard({
  value,
  onRefresh,
}: {
  value: Analytics;
  onRefresh: () => void;
}) {
  const { i, lang } = useInsightsCopy();
  const { summary: s, events, interactions } = value.data;
  const money = (c: string | bigint) => aggregateMoney(c.toString(), lang);
  const count = (c: string | bigint) => aggregateCount(c.toString(), lang);
  const unpaid = big(s.winning_minor) - big(s.paid_winning_minor);
  const bidsPerLot =
    big(s.lots) === 0n
      ? "—"
      : (Number((big(s.bids) * 10n) / big(s.lots)) / 10).toLocaleString(lang);

  return (
    <>
      {/* Snapshot provenance + refresh */}
      <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
        <div className="flex flex-wrap gap-x-6 gap-y-1">
          <span>
            {i("asOf")}:{" "}
            <time
              className="numeric text-foreground"
              dateTime={new Date(value.meta.as_of).toISOString()}
            >
              {new Date(value.meta.as_of).toLocaleString(lang)}
            </time>
          </span>
          <span>
            {i("coverage")}:{" "}
            <span className="numeric text-foreground">
              {value.meta.collection_started_at === null
                ? i("unknown")
                : new Date(value.meta.collection_started_at).toLocaleString(lang)}
            </span>
          </span>
        </div>
        <Button variant="outline" size="sm" onClick={onRefresh}>
          <Icon icon={RefreshCw} />
          {i("refresh")}
        </Button>
      </div>

      {s.lots === "0" && (
        <p role="status" className="border border-hairline bg-paper p-4">
          {i("empty")}
        </p>
      )}

      {/* KPI row */}
      <section aria-label={i("headline")}>
        <dl className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,13rem),1fr))] gap-4">
          <Kpi
            metric="winning_minor"
            icon={Wallet}
            label={i("winningShort")}
            value={money(s.winning_minor)}
            hint={i("includesUnpaid")}
          />
          <Kpi
            metric="sell_through"
            icon={BarChart3}
            label={i("sellThroughShort")}
            value={sellThrough(s)}
            hint={`${count(s.sold)} / ${count(s.terminal)} ${i("soldOfTerminal")}`}
          >
            <Progress
              value={sellPct(s)}
              aria-label={i("sellThroughShort")}
              className="mt-3 h-1.5 rounded-[2px] [&>[data-slot=progress-indicator]]:bg-ink"
            />
          </Kpi>
          <Kpi
            metric="bids"
            icon={Gavel}
            label={i("bidsShort")}
            value={count(s.bids)}
            hint={`${bidsPerLot} ${i("bidsPerLot")}`}
          />
          <Kpi
            metric="bidders"
            icon={Users}
            label={i("biddersShort")}
            value={count(s.bidders)}
            hint={i("notAdditive")}
          />
        </dl>
      </section>

      {/* Composition: outcomes, bid mix, money */}
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{i("outcomes")}</CardTitle>
            <CardDescription>{i("outcomesNote")}</CardDescription>
            <CardAction>
              <Badge>
                {count(s.lots)} {i("lots")}
              </Badge>
            </CardAction>
          </CardHeader>
          <CardContent>
            <Segmented
              label={i("outcomes")}
              parts={[
                { key: "sold", label: i("sold"), value: big(s.sold), text: count(s.sold) },
                { key: "unsold", label: i("unsold"), value: big(s.unsold), text: count(s.unsold) },
                { key: "pending", label: i("pending"), value: big(s.pending), text: count(s.pending) },
              ]}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{i("bidMix")}</CardTitle>
            <CardDescription>{i("bidMixNote")}</CardDescription>
            <CardAction>
              <Badge>
                {count(s.bids)} {i("bidsShort")}
              </Badge>
            </CardAction>
          </CardHeader>
          <CardContent>
            <Segmented
              label={i("bidMix")}
              parts={[
                { key: "non-demo", label: i("non_demo_bids"), value: big(s.non_demo_bids), text: count(s.non_demo_bids) },
                { key: "demo", label: i("demo_bids"), value: big(s.demo_bids), text: count(s.demo_bids) },
              ]}
            />
            <p className="mt-4 text-sm text-muted-foreground">
              {i("non_demo_bidders")}: <span className="numeric text-foreground">{count(s.non_demo_bidders)}</span>
              {" · "}
              {i("demo_bidders")}: <span className="numeric text-foreground">{count(s.demo_bidders)}</span>
            </p>
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>{i("money")}</CardTitle>
            <CardDescription>{i("moneyNote")}</CardDescription>
            <CardAction>
              <Badge>{value.meta.currency}</Badge>
            </CardAction>
          </CardHeader>
          <CardContent className="grid gap-8 md:grid-cols-2">
            <div className="flex flex-col gap-3">
              <MoneyHeadline label={i("winning_minor")} value={money(s.winning_minor)} />
              <Segmented
                label={i("winning_minor")}
                parts={[
                  { key: "paid", label: i("paid_winning_minor"), value: big(s.paid_winning_minor), text: money(s.paid_winning_minor) },
                  ...(unpaid >= 0n
                    ? [{ key: "unpaid", label: i("unpaid_winning"), value: unpaid, text: money(unpaid) }]
                    : []),
                ]}
              />
            </div>
            <div className="flex flex-col gap-3">
              <MoneyHeadline label={i("simulated_paid_minor")} value={money(s.simulated_paid_minor)} />
              <Segmented
                label={i("simulated_paid_minor")}
                parts={[
                  { key: "paid", label: i("paid_winning_minor"), value: big(s.paid_winning_minor), text: money(s.paid_winning_minor) },
                  { key: "fees", label: i("paid_fees_minor"), value: big(s.paid_fees_minor), text: money(s.paid_fees_minor), slot: 2 },
                ]}
              />
              <p className="text-sm text-muted-foreground">
                {i("paid_orders")}: <span className="numeric text-foreground">{count(s.paid_orders)}</span>
                {" · "}
                {i("unpaid_sold_orders")}: <span className="numeric text-foreground">{count(s.unpaid_sold_orders)}</span>
              </p>
            </div>
          </CardContent>
        </Card>
      </div>

      <InteractionCard interactions={interactions} />
      <EventComparison events={events} />
    </>
  );
}

function Kpi({
  metric,
  icon,
  label,
  value,
  hint,
  children,
}: {
  metric: string;
  icon: Parameters<typeof Icon>[0]["icon"];
  label: string;
  value: string;
  hint: string;
  children?: ReactNode;
}) {
  return (
    <Card size="sm" data-metric={metric} className="gap-2 px-5">
      <dt className="flex items-center justify-between gap-2">
        <span className="mono-label">{label}</span>
        <Icon icon={icon} size={16} className="text-muted-foreground" />
      </dt>
      <dd className="m-0 flex flex-col">
        <span className="numeric font-heading text-[1.75rem] leading-tight font-medium tracking-[-0.03em]">
          {value}
        </span>
        <span className="text-xs text-muted-foreground">{hint}</span>
        {children}
      </dd>
    </Card>
  );
}

function MoneyHeadline({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col">
      <span className="mono-label">{label}</span>
      <span className="numeric font-heading text-2xl font-medium tracking-[-0.02em]">
        {value}
      </span>
    </div>
  );
}

/** Part-to-whole bar: 2px surface gaps, exact values in the legend. */
function Segmented({
  label,
  parts,
}: {
  label: string;
  parts: { key: string; label: string; value: bigint; text: string; slot?: number }[];
}) {
  const { i } = useInsightsCopy();
  const total = parts.reduce((t, p) => t + p.value, 0n);
  const color = (n: number, slot?: number) => SERIES[slot ?? n];
  return (
    <figure className="m-0 flex flex-col gap-4">
      <div
        aria-hidden="true"
        className="flex h-3 w-full gap-[2px] overflow-hidden rounded-[2px] bg-muted"
      >
        {total > 0n &&
          parts.map((p, n) =>
            p.value === 0n ? null : (
              <div
                key={p.key}
                className="h-full first:rounded-l-[2px] last:rounded-r-[2px]"
                style={{ flexGrow: ratio(p.value, total), background: color(n, p.slot) }}
              />
            ),
          )}
      </div>
      <figcaption className="sr-only">{label}</figcaption>
      <ul className="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(9rem,1fr))] gap-3 p-0">
        {parts.map((p, n) => (
          <li key={p.key} className="flex flex-col gap-0.5">
            <span className="flex items-center gap-2 text-xs text-muted-foreground">
              <span
                aria-hidden="true"
                className="size-2 shrink-0 rounded-full"
                style={{ background: color(n, p.slot) }}
              />
              {p.label}
            </span>
            <span className="numeric text-base">
              {p.text}
              <span className="ml-2 text-xs text-muted-foreground">
                {total > 0n ? `${ratio(p.value, total).toFixed(1)}%` : "—"}
                <span className="sr-only"> {i("share")}</span>
              </span>
            </span>
          </li>
        ))}
      </ul>
    </figure>
  );
}

function InteractionCard({
  interactions,
}: {
  interactions: Analytics["data"]["interactions"];
}) {
  const { i, lang } = useInsightsCopy();
  const rows = useMemo(
    () =>
      STAGES.map((kind) => {
        const items = interactions.filter((n) => n.kind === kind);
        const total = items.reduce((t, n) => t + big(n.count), 0n);
        return { kind, label: i(kind), total, value: Number(total), items };
      }),
    [interactions, i],
  );
  const config = { value: { label: i("funnel"), color: INK } } satisfies ChartConfig;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon icon={Activity} size={16} />
          {i("interactions")}
        </CardTitle>
        <CardDescription>{i("funnelNote")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <ChartContainer
          config={config}
          className="aspect-auto h-56 w-full"
          aria-label={i("funnel")}
          role="img"
        >
          <BarChart data={rows} layout="vertical" margin={{ left: 8, right: 48 }}>
            <CartesianGrid horizontal={false} strokeDasharray="2 4" />
            <XAxis type="number" hide allowDecimals={false} />
            <YAxis
              type="category"
              dataKey="label"
              width={150}
              tickLine={false}
              axisLine={false}
            />
            <ChartTooltip
              cursor={{ fill: "var(--muted)" }}
              content={
                <ChartTooltipContent
                  hideLabel
                  formatter={(_v, _n, item) => (
                    <span className="flex w-full justify-between gap-4">
                      <span>{item.payload.label}</span>
                      <span className="numeric">
                        {aggregateCount(item.payload.total.toString(), lang)}
                      </span>
                    </span>
                  )}
                />
              }
            />
            <Bar dataKey="value" fill="var(--color-value)" radius={[0, 4, 4, 0]} barSize={18}>
              <LabelList
                dataKey="total"
                position="right"
                className="fill-foreground numeric"
                formatter={(v: unknown) => aggregateCount(String(v), lang)}
              />
            </Bar>
          </BarChart>
        </ChartContainer>
        <dl className="m-0 grid content-start gap-3 text-sm">
          {rows.map((r) => (
            <div key={r.kind} className="border-b border-hairline pb-2">
              <dt className="flex justify-between gap-2">
                <span>{r.label}</span>
                <span className="numeric">{aggregateCount(r.total.toString(), lang)}</span>
              </dt>
              <dd className="m-0 flex flex-wrap gap-x-3 text-xs text-muted-foreground">
                {r.items.length === 0
                  ? aggregateCount("0", lang)
                  : r.items.map((n) => (
                      <span key={n.provenance}>
                        {i(n.provenance as InsightsKey)}:{" "}
                        <span className="numeric">{aggregateCount(n.count, lang)}</span>
                      </span>
                    ))}
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}

function EventComparison({ events }: { events: Analytics["data"]["events"] }) {
  const { i, lang } = useInsightsCopy();
  const [measure, setMeasure] = useState<EventMeasure>("winning_minor");
  const fmt = (m: Metrics, k: MetricKey) =>
    k.endsWith("_minor") ? aggregateMoney(m[k], lang) : aggregateCount(m[k], lang);
  const data = events
    .map((e) => ({
      title: e.title,
      value:
        measure === "sell_through"
          ? sellPct(e.metrics)
          : measure === "winning_minor"
            ? Number(big(e.metrics.winning_minor)) / 100
            : Number(e.metrics[measure]),
      text:
        measure === "sell_through" ? sellThrough(e.metrics) : fmt(e.metrics, measure),
    }))
    .sort((a, b) => b.value - a.value);
  const label = MEASURES.find((m) => m.key === measure)!.label;
  const config = { value: { label: i(label), color: INK } } satisfies ChartConfig;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{i("byEvent")}</CardTitle>
        <CardDescription>{i("byEventNote")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="chart" className="gap-4">
          <TabsList variant="line" className="h-auto">
            {(["chart", "table"] as const).map((v) => (
              <TabsTrigger
                key={v}
                value={v}
                className="min-h-9 rounded-[2px] px-3 font-mono text-xs tracking-[0.09em] uppercase"
              >
                {i(v === "chart" ? "chartView" : "tableView")}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="chart" className="flex flex-col gap-4">
            <div role="group" aria-label={i("byEvent")} className="flex flex-wrap gap-2">
              {MEASURES.map((m) => (
                <Button
                  key={m.key}
                  type="button"
                  size="sm"
                  variant={measure === m.key ? "default" : "outline"}
                  aria-pressed={measure === m.key}
                  onClick={() => setMeasure(m.key)}
                >
                  {i(m.label)}
                </Button>
              ))}
            </div>
            <ChartContainer
              config={config}
              className="aspect-auto w-full"
              style={{ height: Math.max(120, data.length * 44 + 24) }}
              role="img"
              aria-label={`${i("byEvent")}: ${i(label)}`}
            >
              <BarChart data={data} layout="vertical" margin={{ left: 8, right: 120 }}>
                <CartesianGrid horizontal={false} strokeDasharray="2 4" />
                <XAxis type="number" hide domain={measure === "sell_through" ? [0, 100] : undefined} />
                <YAxis
                  type="category"
                  dataKey="title"
                  width={160}
                  tickLine={false}
                  axisLine={false}
                />
                <ChartTooltip
                  cursor={{ fill: "var(--muted)" }}
                  content={
                    <ChartTooltipContent
                      hideLabel
                      formatter={(_v, _n, item) => (
                        <span className="flex w-full justify-between gap-4">
                          <span>{item.payload.title}</span>
                          <span className="numeric">{item.payload.text}</span>
                        </span>
                      )}
                    />
                  }
                />
                <Bar dataKey="value" fill="var(--color-value)" radius={[0, 4, 4, 0]} barSize={18}>
                  <LabelList dataKey="text" position="right" className="fill-foreground numeric" />
                </Bar>
              </BarChart>
            </ChartContainer>
          </TabsContent>
          <TabsContent value="table">
            <div
              className="max-w-full overflow-x-auto"
              role="region"
              aria-label={i("compare")}
              tabIndex={0}
            >
              <Table className="[&_td]:whitespace-nowrap [&_th]:whitespace-nowrap">
                <caption className="sr-only">{i("compare")}</caption>
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col" className="sticky left-0 bg-paper">
                      {i("event")}
                    </TableHead>
                    {metricKeys.map((k) => (
                      <TableHead scope="col" key={k} className="text-end">
                        {i(k)}
                      </TableHead>
                    ))}
                    <TableHead scope="col" className="text-end">
                      {i("sell_through")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {events.map((e) => (
                    <TableRow key={e.slug}>
                      <TableHead scope="row" className="sticky left-0 bg-paper">
                        <span className="flex items-center gap-2">
                          <a href={`/auctions/${encodeURIComponent(e.slug)}`}>{e.title}</a>
                          {e.demo && <Badge tone="ended">{i("demo")}</Badge>}
                        </span>
                      </TableHead>
                      {metricKeys.map((k) => (
                        <TableCell key={k} className={cn("numeric text-end")}>
                          {fmt(e.metrics, k)}
                        </TableCell>
                      ))}
                      <TableCell className="numeric text-end">{sellThrough(e.metrics)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
