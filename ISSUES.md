# ISSUES (Append-only Log)

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

## Features (100–199)

- [x] [GX-100] Implement rewriting namespace. The prototype is under @tools/ns-rewrite. Use workflow/task interface and lean on the already built ability to change file content.
  - Resolution: Added namespace rewrite service, workflow action, and `gix repo namespace rewrite` command with tests and configuration defaults.

- [x] [GX-22] License injection (prototype under tools/licenser)
  - Status: Resolved
  - Category: Feature
  - Context: Prototype exists; not yet slated.
  - Desired: Implement a repo level workflow that would allow distribution of licensing using built in capabilities
  - Resolution: Added `gix repo license apply` to distribute license files via workflow tasks with configuration defaults, README docs, and regression coverage.

- [ ] [GX-23] Git retag (prototype under tools/git_retag)
  - Status: Unresolved (Not ready)
  - Category: Feature
  - Context: Prototype exists; not yet slated.
  - Desired: Implement a repo level workflow that would allow fixing incorrect tagging, including history, into proper sequence

- [ ] [GX-104] Add convenience CLI: `gix repo files add` mapping to apply-tasks
  - Status: Resolved
  - Category: Feature
  - Context: Frequently need to drop a standard file (e.g., POLICY.md) into many repos without crafting a workflow file.
  - Desired: Introduce `gix repo files add --path <relative> [--content-file <path>|--content <text>] [--mode skip-if-exists|overwrite] [--permissions <octal>] [--branch <name>] [--push] [--roots <dir>...]`.
    - Maps to an `apply-tasks` task that writes the file, commits with a configurable message (default `docs: add <path>`), and optionally pushes/opens a PR.
    - Honors `--dry-run`, `-y/--yes`, shared `--remote`, and root discovery flags consistent with other repo commands.
  - Notes: Lives under existing `repo files` namespace alongside `replace`.

- [ ] [GX-105] Refactor the DSL for the workflow and make use of commands and subcommands DSL instead of `operation: rename-directories`. Document the changes.

## Improvements (200–299)

- [x] [GX-200] Remove `--to` flag from default command and accept the new branch as an argument, e.g. `gix b default master`
  - Resolution: `branch default` now takes the target branch as a positional argument while still honoring configured defaults; docs and tests updated accordingly.
- [x] [GX-201] Identify non-critical operations and turn them into warnings, which do not stop the flow:
```
14:58:29 tyemirov@computercat:~/Development/Poodle/product_page_analysis.py [main] $ gix b default --to master
default branch update failed: GitHub Pages update failed: GetPagesConfig operation failed: gh command exited with code 1
```
The Pages may be not configured and that's ok
- Resolution: GitHub Pages lookup/update failures during `branch default` now emit warnings and the migration continues, leaving branch promotion untouched.
- [x] [GX-202] have descriptive and actionable error messages, explaining where was the failure and why the command failed:
```
14:56:43 tyemirov@computercat:~/Development/Poodle $ gix --roots . b cd master
failed to fetch updates: git command exited with code 128
```
If a repository doesnt have a remote, there is nothing to fetch, but we can still change the default branch, methinks. Identify non-critical steps and ensure they produce warnings but are non-blocking. Encode this semntics into tasks and workflows.
- Resolution: `branch cd` now logs `FETCH-SKIP`/`PULL-SKIP` warnings when network operations fail and continues switching branches, so repositories without remotes (or offline) still migrate.
- [x] [GX-203] make gix version and gix --version work the same and display its version
  - Resolution: Added a `version` subcommand backed by the existing resolver so both `gix version` and `gix --version` print identical output; new regression coverage locks the behavior.
- [x] [GX-204] Introduce reusable workflow safeguards that gate repository tasks (clean worktree, branch checks, etc.) and skip repositories when conditions fail.
  - Resolution: Added shared safeguard evaluator, task-level support, and CLI wiring so workflows can skip repositories based on clean-state, branch, or path checks with comprehensive tests.
