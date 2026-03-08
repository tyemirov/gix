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

- [x] [F004] Audit in the web client needs a first-class inspection flow with user-selected roots instead of parsing CLI stdout.
  Requested on 2026-03-08 while refining the `gix --web` audit UX.
  Resolved on 2026-03-08 by adding `POST /api/audit/inspect`, wiring `audit.Service.DiscoverInspections` into the web launcher, adding explicit audit-root controls in the browser, and rendering typed audit rows directly in the web table while keeping the generic runner available for raw CLI execution.
  ### Summary
  The current web interface treats audit as a generic command run and renders the CSV only after `gix audit` executes through the runner. That blocks the next UX steps because the browser has no typed audit model, no independent root selection, and no stable contract for row-level actions.

  ### Analysis
  - `internal/web/server.go` currently exposes repositories, branches, commands, and generic runs only. There is no dedicated audit endpoint.
  - `internal/web/ui/assets/app.js` renders audit results by parsing `stdout` from a `RunSnapshot`, so the browser only sees CSV text rather than typed inspection rows.
  - `cmd/cli/application_web.go` launches the web server with repository discovery rooted in the process working directory. That startup catalog is useful as context, but it is not a substitute for explicit audit roots.
  - The existing audit service in `internal/audit/service.go` already provides the right backend primitive: `DiscoverInspections(ctx, roots, includeAll, debug, depth)` returns typed repository inspections before CSV formatting.
  - The remediation UX requested by the user depends on stable row identity (`path`, `folder_name`, remote state, branch state), which should come from JSON instead of CSV parsing.

  ### Deliverables
  - [x] Add a dedicated web audit inspection API (for example `POST /api/audit/inspect`) that accepts explicit roots plus `include_all`.
  - [x] Return typed audit rows from the backend rather than only CSV text.
  - [x] Add audit-root controls in the web UI so the operator can inspect arbitrary roots without relaunching `gix --web`.
  - [x] Preserve the generic command runner for raw CLI access; the audit workspace should not depend on it for table rendering.
  - [x] Add server/API and browser coverage for user-selected roots and typed audit table rendering.

- [x] [F005] Audit remediation in the web client must be modeled as a pending change queue before execution.
  Requested on 2026-03-08 while defining follow-up UX after the audit table landed.
  Resolved on 2026-03-08 by adding a typed pending-change queue to the web audit workspace, surfacing editable per-item options in the queue, and applying queued changes in a deterministic order so path-changing rename operations run after repository-state fixes.
  ### Summary
  Row-level fixes in the browser should never execute immediately. The user wants every remediation action to become a queued pending change first, with explicit review before apply.

  ### Analysis
  - The current web UI has draft behavior for some commands (`draft_template` usage in `internal/web/ui/assets/app.js`), but it does not have a generalized queue, conflict resolution, or a typed pending-change model.
  - Existing repository operations already exist in the workflow/task layers for rename, canonical remote update, protocol conversion, and default-branch promotion. The missing piece is a browser-side queue model plus a backend execution contract for batched pending items.
  - Some requested actions conflict by construction. Example: deleting a folder conflicts with any queued fix for the same path; multiple remote fixes for one repo should merge or be rejected; sync should be blocked when destructive changes are pending for the same repository.
  - If the queue is modeled as raw argv strings, the browser will be forced to reverse-engineer conflicts and capabilities from command text. The queue needs typed operations instead.

  ### Deliverables
  - [x] Define a typed pending-change model shared by web UI and backend execution.
  - [x] Add queue operations in the UI: add, edit options, remove, clear, and review.
  - [x] Reject or merge conflicting queued operations deterministically.
  - [x] Execute queued changes through typed backend handlers or workflow operations rather than ad hoc shell text.
  - [x] Re-run audit for affected roots after queue execution and refresh the table from typed results.

