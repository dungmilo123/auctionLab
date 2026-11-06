import { useId, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { ChevronDown, SlidersHorizontal } from 'lucide-react'
import { cn } from 'cn'
import { Icon } from './Icon'
import { Button } from '@/components/ui/button'
const query = '(max-width: 767px)'
const subscribe = (change: () => void) => { const media = window.matchMedia(query); media.addEventListener('change', change); return () => media.removeEventListener('change', change) }
const snapshot = () => window.matchMedia(query).matches
export default function Disclosure({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false), id = useId(), trigger = useRef<HTMLButtonElement>(null)
  const narrow = useSyncExternalStore(subscribe, snapshot, () => true)
  return <div className="min-w-0 md:contents" onKeyDown={e => { if (e.key === 'Escape' && open) { setOpen(false); trigger.current?.focus() } }}>
    <Button ref={trigger} type="button" variant="outline" className="md:hidden" aria-expanded={open} aria-controls={id} onClick={() => setOpen(!open)}><Icon icon={SlidersHorizontal} />{label}<Icon icon={ChevronDown} /></Button>
    <div id={id} data-open={open} inert={narrow && !open} aria-hidden={narrow && !open ? true : undefined} className={cn('grid transition-[grid-template-rows] duration-[240ms] ease-[cubic-bezier(.22,1,.36,1)] md:contents', !narrow ? 'grid-rows-[1fr]' : open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]')}><div className={cn('flex min-h-0 flex-wrap gap-4 md:contents', narrow && 'overflow-hidden', narrow && open && 'pt-4')}>{children}</div></div>
  </div>
}
