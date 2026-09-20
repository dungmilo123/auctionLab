import { useEffect, useRef, useState } from 'react'
import { ConnectionLine } from '@/components/ConnectionLine'
import LanguageControl from '@/components/LanguageControl'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { formatUSD } from '@/lib/format'
import { useLanguage, type CopyKey } from '@/lib/i18n'
import { keepRejection, type HeldRejection } from '@/lib/rejection'
import { isSealedAuction } from '@/lib/screen'
import {
  openPlayerSocket,
  readStoredToken,
  type ConnectionStatus,
  type PlayerConnection,
} from '@/ws/client'
import type { ErrorPayload, PlayerSnapshot } from '@/ws/protocol'
import { RESTORE_BOUND_MS, restoreActive, type RestoreSignal } from '@/ws/reconnect'
import { initialViewState, type ViewState } from '@/ws/view'
import Finale from './Finale'
import ItemIntro from './ItemIntro'
import JoinForm from './JoinForm'
import LiveDutch from './LiveDutch'
import LiveEnglish from './LiveEnglish'
import LiveSealed from './LiveSealed'
import Lobby from './Lobby'
import SettledReveal from './SettledReveal'

// The player route (02/03-UI-SPEC.md §Player Route & Interaction Contract).
//
// One card, one header row, one shared status line, the two D-131/D-134
// recovery rows, and a body that switches on the phase of the reduced socket
// state (D-70). Everything the player sees is either the dictionary (bilingual,
// EN default) or a value the server sent — never a server message and never a
// string invented at the call site.
//
// Two socket callbacks feed this route, and they are the same decoded frame:
//   - `onView` is the authoritative reduction (`ViewState`) and the ONLY source
//     of the phase switch, the item, the player's own row and the LIVE overlay;
//   - `onSnapshot` carries the Phase-2 self-sufficient snapshot that the shipped
//     LOBBY body renders from (its prop type is deliberately unchanged, so the
//     Phase 2 screen stays byte-identical).
// Both are set from one frame in one React batch, so they can never disagree.

// `error.code` selects the copy; the server's `message` is never rendered
// (T-02-01). An unexpected code falls through to the generic sentence rather
// than showing the player a protocol string. `rate_limited` never reaches here:
// the socket layer keeps the informational bucket answer at the console
// (D-138/T-07-11), so no route can paint a row for it.
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

/**
 * D-53's private refusal, held WITH the item it was refused for.
 *
 * The rule — and its one narrowing, D-97's `already_sold` lifetime — lives in
 * `lib/rejection.ts`, where the node suite pins it (WR-04). This route only
 * applies it and renders the held payload: a refusal is feedback, not a patch,
 * so it is deliberately not view state.
 */
