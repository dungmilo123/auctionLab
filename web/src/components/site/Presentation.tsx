import { PackageOpen } from 'lucide-react'
import { Icon } from './Icon'
import { useState, type ReactNode } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSiteCopy } from '@/lib/site-copy'

export type LotStatus = 'upcoming' | 'live' | 'ended' | 'sold' | 'unavailable'

export function StatusBadge({ status }: { status: LotStatus }) {
  const { t } = useSiteCopy()
  return <span className={`status status-${status}`}>{t(status)}</span>
}

export function PreviewNotice() {
  const { t } = useSiteCopy()
  return <div className="site-container preview-notice"><strong>{t('preview')}</strong><span>{t('previewNote')}</span></div>
}

export function LotImage({ src, alt, eager = false }: { src?: string; alt: string; eager?: boolean }) {
  const { t } = useSiteCopy()
  const [failedSrc, setFailedSrc] = useState<string | undefined>()
  const missing = !src || failedSrc === src
  return <figure className="lot-image">
    <img key={src} src={missing ? '/site-assets/image-unavailable.svg' : src}
      alt={missing ? t('imageUnavailable') : alt} width="640" height="480" loading={eager ? 'eager' : 'lazy'} decoding="async"
      onError={(event) => {
        if (!missing) setFailedSrc(src)
        else event.currentTarget.style.visibility = 'hidden'
      }} />
    {missing && <figcaption>{t('imageUnavailable')}</figcaption>}
  </figure>
}

export function LotCard({ title, metadata, status, image, href }: {
  title: string; metadata: string; status: LotStatus; image?: string; href: string
}) {
  const { t } = useSiteCopy()
  return <article className="lot-card">
    <LotImage src={image} alt={t('imageAlt', { object: title })} />
    <div className="lot-body stack-small">
      <p className="eyebrow">{t('illustration')}</p>
      <h3><a href={href}>{title}</a></h3>
      <p className="muted">{metadata}</p>
      <StatusBadge status={status} />
      <p className="muted numeric">{t('price')}: {t('unavailable')}</p>
    </div>
  </article>
}

export function Field({ id, label, help, error, ...props }: {
  id: string; label: string; help: string; error?: string
} & Omit<React.ComponentProps<typeof Input>, 'id'>) {
  return <div>
    <Label htmlFor={id}>{label}</Label>
    <Input {...props} id={id} aria-invalid={error ? true : undefined}
      aria-describedby={`${id}-help${error ? ` ${id}-error` : ''}`} />
    <p id={`${id}-help`} className="field-help">{help}</p>
    {error && <p id={`${id}-error`} className="field-error">{error}</p>}
  </div>
}

export function UnavailableState({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useSiteCopy()
  return <div className="empty-state stack">
    <Icon icon={PackageOpen} size={32} />
    <h2>{title}</h2><p className="muted">{children}</p>
    <StatusBadge status="unavailable" />
    <a className="site-button site-button-outline" href="/lots">{t('browseLots')}</a>
  </div>
}

export function PageHeading({ title, intro }: { title: string; intro?: string }) {
  return <div className="stack"><h1>{title}</h1>{intro && <p className="lede">{intro}</p>}</div>
}

export function StatusExamples() {
  const { t } = useSiteCopy()
  const statuses: LotStatus[] = ['upcoming', 'live', 'ended', 'sold', 'unavailable']
  return <section className="stack-small" aria-label={t('statusExamples')}>
    <p className="muted">{t('statusExamples')}</p>
    <div className="status-list">{statuses.map(status => <StatusBadge key={status} status={status} />)}</div>
  </section>
}
