import { expect, it } from 'vitest'
import { startPublicRead } from './usePublicPage'
import type { PublicResponse } from '@/api/public-catalog'
it('cancels obsolete requests even when their promise ignores abort', async () => {
  let resolve!: (value: PublicResponse<string>) => void
  let signal!: AbortSignal
  const successes: string[] = []
  const cleanup = startPublicRead<string>(
    (s) => {
      signal = s
      return new Promise<PublicResponse<string>>((r) => {
        resolve = r
      })
    },
    (v) => successes.push(v.data),
    () => {
      throw Error('obsolete failure')
    },
  )
  cleanup()
  expect(signal.aborted).toBe(true)
  resolve({ data: 'old', serverTime: 100, receivedAt: 0 })
  await Promise.resolve()
  expect(successes).toEqual([])
  startPublicRead(
    async () => ({ data: 'new', serverTime: 200, receivedAt: 0 }),
    (v) => successes.push(v.data),
    () => {},
  )
  await Promise.resolve()
  expect(successes).toEqual(['new'])
})
