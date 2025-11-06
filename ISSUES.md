# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

## Features (110–199)

## Improvements (215–299)

- [x] [GX-215] Enable LLM actions in workflows and output piping between steps
  - Status: Resolved
  - Category: Improvement
  - Context: `apply-tasks` supports `commit.message.generate` and `changelog.message.generate` actions, but they require a programmatic client and there is no way to pass their outputs into subsequent steps (e.g., as a `commit_message` for namespace rewrite).
  - Desired: Allow configuring an LLM client in workflow YAML (model, base-url, api-key env, timeout) and introduce workflow variables so action outputs can be referenced as inputs by later steps.
  - Resolution: Workflow tasks accept an `llm` client block and LLM actions can `capture_as` variables, which become available to subsequent tasks and steps via `.Environment` templating.

- [x] [GX-216] Add details of what exactly is unclean in a directory, which files. Improve the message we are getting now with the details
  - Status: Resolved
  - Resolution: Dirty-task skip logging now includes git status entries, so the warning lists the precise files that block execution.
```
TASK-SKIP: Switch to master if clean /tmp/repos/tyemirov/GAuss repository not clean
TASK-SKIP: Rewrite module namespace /tmp/repos/tyemirov/GAuss repository dirty
```

- [x] [GX-217] Erroneous summary message: total.repos=0, should be total.repos=1, and it also must have taken some time, so the executor shall be returning time that we shall be printing in human readeable foramt.
  - Status: Resolved
  - Resolution: StructuredReporter now falls back to repository paths when identifiers are missing and adds a `duration_human` field alongside milliseconds, producing accurate counts and elapsed time.
RELEASED: /Users/tyemirov/Development/tyemirov/gix -> v0.2.0-rc.6
Summary: total.repos=0 duration_ms=0

- [x] [GX-218] Remove the top level commands `repository` and `branch` and only use their subcommands.
  - Status: Resolved
  - Resolution: CLI namespaces now expose the former `repo` and `branch` subcommands as first-class operations (`folder rename`, `remote update-*`, `branch-*`, etc.), legacy configuration keys are normalized to the new command paths, workflow builders accept both canonical and legacy keys, and documentation plus configuration samples were updated accordingly.

- [ ] [GX-219] Remove the `--dry-run` flag and all associated logic, it gurantees nothing.

- [x] [GX-220] Rename branch-cd CLI surface to cd with default branch fallback
  - Status: Resolved
  - Resolution: The CLI now exposes `gix cd` as the canonical entry point (with `branch-cd` logged as a deprecated alias), configuration loaders emit warnings for legacy keys, default configuration/docs/tests reference `cd`, and branch switching falls back to the repository default or configured branch when no argument is provided.
  - Category: Improvement
  - Context: `cmd/cli/application.go` registers the branch switching command under the `branch-cd` verb, and `internal/branches/cd`, `cmd/cli/default_config.yaml`, `docs/command_warning_matrix.md`, `docs/readme_config_test.go`, `tests/no_remote_integration_test.go`, and workflow command keys all depend on that name while forcing every invocation to supply an explicit branch.
  - Desired: Expose the command as `gix cd` while preserving the existing task runner wiring, make the branch argument optional by defaulting to the repository default (falling back to the configured `branch` in command settings when discovery fails), update configuration keys and docs/tests to use `cd`, and keep legacy `branch-cd` config entries runnable with a deprecation warning.
  - Acceptance: `gix cd` without arguments checks out each repository's default branch or configured fallback, positional branch arguments still work, CLI help/docs/config samples mention `cd`, workflow builders resolve both `cd` and `branch-cd` keys with a warning, and branch switching integration tests cover the implicit-argument path.

