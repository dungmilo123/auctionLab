import camera from '@/assets/illustrations/camera.svg'
import clock from '@/assets/illustrations/clock.svg'
import vinyl from '@/assets/illustrations/vinyl.svg'
import type { SiteKey } from '@/lib/site-copy'
import type { LotStatus } from '@/components/site/Presentation'

// Illustrative presentation data only. No persisted lots, game snapshots, or prices.
export const PREVIEW_LOTS = [
  { title: 'camera', metadata: 'cameraMeta', status: 'upcoming', image: camera, href: '/lots/sample-camera' },
  { title: 'clock', metadata: 'clockMeta', status: 'ended', image: clock, href: '/lots/sample-clock' },
  { title: 'vinyl', metadata: 'vinylMeta', status: 'sold', image: vinyl, href: '/lots/sample-vinyl' },
] as const satisfies ReadonlyArray<{ title: SiteKey; metadata: SiteKey; status: LotStatus; image: string; href: string }>
export const HERO_IMAGE = camera
