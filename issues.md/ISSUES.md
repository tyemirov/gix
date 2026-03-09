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

- Update on 2026-03-08 for audit dirty-files column layout.
  Adjusted the audit table so the `Dirty Files` column stays narrow and wraps long file lists inside the cell instead of expanding the whole table width.
  ### Summary
  The typed audit table rendered `dirty_files` as an unconstrained content-width column, which let long file lists stretch the table horizontally and made the row harder to scan.

  ### Analysis
  - The table renderer already knew each logical audit column name, but it did not assign column-specific classes. Styling therefore had to treat every cell the same way.
  - The `Dirty Files` column has different content characteristics from the rest of the audit data: it is often a semicolon-separated list of paths and benefits from a constrained width with aggressive wrapping.
  - A targeted column class is the lowest-risk change because it keeps the rest of the table layout unchanged while making the path-heavy column fill vertical space instead of horizontal space.

  ### Deliverables
  - [x] Added a dedicated audit-table column class for `dirty_files` in both the typed and parsed audit table renderers.
  - [x] Applied a narrow fixed width plus line wrapping and word breaking to the `Dirty Files` column.

- Update on 2026-03-08 for audit column-value filtering.
  Added per-column header filters to the web audit table so operators can narrow rows to exact values directly from the column headers.
  ### Summary
  The audit table had no interactive filtering, which made it cumbersome to isolate subsets like `name_matches = no`, `in_sync = no`, or `origin_remote_status = missing` before queueing remediation actions.

  ### Analysis
  - The typed audit table already had a stable set of logical columns, so the right place for filtering is in the header itself rather than in a separate global search surface.
  - Exact-value filters fit the audit data model well because many columns are categorical and operators typically want to isolate one specific state at a time.
  - The implementation needed to preserve the existing row-action UX, which means filtering should only change which rows are rendered and summarized, not mutate the underlying inspection rows or queue semantics.
  - Keeping filter state in the browser and re-rendering the table from the inspection rows is the lowest-risk approach because it does not alter the backend audit contract.

  ### Deliverables
  - [x] Added a browser regression that inspects multiple rows, filters the `Name Matches` column from the header, and verifies the table narrows to matching rows only.
  - [x] Added per-column header select controls for typed audit columns when more than one distinct value exists in the inspected rows.
  - [x] Added exact-value filtering and filtered row summaries (for example `1 of 2 rows`) without affecting queue actions or audit data.


## Planning
*do not implement yet*

- [x] [F009] Add a repository-tree explorer to the web interface so the left panel behaves like a filesystem explorer while only exposing Git repositories as selectable leaves.
  Requested on 2026-03-08.
  ### Summary
  The current web interface exposes repositories as a flat filtered list in the left panel. That does not scale well once the launch root contains many nested repositories, because path context is compressed into one line and operators cannot navigate by folder hierarchy. The requested UX is a Windows Explorer style tree in the left panel, but constrained to repository discovery: intermediate folders only exist to organize repositories, and only Git repositories are selectable targets.

  ### Current State
  - `GET /api/repos` already returns absolute repository paths in `RepositoryCatalog.Repositories`, so the browser has enough information to derive a folder hierarchy without a new backend endpoint.
  - The current frontend is a static HTML/CSS/ESM page (`internal/web/ui/index.html`, `internal/web/ui/assets/app.js`, `internal/web/ui/assets/styles.css`) with no package manifest, no bundler, and no frontend dependency pipeline.
  - The existing repo panel owns three coupled behaviors that must survive the tree migration: selected repository identity, checked-repository scope, and text filtering.
  - This issue explicitly chooses a runtime CDN dependency for the tree widget, so the implementation should treat jsDelivr-hosted Wunderbaum as the only supported tree-library path for this feature.

  ### Library Investigation
  - `Wunderbaum` is the chosen library for this issue. Its official quick-start supports direct jsDelivr loading for both CSS and JavaScript, and its tree model, keyboard support, filtering support, and plain-JS integration fit the current static frontend.
  - The official docs expose both a UMD script path and an ESM CDN path. Because the current page already loads `app.js` as a module, the cleanest implementation path is to keep `app.js` local and import Wunderbaum from jsDelivr into that module while loading the Wunderbaum stylesheet from jsDelivr in `index.html`.
  - Wunderbaum still documents itself as beta, so the browser-side tree adapter should isolate the rest of the control-surface code from direct widget-specific data shapes as much as practical.

  ### Recommendation
  - Use Wunderbaum from jsDelivr as the only tree library for this feature.
  - Derive a typed tree model in the browser from `RepositoryCatalog.Repositories`, then adapt that model into Wunderbaum nodes so repository semantics remain owned by the app instead of by the widget.
  - Load the Wunderbaum stylesheet from `https://cdn.jsdelivr.net/npm/wunderbaum@0/dist/wunderbaum.min.css`.
  - Load Wunderbaum JavaScript from the CDN ESM entrypoint `https://cdn.jsdelivr.net/npm/wunderbaum@0/+esm` from the local `app.js` module.

  ### Deliverables
  - [x] Replace the flat repo list in the left panel with a folder tree derived from repository paths.
  - [x] Preserve selected-repo behavior, checked-repo scope, and text filtering within the new tree UX.
  - [x] Ensure only repository leaves are selectable command targets; intermediate folders act as navigation/grouping nodes.
  - [x] Add browser coverage for expanding/collapsing folders, selecting a repository from the tree, and preserving checked scope state.
  - [x] Wire Wunderbaum from jsDelivr into the static web page and initialize it from the local browser-side tree model.
  ## Resolution
  - Resolved on 2026-03-08 by replacing the flat repository list with a Wunderbaum-backed folder tree in the web client, loading the widget stylesheet from jsDelivr in `index.html` and importing the ESM module from jsDelivr in `app.js`.
  - Added a browser-side folder/repository tree model derived from `RepositoryCatalog.Repositories`, kept repository selection and checked-scope state in the existing application model, and mapped those semantics onto Wunderbaum activation and checkbox events.
  - Preserved repository filtering and checked-scope workflows across tree updates, added local icon styling so the tree does not depend on an extra icon-font CDN, and covered the explorer behavior with browser and HTML integration tests.

