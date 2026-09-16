import { useEffect, useRef, useState } from 'react'
import LanguageControl from '@/components/LanguageControl'
import { Card } from '@/components/ui/card'
import { useLanguage, type CopyKey } from '@/lib/i18n'
import {
  openPlayerSocket,
  readStoredToken,
  type ConnectionStatus,
  type PlayerConnection,
} from '@/ws/client'
import type { BidRejected, ErrorPayload, PlayerSnapshot } from '@/ws/protocol'
import { initialViewState, type ViewState } from '@/ws/view'
import Finale from './Finale'
import ItemIntro from './ItemIntro'
import JoinForm from './JoinForm'
import LiveEnglish from './LiveEnglish'
import Lobby from './Lobby'
import SettledReveal from './SettledReveal'

// The player route (02/03-UI-SPEC.md §Player Route & Interaction Contract).
//
// One card, one header row, one polite status line, and a body that switches on
// the phase of the reduced socket state (D-70). Everything the player sees is
// either the dictionary (bilingual, EN default) or a value the server sent —
// never a server message and never a string invented at the call site.
//
// Two socket callbacks feed this route, and they are the same decoded frame:
//   - `onView` is the authoritative reduction (`ViewState`) and the ONLY source
//     of the phase switch, the item, the player's own row and the LIVE overlay;
//   - `onSnapshot` carries the Phase-2 self-sufficient snapshot that the shipped
//     LOBBY body renders from (its prop type is deliberately unchanged, so the
//     Phase 2 screen stays byte-identical).
// Both are set from one frame in one React batch, so they can never disagree.

const STATUS_COPY: Record<ConnectionStatus, CopyKey> = {
  connecting: 'status.connecting',
  open: 'status.open',
  lost: 'status.lost',
  superseded: 'status.superseded',
}

// The square's fill is a cue, never the message: the state text always sits
// beside it. The 2px outline is what keeps the accent fill perceivable against
// the page, so the square is never rendered as a bare accent block.
const STATUS_FILL: Record<ConnectionStatus, string> = {
  connecting: 'bg-muted-foreground',
  open: 'bg-primary',
  lost: 'bg-destructive',
  superseded: 'bg-destructive',
}

// `error.code` selects the copy; the server's `message` is never rendered
// (T-02-01). An unexpected code falls through to the generic sentence rather
// than showing the player a protocol string.
function errorCopyKey(code: string): CopyKey {
  switch (code) {
    case 'nickname_taken':
      return 'error.nicknameTaken'
    case 'nickname_invalid':
      return 'error.nicknameInvalid'
    case 'token_invalid':
      return 'error.sessionExpired'
    default:
      return 'error.generic'
  }
}

