/* oxlint-disable react/only-export-components -- Shared motion policy and its provider intentionally live together. */
import { useSyncExternalStore, type ReactNode } from 'react'
import { MotionConfig, LazyMotion, domAnimation } from 'motion/react'

export const motionTiming = { micro: 0.15, panel: 0.24, entrance: 0.65 } as const
export const motionEase = [0.22, 1, 0.36, 1] as const
const query = '(prefers-reduced-motion: reduce)'
const subscribe = (change: () => void) => {
  const media = window.matchMedia(query)
  media.addEventListener('change', change)
  return () => media.removeEventListener('change', change)
}
const snapshot = () => window.matchMedia(query).matches
// Subscribe directly: a runtime preference change must affect existing portals too.
export function useWebsiteReducedMotion() { return useSyncExternalStore(subscribe, snapshot, () => true) }
export function WebsiteMotion({ children }: { children: ReactNode }) {
  const reduce = useWebsiteReducedMotion()
  return <LazyMotion features={domAnimation} strict><MotionConfig reducedMotion={reduce ? 'always' : 'never'} transition={{ duration: reduce ? 0 : motionTiming.panel, ease: motionEase }}>{children}</MotionConfig></LazyMotion>
}
