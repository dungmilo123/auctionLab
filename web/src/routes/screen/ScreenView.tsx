import { Card } from '@/components/ui/card'

// Screen route shell (01-UI-SPEC.md §Route Shells). `/screen` and `/screen/**`
// both land here; the meta line is the human-verifiable proof of BUILD-03 when
// the route is deep-linked inside the built binary.
function ScreenView({ path }: { path: string }) {
  return (
    <main className="mx-auto flex w-full max-w-[720px] flex-col gap-6 px-6 py-12 md:px-8">
      <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
        <h1 className="text-display leading-[1.2] font-bold tracking-[-0.01em]">
          Màn chiếu
        </h1>
        <p className="text-base leading-[1.5]">
          Khung xương M0 — UI thật dựng ở Phase 4.
        </p>
        <p className="font-mono text-sm leading-[1.5] font-bold">
          route: screen · path: {path}
        </p>
      </Card>
    </main>
  )
}

export default ScreenView