- Update on 2026-03-08 for [F009].
  Tightened the repository tree so the web client only exposes top-level Git repositories as selectable targets.
  ### Summary
  The initial tree implementation used the full discovered repository catalog, so a Git repository nested inside another Git repository still appeared in the left panel and leaked into `All` scope operations. That contradicted the intended UX of operating on top-level repositories only.

  ### Analysis
  - Filtering only at the tree renderer would have been insufficient, because the rest of the web client derives counts, checked scope, and `All` scope command roots from `state.repositories`.
  - The correct boundary for the fix is therefore the browser-side repository set loaded during initialization: once nested repositories are removed there, the tree, the repo count, and all repository-scoped commands stay consistent.
  - Top-level determination is path-based: a repository is excluded when its normalized path is a strict descendant of another discovered repository path.

  ### Deliverables
  - [x] Added a browser regression covering a top-level repository, a nested child repository under it, and an unrelated sibling repository.
  - [x] Filtered the browser-side repository set down to top-level repositories before selection, counts, tree rendering, and scope resolution.
  - [x] Revalidated `make format`, `make test`, `make lint`, and `make ci`.

- Update on 2026-03-08 for [F009].
  Moved the repository explorer into a persistent left sidebar that occupies roughly one fifth of the desktop control surface and lets the folder tree fill the panel vertically.
  ### Summary
  The initial repository-tree delivery rendered the tree inside the old top-row repository card instead of as a true left explorer panel. That made the tree easy to miss, did not read as a file-explorer layout, and did not satisfy the requested 1/5-page sidebar UX.

  ### Analysis
  - The problem was structural, not data-related: the tree control already existed and rendered repository folders correctly, but it lived in the same top context row as the target cards.
  - A CSS-only tweak would not have been enough because the page DOM still treated the repository area as one card in a horizontal trio rather than as a dedicated navigation region.
  - The correct fix is to split the page into a sidebar/main workspace, keep the existing repository-state widgets inside the sidebar, and let the Wunderbaum container expand vertically inside that sidebar.
  - Desktop width should be enforced at the layout container so the explorer remains approximately 20% of the usable workspace, while mobile should still collapse to a single-column layout.

  ### Deliverables
  - [x] Added browser coverage that asserts the repository tree is rendered inside a dedicated left sidebar, appears to the left of the main workspace, and occupies approximately one fifth of the desktop layout width.
  - [x] Restructured the embedded HTML into a `workspace-layout` split with `repo-sidebar` and `workspace-main` regions.
  - [x] Updated CSS so the sidebar stays narrow on desktop, the repository tree fills the panel vertically, and mobile collapses back to one column.

