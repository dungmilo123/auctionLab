import { isProgress, type Progress } from "./fulfillment";
import { AccountError } from "./account";
import { validInstant } from "./public-catalog";
import { MAX_CATALOG_MONEY } from "@/lib/catalog-money";
import { MAX_ORDER_TOTAL } from "@/lib/order-money";
export type Selection = {
  contact: { name: string; email: string; phone: string };
  fulfillment_method: "shipping" | "pickup";
  shipping_address?: {
    line1: string;
    line2: string;
    locality: string;
    region: string;
    postal_code: string;
    country: string;
  };
  payment_method: "demo-card" | "demo-bank-transfer";
};
export type Total = {
  fulfillment_method: Selection["fulfillment_method"];
  instructions: string;
  winning_amount_minor: number;
  fulfillment_fee_minor: number;
  total_minor: number;
};
export type Order = {
 fulfillment?: Progress;
  id: string;
  owner_id: string;
  winning_bid_id: number;
  winning_amount_minor: number;
  currency: "USD";
  state: "awaiting_payment" | "paid";
  closes_at: number;
  recorded_at: number;
  created_at: number;
  snapshot: {
    lot_id: string;
    event_id: string;
    slug: string;
    title: string;
    condition: string;
    image: { path: string; alt: string };
    fulfillment: {
      shipping_enabled: boolean;
      pickup_enabled: boolean;
      shipping_fee_minor: number | null;
      shipping_instructions: string;
      pickup_instructions: string;
    };
    lot_demo: boolean;
    event_demo: boolean;
    account_demo: boolean;
  };
  options: Total[];
  selection: Selection | null;
  total: Total | null;
  receipt: {
    id: string;
    order_id: string;
    owner_id: string;
    payment_method: Selection["payment_method"];
    currency: "USD";
    total_minor: number;
    paid_at: number;
    simulated: true;
  } | null;
};
export type Review = { order: Order; selection: Selection; total: Total };
export type Intent = Selection & { request_id: string };
export type Confirmation = {
  order: Order;
  replay: boolean;
  already_paid: boolean;
};
const object = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const text = (v: unknown): v is string => typeof v === "string";
const id = (v: unknown): v is string => text(v) && /^[0-9a-f]{32}$/.test(v);
const money = (v: unknown, max = MAX_CATALOG_MONEY): v is number =>
  Number.isSafeInteger(v) && (v as number) >= 0 && (v as number) <= max;
