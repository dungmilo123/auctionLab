import { it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { LanguageProvider } from "@/lib/i18n";
import { OrderInspection, OrderSummary } from "./OrderHistory";
import { sampleOrder } from "@/api/fulfillment.fixture";
import { type HistoryOrder } from "@/api/fulfillment";
it("renders paid, shipping and pickup timelines as inert EN/VI text", () => {
  for (const lang of ["en", "vi"] as const) {
    for (const path of ["shipping", "pickup"] as const) {
      const order: HistoryOrder = structuredClone(sampleOrder);
      if (path === "pickup") {
        order.selection!.fulfillment_method = "pickup";
        delete order.selection!.shipping_address;
        order.total = { ...order.options[1]! };
        order.receipt!.total_minor = 12550;
        order.fulfillment = {
          status: "completed",
          version: 3,
          updated_at: 106,
          shipment: null,
          timeline: [
            ...order.fulfillment.timeline,
            {
              version: 2,
              from_status: "paid",
              to_status: "ready_for_pickup",
              occurred_at: 105,
              source: "operator",
              shipment: null,
            },
            {
              version: 3,
              from_status: "ready_for_pickup",
              to_status: "completed",
              occurred_at: 106,
              source: "operator",
              shipment: null,
            },
          ],
        };
      } else {
        const shipment = {
          carrier: "<script>inert</script>",
          tracking_reference: "https://example.test/inert",
        };
        order.fulfillment = {
          status: "shipped",
          version: 3,
          updated_at: 106,
          shipment,
          timeline: [
            ...order.fulfillment.timeline,
            {
              version: 2,
              from_status: "paid",
              to_status: "preparing",
              occurred_at: 105,
              source: "operator",
              shipment: null,
            },
            {
              version: 3,
              from_status: "preparing",
              to_status: "shipped",
              occurred_at: 106,
              source: "operator",
              shipment,
            },
          ],
        };
      }
      const html = renderToStaticMarkup(
        <LanguageProvider lang={lang} setLang={() => {}}>
          <OrderInspection order={order} />
        </LanguageProvider>,
      );
      expect(html).toContain(order.id);
      expect(html).toContain(order.receipt!.id);
      expect(html).toContain("/account/checkout/" + order.id);
      expect(html).toContain("<time");
      expect(html).not.toContain("<script>");
      if (path === "shipping") {
        expect(html).toContain("&lt;script&gt;inert&lt;/script&gt;");
        expect(html).not.toContain('href="https://example.test');
      }
    }
  }
});
it("renders unpaid detail with deliberate checkout and null payment selection", () => {
  const order: HistoryOrder = {
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
  const html = renderToStaticMarkup(
    <LanguageProvider lang="en" setLang={() => {}}>
      <OrderInspection order={order} />
    </LanguageProvider>,
  );
  expect(html).toContain("Continue to checkout");
  expect(html).not.toContain("Buyer");
  expect(html).not.toContain("View payment confirmation");
});
it("keeps owner/contact/receipt out of rendered history summaries", () => {
  const html = renderToStaticMarkup(
    <LanguageProvider lang="en" setLang={() => {}}>
      <OrderSummary
        order={{
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
          state: "awaiting_payment",
          status: "awaiting_payment",
          version: 0,
          method: null,
          total_minor: null,
        }}
        href={"/account/orders/" + sampleOrder.id}
        label="Inspect"
      />
    </LanguageProvider>,
  );
  expect(html).toContain("Unselected");
  expect(html).not.toContain("Buyer");
  expect(html).not.toContain(sampleOrder.receipt!.id);
});
