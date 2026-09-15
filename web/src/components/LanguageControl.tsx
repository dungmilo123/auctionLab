import { cn } from 'cn'
import { buttonVariants } from '@/components/ui/button-variants'
import { LANGS, useLanguage } from '@/lib/i18n'

// The shared language control (03-UI-SPEC §Component Inventory).
//
// One segmented group, lifted out of the Phase 2 player route so `/` and
// `/admin` render the SAME control from the SAME `LanguageProvider`: the
// document-language sync and the `auctionlab.lang` storage key live in `App`,
// and this component only reads the current value and writes the next one.
//
// `cn` (not a raw `buttonVariants({ className })` pass-through) resolves the
// `border-2`/`border-border`/`bg-background` overrides by merge rather than by
// stylesheet order — cva concatenates, so the segment's own surface could
// otherwise lose to the variant's. The group owns the 2px outline and the hard
// shadow; the segments drop both so they cannot stack. `aria-pressed` carries
// the selection, and the accessible name is the language's own name while the
// visible label stays the verbatim `EN`/`VI` code (both untranslated).
function LanguageControl() {
  const { lang, setLang, t } = useLanguage()

  return (
    <div
      role="group"
      aria-label={t('lang.groupLabel')}
      className="flex border-2 border-border shadow-(--shadow-hard)"
    >
      {LANGS.map((code) => (
        <button
          key={code}
          type="button"
          aria-pressed={lang === code}
          aria-label={code === 'vi' ? t('lang.vi') : t('lang.en')}
          onClick={() => setLang(code)}
          className={cn(
            buttonVariants({ variant: 'outline' }),
            'min-h-11 min-w-11 border-0 shadow-none',
            lang === code ? 'bg-background text-foreground' : 'bg-card text-muted-foreground',
          )}
        >
          {code.toUpperCase()}
        </button>
      ))}
    </div>
  )
}

export default LanguageControl
