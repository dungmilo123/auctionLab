import { useState } from 'react'

type HealthResult =
  | { kind: 'ok'; body: string }
  | { kind: 'error'; message: string }

// Temporary walking-skeleton page (plan 01-05 replaces it with the route switch
// and the real UI-SPEC widget). It exists to prove the dev chain end to end:
// browser -> Vite :5173 -> proxy -> Go :8080 -> /api/health.
function App() {
  const [result, setResult] = useState<HealthResult | null>(null)

  async function checkConnection() {
    try {
      const response = await fetch('/api/health')
      const body = await response.text()
      if (!response.ok) {
        setResult({
          kind: 'error',
          message: `/api/health trả về HTTP ${response.status}. Kiểm tra make dev đang chạy (Go :8080 + Vite :5173) rồi thử lại.`,
        })
        return
      }
      setResult({ kind: 'ok', body })
    } catch {
      setResult({
        kind: 'error',
        message:
          'Không gọi được /api/health. Kiểm tra make dev đang chạy (Go :8080 + Vite :5173) rồi thử lại.',
      })
    }
  }

  return (
    <main>
      <h1>Người chơi</h1>
      <button type="button" onClick={checkConnection}>
        Kiểm tra kết nối
      </button>
      {result !== null && (
        <pre>{result.kind === 'ok' ? result.body : result.message}</pre>
      )}
    </main>
  )
}

export default App
