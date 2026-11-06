import { useEffect, useRef, useState, type FormEvent } from 'react'
import { accountAPI, AccountError, type Profile as SafeProfile } from '@/api/account'
import { useAccount } from '@/account/AccountProvider'
import { accountErrorKey, useAccountCopy } from '@/lib/account-copy'
import { Field } from '@/components/site/Presentation'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
  return <section className="flex max-w-[38rem] flex-col gap-6">
    <Badge tone="neutral">{a(profile.role)}</Badge>
    <div className="flex flex-col gap-2.5">
      <Label htmlFor="profile-email">{a('email')}</Label>
      <Input id="profile-email" className="bg-muted" value={profile.email} readOnly aria-describedby="profile-email-help" />
      <p id="profile-email-help" className="mt-2 text-sm text-muted-foreground">{a('identityHelp')}</p>
    </div>
    <form className="flex flex-col gap-6" onSubmit={(event: FormEvent) => { event.preventDefault(); void perform(false) }} noValidate>
      <Field id="profile-name" label={a('name')} help={a('nameHelp')} error={error?.fields.display_name ? a('invalid') : undefined} value={name} onChange={e => setName(e.target.value)} autoComplete="nickname" disabled={pending} />
      <Button className="self-start" disabled={pending}>{a('saveName')}</Button>
    </form>
    <p role={error ? 'alert' : 'status'} className={error ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'}>{pending ? a('pending') : error ? a(accountErrorKey(error.code)) : saved ? a('profileSaved') : ''}</p>
    <Button variant="outline" className="self-start" disabled={pending} onClick={() => void perform(true)}>{a('logout')}</Button>
  </section>
}
