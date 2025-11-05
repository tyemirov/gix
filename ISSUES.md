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

- [ ] [GX-217] Erroneous summary message: total.repos=0, should be total.repos=1, and it also must have taken some time, so the executor shall be returning time that we shall be printing in human readeable foramt.
RELEASED: /Users/tyemirov/Development/tyemirov/gix -> v0.2.0-rc.6
Summary: total.repos=0 duration_ms=0

- [ ] [GX-218] Remove the top level commands `repository` and `branch` and only use their subcommands.

## BugFixes (320–399)

- [ ] [GX-320] The message "namespace rewrite skipped: files ignored by git" doesnt make much sense. Must be a bug. Investigate the real reason the operation hasn't been performed.
  - Resolution: Namespace rewrite now distinguishes between "no references" and "all matches ignored"; skips without matches report `namespace rewrite skipped: no references to <prefix>` while gitignored-only matches keep the git warning. Updated unit tests cover both skip paths.
```shell
-- repo: tyemirov/ctx ----------------------------------------------------------
22:34:53 INFO  REMOTE_SKIP        tyemirov/ctx                       already canonical                        | event=REMOTE_SKIP path=/tmp/repos/ctx reason=already_canonical repo=tyemirov/ctx
22:34:55 INFO  REPO_FOLDER_RENAME                                    /tmp/repos/ctx → /tmp/repos/tyemirov/ctx | event=REPO_FOLDER_RENAME new_path=/tmp/repos/tyemirov/ctx old_path=/tmp/repos/ctx path=/tmp/repos/ctx
22:35:01 INFO  REPO_SWITCHED      tyemirov/ctx                       → master                                 | branch=master created=false event=REPO_SWITCHED path=/tmp/repos/tyemirov/ctx repo=tyemirov/ctx
22:35:01 INFO  NAMESPACE_NOOP     tyemirov/ctx                       namespace rewrite skipped: files ignored by git | event=NAMESPACE_NOOP path=/tmp/repos/tyemirov/ctx reason=namespace_rewrite_skipped:_files_ignored_by_git repo=tyemirov/ctx
```

- [ ] [GX-321] the changelog command produces a summary: "Summary: total.repos=0 duration_ms=0". It shall not produce a summary (and neither shall a commit message command)

- [ ] [GX-322] Investigate the reason of the workflow exit. Ensure it is impossible for a workflow to exit, only the tasks can report the error conditions but a workflow will always finish successfully even if with an error status
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
## Maintenance (410–499)

- [ ] [GX-411] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md

## Planning 
do not work on the issues below, not ready
