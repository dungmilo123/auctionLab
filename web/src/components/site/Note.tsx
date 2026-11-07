import type { ReactNode } from 'react'
import { cn } from 'cn'
import { countdownLabel } from '@/lib/countdown'

/** Note stock tone; the colour always comes from real auction phase data. */
export type NoteTone = 'mint' | 'butter' | 'lilac' | 'blush' | 'paper'

const toneClass: Record<NoteTone, string> = {
  mint: 'bg-note-mint',
  butter: 'bg-note-butter',
  lilac: 'bg-note-lilac',
  blush: 'bg-note-blush',
  paper: 'bg-paper',
}

export function Note({
  tone = 'paper',
  className,
  children,
  ...props
}: React.ComponentProps<'article'> & { tone?: NoteTone }) {
  return (
    <article
      data-tone={tone}
      className={cn(
        'relative flex flex-col gap-2.5 border border-ink/10 px-4 py-5 text-ink shadow-[var(--note-shadow)] transition-[transform,box-shadow] duration-300 ease-[cubic-bezier(.22,1,.36,1)]',
        'hover:-translate-y-1 hover:rotate-0 hover:shadow-[var(--note-shadow-lift)] focus-within:rotate-0',
        'motion-reduce:transform-none motion-reduce:transition-none',
        toneClass[tone],
        className,
      )}
      {...props}
    >
      {children}
    </article>
  )
}

export function NoteMagnet() {
  return (
    <span
      aria-hidden="true"
      className="absolute -top-2.5 left-1/2 -ml-2.5 size-[22px] rounded-full bg-[radial-gradient(circle_at_32%_26%,#FFFFFF_0%,#DEDED9_38%,#ABABA5_72%,#8C8C86_100%)] shadow-[0_2px_4px_#1111114D,inset_0_-1px_2px_#1111112E]"
    />
  )
}

export function NoteCode({
  left,
  right,
}: {
  left: ReactNode
  right?: ReactNode
}) {
  return (
    <div className="flex items-baseline justify-between gap-3 font-mono text-[0.5625rem] font-medium tracking-[0.14em] uppercase text-muted-foreground">
      <span className="min-w-0">{left}</span>
      <span className="max-w-[55%] truncate text-right">{right}</span>
    </div>
  )
}

export function NoteLabel({ children }: { children: ReactNode }) {
  return (
    <span className="block font-mono text-[0.5625rem] font-medium tracking-[0.14em] uppercase text-muted-foreground">
      {children}
    </span>
  )
}

export function NoteStatus({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-x-3.5 gap-y-1">{children}</div>
  )
}

export function NotePrice({
  label,
  children,
  className,
}: {
  label?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <p className={cn('origin-left font-mono text-[0.9375rem] numeric', className)}>
      {label && <NoteLabel>{label}</NoteLabel>}
      {children}
    </p>
  )
}

/** Decorative timer: the absolute close time is always stated in text beside it. */
export function Countdown({
  close,
  now,
  className,
}: {
  close: number | null
  now: number
  className?: string
}) {
  if (close === null || !Number.isSafeInteger(close)) return null
  const closed = close - now < 1000
  const label = countdownLabel(close, now)
  return (
    <span
      aria-hidden="true"
      data-tone={closed ? 'ended' : undefined}
      className={cn(
        'inline-flex items-center gap-1.5 font-mono text-[0.8125rem] tracking-[0.02em] numeric',
        closed ? 'text-muted-foreground' : 'text-ink',
        className,
      )}
    >
      <span
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          closed ? 'bg-[#9C9C96]' : 'bg-[#2C6A50]',
        )}
      />
      {label}
    </span>
  )
}
