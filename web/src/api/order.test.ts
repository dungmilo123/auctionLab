import { describe, expect, it, vi, afterEach } from "vitest";
import {
  isOrder,
  isSelection,
  isTotal,
  orderAPI,
  type Order,
  type Selection,
} from "./order";
import { isParticipation } from "./auction";
import { AccountError } from "./account";
const id = "a".repeat(32);
const selection: Selection = {
  contact: { name: "Buyer", email: "buyer@example.test", phone: "123" },
  fulfillment_method: "shipping",
  shipping_address: {
    line1: "Street",
    line2: "",
    locality: "City",
    region: "",
    postal_code: "",
    country: "VN",
  },
  payment_method: "demo-card",
};
const total = {
  fulfillment_method: "shipping" as const,
  instructions: "Ship",
  winning_amount_minor: 12550,
  fulfillment_fee_minor: 1500,
  total_minor: 14050,
};
const unpaid: Order = {
  id,
  owner_id: "owner",
  winning_bid_id: 1,
  winning_amount_minor: 12550,
  currency: "USD",
  state: "awaiting_payment",
  closes_at: 100,
  recorded_at: 101,
  created_at: 102,
  snapshot: {
    lot_id: "l",
    event_id: "e",
    slug: "lot",
    title: "Lot",
    condition: "Good",
    image: { path: "/images/photo.jpg", alt: "Lot" },
    fulfillment: {
      shipping_enabled: true,
      pickup_enabled: false,
      shipping_fee_minor: 1500,
      shipping_instructions: "Ship",
      pickup_instructions: "",
    },
    lot_demo: false,
    event_demo: false,
    account_demo: false,
  },
  options: [total],
  selection: null,
  total: null,
  receipt: null,
};
const paid: Order = {
  ...unpaid,
  state: "paid",
  selection,
  total,
  receipt: {
    id: "b".repeat(32),
    order_id: id,
    owner_id: "owner",
    payment_method: "demo-card",
    currency: "USD",
    total_minor: 14050,
    paid_at: 103,
    simulated: true,
  },
};
afterEach(() => vi.unstubAllGlobals());
describe("checkout response boundary", () => {
  it("validates owner, identity, state, selected terms, exact sums and simulation receipts", () => {
    expect(isOrder(unpaid, "owner", id)).toBe(true);
    expect(isOrder(paid, "owner", id)).toBe(true);
    expect(isOrder(paid, "foreign", id)).toBe(false);
    expect(isOrder(paid, "owner", "c".repeat(32))).toBe(false);
    for (const mutate of [
      (o: Order) => {
        o.receipt!.order_id = "c".repeat(32);
      },
      (o: Order) => {
        o.receipt!.total_minor++;
      },
      (o: Order) => {
        o.total!.total_minor++;
      },
      (o: Order) => {
        o.receipt!.paid_at = 1;
      },
      (o: Order) => {
        o.selection!.payment_method = "real" as never;
      },
      (o: Order) => {
        o.options[0]!.fulfillment_fee_minor = 0;
      },
      (o: Order) => {
        o.snapshot.fulfillment.shipping_fee_minor = null;
      },
      (o: Order) => {
        o.receipt!.simulated = false as never;
      },
    ]) {
      const o = structuredClone(paid);
      mutate(o);
      expect(isOrder(o, "owner", id)).toBe(false);
    }
    expect(isSelection({ ...selection, fulfillment_method: "pickup" })).toBe(
      false,
    );
    expect(
      isSelection({
        ...selection,
        fulfillment_method: "pickup",
        shipping_address: undefined,
      }),
    ).toBe(true);
    expect(
      isTotal({
        ...total,
        winning_amount_minor: 1e12,
        fulfillment_fee_minor: 1e12,
        total_minor: 2e12,
      }),
    ).toBe(true);
  });
  it("accepts checkout links only on coherent owned wins", () => {
    const p = {
      lot_id: "l",
      slug: "lot",
      title: "Lot",
      status: "won",
      latest: {
        sequence: 1,
        amount_minor: 12550,
        accepted_at: 1,
        request_id: "00000000-0000-0000-0000-000000000001",
      },
      bids: [
        {
          sequence: 1,
          amount_minor: 12550,
          accepted_at: 1,
          request_id: "00000000-0000-0000-0000-000000000001",
        },
      ],
      current_price_minor: 12550,
      revision: 3,
      checkout_available: true,
      order_id: id,
      order_state: "awaiting_payment",
    };
    expect(isParticipation(p)).toBe(true);
    expect(
      isParticipation({ ...p, order_state: "paid", checkout_available: false }),
    ).toBe(true);
    for (const v of [
      { ...p, status: "lost" },
      { ...p, order_state: "paid" },
      { ...p, order_id: undefined },
      { ...p, order_id: "invalid" },
    ])
      expect(isParticipation(v)).toBe(false);
  });
  it("uses private no-store strict requests and rejects malformed server success", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ data: unpaid })));
    vi.stubGlobal("fetch", fetcher);
    await orderAPI.get("owner", id);
    expect(fetcher.mock.calls[0]![1]).toMatchObject({
      credentials: "same-origin",
      cache: "no-store",
      method: "GET",
    });
    fetcher.mockResolvedValue(
      new Response(
        JSON.stringify({ data: { order: unpaid, selection, total } }),
      ),
    );
    await orderAPI.review("owner", id, selection);
    expect(fetcher.mock.calls[1]![1]).toMatchObject({
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-AuctionLab-Account": "1",
      },
    });
    fetcher.mockResolvedValue(
      new Response(
        JSON.stringify({
          data: { order: unpaid, replay: false, already_paid: false },
        }),
      ),
    );
    await expect(
      orderAPI.confirm("owner", id, {
        ...selection,
        request_id: crypto.randomUUID(),
      }),
    ).rejects.toBeInstanceOf(AccountError);
    fetcher.mockResolvedValue(new Response(JSON.stringify({ data: [paid] })));
    await expect(orderAPI.wins("owner")).rejects.toMatchObject({
      code: "invalid_response",
    });
  });
});
