# Visual checks

Calculated from sRGB relative luminance on 2026-10-19. Browser evidence is recorded separately after integration.

| Pair | Ratio | Threshold | Result |
|---|---|---|---|
| foreground / ivory | 14.23:1 | 4.5:1 | Pass |
| muted / ivory | 6.01:1 | 4.5:1 | Pass |
| primary / ivory | 8.57:1 | 4.5:1 | Pass |
| hover / ivory | 10.69:1 | 4.5:1 | Pass |
| olive / ivory | 5.29:1 | 4.5:1 | Pass |
| error / ivory | 7.25:1 | 4.5:1 | Pass |
| warning / ivory | 6.46:1 | 4.5:1 | Pass |
| border / ivory | 3.60:1 | 3:1 | Pass |
| brass / ivory | 3.15:1 | 3:1 | Pass |
| foreground / white | 16.01:1 | 4.5:1 | Pass |
| muted / white | 6.77:1 | 4.5:1 | Pass |
| primary / white | 9.65:1 | 4.5:1 | Pass |
| hover / white | 12.03:1 | 4.5:1 | Pass |
| olive / white | 5.95:1 | 4.5:1 | Pass |
| error / white | 8.16:1 | 4.5:1 | Pass |
| warning / white | 7.26:1 | 4.5:1 | Pass |
| border / white | 4.05:1 | 3:1 | Pass |
| brass / white | 3.54:1 | 3:1 | Pass |

Primary and hover fills use ivory text, so their inverse pairs above also verify filled buttons. These pairs cover default, hover, focus, selected navigation/language, error, status, input placeholder/help, disabled and fallback text. Focus uses primary against ivory/white, separated from the control by a 2px ivory gap. Control borders use border against ivory/white. Brass is decorative only; divider is never the required control boundary. No enabled text opacity or tinted badge backgrounds.

## Rendered browser measurements

Chromium 156.0.8078.4; run 2026-10-19T07:39:04.208Z. Values below come from computed styles on actual rendered components, walking ancestors for opaque backgrounds. Every measured normal-text pair passes 4.5:1. Input default/invalid and outlined-button boundaries pass 3:1. Primary ivory/burgundy default and ivory/darker-burgundy hover fills plus focus color were checked against the calculated pairs above. Disabled button text remains muted at full opacity.

| Website selector | Foreground | Background | Ratio |
|---|---|---|---|
| `h1` | rgb(36, 33, 30) | rgb(246, 241, 232) | 14.23:1 |
| `.muted` | rgb(98, 90, 82) | rgb(246, 241, 232) | 6.01:1 |
| `.preview-notice` | rgb(98, 90, 82) | rgb(246, 241, 232) | 6.01:1 |
| `.status-upcoming` | rgb(113, 81, 22) | rgb(246, 241, 232) | 6.46:1 |
| `.status-live` | rgb(114, 47, 55) | rgb(246, 241, 232) | 8.57:1 |
| `.status-ended` | rgb(98, 90, 82) | rgb(246, 241, 232) | 6.01:1 |
| `.status-sold` | rgb(98, 102, 77) | rgb(246, 241, 232) | 5.29:1 |
| `.status-unavailable` | rgb(98, 90, 82) | rgb(246, 241, 232) | 6.01:1 |
| `.field-error` | rgb(141, 46, 53) | rgb(255, 255, 255) | 8.16:1 |
| `#search-example` | rgb(36, 33, 30) | rgb(255, 255, 255) | 16.01:1 |
| `#invalid-example` | rgb(36, 33, 30) | rgb(255, 255, 255) | 16.01:1 |
| `button:disabled` | rgb(98, 90, 82) | rgb(246, 241, 232) | 6.01:1 |
| `.language-control [aria-pressed=true]` | rgb(114, 47, 55) | rgb(255, 255, 255) | 9.65:1 |
| `.site-button-outline` | rgb(114, 47, 55) | rgb(246, 241, 232) | 8.57:1 |

| Bilingual gate selector | Foreground | Background | Ratio |
|---|---|---|---|
| `h1` | rgb(36, 33, 30) | rgb(255, 255, 255) | 16.01:1 |
| `p` | rgb(36, 33, 30) | rgb(255, 255, 255) | 16.01:1 |
| `.vi` | rgb(98, 90, 82) | rgb(255, 255, 255) | 6.77:1 |
| `.error` | rgb(141, 46, 53) | rgb(255, 255, 255) | 8.16:1 |
| `.error .vi` | rgb(141, 46, 53) | rgb(255, 255, 255) | 8.16:1 |
| `label` | rgb(36, 33, 30) | rgb(255, 255, 255) | 16.01:1 |
| `input` | rgb(36, 33, 30) | rgb(255, 255, 255) | 16.01:1 |
| `button` | rgb(246, 241, 232) | rgb(114, 47, 55) | 8.57:1 |
| `button .vi` | rgb(246, 241, 232) | rgb(114, 47, 55) | 8.57:1 |

## Layout and visual inspection

- Automated: 80 website layout cases (home, lots, account/watchlist, admin/lots, 404 × EN/VI × 320/375/768/1280 CSS px × 100/200 percent text size); 8 always-bilingual gate cases at the same widths/text sizes. All 88 have one h1, no page overflow, no elements extending past the viewport, and no interaction target under 44px. Text-only 200% scaling is exercised by setting the root font size to 200%; this is headless CSS text enlargement, not a physical mobile browser toolbar test.
- Automated: reduced-motion preference makes button transitions 0s in all website cases; gate suppression uses its standalone reduced-motion rule. 150 sequential Tab focus checks across every representative website layout in both languages retain visible 2px focus. Skip-link Enter moves focus to main; native link activation/new-tab and gate field/button traversal pass.
- Visual inspection: reviewed desktop home/admin, narrow Vietnamese account/404, 375px bilingual gate, 200% Vietnamese lot listing, local-image fallback, and combined font/image failure screenshots. Text wraps, controls remain in order, long vinyl copy fits, navigation stays available, disabled copy is readable, and no old orange fills/hard shadows appear.
- Actual Vietnamese glyph rendering: CDP reports custom Noto Serif for h1 and Noto Sans for body. All eight Latin/Vietnamese subset/weight faces load when requested with Vietnamese sample text. With WOFF2 requests blocked, CDP reports non-custom system fonts with readable copy. Fontkit 2.0.4 additionally verified both family/weight pairs cover the Vietnamese sample including all tone forms and Đ/đ.
- Image failure: a failed primary illustration loads `/site-assets/image-unavailable.svg` with a translated caption and preserved image dimensions. If the fallback also fails, the image is hidden while its fixed-ratio region, caption, title and detail link remain usable.

Representative PNGs are in `screenshots/`; machine observations are in `browser-results.json`. No unresolved visual failure was observed in this matrix. Browser/device and assistive-technology coverage is limited to the recorded Chromium checks, keyboard interactions, semantic assertions and screenshots; no physical-device or screen-reader trial is claimed.
