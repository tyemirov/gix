# ISSUES
**Active backlog (open issues only)**

Entries record newly discovered requests or changes, with their outcomes.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes -` [x] [GX-<number>]`. Resolved issues are archived in `issues.md/ARCHIVE.md`.

Read @AGENTS.md, @README.md and ARCHITECTURE.md. Read issues.md/@POLICY.md, issues.md/PLANNING.md, issues.md/@NOTES.md, and issues.md/@ISSUES.md. Start working on open issues. Prioritize bugfixes and maintenance. Work autonomously and stack up PRs. 

Issue IDs in Features, Improvements, BugFixes, and Maintenance never reuse completed numbers; cleanup renumbers remaining entries so numbering stays monotonic.

## Features (111–199)

- [x] [GX-110] Add a step that allows running an arbitrary command, such as `go get -u ./...` and `go mod tidy`.
  The changed files need to be committed after this step. Deliver both the DSL and the implementation.


## Improvements (261–299)

- [x] [GX-251] Improve the workflow summary.
  I ran @configs/account-rename.yaml and I got:
  Summary: total.repos=104 PROTOCOL_SKIP=104 REMOTE_MISSING=1 REMOTE_SKIP=51 REPO_FOLDER_SKIP=52 REPO_SWITCHED=92 TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582 WORKFLOW_STEP_SUMMARY=582 WARN=139 ERROR=1 duration_human=6m55.109s duration_ms=415109
  Remove duration_ms Leave only human duration and rename it to duration.
  remove  TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582
  add missing steps in the summary (like namespace rewrite, namespace delete etc)
- [x] [GX-252] Add steps to @configs/account-rename.yaml that allows to bump up the dependency versions of go.mod (see GX-110).
- [x] [GX-253] Add steps to @configs/account-rename.yaml to upgrade go version in go.mod to `go 1.25.4`
- [x] [GX-254] Embed license templates and wire the license workflow preset to render them per repository.


## BugFixes (348–399)

- [x] [GX-345] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step.
  (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)
  ## Resolution
  - Emit an initial per-repository discovery step summary and add workflow integration coverage for the discovery output.
- [x] [GX-346] (P0) gix prs delete --yes is silent under default console logging.
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

- [x] [GX-354] Workflow file replacements skip some files when glob uses `**/` (suspected in configs/account-rename.yaml).
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

- [x] [GX-355] Workflow replacements sometimes leave files unchanged when many files match.
  ## Report
  - Running `gix workflow configs/account-rename.yaml` across large repos occasionally leaves some `**/*.go` or `docs/**/*.md` matches untouched even though most files update.
  - Suspected early exit from worker channels/workgroups.
  ## Investigation
  - `internal/workflow/executor_runner.go` drains a work channel with a waitgroup; it does not early-exit workers once launched, so a channel/workgroup bug seems unlikely.
  - Repository pipelines stop early only on `repositorySkipError` or action failure; `taskExecutor` returns `repositorySkipError` when any guard/action returns a skip (e.g., dirty worktree), which halts remaining steps for that repo.
  - Replacement planning/execution is sequential in `internal/workflow/task_plan.go` and `internal/workflow/action_builtin.go`; target collection is not concurrent.
  - Partial updates remain possible if `files.apply` returns an error mid-apply (e.g., a write error), which would stop later changes but should surface as a failure/summary warning.
  ## Repro Attempt (ProductScanner)
  - Copied `../../MarcoPoloResearchLab/ProductScanner` to `/tmp/gix-diagnostics/ProductScanner` and ran a trimmed workflow containing only the `namespace-rewrite` task to avoid push/PR steps.
  - Baseline counts: `github.com/temirov` in `**/*.go` = 322, in `go.mod`/`go.sum` = 4, `temirov` in `README.md` = 1, `temirov` in `docs/**/*.md` = 2.
  - After `bin/gix workflow /tmp/gix-diagnostics/account-rename-namespace-only.yaml --roots /tmp/gix-diagnostics/ProductScanner --yes`, all counts dropped to 0; no missing replacements observed.
  ## Repro Attempt (Multi-repo fanout)
  - Copied `../../MarcoPoloResearchLab/ProductScanner` 10x under `/tmp/gix-diagnostics/multi/ProductScanner-*`.
  - Baseline counts across all copies: `github.com/temirov` in `**/*.go` = 3220, in `**/go.mod`/`**/go.sum` = 150, `temirov` in `**/README.md` = 30, `temirov` in `**/docs/**/*.md` = 20.
  - Ran `bin/gix workflow /tmp/gix-diagnostics/account-rename-namespace-only.yaml --roots /tmp/gix-diagnostics/multi --yes --workflow-workers 10` (summary: `STEP_NAMESPACE_REWRITE_APPLIED=10`).
  - After run: `github.com/temirov` in `**/*.go` = 0, no matches in root `go.mod`/`go.sum` or root `README.md`; `**/docs/**/*.md` = 0. Remaining `github.com/temirov`/`temirov` hits were confined to nested `go.mod`/`go.sum` and README files not targeted by the workflow.
  ## Repro Attempt (Multi-repo full workflow, no push/PR)
  - Used `/tmp/gix-diagnostics/account-rename-no-push-pr.yaml` (full `configs/account-rename.yaml` minus `git push`/`pull-request open`; `restore-original-state` now follows `namespace-stage-commit`).
  - Ran against 10 copies under `/tmp/gix-diagnostics/multi` with `--workflow-workers 10`.
  - `folder rename` renamed one repo to `MarcoPoloResearchLab/ProductScanner` and failed for nine with `target exists`; total repos reported = 13.
  - `go-deps-upgrade` failed for all repos (`go get -u ./...` error: module path mismatch `github.com/temirov/GAuss` vs `github.com/tyemirov/GAuss`), so later steps did not run.
  - Despite the failures, no leftover `github.com/temirov` in `**/*.go`, root `go.mod`/`go.sum`, root `README.md`, or `docs/**/*.md` across the 10 repos.
  ## Resolution
  - Could not reproduce after single-repo, multi-repo namespace-only, and multi-repo full workflow (no push/PR) runs; marking as non-reproducible.
  ## Next Steps
  - Capture a repro repo + workflow output to confirm whether the run logged a skip/failure and which pattern was missed.
  - Add integration coverage that asserts full replacement counts for a repo with large file fan-out once repro is confirmed.


## Maintenance (400–499)

- [x] [GX-424] `make ci`/`check-format` emits a Go parse error for `tools/llm-tasks/tasks/sort/task_test.go`.
  ## Observation
  - `gofmt -l` prints `tools/llm-tasks/tasks/sort/task_test.go:60:2: expected declaration, found base`.
  ## Deliverable
  - Fix the invalid test file or exclude it from `check-format` so formatting checks run cleanly.
  ## Resolution
  - Restored the missing test wrapper so the file parses and gofmt/check-format pass.

## Planning
*do not implement yet*
