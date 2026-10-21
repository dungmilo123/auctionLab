import type { Intent, Selection } from "@/api/order";
/** One mounted owner's private requests. Retains an intent only until edit/close. */
export class CheckoutSession {
  private generation = 0;
  private controller: AbortController | null = null;
  private pending = false;
  private intent: Intent | null = null;
  private readonly authorized: () => boolean;
  constructor(authorized: () => boolean) {
    this.authorized = authorized;
  }
  open() {
    this.close();
    this.controller = new AbortController();
  }
  close() {
    this.generation++;
    this.controller?.abort();
    this.controller = null;
    this.pending = false;
    this.intent = null;
  }
  begin() {
    if (this.pending || !this.controller || !this.authorized()) return null;
    this.pending = true;
    const generation = this.generation;
    const signal = this.controller.signal;
    return {
      signal,
      current: () =>
        generation === this.generation && !signal.aborted && this.authorized(),
      finish: () => {
        if (generation === this.generation) this.pending = false;
      },
    };
  }
  hasIntent() {
    return this.intent !== null;
  }
  resetIntent() {
    this.intent = null;
  }
  acceptedIntent(selection: Selection) {
    this.intent ??= {
      ...structuredClone(selection),
      request_id: crypto.randomUUID(),
    };
    return this.intent;
  }
}
