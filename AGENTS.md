<!-- GSD:project-start source:PROJECT.md -->

## Project

**AuctionLab**

AuctionLab là web app demo đấu giá realtime cho presentation môn học. Người chơi quét QR từ điện thoại, nhập nickname để tham gia; admin (người dẫn chương trình) cấu hình item và điều khiển toàn bộ vòng chơi; màn hình lớn (`/screen`) chiếu QR, diễn biến đấu giá, reveal và podium cho khán giả. Không có tài khoản/đăng nhập cho player — toàn bộ app chạy trong 1 binary Go phục vụ React SPA + WebSocket realtime.

**Core Value:** Một buổi đấu giá trực tiếp trước lớp chạy trọn vẹn không gián đoạn: state game nhất quán tuyệt đối (budget, revision, thứ tự event), timing chính xác theo wall-clock kể cả khi crash, và không rò rỉ thông tin bí mật (truth_value, sealed bids).

### Constraints

- **Tech stack**: Go 1.26 + gorilla/websocket + modernc.org/sqlite + React/Vite/TS — đã chốt trong PLAN.md; không CGO; app là 1 binary.
- **Timeline**: ~5.5 ngày làm việc (M0–M6); có thể nén 2–3 ngày nếu tập trung.
- **Performance**: 20 WebSocket connections qua LAN/tunnel; không tối ưu cho scale lớn.
- **Dependencies**: cloudflared optional (install script, không cần root); venue có thể offline hoàn toàn (hotspot).
- **Security**: Không auth player (chấp nhận có chủ đích); ADMIN_KEY sinh bằng `crypto/rand`; role projections là invariant chống leak.
- **Correctness**: non-blocking enqueue broadcast trong lock; revision monotonic; resume theo wall-clock sau crash.

<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->

## Technology Stack

## Executive Verdict

## Locked-Stack Validation

