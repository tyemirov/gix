# GX-411 Refactor & Testing Plan

## Scope

This plan inventories the highest-risk gaps against `POLICY.md` and `AGENTS.md` and proposes sequenced refactors plus accompanying test work. The focus areas surfaced while working through GX-215–GX-322: CLI composition, workflow/task execution, LLM integrations, and structured reporting.

## 1. CLI Composition (`cmd/cli`)

- **Observations**
  - `cmd/cli/application.go` (~1.6k LOC) mixes configuration loading, command wiring, logging bootstrap, and legacy alias handling. Violates confident-programming guidance on narrow interfaces and small composable units.
  - Subcommand builders (workflow, changelog, commit) duplicate dependency wiring (`TaskRunnerFactory`, `ClientFactory`, logging providers).
  - Default configuration embedding lives in the same file, complicating test seams.
- **Actions**
  1. Extract configuration/bootstrap logic into `cmd/cli/bootstrap` (new package): `NewApplication` should delegate to smaller builders per namespace.
  2. Move default configuration / embedded YAML into dedicated module with tests validating invariants (`docs/readme_config_test.go` can point to new helper).
  3. Introduce shared helper for `TaskRunner` wiring (currently duplicated in commit + changelog modules) to ensure identical error handling and option propagation.
  4. Add focused tests for CLI root wiring (currently only high-level tests) verifying that new command layout and alias handling stays stable.

## 2. Workflow Engine (`internal/workflow`)

- **Observations**
  - `executor.go` still couples planning, execution, reporting, and failure formatting. Recent GX-322 work improved failure resilience but the file remains complex and lacks unit tests for branch coverage (metadata capture, nested repo detection, prompt dispatch).
  - Stage planning (`planOperationStages`) is opaque; no tests ensure ordering/parallelisation invariants.
  - Cancellation semantics were loosened; follow-up refactor should clarify context propagation & add metrics on how many operations failed vs. skipped.
- **Actions**
  1. Split executor into: `planner.go` (stage planning), `runner.go` (concurrency & error aggregation), `reporting.go` (summary + repository registration).
  2. Introduce struct `ExecutionOutcome` describing succeeded/failed operations; executor returns this while CLI decides exit code. Facilitates future UX (warnings vs fatal) per POLICY.
  3. Add table-driven tests covering: mixed success/failure sequences, prompt state interactions, metadata skippers, nested repository ordering, and ensure reporter counts remain accurate.
  4. Instrument reporter to emit metrics/events for stage completion to help future observability work.

## 3. Task Operations (`internal/workflow/operations_tasks.go`)

- **Observations**
  - File exceeds 1.3k LOC, combining parsing, templating, execution, LLM wiring, and GitHub PR creation.
  - Error surfaces rely on `fmt.Fprintf` instead of structured reporter events for several branches (e.g., branch exists, remote missing) which conflicts with policy’s structured logging guidance.
  - Testing focuses on planner edges; execution paths (branch/PR workflows, safeguard failures) lack coverage.
- **Actions**
  1. Decompose into subpackages: `tasks/parse`, `tasks/plan`, `tasks/execute`, `tasks/actions`. Each should expose narrow interfaces with explicit dependencies (filesystem, git, reporter).
  2. Introduce strategy objects for branch management & PR creation to allow deterministic unit tests without large stubs.
  3. Migrate user-facing logging to `StructuredReporter` (`TASK_*` codes) and remove direct `fmt.Fprintf` calls.
  4. Add integration-style tests for: ensure-clean failure, branch reuse, PR creation error paths, safeguard evaluation, and LLM action capture-as flows.

## 4. LLM Integrations (`internal/changelog`, `internal/commitmsg`, `cmd/cli/changelog`/`commit`)

- **Observations**
  - `fakeChatClient` tests cover happy paths only; no tests for empty responses, timeouts, or API errors.
  - Client factories duplicate validation of base URL/model/api key.
  - No cache or rate-limiting guard despite policy requirement to inject external effects.
- **Actions**
  1. Extract shared LLM client builder into `pkg/llm/factory` with injectable HTTP client/timeouts; add tests for invalid inputs.
  2. Enhance generator tests with table-driven cases for empty diff, no commits, LLM returning empty string (ensure `ErrNoChanges` flows as expected).
  3. Implement retry/backoff policy configuration (per POLICY “external effects injected”) and add tests verifying context cancellation handling.

## 5. Structured Reporting (`internal/repos/shared/reporting.go`)

- **Observations**
  - Reporter currently tracks counts but lacks API to emit totals per event category; error levels only aggregate WARN/ERROR.
  - No hook for exporting telemetry (timings per operation) despite policy leaning on measurable outcomes.
- **Actions**
  1. Add `RecordEvent(code string, level EventLevel)` helper to simplify event counting from commands; refactor existing modules to consume it (reduces manual `Report` calls).
  2. Expose structured summary object for CLI to print/serialize; tests should validate JSON-able representation.
  3. Add benchmarks (using Go’s `testing.B`) to gauge reporter overhead after repository registration changes.

## 6. Testing & Tooling

- **Observations**
  - `go test ./...` regularly exceeds 45s under harness; need granular targets and possibly parallelisation tweaks.
  - Several integration tests rely on actual filesystem/Git; could benefit from deterministically seeded fake executors to cut runtime.
- **Actions**
  1. Introduce Makefile targets for fast/unit vs. slow/integration suites; update CI script samples accordingly.
  2. Audit test fixtures for redundant repositories; consolidate into reusable helpers (e.g., `tests/internal/gitutil`).
  3. Add coverage gates for critical packages (workflow, tasks, namespace) ensuring no drop below agreed thresholds.

## 7. Backlog Seeding

- File focused improvement tasks (post-refactor) into `ISSUES.md` once validated: e.g., consolidate CLI flag parsing helpers, adopt smart constructors for nested config (commit/changelog clients), and enforce consistent event codes for LLM actions.

## Suggested Sequencing

1. **Short-term (1–2 iterations)**: split `cmd/cli/application.go`, modularise workflow executor, improve error aggregation API, enhance structured reporter.
2. **Mid-term**: refactor task operations into layered packages, bolster LLM generator resiliency, expand test coverage.
3. **Long-term**: telemetry hooks, CI gating improvements, performance benchmarking, and follow-up cleanup derived from new modular boundaries.

This plan gives engineering a concrete roadmap that aligns with POLICY’s confident programming tenets while tackling the most complex/high-risk modules first.