- Update on 2026-03-08 for [F009].
  Fixed current-repository launch mode so the explorer no longer collapses into a visually flat single-leaf row with no visible parent folder context.
  ### Summary
  When `gix --web` was launched from inside a repository, the explorer received a one-repository catalog and rendered it as a lone leaf named after the repo. In practice that looked like a blank panel with one thin row, which did not communicate a tree structure and made the repo entry feel non-interactive.

  ### Analysis
  - The sidebar layout itself was correct after the previous fix; the remaining problem was the browser-side tree model for `current_repo` launch mode.
  - The existing segment builder collapsed a repository whose path matched the launch path down to `[repo-name]`, so Wunderbaum had no parent folder node to render.
  - In current-repo mode the right UX is to synthesize a visible parent-folder context from the repository path itself, then expand that folder by default so the repo leaf is immediately visible.
  - Small cursor affordance changes help the tree read as interactive instead of as static text.

  ### Deliverables
  - [x] Added a browser regression covering current-repo launch mode and asserting the tree shows both the parent folder name and the repository leaf.
  - [x] Updated the tree-model builder so current-repo mode renders `parent-folder -> repository` instead of a single leaf.
  - [x] Auto-expanded current-repo folders and added pointer cursors for folder and repository tree rows.

- Update on 2026-03-08 for [F009].
  Extended current-repo explorer navigation so clicking the top visible folder reveals the next ancestor above it instead of leaving the tree stuck at a single synthetic parent level.
  ### Summary
  After the first current-repo explorer fix, the tree showed one visible parent folder above the repository leaf, but operators still could not climb farther up the hierarchy from the tree itself. Clicking that folder did not reveal the next ancestor, so the explorer still felt truncated.

  ### Analysis
  - The browser-side current-repo tree model was still derived from a fixed two-segment window (`parent -> repo`), so higher ancestors were absent from the rendered node graph.
  - The requested behavior maps naturally to a progressive reveal model: keep the tree compact by default, but when the top visible folder is clicked, expand the visible window upward by one ancestor and rerender.
  - Folder clicks below the top visible node should remain normal tree interactions; only the top visible folder in current-repo mode needs to trigger upward reveal while hidden ancestors remain.

  ### Deliverables
  - [x] Updated browser coverage so current-repo mode starts with the immediate parent visible and reveals the next ancestor when that top folder is clicked.
  - [x] Replaced the fixed current-repo segment builder with a depth-based ancestor window that can grow upward on demand.

- Update on 2026-03-08 for [F009].
  Changed current-repo mode so clicking the repository leaf pivots the sidebar into a real explorer rooted at the repository parent and restyled the tree to read like a filesystem pane instead of site chrome.
  ### Summary
  Even after the ancestor reveal fix, the compact current-repo tree still behaved like a synthetic path widget rather than a file explorer. Operators expected the repository leaf click to populate the full set of Git-enabled sibling folders under the parent directory, and the themed badge-like tree styling made the widget feel unlike a normal explorer.

  ### Analysis
  - The backend catalog for `current_repo` mode only exposed the current repository before the frontend pivoted into full explorer mode, so the browser had no sibling repositories to show when the leaf was clicked.
  - The right boundary for the data fix is the initial repository catalog: when launched from inside a repo, the server should still discover all repositories beneath the current repository's parent folder while keeping the current repo selected.
  - The frontend then needs an explicit compact-vs-explorer mode: compact mode can show the path-oriented current repo context, and clicking the repo leaf should switch into full explorer mode rooted at the discovered parent.
  - The tree styling should be intentionally decoupled from the rest of the site shell so the pane reads as a neutral filesystem explorer with flat rows, standard selection colors, and folder/file-style icons.

  ### Deliverables
  - [x] Added browser coverage that clicks the current-repo leaf and verifies sibling repositories appear in the tree.
  - [x] Updated the current-repo repository catalog to preload repositories beneath the current repository parent and expose that parent as the explorer root.
  - [x] Added compact/explorer switching in the browser tree model and restyled the tree pane with a flatter file-explorer visual language.

