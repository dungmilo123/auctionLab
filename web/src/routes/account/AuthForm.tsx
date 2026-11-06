import { useEffect, useRef, useState, type FormEvent } from 'react'
import { accountAPI, AccountError } from '@/api/account'
import { useAccountCopy, accountErrorKey } from '@/lib/account-copy'
import { authURL, safeReturn } from '@/lib/account-return'
import { Field } from '@/components/site/Presentation'
import { Button } from '@/components/ui/button'
export default function AuthForm({ kind }: { kind: 'login' | 'register' }) {
  const { a } = useAccountCopy(); const target = safeReturn(new URLSearchParams(window.location.search).get('return_to'))
  const [email, setEmail] = useState(''); const [password, setPassword] = useState(''); const [name, setName] = useState('')
  const [pending, setPending] = useState(false); const [error, setError] = useState<AccountError | null>(null)
  const alive = useRef(true); const errorNode = useRef<HTMLParagraphElement>(null)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  async function submit(event: FormEvent) {
    event.preventDefault(); if (pending) return; setPending(true); setError(null)
    try {
      if (kind === 'register') await accountAPI.register({ email, password, display_name: name })
      else await accountAPI.login({ email, password })
      if (alive.current) window.location.assign(target)
    } catch (e) { if (alive.current) { setError(e instanceof AccountError ? e : new AccountError('unavailable')); setPending(false); requestAnimationFrame(() => errorNode.current?.focus()) } }
  }
  const fieldError = (key: string) => error?.fields[key] ? a('invalid') : undefined
  return <section className="flex max-w-[38rem] flex-col gap-6"><h1>{a(kind)}</h1><p className="text-muted-foreground">{a('intro')}</p>
    <form className="flex flex-col gap-6" onSubmit={submit} noValidate aria-busy={pending}>
      <Field id="account-email" label={a('email')} help={a('identityHelp')} error={fieldError('email')} type="email" autoComplete="username" value={email} onChange={e => setEmail(e.target.value)} disabled={pending} required />
      {kind === 'register' && <Field id="account-name" label={a('name')} help={a('nameHelp')} error={fieldError('display_name')} autoComplete="nickname" value={name} onChange={e => setName(e.target.value)} disabled={pending} required />}
      <Field id="account-password" label={a('password')} help={a('passwordHelp')} error={fieldError('password')} type="password" autoComplete={kind === 'register' ? 'new-password' : 'current-password'} value={password} onChange={e => setPassword(e.target.value)} disabled={pending} required />
      <p ref={errorNode} tabIndex={-1} role={error ? 'alert' : 'status'} className={error ? 'text-sm text-destructive' : undefined}>{pending ? a('pending') : error ? a(accountErrorKey(error.code)) : ''}</p>
      <Button className="self-start" disabled={pending}>{a(kind)}</Button>
    </form><a href={authURL(kind === 'login' ? 'register' : 'login', target)} className="text-ink underline underline-offset-4">{a(kind === 'login' ? 'register' : 'login')}</a>
  </section>
}
