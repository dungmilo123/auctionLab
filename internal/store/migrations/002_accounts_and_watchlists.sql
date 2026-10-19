CREATE TABLE commerce_accounts (
 id TEXT COLLATE "C" PRIMARY KEY,
 email TEXT COLLATE "C" NOT NULL UNIQUE,
 display_name TEXT NOT NULL,
 password_verifier TEXT NOT NULL,
 role TEXT NOT NULL DEFAULT 'bidder' CHECK (role IN ('bidder','admin')),
 demo BOOLEAN NOT NULL DEFAULT FALSE,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL
);
CREATE TABLE commerce_sessions (
 token_digest TEXT COLLATE "C" PRIMARY KEY,
 account_id TEXT COLLATE "C" NOT NULL REFERENCES commerce_accounts(id),
 created_at BIGINT NOT NULL,
 expires_at BIGINT NOT NULL CHECK (expires_at > created_at),
 revoked_at BIGINT
);
CREATE INDEX commerce_sessions_account ON commerce_sessions(account_id);
CREATE INDEX commerce_sessions_expiry ON commerce_sessions(expires_at);
CREATE TABLE commerce_watchlist (
 account_id TEXT COLLATE "C" NOT NULL REFERENCES commerce_accounts(id),
 lot_id TEXT NOT NULL REFERENCES catalog_lots(id),
 saved_at BIGINT NOT NULL,
 PRIMARY KEY(account_id,lot_id)
);
CREATE INDEX commerce_watchlist_newest ON commerce_watchlist(account_id,saved_at DESC,lot_id COLLATE "C");
CREATE TABLE commerce_account_seed_runs (
 version INTEGER PRIMARY KEY,
 completed_at BIGINT NOT NULL
);
