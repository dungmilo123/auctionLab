import type { ReactNode } from 'react'
import ItemImage from '@/components/ItemImage'
import { useLanguage, type CopyKey } from '@/lib/i18n'
import { formatUSD } from '@/lib/format'
import type { PublicItem } from '@/ws/protocol'

// Screen C of the player route (03-UI-SPEC §Screen C — ITEM_INTRO).
//
// Body order: item name → info rows → budget block → waiting line. Everything
// rendered comes from the self-sufficient snapshot (D-25) plus the player's own
// row, so a reload mid-intro paints the whole screen with no event history.
// There is no action here and therefore no accent on this screen.
//
// D-01 adds the item's picture between the name and the info rows. It is the
// same element the projector and the admin render (one resolution rule, one
// failure rule), and it renders nothing at all when the item carries no image.

// The four frozen auction types, mapped to their approved type names. A value
// outside the set cannot be created (the admin editor offers exactly four and
// D-66 validates them), so the fallback only exists to keep the body total —
// and it renders the server's own value rather than inventing a name.
const AUCTION_TYPE_KEYS: Readonly<Record<string, CopyKey>> = {
  english: 'auction.english',
  dutch: 'auction.dutch',
  sealed_first: 'auction.sealedFirst',
  vickrey: 'auction.vickrey',
}

/** One label/value row: label 14/700, value 16/400, both baseline-aligned. */
function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-2">
      <p className="text-sm leading-[1.5] font-bold">{label}</p>
      <p className="flex flex-wrap items-baseline gap-2 text-base leading-[1.5]">{children}</p>
    </div>
  )
}

/** The hint range, or null when the item carries no hint at all. */
function hintRange(item: PublicItem): string | null {
  const { hintMin, hintMax } = item
  if (hintMin !== null && hintMax !== null) return `${formatUSD(hintMin)} – ${formatUSD(hintMax)}`
  if (hintMin !== null) return formatUSD(hintMin)
  if (hintMax !== null) return formatUSD(hintMax)
  return null
}

type ItemIntroProps = {
  item: PublicItem
  /** the player's own budget, from the snapshot's `me` row (D-25) */
  budget: number
}

function ItemIntro({ item, budget }: ItemIntroProps) {
  const { t } = useLanguage()
  const typeKey = AUCTION_TYPE_KEYS[item.auctionType]
  const hint = hintRange(item)

  return (
    <>
      {/* Names run to 80 runes: the heading wraps, it never truncates. */}
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em] break-words">
        {item.name}
      </h2>

      {/* D-01: the lot's picture, above the facts it belongs to. A fixed height
          with `object-contain` letterboxes a photo of any shape rather than
          resizing the body or cropping it, and an unconfigured item renders no
          element at all (so there is no empty frame to read as a broken one). */}
      <ItemImage
        src={item.imageUrl}
        alt={item.name}
        className="h-40 w-full border-2 border-border bg-card object-contain"
      />

      <div className="flex flex-col gap-2">
        <InfoRow label={t('intro.typeLabel')}>
          <span className="text-sm leading-[1.5] text-muted-foreground">
            {typeKey === undefined ? item.auctionType : t(typeKey)}
          </span>
        </InfoRow>

        <InfoRow label={t('intro.hintLabel')}>
          {hint === null ? (
            // The both-null case is a fact about the item, not an error, so it
            // gets a sentence rather than an empty value slot.
            <span>{t('intro.hintNone')}</span>
          ) : (
            <>
              <span>{hint}</span>
            </>
          )}
        </InfoRow>

        <InfoRow label={t('intro.startLabel')}>
          <span>{formatUSD(item.startPrice)}</span>
        </InfoRow>
      </div>

      {/* The shipped Phase 2 budget block, reused verbatim: this is the number
          the player is about to spend, so it keeps its own bordered well. */}
      <div className="flex flex-wrap items-baseline gap-2 border-2 border-border bg-background px-4 py-2">
        <p className="text-sm leading-[1.5] font-bold">{t('lobby.budgetLabel')}</p>
        <p className="text-xl leading-[1.2] font-bold">{formatUSD(budget)}</p>
      </div>

      {/* The body's "what happens next" sentence — the host starts the run, not
          the player, so there is nothing to act on here. */}
      <p className="text-base leading-[1.5]">{t('intro.waiting')}</p>
    </>
  )
}

export default ItemIntro
