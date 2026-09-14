# AuctionLab — Build Plan (v1.2)

Web app demo đấu giá cho presentation môn học. Không auth cho player, realtime qua WebSocket,
admin điều khiển toàn bộ, người chơi dùng phone quét QR để tham gia.

> v1.1 — adapted từ review: role-specific projections, runtime timing persistence,
> sealed-bid semantics, WS revision/roles, cùng các fix nhỏ (anti-snipe, LAN detect,
> reset semantics, admin capability token, cloudflared optional).
>
> v1.2 — broadcast ordering (enqueue trong lock + per-client channel), anti-snipe
> `max_extensions`, crypto/rand cho tokens, ghi chú outsider risk của quick tunnel.

---

## 1. Mục tiêu & non-goals

**Mục tiêu**
- User nhập nickname → tham gia game đấu giá nhiều vòng.
- Admin config item (giá, loại đấu giá, thời gian, hint range...) và điều khiển thứ tự/thời điểm.
- 1 item LIVE tại một thời điểm; ai thắng item nào thì `budget −= price; budget += truth_value`.
- Kết thúc: top 3 budget = winners.
- Admin view để điều khiển + screen view chiếu màn hình lớn.
- Chạy được qua cloudflared tunnel (primary) và LAN (fallback).

**Non-goals**
- Authentication cho player / tài khoản / phân quyền.
- Nhiều phòng (room) song song, nhiều game cùng lúc.
- Upload ảnh, thanh toán thật, mobile app.
- Scale production (20 connections là mục tiêu, không phải 20k).

---

## 2. Stack (đã chốt)

| Thành phần | Chọn | Ghi chú |
|---|---|---|
| Backend | **Go 1.26** (đã cài) | `net/http` + `github.com/gorilla/websocket` |
| Storage | **SQLite** `modernc.org/sqlite` | Pure Go, không cần CGO; WAL mode |
| Frontend | **React + Vite + TypeScript** | Node v24 đã cài |
| QR | Render client-side bằng npm `qrcode` | Server chỉ trả join URL |
| Tunnel | **cloudflared** (optional, external dep) | Chỉ dùng khi chạy `--tunnel` |
| Deploy | App build thành **1 binary** | React build embed bằng `embed.FS`; cloudflared là optional runtime dependency |
| Build | Makefile | `make dev` / `make build` / `make run` / `make tunnel` / `make loadtest` |

---

## 3. Kiến trúc

```
┌────────────┐   ┌────────────┐   ┌─────────────┐
│  Player    │   │  Admin     │   │  Screen     │
│  (phone)   │   │  (laptop)  │   │  (projector)│
└─────┬──────┘   └─────┬──────┘   └──────┬──────┘
      │    WS + HTTP   │                 │
      └────────────────┼─────────────────┘
                       ▼
        ┌──────────────────────────────┐
        │   Go server (1 binary)       │
        │   ├─ static files (embed)    │
        │   ├─ /ws hub (broadcast)     │
        │   ├─ game engine (mutex)     │
        │   ├─ projections theo role   │
        │   ├─ sqlite snapshot         │
        │   ├─ interface detect + QR   │
        │   └─ cloudflared manager     │
        └──────────────────────────────┘
```

### 3.1 Contracts bất biến (architecture invariants)

1. **Role-specific projections** — không bao giờ serialize `InternalState` trực tiếp ra network.
   ```go
   type InternalState struct { /* truth_value, sealed bids, mọi budget, runtime... */ }

   func (s *InternalState) PlayerView(playerID string) PlayerSnapshot
   func (s *InternalState) AdminView() AdminSnapshot
   func (s *InternalState) ScreenView() ScreenSnapshot
   ```
   - `PlayerSnapshot`: public item info + hint range + **own** budget + **own** sealed bid.
   - `AdminSnapshot`: mọi thứ (kể cả truth_value, sealed bids).
   - `ScreenSnapshot`: public item info + leaderboard; **không** truth_value trước `SETTLED`.
   - Test bằng JSON assert: `require.NotContains(t, json, "truth_value")`.

