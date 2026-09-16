import { useEffect, useRef, useState } from 'react'
import { cn } from 'cn'
import LanguageControl, { SegmentedChoice } from '@/components/LanguageControl'
import { QrPanel } from '@/components/QrPanel'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatTimeOfDay, formatVND } from '@/lib/format'
import { fill, useLanguage, type CopyKey } from '@/lib/i18n'
import {
  openAdminSocket,
  type AdminCommand,
  type AdminConnection,
  type ResetMode,
} from '@/ws/admin'
import type { ConnectionStatus } from '@/ws/client'
import type { Phase } from '@/ws/protocol'
import { initialViewState, type ViewState } from '@/ws/view'
import {
  FIELD_IDS,
  STARTING_BUDGET_FIELD_ID,
  blankDraft,
  configFrame,
  draftFromItem,
  draftPayload,
  draftValid,
  mutationsEnabled,
  presenterSettle,
  releaseFrame,
  resetConfirmKey,
  resetFrame,
  sortPlayersByJoinedAt,
  urlCandidates,
  validateDraft,
  validateStartingBudget,
  visibleFields,
  type DraftFields,
  type EditorDraft,
  type PresenterSettle,
} from '@/lib/admin'

// The protected control room (ADMN-01..07, D-64/H17).
//
// Three tabs over one authenticated socket — Setup, Control, Players — in the
// fixed order 04-UI-SPEC §Admin Route & Interaction Contract declares. Three
// rules shape the whole file:
//
//   1. The SERVER is the single source of truth (contract #1). Every panel
//      renders `ViewState`, which only a snapshot replaces. Nothing here mutates
//      the catalog or the config optimistically: a reorder is a frame, a URL
//      choice is a frame, and the row that lights up is the row the next
//      snapshot returns.
//   2. Every "in flight" flag is DERIVED from the snapshot generation rather
//      than cleared by an effect: a snapshot rebuilds `items` (events keep the
//      same array), so "the list re-rendered" is a render-time fact. That is
//      what makes the snapshot the confirmation, and what keeps a rejected
//      frame from leaving a control permanently disabled.
//   3. Only the active tab's panel is mounted. Switching tabs unmounts the
//      previous one, which is what makes "the armed confirm cancels on a tab
//      change" (D-79) structural rather than an effect — and the same fact
//      discards an open item draft.
//
// The page's pure rules (validation, the socket gate, the candidate rows) live
// in `lib/admin.ts`, where Vitest pins them; this file renders them.

const STATUS_COPY: Record<ConnectionStatus, CopyKey> = {
  connecting: 'status.connecting',
  open: 'status.open',
  lost: 'status.lost',
  superseded: 'status.superseded',
}

const STATUS_FILL: Record<ConnectionStatus, string> = {
  connecting: 'bg-muted-foreground',
  open: 'bg-primary',
  lost: 'bg-destructive',
  superseded: 'bg-destructive',
}

const PHASE_KEYS: Record<Phase, CopyKey> = {
  LOBBY: 'phase.LOBBY',
  ITEM_INTRO: 'phase.ITEM_INTRO',
  LIVE: 'phase.LIVE',
  SETTLED: 'phase.SETTLED',
  FINALE: 'phase.FINALE',
}

// The same four frozen types the server validates (D-66), with the names the
// player route uses. An unknown value renders itself rather than a guess.
const AUCTION_TYPE_KEYS: Readonly<Record<string, CopyKey>> = {
  english: 'auction.english',
  dutch: 'auction.dutch',
  sealed_first: 'auction.sealedFirst',
  vickrey: 'auction.vickrey',
}

const ITEM_STATUS_KEYS: Readonly<Record<string, CopyKey>> = {
  pending: 'item.status.pending',
  intro: 'item.status.intro',
  live: 'item.status.live',
  settled: 'item.status.settled',
  skipped: 'item.status.skipped',
}

/** The two statuses whose rows are frozen: settle history must not be rewritten. */
const LOCKED_STATUSES = ['live', 'settled']

/**
 * The armed-confirm fill, composed explicitly. The shipped `destructive`
 * variant renders destructive-as-TEXT (`--destructive` on `--card`, 3.24:1),
 * which fails the 4.5:1 text threshold; `#111111` on `#E5484D` measures 4.83:1.
 * Never a bare class the button's own `cn` would have to fight.
 */
const CONFIRM_FILL = 'bg-destructive text-primary-foreground hover:bg-destructive'

/** The three tabs, in the UI-SPEC's fixed order. Control is the default (the presenter's hand). */
type AdminTab = 'setup' | 'control' | 'players'

const TABS: readonly AdminTab[] = ['setup', 'control', 'players']

const TAB_KEYS: Record<AdminTab, CopyKey> = {
  setup: 'admin.tab.setup',
  control: 'admin.controlHeading',
  players: 'admin.playersHeading',
}

const DEFAULT_TAB: AdminTab = 'control'

/**
 * The in-flight signals the Setup panel reads, one per action that waits for a
 * snapshot. `saveLanded` is the same signal one step later, and it is what
 * closes the editor: the row appearing in the list IS the confirmation.
 */
type FlightFlags = {
  saving: boolean
  saveLanded: boolean
  reordering: boolean
  budgetSaving: boolean
  urlChoosing: boolean
  resetting: boolean
  releasing: boolean
}

/** The matching "this frame just went out" events, owned by the page. */
type FlightMark = {
  saved(): void
  clearSaved(): void
  reordered(): void
  budget(): void
  url(): void
  reset(): void
  release(): void
}

