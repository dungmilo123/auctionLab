import { Clock, MinusCircle, TrendingDown, TrendingUp, Trophy } from 'lucide-react'
import type { ParticipationStatus } from '@/api/auction'

/**
 * Bidder standing on a lot: leading (green), outbid (red), won (gold), lost (grey).
 * Colour is always paired with an icon and words, so it is never the only signal.
 * Tokens live in index.css (--standing-*), each pair at ≥ 5:1 contrast.
 */
export type StandingStyle = { box: string; edge: string; text: string; Icon: typeof Clock }
export const standingStyle: Partial<Record<ParticipationStatus, StandingStyle>> = {
  leading: { box: 'border-[var(--standing-leading)] bg-[var(--standing-leading-bg)]', edge: 'border-l-[var(--standing-leading)]', text: 'text-[var(--standing-leading)]', Icon: TrendingUp },
  outbid: { box: 'border-[var(--standing-outbid)] bg-[var(--standing-outbid-bg)]', edge: 'border-l-[var(--standing-outbid)]', text: 'text-[var(--standing-outbid)]', Icon: TrendingDown },
  won: { box: 'border-[var(--standing-won-edge)] bg-[var(--standing-won-bg)]', edge: 'border-l-[var(--standing-won-edge)]', text: 'text-[var(--standing-won)]', Icon: Trophy },
  lost: { box: 'border-[var(--standing-lost)]/40 bg-[var(--standing-lost-bg)]', edge: 'border-l-[var(--standing-lost)]/40', text: 'text-[var(--standing-lost)]', Icon: MinusCircle },
}
/** The countdown matters only while the bidder can still act. */
export const standingIsLive = (status: ParticipationStatus) => status === 'leading' || status === 'outbid'
