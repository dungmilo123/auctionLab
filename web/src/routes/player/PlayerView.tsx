import { useEffect, useRef, useState } from 'react'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import {
  LANGS,
  LanguageProvider,
  readStoredLang,
  translate,
  writeStoredLang,
  type CopyKey,
  type Lang,
} from '@/lib/i18n'
import {
  openPlayerSocket,
  readStoredToken,
  type ConnectionStatus,
  type PlayerConnection,
} from '@/ws/client'
import type { ErrorPayload, PlayerSnapshot } from '@/ws/protocol'
import JoinForm from './JoinForm'
import Lobby from './Lobby'

// The player route (02-UI-SPEC.md §Player Route & Interaction Contract).
//
// One card, two screens, and one polite status line above them. Everything the
// player sees is either the dictionary (bilingual, EN default) or a value from
// the server's self-sufficient snapshot (D-25) — never a server message and
// never a string invented at the call site.

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
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [nickname, setNickname] = useState('')
  const [joining, setJoining] = useState(false)
  // A stored token means the session can be restored, so the form must not
  // flash before the server has answered the resume attempt (D-30).
  const [restoring, setRestoring] = useState(() => readStoredToken() !== '')
  const [errorKey, setErrorKey] = useState<CopyKey | null>(null)
  const [snapshot, setSnapshot] = useState<PlayerSnapshot | null>(null)
  const [lang, setLangState] = useState<Lang>(() => readStoredLang())
  const connectionRef = useRef<PlayerConnection | null>(null)

  // The socket is created INSIDE this effect and closed in its cleanup. At
  // module scope it would open twice under StrictMode and supersede itself —
  // and the superseded connection's 4001 must never surface as an error.
  useEffect(() => {
    const connection = openPlayerSocket({
      onStatus: setStatus,
      onSnapshot: (next) => {
        setSnapshot(next)
        setJoining(false)
        setRestoring(false)
        setErrorKey(null)
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

  // The static document is served in the default language; the toggle keeps the
  // element in sync so a screen reader announces Vietnamese with the right
  // voice.
  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  function changeLang(next: Lang) {
    setLangState(next)
    writeStoredLang(next)
  }

  function submit() {
    const connection = connectionRef.current
    if (connection === null) return
    setJoining(true)
    setErrorKey(null)
    // The raw value is sent: the server owns normalization (D-27), and a
    // rejection comes back typed on the same socket, which stays usable so the
    // player can correct the name without a reload.
    connection.join(nickname)
  }

  const t = (key: CopyKey) => translate(lang, key)

  function renderBody() {
    // The last known lobby stays on screen through a lost or superseded
    // connection — a phone that lost signal must not lose the room it is in.
    if (snapshot !== null) return <Lobby snapshot={snapshot} />
    if (restoring) return <p className="text-base leading-[1.5]">{t('session.restoring')}</p>
    return (
      <JoinForm
        open={status === 'open'}
        joining={joining}
        nickname={nickname}
        onNicknameChange={setNickname}
        onSubmit={submit}
        errorKey={errorKey}
      />
    )
  }

  return (
    <LanguageProvider lang={lang} setLang={changeLang}>
      <main className="mx-auto flex w-full max-w-[720px] flex-col px-6 py-12 md:px-8 md:py-16">
        <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h1 className="text-display leading-[1.2] font-bold tracking-[-0.01em]">{t('h1')}</h1>

            {/* The group owns the outline and the hard shadow, and its segments
                drop both so they cannot stack. Language codes stay verbatim in
                both languages; the accessible name carries the language's own
                name. */}
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
                  onClick={() => changeLang(code)}
                  className={buttonVariants({
                    variant: 'outline',
                    className: `min-h-11 min-w-11 border-0 shadow-none ${
                      lang === code ? 'bg-background text-foreground' : 'bg-card text-muted-foreground'
                    }`,
                  })}
                >
                  {code.toUpperCase()}
                </button>
              ))}
            </div>
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
    </LanguageProvider>
  )
}

export default PlayerView
