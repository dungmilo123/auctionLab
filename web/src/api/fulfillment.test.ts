import { describe, it, expect, vi, afterEach } from "vitest";
import {
  isSummary,
  isHistoryOrder,
  parseFilters,
  nextStatus,
  historyAPI,
  fulfillmentAPI,
  type HistoryOrder,
  type Summary,
} from "./fulfillment";
import { FULFILLMENT_COPY } from "@/lib/fulfillment-copy";
import { isOrder } from "./order";
import { routeFor } from "@/lib/route";
import { safeReturn } from "@/lib/account-return";
import { sampleOrder } from "./fulfillment.fixture";
const summary: Summary = {
  id: sampleOrder.id,
  owner_id: "owner",
  lot_id: "lot",
  event_id: "event",
  slug: "lot",
  title: "Lot",
  image: sampleOrder.snapshot.image,
  lot_demo: true,
  event_demo: true,
  account_demo: true,
  winning_amount_minor: 12550,
  currency: "USD",
  closes_at: 100,
  recorded_at: 101,
  created_at: 102,
  state: "paid",
  status: "paid",
  version: 1,
  method: "shipping",
  total_minor: 14050,
};
afterEach(() => vi.unstubAllGlobals());
describe("order history and fulfillment boundaries", () => {
  it("accepts durable projections while retaining checkout payment semantics", () => {
    expect(isOrder(sampleOrder, "owner")).toBe(true);
    expect(isHistoryOrder(sampleOrder, "owner", sampleOrder.id)).toBe(true);
    expect(isSummary(summary, "owner")).toBe(true);
    const unpaid = {
      ...sampleOrder,
      state: "awaiting_payment",
      selection: null,
      total: null,
      receipt: null,
      fulfillment: {
        status: "awaiting_payment",
        version: 0,
        updated_at: 102,
        shipment: null,
        timeline: sampleOrder.fulfillment.timeline.slice(0, 1),
      },
    };
    expect(isHistoryOrder(unpaid, "owner")).toBe(true);
  });
  it("rejects inconsistent private state, methods, amounts, versions and timelines", () => {
    for (const change of [
      (o: HistoryOrder) => (o.owner_id = "foreign"),
      (o: HistoryOrder) => o.fulfillment.version++,
      (o: HistoryOrder) => (o.fulfillment.timeline[1]!.occurred_at = 99),
      (o: HistoryOrder) => (o.fulfillment.status = "completed"),
      (o: HistoryOrder) => (o.fulfillment.timeline[1]!.source = "operator"),
      (o: HistoryOrder) =>
        (o.fulfillment.shipment = { carrier: "x", tracking_reference: "y" }),
      (o: HistoryOrder) => o.receipt!.total_minor++,
    ]) {
      const o = structuredClone(sampleOrder);
      change(o);
      expect(isHistoryOrder(o, "owner")).toBe(false);
    }
    for (const change of [
      { owner_id: "foreign" },
      { method: "pickup" },
      { version: 5 },
      { total_minor: -1 },
      { status: "shipped" },
      { state: "awaiting_payment" },
      { image: { path: "https://external.test/image", alt: "x" } },
    ])
      expect(isSummary({ ...summary, ...change }, "owner")).toBe(false);
  });
  it("matches exact owner routes and safe returns", () => {
    const path = "/account/orders/" + sampleOrder.id;
    for (const tail of ["", "/", "?x=1", "#t"]) {
      expect(routeFor(path + tail).page).toBe("orderDetail");
      expect(safeReturn(path + tail)).toBe((path + tail).split("#")[0]);
    }
    for (const tail of ["/extra", "//"])
      expect(routeFor(path + tail).page).toBe("notFound");
    expect(
      routeFor("/account/orders/" + sampleOrder.id.toUpperCase()).page,
    ).toBe("notFound");
  });
  it("validates URL filters and terminal method paths", () => {
    expect(
      parseFilters("?status=preparing&method=shipping&view=" + sampleOrder.id)
        .valid,
    ).toBe(true);
    for (const q of [
      "?status=paid&status=all",
      "?method=bad",
      "?other=1",
      "?view=ABC",
    ])
      expect(parseFilters(q).valid).toBe(false);
    expect(nextStatus("shipping", "paid")).toBe("preparing");
    expect(nextStatus("pickup", "paid")).toBe("ready_for_pickup");
    expect(nextStatus("shipping", "completed")).toBeUndefined();
  });
  it("retains EN/VI label parity", () => {
    for (const pair of Object.values(FULFILLMENT_COPY)) {
      expect(Object.keys(pair).sort()).toEqual(["en", "vi"]);
      expect(pair.en.length).toBeGreaterThan(0);
      expect(pair.vi.length).toBeGreaterThan(0);
    }
  });
  it("uses private no-store boundaries, strict ordering and separate admin headers", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue({ ok: true, json: async () => ({ data: [summary] }) });
    vi.stubGlobal("fetch", fetch);
    const signal = new AbortController().signal;
    expect(await historyAPI.list("owner", signal)).toEqual([summary]);
    expect(fetch.mock.calls[0]![1]).toMatchObject({
      credentials: "same-origin",
      cache: "no-store",
      method: "GET",
    });
    fetch.mockResolvedValue({
      ok: true,
      json: async () => ({ data: sampleOrder }),
    });
    await fulfillmentAPI.transition(
      sampleOrder.id,
      1,
      "preparing",
      undefined,
      signal,
    );
    expect(fetch.mock.calls.at(-1)![1]).toMatchObject({
      method: "POST",
      headers: {
        "X-AuctionLab-Admin": "1",
        "Content-Type": "application/json",
      },
    });
    fetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        data: [summary, { ...summary, id: "c".repeat(32), recorded_at: 102 }],
      }),
    });
    await expect(historyAPI.list("owner", signal)).rejects.toMatchObject({
      code: "invalid_response",
    });
    fetch.mockRejectedValue(new Error("lost"));
    await expect(
      fulfillmentAPI.transition(
        sampleOrder.id,
        1,
        "preparing",
        undefined,
        signal,
      ),
    ).rejects.toMatchObject({ code: "commit_unknown" });
  });
});
