import { Button } from '@/components/ui/button'
import { useLanguage, type CopyKey } from '@/lib/i18n'
import type { ConnectionStatus } from '@/ws/client'
import { connectionWord } from '@/ws/reconnect'

// The one connection indicator (G-11): a 12px outlined square beside the state
// word, replacing the three per-route `STATUS_COPY`/`STATUS_FILL` maps so the
// five-cell table has exactly one definition. The word is the message — the
// square is a cue and is `aria-hidden`, and its mandatory 2px outline is what
// keeps a fill perceivable against the page (never a bare filled square).
//
// The indicator itself is fixed; only three things are role-dependent, and each
// is a carried rule rather than a new one:
//
//   - `/` and `/admin` announce a state change through one `role="status"
//     aria-live="polite"` region; `/screen` has NO region at all (a price that
//     re-announces four times a second is unusable), so the caller asks for one
//     only by passing `region`;
//   - the `live` fill is accent on `/` and `/admin` and near-black on `/screen`
//     (§Color: the projector's square is never accent), passed as `liveFill`;
//   - the projector renders the word at its carried Caption role (`muted`).
//
// The retry control and the offline note are the same story: a role that passes
// a callback gets the control, and `/screen` passes none — its read-only
// guarantee stays structural, not a promise in a comment (G-3). The component
// renders the note and the retry OUTSIDE the live region, so a state change is
// announced exactly once and the arrival of a button is not re-announced. The
// retry renders only in `offline`, where no socket is open by definition, so it
// is never presented while unusable (D-82a) — and it is never disabled.

/** The square's fill in every state but `live`, whose fill is the caller's variant. */
const STATUS_FILL: Readonly<Record<Exclude<ConnectionStatus, 'live'>, string>> = {
  reconnecting: 'bg-muted-foreground',
  offline: 'bg-destructive',
  superseded: 'bg-destructive',
}

export type ConnectionLineProps = {
  status: ConnectionStatus
  /**
   * This page instance's last successful open, or null if it never opened — the
   * machine's own first-open boolean, which selects G-1's word for
   * `reconnecting` (`Connecting…` before the first open, `Reconnecting…` after).
   */
  lastOpenAt: number | null
  /** The `live` square's fill: accent on `/` and `/admin`, near-black on `/screen`. */
  liveFill?: string
  /** `/` and `/admin` wrap the square and word in one polite live region. */
  region?: boolean
  /** `/screen` renders the word at the carried muted Caption role. */
  muted?: boolean
  /** The offline note (the admin route's own), rendered under the word. */
  noteKey?: CopyKey
  /**
   * The manual retry (D-130), rendered only while the state is `offline`.
   * Omitted by `/screen` (no handler, G-3) and by the player while its session
   * block owns the single recovery control (G-8).
   */
  onRetry?: () => void
}

export function ConnectionLine({
  status,
  lastOpenAt,
  liveFill = 'bg-primary',
  region = false,
  muted = false,
  noteKey,
  onRetry,
}: ConnectionLineProps) {
  const { t } = useLanguage()
  const fill = status === 'live' ? liveFill : STATUS_FILL[status]

  const squareAndWord = (
    <>
      <span
        aria-hidden="true"
        className={`inline-block size-3 border-2 border-border ${fill}`}
      />
      <p
        className={
          muted
            ? 'text-sm leading-[1.5] text-muted-foreground'
            : 'text-sm leading-[1.5] font-bold'
        }
      >
        {t(connectionWord(status, lastOpenAt))}
      </p>
    </>
  )

  return (
    <>
      {region ? (
        <div role="status" aria-live="polite" className="flex items-center gap-2">
          {squareAndWord}
        </div>
      ) : (
        <div className="flex items-center gap-2">{squareAndWord}</div>
      )}

      {/* D-130's recovery row: the reworded note names both recoveries, and the
          fix sits where the problem is named — directly under it. */}
      {status === 'offline' && noteKey !== undefined && (
        <p className="text-sm leading-[1.5] text-muted-foreground">{t(noteKey)}</p>
      )}

      {status === 'offline' && onRetry !== undefined && (
        <Button type="button" variant="outline" className="min-h-11 w-full" onClick={onRetry}>
          {t('status.retry')}
        </Button>
      )}
    </>
  )
}
