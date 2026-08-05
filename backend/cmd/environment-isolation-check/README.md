# Environment isolation check

Run the read-only Doppler comparison from `backend/`:

```bash
go run ./cmd/environment-isolation-check
```

The command reads `codegym/dev`, `codegym/stg`, and `codegym/prd`, fingerprints
`NEON_CONNECTION_STRING`, `DAYTONA_API_KEY`, and
`CODEGYM_RELAY_TOKEN_SECRET` with SHA-256, and exits non-zero when a required
value is missing or shared. Its output boundary accepts fingerprints only; raw
secret values and raw Doppler subprocess output are never printed. It uses the
encrypted fallback file `/private/tmp/codegym-doppler-fallback` by default.
