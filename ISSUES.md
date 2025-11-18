# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.GIT.md @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes -` [x] [GX-<number>]`

## Features (110–199)

## Improvements (235–299)
- [x] [GX-333] Rethink human-readable workflow logging: collapse repetitive `TASK_PLAN/TASK_APPLY` spam into concise task summaries, retain only essential branch/PR status lines, and surface warnings/errors in a structured “issues” section so the log is useful at a glance.
- [x] [GX-336] Parallelize workflow runner so repository-scoped operations are queued and processed concurrently (e.g., up to 10 repos at a time) instead of strictly sequential; enumerate roots up front, build a task queue, and stream results while respecting per-repo isolation and existing safeguards. — Repository-scoped workflow stages now execute through a configurable worker pool (`--workflow-workers`/`workflow_workers`) so operators can opt into parallelism; global steps still run once with ordered stage summaries preserved.
- [x] [GX-337] Convert `repo-folders-rename` into an embedded workflow preset: encode the current task definition as YAML, teach the CLI command to translate flags/config into workflow variables, and execute via the workflow runtime instead of hand-rolled task runner wiring. — Added `folder-rename` preset plus CLI shim so the command now loads the preset, maps flags to workflow variables, and delegates execution to the workflow runtime.
- [x] [GX-338] Convert `repo-remote-update` (canonical remotes) into a workflow preset/CLI shim so owner constraints, prompts, and logging flow entirely through the workflow executor. — Added `remote-update-to-canonical` embedded preset plus CLI wiring so the command now loads the preset, injects owner preferences, and runs through the workflow executor.
- [x] [GX-339] Convert `repo-protocol-convert` into a workflow preset that validates `from`/`to` in the CLI layer, pushes options via variables, and delegates execution to workflow operations.
- [x] [GX-340] Convert `repo-history-remove` into a preset-driven workflow step covering path lists, remote/push/restore flags, and ensure the CLI simply maps arguments to preset variables.
- [x] [GX-341] Convert `repo-files-add` into a workflow preset (with variables for path/content/mode/branch/push). Update the CLI to load template content and pass it into workflow variables before executing. — Added `files-add` preset plus CLI shim so the command resolves path/content/branch settings, injects them into the preset, and executes via the workflow runtime.
- [x] [GX-342] Convert `repo release`/`repo release retag` commands into workflow presets so tagging logic, remote selection, and messages flow through the standard workflow executor and task actions.
- [x] [GX-343] After the command-specific presets land, delete the bespoke task-runner plumbing in `cmd/cli/repos` (helpers, dependency builders, TaskDefinition construction) so repo commands are thin shims over workflow presets, and update docs/config to reflect the new preset catalog. — Removed repo helpers in favor of the shared workflow executor wiring plus updated commands/tests to consume the workflow-layer factories.
- [x] [GX-344] Convert `repo-files-replace` into a workflow preset so pattern/find/replace/command/safeguard logic is expressed declaratively and the CLI simply maps flags to workflow variables before invoking the standard executor. — Added `files-replace` preset plus CLI shim so the command now loads the preset, injects pattern/find/command/safeguard options, and executes via the workflow runtime with updated tests/docs.

- [x] [GX-345] Split safeguards into hard-stop (abort entire repository execution immediately on failure) and soft-skip (mark operation as skipped but allow other steps to proceed) categories so the DSL clearly expresses whether a violation halts the repo or just the current step; apply this separation to dirty worktree vs. missing remote scenarios. — Added structured `hard_stop`/`soft_skip` safeguard blocks, updated evaluators/CLI configs/tests, and ensured dirty worktree guards abort while soft skips (missing files/branch) only skip the current task.
## BugFixes (330–399)

- [x] [GX-330] the append-if-missing doesnt work. It only appends the first line and skips the rest. so, if a file doesnt have any of the lines we want to add, only the first line will be added. — Fixed by normalizing CR-only line endings before parsing so multi-line templates append every line; added regression tests for CR content.

