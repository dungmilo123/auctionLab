/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { LeaderEntry, PlayerMe } from '@/ws/protocol'
import {
  SCREEN_SCALE_CLASS,
  isDutchAuction,
  isEnglishAuction,
  isSealedAuction,
  joinUrlInvitable,
  playerRank,
  podiumEntries,
} from './screen'

// This is the one test in the suite that reads a shipped file instead of
// importing a module: `tsconfig.app.json` scopes the app's type environment to
// `vite/client` (browser code must not see Node globals), so the reference
// directive above is what lets this file — and only its own imports — reach
// `node:fs`. Vitest itself never needed it: `vitest run` does not type-check.
// (The alternative, `../index.css?raw`, is stubbed to an empty string by
// Vitest's CSS handling, which is why the file is read from disk.)

// The projector's pure rules (D-86/D-88/D-90), pinned in Vitest's node
// environment: there is no DOM harness in this phase, so `/screen` is tested
// where its decisions live — in selectors — plus the one stylesheet contract
// D-90 declares, which a test can read from disk.
//
// Every assertion below is written against 04-UI-SPEC.md §Screen Route &
// Interaction Contract and §Typography, not against the module.

/** `index.css` with its comments removed, so prose about a rule is not a rule. */
function stylesheet(): string {
  return readFileSync(fileURLToPath(new URL('../index.css', import.meta.url)), 'utf8').replace(
    /\/\*[\s\S]*?\*\//g,
    '',
  )
}

/**
 * Every rule written at brace depth 0 — i.e. outside any `@layer`, `@theme` or
 * `@media` block. Depth is what makes "unlayered" checkable: §Spacing requires
 * the two projector rules to outrank the utility layer, and a rule that moved
 * inside `@layer base` would silently stop doing that.
 */
function topLevelRules(css: string): { selector: string; body: string }[] {
  const rules: { selector: string; body: string }[] = []
  let depth = 0
  let start = 0
  for (let i = 0; i < css.length; i += 1) {
    const char = css[i]
    if (char === '{') {
      if (depth === 0) {
        const selector = css.slice(start, i).replace(/\s+/g, ' ').trim()
        const close = css.indexOf('}', i)
        rules.push({
          selector,
          body: (close === -1 ? '' : css.slice(i + 1, close)).replace(/\s+/g, ' ').trim(),
        })
      }
      depth += 1
    } else if (char === '}') {
      depth -= 1
      if (depth === 0) start = i + 1
    }
  }
  return rules
}

describe('the projector base (D-90)', () => {
  it('adds exactly two unlayered rules, both scoped to the route class', () => {
    const rules = topLevelRules(stylesheet()).filter((rule) =>
      rule.selector.includes(SCREEN_SCALE_CLASS),
    )
    expect(rules.map((rule) => rule.selector)).toEqual([
      `html.${SCREEN_SCALE_CLASS}`,
      `html.${SCREEN_SCALE_CLASS} .border-2`,
    ])
  })

  it('declares the projector base as the clamp and the route outline as the xs step', () => {
    const [base, outline] = topLevelRules(stylesheet()).filter((rule) =>
      rule.selector.includes(SCREEN_SCALE_CLASS),
    )
    expect(base?.body).toBe('font-size: clamp(20px, 4.2vmin, 48px);')
    expect(outline?.body).toBe('border-width: 4px;')
  })
})

describe('the lobby QR gate (D-86)', () => {
  it('invites when the selected URL is non-empty and http(s)', () => {
    expect(joinUrlInvitable('http://192.168.1.10:8080/')).toBe(true)
    expect(joinUrlInvitable('https://auction.trycloudflare.com')).toBe(true)
  })

  it('refuses every shape the server itself refuses, so the plate shows screen.noJoinUrl', () => {
    // The exact rules of `internal/game/config.go`'s validateJoinURL: no
    // trimming, a case-sensitive scheme prefix, nothing but http(s).
    for (const url of ['', '   ', '192.168.1.10:8080/', 'ftp://192.168.1.10/', 'HTTP://host/']) {
      expect(joinUrlInvitable(url)).toBe(false)
    }
  })
})

