# ISSUES
**Active backlog (open issues only)**

Entries record newly discovered requests or changes, with their outcomes.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes -` [x] [GX-<number>]`. Resolved issues are archived in `issues.md/ARCHIVE.md`.

Read @AGENTS.md, @README.md and ARCHITECTURE.md. Read issues.md/@POLICY.md, issues.md/PLANNING.md, issues.md/@NOTES.md, and issues.md/@ISSUES.md. Start working on open issues. Prioritize bugfixes and maintenance. Work autonomously and stack up PRs. 

Issue IDs in Features, Improvements, BugFixes, and Maintenance never reuse completed numbers; cleanup renumbers remaining entries so numbering stays monotonic.

## BugFixes

- [x] [B001] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step.
  LegacyExternalID: GX-345
  (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)
  ## Resolution
  - Emit an initial per-repository discovery step summary and add workflow integration coverage for the discovery output.
- [x] [B002] (P0) gix prs delete --yes is silent under default console logging.
  LegacyExternalID: GX-346
  ## Analysis
  - The CLI command in `internal/branches/command.go` runs a workflow `TaskDefinition` that calls the `repo.branches.cleanup` action from `internal/branches/task_action.go`, so output is constrained to workflow reporting and the service logger rather than direct prints.
  - The cleanup action does not write to `environment.Output` (unlike `branch.refresh` in the same file), so it emits no explicit success or no-op line.
  - Workflow execution emits `TASK_PLAN` and `TASK_APPLY` events (`internal/workflow/task_plan.go`, `internal/workflow/task_execute.go`), but `internal/repos/shared/reporting.go` suppresses those event codes in console output via `consoleSuppressedEventCodes`.
  - The cleanup service logs progress only at Info level (`internal/branches/service.go`), while the default config in `cmd/cli/default_config.yaml` sets `log_level: error`, so those logs are filtered.
  - The summary renderer in `pkg/taskrunner/summary.go` returns an empty string for single-repo runs, leaving no fallback output.
  
  ## Deliverables
  - Emit a dedicated, non-suppressed console line for `gix prs delete` per repository that reports the outcome (deleted count or no-op) without requiring a log-level change.
  - Keep suppression of `TASK_*` workflow noise intact so other commands remain quiet; only add the explicit output needed for branch cleanup.
  - Extend `tests/pr_cleanup_integration_test.go` (or add a new adjacent integration test) to capture CLI output and assert it is non-empty for a single-repo `--yes` run.
  - Acceptance: With default config (`log_format: console`, `log_level: error`) and a single repo, the command prints at least one line that includes the repo identifier/path and an outcome.
  - Acceptance: When the GH CLI returns zero closed PR branches, output explicitly states a no-op or zero deletions instead of being silent. title=gix prs delete --yes is silent under default console logging)
  ## Resolution
  - Emit per-repo cleanup summaries (closed/deleted/missing/declined/failed) and add integration coverage for output and zero-branch runs.
- [x] [B003] Workflow file replacements skip some files when glob uses `**/` (suspected in configs/account-rename.yaml).
  LegacyExternalID: GX-354
  ## Investigation
  - `configs/account-rename.yaml` uses `files.apply` with `**/*.go` and `docs/**/*.md`.
  - `internal/workflow/task_plan.go` builds replacement targets via `compileReplacementMatcher`.
  - `compileReplacementMatcher` expands `**` to `.*` but still requires the following `/` in the pattern, so `**/*.go` compiles to `^.*/[^/]*\.go$` and does not match root files like `main.go`; similarly `docs/**/*.md` misses `docs/README.md`.
  - `internal/workflow/executor_runner.go` uses a channel + waitgroup for repo work but does not early-exit the worker loop, so a channel/workgroup premature exit looks unlikely.
  ## Repro
  - Run `gix workflow configs/account-rename.yaml --roots <repo> --yes` on a repo with root-level `main.go` (containing `github.com/temirov`).
  - Observe nested `pkg/**/*.go` files updated, but root-level `main.go` unchanged.
  ## Deliverable
  - Make `**/` match zero or more path segments so `**/*.go` includes root-level files and `docs/**/*.md` includes `docs/*.md`; add coverage for root-level matches.
  ## Resolution
  - Adjusted `**/` glob matching to allow root-level files and added regression coverage for `**/*.go` and `docs/**/*.md`.
- [x] [B004] `gix prs delete` reports `failed=<N>` when local PR branches are already gone (common case).
  LegacyExternalID: GX-355
  ## Observation
  - `gix prs delete --yes` runs `git push <remote> --delete <branch>` then `git branch -D <branch>`.
  - When a closed PR branch exists on the remote but not locally, `git branch -D` exits non-zero, causing the branch to be counted as `failed` even if remote deletion succeeded.
  ## Deliverable
  - Treat missing local branches as a no-op (still count the PR branch cleanup as successful when remote deletion succeeds).
  - When real failures occur, print a short stderr summary of failure reasons (bounded) so operators can diagnose without changing log level.
  ## Resolution
  - Treat missing local branches as already-clean, so successful remote deletions count as deleted.
  - Record failure details in the cleanup summary and print bounded failure samples to stderr when failures occur; added regression coverage.