- [x] [GX-205] Standardize error schema across commands
  - Resolution: Added centralized workflow error formatter that injects repository owner/path metadata, humanizes sentinel-only messages, and ensures all logged failures follow the `CODE: owner/repo (path) message` schema; regression coverage now verifies formatted output for direct log handling and rename operations.

- [x] [GX-206] Remove redundant prefixes; single-source formatting
  - Resolution: Updated the workflow executor to wrap failures in a structured error instead of `workflow operation … failed`, trimming newline duplicates and reusing the GX-205 formatter; regression coverage now asserts that only the standardized `CODE: owner/repo (path) message` text is surfaced.

- [x] [GX-208] Remote/metadata semantics and skip policy
  - Resolution: Remote workflow operations now emit standardized `remote_missing` SKIP messages when the origin remote is absent and `origin_owner_missing` warnings when metadata cannot be resolved, exercised via new workflow unit tests that cover both scenarios for canonical remote updates.

- [x] [GX-207] DAG-based workflow execution
  - Resolution: Added workflow step metadata (`name`/`after`), introduced DAG planners that build topological stages, and taught the executor to run independent operations in parallel with shared error handling; new unit tests cover branching layers and cycle detection while maintaining sequential defaults for legacy configs.

- [ ] [GX-210] Enable LLM actions in workflows and output piping between steps
  - Status: Unresolved
  - Category: Improvement
  - Context: `apply-tasks` supports `commit.message.generate` and `changelog.message.generate` actions, but they require a programmatic client and there is no way to pass their outputs into subsequent steps (e.g., as a `commit_message` for namespace rewrite).
  - Desired: Allow configuring an LLM client in workflow YAML (model, base-url, api-key env, timeout) and introduce workflow variables so action outputs can be referenced as inputs by later steps.

- [x] [GX-211] On a protocol updated the message says: `UPDATE-REMOTE-DONE: /tmp/repos/loopaware origin now ssh://git@github.com/tyemirov/loopaware.git`. But that's incorrect as the new protocol is not `ssh://git@github.com/tyemirov/loopaware.git` but `git@github.com/tyemirov/loopaware.git`. Change logging to display the new procol as what `git remote -v will display`
  - Resolution: Protocol/remote executors now format SSH remotes as `git@github.com:owner/repo.git` in plan/done logs without altering underlying URLs; regression coverage added for plan/success messages, repo CLI, and workflow integration outputs.

- [x] [GX-212] Change logging to a new format: 
- Resolution: Introduced a shared structured reporter that timestamps events, aligns human-readable columns, and emits machine-parseable key/value pairs for every executor log entry.

Single line per event, with two parts:

Human part (left): aligned columns, easy to scan.

- [ ] [GX-212] Add details of what exactly is unclean in a directory, which files. Improve the message we are getting now with the details
```
TASK-SKIP: Switch to master if clean /tmp/repos/tyemirov/GAuss repository not clean
TASK-SKIP: Rewrite module namespace /tmp/repos/tyemirov/GAuss repository dirty
```

### Examples (your lines → refactored)

**Switch**

```
06:22:11 INFO  REPO_SWITCHED     MarcoPoloResearchLab/mpr-ui         → master                                   | event=REPO_SWITCHED repo=MarcoPoloResearchLab/mpr-ui branch=master path=/tmp/repos/loopaware/tools/MarcoPoloResearchLab/mpr-ui
```

**Namespace skip (missing go.mod)**

```
06:22:12 WARN  NAMESPACE_SKIP    MarcoPoloResearchLab/mpr-ui         missing go.mod                             | event=NAMESPACE_SKIP repo=MarcoPoloResearchLab/mpr-ui has_go_mod=false reason=missing_go_mod
```

**Remote missing (still not an error)**

```
06:22:01 WARN  REMOTE_MISSING    integration-org/ns-rewrite          no remote                                  | event=REMOTE_MISSING repo=integration-org/ns-rewrite path=/tmp/repos/gix/tools/integration-org/ns-rewrite
```

