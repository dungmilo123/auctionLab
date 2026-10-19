# Local website assets

Acquired 2026-10-19. Runtime requests stay on the app origin.

## Fonts

- `fonts/noto-sans-{latin,vietnamese}-{400,600}-normal.woff2`: @fontsource/noto-sans **5.3.0**, upstream Google Fonts **v42**, metadata lastModified 2025-09-11.
- `fonts/noto-serif-{latin,vietnamese}-{400,600}-normal.woff2`: @fontsource/noto-serif **5.3.0**, upstream Google Fonts **v33**, metadata lastModified 2025-09-06.
- Exact download pattern: `https://cdn.jsdelivr.net/npm/@fontsource/noto-{sans,serif}@5.3.0/files/noto-{sans,serif}-{latin,vietnamese}-{400,600}-normal.woff2`.
- Metadata and LICENSE fetched from the same pinned package root; copies are alongside the fonts as `noto-*-metadata.json` and `noto-*-OFL.txt`.
- Upstream: https://github.com/google/fonts and https://github.com/notofonts/latin-greek-cyrillic. Copyright 2022 The Noto Project Authors. **SIL Open Font License 1.1**; see the unmodified license files for attribution and reserved-name terms.
- CSS declares separate Latin/Vietnamese ranges, normal 400/600, `font-display: swap`. Fallbacks: Georgia/serif and system-ui/sans-serif. Fontkit verified Latin + Vietnamese coverage across all four family/weight pairs, including Vietnamese vowels with every tone and Đ/đ. The acceptance record includes Vietnamese rendered checks.

## Illustrations

`illustrations/camera.svg`, `illustrations/clock.svg`, `illustrations/vinyl.svg`, `web/public/site-assets/image-unavailable.svg`, and `web/public/favicon.svg` are original project-created vector drawings, authored for AuctionLab on 2026-10-19. No external source imagery, trademarks, or third-party asset license. Distributed as part of this project under its terms; no claim of public-domain licensing. The three illustrations are imports fingerprinted by Vite. The shared fallback uses `/site-assets/image-unavailable.svg` and must not be given immutable caching. Illustrations are preview content, not authenticated objects or published catalog records.

## File integrity

| File | SHA-256 |
|---|---|
| `noto-sans-latin-400-normal.woff2` | `09aee8065d25508f23a4c3d92cd777ac869c52d93fd868a88f025d888a7937d6` |
| `noto-sans-latin-600-normal.woff2` | `79e274470d1c5a0118eb325e2ea6f2eb2a449336d7fde1a4f20a2f32fe1119ed` |
| `noto-sans-vietnamese-400-normal.woff2` | `0f0c8a4858f1f5b2701331cf0a471ee00824ed92d33ecb21c9248ded975e297f` |
| `noto-sans-vietnamese-600-normal.woff2` | `810c4f33639e2e7df436de2154aa921845dd56f5bccc301215bcd6791804b44a` |
| `noto-serif-latin-400-normal.woff2` | `4c0cbe3eec50d260754d681c17ee2af49a43d7fd93ce42877f665fcb1a889b87` |
| `noto-serif-latin-600-normal.woff2` | `abf0abc765331d7a1bbe6eb3603cf86be1cf3d1edbcf911cc2d52f78998c02d9` |
| `noto-serif-vietnamese-400-normal.woff2` | `207dac0723db2e3832fb864c23a416dd953df68b27289acaec79ed94cd37b7ee` |
| `noto-serif-vietnamese-600-normal.woff2` | `b75fe581f1a208d1e38c143c626aa415b55721b01795b0eaafdb8c946c79b143` |