2. **Server authoritative + một mutation path duy nhất** — mọi mutation serialize qua engine mutex.

3. **Runtime timing persist** — `started_at` / `deadline_at` nằm trong SQLite, không chỉ RAM.
   Crash → resume theo wall-clock (xem mục 5, 6).

4. **WS revision** — mỗi mutation thành công `revision++`; snapshot và event mang revision;
   client ignore event có seq cũ hơn snapshot hiện tại.

5. **Broadcast = non-blocking enqueue TRONG lock, I/O ngoài lock**:
   Hub dùng **buffered channel per-client + 1 writer goroutine/client** (gorilla pattern).
   Enqueue nằm trong critical section để bảo toàn thứ tự revision; send là non-blocking,
   channel đầy → drop event + đánh dấu client cần resync (writer gửi `state.snapshot` mới
   trước khi drain tiếp).
   **Không bao giờ unlock rồi mới broadcast** — preemption giữa `Unlock()` và `Broadcast()`
   có thể làm event N+1 tới trước event N; client "ignore seq cũ" sẽ âm thầm mất payload
   (bug chỉ lộ khi bid dồn dập, không crash).
   ```go
   engine.mu.Lock()
   event, projections, err := engine.Apply(cmd)
   if err != nil { engine.mu.Unlock(); return err }
   store.Commit(...)                  // cùng transaction
   hub.Enqueue(projections, revision) // non-blocking per-client channel, TRONG lock
   engine.mu.Unlock()
   // writer goroutine per client: drain channel → real I/O, FIFO, giữ đúng thứ tự
   ```

6. **Duplicate session** — một player chỉ có 1 active connection; connection mới cùng token
   supersede connection cũ (đóng conn cũ với code `4001`).

7. **Secrets dùng `crypto/rand`** — player token, admin key, session id (không dùng `math/rand`).

---

## 4. Cấu trúc thư mục

```
auctionLab/
├── PLAN.md
├── Makefile
├── go.mod
├── embed.go                  # embed web/dist
├── cmd/server/main.go        # flags: --tunnel, --public-url, --port
├── internal/
│   ├── game/                 # state machine + auction logic
│   │   ├── engine.go         # mutex, phase transitions, revision, settle chung
│   │   ├── project.go        # PlayerView / AdminView / ScreenView
│   │   ├── english.go
│   │   ├── dutch.go
│   │   ├── sealed.go         # first-price + vickrey
│   │   ├── validation.go     # budget/price rules
│   │   └── *_test.go
│   ├── hub/                  # ws clients, roles, per-client channel + writer, resync, supersede
│   ├── store/                # schema, load/save snapshot, runtime timing
│   ├── api/                  # http handlers, static, join-info, admin key
│   └── tunnel/               # cloudflared spawn + URL scrape + restart
├── web/
│   ├── src/
│   │   ├── routes/player/    # join, lobby, live, settled, finale
│   │   ├── routes/admin/     # setup, control, players
│   │   ├── routes/screen/    # lobby QR, live dashboard, reveal, podium
│   │   ├── ws/client.ts      # reconnect + resync + revision check
│   │   ├── components/
│   │   └── lib/format.ts     # VND formatting, timer helpers
│   └── package.json
└── scripts/
    ├── loadtest/main.go      # N bot giả lập cho rehearsal
    └── install-cloudflared.sh
```

---

## 5. Data model (SQLite)

