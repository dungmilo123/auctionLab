import { Card } from '@/components/ui/card'

// Player route shell (01-UI-SPEC.md §Route Shells). Exactly one h1, the
// skeleton note and the monospace meta line — the meta line is the
// human-verifiable proof of BUILD-03 on a deep link. The temporary echo widget
// is rendered below this panel (plan 01-05 Task 2; deleted in Phase 2).
function PlayerView({ path }: { path: string }) {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-6 px-6 py-12 md:px-8">
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <h1 className="text-display leading-[1.2] font-bold tracking-[-0.01em]">
          Người chơi
        </h1>
        <p className="text-base leading-[1.5]">
          Khung xương M0 — UI thật dựng ở Phase 2–3.
        </p>
        <p className="font-mono text-sm leading-[1.5] font-bold">
          route: player · path: {path}
        </p>
      </Card>
    </main>
  )
}

export default PlayerView
