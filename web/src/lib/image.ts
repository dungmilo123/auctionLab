/**
 * D-01's ONE place where a stored item image value becomes a URL.
 *
 * The presenter types a value into the admin editor's image field, and it is
 * stored verbatim by the server. This function decides what the browser should
 * actually request, and it is deliberately a whitelist:
 *
 *   - `http:`, `https:` and `data:` values are EXTERNAL and pass through
 *     unchanged — the presenter typed a real URL, and rewriting it would be
 *     the app second-guessing an explicit choice (the D-86 rule: the server
 *     never checks a URL it cannot see, and neither does this).
 *   - a leading slash is already root-relative and passes through unchanged
 *     (`/assets/lot.png`, or a fully-formed `/images/lot.png`).
 *   - EVERYTHING ELSE is a file name under the server's own image route, with
 *     a redundant `./` or `images/` prefix stripped, because that is how a
 *     presenter naturally writes one (`vase.png`, `./vase.png`,
 *     `images/vase.png`).
 *
 * The whitelist is also the safety property: an unsupported scheme
 * (`javascript:…`) is not a URL to this function — it becomes an ordinary path
 * under /images/, which can only ever 404. The app never invents a scheme.
 *
 * An empty (or blank) value answers `''`: the caller renders no image at all,
 * which is the only correct rendering for "not configured".
 */
export function itemImageSrc(raw: string): string {
  const value = raw.trim()
  if (value === '') return ''

  if (value.startsWith('http://') || value.startsWith('https://') || value.startsWith('data:')) {
    return value
  }
  if (value.startsWith('/')) return value

  let name = value
  let stripped = true
  while (stripped) {
    stripped = false
    for (const prefix of ['./', 'images/']) {
      if (name.startsWith(prefix)) {
        name = name.slice(prefix.length)
        stripped = true
      }
    }
  }
  return `/images/${name}`
}