```sql
CREATE TABLE players (
  id           TEXT PRIMARY KEY,           -- uuid
  nickname     TEXT NOT NULL UNIQUE,
  token        TEXT NOT NULL UNIQUE,       -- session token (localStorage), crypto/rand
  budget       INTEGER NOT NULL,
  total_value  INTEGER NOT NULL DEFAULT 0, -- tie-break
  joined_at    INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL            -- connected KHÔNG persist (ephemeral hub state)
);

CREATE TABLE items (
  id            TEXT PRIMARY KEY,
  order_index   INTEGER NOT NULL,
  name          TEXT NOT NULL,
  description   TEXT,
  image_url     TEXT,
  auction_type  TEXT NOT NULL,             -- english | dutch | sealed_first | vickrey
  truth_value   INTEGER NOT NULL,          -- common value, ẩn với player
  hint_min      INTEGER,                   -- hint range hiện từ ITEM_INTRO
  hint_max      INTEGER,
  start_price   INTEGER NOT NULL,
  min_increment INTEGER,                   -- english
  floor_price   INTEGER,                   -- dutch
  decrement_step INTEGER,                  -- dutch
  decrement_interval_ms INTEGER,           -- dutch
  duration_ms   INTEGER,                   -- english | sealed
  anti_snipe_mode TEXT DEFAULT 'disabled', -- disabled | extend (english)
  anti_snipe_window_ms INTEGER,
  max_extensions INTEGER,                  -- optional; null = unlimited
  status        TEXT NOT NULL DEFAULT 'pending' -- pending|intro|live|settled|skipped
);

-- Effective sealed bid: 1 row / player / item, edit ghi đè amount
CREATE TABLE sealed_bids (
  item_id      TEXT NOT NULL REFERENCES items(id),
  player_id    TEXT NOT NULL REFERENCES players(id),
  amount       INTEGER NOT NULL,
  submitted_at INTEGER NOT NULL,           -- lần nộp ĐẦU (dùng cho tie-break)
  updated_at   INTEGER NOT NULL,           -- lần sửa cuối
  PRIMARY KEY (item_id, player_id)
);

-- Audit trail: open bids + accepts + mọi lần edit sealed (append-only)
CREATE TABLE bids (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id    TEXT NOT NULL REFERENCES items(id),
  player_id  TEXT NOT NULL REFERENCES players(id),
  amount     INTEGER NOT NULL,
  kind       TEXT NOT NULL,                -- bid | accept | sealed_edit
  created_at INTEGER NOT NULL
);

-- Runtime timing: đủ để resume đúng chỗ sau crash
CREATE TABLE auction_runtime (
  item_id     TEXT PRIMARY KEY REFERENCES items(id),
  started_at  INTEGER NOT NULL,
  deadline_at INTEGER,                     -- english|sealed: wall-clock deadline (đã tính anti-snipe)
  settled_at  INTEGER,
  revision    INTEGER NOT NULL
);

CREATE TABLE game (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  phase           TEXT NOT NULL,           -- lobby|item_intro|live|settled|finale
  current_item_id TEXT REFERENCES items(id),
  revision        INTEGER NOT NULL,        -- monotonic game-state revision
  config_json     TEXT NOT NULL,           -- starting_budget, currency, defaults...
  updated_at      INTEGER NOT NULL
);

CREATE TABLE events (                      -- audit log, hữu ích khi debug/present
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  type         TEXT NOT NULL,
  payload_json TEXT,
  created_at   INTEGER NOT NULL
);
```

**Ghi chú quan trọng**
- Dutch **không persist price mỗi tick**. Derive từ `started_at`:
  ```
  elapsed = now − started_at
  steps   = elapsed / decrement_interval
  price   = max(start_price − steps × decrement_step, floor_price)
  ```
- English/sealed persist `deadline_at`, update mỗi lần anti-snipe extend (cùng transaction).
- Boot: load toàn bộ state; `phase = live` → resume theo wall-clock (mục 6).
- Config mặc định: `starting_budget = 1000000`, `currency = VND`.

---

## 6. Game state machine

```
LOBBY ──admin start──▶ ITEM_INTRO ──admin start──▶ LIVE ──hết giờ/force-end──▶ SETTLED
  ▲                        │                        │                              │
  │                        └────────admin next──────┴────────────admin next────────┘
  │                                                                                │
  └────────────────────── admin reset ──────────────────────────────────────────┘
                                                                                   ▼
                                                             hết item cuối ──▶ FINALE
```