- [x] [F006] Audit output must report remote presence explicitly in both CLI and web/API contracts.
  Requested on 2026-03-08 after reviewing how missing remotes appear in audit results.
  Resolved on 2026-03-08 by extending audit and workflow CSV output with `origin_remote_status`, making missing remotes emit `missing` with `n/a` protocol/canonical fields, and updating the web inspection API plus integration coverage to surface the status directly.
  ### Summary
  When a repository has no `origin`, audit should say so explicitly. Today the CLI/web output exposes enough indirect fields to infer the situation, but the contract does not contain a dedicated remote-status field, which makes the UX ambiguous and blocks correct remediation suggestions.

  ### Analysis
  - `internal/audit/service.go` already distinguishes the no-remote case internally by returning an inspection with empty `OriginURL`, empty owner/repo fields, and `OriginMatchesCanonical = n/a`.
  - The CSV/report contract drops that distinction. Operators only see related columns such as `final_github_repo`, `remote_protocol`, and `origin_matches_canonical`, none of which explicitly say `no remote configured`.
  - The web client currently labels audit state from parsed CSV, so it inherits the same ambiguity.
  - A dedicated remote-status field is the lowest-risk contract extension because it avoids overloading `origin_matches_canonical` with two meanings.
  - This change must be wired through every audit output path, including direct audit CSV and workflow-backed audit reports in `internal/workflow/operations_audit.go`.

  ### Deliverables
  - [x] Add an explicit audit field/column for origin remote status (for example `configured`, `missing`, `n/a`).
  - [x] Ensure missing-remotes render as such in the web table and are not framed as canonical-remote mismatches.
  - [x] Update CLI and integration tests that assert audit CSV headers and rows.
  - [x] Keep current `origin_matches_canonical` semantics for real remotes; do not repurpose it to mean remote absence.

- [x] [F007] The web audit workspace needs a UX-only folder deletion operation, queued before apply.
  Requested on 2026-03-08 as part of the row-action audit UX.
  Resolved on 2026-03-08 by surfacing a web-only `delete_folder` row action in the audit table, requiring explicit queue confirmation before apply, and covering the queue/apply/refresh flow with a browser test. The current scope intentionally allows deleting any audited folder path from the web queue, including repository folders, while still blocking filesystem-root deletion in the backend.
  ### Summary
  The browser should support deleting a folder directly from the audit workspace, but only as a queued web action rather than a new immediate CLI behavior.

  ### Analysis
  - There is no existing CLI command dedicated to removing an inspected folder from disk. The existing `repo-history-remove` flow rewrites Git history and is unrelated.
  - Because folder deletion is destructive and outside the existing command surface, it should remain web-only until the UX, safety model, and scope are validated.
  - The most sensitive design choice is whether deletion applies only to non-git folders discovered via `include_all` or also to full repositories. The request currently says “delete a folder that matches the repo altogether,” which implies repository deletion may be intended, but that requires stronger guardrails than a simple button.
  - The queue model from [F005] is a prerequisite because this action must never execute immediately from a table row.

  ### Deliverables
  - [x] Define a web-only queued delete-folder operation with explicit confirmation requirements.
  - [x] Restrict or phase the scope intentionally (for example non-git folders first, then repositories if approved).
  - [x] Surface the operation only in the web audit workspace, not in the generic CLI runner.
  - [x] Add end-to-end tests that verify deletion is queued first and only applied after explicit confirmation.

- [x] [F008] The web audit queue needs repository sync and protocol-fix actions backed by existing operations.
  Requested on 2026-03-08 as part of the audit remediation UX.
  Resolved on 2026-03-08 by surfacing queued protocol-fix and sync row actions in the web audit table, adding editable target-protocol and dirty-worktree-policy controls in the queue, and covering the queue/apply/refresh flow with a browser test.
  ### Summary
  The audit table should let operators queue protocol fixes and “sync local with remote” changes directly from mismatched rows.

  ### Analysis
  - Protocol conversion already exists through `remote update-protocol` and `ProtocolConversionOperation`.
  - Local/remote sync is less direct. `gix default` promotes a branch to the remote default branch and changes GitHub configuration; that is too heavy for a generic “bring local into sync” table action. The more appropriate existing behavior is closer to `branch.change` / `gix cd`, which fetches, switches, and rebases for repository roots.
  - The queue UX therefore needs a clearer action taxonomy than the current audit columns provide. A row with `in_sync = no` should produce a queue item that describes the exact operation that will run, branch target, and dirty-worktree policy.
  - These fixes should refresh audit state after apply so the table becomes the post-change source of truth.

  ### Deliverables
  - [x] Add queued protocol-fix actions backed by the existing protocol conversion operation.
  - [x] Add queued local-sync actions backed by an explicit branch-refresh/change workflow, not default-branch migration.
  - [x] Surface per-item options needed for safe execution (branch target, dirty-worktree handling, protocol target).
  - [x] Add integration/browser coverage that verifies queueing plus execution updates the table state.

