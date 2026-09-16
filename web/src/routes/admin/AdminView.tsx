import { useEffect, useRef, useState } from 'react'
import LanguageControl, { SegmentedChoice } from '@/components/LanguageControl'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatVND } from '@/lib/format'
import { fill, useLanguage, type CopyKey } from '@/lib/i18n'
import { openAdminSocket, type AdminConnection } from '@/ws/admin'
import { parseBidAmount } from '@/lib/bid'
import type { ConnectionStatus } from '@/ws/client'
import type { AdminItem, Phase } from '@/ws/protocol'
import { initialViewState, type ViewState } from '@/ws/view'

// The minimal, deliberately UNPROTECTED admin page (D-64, ADMN-01/02).
//
// Three `Card` panels in a fixed order — Control, Items, then a conditional
// Editor — and nothing else: no tabs, no ADMIN_KEY, no reset, no players list,
// no QR/LAN panel, no screen work (ADMN-04..07 are Phase 4, and this page says
// so in its own caption rather than pretending otherwise).
//
// Two rules shape the whole file:
//
//   1. The SERVER is the single source of truth (contract #1). Every panel
//      renders `ViewState`, which only a snapshot replaces. Nothing here mutates
//      the catalog optimistically: a reorder is a frame, and the row that moves
//      is the row the next snapshot returns.
//   2. Every "in flight" flag is DERIVED from the catalog generation rather than
//      cleared by an effect: a snapshot rebuilds `items` (the events keep the
//      same array), so "the list re-rendered" is a render-time fact. That is
//      what makes the snapshot the confirmation, and what keeps a rejected frame
//      from leaving a form permanently disabled.

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

const MAX_ITEM_NAME_RUNES = 80

const FIELD_IDS = {
  name: 'admin-field-name',
  truthValue: 'admin-field-truth-value',
  hintMin: 'admin-field-hint-min',
  hintMax: 'admin-field-hint-max',
  startPrice: 'admin-field-start-price',
  minIncrement: 'admin-field-min-increment',
  durationMs: 'admin-field-duration-ms',
  antiSnipeWindowMs: 'admin-field-anti-snipe-window-ms',
  maxExtensions: 'admin-field-max-extensions',
  floorPrice: 'admin-field-floor-price',
  decrementStep: 'admin-field-decrement-step',
  decrementIntervalMs: 'admin-field-decrement-interval-ms',
} as const

type NumberField = Exclude<keyof typeof FIELD_IDS, 'name'>

/** The whole editor draft: strings as typed, plus the fields the form never shows. */
type DraftFields = {
  name: string
  auctionType: string
  antiSnipeMode: string
  truthValue: string
  hintMin: string
  hintMax: string
  startPrice: string
  minIncrement: string
  durationMs: string
  antiSnipeWindowMs: string
  maxExtensions: string
  floorPrice: string
  decrementStep: string
  decrementIntervalMs: string
}

/**
 * The editor's own object. `description` and `imageUrl` are NOT fields in Phase
 * 3 (CNT-01 is v2): they are carried so the form echoes the loaded item's values
 * back unchanged, and they are empty strings on create.
 */
type EditorDraft = {
  /** '' means "create with a server-minted id" (H14) */
  id: string
  description: string
  imageUrl: string
  fields: DraftFields
}

type DraftErrors = Partial<Record<keyof DraftFields, CopyKey>>

/** Which fields the matrix shows for this type — and therefore which are validated. */
function visibleFields(type: string, antiSnipeMode: string): Record<keyof DraftFields, boolean> {
  const english = type === 'english'
  const dutch = type === 'dutch'
  const timed = english || type === 'sealed_first' || type === 'vickrey'
  return {
    name: true,
    auctionType: true,
    antiSnipeMode: english,
    truthValue: true,
    hintMin: true,
    hintMax: true,
    startPrice: true,
    minIncrement: english,
    durationMs: timed,
    antiSnipeWindowMs: english && antiSnipeMode === 'extend',
    maxExtensions: english,
    floorPrice: dutch,
    decrementStep: dutch,
    decrementIntervalMs: dutch,
  }
}

/** A required field is one the server's own validation would refuse as absent. */
function isRequired(key: NumberField, fields: DraftFields): boolean {
  switch (key) {
    case 'truthValue':
    case 'startPrice':
      return true
    case 'minIncrement':
    case 'durationMs':
      return fields.auctionType === 'english'
    case 'antiSnipeWindowMs':
      return fields.auctionType === 'english' && fields.antiSnipeMode === 'extend'
    default:
      return false
  }
}

