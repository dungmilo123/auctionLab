import { useCallback, useEffect, useState } from 'react'
import { AccountError } from '@/api/account'
import { useAccount } from '@/account/AccountProvider'
export type PrivateRead<T> = { status: 'loading' } | { status: 'success'; data: T } | { status: 'error'; error: AccountError }
// A cancelled response may still resolve; the live flag guards every result.
export function startPrivateRead<T>(load: (s: AbortSignal) => Promise<T>, success: (v: T) => void, failure: (e: AccountError) => void) {
  const controller = new AbortController(); let live = true
  void load(controller.signal).then(v => { if (live) success(v) }, e => { if (live) failure(e instanceof AccountError ? e : new AccountError('unavailable')) })
  return () => { live = false; controller.abort() }
}
export function usePrivateRead<T>(load: (s: AbortSignal) => Promise<T>) {
  const { store } = useAccount(); const [attempt, setAttempt] = useState(0); const [state, setState] = useState<PrivateRead<T>>({ status: 'loading' })
  const retry = useCallback(() => { setState({ status: 'loading' }); setAttempt(n => n + 1) }, [])
  useEffect(() => { return startPrivateRead(load, data => setState({ status: 'success', data }), error => { if (error.status === 401) store.signedOut(); else setState({ status: 'error', error }) }) }, [load, attempt, store])
  return { state, setState, retry }
}
