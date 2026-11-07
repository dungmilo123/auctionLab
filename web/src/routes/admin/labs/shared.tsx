import type { ReactNode } from 'react'
import { FlaskConical } from 'lucide-react'
import { Icon } from '@/components/site/Icon'
import { Button } from '@/components/ui/button'
import { LotImage } from '@/components/site/Presentation'
import { useLabsCopy } from '@/lib/labs-copy'
import { lotArt, type SeedLot } from '@/lib/labs'

export function LabHeader({ title, intro }: { title: string; intro: string }) {
  const { l } = useLabsCopy()
  return (
    <header className="flex flex-col gap-2">
      <span className="mono-label flex items-center gap-2">
        <Icon icon={FlaskConical} size={14} />
        {l('prototype')}
      </span>
      <h2>{title}</h2>
      <p className="max-w-[44rem] text-muted-foreground">{intro}</p>
    </header>
  )
}

export function Rules({ items }: { items: string[] }) {
  const { l } = useLabsCopy()
  return (
    <aside className="rounded-[2px] border border-dashed border-ink/25 p-4 text-sm">
      <p className="mono-label mb-2">{l('rules')}</p>
      <ol className="m-0 flex list-decimal flex-col gap-1 pl-5">
        {items.map((r) => <li key={r}>{r}</li>)}
      </ol>
    </aside>
  )
}

/** Single-select button group; state is announced with aria-pressed. */
export function Choice<T extends string>({
  label, value, options, onChange,
}: {
  label: string
  value: T
  options: readonly { value: T; label: ReactNode }[]
  onChange(v: T): void
}) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {options.map((o) => (
        <Button
          key={o.value}
          type="button"
          size="sm"
          variant={value === o.value ? 'default' : 'outline'}
          aria-pressed={value === o.value}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </Button>
      ))}
    </div>
  )
}

/**
 * The prototype's main surface: the object on the left, the interaction on
 * the right — the same reading order as the real lot page.
 */
export function LotStage({ lot, badge, children }: { lot: SeedLot; badge?: ReactNode; children: ReactNode }) {
  return (
    <article className="grid overflow-hidden rounded-[2px] border border-hairline bg-paper md:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
      <div className="flex flex-col justify-center gap-3 border-b border-hairline bg-muted/60 p-6 md:border-e md:border-b-0 lg:p-10">
        <LotImage src={lotArt(lot)} alt={`${lot.title} illustration`} eager className="mx-auto w-full max-w-[26rem]" />
        <div className="flex gap-2">
          {[false, true].map((d) => (
            <img key={String(d)} src={lotArt(lot, d)} alt="" aria-hidden="true" className="size-14 rounded-[2px] border border-hairline bg-paper object-contain p-1 [filter:grayscale(1)]" />
          ))}
        </div>
      </div>
      <div className="flex flex-col gap-6 p-6 lg:p-8">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex flex-col gap-1">
            <span className="mono-label">Lot {lot.n} · {lot.event}</span>
            <h3 className="!text-2xl">{lot.title}</h3>
          </div>
          {badge}
        </header>
        {children}
      </div>
    </article>
  )
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex min-w-0 flex-col gap-1.5">
      <span className="mono-label">{label}</span>
      {children}
    </label>
  )
}
