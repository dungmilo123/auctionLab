import { useCallback } from "react";
import { orderAPI } from "@/api/order";
import { useOrderCopy } from "@/lib/order-copy";
import { formatOrderMoney } from "@/lib/order-money";
import { usePrivateRead } from "./usePrivateRead";
export default function Wins({ owner }: { owner: string }) {
  const { o } = useOrderCopy();
  const load = useCallback(
    (signal: AbortSignal) => orderAPI.wins(owner, signal),
    [owner],
  );
  const { state, retry } = usePrivateRead(load);
  if (state.status === "loading") return <p role="status">{o("loading")}</p>;
  if (state.status === "error")
    return (
      <section className="empty-state stack">
        <p role="alert">{o("unavailable")}</p>
        <button className="site-button" onClick={retry}>
          {o("retry")}
        </button>
      </section>
    );
  return (
    <section className="stack">
      <p>{o("simulation")}</p>
      {state.data.length === 0 ? (
        <p className="empty-state">{o("empty")}</p>
      ) : (
        <ul className="auction-participation">
          {state.data.map((v) => (
            <li className="catalog-record stack-small" key={v.id}>
              <h2>
                <a href={`/lots/${encodeURIComponent(v.snapshot.slug)}`}>
                  {v.snapshot.title}
                </a>
              </h2>
              <p>{v.snapshot.condition}</p>
              <strong className="numeric">
                {formatOrderMoney(v.winning_amount_minor)}
              </strong>
              <a
                className="site-button site-button-primary"
                href={`/account/checkout/${v.id}`}
              >
                {o("open")}
              </a>
            </li>
          ))}
        </ul>
      )}
      <p><a href={"/account/orders"}>{o("history")}</a></p>
    </section>
  );
}
