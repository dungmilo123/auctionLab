# Archive record — 2026-10-20

`enable-timed-auction-bidding` was archived at the user's explicit request after implementation reached **38/39 tasks**. All four planning artifacts were complete. The request to commit and archive followed disclosure of the remaining acceptance check.

## Outstanding acceptance

Task **8.4 remains unchecked**. Lot detail and `/account/bids` were observed at 320/375 CSS pixels and desktop in EN/VI, with keyboard interaction, but actual **200% browser zoom** and runtime **reduced-motion** verification remain unavailable/unverified. Viewport emulation is not substituted for zoom. Archiving does not mark this change fully accepted or alter Change 4's outstanding checks.

## Spec synchronization

Eight capability deltas were synchronized before moving the change:

- New `timed-auction-bidding`: ten requirements.
- New `auction-realtime`: seven requirements.
- `auction-discovery`: three modified requirements and one added requirement.
- `auction-event-publishing`: one modified requirement.
- `catalog-administration`: one modified requirement.
- `lot-watchlists`: two modified requirements.
- `postgresql-persistence`: three modified requirements and one added requirement.
- `website-navigation`: two modified requirements.

All incoming requirement/scenario content was compared with its corresponding main spec after the merge. Existing titles, Purpose sections, unrelated requirements and surviving scenarios were preserved. All **11 main specs passed standard validation**. Strict main-spec validation reports one warning for the modified catalog-administration requirement exceeding 500 characters. The approved requirement was not shortened or split solely to suppress the warning. The change package itself passed strict validation before archive.

## Preserved verification and commit scope

The [verification report](verification/REPORT.md), screenshots, measured results, source rehearsal helpers and task checkboxes are preserved from the implementation session. Its statements that the change was not yet archived/synced or that these local artifacts were not yet committed describe its pre-archive state. This archive action does not claim a new browser acceptance run.

Implementation verification passed all Go packages, uncached race checks, **29 frontend files / 525 tests**, lint, frontend build and the embedded no-CGO build. The local competition ended at **16007 USD cents**, and measured bid/result visibility was below one second. Restart, accepted-request replay, privacy and account switching evidence remain in the report.

Following the repository's recent archive commits, this archive/evidence and all eight synchronized main specs are explicitly included in the implementation commit despite the general OpenSpec ignore rule. The ignore rule remains unchanged. No credentials or test-environment files are included. `AGENTS.md`, migrations 1–2 and earlier archived evidence remain unchanged. Checkout, payments and orders remain unavailable.
