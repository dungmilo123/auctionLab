import { expect, it } from "vitest";
import { formatOrderMoney, MAX_ORDER_TOTAL } from "./order-money";
import {
  formatCatalogMoney,
  parseCatalogMoney,
  MAX_CATALOG_MONEY,
} from "./catalog-money";
import { routeFor } from "./route";
import { safeReturn } from "./account-return";
import { ORDER_COPY } from "./order-copy";
it("formats exact checkout cents without widening components", () => {
  expect(formatOrderMoney(0)).toBe("$0.00 USD");
  expect(formatOrderMoney(14050)).toBe("$140.50 USD");
  expect(formatOrderMoney(MAX_ORDER_TOTAL)).toBe("$20000000000.00 USD");
  expect(formatOrderMoney(MAX_CATALOG_MONEY)).toBe("$10000000000.00 USD");
  for (const n of [
    -1,
    1.5,
    MAX_ORDER_TOTAL + 1,
    Number.MAX_SAFE_INTEGER + 1,
    NaN,
  ])
    expect(() => formatOrderMoney(n)).toThrow();
  expect(() => formatCatalogMoney(MAX_ORDER_TOTAL)).toThrow();
  expect(() => parseCatalogMoney("20000000000")).toThrow();
});
it("routes explicit order identity with safe login return", () => {
  const path = "/account/checkout/" + "a".repeat(32);
  expect(routeFor(path + "/?x=1#y")).toMatchObject({
    page: "checkout",
    layout: "account",
    orderId: "a".repeat(32),
  });
  expect(safeReturn(path)).toBe(path);
  expect(safeReturn("/account/wins")).toBe("/account/wins");
  for (const p of [
    path + "/extra",
    "/account/checkout",
    "/account/checkout/invalid",
    path + "//",
  ])
    expect(routeFor(p).page).toBe("notFound");
  for (const p of [
    "//evil.test",
    "https://evil.test",
    "/admin/orders",
    "/api/account/wins",
    "/account/login",
    "/account/register",
    "/assets/x",
    "/account/checkout/%gg",
  ])
    expect(safeReturn(p)).toBe("/account");
});
it("provides complete bilingual checkout and simulation copy", () => {
  for (const v of Object.values(ORDER_COPY)) {
    expect(v.en.trim()).not.toBe("");
    expect(v.vi.trim()).not.toBe("");
  }
  expect(ORDER_COPY.simulation.en).toContain("No money");
  expect(ORDER_COPY.history.en).toContain("timeline");
});