const method = (v: unknown) => v === "shipping" || v === "pickup";
const payment = (v: unknown) => v === "demo-card" || v === "demo-bank-transfer";
export function isSelection(v: unknown): v is Selection {
  if (
    !object(v) ||
    !object(v.contact) ||
    !["name", "email", "phone"].every(
      (k) =>
        text(v.contact && (v.contact as Record<string, unknown>)[k]) &&
        (v.contact as Record<string, string>)[k]!.length > 0,
    ) ||
    !method(v.fulfillment_method) ||
    !payment(v.payment_method)
  )
    return false;
  if (v.fulfillment_method === "pickup")
    return v.shipping_address === undefined;
  return (
    object(v.shipping_address) &&
    ["line1", "line2", "locality", "region", "postal_code", "country"].every(
      (k) => text((v.shipping_address as Record<string, unknown>)[k]),
    ) &&
    !!v.shipping_address.line1 &&
    !!v.shipping_address.locality &&
    /^[A-Z]{2}$/.test(v.shipping_address.country as string)
  );
}
export function isTotal(v: unknown): v is Total {
  return (
    object(v) &&
    method(v.fulfillment_method) &&
    text(v.instructions) &&
    money(v.winning_amount_minor) &&
    money(v.fulfillment_fee_minor) &&
    money(v.total_minor, MAX_ORDER_TOTAL) &&
    v.total_minor === v.winning_amount_minor + v.fulfillment_fee_minor &&
    (v.fulfillment_method !== "pickup" || v.fulfillment_fee_minor === 0)
  );
}
export function isOrder(
  v: unknown,
  owner: string,
  orderID?: string,
): v is Order {
  if (
    !object(v) ||
    !id(v.id) ||
    (orderID !== undefined && v.id !== orderID) ||
    v.owner_id !== owner ||
    !Number.isSafeInteger(v.winning_bid_id) ||
    (v.winning_bid_id as number) < 1 ||
    !money(v.winning_amount_minor) ||
    v.currency !== "USD" ||
    !["awaiting_payment", "paid"].includes(v.state as string) ||
    !validInstant(v.closes_at) ||
    !validInstant(v.recorded_at) ||
    !validInstant(v.created_at) ||
    v.closes_at > v.recorded_at ||
    v.recorded_at > v.created_at ||
    !object(v.snapshot)
  )
    return false;
  const s = v.snapshot;
  if (
    !["lot_id", "event_id", "slug", "title", "condition"].every(
      (k) => text(s[k]) && s[k] !== "",
    ) ||
    !object(s.image) ||
    !text(s.image.path) ||
    !/^\/(site-assets|images)\/[^?#\\]+$/.test(s.image.path) ||
    !text(s.image.alt) ||
    !["lot_demo", "event_demo", "account_demo"].every(
      (k) => typeof s[k] === "boolean",
    ) ||
    !object(s.fulfillment)
  )
    return false;
  const f = s.fulfillment;
  if (
    typeof f.shipping_enabled !== "boolean" ||
    typeof f.pickup_enabled !== "boolean" ||
    !text(f.shipping_instructions) ||
    !text(f.pickup_instructions) ||
    !(f.shipping_fee_minor === null || money(f.shipping_fee_minor)) ||
    (f.shipping_enabled && !money(f.shipping_fee_minor))
  )
    return false;
  if (
    !Array.isArray(v.options) ||
    !v.options.every(isTotal) ||
    v.options.length !==
      Number(f.shipping_enabled) + Number(f.pickup_enabled) ||
    v.options.length === 0 ||
    new Set(v.options.map((t) => t.fulfillment_method)).size !==
      v.options.length
  )
    return false;
  if (
    !v.options.every(
      (t) =>
        t.winning_amount_minor === v.winning_amount_minor &&
        (t.fulfillment_method === "shipping"
          ? f.shipping_enabled &&
            t.fulfillment_fee_minor === f.shipping_fee_minor &&
            t.instructions === f.shipping_instructions
          : f.pickup_enabled && t.instructions === f.pickup_instructions),
    )
  )
    return false;
  if (v.fulfillment !== undefined && !isProgress(v.fulfillment, v as Order)) return false;
  if (v.state === "awaiting_payment")
    return v.selection === null && v.total === null && v.receipt === null;
  if (
    !isSelection(v.selection) ||
    !isTotal(v.total) ||
    !v.options.some((t) => sameTotal(t, v.total as Total)) ||
    v.total.fulfillment_method !== v.selection.fulfillment_method ||
    !object(v.receipt)
  )
    return false;
  const r = v.receipt;
  return (
    id(r.id) &&
    r.order_id === v.id &&
    r.owner_id === owner &&
    r.payment_method === v.selection.payment_method &&
    r.currency === "USD" &&
    r.total_minor === v.total.total_minor &&
    r.simulated === true &&
    validInstant(r.paid_at) &&
    r.paid_at >= v.created_at
  );
}
function sameTotal(a: Total, b: Total) {
  return (
    a.fulfillment_method === b.fulfillment_method &&
    a.instructions === b.instructions &&
    a.winning_amount_minor === b.winning_amount_minor &&
    a.fulfillment_fee_minor === b.fulfillment_fee_minor &&
    a.total_minor === b.total_minor
  );
}
async function request(
  path: string,
  signal?: AbortSignal,
  body?: unknown,
): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(path, {
      method: body === undefined ? "GET" : "POST",
      credentials: "same-origin",
      cache: "no-store",
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(6000)])
        : AbortSignal.timeout(6000),
      ...(body === undefined
        ? {}
        : {
            headers: {
              "Content-Type": "application/json",
              "X-AuctionLab-Account": "1",
            },
            body: JSON.stringify(body),
          }),
    });
  } catch (e) {
    if (signal?.aborted) throw e;
    throw new AccountError("unavailable");
  }
  let b: unknown;
  try {
    b = await response.json();
  } catch {
    throw new AccountError("invalid_response", response.status);
  }
  if (!response.ok) {
    const err = object(b) && object(b.error) ? b.error : {};
    const fields: Record<string, string> = {};
    if (object(err.fields))
      for (const [k, v] of Object.entries(err.fields))
        if (text(v)) fields[k] = v;
    throw new AccountError(
      text(err.code) ? err.code : "unavailable",
      response.status,
      fields,
    );
  }
  if (!object(b)) throw new AccountError("invalid_response");
  return b.data;
}
const path = (id: string) => `/api/account/orders/${encodeURIComponent(id)}`;
export const orderAPI = {
  wins: async (owner: string, signal?: AbortSignal): Promise<Order[]> => {
    const d = await request("/api/account/wins", signal);
    if (
      !Array.isArray(d) ||
      !d.every((v) => isOrder(v, owner) && v.state === "awaiting_payment") ||
      new Set(d.map((v) => v.id)).size !== d.length
    )
      throw new AccountError("invalid_response");
    return d;
  },
  get: async (
    owner: string,
    id: string,
    signal?: AbortSignal,
  ): Promise<Order> => {
    const d = await request(path(id), signal);
    if (!isOrder(d, owner, id)) throw new AccountError("invalid_response");
    return d;
  },
  review: async (
    owner: string,
    id: string,
    selection: Selection,
    signal?: AbortSignal,
  ): Promise<Review> => {
    const d = await request(`${path(id)}/review`, signal, selection);
    if (
      !object(d) ||
      !isOrder(d.order, owner, id) ||
      !isSelection(d.selection) ||
      !isTotal(d.total) ||
      !d.order.options.some((t) => sameTotal(t, d.total as Total)) ||
      d.selection.fulfillment_method !== d.total.fulfillment_method
    )
      throw new AccountError("invalid_response");
    return d as Review;
  },
  confirm: async (
    owner: string,
    id: string,
    intent: Intent,
    signal?: AbortSignal,
  ): Promise<Confirmation> => {
    const d = await request(`${path(id)}/confirm`, signal, intent);
    if (
      !object(d) ||
      !isOrder(d.order, owner, id) ||
      d.order.state !== "paid" ||
      typeof d.replay !== "boolean" ||
      typeof d.already_paid !== "boolean"
    )
      throw new AccountError("invalid_response");
    return d as Confirmation;
  },
};
