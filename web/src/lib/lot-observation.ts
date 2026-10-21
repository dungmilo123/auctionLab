// A document-local registry. Nothing is persisted or shared between documents.
export function createObservationRecorder(
  send: typeof fetch = fetch,
  mint: () => string = () => crypto.randomUUID(),
) {
  const observations = new Map<string, string>();
  return (lot: string) => {
    if (observations.has(lot)) return;
    let id: string;
    try {
      id = mint();
    } catch {
      return;
    }
    observations.set(lot, id);
    void send("/api/catalog/lot-views", {
      method: "POST",
      credentials: "omit",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        "X-AuctionLab-Interaction": "1",
      },
      body: JSON.stringify({ lot_id: lot, observation_id: id }),
    }).catch(() => {});
  };
}
export const recordLotObservation = createObservationRecorder();
