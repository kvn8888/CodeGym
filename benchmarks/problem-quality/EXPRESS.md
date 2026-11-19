# Express test prerequisites

The Express corpus case declares `npm test` as its test command and uses the
real Express package. Dependencies are installed during setup, not during
assessment.

## Prerequisites

- Node.js (verified with 22.14.0)
- npm (verified with 10.9.2)
- Express 4.22.2, pinned in `artifact/package.json`
- `artifact/package-lock.json` (lockfileVersion 3)

## Install dependencies

From `benchmarks/problem-quality/cases/express-greeting-endpoint/artifact`:

```bash
npm ci
```

The case manifest uses the same `npm ci` setup command. The assessment phase
only runs `npm test`; it does not install dependencies.

If the npm cache is populated and the machine is offline:

```bash
npm ci --offline
```

The combined local benchmark command can use `npm_config_offline=true` to make
the setup stage use the cache. If neither the network nor the cache can satisfy
`package-lock.json`, the Express execution checks are blocked. Structural
checks, case loading, and the Go/Python fixtures can still run. Do not report a
blocked Express execution as passed.

## Test harness behavior

`artifact/test/greeting.test.js`:

- selects an available loopback port instead of a fixed port;
- starts `src/app.js` as a child process with that port;
- polls for readiness with a bounded timeout;
- sends bounded HTTP requests and checks status, JSON shape, error shape,
  unknown-query handling, and no-trimming behavior;
- bounds captured server stdout/stderr and each HTTP response body; overflow
  is reported as `OUTPUT_OVERFLOW` or `RESPONSE_OVERFLOW` instead of growing
  memory without limit;
- emits `CODEGYM_TEST` lines for the protected tests;
- terminates the server with `SIGTERM` and falls back to `SIGKILL` before
  exiting, including on failure.

`artifact/test/capture.js` contains the bounded capture helpers, and
`artifact/test/capture.test.js` is the focused overflow verification. The
declared test command runs both files (`node test/greeting.test.js && node
--test test/capture.test.js`).

The repository-local benchmark run is documented in the generated integration
report under `benchmark-results/integration/`.
