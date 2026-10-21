import { afterEach, expect, it, vi } from "vitest";
import {
  aggregateMoney,
  aggregateCount,
  decodeAnalytics,
  decodeSuggestions,
  insightsAPI,
  metricKeys,
  parseAnalyticsFilters,
  sellThrough,
  type Metrics,
} from "./insights";
import { routeFor } from "@/lib/route";
import { INSIGHTS_COPY } from "@/lib/insights-copy";
import { createObservationRecorder } from "@/lib/lot-observation";
import { startPrivateRead } from "@/routes/account/usePrivateRead";
afterEach(() => vi.unstubAllGlobals());
const zero = () =>
  ({
    ...Object.fromEntries(metricKeys.map((k) => [k, "0"])),
    sell_through: null,
  }) as Metrics;
const payload = () => ({
  data: {
    summary: zero(),
    events: [],
    interactions: [],
    definitions: { coverage: "incomplete" },
  },
  meta: {
    as_of: 100,
    currency: "USD",
    collection_started_at: null,
    filters: { event: "all", dataset: "all" },
  },
});
it("formats integer totals beyond the safe integer boundary exactly", () => {
  expect(aggregateMoney("900719925474099312345", "en")).toBe(
    "9,007,199,254,740,993,123.45 USD",
  );
  expect(aggregateMoney("900719925474099312345", "vi")).toBe(
    "9.007.199.254.740.993.123,45 USD",
  );
  expect(aggregateCount("900719925474099312345", "en")).toBe(
    "900,719,925,474,099,312,345",
  );
  const m = zero();
  m.sold = "2";
  m.terminal = "3";
  m.sell_through = { sold: "2", terminal: "3" };
  expect(sellThrough(m)).toBe("66.7% (2/3)");
  expect(sellThrough(zero())).toBe("—");
});
it("distinguishes valid empty data from malformed success and inconsistent aggregates", () => {
  expect(decodeAnalytics(payload()).data.summary.lots).toBe("0");
  for (const bad of [
    null,
    {},
    { ...payload(), meta: { ...payload().meta, currency: "VND" } },
    {
      ...payload(),
      data: { ...payload().data, summary: { ...zero(), lots: 0 } },
    },
    {
      ...payload(),
      data: { ...payload().data, summary: { ...zero(), lots: "01" } },
    },
    {
      ...payload(),
      data: { ...payload().data, summary: { ...zero(), lots: "1" } },
    },
  ])
    expect(() => decodeAnalytics(bad)).toThrow();
  expect(() =>
    decodeSuggestions({
      data: [],
      meta: { server_time: 100, strategy: "rules-v1" },
    }),
  ).not.toThrow();
  for (const bad of [
    { data: [], meta: { server_time: 100, strategy: "ml" } },
    {
      data: [{ lot: {}, reason: "closing_soon" }],
      meta: { server_time: 100, strategy: "rules-v1" },
    },
  ])
    expect(() => decodeSuggestions(bad)).toThrow();
});
it("rejects unsafe and duplicate query selections and matches only the analytics route", () => {
  for (const query of [
    "?event=",
    "?dataset=",
    "?event=all&event=all",
    "?unknown=1",
    "?event=Upper",
    "?dataset=other",
  ])
    expect(parseAnalyticsFilters(query)).toBeNull();
  expect(parseAnalyticsFilters("?event=analog-memories&dataset=demo")).toEqual({
    event: "analog-memories",
    dataset: "demo",
  });
  expect(routeFor("/admin/analytics/?dataset=demo")).toMatchObject({
    page: "adminAnalytics",
  });
  for (const path of [
    "/Admin/analytics",
    "/admin/analytics/extra",
    "/admin/Analytics",
  ])
    expect(routeFor(path).page).toBe("notFound");
});
it("loads public and private suggestions with independent credentials and signals", async () => {
  const fetch = vi
    .fn()
    .mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            data: [],
            meta: { strategy: "rules-v1", server_time: 100 },
          }),
        ),
      ),
    );
  vi.stubGlobal("fetch", fetch);
  const a = new AbortController(),
    b = new AbortController();
  await insightsAPI.recommendations("lot", false, a.signal);
  await insightsAPI.recommendations("lot", true, b.signal);
  expect(fetch.mock.calls[0]).toEqual([
    "/api/catalog/recommendations?source=lot",
    { credentials: "omit", cache: "no-store", signal: a.signal },
  ]);
  expect(fetch.mock.calls[1]).toEqual([
    "/api/account/recommendations?source=lot",
    { credentials: "same-origin", cache: "no-store", signal: b.signal },
  ]);
});
it("suppresses delayed cancelled private results including failures", async () => {
  let resolve!: (v: string) => void;
  const success = vi.fn(),
    fail = vi.fn();
  const stop = startPrivateRead(
    () => new Promise<string>((r) => (resolve = r)),
    success,
    fail,
  );
  stop();
  resolve("old account");
  await Promise.resolve();
  expect(success).not.toHaveBeenCalled();
  expect(fail).not.toHaveBeenCalled();
});
it("records once per lot per document and never retries failed observations", async () => {
  const send = vi.fn().mockRejectedValue(new Error("offline")),
    mint = vi.fn().mockReturnValue("00000000-0000-0000-0000-000000000001");
  const record = createObservationRecorder(send, mint);
  record("l");
  record("l");
  record("l");
  await Promise.resolve();
  expect(send).toHaveBeenCalledTimes(1);
  expect(mint).toHaveBeenCalledTimes(1);
  expect(send.mock.calls[0]![1]).toMatchObject({
    method: "POST",
    credentials: "omit",
    cache: "no-store",
  });
  createObservationRecorder(send, mint)("l");
  expect(send).toHaveBeenCalledTimes(2);
});
it("preserves error status, rejects malformed and never converts errors to zeros", async () => {
  const signal = new AbortController().signal;
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response("private", { status: 401 })),
  );
  await expect(
    insightsAPI.analytics({ event: "all", dataset: "all" }, signal),
  ).rejects.toMatchObject({ status: 401 });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("{}")));
  await expect(
    insightsAPI.analytics({ event: "all", dataset: "all" }, signal),
  ).rejects.toMatchObject({ code: "invalid_response" });
});
it("has complete EN/VI metric and reason copy", () => {
  for (const [key, v] of Object.entries(INSIGHTS_COPY)) {
    expect(v[0], key).toBeTruthy();
    expect(v[1], key).toBeTruthy();
  }
});