| Locked choice | Verdict | Basis |
|---|---|---|
| **Go 1.26** (`net/http`) | ✅ **KEEP** — but pin a patched 1.26.x | `go 1.26` is inside Go's two-release support window (1.27.1 is current stable, 1.26.8 is the current 1.26 patch). No stdlib WebSocket exists in 1.26 — an external WS library is mandatory. Go 1.22+ `ServeMux` method+wildcard patterns and `http.FileServerFS` are present, so `net/http` alone is sufficient as the router. Installed locally: go1.26.4 (four security patches behind). |
| **`github.com/gorilla/websocket`** | ✅ **KEEP** — pin `v1.5.3` exactly | Latest tag is `v1.5.3` (2024-06-14), confirmed via `proxy.golang.org/.../@latest`. Repo is **not** archived (Gorilla was revived July 2023, all packages re-released 2023-11-05). The famous "New maintainers needed" issue #370 is from **2018-04-01 and is closed**. `v1.5.3` is also the *minimum security-safe* version: GO-2026-6278 / GHSA-w67g-5rqw-f597 (CWE-338, weak PRNG for mask keys, CVSS 6.9) is fixed in 1.5.3. The hub pattern PLAN.md contract #5 mandates is the canonical gorilla pattern. |
| **SQLite via `modernc.org/sqlite`** | ✅ **KEEP** — requires an explicit DSN | Latest is **v1.58.0** (2026-09-01) — actively released. No entries in the Go vulnerability database. Pure-Go keeps `CGO_ENABLED=0` and `GOOS/GOARCH` cross-compile trivial, which is the whole point of the single-binary constraint. WAL recovery semantics are unaffected by the driver. **The DSN must be explicit — see §3.** |
| **React + Vite + TypeScript** | ✅ **KEEP** — pin TypeScript to `~6.0.2` | `create-vite@9.2.1` `react-ts` template (executed locally) emits React 19.2.8, Vite 8.3.0, `@vitejs/plugin-react` 6.1.1, and **TypeScript `~6.0.2`** — Vite deliberately pins TS 6, not the newer TS 7. Keep the scaffold's pin. |
| **npm `qrcode` for client-side QR** | ⚠️ **WORKS — but `qrcode.react` is the better fit** | Both are npm packages, so this does not change the dependency envelope. `qrcode` 1.5.4 was last published 2024-08-05 and has a Low source-reputation rating on Context7; `qrcode.react` 4.2.0 is High-reputation with 281 indexed snippets, peer-supports React 19, and renders crisp SVG. See §5. |
| **cloudflared quick tunnel (primary)** | ⚠️ **RISK FLAGGED — keep, but demote to "best-effort primary"** | Quick tunnels have no SLA, a 200-in-flight-request cap (429 beyond), no SSE, a single HA connection, a new random URL every start, are unsupported when `.cloudflared/config.yaml` exists, and cloudflared issue #1652 documents the `Upgrade: websocket` header being **dropped** on the default `--protocol=auto` (QUIC) path. See §6. |
| **Single binary via `embed.FS`** | ✅ **KEEP** — needs a committed placeholder | Verified locally on Go 1.26.4: `//go:embed all:web/dist` fails with *"no matching files found"* when the directory is absent and *"cannot embed directory web/dist: contains no embeddable files"* when it is empty. `all:` + a committed `web/dist/.gitkeep` is the fix. See §7. |

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why recommended |
|---|---|---|---|
| **Go** | `go 1.26` in `go.mod`, toolchain ≥ `go1.26.8` | Single-binary server, game engine, WS hub | Already installed; inside the supported window; `CGO_ENABLED=0` static binary; `net/http` + `embed.FS` cover routing + static serving with zero framework. A framework (Chi/Echo/Fiber/Gin) buys nothing here — 10 routes and one `/ws`. |
| **`net/http`** | stdlib (Go 1.26) | HTTP routing, static serving, WS upgrade handler | Go 1.22+ `ServeMux` method+wildcard patterns (`"GET /api/health"`) verified working on Go 1.26.4. `http.FileServerFS(fs.FS)` verified present. No router dependency needed. |
| **`github.com/gorilla/websocket`** | **`v1.5.3`** (pin exactly, no `^`/floating) | WebSocket upgrade, framing, ping/pong | API is frozen and stable, passes the Autobahn server suite, has by far the largest body of community examples, and its per-client-writer pattern maps 1:1 onto PLAN.md contract #5. Pin exact: `v1.5.3` is the security floor *and* the ceiling. |
| **`modernc.org/sqlite`** | **`v1.58.0`** | Persistence: players, items, sealed bids, `auction_runtime` | Pure Go, no CGO, trivial cross-compile, actively released (2026-09-01). Per-statement overhead is a non-issue at ~20 clients — see §3 benchmarks. |
| **React** | `^19.2.8` (19.3.0 current) | All three views (`/`, `/admin`, `/screen`) | Scaffold default. Concurrent rendering means a 250 ms `price.tick` storm does not block input on the admin control panel. |
| **Vite** | `^8.3.0` | Dev server (with `/ws` proxy), production build | Node 24.17.0 satisfies Vite 8's `^20.19.0 \|\| >=22.12.0`. Vite 8 uses Rolldown + Oxc — a bundler change, but this is a plain SPA with two plugins, so the compat layer handles it. **Do not use `rolldown-vite` migration path** — irrelevant for a greenfield project. |
| **TypeScript** | **`~6.0.2`** (as scaffolded) | Type safety across 3 views + WS protocol types | Vite's own template pins TS 6. See §8 for why TS 7 is *not* recommended here even though it is GA. |

### Supporting Libraries

| Library | Version | Purpose | When to use |
|---|---|---|---|
| **`qrcode.react`** *(recommended)* | `^4.2.0` | QR render on `/screen` lobby + admin QR panel | Any time — `<QRCodeSVG value={joinUrl} size={480} level="M" marginSize={2} />`. Declarative, no imperative canvas refs, infinitely crisp on a projector. Peer-supports React 19. |
| **`qrcode`** *(locked choice — acceptable)* | `^1.5.4` | Same, imperative | If you prefer no React-specific dep: `QRCode.toCanvas(url, { errorCorrectionLevel: 'H' })` / `toDataURL`. Works fine; slightly more code, raster output, and last published 2024-08-05. |
| **`github.com/google/uuid`** | `v1.6.0` | Player `id`, item `id` | Only if you want canonical UUID text. **Alternative worth considering: skip it** — `crypto/rand` + `hex.EncodeToString` gives you the same unpredictability with zero deps, and PLAN.md contract #7 already requires `crypto/rand` for tokens. |
| **`github.com/coder/websocket`** | `v1.8.15` | *Not a dependency — the documented migration target* | Only if gorilla is ever replaced. See §4. |

### Development Tools

