
CREATE TABLE IF NOT EXISTS players (
  id           TEXT PRIMARY KEY,
  nickname     TEXT NOT NULL UNIQUE,
  nickname_key TEXT NOT NULL,
  token        TEXT NOT NULL UNIQUE,
  budget       INTEGER NOT NULL,
  total_value  INTEGER NOT NULL DEFAULT 0,
  joined_at    INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_players_nickname_key ON players(nickname_key);

CREATE TABLE IF NOT EXISTS items (
  id                    TEXT PRIMARY KEY,
  order_index           INTEGER NOT NULL,
  name                  TEXT NOT NULL,
  description           TEXT,
  image_url             TEXT,
  auction_type          TEXT NOT NULL,
  truth_value           INTEGER NOT NULL,
  hint_min              INTEGER,
  hint_max              INTEGER,
  start_price           INTEGER NOT NULL,
  min_increment         INTEGER,
  floor_price           INTEGER,
  decrement_step        INTEGER,
  decrement_interval_ms INTEGER,
  duration_ms           INTEGER,
  anti_snipe_mode       TEXT DEFAULT 'disabled',
  anti_snipe_window_ms  INTEGER,
  max_extensions        INTEGER,
  status                TEXT NOT NULL DEFAULT 'pending'
);

CREATE TABLE IF NOT EXISTS sealed_bids (
  item_id      TEXT NOT NULL REFERENCES items(id),
  player_id    TEXT NOT NULL REFERENCES players(id),
  amount       INTEGER NOT NULL,
  submitted_at INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  PRIMARY KEY (item_id, player_id)
);

CREATE TABLE IF NOT EXISTS bids (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id    TEXT NOT NULL REFERENCES items(id),
  player_id  TEXT NOT NULL REFERENCES players(id),
  amount     INTEGER NOT NULL,
  kind       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS auction_runtime (
  item_id         TEXT PRIMARY KEY REFERENCES items(id),
  started_at      INTEGER NOT NULL,
  deadline_at     INTEGER,
  settled_at      INTEGER,
  revision        INTEGER NOT NULL,
  extensions_used INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS game (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  phase           TEXT NOT NULL,
  current_item_id TEXT REFERENCES items(id),
  revision        INTEGER NOT NULL,
  config_json     TEXT NOT NULL,
  updated_at      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  type         TEXT NOT NULL,
  payload_json TEXT,
  created_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS server_config (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
PRAGMA user_version=4;
