# Archive record

Archived on 2026-10-21 (Asia/Bangkok) at the user's explicit request to commit and archive after the implementation report disclosed the remaining acceptance gaps.

- Change: `add-order-history-and-demo-fulfillment`
- Schema: `spec-driven`; all four planning artifacts complete.
- Acceptance: **39/41 tasks verified**. Tasks **7.5 and 7.6 remain unchecked**. Actual 200% browser zoom, active reduced motion and the unmeasured portions of their full matrices remain unaccepted. Archiving does not close those gaps.
- Main specs: three created, seven updated; 18 added requirements and 13 modified requirements synchronized. Existing purposes, unmentioned requirements and surviving scenarios were preserved; all ten capabilities were compared against their deltas after sync (`verification/spec-sync.json`).
- Verification retained: `make test` (543 frontend tests plus Go and lint), `make test-race`, frontend-first `make build`, real PostgreSQL upgrades/rollback/isolation, Vite shipping, embedded pickup, response recovery, restart, and stopped-writer reset/replay evidence.
- Validation: strict change validation passes. Normal main-spec validation passes 16/16. Strict main-spec validation reports only requirement-length warnings in eight files (including existing and newly synced MODIFIED descriptions); it does not pass. The archive preserves those contracts instead of trimming them merely to suppress warnings. See `verification/validate-specs-after-sync*.log`.
- Migrations 1–4 and all earlier archives remain unchanged. No operational schema was reset, no archive gap was relabeled complete, and no push was requested.

The implementation and evidence are committed together with this archive. See `verification/REPORT.md` for the requirement-to-task matrix and measured limitations.
