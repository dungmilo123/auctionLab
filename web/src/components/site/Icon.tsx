import type { LucideIcon } from 'lucide-react'

export function Icon({ icon: Shape, size = 20, className }: { icon: LucideIcon; size?: number; className?: string }) {
  return <Shape className={className ?? 'inline-block shrink-0 align-middle'} size={size} strokeWidth={1.6} aria-hidden="true" focusable="false" />
}