| Tool | Purpose | Notes |
|---|---|---|
| **Vitest** | `^5.0.0` | Frontend tests for `format.ts` (VND) + timer interpolation. Requires Node `^22.12.0 \|\| ^24.0.0 \|\| >=26.0.0` — Node 24.17.0 satisfies it. Vite-native: one `vitest.config.ts` reuses the plugin set. Keep pure-logic tests in the `node` environment; add `jsdom`/`happy-dom` only if you test components. |
| **`oxlint`** | `^1.81.0` | Comes with the Vite scaffold's `react-ts` template as the `lint` script. Do not add ESLint on top. |
| **`go test` + `httptest`** | stdlib | Integration tests: `httptest.NewServer(handler)` + `websocket.DefaultDialer.Dial(wsURL, nil)` over the real transport. This is gorilla's own test pattern and is the only way to assert the revision-monotonicity contract. |
| **`make`** | system | `dev` / `build` / `run` / `tunnel` / `loadtest` / `test` per PLAN.md §10. Keep `dev` and `build` as *separate* frontend pipelines — see §7. |

## §3. `modernc.org/sqlite` — the DSN is the whole risk

### Verified locally on this machine (Go 1.26.4 + `modernc.org/sqlite` v1.58.0)

| DSN | Resulting `journal_mode` | Resulting `busy_timeout` | Verdict |
|---|---|---|---|
| *(none)* | `delete` | `0` | ❌ defaults — unsafe under concurrency |
| `?_journal_mode(WAL)&_txlock=immediate` | `delete` | `0` | ❌ **silently ignored** — malformed pragma key, no error raised |
| `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` | `wal` | `5000` | ✅ works |
| `?_journal_mode=WAL&_busy_timeout=5000` (mattn-style shorthand) | `wal` | `5000` | ✅ works |

