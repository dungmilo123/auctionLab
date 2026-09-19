import { useState } from 'react'
import { itemImageSrc } from '@/lib/image'

// D-01's ONE image element, shared by every surface that shows an item picture:
// the admin catalog row and editor preview, the player's intro/live/settled
// bodies and the projector's item card. `itemImageSrc` owns the resolution rule
// (a bare file name becomes /images/<name>; an external or root-relative value
// passes through), and this component owns the two rendering rules that must
// hold everywhere:
//
//   - an empty value renders NOTHING. There is no placeholder frame, because a
//     presenter who configured no image must not see an empty box that reads as
//     a broken one (and `itemImageSrc('')` is '' by contract).
//   - a source that FAILS to load removes itself. A file name with a typo, or
//     an external host the venue cannot reach (the room may be fully offline),
//     leaves no broken-image frame behind — the layout closes up and the rest of
//     the body reads normally, which is the difference between "no picture
//     configured" and "the app is broken".
//
// The `alt` text comes from the item name, so a screen reader hears which lot
// the picture belongs to rather than "image". Callers pass their own sizing and
// border classes: the projector's typography, the player's narrow column and the
// admin's compact row each have their own scale, and this component deliberately
// owns none of it.
type ItemImageProps = {
  /** the stored value, exactly as the item carries it (never pre-resolved) */
  src: string
  /** the item's name, used as the alt text */
  alt: string
  className?: string
}

function ItemImage({ src, alt, className }: ItemImageProps) {
  // Keyed by the resolved source, so a different item (or an edited value) gets
  // a fresh element with a cleared failure flag instead of inheriting the
  // previous one's — the derived-state pattern the bid field uses for its own
  // per-item state (D-71).
  const [failedSrc, setFailedSrc] = useState<string | null>(null)
  const resolved = itemImageSrc(src)

  if (resolved === '' || resolved === failedSrc) return null

  return (
    <img
      src={resolved}
      alt={alt}
      className={className}
      // A failed load is not an error the room needs to read: it removes the
      // frame and nothing else changes (no toast, no fallback text).
      onError={() => setFailedSrc(resolved)}
    />
  )
}

export default ItemImage