/** `code` selects the copy; the server's `message` is never rendered (carried rule). */
function adminErrorKey(code: string): CopyKey {
  switch (code) {
    case 'bad_message':
      return 'admin.actionRejected'
    case 'not_authorized':
      return 'admin.notAuthorized'
    default:
      return 'error.generic'
  }
}

function typeName(type: string, t: (key: CopyKey) => string): string {
  const key = AUCTION_TYPE_KEYS[type]
  return key === undefined ? type : t(key)
}

function statusName(status: string, t: (key: CopyKey) => string): string {
  const key = ITEM_STATUS_KEYS[status]
  return key === undefined ? status : t(key)
}

/** One labeled field: label, control, and the inline error under it. */
function FieldError({ id, errorKey }: { id: string; errorKey: CopyKey | undefined }) {
  const { t } = useLanguage()
  if (errorKey === undefined) return null
  return (
    <p id={`${id}-error`} className="text-sm leading-[1.5]">
      {t(errorKey)}
    </p>
  )
}

function TextField({
  id,
  labelKey,
  value,
  onChange,
  errorKey,
  disabled,
}: {
  id: string
  labelKey: CopyKey
  value: string
  onChange(value: string): void
  errorKey: CopyKey | undefined
  disabled: boolean
}) {
  const { t } = useLanguage()
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{t(labelKey)}</Label>
      <Input
        id={id}
        className="bg-input"
        type="text"
        autoComplete="off"
        spellCheck={false}
        value={value}
        disabled={disabled}
        aria-invalid={errorKey !== undefined}
        aria-describedby={errorKey === undefined ? undefined : `${id}-error`}
        onChange={(event) => onChange(event.target.value)}
      />
      <FieldError id={id} errorKey={errorKey} />
    </div>
  )
}

function NumberFieldRow({
  id,
  labelKey,
  value,
  onChange,
  errorKey,
  disabled,
}: {
  id: string
  labelKey: CopyKey
  value: string
  onChange(value: string): void
  errorKey: CopyKey | undefined
  disabled: boolean
}) {
  const { t } = useLanguage()
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{t(labelKey)}</Label>
      <Input
        id={id}
        className="bg-input"
        // Raw digits, no grouping dots, no type="number" and no maxLength —
        // the same rule the bid field follows (D-71/FRMT-01).
        type="text"
        inputMode="numeric"
        autoComplete="off"
        value={value}
        disabled={disabled}
        aria-invalid={errorKey !== undefined}
        aria-describedby={errorKey === undefined ? undefined : `${id}-error`}
        onChange={(event) => onChange(event.target.value)}
      />
      <FieldError id={id} errorKey={errorKey} />
    </div>
  )
}

/**
 * Card 1 of the Setup tab (D-81/D-82). The field is remounted by the snapshot's
 * own budget — `key` is the server's number — so a save re-seeds it from the
 * snapshot rather than from an optimistic write, while an unrelated snapshot
 * (a join, a presence change) cannot disturb what the presenter is typing.
 */
function BudgetCard({
  startingBudget,
  online,
  saving,
  onSave,
}: {
  startingBudget: number | null
  online: boolean
  saving: boolean
  onSave(value: number): void
}) {
  const { t } = useLanguage()
  const [raw, setRaw] = useState(() => (startingBudget === null ? '' : String(startingBudget)))
  const error = validateStartingBudget(raw)

  return (
    <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
        {t('admin.setupHeading')}
      </h2>

      <div className="flex flex-col gap-2">
        <Label htmlFor={STARTING_BUDGET_FIELD_ID}>{t('admin.field.startingBudget')}</Label>
        <Input
          id={STARTING_BUDGET_FIELD_ID}
          className="bg-input"
          // Raw digits, no grouping dots, no maxLength (D-71/FRMT-01): the
          // value the server parses is the value on screen.
          type="text"
          inputMode="numeric"
          autoComplete="off"
          value={raw}
          disabled={saving}
          aria-invalid={error !== undefined}
          aria-describedby={error === undefined ? undefined : `${STARTING_BUDGET_FIELD_ID}-error`}
          onChange={(event) => setRaw(event.target.value)}
        />
        <FieldError id={STARTING_BUDGET_FIELD_ID} errorKey={error} />
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          // D-82a + D-82's own rule: an invalid budget or a dead socket must not
          // look saveable, and the field's message is the reason.
          disabled={!online || saving || error !== undefined}
          onClick={() => {
            const value = Number(raw)
            if (error !== undefined) return
            onSave(value)
          }}
        >
          {saving ? t('admin.saving') : t('admin.saveBudget')}
        </Button>
      </div>

      <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.budgetNote')}</p>
    </Card>
  )
}

/**
 * Card 2 of the Setup tab (D-85/D-86): the QR the room will scan. The selected
 * URL is real snapshot state — the empty case names its own fix instead of
 * showing a QR that encodes nothing.
 */
function JoinQrCard({ joinUrl }: { joinUrl: string }) {
  const { t } = useLanguage()
  return (
    <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t('admin.qrHeading')}</h2>

      {joinUrl === '' ? (
        // No plate, no placeholder square, and no "scan to join" caption above
        // an absent QR — the one sentence names the state and the next step.
        <p className="text-base leading-[1.5]">{t('admin.qrNoUrl')}</p>
      ) : (
        <>
          <div className="flex justify-center">
            <QrPanel url={joinUrl} size={176} />
          </div>
          {/* Selectable text, never a clipboard button: `navigator.clipboard`
              does not exist on the plain-http LAN origin this page is served
              from, so a copy control would silently do nothing on stage. */}
          <p className="break-all font-mono text-base leading-[1.5]">{joinUrl}</p>
          <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.qrNote')}</p>
        </>
      )}
    </Card>
  )
}

