import { useEffect, useState } from "react";
import type { Category } from "@/api/catalog";
import { insightsAPI, type Suggestions } from "@/api/insights";
import { useAccount } from "@/account/AccountProvider";
import { useInsightsCopy } from "@/lib/insights-copy";
import { startPrivateRead } from "@/routes/account/usePrivateRead";
import { PublicLotCard } from "./DiscoveryPresentation";
type Read =
  | { status: "loading" }
  | { status: "error" }
  | { status: "ready"; value: Suggestions };
export default function RecommendationSection({
  source = "",
  categories = [],
}: {
  source?: string;
  categories?: Category[];
}) {
  const { state } = useAccount();
  // Remount immediately on every identity generation, before effects can settle.
  const owner = state.status === "signed-in" ? state.profile.id : state.status;
  return (
    <Recommendations
      key={`${source}:${owner}`}
      source={source}
      categories={categories}
    />
  );
}
function Recommendations({
  source,
  categories,
}: {
  source: string;
  categories: Category[];
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
    <section className="stack recommendations" aria-label={i("suggestions")}>
      <h2>{i("suggestions")}</h2>
      <p>{i("rules")}</p>
      <p className="muted">
        {i(privateReady ? "privateLabel" : "publicFallback")}
      </p>
      {state.status === "unavailable" && (
        <p role="status">
          {i("accountUnavailable")}{" "}
          <button
            className="site-button site-button-outline"
            onClick={() => void store.revalidate()}
          >
            {i("retry")}
          </button>
        </p>
      )}
      {signed && priv.status === "error" && (
        <p role="status">
          {i("privateError")}{" "}
          <button className="site-button site-button-outline" onClick={reload}>
            {i("retry")}
          </button>
        </p>
      )}
      {selected.status === "loading" ? (
        <p role="status">{i("loading")}</p>
      ) : selected.status === "error" ? (
        <div role="status">
          <p>{i("error")}</p>
          <button className="site-button site-button-outline" onClick={reload}>
            {i("retry")}
          </button>
        </div>
      ) : selected.value.data.length === 0 ? (
        <p>{i("noSuggestions")}</p>
      ) : (
        <div className="lot-grid">
          {selected.value.data.map(({ lot, reason }) => (
            <div className="stack-small" key={lot.id}>
              <p className="recommendation-reason">{i(reason)}</p>
              <PublicLotCard
                lot={lot}
                categories={categories}
                now={selected.value.meta.server_time}
              />
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
