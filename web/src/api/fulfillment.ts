import { AccountError } from "./account";
import { isOrder, type Order } from "./order";
import { validInstant } from "./public-catalog";
export const STATUSES = [
  "awaiting_payment",
  "paid",
  "preparing",
  "shipped",
  "ready_for_pickup",
  "completed",
] as const;
export type Status = (typeof STATUSES)[number];
export type Shipment = { carrier: string; tracking_reference: string };
export type ProgressEvent = {
  version: number;
  from_status: string;
  to_status: Status;
  occurred_at: number;
  source: "system" | "payment" | "operator";
  shipment: Shipment | null;
};
export type Progress = {
  status: Status;
  version: number;
  updated_at: number;
  shipment: Shipment | null;
  timeline: ProgressEvent[];
};
export type HistoryOrder = Order & { fulfillment: Progress };
export type Summary = {
  id: string;
  owner_id: string;
  lot_id: string;
  event_id: string;
  slug: string;
  title: string;
  image: { path: string; alt: string };
  lot_demo: boolean;
  event_demo: boolean;
  account_demo: boolean;
  winning_amount_minor: number;
  currency: "USD";
  closes_at: number;
  recorded_at: number;
  created_at: number;
  state: "awaiting_payment" | "paid";
  status: Status;
  version: number;
  method: "shipping" | "pickup" | null;
  total_minor: number | null;
};
export type Filters = { status: string; method: string };
const obj = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);
const text = (v: unknown): v is string => typeof v === "string" && v.length > 0;
const id = (v: unknown): v is string =>
  typeof v === "string" && /^[0-9a-f]{32}$/.test(v);
const amount = (v: unknown, max = 1e12): v is number =>
  Number.isSafeInteger(v) && (v as number) >= 0 && (v as number) <= max;
