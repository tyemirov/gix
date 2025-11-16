# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

## Features (110–199)

## Improvements (235–299)
- [x] [GX-333] Rethink human-readable workflow logging: collapse repetitive `TASK_PLAN/TASK_APPLY` spam into concise task summaries, retain only essential branch/PR status lines, and surface warnings/errors in a structured “issues” section so the log is useful at a glance.
- [x] [GX-336] Parallelize workflow runner so repository-scoped operations are queued and processed concurrently (e.g., up to 10 repos at a time) instead of strictly sequential; enumerate roots up front, build a task queue, and stream results while respecting per-repo isolation and existing safeguards. — Repository-scoped workflow stages now execute through a configurable worker pool (`--workflow-workers`/`workflow_workers`) so operators can opt into parallelism; global steps still run once with ordered stage summaries preserved.
- [x] [GX-337] Convert `repo-folders-rename` into an embedded workflow preset: encode the current task definition as YAML, teach the CLI command to translate flags/config into workflow variables, and execute via the workflow runtime instead of hand-rolled task runner wiring. — Added `folder-rename` preset plus CLI shim so the command now loads the preset, maps flags to workflow variables, and delegates execution to the workflow runtime.
- [ ] [GX-338] Convert `repo-remote-update` (canonical remotes) into a workflow preset/CLI shim so owner constraints, prompts, and logging flow entirely through the workflow executor.
- [ ] [GX-339] Convert `repo-protocol-convert` into a workflow preset that validates `from`/`to` in the CLI layer, pushes options via variables, and delegates execution to workflow operations.
- [ ] [GX-340] Convert `repo-history-remove` into a preset-driven workflow step covering path lists, remote/push/restore flags, and ensure the CLI simply maps arguments to preset variables.
- [ ] [GX-341] Convert `repo-files-add` into a workflow preset (with variables for path/content/mode/branch/push). Update the CLI to load template content and pass it into workflow variables before executing.
- [ ] [GX-342] Convert `repo release`/`repo release retag` commands into workflow presets so tagging logic, remote selection, and messages flow through the standard workflow executor and task actions.
- [ ] [GX-343] After the command-specific presets land, delete the bespoke task-runner plumbing in `cmd/cli/repos` (helpers, dependency builders, TaskDefinition construction) so repo commands are thin shims over workflow presets, and update docs/config to reflect the new preset catalog.
- [ ] [GX-344] Convert `repo-files-replace` into a workflow preset so pattern/find/replace/command/safeguard logic is expressed declaratively and the CLI simply maps flags to workflow variables before invoking the standard executor.
- [ ] [GX-346] Align AGENTS*/NOTES/POLICY docs with the latest process guidance copied from the upstream playbook so contributors see consistent repo-wide instructions.

- [ ] [GX-345] Split safeguards into hard-stop (abort entire repository execution immediately on failure) and soft-skip (mark operation as skipped but allow other steps to proceed) categories so the DSL clearly expresses whether a violation halts the repo or just the current step; apply this separation to dirty worktree vs. missing remote scenarios.
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
- [ ] [GX-336] Workflow logging still feels repetetive/confusing (branch change prints both `↪ switched` and `✓ Switch...`). Need redesigned human-readable format: single header per repo with path, grouped phase bullets (remote/folder, branch, file edits, git actions, PR), concise branch transition line, and clear warning/error markers. Update README docs once implemented.

## Maintenance (410–499)

- [ ] [GX-412] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md


## Planning 
do not work on the issues below, not ready

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
