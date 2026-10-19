import { useEffect, useState } from 'react'
import { CatalogError } from '@/api/catalog'
import type { PublicResponse } from '@/api/public-catalog'
import { sampleTime } from '@/lib/discovery'

export type PageState<T> =
  | { key: string; status: 'loading' }
  | { key: string; status: 'error'; error: CatalogError }
  | { key: string; status: 'success'; response: PublicResponse<T> }
/** Aborting and the active guard cover both fetch and promises already resolving. */
export function startPublicRead<T>(
  load: (signal: AbortSignal) => Promise<PublicResponse<T>>,
  success: (value: PublicResponse<T>) => void,
  failure: (error: CatalogError) => void,
) {
  const controller = new AbortController()
  let active = true
  void load(controller.signal).then(
    (value) => {
      if (active) success(value)
    },
    (error) => {
      if (active)
        failure(
          error instanceof CatalogError
            ? error
            : new CatalogError('unavailable'),
        )
    },
  )
  return () => {
    active = false
    controller.abort()
  }
}
export function usePublicPage<T>(
  key: string,
  load: (signal: AbortSignal) => Promise<PublicResponse<T>>,
) {
  const [generation, setGeneration] = useState(0)
  const requestKey = `${key}:${generation}`
  const [state, setState] = useState<PageState<T>>({
    key: requestKey,
    status: 'loading',
  })
  const [monotonic, setMonotonic] = useState(() => performance.now())
  useEffect(() => {
    return startPublicRead(
      load,
      (response) => {
        setMonotonic(performance.now())
        setState({ key: requestKey, status: 'success', response })
      },
      (error) => setState({ key: requestKey, status: 'error', error }),
    )
  }, [requestKey, load])
  useEffect(() => {
    const tick = window.setInterval(() => {
      if (!document.hidden) setMonotonic(performance.now())
    }, 1000)
    const resume = () => {
      if (!document.hidden) setGeneration((g) => g + 1)
    }
    document.addEventListener('visibilitychange', resume)
    return () => {
      window.clearInterval(tick)
      document.removeEventListener('visibilitychange', resume)
    }
  }, [])
  return {
    state:
      state.key === requestKey
        ? state
        : ({ key: requestKey, status: 'loading' } as PageState<T>),
    retry: () => setGeneration((g) => g + 1),
    now: state.status === 'success' ? sampleTime(state.response, monotonic) : 0,
  }
}
