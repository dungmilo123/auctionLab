/**
 * USD amount rendering (D-02).
 *
 * The wire and the database carry whole integers — the game's own accounting
 * units — and every place they are shown to a human goes through this function,
 * so "how much is this" has exactly one answer on every surface. The rendered
 * value is SELF-CONTAINED: the `$`, the comma grouping and the two decimals are
 * all in the string, so a caption or a sentence never has to append a currency
 * symbol of its own (and can never append a stale one).
 *
 * Grouping is done by hand rather than with `Intl.NumberFormat`: the venue may
 * be fully offline and a locale database is not worth loading for two commas.
 * The sign goes before the `$` (`-$1,234.00`), which is how a negative amount is
 * written everywhere else in the world.
 */
export function formatUSD(amount: number): string {
  // `trunc` (never `round`) keeps the sign and the magnitude the amount already
  // had: -1234.56 is $-1,234, not -$1,235. Auction amounts are whole numbers, so
  // this only ever decides the edge the wire can never produce.
  const rounded = Math.trunc(amount)
  const sign = rounded < 0 ? '-' : ''
  const digits = Math.abs(rounded).toString()
  // `\B` keeps the match off the first digit, and the lookahead only matches a
  // position followed by complete groups of three.
  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return `${sign}$${grouped}.00`
}

/**
 * The Players list's time-of-day value (D-80): zero-padded 24-hour LOCAL time,
 * `HH:MM`.
 *
 * Local via `getHours`/`getMinutes` — no `Intl` and no locale database (the
 * venue may be offline), and no date, because the presenter needs "did they join
 * before or after the break" precision rather than a calendar. Its test pins
 * `TZ=UTC` in its own file, which is what makes the literal expectations about
 * this function instead of about the machine that ran the suite.
 */
export function formatTimeOfDay(ms: number): string {
  const at = new Date(ms)
  const hours = String(at.getHours()).padStart(2, '0')
  const minutes = String(at.getMinutes()).padStart(2, '0')
  return `${hours}:${minutes}`
}
