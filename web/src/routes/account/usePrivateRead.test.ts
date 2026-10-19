import { expect, it } from 'vitest'
import { startPrivateRead } from './usePrivateRead'
it('drops obsolete private reads even when abort is ignored by the transport', async () => {
  let finish!: (v: string) => void; let signal!: AbortSignal; const seen: string[] = []
  const cleanup = startPrivateRead(s => { signal = s; return new Promise<string>(resolve => { finish = resolve }) }, v => seen.push(v), () => { throw Error('stale error') })
  cleanup(); expect(signal.aborted).toBe(true); finish('previous account/lot'); await Promise.resolve(); expect(seen).toEqual([])
})