**Namespace apply with push**

```
06:22:53 INFO  NAMESPACE_APPLY   tyemirov/loopaware                  files=7, pushed                            | event=NAMESPACE_APPLY repo=tyemirov/loopaware files_changed=7 push=true branch=chore/ns-rename/20251103-062253Z
```

**Error example (only this goes to stderr)**

```
06:23:04 ERROR REPO_DIRTY        tyemirov/gix                        working tree not clean                     | event=REPO_DIRTY repo=tyemirov/gix path=/tmp/repos/tyemirov/gix
```

- [x] [GX-213] When logging the events Print a thin repo header once, then events; the first token stays parseable so grep still works:
- Resolution: Workflow runs now print scoped repo headers before grouped events via the structured reporter, keeping the first token parseable while adding readable alignment.
```
── repo: MarcoPoloResearchLab/RSVP ─────────────────────────────────────────────────────────
06:22:11 INFO  REPO_SWITCHED     MarcoPoloResearchLab/RSVP           → master                                   | event=REPO_SWITCHED repo=MarcoPoloResearchLab/RSVP branch=master path=/tmp/repos/MarcoPoloResearchLab/RSVP
06:22:21 INFO  NAMESPACE_APPLY   MarcoPoloResearchLab/RSVP           files=31, pushed  
```
- [x] [GX-214] Have a final summary for the whole run, smth like (just an example) Summary: total.repos=12 REPO_SWITCHED=7 NAMESPACE_APPLY=3 NAMESPACE_SKIP=3 REMOTE_UPDATE=2 WARN=1 ERROR=0 duration_ms=5312
- Resolution: Structured reporter tracks per-event counts and prints a workflow summary footer once execution completes, including duration and INFO/WARN/ERROR tallies.

- [ ] [GX-215] the changelog command produces a summary: "Summary: total.repos=0 duration_ms=0". It shall not produce a summary (and neither shall a commit message command)

## BugFixes (300–399)

- [x] [GX-300] `gix b default` aborts for repositories without remotes; it treats the `git fetch` failure as fatal instead of warning and skipping the fetch, so the branch switch never executes.
  - Resolution: The branch change service now enumerates remotes once, skips fetch/pull when none exist, and creates branches without tracking nonexistent remotes. Added regression coverage for the zero-remote case.
- [x] [GX-301] The message is repetitive, it's enough to say -- unable to update default branch. But it's absolutely unclear why or where it has happened. The error message shall be actionable, not informative. We must specify what folder/branch/etc we were at, and what command has failed, and why. That is an error/warning criteria for all commands.
  - Resolution: Default branch update failures now raise `DefaultBranchUpdateError`, which includes repository path, identifier, source/target branches, and the GitHub CLI failure summary; the workflow operation forwards this error without extra wrapping, and new tests assert the actionable messaging.
```
01:06:39 tyemirov@computercat:~/Development/Poodle $ gix --roots . b default master
WORKFLOW-DEFAULT-SKIP: /home/tyemirov/Development/Poodle/ProductScanner already defaults to master
workflow operation apply-tasks failed: default branch update failed: unable to update default branch: UpdateDefaultBranch operation failed: gh command exited with code 1
```
- [x] [GX-302] Produces non-sensical messages about failures. It's not clear what exactly has failed and what shall be the user's action item. Why would it need to create a master branch here, if it already exists ?
```
14:17:45 tyemirov@computercat:~/Development/loopaware [improvement/LA-201-theme-switch-footer] $ gix b cd master
SWITCHED: /home/tyemirov/Development/loopaware -> master
workflow operation apply-tasks failed: failed to create branch "master" from origin: git command exited with code 128
```
  - Resolution: Branch change service now distinguishes missing-branch failures from dirty working tree errors, surfaces the Git diagnostics in returned messages, and adds regression coverage for both scenarios so the CLI reports actionable guidance instead of redundant branch creation attempts.
  - Update: Fetch and pull skip warnings now include repository paths so operators can see which repository triggered the Git error.
  - Update: Missing or inaccessible remotes now raise `WARNING: no remote counterpart for <repo>` so branch-cd skips fetches without dumping Git internals while still pointing to the affected repository.
