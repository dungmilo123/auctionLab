import { AccountError } from "./account";
import type { PublicLot } from "./catalog";
import { isPublicLot, validInstant } from "./public-catalog";
const object = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);
export const reasons = [
  "category_and_era",
  "category",
  "era",
  "watchlist_category_and_era",
  "watchlist_category",
  "watchlist_era",
  "closing_soon",
] as const;
export type Reason = (typeof reasons)[number];
export type Suggestion = { lot: PublicLot; reason: Reason };
export type Suggestions = {
  data: Suggestion[];
  meta: { server_time: number; strategy: "rules-v1" };
};
export const metricKeys = [
  "lots",
  "bids",
  "bidders",
  "demo_bids",
  "non_demo_bids",
  "demo_bidders",
  "non_demo_bidders",
  "sold",
  "unsold",
  "terminal",
  "pending",
  "winning_minor",
  "paid_orders",
  "unpaid_sold_orders",
  "paid_winning_minor",
  "paid_fees_minor",
  "simulated_paid_minor",
] as const;
export type MetricKey = (typeof metricKeys)[number];
export type Metrics = Record<MetricKey, string> & {
  sell_through: null | { sold: string; terminal: string };
};
export type AnalyticsFilters = {
  event: string;
  dataset: "all" | "demo" | "non-demo";
};
export type Analytics = {
  data: {
    summary: Metrics;
    events: { slug: string; title: string; demo: boolean; metrics: Metrics }[];
    interactions: { kind: string; provenance: string; count: string }[];
    definitions: Record<string, string>;
  };
  meta: {
    as_of: number;
    currency: "USD";
    filters: AnalyticsFilters;
    collection_started_at: number | null;
  };
};
export const decimal = (v: unknown): v is string =>
  typeof v === "string" && /^(0|[1-9][0-9]*)$/.test(v);
