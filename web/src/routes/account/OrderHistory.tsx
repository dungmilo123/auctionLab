import { useCallback } from "react";
import { historyAPI, type HistoryOrder, type Summary } from "@/api/fulfillment";
import { useFulfillmentCopy } from "@/lib/fulfillment-copy";
import { formatOrderMoney } from "@/lib/order-money";
import { usePrivateRead } from "./usePrivateRead";
export function OrderSummary({
  order,
  href,
  label,
}: {
  order: Summary;
  href: string;
  label: string;
}) {
  const { f } = useFulfillmentCopy();
  return (
    <li className="catalog-record stack-small">
      <img
        className="order-history-image"
        src={order.image.path}
        alt={order.image.alt}
      />
      <h2>
        <a href={href}>{order.title}</a>
      </h2>
      {(order.lot_demo || order.event_demo || order.account_demo) && (
        <p>{f("demo")}</p>
      )}
      <p>
        {f(order.status)} · {f(order.method ?? "unselected")}
      </p>
      <p className="numeric">
        {formatOrderMoney(order.winning_amount_minor)}
        {order.total_minor !== null && (
          <>
            {" "}
            · {f("total")}: {formatOrderMoney(order.total_minor)}
          </>
        )}
      </p>
      <a className="site-button site-button-outline" href={href}>
        {label}
      </a>
    </li>
  );
}
export function OrderInspection({
  order,
  admin = false,
}: {
  order: HistoryOrder;
  admin?: boolean;
}) {
  const { f, lang } = useFulfillmentCopy();
  const date = (ms: number) =>
    new Intl.DateTimeFormat(lang === "vi" ? "vi-VN" : "en-US", {
      dateStyle: "medium",
      timeStyle: "long",
    }).format(ms);
  const p = order.fulfillment;
  return (
    <section className="order-inspection stack">
      <h2>{order.snapshot.title}</h2>
      <p>{f("simulation")}</p>
      {(order.snapshot.lot_demo ||
        order.snapshot.event_demo ||
        order.snapshot.account_demo) && <p>{f("demo")}</p>}
      <img
        className="order-history-image"
        src={order.snapshot.image.path}
        alt={order.snapshot.image.alt}
      />
      <p>{order.snapshot.condition}</p>
      <p role="status">{f(p.status)}</p>
      <dl className="order-facts">
        <dt>{f("order")}</dt>
        <dd>{order.id}</dd>
        <dt>{f("winning")}</dt>
        <dd className="numeric">
          {formatOrderMoney(order.winning_amount_minor)}
        </dd>
        <dt>{f("closed")}</dt>
        <dd>
          <time dateTime={new Date(order.closes_at).toISOString()}>
            {date(order.closes_at)}
          </time>
        </dd>
        <dt>{f("result")}</dt>
        <dd>{date(order.recorded_at)}</dd>
        {order.total && (
          <>
            <dt>{f("fee")}</dt>
            <dd>{formatOrderMoney(order.total.fulfillment_fee_minor)}</dd>
            <dt>{f("total")}</dt>
            <dd>{formatOrderMoney(order.total.total_minor)}</dd>
            <dt>{f("instructions")}</dt>
            <dd>{order.total.instructions}</dd>
          </>
        )}
        {order.selection && (
          <>
            <dt>{f("method")}</dt>
            <dd>{f(order.selection.fulfillment_method)}</dd>
            <dt>{f("contact")}</dt>
            <dd>
              <p>{order.selection.contact.name}</p>
              <p>{order.selection.contact.email}</p>
              <p>{order.selection.contact.phone}</p>
              {order.selection.shipping_address && (
                <address>
                  {Object.values(order.selection.shipping_address)
                    .filter(Boolean)
                    .join(", ")}
                </address>
              )}
            </dd>
          </>
        )}
        {order.receipt && (
          <>
            <dt>{f("receiptID")}</dt>
            <dd>{order.receipt.id}</dd>
          </>
        )}
      </dl>
      {p.shipment && (
        <section className="stack-small">
          <h3>{f("shipped")}</h3>
          <p>
            {f("carrier")}: {p.shipment.carrier}
          </p>
          <p>
            {f("tracking")}: {p.shipment.tracking_reference}
          </p>
        </section>
      )}
      <section className="stack-small">
        <h3>{f("timeline")}</h3>
        <ol className="order-timeline">
          {p.timeline.map((e) => (
            <li key={e.version}>
              <strong>{e.version === 0 ? f("created") : f(e.to_status)}</strong>
              <p>
                <time dateTime={new Date(e.occurred_at).toISOString()}>
                  {date(e.occurred_at)}
                </time>{" "}
                · {f(e.source)}
              </p>
              {e.shipment && (
                <p>
                  {e.shipment.carrier} · {e.shipment.tracking_reference}
                </p>
              )}
            </li>
          ))}
        </ol>
      </section>
      {!admin && (
        <a
          className="site-button site-button-outline"
          href={`/account/checkout/${order.id}`}
        >
          {f(order.state === "paid" ? "receipt" : "checkout")}
        </a>
      )}
    </section>
  );
}
export default function OrderHistory({
  owner,
  orderID,
}: {
  owner: string;
  orderID?: string;
}) {
  const { f } = useFulfillmentCopy();
  const load = useCallback(
    (signal: AbortSignal) =>
      orderID
        ? historyAPI.get(owner, orderID, signal)
        : historyAPI.list(owner, signal),
    [owner, orderID],
  );
  const { state, retry } = usePrivateRead<HistoryOrder | Summary[]>(load);
  if (state.status === "loading") return <p role="status">{f("loading")}</p>;
  if (state.status === "error")
    return (
      <section className="empty-state stack">
        <p role="alert">
          {f(state.error.status === 404 ? "missing" : "unavailable")}
        </p>
        <button className="site-button" onClick={retry}>
          {f("retry")}
        </button>
      </section>
    );
  return (
    <section className="stack">
      <a href="/account/orders">{f("history")}</a>
      <button className="site-button site-button-outline" onClick={retry}>
        {f("reload")}
      </button>
      {Array.isArray(state.data) ? (
        <>
          <p>{f("simulation")}</p>
          {state.data.length === 0 ? (
            <p className="empty-state">{f("empty")}</p>
          ) : (
            <ul className="order-history-list">
              {state.data.map((order) => (
                <OrderSummary
                  key={order.id}
                  order={order}
                  href={`/account/orders/${order.id}`}
                  label={f("detail")}
                />
              ))}
            </ul>
          )}
        </>
      ) : (
        <OrderInspection order={state.data} />
      )}
    </section>
  );
}
