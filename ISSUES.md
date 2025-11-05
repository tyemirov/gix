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

- [ ] [GX-323] Rename the commands:
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