function metrics(v: unknown): v is Metrics {
  if (!object(v) || !metricKeys.every((k) => decimal(v[k]))) return false;
  const n = (k: MetricKey) => BigInt(v[k] as string);
  if (
    n("sold") + n("unsold") !== n("terminal") ||
    n("terminal") + n("pending") !== n("lots") ||
    n("paid_winning_minor") + n("paid_fees_minor") !==
      n("simulated_paid_minor") ||
    n("demo_bids") + n("non_demo_bids") !== n("bids") ||
    n("demo_bidders") + n("non_demo_bidders") !== n("bidders")
  )
    return false;
  return n("terminal") === 0n
    ? v.sell_through === null
    : object(v.sell_through) &&
        v.sell_through.sold === v.sold &&
        v.sell_through.terminal === v.terminal;
}
export function decodeSuggestions(v: unknown): Suggestions {
  if (
    !object(v) ||
    !object(v.meta) ||
    v.meta.strategy !== "rules-v1" ||
    !validInstant(v.meta.server_time) ||
    !Array.isArray(v.data) ||
    v.data.length > 6 ||
    !v.data.every(
      (x) =>
        object(x) && isPublicLot(x.lot) && reasons.includes(x.reason as Reason),
    )
  )
    throw new AccountError("invalid_response");
  const data = v.data as Suggestion[];
  if (new Set(data.map((x) => x.lot.id)).size !== data.length)
    throw new AccountError("invalid_response");
  return {
    data,
    meta: { server_time: v.meta.server_time, strategy: "rules-v1" },
  };
}
export function parseAnalyticsFilters(raw: string): AnalyticsFilters | null {
  const q = new URLSearchParams(raw);
  if (
    [...q.keys()].some(
      (k) => !["event", "dataset"].includes(k) || q.getAll(k).length !== 1,
    )
  )
    return null;
  const event = q.get("event") ?? "all",
    dataset = q.get("dataset") ?? "all";
  return /^(all|[a-z0-9]+(?:-[a-z0-9]+)*)$/.test(event) &&
    event.length <= 120 &&
    ["all", "demo", "non-demo"].includes(dataset)
    ? { event, dataset: dataset as AnalyticsFilters["dataset"] }
    : null;
}
export function decodeAnalytics(v: unknown): Analytics {
  if (
    !object(v) ||
    !object(v.data) ||
    !object(v.meta) ||
    v.meta.currency !== "USD" ||
    !validInstant(v.meta.as_of) ||
    !(
      v.meta.collection_started_at === null ||
      validInstant(v.meta.collection_started_at)
    ) ||
    !object(v.meta.filters) ||
    typeof v.meta.filters.event !== "string" ||
    typeof v.meta.filters.dataset !== "string" ||
    !parseAnalyticsFilters(
      new URLSearchParams(v.meta.filters as Record<string, string>).toString(),
    ) ||
    !metrics(v.data.summary) ||
    !Array.isArray(v.data.events) ||
    !v.data.events.every(
      (e) =>
        object(e) &&
        typeof e.slug === "string" &&
        typeof e.title === "string" &&
        typeof e.demo === "boolean" &&
        metrics(e.metrics),
    ) ||
    !Array.isArray(v.data.interactions) ||
    !v.data.interactions.every(
      (i) =>
        object(i) &&
        [
          "lot_view",
          "watchlist_add",
          "bid_accepted",
          "payment_confirmed",
        ].includes(i.kind as string) &&
        ["observed", "backfilled", "snapshot"].includes(
          i.provenance as string,
        ) &&
        decimal(i.count),
    ) ||
    !object(v.data.definitions) ||
    !Object.values(v.data.definitions).every((x) => typeof x === "string")
  )
    throw new AccountError("invalid_response");
  const out = v as Analytics;
  for (const k of metricKeys.filter(
    (k) => !["bidders", "demo_bidders", "non_demo_bidders"].includes(k),
  )) {
    if (
      out.data.events.reduce((s, e) => s + BigInt(e.metrics[k]), 0n) !==
      BigInt(out.data.summary[k])
    )
      throw new AccountError("invalid_response");
  }
  return out;
}
async function get(
  path: string,
  privateRead: boolean,
  signal: AbortSignal,
): Promise<unknown> {
  let r: Response;
  try {
    r = await fetch(path, {
      credentials: privateRead ? "same-origin" : "omit",
      cache: "no-store",
      signal,
    });
  } catch (e) {
    if (signal.aborted) throw e;
    throw new AccountError("unavailable");
  }
  if (!r.ok)
    throw new AccountError(
      r.status === 401
        ? "access_required"
        : r.status === 404
          ? "not_found"
          : "unavailable",
      r.status,
    );
  try {
    return await r.json();
  } catch {
    throw new AccountError("invalid_response");
  }
}
export const insightsAPI = {
  recommendations: async (
    source: string,
    privateRead: boolean,
    signal: AbortSignal,
  ) =>
    decodeSuggestions(
      await get(
        `/api/${privateRead ? "account" : "catalog"}/recommendations${source ? `?source=${encodeURIComponent(source)}` : ""}`,
        privateRead,
        signal,
      ),
    ),
  analytics: async (filters: AnalyticsFilters, signal: AbortSignal) => {
    const v = decodeAnalytics(
      await get(
        `/api/admin/analytics?${new URLSearchParams(filters)}`,
        true,
        signal,
      ),
    );
    if (
      v.meta.filters.event !== filters.event ||
      v.meta.filters.dataset !== filters.dataset
    )
      throw new AccountError("invalid_response");
    return v;
  },
};
// Exact integer formatting: aggregate totals never pass through Number.
export function aggregateMoney(cents: string, lang: string): string {
  const n = BigInt(cents);
  return `${new Intl.NumberFormat(lang === "vi" ? "vi-VN" : "en-US").format(n / 100n)}${lang === "vi" ? "," : "."}${(n % 100n).toString().padStart(2, "0")} USD`;
}
export function aggregateCount(n: string, lang: string): string {
  return new Intl.NumberFormat(lang === "vi" ? "vi-VN" : "en-US").format(
    BigInt(n),
  );
}
export function sellThrough(m: Metrics): string {
  if (m.sell_through === null) return "—";
  const tenths =
    (BigInt(m.sold) * 1000n + BigInt(m.terminal) / 2n) / BigInt(m.terminal);
  return `${tenths / 10n}.${tenths % 10n}% (${m.sold}/${m.terminal})`;
}