- [x] [GX-221] Fold branch-refresh behaviors into cd and retire the standalone command
  - Status: Resolved
  - Resolution: `gix cd` exposes `--refresh`, `--stash`, and `--commit` flags that delegate to the former branch refresh workflow, the standalone CLI was removed, default configs/docs/tests now reference `cd`, and legacy `branch-refresh` configuration keys trigger deprecation warnings while mapping to the new options.
  - Category: Improvement
  - Context: `internal/branches/refresh` wires the `branch-refresh` Cobra command with stash/commit recovery flags that overlap with the branch switcher, while `cmd/cli/default_config.yaml`, workflow builders, and docs surface it as a separate entry point.
  - Desired: Extend `gix cd` with options covering fetch/pull plus stash/commit recovery, run the existing `branch.refresh` workflow action from the unified command, and remove `branch-refresh` from the CLI/config/docs while maintaining legacy key compatibility via warnings.
  - Acceptance: `branch-refresh` disappears from `gix --help` and default configs, `gix cd --branch <name>` with the new refresh flags executes the same workflow path as today's command, updated tests cover refresh scenarios under `cd`, and legacy configs referencing `branch-refresh` are mapped with a migration warning.

- [ ] [GX-222] Rename branch-default to default and update dependent workflow plumbing
  - Status: Unresolved
  - Category: Improvement
  - Context: Default branch promotion currently lives under the `branch-default` command path in `cmd/cli/application.go`, configuration fixtures, workflow command keys, and documentation.
  - Desired: Switch the Cobra use string and registration to `default`, propagate the rename through `internal/workflow/command_path.go`, `cmd/cli/default_config.yaml`, docs, and tests, and provide aliasing so existing `branch-default` configuration entries continue to execute while warning users.
  - Acceptance: `gix default` promotes repository defaults end-to-end, help text/docs/config samples use the new name, automated tests are updated, and workflow/config loaders accept `branch-default` with a deprecation notice.

- [ ] [GX-223] Introduce message namespace and migrate changelog message to message changelog
  - Status: Unresolved
  - Category: Improvement
  - Context: The changelog LLM integration is exposed as `gix changelog message`, with configuration/tests anchored to the `changelog` namespace.
  - Desired: Register a top-level `message` command, mount the existing changelog message builder under it (`gix message changelog`), adjust configuration keys, workflow command paths, docs, and tests, and ensure legacy `changelog message` entries still resolve with a warning.
  - Acceptance: `gix message changelog` produces identical output, CLI help/docs/default config reference the new path, integration/unit tests align with the renamed command, and legacy config entries remain functional with migration guidance.

- [ ] [GX-224] Move commit message generator under message commit
  - Status: Unresolved
  - Category: Improvement
  - Context: Commit message generation lives under `gix commit message`, mirroring the changelog structure slated for relocation.
  - Desired: Rehome the Cobra registration for the commit message command beneath the `message` namespace, align configuration defaults, docs, workflow keys, and tests to `gix message commit`, and alias `commit message` with a deprecation warning.
  - Acceptance: `gix message commit` works end-to-end across CLI/config/workflow/test paths, help/docs/default config reflect the new command, and legacy `commit message` entries execute with a warning.

- [ ] [GX-225] Replace repo-license-apply CLI with embedded workflow license
  - Status: Unresolved
  - Category: Improvement
  - Dependencies: Blocked by [GX-228]
  - Context: License distribution currently depends on the standalone `repo-license-apply` command (`cmd/cli/repos/license.go`) plus associated config and tests.
  - Desired: Encode the license distribution steps as an embedded workflow, expose it via the enhanced workflow command (e.g., `gix workflow license`), remove the direct CLI entry, and update docs/config/tests while mapping legacy command usage to the workflow with warnings.
  - Acceptance: Invoking the builtin workflow performs the same operations as the former command, new docs/config samples highlight the workflow, automated coverage exercises the workflow path, and legacy command/config paths delegate to the workflow with migration guidance.

- [ ] [GX-226] Replace namespace CLI command with embedded workflow namespace
  - Status: Unresolved
  - Category: Improvement
  - Dependencies: Blocked by [GX-228]
  - Context: Namespace rewrites rely on the standalone `namespace` command (`cmd/cli/repos/namespace.go`) referenced throughout config, docs, and tests.
  - Desired: Move the namespace rewrite tasks into an embedded workflow surfaced through the workflow command (e.g., `gix workflow namespace`), retire the direct CLI command, and update docs/config/tests while mapping legacy usage to the workflow with warnings.
  - Acceptance: The builtin workflow reproduces the namespace rewrite behavior (including task runner options and reporting), documentation and configuration samples reference the workflow, automated coverage exercises the new path, and legacy command/config invocations delegate with migration guidance.

