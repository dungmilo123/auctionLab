import { useEffect, useRef, useState, type FormEvent } from "react";
import { startPrivateRead } from "./usePrivateRead";
import { CheckoutSession } from "./checkout-session";
import { AccountError } from "@/api/account";
import {
  orderAPI,
  type Order,
  type Selection,
  type Review,
  type Total,
} from "@/api/order";
import { useAccount } from "@/account/AccountProvider";
import { useOrderCopy, type OrderKey } from "@/lib/order-copy";
import { formatOrderMoney } from "@/lib/order-money";
const blankAddress = {
  line1: "",
  line2: "",
  locality: "",
  region: "",
  postal_code: "",
  country: "",
};
const blank: Selection = {
  contact: { name: "", email: "", phone: "" },
  fulfillment_method: "shipping",
  shipping_address: blankAddress,
  payment_method: "demo-card",
};
type Stage =
  | "loading"
  | "reviewing"
  | "edit"
  | "review"
  | "pending"
  | "unknown"
  | "paid"
  | "error";
function Breakdown({ total }: { total: Total }) {
  const { o } = useOrderCopy();
  return (
    <dl className="metadata numeric">
      {(
        [
          "winning_amount_minor",
          "fulfillment_fee_minor",
          "total_minor",
        ] as const
      ).map((key, i) => (
        <div key={key}>
          <dt>{o((["item", "fee", "total"] as const)[i]!)}</dt>
          <dd>{formatOrderMoney(total[key])}</dd>
        </div>
      ))}
    </dl>
  );
}
function Field({
  fieldKey,
  label,
  value,
  onChange,
  maxLength,
  required = true,
  type = "text",
  error,
}: {
  fieldKey: string;
  label: OrderKey;
  value: string;
  onChange: (v: string) => void;
  maxLength: number;
  required?: boolean;
  type?: string;
  error?: string;
}) {
  const { o } = useOrderCopy();
  return (
    <div className="stack-small">
      <label htmlFor={fieldKey}>{o(label)}</label>
      <input
        id={fieldKey}
        type={type}
        value={value}
        maxLength={maxLength * 2}
        required={required}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={!!error}
        aria-describedby={error ? `${fieldKey}-error` : undefined}
      />
      {error && (
        <p id={`${fieldKey}-error`} role="alert">
          {o(error === "required" ? "required" : "invalid")}
        </p>
      )}
    </div>
  );
}
export default function Checkout({
  owner,
  orderID,
}: {
  owner: string;
  orderID: string;
}) {
  const { o, lang } = useOrderCopy();
  const { store } = useAccount();
  const [stage, setStage] = useState<Stage>("loading");
  const [purchase, setPurchase] = useState<Order | null>(null);
  const [form, setForm] = useState<Selection>(blank);
  const [review, setReview] = useState<Review | null>(null);
  const [error, setError] = useState<AccountError | null>(null);
  const [session] = useState(
    () =>
      new CheckoutSession(() => {
        const s = store.get();
        return s.status === "signed-in" && s.profile.id === owner;
      }),
  );
  const heading = useRef<HTMLHeadingElement>(null);
  function failure(e: unknown, next: Stage) {
    const err = e instanceof AccountError ? e : new AccountError("unavailable");
    if (err.status === 401) {
      store.signedOut();
      return;
    }
    setError(err);
    setStage(next);
  }
  async function recover(initial = false) {
    const operation = session.begin();
    if (!operation) return;
    try {
      const v = await orderAPI.get(owner, orderID, operation.signal);
      if (!operation.current()) return;
      setError(null);
      setPurchase(v);
      if (v.state === "paid") {
        setStage("paid");
        session.resetIntent();
        setReview(null);
        setForm(blank);
      } else if (session.hasIntent() && review) {
        setStage("review");
      } else {
        setStage("edit");
        if (initial)
          setForm({
            ...blank,
            fulfillment_method: v.options[0]!.fulfillment_method,
          });
      }
    } catch (e) {
      if (operation.current()) failure(e, initial ? "error" : "unknown");
    } finally {
      operation.finish();
    }
  }
  useEffect(() => {
    session.open();
    const stop = startPrivateRead(
      (signal) => orderAPI.get(owner, orderID, signal),
      (v) => {
        setPurchase(v);
        setStage(v.state === "paid" ? "paid" : "edit");
        if (v.state !== "paid")
          setForm({
            ...blank,
            fulfillment_method: v.options[0]!.fulfillment_method,
          });
      },
      (e) => {
        if (e.status === 401) store.signedOut();
        else {
          setError(e);
          setStage("error");
        }
      },
    );
    return () => {
      stop();
      session.close();
    };
  }, [session, owner, orderID, store]);
  useEffect(() => {
    if (stage === "review" || stage === "paid" || stage === "unknown" || error)
      heading.current?.focus();
  }, [stage, error]);
  function update(next: Selection) {
    setForm(next);
    setReview(null);
    session.resetIntent();
    setError(null);
  }
  async function inspect(e: FormEvent) {
    e.preventDefault();
    const operation = session.begin();
    if (!operation) return;
    setError(null);
    setStage("reviewing");
    try {
      const v = await orderAPI.review(owner, orderID, form, operation.signal);
      if (!operation.current()) return;
      setPurchase(v.order);
      if (v.order.state === "paid") {
        setStage("paid");
        setForm(blank);
      } else {
        setReview(v);
        setForm(v.selection);
        session.resetIntent();
        setStage("review");
      }
    } catch (e) {
      if (operation.current()) failure(e, "edit");
    } finally {
      operation.finish();
    }
  }
  async function confirm() {
    if (!review || stage !== "review") return;
    const operation = session.begin();
    if (!operation) return;
    setStage("pending");
    setError(null);
    try {
      const accepted = session.acceptedIntent(review.selection);
      const result = await orderAPI.confirm(
        owner,
        orderID,
        accepted,
        operation.signal,
      );
      if (!operation.current()) return;
      setPurchase(result.order);
      setStage("paid");
      setForm(blank);
      setReview(null);
      session.resetIntent();
    } catch (e) {
      if (!operation.current()) return;
      const err =
        e instanceof AccountError ? e : new AccountError("unavailable");
      failure(err, err.status === 422 ? "edit" : "unknown");
      if (err.status === 422) session.resetIntent();
    } finally {
      operation.finish();
    }
  }
  const total =
    purchase?.state === "paid"
      ? purchase.total
      : (review?.total ??
        purchase?.options.find(
          (t) => t.fulfillment_method === form.fulfillment_method,
        ));
  const selection =
    purchase?.state === "paid" ? purchase.selection : review?.selection;

  return (
    <section
      className="checkout stack"
      aria-busy={stage === "pending" || stage === "loading"}
    >
      <p className="preview-notice">{o("simulation")}</p>
      <h2 ref={heading} tabIndex={-1}>
        {o(
          stage === "paid"
            ? "paid"
            : stage === "review"
              ? "reviewTitle"
              : "checkout",
        )}
      </h2>
      {stage === "loading" && <p role="status">{o("loading")}</p>}
      {error && (
        <p role="alert">
          {o(
            error.status === 404
              ? "missing"
              : error.status === 422
                ? "validation"
                : error.status === 409
                  ? "conflict"
                  : "unavailable",
          )}
        </p>
      )}
      {stage === "error" && error?.status !== 404 && (
        <button className="site-button" onClick={() => void recover(true)}>
          {o("retry")}
        </button>
      )}
      {purchase && (
        <>
          <div className="checkout-snapshot">
            <img
              src={purchase.snapshot.image.path}
              alt={purchase.snapshot.image.alt}
            />
            <div>
              <h3>{purchase.snapshot.title}</h3>
              <p>
                {o("condition")}: {purchase.snapshot.condition}
              </p>
              {purchase.snapshot.lot_demo && (
                <p className="status-badge">{o("demo")}</p>
              )}
              <p>
                {o("order")}: <span className="numeric">{purchase.id}</span>
              </p>
            </div>
          </div>
          {(stage === "edit" || stage === "reviewing") && (
            <form className="stack" onSubmit={(e) => void inspect(e)}>
              <fieldset className="stack" disabled={stage === "reviewing"}>
                <legend>{o("checkout")}</legend>
                {(["name", "email", "phone"] as const).map((k) => (
                  <Field
                    key={k}
                    fieldKey={`contact.${k}`}
                    label={k}
                    value={form.contact[k]}
                    onChange={(v) =>
                      update({ ...form, contact: { ...form.contact, [k]: v } })
                    }
                    maxLength={k === "name" ? 120 : k === "email" ? 254 : 40}
                    type={
                      k === "email" ? "email" : k === "phone" ? "tel" : "text"
                    }
                    error={error?.fields[`contact.${k}`]}
                  />
                ))}
                <label htmlFor="fulfillment_method">{o("fulfillment")}</label>
                <select
                  id="fulfillment_method"
                  value={form.fulfillment_method}
                  onChange={(e) =>
                    update({
                      ...form,
                      fulfillment_method: e.target
                        .value as Selection["fulfillment_method"],
                      shipping_address:
                        e.target.value === "pickup"
                          ? undefined
                          : { ...blankAddress },
                    })
                  }
                >
                  {purchase.options.map((t) => (
                    <option
                      key={t.fulfillment_method}
                      value={t.fulfillment_method}
                    >
                      {o(t.fulfillment_method)}
                    </option>
                  ))}
                </select>
                {form.fulfillment_method === "shipping" &&
                  (
                    [
                      "line1",
                      "line2",
                      "locality",
                      "region",
                      "postal_code",
                      "country",
                    ] as const
                  ).map((k) => (
                    <Field
                      key={k}
                      fieldKey={`shipping_address.${k}`}
                      label={k}
                      value={form.shipping_address?.[k] ?? ""}
                      onChange={(v) =>
                        update({
                          ...form,
                          shipping_address: {
                            ...(form.shipping_address ?? blankAddress),
                            [k]: v,
                          },
                        })
                      }
                      maxLength={
                        k === "country"
                          ? 2
                          : k === "postal_code"
                            ? 32
                            : k === "region" || k === "locality"
                              ? 120
                              : 200
                      }
                      required={["line1", "locality", "country"].includes(k)}
                      error={error?.fields[`shipping_address.${k}`]}
                    />
                  ))}
                <label htmlFor="payment_method">{o("payment")}</label>
                <select
                  id="payment_method"
                  value={form.payment_method}
                  onChange={(e) =>
                    update({
                      ...form,
                      payment_method: e.target
                        .value as Selection["payment_method"],
                    })
                  }
                >
                  {(["demo-card", "demo-bank-transfer"] as const).map((m) => (
                    <option key={m} value={m}>
                      {o(m)}
                    </option>
                  ))}
                </select>
              </fieldset>
              {total && (
                <>
                  <p>
                    {o("instructions")}: {total.instructions}
                  </p>
                  <Breakdown total={total} />
                </>
              )}
              <button
                className="site-button site-button-primary"
                type="submit"
                disabled={stage === "reviewing"}
              >
                {o("review")}
              </button>
            </form>
          )}
          {(stage === "review" ||
            stage === "pending" ||
            stage === "unknown" ||
            stage === "paid") && (
            <>
              {selection && (
                <div className="stack-small">
                  <h3>
                    {o("fulfillment")}: {o(selection.fulfillment_method)}
                  </h3>
                  <p>
                    {selection.contact.name} · {selection.contact.email} ·{" "}
                    {selection.contact.phone}
                  </p>
                  {selection.shipping_address && (
                    <address>
                      {Object.values(selection.shipping_address)
                        .filter(Boolean)
                        .join(", ")}
                    </address>
                  )}
                  <p>{o(selection.payment_method)}</p>
                </div>
              )}
              {total && (
                <>
                  <p>
                    {o("instructions")}: {total.instructions}
                  </p>
                  <Breakdown total={total} />
                </>
              )}
              {stage === "review" && (
                <div className="actions">
                  <button
                    className="site-button site-button-outline"
                    onClick={() => {
                      setStage("edit");
                      setReview(null);
                      session.resetIntent();
                    }}
                  >
                    {o("edit")}
                  </button>
                  <button
                    className="site-button site-button-primary"
                    onClick={() => void confirm()}
                  >
                    {o("confirm")}
                  </button>
                </div>
              )}
              {stage === "pending" && <p role="status">{o("pending")}</p>}
              {stage === "unknown" && (
                <>
                  <p role="alert">{o("unknown")}</p>
                  <button
                    className="site-button"
                    onClick={() => void recover()}
                  >
                    {o("retry")}
                  </button>
                </>
              )}
              {stage === "paid" && purchase.receipt && (
                <dl className="metadata">
                  <div>
                    <dt>{o("receipt")}</dt>
                    <dd>{purchase.receipt.id}</dd>
                  </div>
                  <div>
                    <dt>{o("paidAt")}</dt>
                    <dd>
                      <time
                        dateTime={new Date(
                          purchase.receipt.paid_at,
                        ).toISOString()}
                      >
                        {new Intl.DateTimeFormat(
                          lang === "vi" ? "vi-VN" : "en-US",
                          { dateStyle: "medium", timeStyle: "short" },
                        ).format(purchase.receipt.paid_at)}
                      </time>
                    </dd>
                  </div>
                </dl>
              )}
            </>
          )}
        </>
      )}
      <p>{o("history")}</p>
      <a href="/account/wins">{o("wins")}</a>
    </section>
  );
}
