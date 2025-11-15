# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

## Features (110–199)

## Improvements (235–299)
- [x] [GX-333] Rethink human-readable workflow logging: collapse repetitive `TASK_PLAN/TASK_APPLY` spam into concise task summaries, retain only essential branch/PR status lines, and surface warnings/errors in a structured “issues” section so the log is useful at a glance.
- [ ] [GX-336] Parallelize workflow runner so repository-scoped operations are queued and processed concurrently (e.g., up to 10 repos at a time) instead of strictly sequential; enumerate roots up front, build a task queue, and stream results while respecting per-repo isolation and existing safeguards.

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

## Maintenance (410–499)

- [ ] [GX-412] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md


## Planning 
do not work on the issues below, not ready

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
