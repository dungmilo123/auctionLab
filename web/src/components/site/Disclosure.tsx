import { useId, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { ChevronDown, SlidersHorizontal } from 'lucide-react'
import { Icon } from './Icon'
const query = '(max-width: 767px)'
const subscribe = (change: () => void) => { const media = window.matchMedia(query); media.addEventListener('change', change); return () => media.removeEventListener('change', change) }
const snapshot = () => window.matchMedia(query).matches
export default function Disclosure({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false), id = useId(), trigger = useRef<HTMLButtonElement>(null)
  const narrow = useSyncExternalStore(subscribe, snapshot, () => true)
  return <div className="filter-disclosure" onKeyDown={e => { if (e.key === 'Escape' && open) { setOpen(false); trigger.current?.focus() } }}>
    <button ref={trigger} type="button" className="site-button site-button-outline filter-trigger" aria-expanded={open} aria-controls={id} onClick={() => setOpen(!open)}><Icon icon={SlidersHorizontal} />{label}<Icon icon={ChevronDown} /></button>
    <div id={id} className="secondary-filters" data-open={open} inert={narrow && !open} aria-hidden={narrow && !open ? true : undefined}><div className="secondary-filter-fields">{children}</div></div>
  </div>
}