## Improvements

- [x] [I001] Improve the workflow summary.
  LegacyExternalID: GX-251
  I ran @configs/account-rename.yaml and I got:
  Summary: total.repos=104 PROTOCOL_SKIP=104 REMOTE_MISSING=1 REMOTE_SKIP=51 REPO_FOLDER_SKIP=52 REPO_SWITCHED=92 TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582 WORKFLOW_STEP_SUMMARY=582 WARN=139 ERROR=1 duration_human=6m55.109s duration_ms=415109
  Remove duration_ms Leave only human duration and rename it to duration.
  remove  TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582
  add missing steps in the summary (like namespace rewrite, namespace delete etc)
- [x] [I002] Add steps to @configs/account-rename.yaml that allows to bump up the dependency versions of go.mod (see GX-110).
  LegacyExternalID: GX-252
- [x] [I003] Add steps to @configs/account-rename.yaml to upgrade go version in go.mod to `go 1.25.4`.
  LegacyExternalID: GX-253
- [x] [I004] Embed license templates and wire the license workflow preset to render them per repository.
  LegacyExternalID: GX-254


## Maintenance

- [ ] [M001] `make ci`/`check-format` emits a Go parse error for `tools/llm-tasks/tasks/sort/task_test.go`.
  LegacyExternalID: GX-424
  ## Observation
  - `gofmt -l` prints `tools/llm-tasks/tasks/sort/task_test.go:60:2: expected declaration, found base`.
  ## Deliverable
  - Fix the invalid test file or exclude it from `check-format` so formatting checks run cleanly.


## Features

- [x] [F001] Add a step that allows running an arbitrary command, such as `go get -u ./...` and `go mod tidy`.
  LegacyExternalID: GX-110
  The changed files need to be committed after this step. Deliver both the DSL and the implementation.
- [ ] [F003] Add a local web interface for `gix`.
  Requested on 2026-03-05.
  ### Summary
  Launching `gix --web <port:-8080>` should start a local HTTP server that exposes the existing CLI command surface through a browser UI, rather than requiring users to compose all operations in the terminal.

  ### Constraints
  - Reuse the existing command implementations rather than forking a second execution stack.
  - Bind locally by default and treat the web UI as a localhost tool, not a remote multi-user service.
  - Preserve the current CLI behavior and tests while adding the web mode behind an explicit flag.
  - Support every current CLI command at least through a generic command runner, even if richer typed forms arrive incrementally.

  ### Deliverables
  - [ ] Add a root `--web` launch mode with default port `8080`.
  - [ ] Serve a browser UI and JSON API from a local embedded HTTP server.
  - [ ] Execute existing `gix` commands in-process from the web layer and stream logs/results back to the browser.
  - [ ] Expose command metadata so the UI can render flags, args, defaults, and help text.
  - [ ] Add integration coverage for web launch, command catalog, and at least one end-to-end command execution path.
- [ ] [F002] (P1) Duplicate logging.
  LegacyExternalID: GX-111
  ### Summary
  When the `gix cd` command fails (for example, due to local changes blocking a branch switch), the error message is printed twice in the terminal. This duplicate logging clutters the output and violates the repository's principle of structured, single-entry reporting.
  
  ### Analysis
  The duplication is caused by a lack of coordination between the domain service, the workflow executor, and the CLI entry point:
  
  1.  **Improper Error Types**: `internal/branches/cd/service.go` returns standard errors via `fmt.Errorf` instead of the structured `repoerrors.OperationError` type. This prevents the workflow layer from identifying the error as a handled repository event.
  2.  **Fallback Printing**: In `internal/workflow/executor_runner.go`, the function `executeRepositoryStageForRepository` attempts to log the error. Because it is not an `OperationError`, it fails the check in `logRepositoryOperationError` (found in `internal/workflow/error_handling.go`) and falls back to a manual `fmt.Fprintln` to `stderr`.
  3.  **CLI Exit Redundancy**: The error is then bubbled up to `main.go`. Since the command's `RunE` returns the error, the main function prints it a second time before exiting.
  4.  **Context Loss**: The `collectOperationErrors` helper in `internal/workflow/executor.go` unwraps the error chain too aggressively, resulting in the same underlying Git error being printed in both instances, stripped of its high-level context (e.g., "Switch branch to master").
  
  ### Deliverables
  - [ ] **Structured Error Implementation**: Refactor `internal/branches/cd/service.go` to utilize `repoerrors.Wrap` for all Git-related failures.
  - [ ] **Reporting Logic Alignment**: Update `internal/workflow/error_handling.go` to ensure that all repository-scoped errors are processed via the `StructuredReporter`, eliminating the need for manual fallback printing.
  - [ ] **CLI Exit Refinement**: Adjust the CLI execution flow to ensure that errors already emitted by the reporter do not trigger a second print at the application exit point.
  - [ ] **Verification**: Add an integration test case that triggers a predictable Git failure and asserts that the resulting error message appears exactly once in the combined output stream.


## Planning
*do not implement yet*
