import { PackageOpen } from 'lucide-react'
import { Icon } from './Icon'
import { useState, type ReactNode } from 'react'
import { cn } from 'cn'
import { Note, NoteStatus } from './Note'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSiteCopy } from '@/lib/site-copy'

export type LotStatus = 'upcoming' | 'live' | 'ended' | 'sold' | 'unavailable'

const statusTone = { upcoming: 'upcoming', live: 'live', ended: 'ended', sold: 'sold', unavailable: 'ended' } as const

export function StatusBadge({ status }: { status: LotStatus }) {
  const { t } = useSiteCopy()
  return <Badge tone={statusTone[status]}>{t(status)}</Badge>
}

export function PreviewNotice() {
  const { t } = useSiteCopy()
  return <div className="mono-label mx-auto flex w-full max-w-[1440px] flex-wrap items-baseline gap-2 border-b border-hairline px-4 py-3 sm:px-6 lg:px-8"><strong className="font-medium text-ink">{t('preview')}</strong><span>{t('previewNote')}</span></div>
}

export function LotImage({ src, alt, eager = false, plain = false, className }: { src?: string; alt: string; eager?: boolean; plain?: boolean; className?: string }) {
  const { t } = useSiteCopy()
  const [failedSrc, setFailedSrc] = useState<string | undefined>()
  const missing = !src || failedSrc === src
  return <figure className={cn('relative m-0 overflow-hidden bg-transparent', className)}>
    <img key={src} src={missing ? '/site-assets/image-unavailable.svg' : src}
      alt={missing ? t('imageUnavailable') : alt} width="640" height="480" loading={eager ? 'eager' : 'lazy'} decoding="async"
      className={cn('block w-full [aspect-ratio:4/3] object-contain', !plain && '[filter:grayscale(1)_contrast(1.04)] mix-blend-multiply')}
      onError={(event) => {
        if (!missing) setFailedSrc(src)
        else event.currentTarget.style.visibility = 'hidden'
      }} />
    {missing && <figcaption className="pt-2 font-mono text-[0.625rem] tracking-[0.1em] uppercase text-muted-foreground">{t('imageUnavailable')}</figcaption>}
  </figure>
}

export function LotCard({ title, metadata, status, image, href }: {
  title: string; metadata: string; status: LotStatus; image?: string; href: string
}) {
  const { t } = useSiteCopy()
  return <Note className="rotate-[-0.5deg]">
    <LotImage src={image} alt={t('imageAlt', { object: title })} />
    <p className="mono-label font-normal">{t('illustration')}</p>
    <h3 className="text-[1.0625rem] leading-snug font-medium tracking-[-0.015em]"><a className="min-h-11 text-ink no-underline hover:underline" href={href}>{title}</a></h3>
    <p className="text-muted-foreground">{metadata}</p>
    <NoteStatus><StatusBadge status={status} /></NoteStatus>
    <p className="font-mono text-[0.9375rem] text-muted-foreground numeric">{t('price')}: {t('unavailable')}</p>
  </Note>
}

export function Field({ id, label, help, error, ...props }: {
  id: string; label: string; help: string; error?: string
} & Omit<React.ComponentProps<typeof Input>, 'id'>) {
  return <div>
    <Label htmlFor={id}>{label}</Label>
    <Input {...props} id={id} aria-invalid={error ? true : undefined}
      aria-describedby={`${id}-help${error ? ` ${id}-error` : ''}`} />
    <p id={`${id}-help`} className="mt-2 text-sm text-muted-foreground">{help}</p>
    {error && <p id={`${id}-error`} className="mt-2 text-sm text-destructive">{error}</p>}
  </div>
}

export function UnavailableState({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useSiteCopy()
  return <div className="flex flex-col gap-6 border border-hairline border-t-ink bg-paper px-6 py-8">
    <Icon icon={PackageOpen} size={32} />
    <h2>{title}</h2><p className="text-muted-foreground">{children}</p>
    <StatusBadge status="unavailable" />
    <a className="inline-flex min-h-11 items-center font-mono text-[0.6875rem] tracking-[0.12em] uppercase text-ink underline underline-offset-4" href="/lots">{t('browseLots')}</a>
  </div>
}

export function PageHeading({ title, intro }: { title: string; intro?: string }) {
  return <div className="flex flex-col gap-6"><h1>{title}</h1>{intro && <p className="max-w-[34rem] text-base leading-relaxed text-muted-foreground">{intro}</p>}</div>
}

export function StatusExamples() {
  const { t } = useSiteCopy()
  const statuses: LotStatus[] = ['upcoming', 'live', 'ended', 'sold', 'unavailable']
  return <section className="flex flex-col gap-2.5" aria-label={t('statusExamples')}>
    <p className="text-muted-foreground">{t('statusExamples')}</p>
    <div className="flex flex-wrap gap-4">{statuses.map(status => <StatusBadge key={status} status={status} />)}</div>
  </section>
}
