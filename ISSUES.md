# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

## Features (110–199)

## Improvements (235–299)

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
## Maintenance (410–499)

- [ ] [GX-412] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md


## Planning 
do not work on the issues below, not ready

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