- [x] [GX-303] the command hangs: `gix r prs delete --yes`
  - Resolution: Branch cleanup now skips GitHub metadata lookups by default, preventing `gh repo view` from blocking the run; runtime options and audits tolerate missing GitHub clients, and new tests cover the metadata-free path.

- [x] [GX-305] The command `gix b default master` fails despite having a valid token in the environment. Ensure we are reading the default GH token GITHUB_API_TOKEN, or, if the env variable is absent or empty and the remote exists, fail fast with an appropriate error message about missing token
  - Resolution: GitHub CLI invocations now normalize `GITHUB_API_TOKEN`/`GITHUB_TOKEN`/`GH_TOKEN`, and default-branch migrations validate token presence before issuing GitHub calls with a clear error when missing; regression coverage added across shell executor, migrate workflow, and integration harnesses.
```shell
12:40:01 tyemirov@computercat:~/Development/Poodle/ProductScanner/tools/mpr-ui [main] $ gix b default master
workflow operation apply-tasks failed: DEFAULT-BRANCH-UPDATE repository=MarcoPoloResearchLab/mpr-ui path=/home/tyemirov/Development/Poodle/ProductScanner/tools/mpr-ui source=main target=master: gh: Validation Failed (HTTP 422)
12:40:07 tyemirov@computercat:~/Development/Poodle/ProductScanner/tools/mpr-ui [main] $ curl -H "Authorization: Bearer $GITHUB_API_TOKEN" https://api.github.com/user
{
  "login": "tyemirov",
  "id": 1078274,
  "node_id": "MDQ6VXNlcjEwNzgyNzQ=",
  "avatar_url": "https://avatars.githubusercontent.com/u/1078274?v=4",
  "gravatar_id": "",
  "url": "https://api.github.com/users/tyemirov",
  "html_url": "https://github.com/tyemirov",
  "followers_url": "https://api.github.com/users/tyemirov/followers",
  "following_url": "https://api.github.com/users/tyemirov/following{/other_user}",
  "gists_url": "https://api.github.com/users/tyemirov/gists{/gist_id}",
  "starred_url": "https://api.github.com/users/tyemirov/starred{/owner}{/repo}",
  "subscriptions_url": "https://api.github.com/users/tyemirov/subscriptions",
  "organizations_url": "https://api.github.com/users/tyemirov/orgs",
  "repos_url": "https://api.github.com/users/tyemirov/repos",
  "events_url": "https://api.github.com/users/tyemirov/events{/privacy}",
  "received_events_url": "https://api.github.com/users/tyemirov/received_events",
  "type": "User",
  "user_view_type": "public",
  "site_admin": false,
  "name": "Vadym Tyemirov",
  "company": "Marco Polo Research Lab",
  "blog": "https://mprlab.com",
  "location": "Los Angeles, CA",
  "email": null,
  "hireable": null,
  "bio": "Father. Husband. Friend.\r\n\r\nAipreneurer. Sus engineer. Founder.",
  "twitter_username": null,
  "notification_email": null,
  "public_repos": 68,
  "public_gists": 11,
  "followers": 4,
  "following": 4,
  "created_at": "2011-09-25T14:17:14Z",
  "updated_at": "2025-10-25T04:56:38Z"
}
```
- [x] [GX-304] No-remote repos cause failures across commands
  - Resolution: Added integration coverage proving `gix branch cd` and `gix workflow` succeed on repositories without remotes, emitting the expected skip/success messages without errors; no additional fixes were required.

