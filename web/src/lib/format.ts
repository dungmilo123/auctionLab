/**
 * VND amount rendering.
 *
 * Amounts are deliberately NOT localized: the dot-grouped `1.000.000` renders
 * identically in EN and VI, so the app has exactly one number format to apply
 * everywhere (FRMT-01 in Phase 8 owns the full formatting surface — this is the
 * one format Phase 2 ships).
 */
export function formatVND(amount: number): string {
  // Grouping is done by hand rather than with Intl.NumberFormat: the venue may
  // be offline and a locale database is not worth loading for a dot every three
  // digits. `\B` keeps the match off the first digit, and the lookahead only
  // matches a position followed by complete groups of three.
  const rounded = Math.trunc(amount)
  const sign = rounded < 0 ? '-' : ''
  const digits = Math.abs(rounded).toString()
  return sign + digits.replace(/\B(?=(\d{3})+(?!\d))/g, '.')
}

/**
 * The Players list's time-of-day value (D-80) — RED DECLARATION.
 *
 * A relative age would need a ticking clock and an absolute date is noise for a
 * one-hour show; the presenter needs "did they join before or after the break"
 * precision, not more.
 */
export function formatTimeOfDay(_ms: number): string {
  return ''
}