- [ ] [GX-227] Nest repo-history-remove under files rm
  - Status: Unresolved
  - Category: Improvement
  - Context: History rewriting currently uses the top-level `rm` command bound to `repo-history-remove` in `cmd/cli/repos/remove.go`, and the name appears across configs/docs/tests.
  - Desired: Introduce a `files` namespace that hosts the history removal command as `gix files rm`, propagate the rename through configuration/workflow mappings/docs/tests, and alias `rm` with a warning for existing configs.
  - Acceptance: `gix files rm` executes the same task runner path as today's command, CLI help/docs/default config show the nested path, automated tests updated, and legacy `rm` entries map to the new command with migration guidance.

- [ ] [GX-228] Extend workflow command to support invoking embedded workflows by name
  - Status: Unresolved
  - Category: Improvement
  - Context: `cmd/cli/workflow/run.go` only accepts external YAML/JSON paths and cannot surface bundled presets, yet GX-323 asks for predefined workflows (license, namespace, etc.).
  - Desired: Embed a catalog of workflow definitions in the binary, extend `gix workflow` to list and run presets (alongside existing file-based execution), document the behavior, and cover it with tests so downstream issues (GX-225, GX-226) can rely on the feature.
  - Acceptance: Users can discover and invoke built-in workflows without supplying files, legacy file-based execution continues to function, docs showcase both modes, and tests exercise preset selection plus backward compatibility.

- [ ] [GX-229] Modularize CLI bootstrap and shared task runner wiring
  - Status: Unresolved
  - Category: Improvement
  - Context: `cmd/cli/application.go` (~1.6k LOC) interleaves configuration loading, command registration, embedded default config management, and dependency wiring for subcommands, creating hard-to-test seams and duplicated `TaskRunnerFactory` setup across `cmd/cli/changelog`, `cmd/cli/commit`, and `cmd/cli/workflow`.
  - Desired: Extract bootstrap logic into a dedicated package (e.g., `cmd/cli/bootstrap`), relocate embedded default configuration helpers into their own module with invariants tests, and introduce a shared helper that constructs task runner dependencies consumed by changelog/commit/workflow builders while centralizing alias handling.
  - Acceptance: `cmd/cli/application.go` delegates to smaller builders, embedded config helpers live outside the application root with updated tests, subcommands reuse a single task runner wiring helper, and new unit tests verify root command wiring plus legacy alias coverage.

- [ ] [GX-230] Refactor workflow executor into planner, runner, and reporting units
  - Status: Unresolved
  - Category: Improvement
  - Context: `internal/workflow/executor.go` still couples planning, execution, reporting, and error formatting despite GX-322 improvements, lacks table-driven coverage for mixed outcomes, and makes it difficult to extend context/telemetry handling.
  - Desired: Split the executor into focused files (planner, runner, reporting), introduce an `ExecutionOutcome` aggregate returned to callers, add stage-level metrics/events to the reporter hooks, and expand tests to cover mixed success/failure runs, nested repository ordering, prompt state transitions, and reporter count accuracy.
  - Acceptance: Executor package exposes modular components with an `ExecutionOutcome` result, CLI layers consume the new return value, instrumentation emits stage completion events, and new tests exercise the scenarios outlined in `docs/refactor_plan_GX-411.md`.

- [ ] [GX-231] Layer workflow task operations into parse/plan/execute packages with structured reporting
  - Status: Unresolved
  - Category: Improvement
  - Dependencies: Blocked by [GX-230]
  - Context: `internal/workflow/operations_tasks.go` (~1.3k LOC) combines parsing, templating, execution, LLM wiring, and GitHub interactions while emitting direct `fmt.Fprintf` logs, leaving execution paths under-tested.
  - Desired: Break the task operations into cohesive subpackages (parse, plan, execute, actions) with explicit dependency injection, introduce strategy types for branch and PR management to enable deterministic tests, migrate user-facing output to the structured reporter, and add integration-style tests for ensure-clean failures, branch reuse, PR errors, safeguard checks, and LLM `capture_as` flows.
  - Acceptance: Task operations are distributed across new packages with clear interfaces, structured reporter events replace direct `fmt.Fprintf` usage, strategy abstractions allow targeted unit tests, and the new test suite covers the execution scenarios listed above.