- Update on 2026-03-08 for [F005].
  Implemented the queue foundation without closing the full remediation program.
  ### What Landed
  - Added a typed apply contract at `POST /api/audit/apply` through `internal/web/types.go`, `internal/web/server.go`, and `cmd/cli/application_web.go`. The browser now sends structured queued changes instead of argv text.
  - Added a browser-side pending-change model in `internal/web/ui/assets/app.js` with queue add/remove/clear/apply behavior, queue summary rendering, and post-apply audit reinspection against the active audit roots.
  - Added backend execution mapping for queued change kinds to existing workflow/task primitives:
    - `rename_folder` -> `workflow.RenameOperation`
    - `update_remote_canonical` -> `workflow.CanonicalRemoteOperation`
    - `convert_protocol` -> `workflow.ProtocolConversionOperation`
    - `sync_with_remote` -> `branch.change` task action with refresh enabled
    - `delete_folder` -> web-only filesystem deletion path guarded by `confirm_delete`
  - Added end-to-end coverage in `cmd/cli/application_web_test.go` and `cmd/cli/application_web_browser_test.go` for queue submission and rename apply flow.

  ### Queue Semantics Implemented
  - The queue is typed by `kind` plus repository `path`, with stable queue IDs generated in the browser.
  - Re-queueing the same `kind` for the same `path` replaces the existing queued item in place instead of duplicating it.
  - `delete_folder` is treated as exclusive for a path: if deletion is already queued, other fixes for that path are rejected; if other fixes exist, deletion is rejected until they are removed.
  - Successful apply results are removed from the pending queue; failed items remain queued.
  - After apply, the web client reruns typed audit inspection for the previously inspected roots so the table reflects post-change state instead of stale pre-apply rows.

  ### Completion Notes
  - The queue now exposes editable options for rename, delete, protocol conversion, and sync items.
  - The browser now surfaces row actions for rename, canonical-remote fixes, protocol fixes, sync, and web-only folder deletion.
  - Queue application is ordered deterministically so repository-state fixes run before rename and delete, avoiding stale-path execution during multi-step remediation.

- Update on 2026-03-08 for review follow-up on [F005], [F007], and [F008].
  Addressed three post-review correctness regressions in the queued web audit flow and locked them with regression coverage before patching.
  ### Summary
  The initial queued-remediation implementation had three edge-case failures: apply could refresh the wrong audit scope, workflow-backed changes that reported `skipped` were surfaced as successful, and the apply endpoint accepted relative paths for destructive operations.

  ### Analysis
  - The browser stored the last successful audit rows, but `applyAuditQueue()` refreshed via `inspectAuditRoots(false)`, which rebuilt the request from the live form and repository catalog. That was incorrect because queued changes semantically belong to the last inspected scope, not whatever the controls happen to contain at apply time.
  - This mismatch was especially dangerous after path-changing operations. If the operator edited the roots input after queueing, or if the repository catalog was stale relative to rename/delete actions, the table refresh could show an unrelated scope or fail to reflect the just-applied changes.
  - The Go apply executor treated `nil` workflow errors as unconditional success. That was too optimistic because workflow/task execution already communicates `skipped` outcomes through `workflow.ExecutionOutcome.ReporterSummaryData.StepOutcomeCounts`, and some remediation actions intentionally skip without returning a hard error.
  - Surfacing `skipped` as `succeeded` caused the browser to drop queued items that had not actually been applied and to report a misleading success state to the operator.
  - `normalizeWebAuditChangePath` only trimmed and cleaned the submitted path. That allowed relative inputs such as `../sibling` to escape the inspected directory context at the API boundary, which is unacceptable now that `/api/audit/apply` can drive destructive filesystem operations.

  ### Deliverables
  - [x] Added a browser regression test that queues a rename, mutates the roots input, applies the queue, and verifies the post-apply refresh still uses the last inspected audit scope.
  - [x] Split audit inspection in the web client into “build request from controls” and “rerun a saved request,” then updated queue apply to re-inspect with `state.auditInspectionRoots` and `state.auditInspectionIncludeAll`.
  - [x] Added backend regression coverage for relative-path rejection and for mapping workflow execution outcomes to `succeeded` vs `skipped` vs `failed`.
  - [x] Changed workflow-backed apply execution to derive result status from `workflow.ExecutionOutcome`, preserving skipped items in the queue and avoiding misleading success messages.
  - [x] Hardened `/api/audit/apply` path normalization so only absolute paths are accepted for queued audit changes.

