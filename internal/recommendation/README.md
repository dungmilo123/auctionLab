# Rule-based lot recommendations

`rules-v1` is a deterministic baseline, not trained ML or evidence of lift.
Eligible published lot/parent pairs have future planned close and no committed
terminal result. Upcoming lots qualify; source and current owner's saved lots
do not. Up to six public projections are returned, with a bounded reason code.
Source may be a historical published lot; missing/draft sources share 404.

Score category equality +4 and nonempty trimmed/lowercased authored era equality
+2. No numeric period inference is made. Source features take precedence over
watchlist features. Without source, each saved-published category/era is a set
feature counted once; no weights accumulate from repeated saves. Sort score
 descending, planned close ascending, slug C-order. Zero-score fillers get
`closing_soon`. Other reasons: `category_and_era`, `category`, `era`,
`watchlist_category_and_era`, `watchlist_category`, `watchlist_era`.

`GET /api/catalog/recommendations[?source=slug]` is cookie-independent.
`GET /api/account/recommendations[?source=slug]` uses existing account dispatch;
current session and owner preferences are read in the same read-only snapshot.
No client account ID, private preference sources, scores or histories are
returned. Both are no-store and return `{data:[{lot,reason}],meta:{server_time,
strategy:"rules-v1"}}`. Unknown/duplicate/empty/invalid query fields are 422,
private access 401, missing/draft source 404, storage failure 503. Successful
empty catalogs return `[]`. GETs initialize no execution or business state.
