#!/usr/bin/env node
// Zero-dependency WebSocket protocol probe: joins as a player and validates
// D-23's handshake on a live /ws.
//
// Node >= 22 ships a global `WebSocket`, so this script needs no package.json,
// no node_modules and no install step — it runs from a clean clone.
//
// Usage:
//   node scripts/ws-probe.mjs [ws-url]
//   WS_URL=ws://host:port/ws node scripts/ws-probe.mjs
//
// With no argument (and no WS_URL) it tries a candidate list, because Vite's
// default `server.host` binds [::1] only on some machines and a single
// `ws://localhost:5173/ws` dial then fails with a bare TypeError/1006 even
// though the proxy is fine. That candidate list is also why this probe still
// exists beside `make loadtest`: it is the browser-shaped path (the Vite proxy,
// a real Origin), while the Go harness owns the seq/rev stream contract.
//
// Exit 0 only after the two handshake frames arrive and validate:
//   1. hello          — seq 1, data.role "player", a player id and a token
//   2. state.snapshot — seq 2, a self-sufficient payload (D-25)
// Every frame must carry the five frozen envelope fields (D-21). A server that
// still echoes arbitrary payloads — the retired Phase 1 handler — fails here,
// which is the point: an obsolete echo surface must not look healthy.
//
// Exit 1 (naming every attempted URL) when no candidate completed the handshake.

const candidates = process.argv[2]
  ? [process.argv[2]]
  : [
      process.env.WS_URL,
      'ws://localhost:5173/ws',
      'ws://127.0.0.1:5173/ws',
      'ws://[::1]:5173/ws',
    ].filter(Boolean);

// The five frozen envelope keys (D-21) — the contract, not the field names.
const ENVELOPE_KEYS = ['type', 'seq', 'rev', 'ts', 'data'];

// The probe's own player. The per-run suffix keeps a repeated run from colliding
// with the players the previous one left in the dev database (D-28's uniqueness
// is server-wide), and it stays well inside the 20-rune cap.
const nickname = `probe-${Date.now() % 1000000}`;
const joinFrame = JSON.stringify({ type: 'join', role: 'player', nickname });

const DIAL_TIMEOUT_MS = 5000;

function isRecord(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

// Validate one frame's envelope against the stream position it must occupy.
function envelopeProblems(env, wantSeq) {
  const problems = [];
  for (const key of ENVELOPE_KEYS) {
    if (!(key in env)) problems.push(`envelope has no ${key} field (D-21)`);
  }
  if (typeof env.type !== 'string' || env.type === '') {
    problems.push(`type is ${JSON.stringify(env.type)}, want a non-empty string`);
  }
  if (typeof env.seq !== 'number') {
    problems.push(`seq is ${JSON.stringify(env.seq)}, want a number`);
  } else if (env.seq !== wantSeq) {
    problems.push(`seq is ${env.seq}, want ${wantSeq} (D-21: 1 at hello, no gaps)`);
  }
  if (typeof env.rev !== 'number') {
    problems.push(`rev is ${JSON.stringify(env.rev)}, want a number (D-21)`);
  }
  if (typeof env.ts !== 'number' || !(env.ts > 0)) {
    problems.push(`ts is ${JSON.stringify(env.ts)}, want a number > 0 (D-21)`);
  }
  return problems;
}

// One attempt: resolve { ok, detail } — never rejects.
function attempt(url) {
  return new Promise((resolve) => {
    let settled = false;
    let timer = null;
    const finish = (ok, detail) => {
      if (settled) return;
      settled = true;
      if (timer) clearTimeout(timer);
      try {
        ws.close();
      } catch {
        /* already dead */
      }
      resolve({ ok, detail });
    };

    let ws;
    try {
      ws = new WebSocket(url);
    } catch (err) {
      finish(false, `constructor error: ${err && err.message ? err.message : err}`);
      return;
    }

    timer = setTimeout(() => {
      finish(false, `timeout after ${DIAL_TIMEOUT_MS}ms waiting for the handshake`);
    }, DIAL_TIMEOUT_MS);

    // The handshake the server owes is hello at seq 1 and then the role snapshot
    // at seq 2 (D-23), in that order on one connection.
    let step = 0;

    ws.addEventListener('open', () => {
      try {
        ws.send(joinFrame);
      } catch (err) {
        finish(false, `send failed: ${err && err.message ? err.message : err}`);
      }
    });

    ws.addEventListener('message', (event) => {
      const raw = typeof event.data === 'string' ? event.data : String(event.data);
      let env;
      try {
        env = JSON.parse(raw);
      } catch (err) {
        finish(false, `reply is not JSON: ${err && err.message ? err.message : err}`);
        return;
      }
      if (!isRecord(env)) {
        finish(false, `frame is not a JSON object: ${raw}`);
        return;
      }
      if (env.type === 'error') {
        const code = isRecord(env.data) ? env.data.code : undefined;
        finish(false, `server answered error ${JSON.stringify(code)} for the join`);
        return;
      }

      if (step === 0) {
        const problems = envelopeProblems(env, 1);
        if (env.type !== 'hello') {
          problems.push(`first frame type is ${JSON.stringify(env.type)}, want "hello" (D-23)`);
        }
        if (!isRecord(env.data)) {
          problems.push('hello has no data payload');
        } else {
          if (env.data.role !== 'player') {
            problems.push(`hello role is ${JSON.stringify(env.data.role)}, want "player"`);
          }
          if (typeof env.data.player !== 'string' || env.data.player === '') {
            problems.push('hello carries no player id (D-23)');
          }
          if (typeof env.data.token !== 'string' || env.data.token === '') {
            problems.push('hello carries no token (D-29)');
          }
        }
        if (problems.length > 0) {
          finish(false, problems.join('; '));
          return;
        }
        step = 1;
        return;
      }

      if (step === 1) {
        const problems = envelopeProblems(env, 2);
        if (env.type !== 'state.snapshot') {
          problems.push(`second frame type is ${JSON.stringify(env.type)}, want "state.snapshot" (D-23)`);
        }
        if (!isRecord(env.data)) {
          problems.push('state.snapshot has no data payload (D-25)');
        } else if (!isRecord(env.data.me) || typeof env.data.me.nickname !== 'string') {
          problems.push('state.snapshot carries no `me` row, so it is not self-sufficient (D-25)');
        }
        if (problems.length > 0) {
          finish(false, problems.join('; '));
          return;
        }
        step = 2;
        finish(true, `joined as ${nickname}: hello seq 1 → state.snapshot seq 2 (rev ${env.rev})`);
      }
      // A healthy connection keeps talking after the handshake; the two frames
      // above are the whole contract this probe owns.
    });

    ws.addEventListener('error', () => {
      finish(false, 'connection error (is the server running?)');
    });

    ws.addEventListener('close', (event) => {
      finish(false, `socket closed before the handshake completed (code ${event.code})`);
    });
  });
}

const failures = [];
for (const url of candidates) {
  const result = await attempt(url);
  if (result.ok) {
    console.log(`ws-probe: OK ${url} — ${result.detail}`);
    process.exit(0);
  }
  failures.push(`${url} → ${result.detail}`);
}

console.error('ws-probe: FAIL — no candidate completed the player handshake:');
for (const line of failures) console.error(`  - ${line}`);
process.exit(1);
