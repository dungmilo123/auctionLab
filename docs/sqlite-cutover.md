# Offline SQLite-v4 cutover and rollback

Only a stopped, consistent schema-v4 source is supported. Do not run these commands against an unprepared live file. Preserve the prior compatible binary, a verified snapshot, and the operator image directory separately. Catalog media references are paths, not embedded database blobs; inventory and retain those files.

1. Stop the prior writer. Use SQLite's backup facility after a completed checkpoint to prepare a standalone snapshot; never copy only the main file while a writer is active or discard sidecars containing commits. For example, using the SQLite CLI on the stopped source:

   ```sh
   sqlite3 /path/to/stopped-working-copy.db 'PRAGMA wal_checkpoint(TRUNCATE);'
   sqlite3 /path/to/stopped-working-copy.db ".backup '/safe/location/source-v4.snapshot.db'"
   sha256sum /safe/location/source-v4.snapshot.db
   ```

   Close all SQLite connections before import. The importer rejects snapshots with `-wal`, `-shm`, or `-journal` sidecars; create a standalone consistent backup rather than deleting sidecars. It opens read-only, immutable access only after this standalone preflight, and holds a read transaction. It checks source bytes before/after, version, integrity, foreign keys, representable exact integer/text/null values and identity range. It neither repairs nor upgrades a source. For older versions, first upgrade a **separate working copy** using the prior compatible binary, stop/checkpoint it, then make the snapshot. Future versions are refused.

2. Configure a PostgreSQL 18 app role and **unused** target schema as described in [PostgreSQL operations](postgresql.md). Do not start the normal app or seed first: bootstrap/key rows make the target occupied. Do not set `DB_PATH`. Build the new binary with `make build` after preserving the old binary.

   ```sh
   export DATABASE_SCHEMA=auctionlab_cutover
   ./bin/auctionlab --import-sqlite /safe/location/source-v4.snapshot.db
   ./bin/auctionlab --import-sqlite /safe/location/source-v4.snapshot.db --verify-only
   ```

   `DATABASE_URL` comes from private environment configuration, never a URL argument. Import takes exclusive writer ownership and applies PostgreSQL DDL, then inserts all eight legacy tables, compares canonical typed counts/SHA-256 digests, restarts bid/event identities above both row maxima and SQLite sequence history, and writes a completion marker in one transaction. On failure all data/sequence/marker changes roll back; empty migrated DDL/categories may remain. Existing IDs, nickname keys, tokens/key, raw JSON whitespace, nulls, amounts, revisions, times and sealed bids are preserved without game bootstrap or settlement. An identical import reports `already_imported` without overwriting or claiming a new verification. Different sources and occupied targets are refused.

   Verify-only opens both databases read-only, takes no writer lease, does not migrate/bootstrap/seed, and compares actual rows plus identity readiness even if a marker exists. Its report contains table counts/digests and a logical source fingerprint, never row values, credentials or secrets. Source version 4 is independent of PostgreSQL ledger version 1. A drifted target fails verification. Keep both sides quiescent and use the same fixed snapshot.

3. Inspect the credential-free verification report and source checksum, then start the new app without maintenance flags. Without an `ADMIN_KEY` override, the imported persisted capability remains authoritative. Existing legacy deadlines resume using actual wall-clock time and may settle immediately; compare imported rows **before** that first normal boot. Such expected post-boot settlement is separate from migration equality. Verify readiness and admin access. A competing app/seed/import writer for the same PostgreSQL schema is refused.

4. Keep the snapshot, prior binary, image inventory and PostgreSQL backup. Use `pg_dump`/`pg_restore` with private libpq connection configuration (`PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD` or a service file), not credentials in report text. A schema backup/restore rehearsal can restore into an isolated disposable database containing that schema:

   ```sh
   pg_dump --dbname=auctionlab --schema=auctionlab_cutover --format=custom --file=auctionlab-cutover.dump
   createdb auctionlab_restore_rehearsal
   pg_restore --dbname=auctionlab_restore_rehearsal --no-owner --exit-on-error auctionlab-cutover.dump
   ```

   Compare restored counts/digests and normal app behavior using a dedicated role/URL for the disposable restore target. Drop only resources you created for rehearsal. Preserve operational backups according to your retention policy.

Before PostgreSQL accepts new app writes, rollback means stop the new writer and open a working copy of the preserved SQLite snapshot using the prior compatible binary and its old `DB_PATH` configuration. Keep the preserved snapshot itself untouched. After cutover writes, first preserve a PostgreSQL backup, disclose which new writes will be absent from the SQLite snapshot, and make the rollback choice explicitly. There is **no automatic reverse synchronization or lossless post-write rollback**. Never run old and new active writers for the same demo.
