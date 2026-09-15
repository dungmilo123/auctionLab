/**
 * VND amount rendering.
 *
 * Amounts are deliberately NOT localized: the dot-grouped `1.000.000` renders
 * identically in EN and VI, so the app has exactly one number format to apply
 * everywhere (FRMT-01 in Phase 8 owns the full formatting surface — this is the
 * one format Phase 2 ships).
 */
export function formatVND(amount: number): string {
  // RED sentinel: the surface is declared, the grouping rule is not — the
  // number is returned unformatted until the GREEN commit replaces this body.
  return String(amount)
}