describe('the Body 2 type gates (D-88)', () => {
  it('renders the sealed progress line for the two sealed types only', () => {
    expect(isSealedAuction('sealed_first')).toBe(true)
    expect(isSealedAuction('vickrey')).toBe(true)
    for (const type of ['english', 'dutch', '']) {
      expect(isSealedAuction(type)).toBe(false)
    }
  })

  it('renders the newest-first bid feed for an English run only', () => {
    expect(isEnglishAuction('english')).toBe(true)
    for (const type of ['dutch', 'sealed_first', 'vickrey', '']) {
      expect(isEnglishAuction(type)).toBe(false)
    }
  })

  it('branches the falling-price arm for a Dutch run only (D-105)', () => {
    expect(isDutchAuction('dutch')).toBe(true)
    for (const type of ['english', 'sealed_first', 'vickrey', '']) {
      expect(isDutchAuction(type)).toBe(false)
    }
  })
})

/** One standing row, in the shape `readLeaderboard` hands the UI. */
function row(rank: number, nickname: string): LeaderEntry {
  return { rank, nickname, totalValue: 0, budget: 1_000_000 }
}

/** The player's own row, with the one field each case varies. */
function me(rank: number): PlayerMe {
  return {
    id: 'p1',
    nickname: 'An',
    budget: 1_000_000,
    totalValue: 0,
    connected: true,
    lastBid: null,
    isTopBidder: false,
    rank,
  }
}

describe('the podium (D-89)', () => {
  it("takes the standings' own prefix, in the server's order and never re-sorted", () => {
    // Deliberately out of rank-number order: the server owns ECON-05's order
    // (budget → total_value → joined_at → player id), so a client sort by the
    // `rank` NUMBER would put these three back in a different order than the
    // room saw the scoreboard.
    const standings = [row(3, 'Chi'), row(1, 'An'), row(2, 'Bình')]
    expect(podiumEntries(standings).map((entry) => entry.nickname)).toEqual(['Chi', 'An', 'Bình'])
  })

  it('caps the celebration at three entries and copies the list it was given', () => {
    const standings = [row(1, 'An'), row(2, 'Bình'), row(3, 'Chi'), row(4, 'Dung')]
    const podium = podiumEntries(standings)
    expect(podium).toHaveLength(3)
    expect(podium.map((entry) => entry.nickname)).toEqual(['An', 'Bình', 'Chi'])
    expect(podium).not.toBe(standings)
  })

  it('renders one or two blocks with no empty frame, and nothing at all for zero players', () => {
    expect(podiumEntries([row(1, 'An')])).toHaveLength(1)
    expect(podiumEntries([row(1, 'An'), row(2, 'Bình')])).toHaveLength(2)
    expect(podiumEntries([])).toEqual([])
  })
})

describe("the player's own rank (D-89/ECON-05)", () => {
  it('reads the server-computed rank with the standings it was given', () => {
    expect(playerRank(me(2), [row(1, 'An'), row(2, 'Bình'), row(3, 'Chi')])).toEqual({
      rank: 2,
      total: 3,
    })
  })

  it('omits the block entirely when the server sent no standing', () => {
    // 0 is the wire's "no standing" (04-06's readRank), and a null `me` is the
    // screen role's shape: neither may render a blank rank scaffold.
    expect(playerRank(me(0), [row(1, 'An')])).toBeNull()
    expect(playerRank(null, [row(1, 'An')])).toBeNull()
  })

  it('counts zero standings as zero rather than inventing a place', () => {
    expect(playerRank(me(0), [])).toBeNull()
  })
})

// T-04-14's second half, pinned where it can be. `openScreenSocket`'s handle has
// no `send` at all (client.test.ts asserts that half at runtime), but the route
// itself has no DOM harness in this phase — so "the projector renders no
// control" is asserted against the source the bundler ships. A future screen
// control therefore has to delete this test on purpose, in a visible diff.
const screenViewSource = readFileSync(
  fileURLToPath(new URL('../routes/screen/ScreenView.tsx', import.meta.url)),
  'utf8',
)

describe('the read-only guarantee (D-87/T-04-14)', () => {
  it('mounts no control, binds no handler and sends nothing', () => {
    expect(screenViewSource).not.toMatch(/<Button|<button|on(Click|Change|Submit|KeyDown|Input)/)
    expect(screenViewSource).not.toMatch(/\.send\(/)
  })
})
