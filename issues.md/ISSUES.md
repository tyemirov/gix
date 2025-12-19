# ISSUES
**Active backlog (open issues only)**

Entries record newly discovered requests or changes, with their outcomes.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes -` [x] [GX-<number>]`. Resolved issues are archived in `issues.md/ARCHIVE.md`.

Read @AGENTS.md, @README.md and ARCHITECTURE.md. Read issues.md/@POLICY.md, issues.md/PLANNING.md, issues.md/@NOTES.md, and issues.md/@ISSUES.md. Start working on open issues. Prioritize bugfixes and maintenance. Work autonomously and stack up PRs. 

Issue IDs in Features, Improvements, BugFixes, and Maintenance never reuse completed numbers; cleanup renumbers remaining entries so numbering stays monotonic.

## Features (110–199)

## Improvements (251–299)

## BugFixes (340–399)

- [ ] [GX-345] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step. (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)

## Maintenance (422–499)

## Planning
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