- Update on 2026-03-08 for browser-test harness stability in CI.
  Addressed a CI-only Chrome startup failure that surfaced after the audit browser suite expanded.
  ### Summary
  The browser integration suite could fail in Linux CI even when a Chrome executable was present because the process exited during startup with crashpad-related output before DevTools became available.

  ### Analysis
  - The previous guard only skipped browser tests when no browser executable could be found. That was insufficient for CI runners where Chrome exists on disk but cannot boot cleanly in the available kernel/container environment.
  - The observed failure occurred before any page assertions ran and presented as `chrome failed to start` with crashpad file access errors under `/sys/devices/system/cpu/...`. That places the defect in the shared test harness, not in any specific browser test case.
  - Without a startup probe, each browser test assumed allocator creation implied a usable browser session. In practice, chromedp can return a startup failure only when the first action is executed, which makes the suite brittle and produces noisy failures unrelated to product behavior.
  - Disabling crash-reporting flags reduces the chance of environment-specific startup exits, while an explicit startup probe lets the suite distinguish “browser unavailable in this runner” from real DOM or workflow regressions.

  ### Deliverables
  - [x] Added a small regression test for the browser-startup skip classifier.
  - [x] Hardened the shared browser allocator with crash-reporting disable flags.
  - [x] Added an explicit `about:blank` startup probe in `newBrowserTestContext` and skip browser tests only when Chrome cannot start in the runner environment.
  - [x] Revalidated the previously failing protocol/sync browser test plus the full `make format`, `make test`, `make lint`, and `make ci` sequence.

- Update on 2026-03-08 for web audit runner UX.
  Fixed a mismatch between the main Run button and the action-capable audit table.
  ### Summary
  The web client exposed two audit render paths: the Audit task’s typed inspection flow, which supports row actions, and the generic runner flow, which only parsed stdout into a read-only table. Operators could therefore click the main Run button while `gix audit` was selected, see an audit table, and still have no remediation actions available.

  ### Analysis
  - This was a real product bug, not just a discoverability issue. The initial command selection is `gix audit`, and the main runner affordance remained active, so the UI naturally suggested that running audit from there should produce the same actionable table as the task panel.
  - The generic runner path cannot support queued remediation safely because parsed audit stdout does not carry the typed row contract the browser uses for path-based actions.
  - Leaving both paths active meant the same visible command produced two materially different audit experiences, one actionable and one read-only, depending only on which button the user clicked.
  - The correct fix is to route `gix audit` through the typed inspection API regardless of whether the user clicks the Audit task button or the main Run button, and to label the button accordingly.

  ### Deliverables
  - [x] Added a browser regression that selects `gix audit`, clicks the main Run button, and asserts the resulting table includes row-action controls.
  - [x] Routed main-button audit execution through the typed inspection flow instead of the generic run API.
  - [x] Updated the main button label to `Inspect audit table` when `gix audit` is the selected command so the UX matches the actual behavior.

- Update on 2026-03-08 for audit root selection from the editable draft.
  Fixed a mismatch between the editable `gix audit` argument draft and the actual inspection scope used by the Audit task.
  ### Summary
  The web UI allowed operators to edit the audit draft in the arguments textarea, but `Inspect audit table` still read only the dedicated Audit controls. Changing `--roots` in the visible draft therefore did not change the inspected folder, which made the UI show one audit command and execute another.

  ### Analysis
  - This was a state divergence bug inside the browser client. The audit task generated a draft command from `audit-roots-input` and `audit-include-all`, but manual edits to the shared arguments editor were not reflected back into those controls.
  - Once the main Run button was routed through typed audit inspection, this mismatch became more visible because the web interface effectively had two editable audit surfaces pointing at one operation.
  - The correct fix is to treat the supported typed audit flags in the draft (`--roots` and `--all`) as another valid edit surface for the Audit task and resolve inspection requests from that parsed draft when available.
  - Unsupported or incomplete draft arguments should not clobber the task controls; the parser therefore only synchronizes when it can read a coherent audit request.

  ### Deliverables
  - [x] Added a browser regression that edits `gix audit` arguments directly, clicks `Inspect audit table`, and verifies the inspected root matches the edited `--roots` value.
  - [x] Added browser-side parsing for `gix audit` draft arguments covering `--roots` and `--all`.
  - [x] Synchronized parsed audit draft arguments back into the Audit task controls so the visible task state and the draft command stay aligned.
  - [x] Resolved typed audit inspections from the parsed draft request when available, eliminating the scope mismatch between the draft editor and the Inspect action.


## Planning
*do not implement yet*
