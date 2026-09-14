#!/usr/bin/env node
// Zero-dependency WebSocket round-trip probe.
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
// though the proxy is fine.
//
// Exit 0 only after a well-formed echo envelope comes back:
//   {"type":"echo","ts":<server_ms>,"data":<payload verbatim>}
// Exit 1 (naming every attempted URL) when no candidate completes a round-trip.

const candidates = process.argv[2]
  ? [process.argv[2]]
  : [
      process.env.WS_URL,
      'ws://localhost:5173/ws',
      'ws://127.0.0.1:5173/ws',
      'ws://[::1]:5173/ws',
    ].filter(Boolean);

const payload = { probe: 'ws-probe', at: Date.now() };

const DIAL_TIMEOUT_MS = 5000;

function deepEqual(a, b) {
  if (a === b) return true;
  if (typeof a !== typeof b) return false;
  if (a === null || b === null || typeof a !== 'object') return false;
  const ak = Object.keys(a);
  const bk = Object.keys(b);
  if (ak.length !== bk.length) return false;
  return ak.every((k) => Object.prototype.hasOwnProperty.call(b, k) && deepEqual(a[k], b[k]));
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
      try {
        ws.close();
      } catch {
        /* already dead */
      }
      finish(false, `timeout after ${DIAL_TIMEOUT_MS}ms waiting for an echo`);
    }, DIAL_TIMEOUT_MS);

    ws.addEventListener('open', () => {
      try {
        ws.send(JSON.stringify(payload));
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
      const problems = [];
      if (env.type !== 'echo') problems.push(`type is ${JSON.stringify(env.type)}, want "echo"`);
      if (typeof env.ts !== 'number' || !(env.ts > 0)) {
        problems.push(`ts is ${JSON.stringify(env.ts)}, want a number > 0`);
      }
      if ('seq' in env || 'revision' in env) {
        problems.push('envelope carries a seq/revision field (must not exist in Phase 1)');
      }
      if (!deepEqual(env.data, payload)) {
        problems.push(`data is ${JSON.stringify(env.data)}, want ${JSON.stringify(payload)} verbatim`);
      }
      if (problems.length > 0) {
        finish(false, problems.join('; '));
        return;
      }
      finish(true, `type=echo ts=${env.ts} data=${JSON.stringify(env.data)}`);
    });

    ws.addEventListener('error', () => {
      finish(false, 'connection error (is the server running?)');
    });

    ws.addEventListener('close', (event) => {
      finish(false, `socket closed before an echo arrived (code ${event.code})`);
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

console.error('ws-probe: FAIL — no candidate completed an echo round-trip:');
for (const line of failures) console.error(`  - ${line}`);
process.exit(1);
