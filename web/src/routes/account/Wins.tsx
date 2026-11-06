import { displayOrder } from '@/lib/seed-display';
import { useCallback } from "react";
import { orderAPI } from "@/api/order";
import { useOrderCopy } from "@/lib/order-copy";
import { formatOrderMoney } from "@/lib/order-money";
import { usePrivateRead } from "./usePrivateRead";
import { Button } from "@/components/ui/button";
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
      <section className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8">
        <p role="alert">{o("unavailable")}</p>
        <Button variant="outline" className="self-start" onClick={retry}>
          {o("retry")}
        </Button>
      </section>
    );
  return (
    <section className="flex flex-col gap-6">
      {state.data.length === 0 ? (
        <p className="border border-hairline border-t-ink bg-paper px-6 py-8 text-muted-foreground">{o("empty")}</p>
      ) : (
        <ul className="grid gap-4">
          {state.data.map(displayOrder).map((v) => (
            <li className="flex flex-col gap-2.5 border border-hairline bg-paper p-5" key={v.id}>
              <h2>
                <a href={`/lots/${encodeURIComponent(v.snapshot.slug)}`}>
                  {v.snapshot.title}
                </a>
              </h2>
              <p>{v.snapshot.condition}</p>
              <strong className="font-mono text-lg numeric">
                {formatOrderMoney(v.winning_amount_minor)}
              </strong>
              <Button asChild className="self-start">
                <a href={`/account/checkout/${v.id}`}>{o("open")}</a>
              </Button>
            </li>
          ))}
        </ul>
      )}
      <p><a href={"/account/orders"}>{o("history")}</a></p>
    </section>
  );
}