- [x] [GX-310] Surface namespace rewrite git failures and handle push errors gracefully
  - Status: Unresolved
  - Category: BugFix
  - Context: Running the owner-renaming workflow on `/tmp/repos/tyemirov/gix` aborted with `namespace_rewrite_failed: git command exited with code 1`, hiding the underlying `git push --set-upstream` failure (no credentials in test environment) and stopping the entire workflow.
  - Desired: Capture and display the git stderr for namespace rewrite failures, and degrade push/authentication failures into actionable SKIP messages (or allow `push: true` to fall back to no push) so the workflow can continue.
  - Notes: Repro via `gix workflow configs/cleanup.yaml --roots /tmp/repos --yes` as seen in the provided run log.
  - Resolution: Namespace rewrite now wraps push failures with `namespace_push_failed`, returns partial results, and the workflow handler logs the formatted error while continuing with `push=false` output and a skip reason; regression tests cover service and workflow behavior.
  - Resolution: Additional safeguards skip pushes when `remote.<name>.url` is missing or when the remote branch already matches `HEAD`, emitting structured skip logs instead of surfacing opaque git errors.

- [x] [GX-311] Fix namespace task log formatting emitting literal `\n`
  - Status: Resolved
  - Category: BugFix
  - Context: Workflow output shows lines like `NAMESPACE-NOOP: ... reason=namespace already up to date\nUPDATE-REMOTE-SKIP: ...`, indicating the namespace log templates use `\n` (escaped newline), so the literal `\n` leaks into output.
  - Desired: Update namespace task templates (`namespaceNoopMessageTemplate`, `namespaceApplyMessageTemplate`, etc.) to use actual newlines and ensure all workflow log helpers emit newline-separated entries without escape sequences.
  - Notes: Observed during the owner-renaming workflow run on `/tmp/repos`.
  - Resolution: Namespace workflow templates now emit actual newline characters, regression tests enforce the absence of literal `\n`, and push/skip outputs render as separate lines.

- [x] [GX-312] When replacing the namespace, include tests. E.g. The dependency has been switched to `github.com/tyemirov/GAuss`, but the test suites still import `github.com/temirov/GAuss` (see `cmd/server/auth_redirect_test.go`, `internal/httpapi/auth_test.go`, `internal/httpapi/dashboard_integration_test.go`). Once the upstream module renames its `module` path, these imports will no longer resolve and `go test ./...` fails at compile time with “cannot find module providing github.com/temirov/GAuss/...”. The tests need their imports rewritten to the new namespace to keep the build green.
  - Resolution: Namespace rewrite now scans Go test files, rewrites imports, stages them, and regression coverage locks `_test.go` handling.

- [x] [GX-313] Repository discovery should skip nested repositories ignored by parent `.gitignore`
  - Category: BugFix
  - Context: Workflow runs still act on repositories located under ignored directories (e.g., `tools/licenser` inside `gix`), staging ignored files and slowing execution.
  - Desired: Leverage `git check-ignore` to filter out ignored nested repositories before executing operations across commands and workflows.
  - Resolution: Added shared gitignore helpers, wired audit discovery to remove gitignored children, updated namespace rewrite to reuse the helper, and refreshed unit/integration coverage so workflows skip ignored repositories.

- [x] [GX-314] The changelog command generates the message twice
  - Resolution: Added workflow-level repository deduplication so changelog actions run once per path and added regression coverage preventing duplicate CLI output.

- [x] [GX-315] Invesigate the bug and write a plan for fixing it:
```
-- repo: tyemirov/product_page_analysis.py -------------------------------------
13:39:03 INFO  REMOTE_SKIP        tyemirov/product_page_analysis.py  already canonical                        | event=REMOTE_SKIP path=/tmp/repos/Poodle/product_page_analysis.py reason=already_canonical repo=tyemirov/product_page_analysis.py
13:39:04 INFO  REPO_FOLDER_RENAME                                    /tmp/repos/Poodle/product_page_analysis.py → /tmp/repos/Poodle/tyemirov/product_page_analysis.py | event=REPO_FOLDER_RENAME new_path=/tmp/repos/Poodle/tyemirov/product_page_analysis.py old_path=/tmp/repos/Poodle/product_page_analysis.py path=/tmp/repos/Poodle/product_page_analysis.py
apply-tasks: failed to switch to branch "master": fatal: invalid reference: master: git command exited with code 128
```
  - Resolution: Workflow task execution now verifies the start point branch before checkout, logs a warning when missing, and falls back to the current HEAD so namespace workflows continue on repositories without a `master` branch.

