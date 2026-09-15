import { useEffect, useState } from 'react'
import { LanguageProvider, readStoredLang, writeStoredLang, type Lang } from '@/lib/i18n'
import { routeFor } from '@/lib/route'
import AdminView from '@/routes/admin/AdminView'
import PlayerView from '@/routes/player/PlayerView'
import ScreenView from '@/routes/screen/ScreenView'

// Route switch over the pure selector (D-04): no router dependency, no
// history handling. The pathname is passed to the shells that print their own
// `route: … · path: …` line — the human-verifiable proof of BUILD-03 when
// /admin or /screen is refreshed inside the built binary. The player route is
// real UI now and no longer prints one.
//
// Anything that is not /admin or /screen renders the player route: that is the
// walking-skeleton fallback (01-UI-SPEC.md §Routes), revisited when real
// routing lands.
//
// ONE language provider sits above the switch (03-UI-SPEC §Admin language
// control): `/` and `/admin` share one state and one storage key, and the
// document-language sync lives here rather than inside a route. The static
// document is served in the default language; the sync keeps a screen reader
// announcing Vietnamese with the right voice.
function App() {
  const path = window.location.pathname
  const [lang, setLang] = useState<Lang>(() => readStoredLang())

  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  function changeLang(next: Lang) {
    setLang(next)
    writeStoredLang(next)
  }

  function renderRoute() {
    switch (routeFor(path)) {
      case 'admin':
        return <AdminView />
      case 'screen':
        return <ScreenView path={path} />
      default:
        return <PlayerView />
    }
  }

  return (
    <LanguageProvider lang={lang} setLang={changeLang}>
      {renderRoute()}
    </LanguageProvider>
  )
}

export default App
