import { useEffect, useRef, useState, type FormEvent } from 'react'
import { accountAPI, AccountError, type Profile as SafeProfile } from '@/api/account'
import { useAccount } from '@/account/AccountProvider'
import { accountErrorKey, useAccountCopy } from '@/lib/account-copy'
import { Field } from '@/components/site/Presentation'
export default function Profile({ profile }: { profile: SafeProfile }) {
  const { a } = useAccountCopy(); const { state, store } = useAccount(); const [name, setName] = useState(profile.display_name)
  const [pending, setPending] = useState(false); const [error, setError] = useState<AccountError | null>(null); const [saved, setSaved] = useState(false)
  const alive = useRef(true); useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  async function perform(logout: boolean) {
    if (pending) return; setPending(true); setError(null); setSaved(false)
    try { if (logout) { await accountAPI.logout(); if (alive.current && store.get() === state) store.signedOut() }
      else { const p = await accountAPI.update(name); if (alive.current && store.get() === state) { store.confirm(p, state); setName(p.display_name); setSaved(true) } }
    } catch (e) { if (alive.current) { const err = e instanceof AccountError ? e : new AccountError('unavailable'); setError(err); if (err.status === 401) store.signedOut() } }
    finally { if (alive.current) setPending(false) }
  }
  return <section className="stack account-panel">
    <p className="eyebrow">{a(profile.role)}</p>
    <label htmlFor="profile-email">{a('email')}</label><input id="profile-email" className="site-input" value={profile.email} readOnly aria-describedby="profile-email-help" /><p id="profile-email-help">{a('identityHelp')}</p>
    <form className="stack" onSubmit={(event: FormEvent) => { event.preventDefault(); void perform(false) }} noValidate>
      <Field id="profile-name" label={a('name')} help={a('nameHelp')} error={error?.fields.display_name ? a('invalid') : undefined} value={name} onChange={e => setName(e.target.value)} autoComplete="nickname" disabled={pending} />
      <button className="site-button site-button-primary" disabled={pending}>{a('saveName')}</button>
    </form>
    <p role={error ? 'alert' : 'status'}>{pending ? a('pending') : error ? a(accountErrorKey(error.code)) : saved ? a('profileSaved') : ''}</p>
    <button className="site-button site-button-outline" disabled={pending} onClick={() => void perform(true)}>{a('logout')}</button>
  </section>
}