- [x] [GX-316] The stats and logs show switching repo twice. There is only one repo to switch. Fix the bug and ensure we don't have duplicate logic of identifying repos.
  - Resolution: Workflow executor now canonicalizes repository paths from roots so relative entries (like `.`) cannot duplicate the same repository, and branch logs emit a single `REPO_SWITCHED` per repo.
```
14:32:50 tyemirov@computercat:~/Development/Research/TAuth/tools/mpr-ui [improvement/TA-208-nonce-cdn] $ gix b cd master
-- repo: MarcoPoloResearchLab/mpr-ui -------------------------------------------
14:33:16 INFO  REPO_SWITCHED      MarcoPoloResearchLab/mpr-ui        → master                                 | branch=master created=false event=REPO_SWITCHED path=/home/tyemirov/Development/Research/TAuth/tools/mpr-ui repo=MarcoPoloResearchLab/mpr-ui
14:33:19 INFO  REPO_SWITCHED                                         → master                                 | branch=master created=false event=REPO_SWITCHED path=.
Summary: total.repos=1 REPO_SWITCHED=2 WARN=0 ERROR=0 duration_ms=5792
14:33:19 tyemirov@computercat:~/Development/Research/TAuth/tools/mpr-ui [master] $ ll
total 72K
drwxrwxr-x 3 tyemirov tyemirov 4.0K Nov  3 14:33 ./
drwxrwxr-x 3 tyemirov tyemirov 4.0K Nov  3 10:34 ../
-rw-rw-r-- 1 tyemirov tyemirov  17K Nov  3 10:43 footer.js
drwxrwxr-x 8 tyemirov tyemirov 4.0K Nov  3 14:33 .git/
-rw-rw-r-- 1 tyemirov tyemirov 2.2K Nov  2 12:27 .gitignore
-rw-rw-r-- 1 tyemirov tyemirov  21K Nov  3 14:33 mpr-ui.js
-rw-rw-r-- 1 tyemirov tyemirov  12K Nov  3 10:44 README.md
14:33:42 tyemirov@computercat:~/Development/Research/TAuth/tools/mpr-ui [master] $ 
```

- [x] [GX-316] `git b cd` command must pull down the latest version of the code after switching to the sepcified branch
  - Resolution: `branch cd` now falls back to fetching all remotes and running a pull when the configured remote is missing, emitting a `FETCH-FALLBACK` warning instead of silently skipping updates.
```
14:36:12 tyemirov@computercat:~/Development/Research/TAuth [improvement/TA-208-nonce-validation] $ gix b cd master
-- repo: tyemirov/TAuth --------------------------------------------------------
14:36:18 INFO  REPO_SWITCHED      tyemirov/TAuth                     → master                                 | branch=master created=false event=REPO_SWITCHED path=/home/tyemirov/Development/Research/TAuth repo=tyemirov/TAuth
14:36:21 INFO  REPO_SWITCHED                                         → master                                 | branch=master created=false event=REPO_SWITCHED path=.
Summary: total.repos=1 REPO_SWITCHED=2 WARN=0 ERROR=0 duration_ms=5687
14:36:21 tyemirov@computercat:~/Development/Research/TAuth [master] $ 
14:40:47 tyemirov@computercat:~/Development/Research/TAuth [master] $ git pull
Updating cd6d70a..c430e78
Fast-forward
 ARCHITECTURE.md                             |   6 ++-
 CHANGELOG.md                                |   2 +
 ISSUES.md                                   |   5 +-
 README.md                                   |  44 +++++++++++++++-
 cmd/server/main.go                          |  12 ++++-
 internal/authkit/config.go                  |   1 +
 internal/authkit/nonce_store.go             |  92 +++++++++++++++++++++++++++++++++
 internal/authkit/nonce_store_test.go        |  48 +++++++++++++++++
 internal/authkit/routes.go                  |  48 +++++++++++++++--
 internal/authkit/routes_http_test.go        |  69 +++++++++++++++++++------
 internal/authkit/routes_integration_test.go | 303 +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++---------------------------------
 tests/mpr-auth-header.test.js               | 165 ++++++++++++++++++++++++++++++++++++++++++++++++++++++++---
 tests/mpr-footer.test.js                    |  74 +++++++++++++++++++++++++++
 web/demo.html                               | 168 ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++--
 web/mpr-ui.js                               | 296 ---------------------------------------------------------------------------------------------------------
 15 files changed, 908 insertions(+), 425 deletions(-)
 create mode 100644 internal/authkit/nonce_store.go
 create mode 100644 internal/authkit/nonce_store_test.go
 create mode 100644 tests/mpr-footer.test.js
 delete mode 100644 web/mpr-ui.js
```

