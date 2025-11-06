# GX-402 Refactor Roadmap (Policy Compliance)

## 1. Gaps against `POLICY.md`

- **Domain types missing** – repository paths, owner/repo slugs, remote URLs, and branch names flow through the system as raw `string` values (`internal/repos/rename/executor.go`, `internal/repos/remotes/executor.go`, `internal/workflow/task_runner.go`). Smart constructors and invariants are absent, so illegal states remain representable.
- **Edge validation vs. core trust** – CLI builders parse flags but do not construct domain types; downstream executors defensively trim and validate strings repeatedly (for example, `strings.TrimSpace` in `internal/repos/rename/executor.go:59`, `internal/repos/protocol/executor.go:61`, `internal/repos/rename/planner.go:28`), violating the “validate exactly once at edges” rule.
- **Error taxonomy** – executors mostly print formatted strings to `io.Writer` sinks instead of returning contextual, wrapped errors (`internal/repos/protocol/executor.go:37`, `internal/repos/remotes/executor.go:55`, `internal/repos/history/executor.go:70`), making it impossible to attach stable codes.
- **Testing gaps** – packages such as `internal/repos/protocol` have no direct tests for protocol detection, owner inference, or error branches; `internal/repos/dependencies` (resolver logic) and the new domain constructors lack coverage.
- **Duplication / slop** – prompt formatting, output helpers (`printfOutput`/`printfError`), and owner/repo parsing are implemented separately in multiple packages.

## 2. Refactor Phases

### Phase A – Domain Modeling
1. Introduce `internal/repos/domain` (or extend `shared`) with smart constructors for:
   - `RepositoryPath`, `OwnerSlug`, `RepositoryName`, `OwnerRepository`, `RemoteURL`, `RemoteName`, `BranchName`, and `RemoteProtocol`.
   - Each constructor enforces non-empty values, canonical casing, and safe filesystem segments; return `(Type, error)` with sentinel errors.
2. Replace raw strings in options/structs across `internal/repos` and `internal/workflow` with the new domain types.
3. Migrate CLI edge code (`cmd/cli/repos/*.go`, `cmd/cli/workflow/*.go`) to validate inputs once, constructing domain values before invoking services.

### Phase B – Error Strategy
1. Define typed error values (e.g., `ErrUnknownProtocol`, `ErrCanonicalOwnerMissing`) plus helpers that wrap them with operation + subject + stable code (`repo.remote.update.unknown_protocol`).
2. Refactor executors (`internal/repos/remotes`, `protocol`, `rename`, `history`) to return `error` instead of (or in addition to) printing failure messages. CLI layers will format user-facing output while retaining structured errors for logs/tests.
3. Update tests to assert on wrapped errors rather than stdout strings, providing deterministic validation.

### Phase C – Service Cleanup
1. Extract shared helpers:
   - Owner/repo parsing into a single module reused by `rename`, `remotes`, and `protocol`.
   - Output/prompt formatting into a composable reporter interface bound at the CLI level.
2. Remove repeated `strings.TrimSpace` blocks; rely on normalized domain types.
3. Review boolean options (e.g., `AssumeYes`, `RequireCleanWorktree`) and document why they remain booleans or replace them with explicit enums if multiple behaviors are combined.

### Phase D – Testing Expansion
1. Add table-driven unit tests for new constructors, ensuring invalid inputs are rejected with precise sentinel errors.
2. Cover protocol conversion edge cases (`internal/repos/protocol`): mismatched current protocol, missing owner/repo, unknown protocol errors, and general plan output.
3. Add resolver tests for `internal/repos/dependencies.ResolveGitExecutor/ResolveGitRepositoryManager` verifying logger wiring and error propagation.
4. Expand workflow integration tests to ensure domain types propagate correctly after refactors.

### Phase E – Documentation & Tooling
1. Update `POLICY.md` appendix or a new `docs/refactor_status.md` with the domain types and error codes introduced.
2. Document the edge-validation contract in `docs/cli_design.md` so future commands follow the same pattern.
3. Wire CI to run `staticcheck` and `ineffassign` alongside existing `go test` gate once code compiles with new types.

## 3. Sequencing & Risk Mitigation

- **Incremental rollout**: start with domain types and a single slice of functionality (e.g., remote updates) to prove the pattern, then propagate across other executors.
- **Backward compatibility**: maintain current CLI output initially by adapting reporters; once tests assert on structured errors, gradually migrate user messaging.
- **Testing first**: create regression tests around existing behavior before refactors to prevent accidental behavior regression (especially around rename skips and history purge safeguards).
- **Communication**: track progress in `CHANGELOG.md` and consider opening follow-up issues per phase to manage scope.

This roadmap satisfies GX-402 by pinpointing policy violations, defining required structural changes, and outlining a test-backed migration path.
