import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useLanguage, type CopyKey } from '@/lib/i18n'

// Screen A of the player route (02-UI-SPEC.md §Screen A — Join).
//
// Body order is label → input → (hint OR error) → CTA. The hint and the error
// share one slot, so a rejection REPLACES the hint instead of stacking under
// it, and there is never more than one message on screen. The three blocks are
// returned as siblings so the Card's own 16px rhythm spaces them; the
// label/field pair carries its own 8px gap.

const NICKNAME_ID = 'join-nickname'
const ERROR_ID = 'join-error'

type JoinFormProps = {
  /** The socket is open, so the one action on this screen is available. */
  open: boolean
  /** The join frame is out: the controls lock until the server answers. */
  joining: boolean
  nickname: string
  onNicknameChange(value: string): void
  onSubmit(): void
  /** Dictionary key of the current rejection, or null while there is none. */
  errorKey: CopyKey | null
}

function JoinForm({ open, joining, nickname, onNicknameChange, onSubmit, errorKey }: JoinFormProps) {
  const { t } = useLanguage()
  // Enter in the empty field does nothing: the same condition that disables the
  // CTA guards the submit path, so an accidental keystroke cannot send a blank
  // nickname for the server to reject.
  const canSubmit = open && !joining && nickname.trim() !== ''

  function submit() {
    if (canSubmit) onSubmit()
  }

  return (
    <>
      <div className="flex flex-col gap-2">
        <Label htmlFor={NICKNAME_ID}>{t('form.nicknameLabel')}</Label>
        <Input
          id={NICKNAME_ID}
          // The field reads as a field against the card: the declared input
          // surface, not the primitive's transparent default.
          className="bg-input"
          value={nickname}
          placeholder={t('form.nicknamePlaceholder')}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          inputMode="text"
          // Deliberately NO maxLength: the browser counts UTF-16 code units
          // while the server caps 20 runes (D-28), so a browser cap would reject
          // legal Vietnamese names. The limit is communicated by the hint and
          // enforced by the server's typed rejection.
          disabled={joining}
          aria-invalid={errorKey !== null}
          aria-describedby={errorKey === null ? undefined : ERROR_ID}
          onChange={(event) => onNicknameChange(event.target.value)}
          onKeyDown={(event) => {
            // Enter submits — there is no form element and no page reload.
            if (event.key === 'Enter') {
              event.preventDefault()
              submit()
            }
          }}
        />
      </div>

      {errorKey === null ? (
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('form.nicknameHint')}</p>
      ) : (
        // The rejection is a form error the player must act on, which is why it
        // is announced assertively rather than politely. The destructive cue is
        // the border only: the copy stays near-black because the destructive
        // red on this surface fails the text-contrast threshold.
        <p
          id={ERROR_ID}
          role="alert"
          className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]"
        >
          {t(errorKey)}
        </p>
      )}

      <Button
        type="button"
        // The accent fill marks the one available action — and only while it is
        // actually available. A blocked CTA renders as an outline and never
        // carries the accent.
        variant={canSubmit ? 'default' : 'outline'}
        onClick={submit}
        disabled={!canSubmit}
      >
        {joining ? t('cta.joining') : t('cta.join')}
      </Button>
    </>
  )
}

export default JoinForm
