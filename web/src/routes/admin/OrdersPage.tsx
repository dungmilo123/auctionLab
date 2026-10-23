import { RefreshCw } from 'lucide-react';
import { Icon } from '@/components/site/Icon';
import { useCallback, useEffect, useRef, useState } from "react";
import { AccountError } from "@/api/account";
import {
  fulfillmentAPI,
  nextStatus,
  parseFilters,
  STATUSES,
  type HistoryOrder,
  type Shipment,
  type Summary,
} from "@/api/fulfillment";
import {
  useFulfillmentCopy,
  type FulfillmentKey,
} from "@/lib/fulfillment-copy";
import { PageHeading } from "@/components/site/Presentation";
import { OrderInspection, OrderSummary } from "@/routes/account/OrderHistory";
import { startPrivateRead } from "@/routes/account/usePrivateRead";

type ReadState = "loading" | "ready" | "error" | "access" | "missing";
type MutationState =
  | "idle"
  | "review"
  | "pending"
  | "unknown"
  | "conflict"
  | "invalid"
  | "saved";
export default function OrdersPage() {
  const parsed = parseFilters(window.location.search);
  return <OrdersWorkspace key={window.location.search} parsed={parsed} />;
}
function OrdersWorkspace({
  parsed,
}: {
  parsed: ReturnType<typeof parseFilters>;
}) {
  const { f } = useFulfillmentCopy();
  const { filters, view, valid } = parsed;
  const { status: statusFilter, method: methodFilter } = filters;
  const [list, setList] = useState<Summary[]>([]),
    [detail, setDetail] = useState<HistoryOrder | null>(null),
    [read, setRead] = useState<ReadState>("loading"),
    [mutation, setMutation] = useState<MutationState>("idle"),
    [shipment, setShipment] = useState<Shipment>({
      carrier: "",
      tracking_reference: "",
    }),
    [fields, setFields] = useState<Record<string, string>>({}),
    [attempt, setAttempt] = useState(0);
  const version = useRef(0),
    pending = useRef(false),
    controller = useRef<AbortController | null>(null);
  const cancelPending = useCallback(() => {
    version.current++;
    controller.current?.abort();
  }, []);
  const clear = useCallback(() => {
    version.current++;
    pending.current = false;
    controller.current?.abort();
    setList([]);
    setDetail(null);
    setShipment({ carrier: "", tracking_reference: "" });
    setFields({});
    setRead("loading");
    setMutation("idle");
  }, []);
  const reload = useCallback(() => {
    version.current++;
    controller.current?.abort();
    pending.current = false;
    setDetail(null);
    setList([]);
    setRead("loading");
    setAttempt((n) => n + 1);
  }, []);
  useEffect(() => {
    const hide = () => clear();
    const show = () => {
      clear();
      setAttempt((n) => n + 1);
    };
    const focus = () => {
      if (!pending.current) {
        setDetail(null);
        setList([]);
        setRead("loading");
        setAttempt((n) => n + 1);
      }
    };
    window.addEventListener("pagehide", hide);
    window.addEventListener("pageshow", show);
    window.addEventListener("focus", focus);
    return () => {
      cancelPending();
      window.removeEventListener("pagehide", hide);
      window.removeEventListener("pageshow", show);
      window.removeEventListener("focus", focus);
    };
  }, [clear, cancelPending]);
  useEffect(() => {
    if (!valid) return;
    const generation = ++version.current;
    return startPrivateRead(
      async (signal) => {
        const records = await fulfillmentAPI.list(
          { status: statusFilter, method: methodFilter },
          signal,
        );
        const selected = view ? await fulfillmentAPI.get(view, signal) : null;
        return { records, selected };
      },
      ({ records, selected }) => {
        if (version.current !== generation) return;
        setList(records);
        setDetail(selected);
        setRead("ready");
        setMutation((current) =>
          current === "unknown" || current === "conflict" ? "idle" : current,
        );
      },
      (e) => {
        if (version.current !== generation) return;
        setDetail(null);
        setList([]);
        if (e.status === 401) {
          clear();
          setRead("access");
        } else setRead(e.status === 404 ? "missing" : "error");
      },
    );
  }, [attempt, statusFilter, methodFilter, view, valid, clear]);
  async function advance() {
    if (
      pending.current ||
      !detail ||
      mutation === "unknown" ||
      mutation === "conflict"
    )
      return;
    const next = nextStatus(
      detail.selection?.fulfillment_method ?? null,
      detail.fulfillment.status,
    );
    if (!next) return;
    if (next === "shipped" && mutation !== "review") {
      setMutation("review");
      return;
    }
    const generation = version.current;
    const c = new AbortController();
    controller.current = c;
    pending.current = true;
    setMutation("pending");
    setFields({});
    try {
      const saved = await fulfillmentAPI.transition(
        detail.id,
        detail.fulfillment.version,
        next,
        next === "shipped" ? shipment : undefined,
        c.signal,
      );
      if (version.current !== generation) return;
      setDetail(saved);
      setShipment({ carrier: "", tracking_reference: "" });
      setMutation("saved");
      setList((records) =>
        records
          .map((row) =>
            row.id === saved.id
              ? {
                  ...row,
                  status: saved.fulfillment.status,
                  version: saved.fulfillment.version,
                }
              : row,
          )
          .filter(
            (row) =>
              (statusFilter === "all" || row.status === statusFilter) &&
              (methodFilter === "all" ||
                (row.method ?? "unselected") === methodFilter),
          ),
      );
    } catch (e) {
      if (version.current !== generation) return;
      const error =
        e instanceof AccountError ? e : new AccountError("commit_unknown");
      if (error.status === 401) {
        clear();
        setRead("access");
      } else if (error.status === 422 || error.status === 400) {
        setFields(error.fields);
        setMutation("invalid");
      } else if (error.status === 409) setMutation("conflict");
      else setMutation("unknown");
    } finally {
      if (version.current === generation) pending.current = false;
    }
  }
  const href = (id: string) =>
    `/admin/orders?${new URLSearchParams({ ...filters, view: id })}`;
  const next = detail
    ? nextStatus(
        detail.selection?.fulfillment_method ?? null,
        detail.fulfillment.status,
      )
    : undefined;
  const action: FulfillmentKey =
    next === "preparing"
      ? "startPreparing"
      : next === "shipped"
        ? "markShipped"
        : next === "ready_for_pickup"
          ? "markReady"
          : "markCompleted";
  return (
    <div className="order-workspace stack">
      <PageHeading title={f("history")} />
      {!valid ? (
        <section className="empty-state stack">
          <p role="alert">{f("invalidQuery")}</p>
          <a href="/admin/orders">{f("resetFilters")}</a>
        </section>
      ) : (
        <>
          <form className="order-filters" action="/admin/orders" method="GET">
            <label>
              {f("status")}
              <select name="status" defaultValue={filters.status}>
                {["all", ...STATUSES].map((s) => (
                  <option key={s} value={s}>
                    {f(s as FulfillmentKey)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {f("method")}
              <select name="method" defaultValue={filters.method}>
                {["all", "shipping", "pickup", "unselected"].map((s) => (
                  <option key={s} value={s}>
                    {f(s as FulfillmentKey)}
                  </option>
                ))}
              </select>
            </label>
            <button className="site-button" type="submit">
              {f("filter")}
            </button>
          </form>
          <button
            className="site-button site-button-outline"
            onClick={reload}
            disabled={mutation === "pending"}
          >
            <Icon icon={RefreshCw} />{f("reload")}
          </button>
          {mutation !== "idle" && mutation !== "review" && (
            <p
              className={mutation === "saved" ? "confirmed-feedback" : undefined}
              role={
                mutation === "saved" || mutation === "pending"
                  ? "status"
                  : "alert"
              }
            >
              {f(mutation)}
            </p>
          )}

          {read === "loading" ? (
            <p role="status">{f("loading")}</p>
          ) : read === "access" ? (
            <section className="empty-state stack">
              <p role="alert">{f("access")}</p>
              <a href="/admin">{f("gate")}</a>
            </section>
          ) : read === "error" || read === "missing" ? (
            <p role="alert">
              {f(read === "missing" ? "missing" : "unavailable")}
            </p>
          ) : (
            <>
              {detail && (
                <section className="catalog-record stack">
                  <OrderInspection order={detail} admin />
                  {(mutation === "unknown" || mutation === "conflict") && (
                    <button className="site-button" onClick={reload}>
                      {f("retry")}
                    </button>
                  )}
                  {next && (
                    <form
                      className="stack"
                      onSubmit={(e) => {
                        e.preventDefault();
                        void advance();
                      }}
                    >
                      {next === "shipped" && (
                        <fieldset
                          className="stack"
                          disabled={
                            mutation === "pending" ||
                            mutation === "unknown" ||
                            mutation === "conflict"
                          }
                        >
                          <legend>{f("reviewHint")}</legend>
                          {(["carrier", "tracking_reference"] as const).map(
                            (key) => (
                              <label
                                className="stack-small"
                                key={key}
                                htmlFor={`shipment-${key}`}
                              >
                                {f(key === "carrier" ? "carrier" : "tracking")}
                                <input
                                  id={`shipment-${key}`}
                                  value={shipment[key]}
                                  required
                                  readOnly={mutation === "review"}
                                  aria-invalid={
                                    fields[`shipment.${key}`]
                                      ? "true"
                                      : undefined
                                  }
                                  aria-describedby={
                                    fields[`shipment.${key}`]
                                      ? `error-${key}`
                                      : undefined
                                  }
                                  onChange={(e) => {
                                    setShipment((s) => ({
                                      ...s,
                                      [key]: e.target.value,
                                    }));
                                    setMutation("idle");
                                  }}
                                />
                                {fields[`shipment.${key}`] && (
                                  <span role="alert" id={`error-${key}`}>
                                    {f("invalid")}
                                  </span>
                                )}
                              </label>
                            ),
                          )}
                          {mutation === "review" && (
                            <button
                              className="site-button site-button-outline"
                              type="button"
                              onClick={() => setMutation("idle")}
                            >
                              {f("edit")}
                            </button>
                          )}
                        </fieldset>
                      )}
                      <button
                        className="site-button site-button-primary"
                        disabled={["pending", "unknown", "conflict"].includes(
                          mutation,
                        )}
                        type="submit"
                      >
                        {f(
                          next === "shipped" && mutation !== "review"
                            ? "review"
                            : action,
                        )}
                      </button>
                    </form>
                  )}
                </section>
              )}
              {list.length === 0 ? (
                <p className="empty-state">{f("empty")}</p>
              ) : (
                <ul className="order-history-list">
                  {list.map((order) => (
                    <OrderSummary
                      key={order.id}
                      order={order}
                      href={href(order.id)}
                      label={f("select")}
                    />
                  ))}
                </ul>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