/** Card 3 of the Setup tab (D-83/D-84/D-86): the candidate rows. */
function JoinUrlsCard({
  joinUrls,
  selected,
  pageOrigin,
  online,
  choosing,
  onChoose,
}: {
  joinUrls: string[]
  selected: string
  pageOrigin: string
  online: boolean
  choosing: boolean
  onChoose(url: string): void
}) {
  const { t } = useLanguage()
  const rows = urlCandidates(joinUrls, pageOrigin)

  return (
    <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
        {t('admin.joinUrlsHeading')}
      </h2>
      <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.urlNote')}</p>

      {/* Never empty by construction: `urlCandidates` appends this page's own
          origin when the server's boot-time enumeration did not list it, so
          there is no blank state to invent copy for (D-86). */}
      <ul className="flex flex-col">
        {rows.map((url) => {
          const isSelected = url === selected
          return (
            <li key={url}>
              <button
                type="button"
                aria-pressed={isSelected}
                disabled={!online || choosing}
                onClick={() => onChoose(url)}
                className={cn(
                  buttonVariants({ variant: 'outline' }),
                  'flex min-h-11 w-full flex-wrap items-center justify-start gap-2 px-2 py-2 text-left',
                  // The 2px accent left stripe is the carried "this is the
                  // marked row" marker — and it never carries the state alone:
                  // the selected row also shows the word.
                  isSelected ? 'border-l-primary' : 'border-l-transparent',
                )}
              >
                <span className="min-w-0 flex-1 break-all font-mono text-base leading-[1.5]">
                  {url}
                </span>
                {isSelected && (
                  <span className="text-sm leading-[1.5] font-bold">{t('admin.urlSelected')}</span>
                )}
              </button>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

/** The whole Setup tab, in the UI-SPEC's fixed card order. */
function SetupPanel({
  view,
  online,
  send,
  flags,
  mark,
}: {
  view: ViewState
  online: boolean
  send(frame: AdminCommand): void
  flags: FlightFlags
  mark: FlightMark
}) {
  const { t } = useLanguage()
  const [armedDelete, setArmedDelete] = useState<string | null>(null)
  const [draft, setDraft] = useState<EditorDraft | null>(null)
  const editorRef = useRef<HTMLDivElement | null>(null)

  const items = [...view.items].sort((a, b) => a.orderIndex - b.orderIndex)

  // The editor closes when the save's own snapshot arrives: the row appearing
  // (or changing) in the list IS the confirmation, so no toast exists or is
  // added. A fresh draft clears the signal, so opening the editor again works.
  const editor = draft !== null && !flags.saveLanded ? draft : null
  const editorKey = editor === null ? null : editor.id === '' ? 'new' : editor.id
  const errors = editor === null ? {} : validateDraft(editor)
  // The matrix, evaluated once per render: which fields this draft's type shows
  // (and therefore which ones the Save gate validates).
  const visible = visibleFields(
    editor?.fields.auctionType ?? 'english',
    editor?.fields.antiSnipeMode ?? 'disabled',
  )
  // D-82b: the button's `disabled` prop and the messages come from ONE call.
  const saveable = editor !== null && draftValid(editor)

  const deleteArmedId =
    armedDelete !== null && items.some((item) => item.id === armedDelete) ? armedDelete : null

  // `scrollIntoView` is the one external effect this page has: opening the
  // editor brings the card into view INSTANTLY (`block: 'nearest'`, never
  // `behavior: 'smooth'` — no motion beside the inherited reduced-motion rule).
  useEffect(() => {
    if (editorKey === null) return
    editorRef.current?.scrollIntoView({ block: 'nearest' })
  }, [editorKey])

  function openDraft(next: EditorDraft) {
    mark.clearSaved()
    setArmedDelete(null)
    setDraft(next)
  }

  function editField<K extends keyof DraftFields>(key: K, value: string) {
    if (editor === null) return
    setDraft({ ...editor, fields: { ...editor.fields, [key]: value } })
  }

  function save() {
    if (editor === null) return
    const invalid = Object.keys(errors)
    if (invalid.length > 0) {
      // Blocked Save scrolls the FIRST invalid field into view so the sentence
      // that explains it is on screen.
      for (const key of Object.keys(FIELD_IDS) as (keyof typeof FIELD_IDS)[]) {
        if (invalid.includes(key)) {
          document.getElementById(FIELD_IDS[key])?.scrollIntoView({ block: 'nearest' })
          break
        }
      }
      return
    }
    mark.saved()
    send({ type: 'admin.item.upsert', item: draftPayload(editor) })
  }

  function move(itemId: string, direction: -1 | 1) {
    const ids = items.map((item) => item.id)
    const from = ids.indexOf(itemId)
    const to = from + direction
    if (from < 0 || to < 0 || to >= ids.length) return
    const next = [...ids]
    next.splice(from, 1)
    next.splice(to, 0, itemId)
    mark.reordered()
    // The COMPLETE permutation, in list order (D-66): the server validates it
    // as a full permutation and rejects without a partial write.
    send({ type: 'admin.item.reorder', ids: next })
  }

  return (
    <>
      <BudgetCard
        key={view.config === null ? 'pending' : String(view.config.startingBudget)}
        startingBudget={view.config === null ? null : view.config.startingBudget}
        online={online}
        saving={flags.budgetSaving}
        onSave={(value) => {
          mark.budget()
          send(configFrame({ starting_budget: value }))
        }}
      />

      <JoinQrCard joinUrl={view.joinUrl} />

      <JoinUrlsCard
        joinUrls={view.joinUrls}
        selected={view.joinUrl}
        pageOrigin={window.location.origin}
        online={online}
        choosing={flags.urlChoosing}
        onChoose={(url) => {
          mark.url()
          send(configFrame({ join_url: url }))
        }}
      />

      {/* Card 4 — Items, carried verbatim from Phase 3 (one behaviour change:
          D-82b's immediate validation above). */}
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
            {t('admin.itemsHeading')}
          </h2>
          <Button
            type="button"
            variant="outline"
            disabled={!online}
            onClick={() => openDraft(blankDraft())}
          >
            {t('admin.newItem')}
          </Button>
          {/* One caption for the whole list: the bare numbers below are all VND. */}
          <p className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</p>
        </div>

        {items.length === 0 ? (
          <p className="text-base leading-[1.5]">{t('admin.emptyList')}</p>
        ) : (
          <ul>
            {items.map((item, index) => {
              const locked = LOCKED_STATUSES.includes(item.status)
              return (
                <li
                  key={item.id}
                  className="flex min-h-11 flex-wrap items-center gap-2 border-t-2 border-t-border px-2 py-1 first:border-t-0"
                >
                  <span className="text-sm leading-[1.5] font-bold">{index + 1}</span>
                  {/* The name keeps a 10rem floor, so a crowded line breaks
                      BELOW it: the row's `flex-wrap` drops the action group to
                      the next line instead of squeezing the name to a sliver
                      (03-UI-SPEC.md §UI Considerations, overflow row). */}
                  <span className="min-w-40 flex-1 break-words text-base leading-[1.5]">
                    {item.name}
                  </span>
                  <span className="text-sm leading-[1.5] text-muted-foreground">
                    {typeName(item.auctionType, t)}
                  </span>
                  {/* The presenter's own view: truth_value is never public. */}
                  <span className="text-sm leading-[1.5] text-muted-foreground">
                    {formatVND(item.truthValue)}
                  </span>
                  {/* A WORD, never a colour alone. */}
                  <span className="text-sm leading-[1.5] font-bold">
                    {statusName(item.status, t)}
                  </span>

                  {deleteArmedId === item.id ? (
                    <div className="flex w-full flex-col gap-2 border-2 border-destructive bg-card px-2 py-2">
                      <p className="min-w-0 break-words text-base leading-[1.5]">
                        {fill(t('admin.deleteConfirm'), { name: item.name })}
                      </p>
                      <div className="flex flex-wrap gap-2">
                        <Button
                          type="button"
                          variant="default"
                          className={CONFIRM_FILL}
                          disabled={!online}
                          onClick={() => {
                            setArmedDelete(null)
                            send({ type: 'admin.item.delete', id: item.id })
                          }}
                        >
                          {t('admin.delete')}
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          onClick={() => setArmedDelete(null)}
                        >
                          {t('admin.keepItem')}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex flex-wrap gap-1">
                      {/* Text labels, never a bare chevron; the accessible name
                          carries the item so four "Up"s are not four of the
                          same button to a screen reader. */}
                      <Button
                        type="button"
                        variant="outline"
                        aria-label={fill(t('admin.moveUpLabel'), { name: item.name })}
                        disabled={!online || index === 0 || flags.reordering}
                        onClick={() => move(item.id, -1)}
                      >
                        {t('admin.moveUp')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        aria-label={fill(t('admin.moveDownLabel'), { name: item.name })}
                        disabled={!online || index === items.length - 1 || flags.reordering}
                        onClick={() => move(item.id, 1)}
                      >
                        {t('admin.moveDown')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        aria-label={fill(t('admin.editLabel'), { name: item.name })}
                        disabled={locked}
                        onClick={() => openDraft(draftFromItem(item))}
                      >
                        {t('admin.edit')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        aria-label={fill(t('admin.deleteLabel'), { name: item.name })}
                        disabled={locked || !online}
                        onClick={() => setArmedDelete(item.id)}
                      >
                        {t('admin.delete')}
                      </Button>
                    </div>
                  )}
                </li>
              )
            })}
          </ul>
        )}

        <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.lockedNote')}</p>
      </Card>

      {/* The editor, rendered only while a draft is open */}
      {editor !== null && (
        <Card ref={editorRef} className="gap-4 px-6 py-6 md:px-8 md:py-8">
          <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
            {editor.id === '' ? t('admin.editorNew') : t('admin.editorEdit')}
          </h2>

          <TextField
            id={FIELD_IDS.name}
            labelKey="admin.field.name"
            value={editor.fields.name}
            errorKey={errors.name}
            disabled={flags.saving}
            onChange={(value) => editField('name', value)}
          />

          <div className="flex flex-col gap-2">
            <Label>{t('admin.field.auctionType')}</Label>
            <div className="flex flex-wrap">
              <SegmentedChoice
                label={t('admin.field.auctionType')}
                selected={editor.fields.auctionType}
                segments={[
                  { value: 'english', label: t('auction.english') },
                  { value: 'dutch', label: t('auction.dutch') },
                  { value: 'sealed_first', label: t('auction.sealedFirst') },
                  { value: 'vickrey', label: t('auction.vickrey') },
                ]}
                onSelect={(value) => editField('auctionType', value)}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <NumberFieldRow
              id={FIELD_IDS.truthValue}
              labelKey="admin.field.truthValue"
              value={editor.fields.truthValue}
              errorKey={errors.truthValue}
              disabled={flags.saving}
              onChange={(value) => editField('truthValue', value)}
            />
            <NumberFieldRow
              id={FIELD_IDS.startPrice}
              labelKey="admin.field.startPrice"
              value={editor.fields.startPrice}
              errorKey={errors.startPrice}
              disabled={flags.saving}
              onChange={(value) => editField('startPrice', value)}
            />
            <NumberFieldRow
              id={FIELD_IDS.hintMin}
              labelKey="admin.field.hintMin"
              value={editor.fields.hintMin}
              errorKey={errors.hintMin}
              disabled={flags.saving}
              onChange={(value) => editField('hintMin', value)}
            />
            <NumberFieldRow
              id={FIELD_IDS.hintMax}
              labelKey="admin.field.hintMax"
              value={editor.fields.hintMax}
              errorKey={errors.hintMax}
              disabled={flags.saving}
              onChange={(value) => editField('hintMax', value)}
            />

            {visible.minIncrement && (
              <NumberFieldRow
                id={FIELD_IDS.minIncrement}
                labelKey="admin.field.minIncrement"
                value={editor.fields.minIncrement}
                errorKey={errors.minIncrement}
                disabled={flags.saving}
                onChange={(value) => editField('minIncrement', value)}
              />
            )}
            {visible.durationMs && (
              <NumberFieldRow
                id={FIELD_IDS.durationMs}
                labelKey="admin.field.durationMs"
                value={editor.fields.durationMs}
                errorKey={errors.durationMs}
                disabled={flags.saving}
                onChange={(value) => editField('durationMs', value)}
              />
            )}
            {visible.antiSnipeWindowMs && (
              <NumberFieldRow
                id={FIELD_IDS.antiSnipeWindowMs}
                labelKey="admin.field.antiSnipeWindowMs"
                value={editor.fields.antiSnipeWindowMs}
                errorKey={errors.antiSnipeWindowMs}
                disabled={flags.saving}
                onChange={(value) => editField('antiSnipeWindowMs', value)}
              />
            )}
            {visible.maxExtensions && (
              <NumberFieldRow
                id={FIELD_IDS.maxExtensions}
                labelKey="admin.field.maxExtensions"
                value={editor.fields.maxExtensions}
                errorKey={errors.maxExtensions}
                disabled={flags.saving}
                onChange={(value) => editField('maxExtensions', value)}
              />
            )}
            {visible.floorPrice && (
              <NumberFieldRow
                id={FIELD_IDS.floorPrice}
                labelKey="admin.field.floorPrice"
                value={editor.fields.floorPrice}
                errorKey={errors.floorPrice}
                disabled={flags.saving}
                onChange={(value) => editField('floorPrice', value)}
              />
            )}
            {visible.decrementStep && (
              <NumberFieldRow
                id={FIELD_IDS.decrementStep}
                labelKey="admin.field.decrementStep"
                value={editor.fields.decrementStep}
                errorKey={errors.decrementStep}
                disabled={flags.saving}
                onChange={(value) => editField('decrementStep', value)}
              />
            )}
            {visible.decrementIntervalMs && (
              <NumberFieldRow
                id={FIELD_IDS.decrementIntervalMs}
                labelKey="admin.field.decrementIntervalMs"
                value={editor.fields.decrementIntervalMs}
                errorKey={errors.decrementIntervalMs}
                disabled={flags.saving}
                onChange={(value) => editField('decrementIntervalMs', value)}
              />
            )}
          </div>

          {visible.antiSnipeMode && (
            <div className="flex flex-col gap-2">
              <Label>{t('admin.field.antiSnipe')}</Label>
              <div className="flex flex-wrap">
                <SegmentedChoice
                  label={t('admin.field.antiSnipe')}
                  selected={editor.fields.antiSnipeMode}
                  segments={[
                    { value: 'disabled', label: t('admin.antiSnipeOff') },
                    { value: 'extend', label: t('admin.antiSnipeExtend') },
                  ]}
                  onSelect={(value) => editField('antiSnipeMode', value)}
                />
              </div>
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={flags.saving}
              onClick={() => {
                setDraft(null)
                mark.clearSaved()
              }}
            >
              {t('admin.discardEdits')}
            </Button>
            <Button
              type="button"
              variant="outline"
              // D-82b: the button is disabled for as long as ANY visible field
              // is invalid — the same call that renders the messages — and
              // D-82a keeps it off while the socket cannot deliver.
              disabled={!online || flags.saving || !saveable}
              onClick={save}
            >
              {flags.saving ? t('admin.saving') : t('admin.save')}
            </Button>
          </div>
        </Card>
      )}
    </>
  )
}

/** One settle read-out row: label 14/700, value 16/400, baseline-aligned. */
function SettleRow({
  labelKey,
  value,
  caption,
}: {
  labelKey: CopyKey
  value: string
  caption: boolean
}) {
  const { t } = useLanguage()
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-2">
      <p className="text-sm leading-[1.5] font-bold">{t(labelKey)}</p>
      <p className="flex flex-wrap items-baseline gap-2 text-base leading-[1.5]">
        <span>{value}</span>
        {caption && (
          <span className="text-sm leading-[1.5] text-muted-foreground">{t('amount.caption')}</span>
        )}
      </p>
    </div>
  )
}

/**
 * The presenter's settle card (D-81/ADMN-07), rendered only at SETTLED. It is a
 * read-out: no accent, no destructive, no action — and every number on it is the
 * server's own (the reveal object the settle commit built), so the browser never
 * recomputes an outcome.
 */
function PresenterSettleCard({ card }: { card: PresenterSettle }) {
  const { t } = useLanguage()
  return (
    <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">{t(card.headingKey)}</h2>

      {card.outcome === 'unsold' && (
        <p className="text-base leading-[1.5]">{t('settle.unsoldBody')}</p>
      )}

      <div className="flex flex-col gap-2">
        {/* Omitted, never blanked: an empty winner slot would read as a missing
            value rather than as "nobody won" (ECON-03). */}
        {card.winner !== null && (
          <SettleRow labelKey="settle.winnerLabel" value={card.winner} caption={false} />
        )}
        <SettleRow labelKey="settle.priceLabel" value={formatVND(card.price)} caption />
        {/* The presenter's secret view: truth_value is never public. */}
        <SettleRow labelKey="settle.truthLabel" value={formatVND(card.truthValue)} caption />
        {card.outcome === 'sold' && (
          <SettleRow labelKey="settle.deltaLabel" value={card.deltaText} caption />
        )}
        {card.winnerBudget !== null && (
          <SettleRow
            labelKey="admin.winnerBudgetLabel"
            value={formatVND(card.winnerBudget)}
            caption
          />
        )}
      </div>
    </Card>
  )
}

/** The Control tab (ADMN-02, ADMN-03, ADMN-07) — the presenter's live hand. */
function ControlPanel({
  view,
  online,
  send,
  flags,
  mark,
}: {
  view: ViewState
  online: boolean
  send(frame: AdminCommand): void
  flags: FlightFlags
  mark: FlightMark
}) {
  const { t } = useLanguage()
  const [armedForceEnd, setArmedForceEnd] = useState<Phase | null>(null)
  const [armedReset, setArmedReset] = useState<{ mode: ResetMode; phase: Phase } | null>(null)

  const items = [...view.items].sort((a, b) => a.orderIndex - b.orderIndex)
  const currentItem = view.item
  const phase = view.phase
  const firstPending = items.find((item) => item.status === 'pending') ?? null

  // D-65's start target: the CURRENT item while it is on stage, otherwise the
  // first pending one in order. No target means nothing to start — which is a
  // caption, not a silent dead button.
  const startId = phase === 'ITEM_INTRO' && currentItem !== null ? currentItem.id : firstPending?.id ?? null
  // D-82a: the phase's own enablement AND an open socket. Every button in the
  // row derives its `disabled` prop from this pair, so a lost socket can never
  // leave a control that looks available.
  const startEnabled =
    online && (phase === 'LOBBY' || phase === 'ITEM_INTRO' || phase === 'FINALE') && startId !== null
  const forceEndEnabled = online && phase === 'LIVE'
  const nextEnabled = online && (phase === 'ITEM_INTRO' || phase === 'SETTLED')

  // The armed strips are phase-derived: a force-end that moves the phase — or a
  // reset that returns it to LOBBY — dismisses its own strip.
  const forceEndArmed = armedForceEnd !== null && armedForceEnd === phase
  const resetArmed = armedReset !== null && armedReset.phase === phase ? armedReset.mode : null
  // D-81's read-out, from the snapshot only.
  const settle = presenterSettle(phase, currentItem, view.players)

  return (
    <>
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
          {t('admin.controlHeading')}
        </h2>

        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <p className="text-sm leading-[1.5] font-bold">{t('admin.phaseLabel')}</p>
          <p className="text-base leading-[1.5]">{t(PHASE_KEYS[phase])}</p>
          <span aria-hidden="true">·</span>
          <p className="text-sm leading-[1.5] font-bold">{t('admin.currentItemLabel')}</p>
          {currentItem === null ? (
            <p className="text-base leading-[1.5]">{t('admin.noCurrentItem')}</p>
          ) : (
            <>
              <p className="min-w-0 break-words text-base leading-[1.5]">{currentItem.name}</p>
              <p className="text-sm leading-[1.5] font-bold">{statusName(currentItem.status, t)}</p>
            </>
          )}
        </div>

        {forceEndArmed ? (
          <div className="flex flex-col gap-2 border-2 border-destructive bg-card px-2 py-2">
            <p className="text-base leading-[1.5]">{t('admin.forceEndConfirm')}</p>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="default"
                className={CONFIRM_FILL}
                disabled={!online}
                onClick={() => {
                  setArmedForceEnd(null)
                  send({ type: 'admin.item.force_end' })
                }}
              >
                {t('admin.forceEnd')}
              </Button>
              <Button type="button" variant="outline" onClick={() => setArmedForceEnd(null)}>
                {t('admin.dontEnd')}
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex flex-wrap gap-2">
            {/* Fixed positions, never appearing or disappearing: the presenter's
                hand learns one layout. Exactly one of these can carry the accent
                and `Force end` never does — it is irreversible. */}
            <Button
              type="button"
              variant={startEnabled ? 'default' : 'outline'}
              disabled={!startEnabled}
              onClick={() => {
                if (startId !== null) send({ type: 'admin.item.start', id: startId })
              }}
            >
              {t('admin.start')}
            </Button>

            <Button
              type="button"
              variant="outline"
              disabled={!forceEndEnabled}
              onClick={() => setArmedForceEnd(phase)}
            >
              {t('admin.forceEnd')}
            </Button>

            <Button
              type="button"
              variant={nextEnabled && phase === 'SETTLED' ? 'default' : 'outline'}
              disabled={!nextEnabled}
              onClick={() => send({ type: 'admin.item.next' })}
            >
              {phase === 'ITEM_INTRO' ? t('admin.skipItem') : t('admin.next')}
            </Button>
          </div>
        )}

        {(phase === 'LOBBY' || phase === 'FINALE') && startId === null && (
          <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.noPendingItem')}</p>
        )}
      </Card>

      {/* The presenter settles the room reads: exactly at SETTLED, above the
          reset card, and never anywhere else. */}
      {settle !== null && <PresenterSettleCard card={settle} />}

      {/* Card 3 — Reset (ADMN-03, D-78/D-79). Both buttons are outline and
          neither is ever accent: an irreversible control must not be the most
          inviting thing on the page. They are gated by the socket alone and
          never by phase — a `keep_players` reset in a fresh lobby is a
          server-side no-op, and the server decides that, not this panel. */}
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
          {t('admin.resetHeading')}
        </h2>
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.resetNote')}</p>

        {resetArmed !== null ? (
          <div className="flex flex-col gap-2 border-2 border-destructive bg-card px-2 py-2">
            <p className="text-base leading-[1.5]">{t(resetConfirmKey(resetArmed))}</p>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="default"
                className={CONFIRM_FILL}
                disabled={!online || flags.resetting}
                onClick={() => {
                  // Disarm on the confirm click — the carried force-end/delete
                  // shape — because a no-op reset answers with NO frame at all
                  // (the engine short-circuits a fresh lobby), so a strip that
                  // waited for a snapshot to close could never close.
                  setArmedReset(null)
                  mark.reset()
                  send(resetFrame(resetArmed))
                }}
              >
                {t('admin.resetConfirm')}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={flags.resetting}
                onClick={() => setArmedReset(null)}
              >
                {t('admin.resetCancel')}
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!online || flags.resetting}
              onClick={() => setArmedReset({ mode: 'keep_players', phase })}
            >
              {t('admin.resetKeep')}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!online || flags.resetting}
              onClick={() => setArmedReset({ mode: 'clear_players', phase })}
            >
              {t('admin.resetClear')}
            </Button>
          </div>
        )}
      </Card>
    </>
  )
}

/**
 * The Players tab (ADMN-05, D-80). Rows are rendered from explicit safe
 * properties — nickname, budget, total_value, presence, joined_at and the
 * offline last_seen — sorted by `joined_at`; a token is not reachable from the
 * row type at all (T-04-04), so no render, copy or spread path can leak one.
 */
function PlayersPanel({
  view,
  online,
  send,
  flags,
  mark,
}: {
  view: ViewState
  online: boolean
  send(frame: AdminCommand): void
  flags: FlightFlags
  mark: FlightMark
}) {
  const { t, count } = useLanguage()
  const [armedRelease, setArmedRelease] = useState<{ id: string; phase: Phase } | null>(null)

  const players = sortPlayersByJoinedAt(view.players)
  // The armed row is derived: a tab change unmounts this panel, a phase change
  // and a snapshot in which the player is gone both dismiss the strip.
  const releaseArmedId =
    armedRelease !== null &&
    armedRelease.phase === view.phase &&
    players.some((player) => player.id === armedRelease.id)
      ? armedRelease.id
      : null

  return (
    <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
      <div className="flex flex-wrap items-baseline gap-2">
        <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
          {t('admin.playersHeading')}
        </h2>
        <p className="text-base leading-[1.5]">{count(players.length)}</p>
      </div>

      {players.length === 0 ? (
        <p className="text-base leading-[1.5]">{t('admin.playersEmpty')}</p>
      ) : (
        <ul>
          {players.map((player) => (
            <li
              key={player.id}
              className="flex min-h-11 flex-wrap items-center gap-2 border-t-2 border-t-border px-2 py-1 first:border-t-0"
            >
              <span
                aria-hidden="true"
                className={`size-3 shrink-0 border-2 border-border ${
                  player.connected ? 'bg-foreground' : 'bg-transparent'
                }`}
              />
              {/* Presence is never carried by the square alone: the fill is
                  hidden from assistive technology and this text is the state. */}
              <span className="sr-only">
                {player.connected ? t('presence.connected') : t('presence.disconnected')}
              </span>
              {/* The name keeps a 10rem floor, so a crowded line breaks BELOW
                  it: the row's `flex-wrap` drops the trailing values and the
                  action to the next line instead of squeezing the name. */}
              <span className="min-w-40 flex-1 break-words text-base leading-[1.5]">
                {player.nickname}
              </span>
              <span className="text-sm leading-[1.5] font-bold">{t('admin.playerBudget')}</span>
              <span className="text-base leading-[1.5]">{formatVND(player.budget)}</span>
              <span className="text-sm leading-[1.5] font-bold">{t('admin.playerValue')}</span>
              <span className="text-base leading-[1.5]">{formatVND(player.totalValue)}</span>
              <span className="text-sm leading-[1.5] font-bold">{t('admin.playerJoined')}</span>
              <span className="font-mono text-sm leading-[1.5] text-muted-foreground">
                {formatTimeOfDay(player.joinedAt)}
              </span>
              {!player.connected && (
                <>
                  <span className="text-sm leading-[1.5] font-bold">
                    {t('admin.playerLastSeen')}
                  </span>
                  <span className="font-mono text-sm leading-[1.5] text-muted-foreground">
                    {formatTimeOfDay(player.lastSeenAt)}
                  </span>
                </>
              )}

              {releaseArmedId === player.id ? (
                <div className="flex w-full flex-col gap-2 border-2 border-destructive bg-card px-2 py-2">
                  <p className="min-w-0 break-words text-base leading-[1.5]">
                    {fill(t('admin.releaseConfirm'), { name: player.nickname })}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="default"
                      className={CONFIRM_FILL}
                      disabled={!online || flags.releasing}
                      onClick={() => {
                        setArmedRelease(null)
                        mark.release()
                        send(releaseFrame(player.id))
                      }}
                    >
                      {t('admin.releaseSession')}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={flags.releasing}
                      onClick={() => setArmedRelease(null)}
                    >
                      {t('admin.keepSession')}
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  type="button"
                  variant="outline"
                  disabled={!online}
                  onClick={() => setArmedRelease({ id: player.id, phase: view.phase })}
                >
                  {t('admin.releaseSession')}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

function AdminView() {
  const { t } = useLanguage()
  const [status, setStatus] = useState<ConnectionStatus>('connecting')
  const [view, setView] = useState<ViewState>(() => initialViewState())
  // The catalog generation: bumped once per admin snapshot, and only per
  // snapshot — an EVENT keeps the same `items` array, so its reference is the
  // marker that tells the two apart without a second decoder.
  const [catalogSeq, setCatalogSeq] = useState(0)
  const seqRef = useRef(0)
  const lastItemsRef = useRef<ViewState['items'] | null>(null)
  const [errorAt, setErrorAt] = useState<{ code: string; seq: number } | null>(null)
  const [tab, setTab] = useState<AdminTab>(DEFAULT_TAB)

  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [reorderAt, setReorderAt] = useState<number | null>(null)
  const [budgetAt, setBudgetAt] = useState<number | null>(null)
  const [urlAt, setUrlAt] = useState<number | null>(null)
  const [resetAt, setResetAt] = useState<number | null>(null)
  const [releaseAt, setReleaseAt] = useState<number | null>(null)
  const connectionRef = useRef<AdminConnection | null>(null)

  // The socket is created INSIDE this effect and closed in its cleanup, so a
  // StrictMode double-invoke cannot leave two admin connections behind.
  useEffect(() => {
    const connection = openAdminSocket({
      onStatus: setStatus,
      onView: (next) => {
        setView(next)
        if (next.items !== lastItemsRef.current) {
          lastItemsRef.current = next.items
          seqRef.current += 1
          setCatalogSeq(seqRef.current)
        }
      },
      onError: (error) => {
        // A refusal is not a snapshot: the in-flight flags are released so the
        // form is usable again, and the row renders the mapped copy.
        setErrorAt({ code: error.code, seq: seqRef.current })
        setSavedAt(null)
        setReorderAt(null)
        setBudgetAt(null)
        setUrlAt(null)
        setResetAt(null)
        setReleaseAt(null)
      },
    })
    connectionRef.current = connection

    return () => {
      connectionRef.current = null
      connection.close()
    }
  }, [])

  const online = mutationsEnabled(status)
  // Every in-flight flag clears itself when the next snapshot lands.
  const flags: FlightFlags = {
    saving: savedAt !== null && catalogSeq <= savedAt,
    saveLanded: savedAt !== null && catalogSeq > savedAt,
    reordering: reorderAt !== null && catalogSeq <= reorderAt,
    budgetSaving: budgetAt !== null && catalogSeq <= budgetAt,
    urlChoosing: urlAt !== null && catalogSeq <= urlAt,
    resetting: resetAt !== null && catalogSeq <= resetAt,
    releasing: releaseAt !== null && catalogSeq <= releaseAt,
  }
  const mark: FlightMark = {
    saved: () => setSavedAt(catalogSeq),
    clearSaved: () => setSavedAt(null),
    reordered: () => setReorderAt(catalogSeq),
    budget: () => setBudgetAt(catalogSeq),
    url: () => setUrlAt(catalogSeq),
    reset: () => setResetAt(catalogSeq),
    release: () => setReleaseAt(catalogSeq),
  }
  const errorCode = errorAt !== null && catalogSeq <= errorAt.seq ? errorAt.code : null

  function send(frame: AdminCommand) {
    connectionRef.current?.send(frame)
  }

  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-6 px-6 py-12 md:px-8">
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h1 className="text-display leading-[1.2] font-bold tracking-[-0.01em]">{t('admin.h1')}</h1>

          <LanguageControl />
        </div>

        {/* The four-state line is what stops a presenter from clicking a control
            on a dead socket and seeing nothing happen. `superseded` is
            unreachable for this role, but the shared shape keeps the state. */}
        <div role="status" aria-live="polite" className="flex items-center gap-2">
          <span
            aria-hidden="true"
            className={`inline-block size-3 border-2 border-border ${STATUS_FILL[status]}`}
          />
          <p className="text-sm leading-[1.5] font-bold">{t(STATUS_COPY[status])}</p>
        </div>

        {/* Every mutating control on every tab is disabled while the socket is
            down (D-82a); the note names the recovery rather than implying the
            page still works. */}
        {status === 'lost' && (
          <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.offlineNote')}</p>
        )}

        {errorCode !== null && (
          <p role="alert" className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]">
            {t(adminErrorKey(errorCode))}
          </p>
        )}

        {/* The tab row scrolls with the page — the presenter spends the show on
            Control, whose panel is short by construction. It is the shipped
            segmented control (a `role="group"` of pressed buttons), not an ARIA
            tablist: no second keyboard pattern enters the codebase. */}
        <SegmentedChoice<AdminTab>
          label={t('admin.tabsLabel')}
          selected={tab}
          segments={TABS.map((value) => ({ value, label: t(TAB_KEYS[value]) }))}
          onSelect={setTab}
        />
      </div>

      {/* Only the active panel is mounted: switching tabs unmounts the previous
          one, which is what cancels an armed confirm (D-79) structurally. */}
      {tab === 'setup' && (
        <SetupPanel view={view} online={online} send={send} flags={flags} mark={mark} />
      )}
      {tab === 'control' && (
        <ControlPanel view={view} online={online} send={send} flags={flags} mark={mark} />
      )}
      {tab === 'players' && (
        <PlayersPanel view={view} online={online} send={send} flags={flags} mark={mark} />
      )}
    </main>
  )
}

export default AdminView
