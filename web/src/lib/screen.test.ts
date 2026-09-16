import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { SCREEN_SCALE_CLASS, isEnglishAuction, isSealedAuction, joinUrlInvitable } from './screen'

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
})
