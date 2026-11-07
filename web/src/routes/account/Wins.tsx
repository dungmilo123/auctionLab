import { displayOrder } from '@/lib/seed-display';
import { useCallback } from "react";
import { orderAPI } from "@/api/order";
import { useOrderCopy } from "@/lib/order-copy";
import { formatOrderMoney } from "@/lib/order-money";
import { usePrivateRead } from "./usePrivateRead";
import { Button } from "@/components/ui/button";
import { LotImage } from "@/components/site/Presentation";
export default function Wins({ owner }: { owner: string }) {
  const { o, lang } = useOrderCopy();
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
        <>
          <ul className="grid gap-4">
            {state.data.map(displayOrder).map((v) => (
              <li className="grid grid-cols-[minmax(0,9rem)_minmax(0,1fr)] items-start gap-5 border border-hairline bg-paper p-5 max-sm:grid-cols-1" key={v.id}>
                <a href={`/lots/${encodeURIComponent(v.snapshot.slug)}`} tabIndex={-1} aria-hidden="true" className="block border border-hairline bg-secondary">
                  <LotImage src={v.snapshot.image.path} alt={v.snapshot.image.alt} plain />
                </a>
                <div className="flex min-w-0 flex-col gap-2.5">
                  <h2>
                    <a href={`/lots/${encodeURIComponent(v.snapshot.slug)}`}>
                      {v.snapshot.title}
                    </a>
                  </h2>
                  <p className="text-muted-foreground">{v.snapshot.condition}</p>
                  <strong className="font-mono text-lg numeric">
                    {formatOrderMoney(v.winning_amount_minor, lang)}
                  </strong>
                  <Button asChild className="self-start">
                    <a href={`/account/checkout/${v.id}`}>{o("open")}</a>
                  </Button>
                </div>
              </li>
            ))}
          </ul>
          <p className="flex items-baseline justify-between gap-4 border-t-2 border-ink pt-3">
            <span className="mono-label">{o("subtotal")} · {state.data.length}</span>
            <strong className="font-mono text-lg numeric">
              {formatOrderMoney(state.data.reduce((sum, v) => sum + v.winning_amount_minor, 0), lang)}
            </strong>
          </p>
        </>
      )}
      <p><a href={"/account/orders"}>{o("history")}</a></p>
    </section>
  );
}
