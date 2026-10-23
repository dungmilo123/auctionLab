# Change 9 evidence

Dedicated service: existing PostgreSQL 18 test container on loopback port 55437,
started for this run. A new `auctionlab_polish_test` database isolates this work
from operational databases. `fixture.go` requires `TEST_DATABASE_URL`, creates a
fresh random `auctionlab_demo_change9_*` schema through `SeedDemo`, and creates
shipping/pickup paid orders plus an unpaid win using the production repositories.
Two independent bidders are a and b; c has no purchases. Credentials and capability
are generated, written only to a mode-0600 temporary manifest and excluded here.
The test fixture alone uses explicit SQL to arrange published schedules; no
operational schema, existing publication or source SQLite file is touched.

Run `go run openspec/changes/archive/2026-10-23-polish-auction-website-experience/verification/fixture.go`
with the dedicated test URL. Serve its selected schema at 8080 and Vite at 5173.
Temporary manifest location is documented by the script; never commit it.

`baseline.cjs` captures 20 routes at 1280x900 and 375x900, English, plus the
unauthorized gate. The authenticated browser is bidder a with separate catalog
capability; no credentials or private input values are logged. Stored shipping
and pickup contacts are synthetic fixture data. `baseline-states.cjs` captures
signed-out forms and explicitly intercepted empty/long-title/missing-image
presentation fixtures. Intercepted responses are stress fixtures, not persisted
catalog content or evidence of server behavior. Normal before/after views use the
same stored records. `baseline*.json` provide route, state and measurement metadata.

Build measurements enumerate raw and gzip bytes under web/dist. Browser transfer
measurements use Resource Timing's encodedBodySize/transferSize. The baseline
network capture follows signed-out form visits and has a warm JS/font cache;
compare encoded bytes/file sizes rather than claiming cold transfer equality.
Final performance captures additionally measure fresh cold contexts.
The fixed local trace uses Chromium CDP: 150 ms latency, 200000 bytes/sec download,
100000 upload, 4x CPU slowdown, a native lot navigation and second gallery image
selection. Timings are local comparative observations, not a service SLA. Network
logs contain only methods and URL paths, never request bodies, cookies or headers.

After acceptance, the temporary Go writer and Vite processes were stopped. The
dedicated disposable test database remains available for an explicit rerun; no
operational schema was used or removed.
