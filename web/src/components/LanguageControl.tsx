import { cn } from 'cn'
import { buttonVariants } from '@/components/ui/button-variants'
import { LANGS, useLanguage } from '@/lib/i18n'

// Two exports, one pattern (03-UI-SPEC §Component Inventory).
//
// `SegmentedChoice` is the segmented control the UI-SPEC calls for: a
// `role="group"` wrapper that owns the 2px outline and the hard shadow, with
// segments COMPOSED from the shipped `button` primitive. It is the Phase 2
// language control generalised — the admin editor's `Auction type` (4 segments)
// and `Anti-snipe` (2 segments) are the same control over a different enum, and
// no token, component library entry or dependency is added to get them.
//
// `cn` (not a raw `buttonVariants({ className })` pass-through) resolves the
// `border-2`/`border-border`/`bg-background` overrides by merge rather than by
// stylesheet order — cva concatenates, so a segment's own surface could
// otherwise lose to the variant's. The group owns the outline and the shadow;
// the segments drop both so they cannot stack. `aria-pressed` carries the
// selection and `aria-label` carries the accessible name where the visible
// label is a technical token (`EN`/`VI`, `+10k`).
export type Segment<T extends string> = {
  value: T
  /** the visible label — a dictionary string or a verbatim technical token */
  label: string
  /** the accessible name, when the visible label alone would not read */
  accessibleName?: string
}

export function SegmentedChoice<T extends string>({
  label,
  selected,
  segments,
  onSelect,
}: {
  label: string
  selected: T
  segments: readonly Segment<T>[]
  onSelect(value: T): void
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex border-2 border-border shadow-(--shadow-hard)"
    >
      {segments.map((segment) => (
        <button
          key={segment.value}
          type="button"
          aria-pressed={selected === segment.value}
          aria-label={segment.accessibleName ?? segment.label}
          onClick={() => onSelect(segment.value)}
          className={cn(
            buttonVariants({ variant: 'outline' }),
            'min-h-11 min-w-11 border-0 shadow-none',
            selected === segment.value
              ? 'bg-background text-foreground'
              : 'bg-card text-muted-foreground',
          )}
        >
          {segment.label}
        </button>
      ))}
    </div>
  )
}

// The shared language control: one provider above the route switch (see App),
// one storage key, and the same control on `/` and `/admin`. Language codes stay
// verbatim in both languages; the accessible name carries the language's own
// name so a screen reader announces it in the right voice.
function LanguageControl() {
  const { lang, setLang, t } = useLanguage()

  return (
    <SegmentedChoice
      label={t('lang.groupLabel')}
      selected={lang}
      segments={LANGS.map((code) => ({
        value: code,
        label: code.toUpperCase(),
        accessibleName: code === 'vi' ? t('lang.vi') : t('lang.en'),
      }))}
      onSelect={setLang}
    />
  )
}

export default LanguageControl
