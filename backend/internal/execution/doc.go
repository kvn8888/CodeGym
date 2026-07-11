// Package execution runs user-submitted code in isolated Daytona sandboxes.
//
// The package has three tracks, in order of maturity:
//
//   - SubmissionRunner (live): deterministic, LeetCode-style submission runs
//     in ephemeral, fully network-blocked sandboxes. Service, stores,
//     handlers, and validation are implemented; the Daytona-facing core in
//     daytona_runner.go is a scaffold with TODOs (Kevin implements it — the
//     gated tests in daytona_runner_test.go are the finish line, and
//     spikes/daytona/main.go is the validated reference).
//
//   - EnvironmentBuilder (interface only): the agent-scaffold track for
//     runtimes the language registry doesn't cover; built environments get
//     promoted to snapshots for deterministic reuse.
//
//   - WorkspaceProxy (interface only): sandbox filesystem + PTY access for
//     a future Monaco file-explorer workspace UI.
//
// Runs are executed synchronously in v0 but persist through
// queued → running → passed/failed/error, so the model survives a move to
// asynchronous workers unchanged.
package execution