/**
 * The UI-SPEC's validation table, applied to the VISIBLE fields only. The server
 * stays the authority (D-66): the client mirrors the same rules so the presenter
 * has a field to fix, because the server's refusal carries only the frozen
 * `bad_message` code and its `message` is log-only.
 */
function validateDraft(draft: EditorDraft): DraftErrors {
  const fields = draft.fields
  const visible = visibleFields(fields.auctionType, fields.antiSnipeMode)
  const errors: DraftErrors = {}

  const name = fields.name.trim()
  if (name.length === 0 || [...name].length > MAX_ITEM_NAME_RUNES) errors.name = 'admin.err.name'

  const check = (key: NumberField, floor: 0 | 1) => {
    if (!visible[key]) return
    const raw = fields[key]
    if (raw === '') {
      if (isRequired(key, fields)) {
        errors[key] = floor === 1 ? 'admin.err.positive' : 'admin.err.notNegative'
      }
      return
    }
    // One parse rule for both routes: a raw digit string, exactly what the bid
    // field accepts (D-71). Grouped amounts are a display form only.
    const parsed = parseBidAmount(raw)
    if (parsed === null || parsed < floor) {
      errors[key] = floor === 1 ? 'admin.err.positive' : 'admin.err.notNegative'
    }
  }

  for (const key of ['truthValue', 'hintMin', 'hintMax'] as const) check(key, 0)
  for (const key of [
    'startPrice',
    'minIncrement',
    'durationMs',
    'antiSnipeWindowMs',
    'maxExtensions',
    'floorPrice',
    'decrementStep',
    'decrementIntervalMs',
  ] as const) {
    check(key, 1)
  }

  const min = fields.hintMin === '' ? null : parseBidAmount(fields.hintMin)
  const max = fields.hintMax === '' ? null : parseBidAmount(fields.hintMax)
  if (min !== null && max !== null && min > max) errors.hintMin = 'admin.err.hintOrder'

  return errors
}

/**
 * The `admin.item.upsert` payload (D-66): the FULL object, so the editor
 * round-trips exactly the row it was shown. Every numeric field is sent as the
 * integer its raw digit string parses to, or null when it is empty — including
 * the fields the current type hides, which keeps their loaded values instead of
 * discarding them when the presenter visits another type on the way to Save.
 */
function draftPayload(draft: EditorDraft): Record<string, unknown> {
  const fields = draft.fields
  const num = (key: NumberField): number | null => {
    const raw = fields[key]
    return raw === '' ? null : parseBidAmount(raw)
  }

  return {
    id: draft.id,
    name: fields.name.trim(),
    description: draft.description,
    image_url: draft.imageUrl,
    auction_type: fields.auctionType,
    truth_value: num('truthValue') ?? 0,
    hint_min: num('hintMin'),
    hint_max: num('hintMax'),
    start_price: num('startPrice') ?? 0,
    min_increment: num('minIncrement'),
    floor_price: num('floorPrice'),
    decrement_step: num('decrementStep'),
    decrement_interval_ms: num('decrementIntervalMs'),
    duration_ms: num('durationMs'),
    anti_snipe_mode: fields.antiSnipeMode,
    anti_snipe_window_ms: num('antiSnipeWindowMs'),
    max_extensions: num('maxExtensions'),
  }
}

function blankDraft(): EditorDraft {
  return {
    id: '',
    description: '',
    imageUrl: '',
    fields: {
      name: '',
      auctionType: 'english',
      antiSnipeMode: 'disabled',
      truthValue: '',
      hintMin: '',
      hintMax: '',
      startPrice: '',
      minIncrement: '',
      durationMs: '',
      antiSnipeWindowMs: '',
      maxExtensions: '',
      floorPrice: '',
      decrementStep: '',
      decrementIntervalMs: '',
    },
  }
}

