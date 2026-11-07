import { useEffect, useState } from 'react'
import { PageHeading } from '@/components/site/Presentation'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useLabsCopy, type LabsKey } from '@/lib/labs-copy'
import AnalyticsLab from './AnalyticsLab'
import CurrencyLab from './CurrencyLab'
import ProxyLab from './ProxyLab'
import SoftCloseLab from './SoftCloseLab'

const LABS = [
  ['currency', 'tabCurrency', CurrencyLab],
  ['soft-close', 'tabSoftClose', SoftCloseLab],
  ['proxy', 'tabProxy', ProxyLab],
  ['analytics', 'tabAnalytics', AnalyticsLab],
] as const satisfies ReadonlyArray<readonly [string, LabsKey, () => React.JSX.Element]>

/** /admin/labs — fixture-only prototypes, in roadmap order. `#proxy` deep-links a tab. */
export default function LabsPage() {
  const { l } = useLabsCopy()
  const fromHash = () => {
    const hash = window.location.hash.slice(1)
    return LABS.some(([k]) => k === hash) ? hash : 'currency'
  }
  const [tab, setTab] = useState(fromHash)
  useEffect(() => {
    const sync = () => setTab(fromHash())
    window.addEventListener('hashchange', sync)
    return () => window.removeEventListener('hashchange', sync)
  }, [])
  return (
    <div className="flex flex-col gap-8">
      <PageHeading title={l('title')} intro={l('intro')} />
      <Tabs value={tab} onValueChange={(v) => { setTab(v); history.replaceState(null, '', `#${v}`) }} className="gap-6">
        <TabsList variant="line" className="h-auto w-full flex-wrap justify-start border-b border-hairline pb-1">
          {LABS.map(([k, label]) => (
            <TabsTrigger key={k} value={k} className="min-h-10 flex-none rounded-[2px] px-3 font-mono text-xs tracking-[0.09em] uppercase">
              {l(label)}
            </TabsTrigger>
          ))}
        </TabsList>
        {LABS.map(([k, , Lab]) => (
          <TabsContent key={k} value={k}><Lab /></TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