export function nextStatus(
  method: string | null,
  status: string,
): Status | undefined {
  if (status === "paid") {
    if (method === "shipping") return "preparing";
    if (method === "pickup") return "ready_for_pickup";
  }
  if (method === "shipping" && status === "preparing") return "shipped";
  if (
    (method === "shipping" && status === "shipped") ||
    (method === "pickup" && status === "ready_for_pickup")
  )
    return "completed";
}
export function isShipment(v: unknown): v is Shipment {
  return (
    obj(v) &&
    [
      ["carrier", 120],
      ["tracking_reference", 200],
    ].every(([key, limit]) => {
      const s = v[key as string];
      return (
        text(s) &&
        s === s.trim() &&
        [...s].length <= (limit as number) &&
        !/[\p{Cc}\p{Cs}]/u.test(s)
      );
    })
  );
}
function validStage(method: unknown, status: unknown, version: unknown) {
  const path =
    method === "shipping"
      ? ["paid", "preparing", "shipped", "completed"]
      : method === "pickup"
        ? ["paid", "ready_for_pickup", "completed"]
        : [];
  return (
    Number.isSafeInteger(version) &&
    (version as number) >= 1 &&
    path[(version as number) - 1] === status
  );
}
export function isSummary(v: unknown, owner?: string): v is Summary {
  if (
    !obj(v) ||
    !id(v.id) ||
    !text(v.owner_id) ||
    (owner !== undefined && v.owner_id !== owner) ||
    !["lot_id", "event_id", "slug", "title"].every((k) => text(v[k])) ||
    !obj(v.image) ||
    !text(v.image.path) ||
    !/^\/(site-assets|images)\/[^?#\\]+$/.test(v.image.path) ||
    !text(v.image.alt) ||
    !["lot_demo", "event_demo", "account_demo"].every(
      (k) => typeof v[k] === "boolean",
    ) ||
    !amount(v.winning_amount_minor) ||
    v.currency !== "USD" ||
    !validInstant(v.closes_at) ||
    !validInstant(v.recorded_at) ||
    !validInstant(v.created_at) ||
    v.recorded_at < v.closes_at ||
    v.created_at < v.recorded_at
  )
    return false;
  if (v.state === "awaiting_payment")
    return (
      v.status === "awaiting_payment" &&
      v.version === 0 &&
      v.method === null &&
      v.total_minor === null
    );
  return (
    v.state === "paid" &&
    validStage(v.method, v.status, v.version) &&
    amount(v.total_minor, 2e12) &&
    v.total_minor >= v.winning_amount_minor &&
    (v.method !== "pickup" || v.total_minor === v.winning_amount_minor)
  );
}
export function isProgress(v: unknown, o: Order): v is Progress {
  if (
    !obj(v) ||
    !validInstant(v.updated_at) ||
    !Array.isArray(v.timeline) ||
    v.timeline.length !== (v.version as number) + 1
  )
    return false;
  if (o.state === "awaiting_payment") {
    if (
      v.status !== "awaiting_payment" ||
      v.version !== 0 ||
      v.updated_at !== o.created_at ||
      v.shipment !== null
    )
      return false;
  } else if (
    !validStage(o.selection?.fulfillment_method, v.status, v.version) ||
    !o.receipt
  )
    return false;
  let previous = "";
  let time = o.created_at;
  let shipment: unknown = null;
  for (let i = 0; i < v.timeline.length; i++) {
    const e: unknown = v.timeline[i];
    if (
      !obj(e) ||
      e.version !== i ||
      e.from_status !== previous ||
      !validInstant(e.occurred_at) ||
      e.occurred_at < time
    )
      return false;
    const status =
      i === 0
        ? "awaiting_payment"
        : i === 1
          ? "paid"
          : nextStatus(o.selection?.fulfillment_method ?? null, previous);
    if (
      e.to_status !== status ||
      e.source !== (i === 0 ? "system" : i === 1 ? "payment" : "operator") ||
      (i === 0 && e.occurred_at !== o.created_at) ||
      (i === 1 && e.occurred_at !== o.receipt?.paid_at)
    )
      return false;
    if (status === "shipped") {
      if (!isShipment(e.shipment)) return false;
      shipment = e.shipment;
    } else if (e.shipment !== null) return false;
    previous = e.to_status as string;
    time = e.occurred_at;
  }
  return (
    previous === v.status &&
    time === v.updated_at &&
    (shipment === null
      ? v.shipment === null
      : isShipment(v.shipment) &&
        obj(shipment) &&
        shipment.carrier === v.shipment.carrier &&
        shipment.tracking_reference === v.shipment.tracking_reference)
  );
}
export function isHistoryOrder(
  v: unknown,
  owner: string,
  orderID?: string,
): v is HistoryOrder {
  return isOrder(v, owner, orderID) && obj(v) && isProgress(v.fulfillment, v);
}
export function parseFilters(search: string): {
  filters: Filters;
  view: string;
  valid: boolean;
} {
  const q = new URLSearchParams(search);
  const filters = {
    status: q.get("status") ?? "all",
    method: q.get("method") ?? "all",
  };
  const view = q.get("view") ?? "";
  const valid =
    [...q.keys()].every(
      (k) =>
        ["status", "method", "view"].includes(k) && q.getAll(k).length === 1,
    ) &&
    ["all", ...STATUSES].includes(filters.status) &&
    ["all", "shipping", "pickup", "unselected"].includes(filters.method) &&
    (view === "" || id(view));
  return { filters, view, valid };
}
async function request(
  path: string,
  signal: AbortSignal,
  body?: unknown,
  admin = false,
): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(path, {
      method: body === undefined ? "GET" : "POST",
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.any([signal, AbortSignal.timeout(6000)]),
      ...(body === undefined
        ? {}
        : {
            headers: {
              "Content-Type": "application/json",
              "X-AuctionLab-Admin": "1",
            },
            body: JSON.stringify(body),
          }),
    });
  } catch (e) {
    if (signal.aborted) throw e;
    throw new AccountError(
      body === undefined ? "unavailable" : "commit_unknown",
    );
  }
  let data: unknown;
  try {
    data = await response.json();
  } catch {
    throw new AccountError(
      body === undefined ? "invalid_response" : "commit_unknown",
      response.status,
    );
  }
  if (!response.ok) {
    const err = obj(data) && obj(data.error) ? data.error : {};
    throw new AccountError(
      typeof err.code === "string" ? err.code : "unavailable",
      response.status,
      obj(err.fields)
        ? Object.fromEntries(
            Object.entries(err.fields).filter(
              (e): e is [string, string] => typeof e[1] === "string",
            ),
          )
        : {},
    );
  }
  if (!obj(data))
    throw new AccountError(
      body === undefined ? "invalid_response" : "commit_unknown",
    );
  void admin;
  return data.data;
}
function summaries(data: unknown, owner?: string): Summary[] {
  if (
    !Array.isArray(data) ||
    !data.every((v) => isSummary(v, owner)) ||
    new Set(data.map((v) => v.id)).size !== data.length ||
    data.some(
      (v, i) =>
        i > 0 &&
        (data[i - 1]!.recorded_at < v.recorded_at ||
          (data[i - 1]!.recorded_at === v.recorded_at &&
            data[i - 1]!.id > v.id)),
    )
  )
    throw new AccountError("invalid_response");
  return data;
}
function detail(
  data: unknown,
  owner: string | undefined,
  id: string,
): HistoryOrder {
  if (!obj(data) || !isHistoryOrder(data, owner ?? String(data.owner_id), id))
    throw new AccountError("invalid_response");
  return data;
}
export const historyAPI = {
  list: async (owner: string, signal: AbortSignal) =>
    summaries(await request("/api/account/orders", signal), owner),
  get: async (owner: string, id: string, signal: AbortSignal) =>
    detail(
      await request(`/api/account/orders/${encodeURIComponent(id)}`, signal),
      owner,
      id,
    ),
};
export const fulfillmentAPI = {
  list: async (filters: Filters, signal: AbortSignal) =>
    summaries(
      await request(
        `/api/admin/orders?${new URLSearchParams(filters)}`,
        signal,
      ),
    ),
  get: async (id: string, signal: AbortSignal) =>
    detail(
      await request(`/api/admin/orders/${encodeURIComponent(id)}`, signal),
      undefined,
      id,
    ),
  transition: async (
    id: string,
    version: number,
    next: Status,
    shipment: Shipment | undefined,
    signal: AbortSignal,
  ) =>
    detail(
      await request(
        `/api/admin/orders/${encodeURIComponent(id)}/transition`,
        signal,
        {
          expected_version: version,
          next_status: next,
          ...(shipment ? { shipment } : {}),
        },
        true,
      ),
      undefined,
      id,
    ),
};