/** Load one catalog row into the form, keeping every value it was shown. */
function draftFromItem(item: AdminItem): EditorDraft {
  const text = (value: number | null): string => (value === null ? '' : String(value))
  return {
    id: item.id,
    description: item.description,
    imageUrl: item.imageUrl,
    fields: {
      name: item.name,
      auctionType: item.auctionType,
      antiSnipeMode: item.antiSnipeMode === '' ? 'disabled' : item.antiSnipeMode,
      truthValue: String(item.truthValue),
      hintMin: text(item.hintMin),
      hintMax: text(item.hintMax),
      startPrice: String(item.startPrice),
      minIncrement: text(item.minIncrement),
      durationMs: text(item.durationMs),
      antiSnipeWindowMs: text(item.antiSnipeWindowMs),
      maxExtensions: text(item.maxExtensions),
      floorPrice: text(item.floorPrice),
      decrementStep: text(item.decrementStep),
      decrementIntervalMs: text(item.decrementIntervalMs),
    },
  }
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

  const [armedForceEnd, setArmedForceEnd] = useState<Phase | null>(null)
  const [armedDelete, setArmedDelete] = useState<string | null>(null)
  const [draft, setDraft] = useState<EditorDraft | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [reorderAt, setReorderAt] = useState<number | null>(null)
  const editorRef = useRef<HTMLDivElement | null>(null)
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
      },
    })
    connectionRef.current = connection

    return () => {
      connectionRef.current = null
      connection.close()
    }
  }, [])

  const items = [...view.items].sort((a, b) => a.orderIndex - b.orderIndex)
  const currentItem = view.item
  const phase = view.phase
  const firstPending = items.find((item) => item.status === 'pending') ?? null

  // D-65's start target: the CURRENT item while it is on stage, otherwise the
  // first pending one in order. No target means nothing to start — which is a
  // caption, not a silent dead button.
  const startId = phase === 'ITEM_INTRO' && currentItem !== null ? currentItem.id : firstPending?.id ?? null
  const startEnabled =
    (phase === 'LOBBY' || phase === 'ITEM_INTRO' || phase === 'FINALE') && startId !== null
  const forceEndEnabled = phase === 'LIVE'
  const nextEnabled = phase === 'ITEM_INTRO' || phase === 'SETTLED'

  // Every in-flight flag clears itself when the next snapshot lands.
  const saving = savedAt !== null && catalogSeq <= savedAt
  const reordering = reorderAt !== null && catalogSeq <= reorderAt
  // The armed strips are phase/item-derived too: a force-end that moves the
  // phase, or a delete that removes the row, dismisses its own strip.
  const forceEndArmed = armedForceEnd !== null && armedForceEnd === phase
  const deleteArmedId =
    armedDelete !== null && items.some((item) => item.id === armedDelete) ? armedDelete : null
  // The editor closes when the save's own snapshot arrives: the row appearing
  // (or changing) in the list IS the confirmation, so no toast exists or is
  // added. A fresh draft clears `savedAt`, so opening the editor again works.
  const editor =
    draft !== null && !(savedAt !== null && catalogSeq > savedAt) ? draft : null
  const editorKey = editor === null ? null : editor.id === '' ? 'new' : editor.id
  const errors = editor === null ? {} : validateDraft(editor)
  // The matrix, evaluated once per render: which fields this draft's type shows
  // (and therefore which ones the Save gate validates).
  const visible = visibleFields(
    editor?.fields.auctionType ?? 'english',
    editor?.fields.antiSnipeMode ?? 'disabled',
  )
  const errorCode = errorAt !== null && catalogSeq <= errorAt.seq ? errorAt.code : null

  // `scrollIntoView` is the one external effect this page has: opening the
  // editor brings the card into view INSTANTLY (`block: 'nearest'`, never
  // `behavior: 'smooth'` — no motion beside the inherited reduced-motion rule).
  useEffect(() => {
    if (editorKey === null) return
    editorRef.current?.scrollIntoView({ block: 'nearest' })
  }, [editorKey])

  function send(frame: unknown) {
    connectionRef.current?.send(frame)
  }

  function openDraft(next: EditorDraft) {
    setSavedAt(null)
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
    setSavedAt(catalogSeq)
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
    setReorderAt(catalogSeq)
    // The COMPLETE permutation, in list order (D-66): the server validates it
    // as a full permutation and rejects without a partial write.
    send({ type: 'admin.item.reorder', ids: next })
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

        {/* D-64 made this page reachable by anyone on the network, so it says so
            rather than implying a protection it does not have (ADMN-04). */}
        <p className="text-sm leading-[1.5] text-muted-foreground">{t('admin.noKeyNote')}</p>

        {errorCode !== null && (
          <p role="alert" className="border-2 border-destructive px-2 py-2 text-base leading-[1.5]">
            {t(adminErrorKey(errorCode))}
          </p>
        )}
      </div>

      {/* Panel 1 — Control */}
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

      {/* Panel 2 — Items */}
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
            {t('admin.itemsHeading')}
          </h2>
          <Button type="button" variant="outline" onClick={() => openDraft(blankDraft())}>
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
                        disabled={index === 0 || reordering}
                        onClick={() => move(item.id, -1)}
                      >
                        {t('admin.moveUp')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        aria-label={fill(t('admin.moveDownLabel'), { name: item.name })}
                        disabled={index === items.length - 1 || reordering}
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
                        disabled={locked}
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

      {/* Panel 3 — Editor, rendered only while a draft is open */}
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
            disabled={saving}
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
              disabled={saving}
              onChange={(value) => editField('truthValue', value)}
            />
            <NumberFieldRow
              id={FIELD_IDS.startPrice}
              labelKey="admin.field.startPrice"
              value={editor.fields.startPrice}
              errorKey={errors.startPrice}
              disabled={saving}
              onChange={(value) => editField('startPrice', value)}
            />
            <NumberFieldRow
              id={FIELD_IDS.hintMin}
              labelKey="admin.field.hintMin"
              value={editor.fields.hintMin}
              errorKey={errors.hintMin}
              disabled={saving}
              onChange={(value) => editField('hintMin', value)}
            />
            <NumberFieldRow
              id={FIELD_IDS.hintMax}
              labelKey="admin.field.hintMax"
              value={editor.fields.hintMax}
              errorKey={errors.hintMax}
              disabled={saving}
              onChange={(value) => editField('hintMax', value)}
            />

            {visible.minIncrement && (
              <NumberFieldRow
                id={FIELD_IDS.minIncrement}
                labelKey="admin.field.minIncrement"
                value={editor.fields.minIncrement}
                errorKey={errors.minIncrement}
                disabled={saving}
                onChange={(value) => editField('minIncrement', value)}
              />
            )}
            {visible.durationMs && (
              <NumberFieldRow
                id={FIELD_IDS.durationMs}
                labelKey="admin.field.durationMs"
                value={editor.fields.durationMs}
                errorKey={errors.durationMs}
                disabled={saving}
                onChange={(value) => editField('durationMs', value)}
              />
            )}
            {visible.antiSnipeWindowMs && (
              <NumberFieldRow
                id={FIELD_IDS.antiSnipeWindowMs}
                labelKey="admin.field.antiSnipeWindowMs"
                value={editor.fields.antiSnipeWindowMs}
                errorKey={errors.antiSnipeWindowMs}
                disabled={saving}
                onChange={(value) => editField('antiSnipeWindowMs', value)}
              />
            )}
            {visible.maxExtensions && (
              <NumberFieldRow
                id={FIELD_IDS.maxExtensions}
                labelKey="admin.field.maxExtensions"
                value={editor.fields.maxExtensions}
                errorKey={errors.maxExtensions}
                disabled={saving}
                onChange={(value) => editField('maxExtensions', value)}
              />
            )}
            {visible.floorPrice && (
              <NumberFieldRow
                id={FIELD_IDS.floorPrice}
                labelKey="admin.field.floorPrice"
                value={editor.fields.floorPrice}
                errorKey={errors.floorPrice}
                disabled={saving}
                onChange={(value) => editField('floorPrice', value)}
              />
            )}
            {visible.decrementStep && (
              <NumberFieldRow
                id={FIELD_IDS.decrementStep}
                labelKey="admin.field.decrementStep"
                value={editor.fields.decrementStep}
                errorKey={errors.decrementStep}
                disabled={saving}
                onChange={(value) => editField('decrementStep', value)}
              />
            )}
            {visible
              .decrementIntervalMs && (
              <NumberFieldRow
                id={FIELD_IDS.decrementIntervalMs}
                labelKey="admin.field.decrementIntervalMs"
                value={editor.fields.decrementIntervalMs}
                errorKey={errors.decrementIntervalMs}
                disabled={saving}
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
              disabled={saving}
              onClick={() => {
                setDraft(null)
                setSavedAt(null)
              }}
            >
              {t('admin.discardEdits')}
            </Button>
            <Button type="button" variant="outline" disabled={saving} onClick={save}>
              {saving ? t('admin.saving') : t('admin.save')}
            </Button>
          </div>
        </Card>
      )}
    </main>
  )
}

export default AdminView
