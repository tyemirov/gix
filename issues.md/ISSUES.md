# ISSUES
**Active backlog (open issues only)**

Resolved issues are archived in `issues.md/ARCHIVE.md`.

Each issue is formatted as `- [ ] [GX-<number>]`.

## Features (110–199)

- [x] [GX-110] Add a step that allows running an arbitrary command, such as `go get -u ./...` and `go mod tidy`. (Added `command run` workflow step with tests and docs.)

## Improvements (251–299)

- [ ] [GX-251] Improve the workflow summary. I ran @configs/account-rename.yaml and I got:

Summary: total.repos=104 PROTOCOL_SKIP=104 REMOTE_MISSING=1 REMOTE_SKIP=51 REPO_FOLDER_SKIP=52 REPO_SWITCHED=92 TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582 WORKFLOW_STEP_SUMMARY=582 WARN=139 ERROR=1 duration_human=6m55.109s duration_ms=415109

Remove duration_ms Leave only human duration and rename it to duration.
remove  TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582
add missing steps in the summary (like namespace rewrite, namespace delete etc)

- [x] [GX-252] Add steps to @configs/account-rename.yaml that allows to bump up the dependency versions of go.mod (see GX-110). (Added go get/go mod tidy workflow steps with go.mod safeguards.)
- [ ] [GX-253] Add steps to @configs/account-rename.yaml to upgrade go version in go.mod to `go 1.25.4`

## BugFixes (340–399)

- [ ] [GX-345] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step. (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)

## Maintenance (422–499)

- [ ] [GX-423] Cleanup:
  1. Review the completed issues and compare the code against the README.md and ARCHITECTURE.md files.
  2. Update the README.md and ARCHITECTURE.
  3. Clean up the completed issues.

## Planning
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
