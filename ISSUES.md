# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

## Features (110–199)

## Improvements (235–299)

## BugFixes (330–399)

- [ ] [GX-330] the append-if-missing doesnt work. It only appends the first line and skips the rest. so, if a file doesnt have any of the lines we want to add, only the first line will be added.

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
## Maintenance (410–499)

- [ ] [GX-412] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md


## Planning 
do not work on the issues below, not ready

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails

