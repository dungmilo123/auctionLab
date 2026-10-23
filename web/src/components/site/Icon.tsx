import type { LucideIcon } from 'lucide-react'

export function Icon({ icon: Shape, size = 20 }: { icon: LucideIcon; size?: number }) {
  return <Shape className="site-icon" size={size} strokeWidth={1.6} aria-hidden="true" focusable="false" />
}