function PlayerView() {
  const { t } = useLanguage()
  const [status, setStatus] = useState<ConnectionStatus>('reconnecting')
  // G-1: the machine's own first-open boolean, mirrored here only so the word
  // can split `reconnecting` into `Connecting…` / `Reconnecting…`. The rule
  // itself stays in `reconnect.ts` (`connectionWord`).
  const [lastOpenAt, setLastOpenAt] = useState<number | null>(null)
  const [nickname, setNickname] = useState('')
  const [joining, setJoining] = useState(false)
  // A stored token means the session can be restored, so the join form must not
  // flash before the server has answered the resume attempt (D-30) — and D-134
  // bounds that wait rather than letting it become a dead end.
  const [hasToken, setHasToken] = useState(() => readStoredToken() !== '')
  const [errorKey, setErrorKey] = useState<CopyKey | null>(null)
  const [snapshot, setSnapshot] = useState<PlayerSnapshot | null>(null)
  // The phase body's state, reduced from every admitted frame (D-57/D-70).
  const [view, setView] = useState<ViewState>(() => initialViewState())
  // D-53's private refusal, held only long enough for the feedback row: it is
  // not view state, and it clears on the next edit or the next snapshot — with
  // ONE narrowing (D-97): the Dutch race loser's `already_sold` notice arrives
  // after the settle's own snapshot and survives the later snapshots for the
  // same item. See `keepRejection`.
  const [rejection, setRejection] = useState<HeldRejection | null>(null)
  // D-134's bound: the instant it is measured from, the clock it is read
  // against, and the FIRST clearing signal that arrived (the closed vocabulary
  // `RESTORE_CLEARING_SIGNALS` declares). The pure rule both read is
  // `restoreActive`; the timer below is what advances `restoreNow` to the bound.
  const [mountedAt] = useState(() => Date.now())
  const [restoreNow, setRestoreNow] = useState(mountedAt)
  const [restoreClearedBy, setRestoreClearedBy] = useState<RestoreSignal | null>(null)
  // Mirrors `lastOpenAt` for the callback closure, which is created once.
  const openedRef = useRef(false)
  const connectionRef = useRef<PlayerConnection | null>(null)
  // The item the reduced view is on RIGHT NOW. The socket callbacks are created
  // once (empty deps), so a refusal that has to record its own scope reads the
  // current item id from here instead of from a stale closure.
  const itemIdRef = useRef<string | null>(null)

  // The socket is created INSIDE this effect and closed in its cleanup. At
  // module scope it would open twice under StrictMode and supersede itself —
  // and the superseded connection's 4001 must never surface as an error.
  useEffect(() => {
    const connection = openPlayerSocket({
      onStatus: (next) => {
        if (next === 'live') {
          openedRef.current = true
          setLastOpenAt(Date.now())
        } else if (openedRef.current) {
          // D-134: the FIRST close after an open ends the restoring line early
          // — the first failure. A page that never opened is bounded by the
          // timer alone; the machine deliberately reports the same
          // `reconnecting` for both, so this is the one close a route can
          // observe without a second socket capability.
          setRestoreClearedBy((prev) => prev ?? 'close')
        }
        setStatus(next)
      },
      onView: (next) => {
        itemIdRef.current = next.item?.id ?? null
        setView(next)
      },
      onSnapshot: (next) => {
        setSnapshot(next)
        setJoining(false)
        setRestoreClearedBy((prev) => prev ?? 'snapshot')
        setErrorKey(null)
        setRejection((prev) => keepRejection(prev, next))
      },
      onBidRejected: (next) => setRejection({ payload: next, itemId: itemIdRef.current }),
      // D-78/D-80: the presenter ended this identity's session (a cleared room
      // or a released row) and the server closed the socket with 4002. The body
      // on screen describes a lobby that no longer exists, so the route returns
      // to the join form's entry state. The token is gone with the identity, so
      // the session block must not offer to restore it.
      onSessionEnded: () => {
        setHasToken(false)
        setRestoreClearedBy((prev) => prev ?? 'session_ended')
        setView(initialViewState())
        setSnapshot(null)
        setJoining(false)
        setErrorKey(null)
        setRejection(null)
      },
      onError: (error: ErrorPayload) => {
        // D-30 (3): `token_invalid` deletes the stored token and the session
        // block disappears with it — the form's `error.sessionExpired` line is
        // the explanation and the form is the path.
        if (error.code === 'token_invalid') setHasToken(false)
        setRestoreClearedBy(
          (prev) => prev ?? (error.code === 'token_invalid' ? 'token_invalid' : 'error'),
        )
        setJoining(false)
        setErrorKey(errorCopyKey(error.code))
      },
    })
    connectionRef.current = connection

    return () => {
      connectionRef.current = null
      connection.close()
    }
  }, [])

  // D-134's bound line: true only while the mount is younger than
  // `RESTORE_BOUND_MS` and no clearing signal has arrived. The session block
  // renders only while the block itself is visible (a stored token, no snapshot
  // yet, not superseded), and the timer is cleared whenever the line stops
  // rendering — the effect's own cleanup, plus the unmount case.
  const sessionBlockVisible = view.me === null && hasToken && status !== 'superseded'
  const restoringLine =
    sessionBlockVisible &&
    restoreActive({
      mountedAt,
      now: restoreNow,
      clearedBy: restoreClearedBy,
    })

  useEffect(() => {
    if (!restoringLine) return
    const remaining = Math.max(0, mountedAt + RESTORE_BOUND_MS - Date.now())
    const id = window.setTimeout(() => setRestoreNow(Date.now()), remaining)
    return () => window.clearTimeout(id)
  }, [restoringLine, mountedAt])

  // G-8's single ordering: the session block's retry WINS when it is present,
  // so the socket retry renders only while the state is offline AND no session
  // block owns the recovery — at most one retry control per screen-state.
  const socketRetryVisible = status === 'offline' && !sessionBlockVisible

  function retrySocket() {
    // One capability, three call sites: the offline retry, the session block's
    // retry and the superseded notice's "Use this device" all reach the same
    // `retry()` — reset the backoff, connect now, re-send the stored token.
    connectionRef.current?.retry()
  }

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

  function placeAccept() {
    // D-95/D-104: the Dutch acceptance carries NO value at all — the server
    // derives the price it charges at the instant it handles the frame, so
    // there is nothing here a tampered client could inflate or discount
    // (T-05-01). One tap sends exactly this one frame.
    connectionRef.current?.send({ type: 'dutch.accept' })
  }

  function submitSealed(amount: number) {
    // D-111/D-119: the amount is the raw integer the field parsed; the frame is
    // `sealed.submit` and the server owns every rule (D-52). An edit rides the
    // same command — the engine upserts `(item_id, player_id)`.
    connectionRef.current?.send({ type: 'sealed.submit', amount })
  }

  function renderBody() {
    // No snapshot has been applied yet: the player has not joined (or is
    // resuming), so the join column is the only body that exists.
    if (view.me === null) {
      // D-134's bound: while it runs, the restoring line stands INSTEAD of the
      // form (the carried D-30 shape — the form must not flash before the
      // server has answered) but always with an attempt in flight and for at
      // most `RESTORE_BOUND_MS`. After the bound (or the first failure) the
      // normal join form plus the session retry render — there is no state in
      // which the player has no path forward.
      if (restoringLine) {
        return <p className="text-base leading-[1.5]">{t('session.restoring')}</p>
      }
      return (
        <>
          {sessionBlockVisible && (
            <>
              <p className="text-sm leading-[1.5] text-muted-foreground">
                {t('session.restoreNote')}
              </p>
              {/* Disabled only while the socket is already open: the retry is
                  the path when the resume attempt has failed or is still
                  silent, and there is nothing to reset once it is live. */}
              <Button
                type="button"
                variant="outline"
                className="min-h-11 w-full"
                disabled={status === 'live'}
                onClick={retrySocket}
              >
                {t('session.retry')}
              </Button>
            </>
          )}
          <JoinForm
            open={status === 'live'}
            joining={joining}
            nickname={nickname}
            onNicknameChange={changeNickname}
            onSubmit={submitJoin}
            errorKey={errorKey}
          />
        </>
      )
    }

    // A held refusal is only ever visible on the item it was refused for: the
    // item id it carries is compared with the item on screen now, so a stale
    // notice cannot paint a later item's body whatever the clear rules did —
    // the scope is structural, not a matter of frame order.
    const held =
      rejection !== null && rejection.itemId === (view.item?.id ?? null) ? rejection.payload : null

    switch (view.phase) {
      case 'LIVE':
        // The LIVE body must render `price.tick` and `bid.accepted` updates as
        // well as the snapshot, so an anti-snipe deadline jump and a moving
        // `min_next_bid` are visible without waiting for a snapshot (D-56/D-71).
        //
        // D-103: the LIVE arm branches on the item's own type BEFORE the
        // English body is chosen. Phase 6 adds the sealed arm (D-119): a sealed
        // run has no price and no ladder, so it gets its own body — the
        // `isSealedAuction` predicate is the app's one definition of the two
        // sealed types, never a literal compared here. `LiveEnglish` keeps its
        // `PendingLive` fallback as the whole family's defensive body.
        return view.item?.auctionType === 'dutch' ? (
          <LiveDutch
            view={view}
            status={status}
            rejection={held}
            onClearRejection={() => setRejection(null)}
            onAccept={placeAccept}
          />
        ) : isSealedAuction(view.item?.auctionType ?? '') ? (
          <LiveSealed
            view={view}
            status={status}
            rejection={held}
            onClearRejection={() => setRejection(null)}
            onSubmit={submitSealed}
          />
        ) : (
          <LiveEnglish
            view={view}
            status={status}
            rejection={held}
            onClearRejection={() => setRejection(null)}
            onPlaceBid={placeBid}
          />
        )
      case 'SETTLED':
        // D-97: the race loser's scoped refusal rides the reveal, so the one
        // surface that can carry "why my tap did not count" is the body that is
        // actually on screen when the refusal lands.
        return <SettledReveal item={view.item} me={view.me} rejection={held} />
      case 'FINALE':
        return <Finale view={view} />
      case 'ITEM_INTRO':
        if (view.item !== null) return <ItemIntro item={view.item} budget={view.me.budget} />
        break
      default:
        break
    }

    // LOBBY keeps its shipped Phase-2 body, rendered from the snapshot it was
    // written for. The last known body deliberately stays on screen through an
    // `offline` or `superseded` connection: a phone that lost signal must not
    // lose the room it is in. (The `null` arm is unreachable — `me` and the
    // snapshot arrive from the same frame — and is written out only to keep
    // this function total.)
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
            is announced — not just shown by the square's colour. D-130's retry
            renders here only while the join column below does not own it (G-8). */}
        <ConnectionLine
          status={status}
          lastOpenAt={lastOpenAt}
          region
          onRetry={socketRetryVisible ? retrySocket : undefined}
        />

        {view.me !== null && (
          <div className="border-2 px-3 py-2">
            <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
            <p className="text-xl leading-[1.2] font-bold">{formatUSD(view.me.budget)}</p>
          </div>
        )}

        {/* D-131: the terminal 4001 notice and its one deliberate rejoin. The
            stored token is KEPT (it is the identity the action re-claims), the
            last known body stays on screen, and this row plus the action under
            it are the only way back on this page instance. Player-only by
            construction: 4001 is token-scoped, so no other role can reach it
            (G-10). The notice is assertive — a terminal fact the player's next
            action depends on. */}
        {status === 'superseded' && (
          <>
            <p
              role="alert"
              className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]"
            >
              {t('session.supersededNote')}
            </p>
            <Button
              type="button"
              variant="outline"
              className="min-h-11 w-full"
              onClick={retrySocket}
            >
              {t('session.useThisDevice')}
            </Button>
          </>
        )}

        {renderBody()}
      </Card>
    </main>
  )
}

export default PlayerView
