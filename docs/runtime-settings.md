# Runtime settings operator control

CodeGym stores registered runtime settings in the `runtime_settings` table,
scoped by the caller's personal workspace. The backend reads these values at
request time through a 10-second in-process cache, so a change takes effect on
each API replica within seconds without a redeploy.

There is currently no administrator role. Consequently, the authenticated
`GET` and `PUT /api/v1/settings/{key}` endpoints are an operator control only
for the caller's own workspace. They are not a platform-wide admin API, and no
settings UI is exposed.

The first registered key is `execution.hedge_count`. Its compiled default is
`1`, which disables hedged sandbox creation. Values are clamped to `1..3`.
Missing rows, malformed rows, unavailable storage, or missing request scope
fall back to `1` and are logged; they never fail a submission.

Example:

```http
PUT /api/v1/settings/execution.hedge_count
Authorization: Bearer <token>
Content-Type: application/json

{"value": 2}
```

## Daytona orphan sweeper

When Daytona is configured, the API also runs a conservative background
sweeper every five minutes. It lists only sandboxes labelled
`codegym=submission` and deletes one only when it is older than the safety
margin and its per-run ID is not tracked as active. The default margin is 15
minutes. A run remains active until both the winner has finished and every
hedged loser has completed creation and cleanup.

The controls are `CODEGYM_SANDBOX_SWEEPER_DISABLED`,
`CODEGYM_SANDBOX_SWEEPER_INTERVAL`, and
`CODEGYM_SANDBOX_SWEEPER_MAX_AGE`. Unknown or malformed creation timestamps are
left alone.
