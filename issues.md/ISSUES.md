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


## BugFixes (347–399)

- [ ] [GX-345] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step.
  (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)
- [ ] [GX-346] (P0) gix prs delete --yes is silent under default console logging.
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


## Maintenance (400–499)

## Planning
*do not implement yet*

