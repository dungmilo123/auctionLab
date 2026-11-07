import { CheckCircle2, CreditCard, MapPin, Package, Truck } from 'lucide-react';
import { Icon } from '@/components/site/Icon';
import { LotImage } from '@/components/site/Presentation';
import { displayOrder, displaySummary } from '@/lib/seed-display';
import { useCallback } from "react";
import { historyAPI, type HistoryOrder, type Summary } from "@/api/fulfillment";
import { useFulfillmentCopy } from "@/lib/fulfillment-copy";
import { formatOrderMoney } from "@/lib/order-money";
import { usePrivateRead } from "./usePrivateRead";
import { Button } from "@/components/ui/button";
export function OrderSummary({
  order: rawOrder,
  href,
  label,
}: {
  order: Summary;
  href: string;
  label: string;
}) {
  const { f, lang } = useFulfillmentCopy();
  const order = displaySummary(rawOrder);
  return (
    <li className="flex flex-col gap-2.5 rounded-[2px] border border-hairline bg-paper p-5">
      <LotImage src={order.image.path} alt={order.image.alt} />
      <h2>
        <a href={href}>{order.title}</a>
      </h2>
      <p>
        {f(order.status)} · {f(order.method ?? "unselected")}
      </p>
      <p className="font-mono numeric">
        {formatOrderMoney(order.winning_amount_minor, lang)}
        {order.total_minor !== null && (
          <>
            {" "}
            · {f("total")}: {formatOrderMoney(order.total_minor, lang)}
          </>
        )}
      </p>
      <Button asChild variant="outline" className="self-start">
        <a href={href}>{label}</a>
      </Button>
    </li>
  );
}
export function OrderInspection({
  order: rawOrder,
  admin = false,
}: {
  order: HistoryOrder;
  admin?: boolean;
}) {
  const { f, lang } = useFulfillmentCopy();
  const order = displayOrder(rawOrder);
  const date = (ms: number) =>
    new Intl.DateTimeFormat(lang === "vi" ? "vi-VN" : "en-US", {
      dateStyle: "medium",
      timeStyle: "long",
    }).format(ms);
  const p = order.fulfillment;
  return (
    <section className="flex flex-col gap-6">
      <h2>{order.snapshot.title}</h2>
      <img
        className="block h-48 w-full max-w-80 object-contain"
        src={order.snapshot.image.path}
        alt={order.snapshot.image.alt}
      />
      <p>{order.snapshot.condition}</p>
      <p role="status">{f(p.status)}</p>
      <dl className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-3 max-[480px]:grid-cols-1 [&>dd]:m-0 [&>dt]:font-semibold">
        <dt>{f("order")}</dt>
        <dd>{order.id}</dd>
        <dt>{f("winning")}</dt>
        <dd className="numeric">
          {formatOrderMoney(order.winning_amount_minor, lang)}
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
            <dd>{formatOrderMoney(order.total.fulfillment_fee_minor, lang)}</dd>
            <dt>{f("total")}</dt>
            <dd>{formatOrderMoney(order.total.total_minor, lang)}</dd>
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
        <section className="flex flex-col gap-2.5">
          <h3>{f("shipped")}</h3>
          <p>
            {f("carrier")}: {p.shipment.carrier}
          </p>
          <p>
            {f("tracking")}: {p.shipment.tracking_reference}
          </p>
        </section>
      )}
      <section className="flex flex-col gap-2.5">
        <h3>{f("timeline")}</h3>
        <ol className="m-0 list-none p-0">
          {p.timeline.map((e) => (
            <li key={e.version} className="relative ms-3 border-s border-hairline pb-6 ps-10 [&>svg]:absolute [&>svg]:-start-3 [&>svg]:top-0 [&>svg]:bg-background [&>svg]:text-[#3F4B43]">
<Icon icon={e.to_status === "completed" ? CheckCircle2 : e.to_status === "shipped" ? Truck : e.to_status === "ready_for_pickup" ? MapPin : e.to_status === "paid" ? CreditCard : Package} />
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
        <Button asChild variant="outline" className="self-start">
          <a href={`/account/checkout/${order.id}`}>
            {f(order.state === "paid" ? "receipt" : "checkout")}
          </a>
        </Button>
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
      <section className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8">
        <p role="alert">
          {f(state.error.status === 404 ? "missing" : "unavailable")}
        </p>
        <Button variant="outline" className="self-start" onClick={retry}>
          {f("retry")}
        </Button>
      </section>
    );
  return (
    <section className="flex flex-col gap-6">
      <a href="/account/orders" className="self-start">{f("history")}</a>
      <Button variant="outline" className="self-start" onClick={retry}>
        {f("reload")}
      </Button>
      {Array.isArray(state.data) ? (
        <>
              {state.data.length === 0 ? (
            <p className="border border-hairline border-t-ink bg-paper px-6 py-8 text-muted-foreground">{f("empty")}</p>
          ) : (
            <ul className="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(min(100%,20rem),1fr))] gap-6 p-0">
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
