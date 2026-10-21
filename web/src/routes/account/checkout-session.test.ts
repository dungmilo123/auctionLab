import { expect, it, vi } from "vitest";
import { CheckoutSession } from "./checkout-session";
import type { Selection } from "@/api/order";
const selection: Selection = {
  contact: { name: "Buyer", email: "buyer@example.test", phone: "123" },
  fulfillment_method: "pickup",
  payment_method: "demo-card",
};
it("suppresses double clicks and obsolete reads/reviews/confirmations across repeated setup", () => {
  let owner = true;
  const s = new CheckoutSession(() => owner);
  s.open();
  const first = s.begin()!;
  expect(s.begin()).toBeNull();
  s.close();
  s.open();
  const second = s.begin()!;
  expect(first.signal.aborted).toBe(true);
  expect(first.current()).toBe(false);
  first.finish();
  expect(s.begin()).toBeNull();
  second.finish();
  expect(s.begin()).not.toBeNull();
  owner = false;
  expect(second.current()).toBe(false);
  s.close();
  expect(s.begin()).toBeNull();
});
it("retains the exact intent for deliberate retry and clears it on edit or restoration", () => {
  const uuid = vi.spyOn(crypto, "randomUUID");
  const s = new CheckoutSession(() => true);
  s.open();
  expect(uuid).not.toHaveBeenCalled();
  const a = s.acceptedIntent(selection);
  const b = s.acceptedIntent({
    ...selection,
    payment_method: "demo-bank-transfer",
  });
  expect(b).toEqual(a);
  expect(uuid).toHaveBeenCalledTimes(1);
  s.resetIntent();
  expect(s.acceptedIntent(selection).request_id).not.toBe(a.request_id);
  s.close();
  s.open();
  expect(s.hasIntent()).toBe(false);
  expect(uuid).toHaveBeenCalledTimes(2);
  uuid.mockRestore();
});