| Phase | Player thấy gì | Admin làm gì |
|---|---|---|
| `LOBBY` | QR/URL, danh sách người chơi | Config items, bấm start item đầu |
| `ITEM_INTRO` | Tên item, loại, hint range, budget của mình | Bấm start / skip item |
| `LIVE` | UI theo loại đấu giá + timer | Force-end / xem trạng thái |
| `SETTLED` | Reveal value, winner, giá, budget delta | Next item / kết thúc game |
| `FINALE` | Top 3 + rank của mình | Reset game mới |

**Crash recovery semantics (khóa contract)**
- Restart khi `LIVE` → auction tiếp tục theo **wall-clock deadline**.
- Nếu `deadline_at <= now` lúc boot → **settle ngay khi boot**.
- Dutch → derive giá từ `started_at`; nếu đã qua floor → đứng ở `floor_price`, chờ accept/force-end.
- Không cần thêm `PAUSED` state.

**Admin actions & reset semantics**
| Action | Hiệu ứng |
|---|---|
| `item.start` | `ITEM_INTRO` → `LIVE`, ghi `auction_runtime` (started_at, deadline_at), revision++ |
| `item.force_end` | `LIVE` → `SETTLED`, settle theo luật loại đấu giá |
| `item.next` | `SETTLED` → item kế `ITEM_INTRO`; hết item → `FINALE` |
| `item.skip` | Item `pending` → `skipped`, chuyển item kế |
| `game.reset` (keep players) | Clear bids + sealed_bids + auction_runtime; budgets → starting_budget; total_value = 0; item statuses → pending; current_item = null; phase = lobby; **giữ item config + players** |
| `game.reset_players` | Như trên + xóa toàn bộ players |

Nút reset tách riêng 2 mode — không bao giờ xóa item setup ngoài ý muốn.

---

## 7. Auction mechanics

**Luật chung (mọi loại)**
- Bid/accept phải `≤ budget hiện tại` của player — validate tại thời điểm bid.
- Thắng item: `budget = budget − price + truth_value`, `total_value += truth_value`.
- `truth_value` là common value, **ẩn** trong lúc đấu; `hint_min/hint_max` hiện từ `ITEM_INTRO`;
  value thật công bố tại `SETTLED`.
- Không ai bid → item `unsold`, không ai bị trừ budget.

**English**
- First bid `≥ start_price`; bid sau `≥ current + min_increment`.
- Không cho outbid chính mình (đang là top bidder → reject).
- Countdown `duration_ms`; anti-snipe per item:
  - `disabled`: hết giờ là chốt (hard deadline).
  - `extend`: bid trong `anti_snipe_window_ms` cuối → `deadline_at = now + anti_snipe_window_ms`,
    persist ngay trong cùng transaction. `max_extensions` (optional) cap số lần extend —
    hết quota thì giữ nguyên deadline hiện tại (hard stop).
- Settle khi `deadline_at` tới hoặc force-end: top bidder thắng, trả giá mình đã bid.

**Dutch**
- Giá bắt đầu `start_price`, giảm `decrement_step` mỗi `decrement_interval_ms`, dừng ở `floor_price`.
- Player bấm ACCEPT → thắng ngay tại giá hiện tại (nếu `≤ budget`).
- First accept wins — serialize qua engine nên không tranh chấp.
- Chạy đến khi có accept hoặc force-end (tới floor mà không ai accept → unsold).

**Sealed-bid first-price**
- Window `duration_ms`; mỗi người nộp 1 bid, **được sửa** đến khi đóng; bid ẩn với người khác.
- Ràng buộc: `start_price ≤ bid ≤ budget`.
- Effective bid = row trong `sealed_bids` (không phải append `bids`).
- Settle: highest effective bid wins, trả đúng giá mình đặt.
- **Tie-break (khóa contract)**: equal final bids → ai có `submitted_at` (lần nộp đầu) sớm hơn thắng.