- [ ] [GX-232] Centralize LLM client factory and harden generator resiliency
  - Status: Unresolved
  - Category: Improvement
  - Context: Changelog and commit message generators duplicate client validation logic, rely on happy-path test doubles, and lack retry/backoff behavior or guards against empty responses/timeouts.
  - Desired: Introduce a shared LLM factory in `pkg/llm` with injectable HTTP client/timeout support, enforce validation for base URL/model/api key, implement configurable retry/backoff respecting context cancellation, and expand changelog/commit generator tests with table-driven cases covering empty diffs, no commits, API errors, and empty LLM responses.
  - Acceptance: Both generators consume the shared factory, retry/backoff policies are configurable and exercised in tests, and new unit tests validate error handling for the edge cases described in `docs/refactor_plan_GX-411.md`.

- [ ] [GX-233] Expand structured reporter API for event counts and telemetry export
  - Status: Unresolved
  - Category: Improvement
  - Context: `internal/repos/shared/reporting.go` only aggregates totals per severity and lacks simple helpers for emitting event counters or exporting telemetry objects usable by the CLI/metrics pipelines.
  - Desired: Add an ergonomic `RecordEvent(code, level)` helper, expose a serializable summary struct for CLI output, wire reporter instrumentation to surface timings per operation, and benchmark the reporter to quantify overhead after the enhancements.
  - Acceptance: Commands adopt the new helper instead of manual `Report` calls, CLI layers consume the summary struct, timing data is available for future telemetry integrations, and benchmarks demonstrate acceptable overhead.

- [ ] [GX-234] Split fast and slow test targets to improve feedback loop
  - Status: Unresolved
  - Category: Improvement
  - Context: `go test ./...` approaches the 45s harness limit due to integration suites that spawn real Git repositories and redundant fixtures, reducing developer feedback speed.
  - Desired: Introduce Makefile or script targets that separate fast/unit tests from slow/integration suites, refactor shared Git fixtures into reusable helpers to cut setup time, and document the workflow so CI and developers can run the appropriate subsets.
  - Acceptance: New fast/slow targets exist with documentation, integration fixtures are consolidated into helpers reducing runtime, and CI guidance reflects the updated test workflow.

## BugFixes (320–399)

- [x] [GX-320] The message "namespace rewrite skipped: files ignored by git" doesnt make much sense. Must be a bug. Investigate the real reason the operation hasn't been performed.
  - Status: Resolved
  - Resolution: Namespace rewrite skips now report the actual ignored paths (for example `namespace rewrite skipped: all matching files ignored by git (go.mod, vendor/pkg.go)`), while the "no references" case keeps its dedicated message. Updated unit tests lock in the richer diagnostics.
```shell
-- repo: tyemirov/ctx ----------------------------------------------------------
22:34:53 INFO  REMOTE_SKIP        tyemirov/ctx                       already canonical                        | event=REMOTE_SKIP path=/tmp/repos/ctx reason=already_canonical repo=tyemirov/ctx
22:34:55 INFO  REPO_FOLDER_RENAME                                    /tmp/repos/ctx → /tmp/repos/tyemirov/ctx | event=REPO_FOLDER_RENAME new_path=/tmp/repos/tyemirov/ctx old_path=/tmp/repos/ctx path=/tmp/repos/ctx
22:35:01 INFO  REPO_SWITCHED      tyemirov/ctx                       → master                                 | branch=master created=false event=REPO_SWITCHED path=/tmp/repos/tyemirov/ctx repo=tyemirov/ctx
22:35:01 INFO  NAMESPACE_NOOP     tyemirov/ctx                       namespace rewrite skipped: files ignored by git | event=NAMESPACE_NOOP path=/tmp/repos/tyemirov/ctx reason=namespace_rewrite_skipped:_files_ignored_by_git repo=tyemirov/ctx
```

