import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

// Temporary WebSocket echo widget (D-03 #2, 01-UI-SPEC.md §Temporary echo widget).
//
// It exists for one reason: it is the only path that exercises gorilla's
// same-origin CheckOrigin from a real browser — the Go integration test dials
// with DefaultDialer, which sends no Origin header, so a green `go test` says
// nothing about the browser path (RESEARCH Pitfall 12). Deleted in Phase 2 when
// the real hub replaces internal/hub/echo.go.

type ConnectionState = 'idle' | 'connecting' | 'open' | 'error'

type LogRow = {
  id: number
  text: string
}

const MAX_ROWS = 200
const MAX_PAYLOAD = 280

const STATUS_COPY: Record<ConnectionState, string> = {
  idle: 'Chưa kết nối',
  connecting: 'Đang kết nối…',
  open: 'Đã kết nối tới /ws',
  error: 'Mất kết nối',
}

// Status is always the text plus the indicator — the square never appears alone.
const STATUS_DOT: Record<ConnectionState, string> = {
  idle: 'bg-muted-foreground',
  connecting: 'bg-muted-foreground',
  open: 'bg-primary',
  error: 'bg-destructive',
}

const ERROR_COPY =
  'Không kết nối được /ws. Kiểm tra make dev đang chạy (Go :8080 + Vite :5173) rồi bấm Kết nối.'

// Same-origin by construction: scheme and host come from the page that loaded
// the widget, so dev goes through the Vite proxy and the built binary answers
// directly. Never an absolute or foreign URL (T-05-01).
function echoURL(): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${window.location.host}/ws`
}

function EchoWidget() {
  const [state, setState] = useState<ConnectionState>('idle')
  const [payload, setPayload] = useState('')
  const [rows, setRows] = useState<LogRow[]>([])
  const socketRef = useRef<WebSocket | null>(null)
  const nextRowId = useRef(0)
  const logRef = useRef<HTMLDivElement | null>(null)

  // Newest row at the bottom, scrolled into view. Instant (no smooth) scroll so
  // nothing animates under prefers-reduced-motion.
  useEffect(() => {
    const log = logRef.current
    if (log !== null) log.scrollTop = log.scrollHeight
  }, [rows])

  // Close the socket on unmount — no listener survives the widget.
  useEffect(() => {
    return () => {
      const socket = socketRef.current
      socketRef.current = null
      if (socket !== null) {
        socket.onopen = null
        socket.onmessage = null
        socket.onclose = null
        socket.onerror = null
        socket.close()
      }
    }
  }, [])

  function appendRow(text: string) {
    nextRowId.current += 1
    const id = nextRowId.current
    setRows((prev) => {
      const next = [...prev, { id, text }]
      // Cap at 200 rows, dropping oldest first.
      return next.length > MAX_ROWS ? next.slice(next.length - MAX_ROWS) : next
    })
  }

  function connect() {
    if (socketRef.current !== null) return // guarded against double-connect
    setState('connecting')

    const socket = new WebSocket(echoURL())
    socketRef.current = socket

    socket.onopen = () => {
      if (socketRef.current === socket) setState('open')
    }

    socket.onmessage = (event) => {
      if (socketRef.current !== socket) return
      let envelope: unknown
      try {
        envelope = JSON.parse(typeof event.data === 'string' ? event.data : String(event.data))
      } catch {
        return // not JSON — ignore it rather than render junk
      }
      if (typeof envelope !== 'object' || envelope === null) return

      // Hard rule (D-01): only type/ts/data ever reach the log. A revision or
      // sequence field is never rendered, invented, or labelled — the protocol
      // is not frozen until Phase 2.
      const { type, ts, data } = envelope as { type?: unknown; ts?: unknown; data?: unknown }
      if (type !== 'echo' || typeof ts !== 'number') return
      appendRow(`↓ echo · ts=${ts} · ${JSON.stringify(data)}`)
    }

    socket.onclose = () => {
      if (socketRef.current !== socket) return // a deliberate close already ran
      socketRef.current = null
      setState('error')
    }
  }

  function disconnect() {
    const socket = socketRef.current
    socketRef.current = null
    if (socket !== null) {
      socket.onopen = null
      socket.onmessage = null
      socket.onclose = null
      socket.onerror = null
      socket.close()
    }
    setState('idle')
  }

  function send() {
    const socket = socketRef.current
    const text = payload.trim()
    if (socket === null || socket.readyState !== WebSocket.OPEN || text === '') return

    // The Go handler drops frames that are not valid JSON, so a plain word goes
    // out as a JSON string and anything JSON-shaped is sent verbatim.
    let wire = text
    try {
      JSON.parse(text)
    } catch {
      wire = JSON.stringify(text)
    }

    socket.send(wire)
    appendRow(`↑ ${text}`)
    setPayload('')
  }

  const isOpen = state === 'open'
  const isBusy = state === 'connecting'
  const canSend = isOpen && payload.trim() !== ''

  return (
    <Card className="gap-4 px-6 py-6 md:px-8 md:py-8">
      <h2 className="text-xl leading-[1.2] font-bold tracking-[-0.01em]">
        Kiểm tra WebSocket (tạm)
      </h2>
      <p className="text-base leading-[1.5]">
        Widget này bị xoá ở Phase 2 khi hub thật thay thế.
      </p>

      <p className="flex items-center gap-2 text-sm leading-[1.5] font-bold">
        <span aria-hidden="true" className={`inline-block size-2 ${STATUS_DOT[state]}`} />
        {STATUS_COPY[state]}
      </p>
      {state === 'error' && <p className="text-base leading-[1.5]">{ERROR_COPY}</p>}

      <div className="flex flex-wrap items-end gap-2">
        <div className="flex min-w-0 flex-1 basis-48 flex-col gap-2">
          <Label htmlFor="echo-payload">Nội dung gửi</Label>
          <Input
            id="echo-payload"
            value={payload}
            maxLength={MAX_PAYLOAD}
            placeholder='{"ping": 1}'
            autoComplete="off"
            spellCheck={false}
            onChange={(event) => setPayload(event.target.value)}
            onKeyDown={(event) => {
              // Enter in the input submits — no form, no page reload.
              if (event.key === 'Enter') {
                event.preventDefault()
                send()
              }
            }}
          />
        </div>
        <Button type="button" onClick={send} disabled={!canSend}>
          Gửi
        </Button>
        <Button type="button" variant="outline" onClick={isOpen ? disconnect : connect} disabled={isBusy}>
          {isOpen ? 'Ngắt' : 'Kết nối'}
        </Button>
      </div>

      <div
        ref={logRef}
        role="log"
        aria-live="polite"
        className="max-h-[288px] overflow-y-auto border-2 border-border bg-background p-2"
      >
        {rows.length === 0 ? (
          <div className="p-2">
            <p className="text-base leading-[1.5] font-bold">Chưa có phản hồi</p>
            <p className="text-base leading-[1.5]">Bấm Gửi để kiểm tra echo từ server.</p>
          </div>
        ) : (
          rows.map((row) => (
            // Payloads render as React text nodes — never as raw HTML (T-05-02).
            <p key={row.id} className="font-mono text-sm leading-[1.5] [overflow-wrap:anywhere]">
              {row.text}
            </p>
          ))
        )}
      </div>
    </Card>
  )
}

export default EchoWidget