**Vickrey (second-price sealed)**
- Như sealed; winner trả `max(second_highest, start_price)`.
- Nhờ ràng buộc `bid ≥ start_price`, winner **không bao giờ** trả nhiều hơn bid của chính mình.
- Chỉ 1 bidder → trả `start_price`. Tie highest → earliest `submitted_at`, trả second price.

**Finale**
- Sort: `budget DESC → total_value DESC → joined_at ASC`; top 3 = winners.

---

## 8. WS protocol

Envelope: `{ "type": "...", "seq": <revision>, "ts": <server_ms>, "data": { ... } }`

- `seq` = game-state revision tại thời điểm event được tạo.
- `state.snapshot` mang revision hiện tại; event tiếp theo = `revision + 1`.
- Client nhận event có `seq ≤ current revision` → **ignore**.
- Thứ tự revision được bảo toàn vì enqueue nằm trong lock (contract #5, §3.1).

**Roles & handshake**
| Client | Gửi | Nhận |
|---|---|---|
| Player | `join {role:"player", nickname, token?}` | `hello {role, player, token, revision}` hoặc `{error}` |
| Screen | `join {role:"screen"}` | `hello {role:"screen", revision}` + snapshot |
| Admin | `join {role:"admin", key}` | `hello {role:"admin", revision}` hoặc reject |

- Player flow không đổi: nickname → chơi. Không account, không login.
- Admin dùng **capability token** (`ADMIN_KEY` env, hoặc auto-generate lúc boot và in ra console,
  sinh bằng `crypto/rand`): `/admin?k=...` (set cookie) + WS admin kèm key.
  Vài chục dòng code, chặn red-team giữa giờ present.
- Mọi lệnh `admin.*` bị reject nếu connection không có role admin.
- Duplicate token: connection mới supersede connection cũ (đóng cũ code `4001`).

| Client → Server | Payload | Ghi chú |
|---|---|---|
| `join` | `{role, nickname?, token?, key?}` | |
| `resync` | `{}` | xin snapshot đúng role |
| `bid.place` | `{amount}` | english |
| `dutch.accept` | `{}` | dutch |
| `sealed.submit` | `{amount}` | sealed/vickrey, sửa được khi còn window |
| `admin.item.upsert` | `{item}` | create/update |
| `admin.item.delete` | `{id}` | |
| `admin.item.reorder` | `{ids[]}` | |
| `admin.item.start` | `{id}` | |
| `admin.item.force_end` | `{}` | |
| `admin.item.next` | `{}` | |
| `admin.game.reset` | `{mode: "keep_players"\|"clear_players"}` | |
| `admin.config.update` | `{config}` | starting_budget... |

| Server → Client | Payload | Ghi chú |
|---|---|---|
| `hello` | `{role, player?, token?, revision}` hoặc `{error}` | sau `join` |
| `state.snapshot` | projection theo role + revision | connect/resync |
| `phase.changed` | `{phase, current_item}` | |
| `item.intro` | item public + hint range | không có truth_value |
| `item.live` | params theo loại | start_price, duration... |
| `price.tick` | `{current, remaining_ms}` | english/dutch |
| `bid.accepted` | `{player, amount}` | broadcast |
| `bid.rejected` | `{reason}` | riêng cho người bid |
| `sealed.received` | `{count, total}` | không lộ giá |
| `item.settled` | `{winner, price, truth_value, budgets}` | reveal |
| `leaderboard` | `[{player, budget, total_value}]` | |
| `finale` | top 3 + stats | |
| `tunnel.status` | `{url, healthy}` | admin/screen |
| `error` | `{message}` | |

Timer: server broadcast `price.tick` mỗi 250ms khi LIVE (dutch bắt buộc, english kèm `remaining_ms`);
client nội suy giữa các tick để hiển thị mượt.

---

## 9. Views

**Player `/`**
- Join: nickname (unique, validate server-side), token lưu localStorage.
- Lobby: danh sách người chơi, trạng thái kết nối.
- Intro: item info + hint range + budget của mình.
- Live: english → giá hiện tại, top bidder, input + quick buttons (+10k/+50k/+100k), timer;
  dutch → giá giảm animated + nút ACCEPT to; sealed/vickrey → input + submit/update + countdown.
- Settled: reveal value, winner, giá, budget delta.
- Finale: podium + rank của mình.
- Auto-reconnect + resync (revision check), indicator mất kết nối.

**Admin `/admin`** (yêu cầu `ADMIN_KEY`)
- Tab Setup: starting budget, item CRUD (fields theo từng loại đấu giá), reorder, QR panel.
- Tab Control: phase hiện tại, start / force-end / next / skip / reset (2 mode).
- Tab Players: danh sách, budget, total_value, trạng thái connected.
- Admin luôn thấy `truth_value` và net của winner (dành cho người dẫn chương trình).
- Join URLs panel: Tunnel URL + LAN URL(s), chọn được nếu nhiều interface.

**Screen `/screen`** (read-only, chiếu lớn)
- Lobby: QR + URL cỡ lớn + player grid đang join.
- Live: item, loại, hint range, giá/timer, bid feed (english), "X/Y đã nộp" (sealed).
- Settled: animation reveal value + winner + giá + leaderboard cập nhật.
- Finale: podium top 3.
- Projection chỉ chứa public data (không truth_value trước `SETTLED`); full leaderboard budget
  là trade-off đã chấp nhận. Optional: set `SCREEN_KEY` nếu muốn chặn xem trước.

---

## 10. Deployment

**Thứ tự ưu tiên kết nối**
1. **cloudflared tunnel** (primary, optional runtime dep): `auctionlab --tunnel` / `make tunnel`.
   Server spawn subprocess `cloudflared tunnel --url http://127.0.0.1:8080`, parse URL
   `https://*.trycloudflare.com` từ stderr, broadcast cho admin/screen, auto-restart với backoff.
   Không cần LAN chung, người chơi dùng 4G.
   **Mặc định `make run` không spawn tunnel** — app chạy bình thường nếu cloudflared không được cài.
   Lưu ý: quick tunnel URL là **public** — ai có link đều join được (trade-off có chủ đích cho demo;
   `ADMIN_KEY` chặn quyền admin; optional `JOIN_CODE` nếu muốn siết).
2. **LAN** (fallback khi venue mất internet): enumerate network interfaces → loại loopback/down →
   lấy private IPv4 (prefer `192.168.x.x` / `10.x.x.x` / `172.16–31.x.x`).
   **Không phụ thuộc route tới public IP.** Nhiều interface → admin chọn URL.
3. **Hotspot điện thoại** (chốt cuối, offline hoàn toàn): cả lớp vào hotspot của presenter.
   Lưu ý giới hạn client (iOS ~5, Android ~10) → chấp nhận giảm số người hoặc chơi theo nhóm.

- `PUBLIC_URL` env override mọi auto-detect.
- QR render client-side từ join URL; admin/screen tự cập nhật khi URL đổi.
- cloudflared cài bằng `scripts/install-cloudflared.sh` — single binary, không cần root nếu để local.
- **Không mua VPS** — không cần thiết cho 15–20 connections.

**Makefile targets**
```
make dev       # go run + vite dev (proxy /ws + /api → :8080)
make build     # vite build → go build (embed) → bin/auctionlab
make run       # chạy binary local/LAN, không tunnel
make tunnel    # chạy kèm managed cloudflared
make loadtest  # 20 bot giả lập
make test      # go test ./... + vitest
```

---

## 11. Milestones

| # | Tên | Nội dung | DoD | Ước lượng |
|---|---|---|---|---|
| M0 | Skeleton | git init, go.mod, Makefile, static embed, /api/health, /ws echo, Vite app 3 routes, dev proxy | `make dev` chạy, WS echo OK | 0.5 ngày |
| M1 | Engine + persistence + projections | SQLite schema, snapshot load/save, state machine, **role projections**, **WS revision/resync**, hub per-client channel + writer goroutine, join/token + supersede, lobby UI | 2 browser join thấy nhau; restart server → state còn nguyên; JSON leak test pass | 1 ngày |
| M2 | English E2E + crash recovery | Item CRUD, start/force-end/next, bid validation, anti-snipe extend/disabled + max_extensions, timer, settle, budget update, reveal, **auction_runtime persist** | Full vòng English; **kill -9 giữa LIVE sau anti-snipe extend → restart → deadline đúng, game tiếp tục** | 1 ngày |
| M3 | Admin + Screen | Admin 3 tabs, item editor, control panel, ADMIN_KEY; screen lobby QR / live / reveal / leaderboard; LAN interface detect + QR | Chiếu /screen full flow, phone khác máy join qua LAN; admin key chặn WS admin | 1 ngày |
| M4 | Dutch | Price clock derive từ started_at, accept, floor, settle; concurrent accept test; UI accept + giá giảm | Full vòng Dutch; 2 accept đồng thời → chỉ 1 thắng; restart giữa Dutch → giá derive đúng | 0.5 ngày |
| M5 | Sealed + Vickrey | `sealed_bids` effective + edit, settle first-price; second-price + edge cases (tie, 1 bidder, 0 bid) | Cả 2 chạy end-to-end, edge tests pass; sửa bid 3 lần → chỉ final tham gia settle | 0.5–1 ngày |
| M6 | cloudflared + polish + rehearsal | tunnel manager + auto-restart, reconnect hardening, VND format, reveal animation, podium, loadtest 20 bots rapid-fire (ordering), rehearsal checklist | 20 bots + 2 phone thật chơi game hoàn chỉnh qua tunnel; cloudflared chết → LAN/local vẫn chạy | 1 ngày |

Tổng: ~5.5 ngày làm việc. Có thể nén còn 2–3 ngày nếu tập trung liên tục.

---

## 12. Testing

- **Unit (Go)**: table-driven cho từng auction type — bid validation, anti-snipe, settle, tie, no-bid,
  budget edges (`bid == budget`, `value > budget`, budget âm không xảy ra).
- **Leakage tests (bắt buộc)**: serialize projection ra JSON rồi assert —
  `require.NotContains(t, json, "truth_value")`; player A không đọc được sealed bid của player B.
  UI giấu field vô nghĩa nếu API vẫn gửi xuống browser.
- **Integration (Go)**: WS test bằng `httptest` + gorilla dial — 2 players join → bid → settle.
- **Ordering (bắt buộc)**: client assert revision tăng đơn điệu, không gap; rapid-fire bid là
  điều kiện trigger bug reorder — event N+1 không bao giờ tới trước N. Channel đầy → drop +
  resync snapshot, không mất state vĩnh viễn.
- **Load**: `scripts/loadtest` — N bot join + bid rapid-fire, đo latency + xác nhận không race/reorder.
- **Frontend**: Vitest cho `format.ts` (VND) + timer interpolation; không over-test UI.
- **Manual UAT checklist** (trước presentation):
  - [ ] Join từ iOS Safari + Android Chrome
  - [ ] Player snapshot **không chứa** truth_value khi intro/live
  - [ ] Player A không đọc được sealed bid của Player B
  - [ ] English: bid, anti-snipe extend kích hoạt đúng (kể cả max_extensions cap), disabled cutoff đúng
  - [ ] English anti-snipe extend → kill server → restart → deadline vẫn đúng
  - [ ] Duplicate token connection: new WS supersedes old WS
  - [ ] Sealed player edit 3 lần: chỉ final bid tham gia settle, tie timestamp đúng semantics
  - [ ] Dutch: accept đúng giá tại thời điểm bấm
  - [ ] Sealed/Vickrey: không lộ bid trước khi settle, winner trả đúng giá
  - [ ] Reveal + budget math đúng cho từng winner
  - [ ] Tunnel: tắt 4G/WiFi của 1 phone → reconnect → đúng người, đúng budget
  - [ ] Kill server giữa LIVE → restart → resume đúng phase
  - [ ] Cloudflared chết → game engine vẫn hoạt động LAN/local
  - [ ] Rapid-fire loadtest: revision monotonic trên mọi client, không miss payload
  - [ ] /screen hiển thị đúng trên projector (độ phân giải, font size)

---

## 13. Risks & mitigations

| Risk | Mitigation |
|---|---|
| WiFi trường bật client isolation | cloudflared primary; test venue trước; hotspot là chốt cuối |
| Venue mất internet | LAN fallback; hotspot offline; LAN detect không phụ thuộc public route |
| cloudflared chết giữa game | auto-restart backoff + status hiện trên /admin; engine không phụ thuộc tunnel |
| Tunnel URL đổi mỗi lần chạy | QR auto-update từ tunnel.status; không cần nhớ URL |
| Player mở /admin qua tunnel | ADMIN_KEY capability token; WS admin reject nếu thiếu key |
| Outsider join/spam qua tunnel URL | Trade-off có chủ đích cho demo; optional `JOIN_CODE` nếu cần siết |
| Accept/bid đồng thời | serialize qua engine mutex; server timestamp quyết định |
| Event reorder khi bid dồn dập | enqueue trong lock + per-client channel (contract #5); ordering test |
| Clock drift trên phone | server gửi `remaining_ms` mỗi tick, client nội suy |
| Crash giữa game | write-through SQLite + auction_runtime; resume theo wall-clock |
| Leak truth_value / sealed bids | role projections + JSON assert tests |
| Hết thời gian present | M6 rehearsal checklist + chạy thử toàn bộ trước 1 ngày |

---

## 14. Decision log (đã chốt)

1. Không auth cho player; **admin capability token** (`ADMIN_KEY`, auto-generate nếu không set).
2. Truth value: **common**, ẩn khi đấu + hint range mỗi item, reveal tại SETTLED.
3. Budget: `−price +value`; bid ≤ budget hiện tại.
4. 4 loại: English + Dutch trước, sealed first-price + Vickrey sau.
5. Anti-snipe per item: `disabled | extend` (bỏ `none`/`hard` trùng nghĩa).
6. SQLite snapshot write-through + `auction_runtime`; resume theo wall-clock, deadline qua → settle khi boot.
7. cloudflared optional (`--tunnel`), LAN fallback (interface enumerate), hotspot chốt cuối. Không VPS.
8. 3 views: `/` player, `/admin` control, `/screen` projector.
9. Top 3 budget cuối; tie-break `total_value` → `joined_at`.
10. 1 item LIVE tại một thời điểm, admin điều khiển start/end/next.
11. Currency VND, start budget 1.000.000 mặc định (admin config).
12. App = 1 binary (React embed); cloudflared = optional external runtime dependency.
13. Role projections là architecture invariant — không serialize internal state ra network.
14. WS revision monotonic + role handshake + duplicate session supersede.
15. Sealed: effective bid trong `sealed_bids`, tie = earliest `submitted_at`; ràng buộc `start_price ≤ bid ≤ budget`.
16. Reset tách 2 mode: keep players / clear players; không bao giờ xóa item config ngoài ý muốn.
17. Broadcast = non-blocking enqueue per-client channel **trong lock**; writer goroutine/client; channel đầy → drop + resync. Loại hẳn class bug reorder.
18. Token player + admin key dùng `crypto/rand`.
19. Anti-snipe `extend` có optional `max_extensions`; hết quota → hard stop tại deadline hiện tại.

**Open items (quyết khi build, không blocking)**
- Item image: dùng URL + preset icon, **không** upload file.
- Kick player ở admin: thêm nếu kịp (nice-to-have).
- `SCREEN_KEY` optional: mặc định screen public.
- `JOIN_CODE` optional: shared code khi join, nếu muốn chặn outsider qua tunnel URL.
