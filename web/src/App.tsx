import { routeFor } from '@/lib/route'
import AdminView from '@/routes/admin/AdminView'
import PlayerView from '@/routes/player/PlayerView'
import ScreenView from '@/routes/screen/ScreenView'

// Route switch over the pure selector (D-04): no router dependency, no state
// provider, no history handling. The pathname is passed to the shells that
// print their own `route: … · path: …` line — the human-verifiable proof of
// BUILD-03 when /admin or /screen is refreshed inside the built binary. The
// player route is real UI now and no longer prints one.
//
// Anything that is not /admin or /screen renders the player route: that is the
// walking-skeleton fallback (01-UI-SPEC.md §Routes), revisited when real
// routing lands.
function App() {
  const path = window.location.pathname

  switch (routeFor(path)) {
    case 'admin':
      return <AdminView path={path} />
    case 'screen':
      return <ScreenView path={path} />
    default:
      return <PlayerView />
  }
}

export default App