- [x] [GX-317] I am getting cryptic error. Ensure that all errors in all commands have detailed information about when an error occured, and what exactly has happend to cause the error
```
20:31:21 tyemirov@computercat:~/Development/gix [master] $ gix r release v0.2.0-rc.4
RELEASED: /home/tyemirov/Development/gix -> v0.2.0-rc.4
apply-tasks: failed to create tag "v0.2.0-rc.4": git command exited with code 128
```
What's interesting, teh command actually worked, and I can see the tag I wanted both locally and remotely. So the error is especially infuriating as it's a complete bogus.
- Resolution: Release tag creation/push failures now emit repository-scoped operation errors that include the git command, exit code, and sanitized stderr, with regression tests covering annotate and push scenarios.

- [ ] [GX-318] The message "namespace rewrite skipped: files ignored by git" doesnt make much sense. Must be a bug. Investigate the real reason the operation hasn't been performed.
```
-- repo: tyemirov/ctx ----------------------------------------------------------
22:34:53 INFO  REMOTE_SKIP        tyemirov/ctx                       already canonical                        | event=REMOTE_SKIP path=/tmp/repos/ctx reason=already_canonical repo=tyemirov/ctx
22:34:55 INFO  REPO_FOLDER_RENAME                                    /tmp/repos/ctx → /tmp/repos/tyemirov/ctx | event=REPO_FOLDER_RENAME new_path=/tmp/repos/tyemirov/ctx old_path=/tmp/repos/ctx path=/tmp/repos/ctx
22:35:01 INFO  REPO_SWITCHED      tyemirov/ctx                       → master                                 | branch=master created=false event=REPO_SWITCHED path=/tmp/repos/tyemirov/ctx repo=tyemirov/ctx
22:35:01 INFO  NAMESPACE_NOOP     tyemirov/ctx                       namespace rewrite skipped: files ignored by git | event=NAMESPACE_NOOP path=/tmp/repos/tyemirov/ctx reason=namespace_rewrite_skipped:_files_ignored_by_git repo=tyemirov/ctx
```

## Maintenance (400–499)

- [x] [GX-400] Update the documentation @README.md and focus on the usefullness to the user. Move the technical details to @ARCHITECTURE.md
- [x] [GX-401] Ensure architrecture matches the reality of code. Update @ARCHITECTURE.md when needed
  - Resolution: `ARCHITECTURE.md` now documents the current Cobra command flow, workflow step registry, and per-package responsibilities so the guide mirrors the Go CLI.
- [x] [GX-402] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md
  - Resolution: Authored `docs/policy_refactor_plan.md` detailing domain-model introductions, error strategy, shared helper cleanup, and new test coverage aligned with the confident-programming policy.
- [x] [GX-403] Introduce domain types for repository metadata and enforce edge validation
  - Resolution: Added smart constructors in `internal/repos/shared` for repository paths, owners, repositories, remotes, branches, and protocols, refactored repos/workflow options to require these types, updated CLI/workflow edges to construct them once, and expanded tests to cover the new constructors.