- [x] [GX-321] the changelog command produces a summary: "Summary: total.repos=0 duration_ms=0". Obviously, this is wrong -- there was a repo it analyzed and it tooks some time.
  - Status: Resolved
  - Resolution: Workflow execution now pre-registers repositories with the structured reporter, so summary output reflects the actual repository count and elapsed duration even when operations emit no events. Added reporter and executor tests to guarantee the behaviour.

- [x] [GX-322] Investigate the reason of the workflow exit. Ensure it is impossible for a workflow to exit, only the tasks can report the error conditions but a workflow will always finish successfully even if with an error status
  - Status: Resolved
  - Resolution: Workflow executor now records failures without cancelling subsequent operations, accumulates errors for reporting, and task execution continues across repositories. New tests cover mixed success/failure runs to prove the workflow completes and summaries still emit.
```shell
17:28:34 tyemirov@Vadyms-MacBook-Pro:~/Development/tyemirov/gix - [improvement/GX-212-summary-warnings] $ go run ./... b default master -
-roots /tmp/repos/
WORKFLOW-DEFAULT-SKIP: /tmp/repos/Research/MarcoPoloResearchLab/skazka already defaults to master
WORKFLOW-DEFAULT-SKIP: /tmp/repos/Research/apache/superset already defaults to master
WORKFLOW-DEFAULT-SKIP: /tmp/repos/Research/tyemirov/git_retag already defaults to master
WORKFLOW-DEFAULT: /tmp/repos/Research/tyemirov/licenser (master → master) safe_to_delete=false
PAGES-SKIP: tyemirov/licenser (gh: Not Found (HTTP 404))
PROTECTION-SKIP: gh: Upgrade to GitHub Pro or make this repository public to enable this feature. (HTTP 403)
WORKFLOW-DEFAULT-SKIP: /tmp/repos/Research/tyemirov/prep-yaml already defaults to master
WORKFLOW-DEFAULT: /tmp/repos/pinguin (master → master) safe_to_delete=false
PAGES-SKIP: tyemirov/pinguin (gh: Not Found (HTTP 404))
WORKFLOW-DEFAULT-SKIP: /tmp/repos/tyemirov/ETS already defaults to master
WORKFLOW-DEFAULT-SKIP: /tmp/repos/tyemirov/GAuss already defaults to master
WORKFLOW-DEFAULT-SKIP: /tmp/repos/tyemirov/MediaOps already defaults to master
WORKFLOW-DEFAULT-SKIP: /tmp/repos/tyemirov/Obsidian-santizer already defaults to master
WORKFLOW-DEFAULT-SKIP: /tmp/repos/tyemirov/SummerCamp24 already defaults to master
repo tasks apply: default branch checkout failed: CheckoutBranch operation failed: git command exited with code 1
exit status 1
```

- [x] [GX-323] Rename the commands:
branch-cd      => rename to cd. add a default argument being master (or whatever the default branch is)
branch-default => rename to default
branch-refresh => non needed, the cd command shall be able to handle it passing the name of the branch a user is on
changelog      => message changelog
commit         => message commit
license        => not needed as a separate command but must be a part of a default embedded config.yaml that is invokabl with workflow, e.g. w license
namespace     => not needed as a separate command but must be a part of a default embedded config.yaml that is invokabl with workflow, e.g. w namespace
rm             => should be a subcommand of files
workflow       => shall allow invoking various predefined workflows or specify the new ones. 

Let's consider each rename as a separate issue and what consequences it entails

## Maintenance (410–499)

- [x] [GX-411] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md
  - Status: Resolved
  - Resolution: Documented a multi-stage refactor/test roadmap (`docs/refactor_plan_GX-411.md`) covering CLI composition, workflow/task execution, LLM integrations, reporting, and testing gaps, aligned with POLICY confidences. The plan seeds follow-up issues for concrete implementation work.

## Planning 
do not work on the issues below, not ready