- **Driver name is `"sqlite"`** for modernc (`"sqlite3"` is mattn's). `sql.Open("sqlite3", ...)` will register-fail at first use.
- `busy_timeout` is applied before `journal_mode` by the driver **regardless of DSN order** in v1.58.0 (pkg.go.dev documents a fixed application order: `_busy_timeout`/`_auto_vacuum` first, then `_pragma` values, then the rest, `_query_only` last). This fixes the older intermittent `SQLITE_BUSY`-on-connection-open bug where WAL was attempted before a busy handler was installed.
- **Pragmas are per-connection.** `db.Exec("PRAGMA journal_mode=WAL")` after `sql.Open` only configures whichever pooled connection happens to be checked out. Put them in the DSN so every new connection gets them. This is the #1 real-world mistake with this driver.
- **`cache=shared` is unusable with modernc** — it fails with `SQLITE_LOCKED (262)` under concurrent writers. Do not copy a mattn DSN that has it.
- **`_txlock=immediate`** is available if you want `BEGIN IMMEDIATE` on the write path so writers serialize at `BEGIN` rather than at first write. For this project's write volume it is optional; add it if ordering tests show contention.

### Concurrency probe — verified locally

### Why modernc despite being slower

## §4. gorilla/websocket vs coder/websocket — KEEP gorilla, know the trigger

| Claim | Reality |
|---|---|
| "Archived in Dec 2022" | True, but the Gorilla toolkit **revived July 2023** under new core maintainers; every package was re-released 2023-11-05. Repos are **not archived**. |
| "No maintainer" | Issue #370 "⚠️ New maintainers needed" is dated **2018-04-01** and is **closed**. |
| "No releases" | `v1.5.3` published **2024-06-14**; last commit push 2025-03-19; 47 open issues / 35 open PRs. Low activity, stable API — not abandoned. |
| "Security concerns" | Two Go vuln-DB entries: GO-2020-0019 (fixed 1.4.1) and **GO-2026-6278 / GHSA-w67g-5rqw-f597** (CWE-338, `math/rand` mask keys, CVSS 6.9, fixed in **1.5.3**). **There is no unfixed vuln at v1.5.3.** |
| "coder/websocket is the modern choice" | True that it is more actively maintained (v1.8.15, 2026-06-15) with `context.Context` throughout, internal concurrent-write safety, zero-alloc I/O, and full permessage-deflate. Real migrations happened in 2026 (erigon #20097, fission #3507, gqlgen deprecation). |

- Exactly **one** goroutine may call `WriteMessage`/`WriteJSON` per connection. PLAN.md's writer goroutine satisfies this.
- `WriteControl` (ping/close) **is** safe to call concurrently with a writer — that is the only exception, and it is what your heartbeat should use.
- Every client needs `SetReadLimit` (e.g. 4 KB), `SetReadDeadline`, and a `SetPongHandler` that refreshes the deadline, or a dead phone will hold a goroutine forever.
- `CheckOrigin`: default rejects cross-origin. Your LAN URL and the `trycloudflare.com` hostname are both *same-origin* to the page that opened the socket, so the default is correct — but it will reject an `/admin` page opened from a *different* detected LAN interface than the one it connected on. **Verify this in the M3 rehearsal**, and if it bites, allow the specific detected origins rather than returning `true` unconditionally.

## §5. QR rendering — `qrcode.react` over `qrcode`

| | `qrcode` 1.5.4 | `qrcode.react` 4.2.0 |
|---|---|---|
| Last published | 2024-08-05 | 2024-12-11 |
| Context7 source reputation | **Low** | **High** (281 snippets, benchmark 97.4) |
| API | Imperative: `QRCode.toCanvas(text, opts)` / `toDataURL` → needs a `ref` + `useEffect` | Declarative: `<QRCodeSVG value={url} size={480} level="M" />` |
| Output for a projector | Raster canvas — blurry when scaled | **SVG — resolution-independent** |
| React 19 | Works via bundler | Peer-declared `^19.0.0` |

## §6. cloudflared — RISK FLAGGED: demote "primary" to "best-effort primary"

### What is verified about quick tunnels

| Property | Evidence |
|---|---|
| Command | `cloudflared tunnel --url http://127.0.0.1:8080` ✅ official docs |
| URL acquisition | Client POSTs `https://api.trycloudflare.com/tunnel`; the JSON `hostname` field is the value printed. Printed via zerolog `INF` in an ASCII box to **stderr** — so PLAN.md §10's "parse URL from stderr" is correct, but scrape for the `https://*.trycloudflare.com` pattern rather than a fixed log prefix. |
| **No SLA / uptime guarantee** | Cloudflare's own disclaimer, printed on every run. "Meant to be used for testing and development, not for deploying a production website." |
| **200 in-flight request cap** | Official docs: beyond it you get `429`. Outbound WS frames do not count, but 20 phones cold-loading a JS bundle simultaneously is a burst worth watching. |
| **No Server-Sent Events** | Official docs. Irrelevant if you use WS only — but it kills any SSE-based fallback idea. |
| **Single HA connection** | `HAConnections` is hardcoded to `1` for quick tunnels. No redundancy. |
| **Random URL every start** | Official docs + community. Fine — PLAN.md's QR auto-update handles it. |
| **Blocked by a pre-existing `config.yaml`** | Official docs: quick tunnels are unsupported if `~/.cloudflared/config.yaml` exists; you must temporarily rename it. **Your tunnel manager must either detect this and warn, or isolate with `--config`/a clean home.** This is a hard, non-obvious failure that would present as "cloudflared installed but --tunnel does nothing". |
| **Default protocol is QUIC** | `cloudflared` sets `--protocol quic` unless overridden. |
| **WebSocket support** | Generally yes — Cloudflare supports WS proxying on all plans, and their docs state quick tunnels handle WebSocket upgrades. |
| ⚠️ **But: `Upgrade: websocket` header can be dropped** | cloudflared **issue #1652**: on default `--protocol=auto` (QUIC), the `Upgrade` header is stripped before reaching the origin; the origin sees a plain `GET` and returns 400; the browser reports `code 1006`, `wasClean: false`, **no `open` event ever fires**. The suggested workaround `--protocol=http2` was reported as *not reliable* — the symptom recurred on the HTTP/2 path on 2026-05-29 with no user-side change. Issue is single-source (GitHub) but detailed and reproducible. |

### Recommended mitigations (all cheap, all worth doing)

## §7. Single-binary embed — two build-order traps

- **Commit `web/dist/.gitkeep`** and make sure `.gitignore` excludes `web/dist/*` but **re-includes** `!web/dist/.gitkeep`. `.gitkeep` only works because of the `all:` prefix — without it, dot-prefixed files are skipped and the embed still fails.
- **`make build` must run `vite build` before `go build`.** Never the reverse.
- **`make dev` must not depend on `web/dist` at all.** Run `go run` and `vite dev` as two processes with Vite proxying `/ws` and `/api` to `:8080`. That is what PLAN.md §10 already specifies — just keep the two pipelines cleanly separated so a broken frontend build never blocks backend iteration.
- **`Cache-Control` on `index.html` must be `no-cache`.** Vite emits content-hashed filenames under `assets/` (safe to cache immutably forever) but `index.html` keeps its name — if it gets cached, players will run a stale app after a tunnel restart, which is indistinguishable from a server bug mid-demo.
- Verified available on Go 1.26.4: `http.FileServerFS`, `fs.Sub`, `http.ServeMux` method patterns (`"GET /api/health"`).

## §8. TypeScript — stay on 6, do not chase 7

## Alternatives Considered

| Category | Recommended | Alternative | Why not (for *this* project) |
|---|---|---|---|
| WebSocket library | `gorilla/websocket` v1.5.3 | `coder/websocket` v1.8.15 | Better maintained and safer by default, but requires redesigning the hub around `conn.Write(ctx,…)`, loses `SetWriteDeadline`, and buys nothing that PLAN.md contract #5 does not already give you. See §4 for the revisit trigger. |
| WebSocket library | `gorilla/websocket` | `golang.org/x/net/websocket` | **Deprecated** (golang/go#18152). Never use. |
| WebSocket library | `gorilla/websocket` | `gobwas/ws` | Lowest-level and fastest, but you hand-roll framing, control frames, and close handshakes. Exactly the wrong trade when correctness-under-pressure is the Core Value. |
| WebSocket library | `gorilla/websocket` | `nhooyr.io/websocket` | Dead import path — the project moved to `github.com/coder/websocket`. If a tutorial/LLM suggests this, it is stale. |
| SQLite driver | `modernc.org/sqlite` v1.58.0 | `mattn/go-sqlite3` | Faster per statement, but needs a C toolchain and breaks `CGO_ENABLED=0`, which is the entire basis of the single-binary constraint. |
| SQLite driver | `modernc.org/sqlite` | `ncruces/go-sqlite3` v0.35.4 | Also pure-Go (WASM-based) and genuinely viable, but smaller ecosystem and less documented DSN surface. modernc is the safer default. |
| Router | `net/http` `ServeMux` | Chi / Echo / Gin / Fiber | 10 routes, one WS endpoint. A framework adds a dependency and a learning surface for zero benefit; Go 1.22+ patterns already cover methods and wildcards. |
| Static serving | `embed.FS` + `http.FileServerFS` | `spa-to-go`, `go:embed` + custom middleware | The stdlib combination is ~30 lines and verified working; third-party SPA handlers add a dependency to save a `fs.Stat` check. |
| QR | `qrcode.react` v4.2.0 | `qrcode` v1.5.4 | Fine, just imperative and raster. See §5. |
| QR | `qrcode.react` v4.2.0 | `qr-code-styling`, `qrcodejs` | Styling-heavy or abandoned; you need a plain black-on-white square that a 2015 phone camera can read. |
| Frontend state | React `useState`/`useReducer` + a thin WS client | Redux / Zustand / TanStack Query | The server is authoritative and the client holds one snapshot + one revision. A store library would be a second source of truth — directly at odds with contract #1. |
| Tunnel | cloudflared quick tunnel | ngrok free | Also ephemeral, adds an account requirement and a session limit, and PLAN.md already excludes paid infra. Cloudflare's path is better (no account, no bandwidth cap). |
| Tunnel | cloudflared quick tunnel | named Cloudflare Tunnel | Requires an account and a domain — PLAN.md explicitly rules out anything requiring setup beyond `--tunnel`. |
| Deployment | single Go binary | Docker / VPS | Explicitly out of scope (PLAN.md non-goals). |
| TypeScript | `~6.0.2` | `typescript@7.0.2` | Faster, but defaults/removed-option churn is unforced risk here. See §8. |

## What NOT to Use

| Avoid | Why | Use instead |
|---|---|---|
| `golang.org/x/net/websocket` | Deprecated for a decade (golang/go#18152); no HTTP/2 WS in 1.26 either. | `gorilla/websocket` v1.5.3 |
| `nhooyr.io/websocket` | Moved to `github.com/coder/websocket`; tutorials referencing it are stale. | `github.com/coder/websocket` (or gorilla) |
| `github.com/gorilla/websocket` **< v1.5.3** | GO-2026-6278 / GHSA-w67g-5rqw-f597: predictable mask keys (CWE-338, CVSS 6.9) enabling request smuggling / cache poisoning. | Pin **`v1.5.3`** exactly |
| **Any** network-library choice other than `net/http`+`embed` for hosting the SPA | You would lose the single-binary property that makes `--tunnel` a one-command demo. | `embed.FS` + `fs.Sub` + `http.FileServerFS` |
| `crypto/rand` misuse — i.e. `math/rand` for tokens | PLAN.md contract #7 already forbids it; note that gorilla's own CVE was exactly this bug. | `crypto/rand` + `hex`/`base64` (or `google/uuid` v1.6.0) |
| `db.Exec("PRAGMA journal_mode=WAL")` at startup | Configures one pooled connection only; the rest silently run with defaults. | Put all pragmas in the **DSN** (§3) |
| `cache=shared` in the SQLite DSN | `modernc` rejects it with `SQLITE_LOCKED (262)` under concurrent writers. | Single-writer pool and/or `_txlock=immediate` |
| `synchronous=OFF` | Unsafe — corruption risk on power loss or panic, not just lost commits. | `synchronous=NORMAL` under WAL |
| `sql.Open("sqlite3", …)` with the modernc import | Driver is registered as **`"sqlite"`**; the `sqlite3` name belongs to mattn. | `sql.Open("sqlite", dsn)` |
| Redis / Postgres / an external DB | Adds an install step and a second failure surface for 20 clients. | SQLite with WAL |
| Docker / VPS in the critical path | Explicit non-goal; also adds a failure mode you cannot debug at the venue. | One binary + cloudflared + hotspot |
| Any Server-Sent Events fallback design | **Quick tunnels do not support SSE.** | WebSocket, or plain HTTP polling |
| `Cloudflare Tunnel` behind a pre-existing `~/.cloudflared/config.yaml` | Quick tunnels are unsupported when that file exists — silent, confusing failure. | Detect at startup and warn; isolate config |
| A frontend state library (Redux/Zustand/TanStack Query) | Creates a second source of truth against contract #1; the server owns state. | One `useReducer` + revision-guarded WS client |
| `@vitejs/plugin-react-swc` | `plugin-react` 6.1.1 now runs on Oxc under Vite 8; the SWC variant is redundant. | `@vitejs/plugin-react` ^6.1.1 |
| ESLint + the full plugin stack | The Vite 8 template now ships `oxlint`; adding ESLint duplicates linting for no gain. | `oxlint` ^1.81.0 |
| `target: "es5"`, `baseUrl`, `moduleResolution: "node"` in tsconfig | Hard errors under TS 7, deprecated under TS 6. | `bundler` / project-root-relative `paths` |

## Stack Patterns by Variant

- Use cloudflared for *everyone*, and have players on 4G/5G mobile data rather than venue WiFi.
- Because LAN fallback is unusable in that building, this is the one scenario where the tunnel's #1652 WebSocket flakiness is a genuine single point of failure — so run the `--protocol` toggle and the WS pre-flight probe (§6), and consider a phone-hotspot backup with the presenter's laptop as the only tunnel client.
- Skip the tunnel entirely: `make run`, hotspot, LAN URLs only.
- Because `{url, healthy}` for the tunnel is already a distinct protocol event, `/screen` and `/admin` degrade correctly with no code path change. Verify this is true in M3 — it is the cheapest insurance in the whole project.
- Add `_txlock=immediate` to the DSN first (serializes writers at `BEGIN` instead of at first write).
- Only then reach for `db.SetMaxOpenConns(1)`, and only on the write handle. If you split handles, open a read-only handle with `mode=ro` — WAL lets readers proceed concurrently.
- Confirm the Vite config uses `ws: true` on the `/ws` key with `target: 'ws://127.0.0.1:8080'`. Requests matching a proxy rule are **not** transformed by Vite, which is what you want.
- Do **not** set `rewriteWsOrigin: true` — the Vite docs explicitly caution it opens CSRF, and your server-side `CheckOrigin` is a real defence here.
- If you ever run Vite in `middlewareMode`, you must pass `server: parentServer` or the WS proxy will not bind.
- The loadtest must assert **revision monotonicity with no gaps** on every client, and separately assert the *drop-and-resync* path by shrinking one client's channel buffer to force a drop. An ordering test that never fills a channel proves nothing about contract #5.

## Version Compatibility

| Package A | Compatible with | Notes |
|---|---|---|
| `go 1.26.x` | `gorilla/websocket` v1.5.3 | gorilla declares `go 1.12` minimum — no constraint. Verified: resolves and builds on go1.26.4. |
| `go 1.26.x` | `modernc.org/sqlite` v1.58.0 | Verified: builds and runs on go1.26.4; pulls `modernc.org/libc` v1.75.6, `modernc.org/memory` v1.12.1, `modernc.org/mathutil` v1.7.1, `golang.org/x/sys` v0.47.0. |
| `modernc.org/sqlite` v1.58.0 | `CGO_ENABLED=0` | The entire point. Verify `CGO_ENABLED=0 go build` in CI from day one — a cgo dependency sneaking in is silent until the build machine differs. |
| Node 24.17.0 | Vite 8.3.0 | Requires `^20.19.0 \|\| >=22.12.0` ✅ |
| Node 24.17.0 | Vitest 5.0.0 | Requires `^22.12.0 \|\| ^24.0.0 \|\| >=26.0.0` ✅ |
| React 19.2.8 | `@vitejs/plugin-react` 6.1.1 | Peer requires `vite ^8.0.0` ✅ (plus optional `oxc-transform-react`, `@rolldown/plugin-babel` peers) |
| React 19.2.8 | `qrcode.react` 4.2.0 | Peer `react: ^16.8.0 \|\| ^17 \|\| ^18 \|\| ^19` ✅ |
| TypeScript `~6.0.2` | Vite 8.3.0 scaffold | This is the template's own pin — do not raise it without doing §8's checklist. |
| Go 1.26 | Go 1.27.1 (current stable) | 1.26 is inside the supported two-release window; upgrade is not forced. Patch to ≥ **1.26.8** (installed 1.26.4 is four security patches behind). |

## Risk Register (locked-stack items only)

| # | Risk | Severity | Confidence | Action |
|---|---|---|---|---|
| 1 | SQLite runs in `delete` journal mode silently if the DSN pragma syntax is wrong | **High** — corrupts timing/ordering assumptions under load, surfaces mid-demo | HIGH (verified locally, 4 DSN variants) | Use the exact DSN in §3; add a boot assertion that `PRAGMA journal_mode` returns `wal`. |
| 2 | Tunnel WebSocket upgrade fails (`Upgrade` header dropped) | **High** — the "primary" path silently produces `code 1006` with no `open` event | MEDIUM (single detailed GitHub issue + Cloudflare docs confirming WS *is* supported) | WS pre-flight probe before marking healthy; `--protocol` toggle; LAN co-primary; rehearse at venue (§6). |
| 3 | `//go:embed all:web/dist` breaks `go build` on a clean clone — **and the repo's current `.gitignore` line 4 (`web/dist/`) guarantees the placeholder is never committed** | Medium-High — blocks M0 and any fresh checkout, but invisible on the machine where `vite build` was last run | HIGH (verified locally, 3 embed cases + `.gitignore` read) | Change `.gitignore` to `web/dist/*` + `!web/dist/.gitkeep`; commit the placeholder; `build` runs `vite build` first (§7). |
| 4 | `index.html` cached after a tunnel restart → players run a stale app | Medium — looks like a server bug, wastes presentation minutes | HIGH (mechanism verified) | `Cache-Control: no-cache` on HTML, `immutable` on `/assets/*` (§7). |
| 5 | gorilla/websocket is low-activity; a future vuln may go unpatched | Low — no unfixed vuln at v1.5.3; API frozen | HIGH (vuln DB + release dates verified) | Pin exact `v1.5.3`; re-check the Go vuln DB before the presentation; §4 has the migration recipe. |
| 6 | `CheckOrigin` rejects the admin page when opened on a different detected LAN interface | Low-Medium — would look like "admin is broken on LAN" | MEDIUM (gorilla default behaviour; project-specific consequence reasoned, **not** tested) | Test during M3 with two real interfaces. |
| 7 | TypeScript 7 pulled in by a floating `^` range and the tsconfig errors | Low — loud, immediate, fixable | HIGH (TS 7 GA + defaults documented) | Keep the scaffold's `~6.0.2`. |
| 8 | `~/.cloudflared/config.yaml` present → `--tunnel` silently does nothing | Low-Medium — confusing, but local and diagnosable | HIGH (official Cloudflare docs) | Detect at startup, print an actionable message (§6). |

## Sources

- `proxy.golang.org/github.com/gorilla/websocket/@latest` → `v1.5.3` (2024-06-14) — exact version pin
- `proxy.golang.org/modernc.org/sqlite/@latest` → `v1.58.0` (2026-09-01) — exact version pin
- `proxy.golang.org/github.com/coder/websocket/@latest` → `v1.8.15`; `github.com/google/uuid` → `v1.6.0`; `github.com/ncruces/go-sqlite3` → `v0.35.4`
- `go.dev/dl/?mode=json` → `go1.27.1` stable, `go1.26.8` latest 1.26 patch
- `npm view <pkg> version|engines|peerDependencies|time` → qrcode 1.5.4, qrcode.react 4.2.0, vite 8.3.0, vitest 5.0.0, react 19.3.0, @vitejs/plugin-react 6.1.1, typescript 7.0.2 (`latest`), @types/react 19.3.0
- Go vulnerability DB (`vuln.go.dev/index/modules.json`) → gorilla/websocket: GO-2020-0019 (fixed 1.4.1), GO-2026-6278 (fixed 1.5.3); modernc.org/sqlite: **no entries**
- `npm create vite@latest myapp -- --template react-ts` (create-vite 9.2.1, executed locally) → full scaffold: package.json, tsconfig.json/app/node, vite.config.ts
- `/tmp/opencode/gospike` — modernc.org/sqlite v1.58.0 DSN matrix (4 variants), 60-goroutine concurrent-writer probe, `http.ServeMux` method patterns, `http.FileServerFS` presence
- `/tmp/opencode/embedtest` — `//go:embed all:web/dist` across three directory states
- `/gorilla/websocket` — upgrade, `CheckOrigin`, ping/pong with `SetReadDeadline`+`SetPongHandler`, `WriteControl`, read limits
- `/websites/pkg_go_dev_modernc_org_sqlite` — `Driver.Open` DSN parameters, shorthand-key application order, `NewConnector`
- `/vitejs/vite` — `server.proxy` with `ws: true`, `rewriteWsOrigin` caution, middlewareMode parent-server requirement, `server.ws` (HMR, distinct from app proxy)
- `/soldair/node-qrcode` — `toCanvas`/`toDataURL` browser API
- `/zpao/qrcode.react` — `QRCodeSVG`/`QRCodeCanvas` props
- `/cloudflare/cloudflared` — `tunnel --url` syntax, `TUNNEL_PROTOCOL` env, `QuickTunnel` response struct, `RunQuickTunnel` decision logic
- `/golang/go` — version list through `go1.26.7`
- Go 1.26 release notes + blog (`go.dev/doc/go1.26`, GA 2026-02-10) — no stdlib WebSocket; `ServeMux` 307 trailing-slash change; `ReverseProxy.Director` deprecation
- `developers.cloudflare.com/.../trycloudflare/` — the authoritative limitations list (no SLA, 200 in-flight cap, no SSE, `config.yaml` conflict)
- `developers.cloudflare.com/network/websockets/` — WS supported on all plans
- Cloudflare quick-tunnel disclaimer text (source: `cmd/cloudflared/tunnel/quick_tunnel.go`)
- `vite.dev/blog/announcing-vite8` (2026-03-12) — Node requirements, Rolldown/Oxc merge
- `devblogs.microsoft.com/typescript/announcing-typescript-7-0/` (GA 2026-07-08) — new defaults, hard-error list, no programmatic API until 7.1
- `vitest.dev` / npm engines metadata
- `github.com/cloudflare/cloudflared/issues/1652` — `Upgrade: websocket` dropped on `--protocol=auto`/QUIC; `--protocol=http2` not reliably fixing it (single source, detailed reproduction)
- `github.com/cloudflare/cloudflared/issues/1313` — 2024 quick-tunnel WS outage; Cloudflare response "designed only to support web traffic"
- Benchmarks: Gitea PR #37105 (mattn→modernc, WAL + `synchronous=NORMAL`), `kalafut/go-sqlite-bench`, Mako `sqlite/connect-from-go` (2026-06-16), `freshteapot.net` DSN-order analysis (2026-09-08)
- `github.com/endoflife-date/endoflife.date` PR #10239 — documents that the "gorilla is EOL/archived" classification is a **false positive**

<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->

## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->

## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->

## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->

## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:

- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->

## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