- [x] [GX-404] Establish contextual error strategy for repository executors
  - Resolution: Added `internal/repos/errors` sentinel catalog, refactored remotes/protocol/rename/history executors to wrap failures with operation-specific codes, taught workflow operations to log the contextual errors, and extended unit/integration tests to assert on the new propagation semantics.
- [x] [GX-405] Consolidate shared helpers and eliminate duplicated validation
  - Resolution: Added shared reporter/policy helpers for repository executors, refactored protocol/remotes/rename workflows to reuse optional owner parsing and structured confirmation policies, and updated tests/CLI bridges to exercise the new abstractions without redundant trimming or boolean flags.
- [x] [GX-406] Expand regression coverage for policy compliance
  - Add table-driven tests for the new domain constructors and protocol conversion edge cases (current vs. target protocol mismatches, missing owner slugs, unknown protocols).
  - Test dependency resolvers in `internal/repos/dependencies` to ensure logger wiring and error propagation.
  - Extend workflow integration tests to confirm domain types propagate correctly through task execution.
- Resolution: Added shared constructor/optional parser tables, expanded protocol executor edge cases, introduced resolver unit tests, and enforced canonical messaging in workflow integration output; suites now cover policy boundaries.
- [x] [GX-407] Update documentation and CI tooling for the refactor
  - Document newly introduced domain types, error codes, and edge-validation flow in `docs/cli_design.md` (or a dedicated `docs/refactor_status.md`) and cross-link from `POLICY.md`.
  - Update developer docs describing prompt/output handling after GX-405 cleanup.
  - Extend CI to run `staticcheck` and `ineffassign` alongside the existing `go test ./...` gate.
  - Resolution: Added domain model section and prompt guidance to `docs/cli_design.md`, cross-linked from `POLICY.md`, refreshed README developer notes, wired `staticcheck`/`ineffassign` into `make ci`, and resolved all new lint findings.
- [x] [GX-408] Update the @README.md with the full list of the commands that gix supports, their syntax and modifiers
  - Resolution: Added a comprehensive Command Reference to README with all namespaces, subcommands, aliases, and flags; corrected examples (use `gix branch commit message --roots .` and documented shared `--remote`, `--version`, and `--init` flags).
- [x] [GX-409] Describe workflow syntax and how workflows can be build for custom operations and sequences in the @README.md
  - Resolution: Expanded README “Automate sequences with workflows” to document YAML schema (`workflow: [ { step: { name, after, operation, with } } ]`), DAG semantics, built-in operations and their `with` options, `apply-tasks` schema with templating and supported actions, safeguards (`require_clean`, `branch`, `branch_in`, `paths`), and execution defaults (`--dry-run`, `--require-clean`).
- [x] [GX-410] let's add an example of a workflow that:
  1. Changes all repos' remotes to canonical
  2. Changes all folders to canonical names with owners
  3. Switch the active branch to master if the git tree is clean
  4. Updates the Go namespaces to match the new canonical (FYI I have changed the name of my account, so I now need to rename the go modules from github.com/
  temirov to github.com/tyemirov)
  5. Have a commit message for the rename through LLM
  6. Commit and push the changes
  If some of the steps are not possible, let's document them as new features or improvements
  - Resolution: Added a complete YAML workflow example to README under “Example: Canonicalize after owner rename”, implementing canonical remote updates, folder renames with owners, conditional branch switch (via safeguards), and namespace rewrite with commit/push. Documented the current limitation around LLM-generated commit messages inside workflows and referenced new improvement items.
- [ ] [GX-4011] Review @POLICY.md and verify what code areas need improvements and refactoring. Prepare a detailed plan of refactoring. Check for bugs, missing tests, poor coding practices, uplication and slop. Ensure strong encapsulation and following the principles og @AGENTS.md and policies of @POLICY.md


## Planning 
do not work on the issues below, not ready
