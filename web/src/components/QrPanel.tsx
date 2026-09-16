import { QRCodeSVG } from 'qrcode.react'
import { useLanguage } from '@/lib/i18n'

// The one QR renderer both surfaces share (D-85, 04-UI-SPEC §Component
// Inventory). It is deliberately dumb: it encodes the URL it is given — the
// admin's selected join URL (D-84) — and nothing else. No query strings, no
// tracking parameters, no styling of the symbol itself: black modules on white
// are what a 2015 phone camera reads from across a room, and scannability beats
// branding.
//
// The rendered `<svg>` keeps its own `viewBox`, so the one CSS knob a route
// needs is a width class: `/screen` passes `w-[40vmin] h-auto` and the symbol
// scales without blur, while `/admin` passes no class and renders at its
// intrinsic `size={176}`. `className` therefore lands on the SVG, not on the
// plate (qrcode.react types `QRCodeSVG` as `QRProps & SVGAttributes<SVGSVGElement>`,
// so the class is forwarded — verified against the installed 4.2.0 typings).
//
// The plate is the white quiet zone: `bg-input` is the theme's white surface in
// both themes, its `p-4` is the route's 16px step, and it lifts `marginSize={2}`
// (8 modules' worth of padding at the largest size) to an effective ≥4-module
// quiet zone around the symbol. The 2px outline is the shared token; on
// `/screen` the route's own `border-width: 4px` rule renders it at projector
// scale (04-UI-SPEC §Spacing).
//
// The title is the SVG's accessible name, not decoration: a projector has no
// text alternative without it, and it is the only string this component
// invents — every other word on both surfaces comes from the dictionary.
export function QrPanel({
  url,
  size,
  className,
}: {
  /** the URL to encode — the selected join URL, never a list */
  url: string
  /** the intrinsic pixel size; a width class may scale it with CSS */
  size: number
  /** forwarded to the `<svg>`, e.g. `w-[40vmin] h-auto` on `/screen` */
  className?: string
}) {
  const { t } = useLanguage()
  return (
    <div className="w-fit border-2 border-border bg-input p-4">
      <QRCodeSVG
        value={url}
        size={size}
        level="M"
        marginSize={2}
        bgColor="#FFFFFF"
        fgColor="#111111"
        title={t('screen.qrTitle')}
        className={className}
      />
    </div>
  )
}
