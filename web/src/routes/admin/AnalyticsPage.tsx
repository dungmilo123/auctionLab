import { BarChart3, SlidersHorizontal } from 'lucide-react';
import { Icon } from '@/components/site/Icon';
import { useEffect, useState, useRef } from "react";
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
import { PageHeading } from "@/components/site/Presentation";
import { startPrivateRead } from "@/routes/account/usePrivateRead";
type Read =
  | { status: "loading" | "error" | "access" }
  | { status: "ready"; value: Analytics };
export default function AnalyticsPage() {
  return <AnalyticsWorkspace key={window.location.search} />;
}
function AnalyticsWorkspace() {
  const { i, lang } = useInsightsCopy(),
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
  const metric = (m: Metrics, k: MetricKey) =>
    k.endsWith("_minor")
      ? aggregateMoney(m[k], lang)
      : aggregateCount(m[k], lang);
  const refresh = () => {
    setRead({ status: "loading" });
    retry((n) => n + 1);
  };
  return (
    <>
      <PageHeading title={i("analytics")} intro={i("intro")} />
      <form
        method="get"
        action="/admin/analytics"
        className="analytics-controls"
      >
        <label>
          {i("event")}
          <input
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
        <label>
          {i("dataset")}
          <select name="dataset" defaultValue={dataset}>
            <option value="all">{i("all")}</option>
            <option value="demo">{i("demo")}</option>
            <option value="non-demo">{i("nonDemo")}</option>
          </select>
        </label>
        <button className="site-button site-button-primary">
          <Icon icon={SlidersHorizontal} />{i("apply")}
        </button>
      </form>
      {!filters ? (
        <p role="alert">{i("error")}</p>
      ) : read.status !== "ready" ? (
        <div role="status">
          <p>
            {i(
              read.status === "access"
                ? "access"
                : read.status === "error"
                  ? "error"
                  : "loading",
            )}
          </p>
          {read.status !== "loading" && (
            <button
              className="site-button site-button-outline"
              onClick={refresh}
            >
              {i("retry")}
            </button>
          )}
        </div>
      ) : (
        <>
          <p>
            {i("asOf")}:{" "}
            <time dateTime={new Date(read.value.meta.as_of).toISOString()}>
              {new Date(read.value.meta.as_of).toLocaleString(lang)}
            </time>
          </p>
          <p>
            {i("coverage")}:{" "}
            {read.value.meta.collection_started_at === null
              ? i("unknown")
              : new Date(read.value.meta.collection_started_at).toLocaleString(
                  lang,
                )}
          </p>
          <button className="site-button site-button-outline" onClick={refresh}>
            {i("refresh")}
          </button>
          {read.value.data.summary.lots === "0" && (
            <p role="status">{i("empty")}</p>
          )}
          <div className="analytics-section-title"><Icon icon={BarChart3} /><span>{i("analytics")}</span></div>
          <dl className="analytics-summary">
            {metricKeys.map((k) => (
              <div key={k} data-metric={k}>
                <dt>{i(k)}</dt>
                <dd className="numeric">
                  {metric(read.value.data.summary, k)}
                </dd>
              </div>
            ))}
            <div>
              <dt>{i("sell_through")}</dt>
              <dd>{sellThrough(read.value.data.summary)}</dd>
            </div>
          </dl>
          <div
            className="analytics-table-scroll"
            role="region"
            aria-label={i("compare")}
            tabIndex={0}
          >
            <table className="analytics-table">
              <caption>{i("compare")}</caption>
              <thead>
                <tr>
                  <th scope="col">{i("event")}</th>
                  {metricKeys.map((k) => (
                    <th scope="col" key={k}>
                      {i(k)}
                    </th>
                  ))}
                  <th scope="col">{i("sell_through")}</th>
                </tr>
              </thead>
              <tbody>
                {read.value.data.events.map((e) => (
                  <tr key={e.slug}>
                    <th scope="row">
                      <a href={`/auctions/${encodeURIComponent(e.slug)}`}>
                        {e.title}
                      </a>
                      {e.demo && (
                        <span className="demo-marker">{i("demo")}</span>
                      )}
                    </th>
                    {metricKeys.map((k) => (
                      <td className="numeric" key={k}>
                        {metric(e.metrics, k)}
                      </td>
                    ))}
                    <td>{sellThrough(e.metrics)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <section className="stack-small">
            <h2>{i("interactions")}</h2>
            <p>{i("observations")}</p>
            {read.value.data.interactions.length ? (
              <dl className="metadata">
                {read.value.data.interactions.map((n) => (
                  <div key={`${n.kind}:${n.provenance}`}>
                    <dt>
                      {i(n.kind as InsightsKey)} ·{" "}
                      {i(n.provenance as InsightsKey)}
                    </dt>
                    <dd>{aggregateCount(n.count, lang)}</dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p>{aggregateCount("0", lang)}</p>
            )}
          </section>
        </>
      )}
      <section className="stack-small">
        <h2>{i("definitions")}</h2>
        <p>{i("cohort")}</p>
        <p>{i("formula")}</p>
        <p>{i("moneyDefinition")}</p>
        <p>{i("observations")}</p>
      </section>
    </>
  );
}