```yaml
  - step:
      name: gitignore-apply
      after: ["gitignore-branch"]
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: "Ensure gitignore entries"
            safeguards:
              paths:
                - ".gitignore"
            steps:
              - files.apply
            files:
              - path: .gitignore
                content: |
                  # Managed by gix gitignore workflow
                  .env
                  tools/
                  bin/
                mode: append-if-missing
```
- [x] [GX-331] Workflow execution does not halt after a repository-scoped step emits `TASK_SKIP` (for example, when the `newCleanWorktreeGuard` rejects a dirty worktree), so subsequent steps like `git stage-commit` still run and fail even though the repository should have been skipped entirely. — Introduced a repository-skip sentinel error, taught the executor to stop additional operations when it appears, and added regression coverage to ensure later steps never run on skipped repositories.
- [x] [GX-332] Workflow executor logs every repository-scoped stage (e.g., `stage 1 … switch-master`), leaking implementation detail; only the final summary should remain visible. — Removed the per-stage zap logging and CLI post-run dump so only the reporter’s summary remains.
- [x] [GX-333] Rethink human-readable workflow logging: collapse repetitive `TASK_PLAN/TASK_APPLY` spam into concise task summaries, retain only essential branch/PR status lines, and surface warnings/errors in a structured “issues” section so the log is useful at a glance. — Added a workflow-specific event formatter that groups logs per repository, prints single-line task results, and highlights warnings/errors without overwhelming noise.
- [x] [GX-334] `branch.change` still runs `git pull --rebase` after creating a brand new local branch without a tracking remote, producing noisy `PULL-SKIP` warnings during workflows (there’s nothing to pull, so we should skip automatically). — Skip the pull step when a branch is created without remote tracking so new automation branches no longer emit useless warnings.

- [x] [GX-335] the content of the action in @configs/gitignore.yaml   says
```yaml
    content: |
                      # Managed by gix gitignore workflow
                      .env
                      tools/
                      bin/
```

    but after running the workflow the line that says `.env` never gets into the diffs (PRs). I suspect that instead of string matching for appending them, we use regex, and we shall not use regex in this case. We match on the entire line, whatever it is (probably trimming)
— Append-if-missing now compares literal line content (whitespace intact) so substrings like `.envrc` or indented variants no longer satisfy `.env`; added tests covering those scenarios.
- [x] [GX-336] Workflow logging still feels repetetive/confusing (branch change prints both `↪ switched` and `✓ Switch...`). Need redesigned human-readable format: single header per repo with path, grouped phase bullets (remote/folder, branch, file edits, git actions, PR), concise branch transition line, and clear warning/error markers. Update README docs once implemented. — Human formatter now emits bullet-grouped phases, consolidates warnings/errors under an `issues` block, and README/docs were refreshed to describe the new layout.

## Maintenance (410–499)

- [x] [GX-412] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md — Documented the refactor plan in `docs/GX-412-refactor-plan.md`, covering CLI preset helpers, typed workflow builders, domain smart constructors, taskrunner dependency injection, and missing negative-path tests.
- [x] [GX-413] Eliminate duplicate preset CLI plumbing by introducing a shared `workflowcmd.PresetCommand` helper that centralizes flag parsing, root/config validation, execution flag overrides (including `--workflow-workers`), and dependency wiring before migrating repo commands (files-add/replace/remove/etc.) plus their tests to the new abstraction so edge validation only happens once. — Added `workflowcmd.PresetCommand` with unit tests plus helper builders, then migrated files-add/replace/remove/remotes/protocol/rename/release commands to the new abstraction; existing tests were updated and `go test ./cmd/... ./pkg/taskrunner` passes.
- [x] [GX-414] Replace untyped preset mutation (current `map[string]any` hacking in repo commands) with typed workflow builder structs (e.g., `workflow.TasksApplyConfig`, file action helpers) that validate allowed combinations of patterns, safeguards, and branch blocks at construction time, deleting the bespoke `update*PresetOptions` helpers and adding serialization tests for single vs multi-pattern and safeguard permutations. — Added `workflow.TasksApplyDefinition` + builder utilities (with tests) and refactored files-add/replace/history-remove/release/retag commands to construct `TaskDefinition` values instead of mutating raw maps.
- [x] [GX-415] Add smart constructors for domain option structs in `internal/repos/history` and `internal/repos/rename` so CLI layers pass typed owner/repository identifiers and sanitized path segments instead of raw strings/booleans; delete redundant `strings.TrimSpace` validation in CLI shims and expand unit tests around invalid constructor inputs. — Added `shared.RepositoryPathSegment`, history/rename option builders, CLI/workflow wiring that now builds typed inputs, plus new tests covering invalid segments and folder names.
- [ ] [GX-416] Refactor `pkg/taskrunner/dependencies.go` into composable builders (`NewGitExecutionEnvironment`, `NewRepositoryEnvironment`, `NewPromptEnvironment`) that fail fast when dependencies are missing, honor `SkipGitHubResolver` without instantiating clients, and inject IO/loggers explicitly; update workflow + CLI builders plus resolver tests accordingly.
- [ ] [GX-417] Backfill CLI negative-path tests for file-based commands (`cmd/cli/repos/files_add.go`, `replace.go`, `remove.go`) covering `--content-file` IO errors, conflicting `--content` vs `--content-file` flags, and permission parse failures, ensuring errors are wrapped with context and presets are not invoked when validation fails.

## Planning 
do not work on the issues below, not ready

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
