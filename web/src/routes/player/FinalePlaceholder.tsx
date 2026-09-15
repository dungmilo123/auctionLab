import { useLanguage } from '@/lib/i18n'

// Screen F of the player route (03-UI-SPEC §Screen F — FINALE placeholder).
//
// Two lines and nothing else. The podium, the ranking list and the player's
// personal rank are Phase 4 (ECON-05/SCRN-03): the snapshot's `leaderboard` is
// decoded and kept in the view, but it is deliberately not rendered here.
function FinalePlaceholder() {
  const { t } = useLanguage()

  return (
    <>
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('finale.heading')}</h2>
      <p className="text-base leading-[1.5]">{t('finale.body')}</p>
    </>
  )
}

export default FinalePlaceholder
