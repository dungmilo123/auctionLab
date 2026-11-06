import { useEffect, useState } from "react";
import type { Category } from "@/api/catalog";
import { insightsAPI, type Suggestions } from "@/api/insights";
import { useAccount } from "@/account/AccountProvider";
import { useInsightsCopy } from "@/lib/insights-copy";
import { startPrivateRead } from "@/routes/account/usePrivateRead";
import { LotGrid, PublicLotCard } from "./DiscoveryPresentation";
import { Button } from "@/components/ui/button";
type Read =
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; value: Suggestions };
export default function RecommendationSection({
  source = "",
  categories = [],
  index,
}: {
  source?: string;
  categories?: Category[];
  index?: string;
}) {
  const { state } = useAccount();
  // Remount immediately on every identity generation, before effects can settle.
  const owner = state.status === "signed-in" ? state.profile.id : state.status;
  return (
    <Recommendations
      key={`${source}:${owner}`}
      source={source}
      categories={categories}
      index={index}
    />
  );
}
function Recommendations({
  source,
  categories,
  index,
}: {
  source: string;
  categories: Category[];
  index?: string;
}) {
  const { state, store } = useAccount(),
    { i } = useInsightsCopy();
  const [pub, setPub] = useState<Read>({ status: "loading" }),
    [priv, setPriv] = useState<Read>({ status: "loading" }),
    [attempt, retry] = useState(0);
  const signed = state.status === "signed-in";
  useEffect(
    () =>
      startPrivateRead(
        (s) => insightsAPI.recommendations(source, false, s),
        (v) => setPub({ status: "ready", value: v }),
        () => setPub({ status: "error" }),
      ),
    [source, attempt],
  );
  useEffect(() => {
    if (!signed) return;
    return startPrivateRead(
      (s) => insightsAPI.recommendations(source, true, s),
      (v) => setPriv({ status: "ready", value: v }),
      (e) => {
        setPriv({ status: "error" });
        if (e.status === 401) void store.revalidate();
      },
    );
  }, [source, signed, attempt, store]);
  const privateReady = signed && priv.status === "ready",
    selected = privateReady ? priv : pub;
  function reload() {
    setPub({ status: "loading" });
    setPriv({ status: "loading" });
    retry((n) => n + 1);
  }
  return (
    <section className="flex flex-col gap-6 border-t border-ink pt-3.5" aria-label={i("suggestions")}>
      <h2 className="flex flex-wrap items-baseline gap-3">
        {index && <span className="mono-label">{index}</span>}
        {i("suggestions")}
      </h2>
      <p className="text-muted-foreground">
        {i(privateReady ? "privateLabel" : "publicFallback")}
      </p>
      {state.status === "unavailable" && (
        <p role="status">
          {i("accountUnavailable")}{" "}
          <Button
            variant="outline"
            onClick={() => void store.revalidate()}
          >
            {i("retry")}
          </Button>
        </p>
      )}
      {signed && priv.status === "error" && (
        <p role="status">
          {i("privateError")}{" "}
          <Button variant="outline" onClick={reload}>
            {i("retry")}
          </Button>
        </p>
      )}
      {selected.status === "loading" ? (
        <p role="status">{i("loading")}</p>
      ) : selected.status === "error" ? (
        <div role="status">
          <p>{i("error")}</p>
          <Button variant="outline" onClick={reload}>
            {i("retry")}
          </Button>
        </div>
      ) : selected.value.data.length === 0 ? (
        <p>{i("noSuggestions")}</p>
      ) : (
        <LotGrid>
          {selected.value.data.map(({ lot, reason }) => (
            <div className="flex flex-col gap-2.5" key={lot.id}>
              <p className="mono-label text-ink">{i(reason)}</p>
              <PublicLotCard
                lot={lot}
                categories={categories}
                now={selected.value.meta.server_time}
              />
            </div>
          ))}
        </LotGrid>
      )}
    </section>
  );
}