- Update on 2026-03-08 for [F008].
  Fixed audit-table action buttons so their labels remain visible in the web UI instead of rendering as oversized blank pills.
  ### Summary
  The audit results table reused the generic `.secondary-button` styling inside the Actions column. Those buttons inherited a transparent border and full-width layout, which made the action pills look empty and pushed their labels into a visually awkward position inside the horizontally scrollable table.

  ### Analysis
  - The labels were present in the DOM; this was a presentation regression in the shared button CSS, not missing audit action data.
  - The transparent border made the controls visually weak, and the inherited `width: 100%` caused each action button to stretch across the full cell width instead of sizing to its content.
  - The correct fix is to make secondary buttons explicitly legible and override the audit-table action buttons to render as compact content-width pills with left-aligned text.

  ### Deliverables
  - [x] Added a browser regression that verifies the rename action button exposes non-transparent styling and does not consume the full width of its table cell.
  - [x] Made `.secondary-button` text and border styling explicit.
  - [x] Updated audit action pills to size to content and wrap their labels within the button.

- Update on 2026-03-08 for [F009].
  Changed compact current-repo folder clicks so selecting the current repo parent pivots into the full sibling-repository explorer instead of only revealing one higher ancestor.
  ### Summary
  In current-repo launch mode, clicking the visible parent folder above the current repository still followed the old ancestor-reveal flow. Operators expected that click to populate the tree with all Git-enabled repositories under that parent folder, matching the behavior already available on the repository leaf itself.

  ### Analysis
  - The backend catalog already contained the discovered repositories beneath `ExplorerRoot`, so the missing behavior was entirely in the browser click handler.
  - The folder node path in compact mode is relative, so the browser needs to resolve that visible folder path back to the absolute compact-tree folder path before deciding whether it represents the current explorer root.
  - Once the clicked compact folder resolves to `ExplorerRoot`, the UI should enter the same expanded explorer mode used by the repo-leaf click and reuse the preloaded sibling repository catalog.

  ### Deliverables
  - [x] Added a browser regression that clicks the visible current-repo parent folder and verifies sibling repositories appear in the tree.
  - [x] Updated compact folder clicks so the current repo parent folder expands into the full sibling-repository explorer.

- Update on 2026-03-08 for [F009].
  Fixed repository-tree sibling ordering and indent so mixed sibling sets render like a file explorer instead of grouping expandable folders first and offsetting leaf repos to the right.
  ### Summary
  Under one parent folder, the tree still sorted intermediate folders ahead of repository leaves and rendered repository titles farther to the right because repo rows carried a checkbox slot while folder rows did not. That made the tree look unlike a normal file explorer and broke the expected alphabetical sibling order.

  ### Analysis
  - The ordering bug was in the browser-side tree sort, which still preferred `folder` nodes over `repository` nodes before comparing titles.
  - The indent bug was structural in the rendered row chrome: repository rows include a checkbox slot for checked-scope selection, while folder rows only include the expander slot. Without a matching spacer, sibling titles cannot line up.
  - The correct fix is to sort sibling nodes purely by title and reserve the checkbox slot width on folder rows with a hidden spacer so the expander remains the only visible difference between expandable and non-expandable siblings.

  ### Deliverables
  - [x] Added a browser regression that expands a sibling set containing both a repository leaf and an intermediate folder, then asserts alphabetical order and matching title indent.
  - [x] Removed folder-first sorting from the repository tree model.
  - [x] Added a hidden checkbox-width spacer on folder rows so sibling titles align.

- Update on 2026-03-08 for [F009].
  Restored upward ancestor reveal after current-repo expansion so the top of the explorer remains a parent folder and can keep climbing toward the filesystem root one level at a time.
  ### Summary
  The sibling-explorer change made current-repo expansion stop at the explorer root, so after promoting the current repo parent into the tree, operators no longer saw the next higher parent folder above it. That regressed the earlier expectation that the current-repo tree can continue revealing one more parent at the top on each click.

  ### Analysis
  - The repository catalog already had enough data for sibling expansion; the missing behavior was that expanded mode rendered repositories directly under `ExplorerRoot` without wrapping them in the visible ancestor chain above that root.
  - Expanded current-repo mode therefore needs its own ancestor-depth state separate from the compact `parent -> repo` reveal depth.
  - The correct browser behavior is:
    1. compact mode starts at `parent-folder -> repo`
    2. clicking the repo leaf or current parent folder switches to sibling explorer mode
    3. sibling explorer mode renders the current parent folder beneath one visible higher ancestor when available
    4. clicking the top visible ancestor reveals the next one above it

  ### Deliverables
  - [x] Extended the browser regression so expanding the current repo parent must reveal both sibling repositories and the next higher parent folder, then allow one more upward reveal on the new top folder.
  - [x] Added expanded-mode ancestor wrapping and top-folder reveal logic for current-repo explorer mode.

