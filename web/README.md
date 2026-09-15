# AuctionLab — frontend

React 19 + Vite 8 + TypeScript cho AuctionLab. Đây là **toàn bộ** frontend của app: `vite build`
xuất ra asset tĩnh trong `dist/`, và binary Go nhúng thư mục đó bằng `//go:embed all:web/dist`
(`embed.go` ở repo root). Lúc demo không có tiến trình Node nào chạy.

## Trong thư mục này

- `src/main.tsx`, `src/App.tsx` — điểm vào của SPA; `src/lib/route.ts` là hàm thuần
  `routeFor(pathname)` chọn một trong ba route tĩnh `/` (player), `/admin`, `/screen` — chưa dùng
  router dependency.
- `src/routes/{player,admin,screen}/` — component cho từng route.
- `src/components/ui/` — bốn primitive shadcn (`button`, `input`, `label`, `card`), đã restyle theo
  style neobrutalist của design contract.
- `src/components/EchoWidget.tsx` — widget echo tạm trên `/` để kiểm tra WebSocket; xoá ở Phase 2
  khi hub thật thay thế.
- `src/index.css` — token màu, typography và spacing của design contract.
- `public/` — asset tĩnh, được copy nguyên trạng vào output build.
- `dist/` — output của `vite build`; không commit (chỉ `dist/.gitkeep` được track, để `go:embed`
  không vỡ trên clean clone khi chưa build).

## Lệnh

Chạy từ **repo root**, không phải trong `web/`:

| Lệnh | Việc |
|---|---|
| `make dev` | Chạy song song Go `:8080` và Vite `:5173`; Vite proxy `/api` và `/ws` sang `:8080` |
| `make build` | `vite build` **trước**, rồi `CGO_ENABLED=0 go build -o bin/auctionlab ./cmd/server` |
| `make test` | `go test ./...` + `vitest run` + `oxlint` |
| `make dev-web` | Chỉ chạy Vite, khi backend đã chạy sẵn |

Khi cần chạy riêng trong `web/`: `npm run dev`, `npm run build`, `npm run test` (một lượt),
`npm run lint`.

## Tài liệu gốc

- `PLAN.md` — build plan đầy đủ: kiến trúc, WS protocol, milestones, testing.
- `AGENTS.md` — hướng dẫn project cho agent runtime (stack, conventions, workflow).