function PlayerView() {
  const { t } = useLanguage()
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [nickname, setNickname] = useState('')
  const [joining, setJoining] = useState(false)
  // A stored token means the session can be restored, so the form must not
  // flash before the server has answered the resume attempt (D-30).
  const [restoring, setRestoring] = useState(() => readStoredToken() !== '')
  const [errorKey, setErrorKey] = useState<CopyKey | null>(null)
  const [snapshot, setSnapshot] = useState<PlayerSnapshot | null>(null)
  // The phase body's state, reduced from every admitted frame (D-57/D-70).
  const [view, setView] = useState<ViewState>(() => initialViewState())
  // D-53's private refusal, held only long enough for the feedback row: it is
  // not view state, and it clears on the next edit or the next snapshot.
  const [rejection, setRejection] = useState<BidRejected | null>(null)
  const connectionRef = useRef<PlayerConnection | null>(null)

  // The socket is created INSIDE this effect and closed in its cleanup. At
  // module scope it would open twice under StrictMode and supersede itself —
  // and the superseded connection's 4001 must never surface as an error.
  useEffect(() => {
    const connection = openPlayerSocket({
      onStatus: setStatus,
      onView: setView,
      onSnapshot: (next) => {
        setSnapshot(next)
        setJoining(false)
        setRestoring(false)
        setErrorKey(null)
        setRejection(null)
      },
      onBidRejected: setRejection,
      // D-78/D-80: the presenter ended this identity's session (a cleared room
      // or a released row) and the server closed the socket with 4002. The body
      // on screen describes a lobby that no longer exists, so the route returns
      // to the join form's entry state. The status line then reports the closed
      // socket, and reload is the recovery (the phase's accepted one).
      onSessionEnded: () => {
        setView(initialViewState())
        setSnapshot(null)
        setJoining(false)
        setRestoring(false)
        setErrorKey(null)
        setRejection(null)
      },
      onError: (error: ErrorPayload) => {
        setJoining(false)
        setRestoring(false)
        setErrorKey(errorCopyKey(error.code))
      },
    })
    connectionRef.current = connection

    return () => {
      connectionRef.current = null
      connection.close()
    }
  }, [])

  function changeNickname(value: string) {
    setNickname(value)
    // The nickname rejection replaces itself as soon as the player starts
    // fixing the value (UI-SPEC §Join/mutation states).
    setErrorKey(null)
  }

  function submitJoin() {
    const connection = connectionRef.current
    if (connection === null) return
    setJoining(true)
    setErrorKey(null)
    // The raw value is sent: the server owns normalization (D-27), and a
    // rejection comes back typed on the same socket, which stays usable so the
    // player can correct the name without a reload.
    connection.join(nickname)
  }

  function placeBid(amount: number) {
    // The amount is the raw integer the field parsed (D-71): no grouping dots
    // ever reach the wire.
    connectionRef.current?.send({ type: 'bid.place', amount })
  }

  function renderBody() {
    // No snapshot has been applied yet: the player has not joined (or is
    // resuming), so the join screen is the only body that exists.
    if (view.me === null) {
      if (restoring) return <p className="text-base leading-[1.5]">{t('session.restoring')}</p>
      return (
        <JoinForm
          open={status === 'open'}
          joining={joining}
          nickname={nickname}
          onNicknameChange={changeNickname}
          onSubmit={submitJoin}
          errorKey={errorKey}
        />
      )
    }

    switch (view.phase) {
      case 'LIVE':
        // The LIVE body must render `price.tick` and `bid.accepted` updates as
        // well as the snapshot, so an anti-snipe deadline jump and a moving
        // `min_next_bid` are visible without waiting for a snapshot (D-56/D-71).
        return (
          <LiveEnglish
            view={view}
            status={status}
            rejection={rejection}
            onClearRejection={() => setRejection(null)}
            onPlaceBid={placeBid}
          />
        )
      case 'SETTLED':
        return <SettledReveal item={view.item} me={view.me} />
      case 'FINALE':
        return <Finale view={view} />
      case 'ITEM_INTRO':
        if (view.item !== null) return <ItemIntro item={view.item} budget={view.me.budget} />
        break
      default:
        break
    }

    // LOBBY keeps its shipped Phase-2 body, rendered from the snapshot it was
    // written for. The last known body deliberately stays on screen through a
    // `lost` or `superseded` connection: a phone that lost signal must not lose
    // the room it is in. (The `null` arm is unreachable — `me` and the snapshot
    // arrive from the same frame — and is written out only to keep this
    // function total.)
    return snapshot === null ? null : <Lobby snapshot={snapshot} />
  }

  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col px-6 py-12 md:px-8 md:py-16">
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h1 className="text-display leading-[1.2] font-bold tracking-[-0.01em]">{t('h1')}</h1>

          <LanguageControl />
        </div>

        {/* One polite live region for the whole status line, so a state change
            is announced — not just shown by the square's colour. */}
        <div role="status" aria-live="polite" className="flex items-center gap-2">
          <span
            aria-hidden="true"
            className={`inline-block size-3 border-2 border-border ${STATUS_FILL[status]}`}
          />
          <p className="text-sm leading-[1.5] font-bold">{t(STATUS_COPY[status])}</p>
        </div>

        {renderBody()}
      </Card>
    </main>
  )
}

export default PlayerView