- Update on 2026-03-08 for [F009].
  Simplified current-repo tree construction so compact mode and expanded sibling-explorer mode both use one ancestor-depth model anchored to a single path instead of separate compact and expanded depth implementations.
  ### Summary
  The earlier implementation worked, but it carried two different reveal counters and separate helpers for compact and expanded current-repo trees. That made a straightforward rule harder to read in code than it needed to be.

  ### Analysis
  - Both modes follow the same shape: show a visible ancestor chain above an anchor path, then append repository-relative path segments beneath that anchor.
  - In compact mode the anchor path is the selected current repository path.
  - In expanded mode the anchor path is `ExplorerRoot`.
  - A single `currentRepoAncestorDepth` plus `currentRepositoryAnchorPath()` and `currentRepositoryVisibleAncestorSegments()` is enough to express both flows.

  ### Deliverables
  - [x] Collapsed the separate compact and expanded current-repo depth logic into one shared ancestor-depth model in the browser tree renderer.
  - [x] Preserved the existing browser regressions for repo-leaf expansion, parent-folder expansion, alphabetical sibling order, and upward ancestor reveal.

- Update on 2026-03-08 for [F009].
  Added folder-to-audit-root selection in the repository tree so clicking a folder writes that folder path into the audit draft, and Cmd-click appends more folder roots.
  ### Summary
  Operators wanted the tree to serve as the source for audit scope selection. Clicking a folder should make that folder the audit root, and Cmd-clicking additional folders should accumulate multiple roots without leaving the tree.

  ### Analysis
  - The existing audit draft plumbing already rebuilds the `gix audit` arguments from `#audit-roots-input`, so the cleanest implementation is to reuse that input rather than introduce separate tree-selection state.
  - Folder tree nodes therefore need their absolute filesystem path available in the browser, not just the relative visible path segments used for rendering.
  - Once folder nodes expose their absolute path, a container-level click handler can update the audit roots field directly while preserving the tree’s existing expand/reveal behavior.
  - The audit roots parser should accept both commas and newlines so tree-driven multi-selection and manual edits share one format.

  ### Deliverables
  - [x] Added browser coverage for plain folder click replacing the audit root and Cmd-click appending another folder root.
  - [x] Added absolute folder paths to browser tree nodes.
  - [x] Reused the audit roots input as the single source of truth, with comma-or-newline parsing and comma-separated formatting.

- Update on 2026-03-09 for [F008].
  Fixed web audit protocol changes for GitHub SCP-style remotes so `git@github.com:owner/repo.git` is treated as SSH instead of a separate `git` protocol, and SSH conversions now write the standard `git@github.com:` form instead of `ssh://git@github.com/...`.
  ### Summary
  The web audit flow exposed a misleading protocol fix on repositories whose `origin` already used the common SSH shorthand form `git@github.com:owner/repo.git`. Audit reported that transport as `git`, the web queue offered a follow-up change to `ssh`, and applying it rewrote the remote to `ssh://git@github.com/owner/repo.git`. From the operator’s perspective that looked like the protocol change flow was not behaving properly.

  ### Analysis
  - The bug was semantic, not transport-level: the code treated GitHub SCP-style SSH URLs and `ssh://git@github.com/...` URLs as two distinct protocols.
  - That split leaked through three layers:
    - audit classified `git@github.com:` as `git`
    - workflow protocol conversion detected `git@github.com:` as `git`
    - remote builders emitted `ssh://git@github.com/...` for the `ssh` target
  - The correct behavior for GitHub remotes is to treat the SCP-style `git@github.com:` form as the SSH transport and present only `ssh` and `https` in the web UX.
  - The existing `git` token is preserved only as a backward-compatible input alias so older CLI/config values still parse, but it is normalized to SSH behavior.

  ### Deliverables
  - [x] Added failing expectations that `git@github.com:` audits as `ssh` and SSH conversions materialize as `git@github.com:owner/repo.git`.
  - [x] Normalized the shared/web/workflow protocol handling so the legacy `git` label aliases to SSH behavior instead of remaining a separate user-visible transport.
  - [x] Reduced the web protocol picker to the two meaningful user-facing choices: `ssh` and `https`.
