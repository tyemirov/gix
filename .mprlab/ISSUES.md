# ISSUES

Entries record newly discovered requests or changes.

Read @AGENTS.md (Workflow section), @POLICY.md, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

## BugFixes

- [x] [B001] (P1) `gix sync` should not require `LLM_PROXY_SECRET` unless the LLM Proxy transport is explicitly configured.
  Requested on 2026-06-29 after `gix sync` in `/Users/tyemirov/Development/llm-proxy` on branch `feature/F007-dashboard-settings-modal` failed with `environment variable LLM_PROXY_SECRET must be set to generate a commit message`.
  ## Observation
  - Dirty `gix sync` inherited the `message commit` LLM defaults.
  - The embedded `message commit` and `message changelog` defaults still pointed at the MPR LLM Proxy endpoint and `LLM_PROXY_SECRET`.
  - The LLM client selected the proxy transport implicitly from the base URL, so an operator who had not explicitly selected the proxy transport still hit the proxy-specific secret requirement.
  ## Deliverable
  - Make the canonical default LLM transport OpenAI-compatible with `OPENAI_API_KEY`.
  - Add an explicit `transport` setting for message, sync, web, and workflow LLM client construction.
  - Use the MPR LLM Proxy transport and `LLM_PROXY_SECRET` only when `transport: llm_proxy` or `--transport llm_proxy` is configured.
  - Preserve explicit custom OpenAI-compatible base URLs without proxy inference.
  ## Resolution
  - Added transport-aware LLM client configuration with `openai_compatible` as the default transport and `llm_proxy` as an explicit transport.
  - Updated embedded message defaults, sync commit-message config, web message helpers, and workflow LLM configuration to pass the transport field.
  - Added regressions for embedded sync defaults, transport default env/base URL selection, workflow omitted-env behavior, and transport-aware web/message tests.
  - Updated README LLM configuration docs.
- [x] [B002] (P1) `gix sync` should accept open chained pull requests instead of requiring the PR to target `master`.
  Requested on 2026-06-23 after `gix sync` in `/Users/tyemirov/Development/MediaOps` on branch `tyemirov/bugfix/B159-mcp-migration-capability-inventory` failed with `branch "tyemirov/bugfix/B159-mcp-migration-capability-inventory" does not have an open pull request into master`, while `gh pr view` showed an open PR from that branch into `tyemirov/bugfix/B158-mcp-cli-parity-schemas`.
  ## Observation
  - Strict sync listed open pull requests with `gh pr list --base master`, so chained PRs targeting a previous issue branch were invisible.
  - After the base-filtered lookup missed the open PR, sync fell through to the missing-PR error and never switched, merged, or pushed the branch.
  - Even if the lookup accepted the PR, syncing against hard-coded `origin/master` would not align a chained branch with its actual review base.
  ## Deliverable
  - List open pull requests for the branch without filtering by base.
  - Capture `baseRefName` from GitHub and merge that base branch for existing PR-backed syncs.
  - Keep merged-PR handoff checks scoped to the configured base branch.
  - Reject malformed open-PR responses that do not report a base branch instead of falling back silently.
  ## Resolution
  - `githubcli.ListPullRequests` now supports optional base filters and parses `baseRefName`.
  - Existing strict sync PR branches now match open PRs with `--head` and without `--base`, then merge `origin/<baseRefName>` before pushing.
  - Dirty generated-branch reuse uses the same head-filtered open-PR lookup.
  - Missing-PR errors no longer claim the lookup was specifically into `master`.
  - Added strict-sync and GitHub client regressions for chained PR bases, missing PR base metadata, optional `--base`, `--head`, and parsed `baseRefName`.
- [x] [B003] (P1) `gix sync` should offer to sync `master` when a branch pull request is already merged.
  Requested on 2026-06-07 after `gix sync` in `/Users/tyemirov/Development/MediaOps` on branch `gix/add-provider-gated-speech-speed-capability-and` printed `branch "gix/add-provider-gated-speech-speed-capability-and" does not have an open pull request into master` twice instead of recognizing that a closed-and-merged pull request should hand off to the base branch.
  ## Observation
  - Direct `gix sync` runs the sync action through the workflow task runner.
  - Strict PR sync checks only for open pull requests before returning the missing-PR error, so a branch whose PR has already merged is treated like a branch that never had a PR.
  - The workflow runner also writes the operation failure to stderr, then returns the same failure to `main`, which prints it again.
  ## Deliverable
  - When a branch has no open PR but has a merged PR into the configured base branch, prompt the user to sync the base branch instead.
  - Make `--yes` accept that base-branch sync handoff without prompting.
  - Keep true missing-PR failures visible exactly once.
  - Preserve direct `gix sync` success output such as `SYNCED: ...`.
  - Preserve workflow logging for explicit `gix workflow` runs.
  ## Resolution
  - Strict PR sync now checks for a merged pull request after the open-PR lookup fails.
  - When a merged PR exists, direct `gix sync` prompts to sync the configured base branch instead; `--yes` accepts that handoff without prompting.
  - Declining the prompt preserves the true missing-open-PR failure path.
  - Direct `gix sync` suppresses only the duplicate workflow stderr echo, while workflow failures still return to the command and workflow stdout remains intact.
  - Focused syncflow/workflow tests, `make test-fast`, `make test-slow`, `make test`, `make lint`, and `make ci` passed locally.
- [x] [B004] (P1) `gix sync` leaves tracked ignored dirty paths after a successful sync.
  Requested on 2026-06-06 after `v0.6.5` allowed `gix sync` to finish in `/Users/tyemirov/Development/llm-proxy`, but `git status` still showed deleted `python/llm_proxy_client.egg-info/*` files and modified/deleted tracked `__pycache__` files.
  ## Observation
  - B010 filters tracked ignored paths out of dirty sync staging so they no longer trigger ignored pathspec failures.
  - Filtered tracked ignored paths are then dropped from the dirty set entirely, so sync can report `SYNCED` while `git status --porcelain` still contains tracked ignored generated artifacts.
  ## Deliverable
  - Restore tracked ignored dirty paths before sync proceeds so ignored generated artifacts do not remain as tracked worktree dirt.
  - Preserve the B010 guarantee that ignored generated pathspecs never reach `git add --all --`.
  - Preserve auto-commit behavior for ordinary tracked and unignored untracked changes.
  - Add integration coverage proving `gix sync` leaves the worktree clean after mixed ordinary and tracked ignored dirty paths.
  ## Resolution
  - Dirty sync now keeps stageable status entries separate from tracked ignored status entries.
  - After the remote fetch succeeds, `gix sync` restores tracked ignored dirty paths with `git restore --staged --worktree -- <paths>` before saving ordinary dirty work.
  - Ignored untracked paths remain unstaged, tracked ignored generated artifacts are restored to `HEAD`, and ordinary dirty files still flow through the existing generated-commit path.
  - Updated strict-sync table coverage to assert restore commands for cached ignored modifications/deletions and ignored-only tracked dirt.
  - Added strict-sync failure-mode coverage proving staged tracked ignored dirt is restored under `--require-clean`, `--stash` restores tracked ignored dirt before stashing ordinary work, fetch failures do not restore paths, and restore failures stop before branch switching or commits.
  - Updated the black-box sync integration test to prove modified/deleted tracked `.pyc` files are restored, ignored pathspecs never reach `git add --all --`, tracked `egg-info` deletions are committed, and final `git status --porcelain` is clean.
  - `make test-fast`, non-cached `make test-slow`, `make test`, `make lint`, and `make ci` passed locally.
- [x] [B005] (P1) `gix sync` still stages tracked files under ignored parent directories.
  Requested on 2026-06-06 after installing `v0.6.4` and rerunning `gix sync` in `/Users/tyemirov/Development/llm-proxy`.
  ## Observation
  - B009 filtered paths reported by `git check-ignore --stdin`.
  - `git check-ignore` intentionally does not report tracked files, so tracked `.pyc` files under ignored `__pycache__` directories still reached `git add --all --`.
  - Git rejects that explicit add with the ignored parent directory error, even though the files are already tracked.
  ## Deliverable
  - Detect tracked dirty paths that match ignore rules through Git's cached ignored view.
  - Filter those paths from dirty sync status and staging clusters before `git add --all --`.
  - Preserve auto-commit behavior for ordinary tracked changes and unignored untracked files.
  - Add regression coverage for tracked ignored pathspecs.
  ## Resolution
  - `CheckIgnoredPaths` now combines `git check-ignore --stdin` with `git ls-files --cached --ignored --exclude-standard --` so tracked files that now match ignore rules are reported.
  - Dirty sync keeps filtering ignored status entries and staging clusters through the shared helper, so cached ignored `.pyc` pathspecs are removed before `git add --all --`.
  - Refactored ignore-path and strict-sync regressions into table-driven coverage for exact ignored paths, ignored parent directory output, Windows-style Git output, cached ignored exact matches, cached ignored child output under directory pathspecs, modified/deleted tracked ignored files, untracked ignored files, and mixed ignored sources.
  - Added strict-sync coverage proving cached ignored modifications and deletions are inspected but not staged while unignored files are still committed.
  - Added a black-box `gix sync` integration test that force-tracks `.pyc` files under ignored `__pycache__` directories, modifies/deletes them, and verifies the CLI commits only the normal dirty file without invoking `git add --all --` on ignored pathspecs.
  - `make test-fast`, `make test-slow`, `make lint`, and `make ci` passed locally.
- [x] [B006] (P1) `gix sync` tries to stage ignored generated paths during dirty auto-commit.
  Requested on 2026-06-06 after `gix sync` in `/Users/tyemirov/Development/llm-proxy` failed while running `git add --all --` with Python generated files from `egg-info` and ignored `__pycache__` folders.
  ## Observation
  - Dirty sync builds explicit pathspec clusters from worktree status and passes them to `git add --all --`.
  - In the reported failure, ignored `python/llm_proxy_client/__pycache__` and `python/tests/__pycache__` paths reached `git add`, which Git rejected because they are ignored by `.gitignore`.
  ## Deliverable
  - Ensure dirty sync filters ignored generated paths before staging explicit pathspec clusters.
  - Preserve auto-commit behavior for tracked changes and unignored untracked files.
  - Add regression coverage proving ignored paths do not reach `git add`.
  ## Resolution
  - Dirty sync now checks each selected staging pathspec with Git ignore rules before invoking `git add --all --`.
  - Ignored pathspecs are removed from their commit cluster while unignored generated metadata files remain stageable.
  - Ignored-only status entries are filtered before dirty branch selection, so `gix sync master` stays on the clean base-branch sync path when there is nothing stageable.
  - Added strict-sync action coverage and CLI-level sync coverage for Python `egg-info` files mixed with ignored `__pycache__` entries.
  - `make test`, `make lint`, and `make ci` passed locally.
- [x] [B007] (P1) `gix sync` strict PR flow no longer adopts dirty sibling worktrees for the requested branch.
  Requested on 2026-06-03 after the `v0.6.1` release check showed the old sibling-worktree adoption helper still existed, but the public strict `gix sync` path did not call it when Git refused to switch to a branch already checked out in another folder.
  ## Observation
  - `gix sync` now routes public branch syncs through strict PR sync.
  - Strict PR sync called `git switch` directly and returned Git's branch-in-worktree error instead of adopting the sibling worktree.
  - The intended behavior is conditional: when the target branch is checked out in another worktree and that sibling has uncommitted files, commit and push those sibling changes before removing/pruning the sibling and retrying the switch.
  ## Resolution
  - Routed strict sync branch switching through an adoption-aware helper that only triggers on Git's sibling-worktree collision error.
  - Reused the existing sibling adoption service so dirty siblings are staged, commit-message generated, committed, pushed, removed, pruned, and then retried through the normal strict sync path.
  - Centralized the strict and non-strict retry paths behind `worktreeAdoptionService.Change` so branch switching no longer carries separate sibling-worktree adoption blocks.
  - Refetched the configured remote after adoption so strict sync ahead checks use the pushed sibling state.
  - Added focused strict-sync regression coverage for a dirty sibling worktree on the requested PR branch.
  - `make ci` passed locally.
- [x] [B008] (P1) `.gitignore` hides new source and test files by default.
  Requested on 2026-05-21 after new `gix cd` implementation files were present on disk but only visible under ignored status because `.gitignore` used a catch-all `*` with a narrow allowlist.
  ## Observation
  - The catch-all ignore shape made normal implementation files invisible to `git status`.
  - That increased the chance of shipping partial changes unless new files were manually force-added.
  ## Deliverable
  - Replace the blanket ignore with explicit rules for OS/editor noise, local environment files, Go build/test artifacts, generated output, browser automation traces, and local planning scratch.
  - Ensure normal source, test, docs, config, and workflow files are trackable without force-add.
  ## Resolution
  - Rewrote `.gitignore` as an explicit project ignore list.
  - New Go source and integration test files now appear in regular `git status`.
- [x] [B009] (P1) `gix cd` fails when the target branch is already checked out in a sibling worktree.
  Requested on 2026-05-21 after `gix cd tyemirov/bugfix/B005-sample-title-center` failed in `/Users/tyemirov/Development/Hecate` because the branch was already used by `/Users/tyemirov/Development/Hecate-pr108`.
  ## Observation
  - Git refuses to switch a branch that is already checked out by another worktree.
  - The operator intent for `gix cd` is to make the requested branch active in the current checkout, automatically preserving and removing the disposable sibling worktree when safe.
  ## Deliverable
  - Detect sibling worktrees that own the requested branch before switching.
  - If the sibling worktree has uncommitted changes, stage all changes, generate a commit message, commit them, and push the branch to the configured remote.
  - If the sibling worktree is clean, skip the commit step; if it is clean but ahead of upstream, push before removal.
  - Remove the sibling worktree with `git worktree remove`, prune worktree metadata, then switch and pull the requested branch through the existing `gix cd` path.
  - Abort before removal when commit generation, commit, push, or worktree removal fails.
  ## Resolution
  - `gix cd` now recognizes the Git worktree collision error, adopts the sibling worktree for the requested branch, commits dirty sibling changes with the configured generated commit-message path, pushes preserved commits, removes the sibling with `git worktree remove`, prunes metadata, and retries the normal switch/pull flow.
  - Clean sibling worktrees skip the commit step; clean sibling worktrees that contain commits missing from the configured remote are pushed before removal, even when the branch has no upstream metadata.
  - Added black-box integration coverage for dirty sibling adoption and clean-ahead sibling adoption.
  - `make ci` passed locally.
- [x] [B010] (P1) `gix cd` skips remote fast-forward updates when the worktree has tracked local changes.
  Requested on 2026-05-10 after `gix cd` reported `refresh skipped (dirty worktree)` in `/Users/tyemirov/Documents/Projects/Fiction`, while a manual `git pull` immediately fast-forwarded `master`.
  ## Observation
  - Dirty tracked changes disable the clean-worktree refresh path and are passed through to the branch-change service as a full pull skip.
  - That prevents safe fast-forward updates that Git can apply when the remote changes do not overlap with local edits.
  ## Deliverable
  - Keep the clean-worktree refresh skip for tracked local changes, but still attempt a fast-forward-only pull after switching to the target branch.
  - Preserve warnings for real pull failures instead of turning conflicts into hard crashes.
  - Add black-box `gix cd` coverage proving unrelated remote changes land while local tracked edits remain.
  ## Resolution
  - Dirty refresh skips now use a fast-forward-only pull mode, leaving the strict clean-worktree refresh disabled while still accepting safe remote updates.
  - Added black-box coverage that keeps a tracked local edit, advances the remote on an unrelated file, and verifies `gix cd master` runs `git pull --ff-only`, lands the remote file, and preserves the local edit.
  - `make ci` passed locally.
- [x] [B011] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step.
  LegacyExternalID: GX-345
  (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)
  ## Resolution
  - Emit an initial per-repository discovery step summary and add workflow integration coverage for the discovery output.
- [x] [B012] (P0) gix prs delete --yes is silent under default console logging.
  LegacyExternalID: GX-346
  ## Analysis
  - The CLI command in `internal/branches/command.go` runs a workflow `TaskDefinition` that calls the `repo.branches.cleanup` action from `internal/branches/task_action.go`, so output is constrained to workflow reporting and the service logger rather than direct prints.
  - The cleanup action does not write to `environment.Output` (unlike `branch.refresh` in the same file), so it emits no explicit success or no-op line.
  - Workflow execution emits `TASK_PLAN` and `TASK_APPLY` events (`internal/workflow/task_plan.go`, `internal/workflow/task_execute.go`), but `internal/repos/shared/reporting.go` suppresses those event codes in console output via `consoleSuppressedEventCodes`.
  - The cleanup service logs progress only at Info level (`internal/branches/service.go`), while the default config in `cmd/cli/default_config.yaml` sets `log_level: error`, so those logs are filtered.
  - The summary renderer in `pkg/taskrunner/summary.go` returns an empty string for single-repo runs, leaving no fallback output.
  ## Deliverables
  - Emit a dedicated, non-suppressed console line for `gix prs delete` per repository that reports the outcome (deleted count or no-op) without requiring a log-level change.
  - Keep suppression of `TASK_*` workflow noise intact so other commands remain quiet; only add the explicit output needed for branch cleanup.
  - Extend `tests/pr_cleanup_integration_test.go` (or add a new adjacent integration test) to capture CLI output and assert it is non-empty for a single-repo `--yes` run.
  - Acceptance: With default config (`log_format: console`, `log_level: error`) and a single repo, the command prints at least one line that includes the repo identifier/path and an outcome.
  - Acceptance: When the GH CLI returns zero closed PR branches, output explicitly states a no-op or zero deletions instead of being silent. title=gix prs delete --yes is silent under default console logging)
  ## Resolution
  - Emit per-repo cleanup summaries (closed/deleted/missing/declined/failed) and add integration coverage for output and zero-branch runs.
- [x] [B013] Workflow file replacements skip some files when glob uses `**/` (suspected in configs/account-rename.yaml).
  LegacyExternalID: GX-354
  ## Investigation
  - `configs/account-rename.yaml` uses `files.apply` with `**/*.go` and `docs/**/*.md`.
  - `internal/workflow/task_plan.go` builds replacement targets via `compileReplacementMatcher`.
  - `compileReplacementMatcher` expands `**` to `.*` but still requires the following `/` in the pattern, so `**/*.go` compiles to `^.*/[^/]*\.go$` and does not match root files like `main.go`; similarly `docs/**/*.md` misses `docs/README.md`.
  - `internal/workflow/executor_runner.go` uses a channel + waitgroup for repo work but does not early-exit the worker loop, so a channel/workgroup premature exit looks unlikely.
  ## Repro
  - Run `gix workflow configs/account-rename.yaml --roots <repo> --yes` on a repo with root-level `main.go` (containing `github.com/temirov`).
  - Observe nested `pkg/**/*.go` files updated, but root-level `main.go` unchanged.
  ## Deliverable
  - Make `**/` match zero or more path segments so `**/*.go` includes root-level files and `docs/**/*.md` includes `docs/*.md`; add coverage for root-level matches.
  ## Resolution
  - Adjusted `**/` glob matching to allow root-level files and added regression coverage for `**/*.go` and `docs/**/*.md`.
- [x] [B014] `gix prs delete` reports `failed=<N>` when local PR branches are already gone (common case).
  LegacyExternalID: GX-355
  ## Observation
  - `gix prs delete --yes` runs `git push <remote> --delete <branch>` then `git branch -D <branch>`.
  - When a closed PR branch exists on the remote but not locally, `git branch -D` exits non-zero, causing the branch to be counted as `failed` even if remote deletion succeeded.
  ## Deliverable
  - Treat missing local branches as a no-op (still count the PR branch cleanup as successful when remote deletion succeeds).
  - When real failures occur, print a short stderr summary of failure reasons (bounded) so operators can diagnose without changing log level.
  ## Resolution
  - Treat missing local branches as already-clean, so successful remote deletions count as deleted.
  - Record failure details in the cleanup summary and print bounded failure samples to stderr when failures occur; added regression coverage.
- [x] [B015] (P1) `gix init --user` should initialize user config.
  Requested on 2026-06-29 after installing `github.com/tyemirov/gix@latest` at `v0.7.0` and running `gix init --user`, which failed with `unknown command "init" for "gix"`.
  ## Observation
  - The released setup documentation and discussion led users toward an initialization command, but the CLI only exposed initialization as the awkward root `--init` flag.
  - The natural command shape for writing `$HOME/.gix/config.yaml` is `gix init --user`.
  ## Resolution
  - Added a top-level `gix init` command that writes local `./config.yaml` by default and writes `$HOME/.gix/config.yaml` with `--user`.
  - Kept `gix --init user` unsupported; it now remains an unknown root flag instead of a compatibility path.
  - Kept `--force` on the init command for explicit overwrite and rejected conflicting `--local` plus `--user` selections.
  - Updated README, architecture notes, docs site copy, in-process tests, and black-box integration tests to use `gix init --user`.
  ## Validation
  - `go test ./cmd/cli ./tests`
  - `go run . init --user` with a temporary `HOME`
  - `go run . --init user` fails with `unknown flag: --init`
  - `make ci`
- [x] [B016] (P1) `gix sync` should push local-ahead work branches.
  Requested on 2026-06-29 after running `gix sync` on `tyemirov/bugfix/init-subcommand` and seeing `local branch "tyemirov/bugfix/init-subcommand" has commits not on origin/tyemirov/bugfix/init-subcommand`.
  ## Observation
  - Work branches are intended to be remote-backed and PR-backed.
  - A local-ahead work branch represents unpublished local work that `gix sync` should synchronize by pushing, not a terminal error.
  - A stale local branch whose PR already merged and has no commits beyond the base should still hand off to the base branch instead of recreating work.
  ## Resolution
  - Existing remote PR branches with local-ahead commits now merge the remote branch, merge the PR base, and push the local commits.
  - Local-only work branches with commits beyond the base now merge the base, push with upstream, and create the pull request.
  - Merged/pruned local branches with no commits beyond the base keep the existing prompt to sync the base branch.
  - Local-only branches with no commits beyond the base and no merged pull request handoff now stop before push or pull request creation.
  ## Validation
  - `go test ./internal/branches/syncflow`
  - `go test ./cmd/cli ./internal/branches/syncflow ./tests`
  - `make ci`
- [x] [B017] (P1) Dirty `gix sync master` should not conflict by merging `origin/master` after creating a generated branch.
  Requested on 2026-06-29 after `gix sync` in `/Users/tyemirov/Development/ISSUES.md` created `gix/support-agentic-model-and-reasoning-effort-settings-in` from dirty `master`, then failed with `CONFLICT (content): Merge conflict in .mprlab/ISSUES.md` while running `merge --no-edit origin/master`.
  ## Observation
  - Dirty base-branch sync is a preservation flow: create a generated work branch at the current checkout, commit the dirty tree there, then publish that branch through the PR flow.
  - After B016, the generated dirty branch reached the same local-only work-branch path as preexisting local branches and merged `origin/master` before push.
  - That base merge can conflict when local `master` was stale, but avoiding the merge would leave the generated branch unsynchronized with the current base.
  ## Resolution
  - Added an AI-backed merge conflict resolution service for strict sync merges.
  - When a strict sync merge stops with unmerged files, inspect the Git stages for each conflicted path, send BASE/OURS/THEIRS context to the configured LLM client, write the resolved file, stage it, and complete the merge with `git commit --no-edit`.
  - The resolver prompt explicitly preserves local OURS changes while integrating compatible remote THEIRS changes, and rejects LLM output that still contains conflict markers.
  - The resolver can now stage an intentional deletion with `git rm -f -- <path>` when the AI returns the canonical delete directive.
  - Dirty generated branches still merge `origin/master`; conflicts are resolved as part of that merge before push and PR creation.
  ## Validation
  - `go test ./internal/branches/syncflow`
  - `git diff --check`
  - `make test`
  - `make lint`
  - `make ci`
- [x] [B018] (P1) `gix sync` should open a pull request for an existing remote branch with real changes and no open PR.
  Requested on 2026-07-05 after `gix sync` in `/Users/tyemirov/Development/social_threader` on branch `gix/add-ci-for-mobile-app-with-coverage-and-config-checks` failed with `branch "gix/add-ci-for-mobile-app-with-coverage-and-config-checks" does not have an open pull request`, even though the branch had a diff against `origin/master`.
  ## Observation
  - Strict sync handled local-only branches with commits beyond the base by pushing and creating a pull request.
  - When the remote branch already existed, strict sync checked only for an open PR and a merged-PR handoff before returning the missing-PR error.
  - That skipped the real branch-diff check for `origin/<branch>` and refused to create a PR for an already-pushed work branch.
  ## Resolution
  - Existing remote branches with no open PR now check both `origin/<base>..origin/<branch>` and the local branch before falling back to merged-PR handoff.
  - When either ref has commits beyond the base, strict sync switches to the branch, aligns it with the remote branch, merges the base, generates PR metadata from the diff, pushes, and opens the PR.
  - Branches with no base delta still keep the merged-PR handoff and true missing-PR error paths.
  ## Validation
  - `go test ./tests -run TestSyncExistingRemoteBranchWithoutPullRequestCreatesPullRequest -count=1`
  - `git diff --check`
  - `make test`
  - `make lint`
  - `make ci`
- [x] [B019] (P1) `gix sync` should hand off merged PR branches before creating new PRs and should honor explicit `master` sync from linked worktrees.
  Requested on 2026-07-06 after `gix sync` on `improvement/I053-provider-history-handles` reported no open pull request instead of prompting to sync `master`, and `gix sync master` then failed while trying to remove `/Users/tyemirov/Development/MediaOps` even though that path is the main working tree.
  ## Observation
  - The existing-remote no-PR path checked branch diffs before checking merged pull requests, so squash-merged branches could look like new review branches because their commits were not ancestors of `origin/master`.
  - Explicit base-branch sync used the sibling-worktree adoption flow when `master` was already checked out elsewhere, and that flow always attempted `git worktree remove`, which Git rejects for a main working tree.
  ## Resolution
  - Strict sync now checks merged pull request handoff before creating a pull request for an existing remote branch with no open PR.
  - If the merged-PR prompt is declined, sync keeps the missing-open-PR error instead of opening a new PR for the already-merged branch.
  - Worktree adoption now preserves any sibling worktree changes, pushes when needed, and detaches a sibling main worktree instead of trying to remove it; removable linked worktrees still use the existing remove-and-prune path.
  ## Validation
  - `go test ./tests -run 'TestSync(CurrentMergedBranchPromptsAndSyncsMasterBeforeCreatingPullRequest|ExplicitMasterReleasesMainWorktreeAndSwitchesLinkedWorktree|ExistingRemoteBranchWithoutPullRequestCreatesPullRequest)' -count=1`
  - `git diff --check`
  - `make test`
  - `make lint`
  - `make ci`


## Improvements

- [x] [I001] Improve the workflow summary.
  LegacyExternalID: GX-251
  I ran @configs/account-rename.yaml and I got:
  Summary: total.repos=104 PROTOCOL_SKIP=104 REMOTE_MISSING=1 REMOTE_SKIP=51 REPO_FOLDER_SKIP=52 REPO_SWITCHED=92 TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582 WORKFLOW_STEP_SUMMARY=582 WARN=139 ERROR=1 duration_human=6m55.109s duration_ms=415109
  Remove duration_ms Leave only human duration and rename it to duration.
  remove  TASK_APPLY=237 TASK_PLAN=191 TASK_SKIP=139 WORKFLOW_OPERATION_SUCCESS=582
  add missing steps in the summary (like namespace rewrite, namespace delete etc)
- [x] [I002] Add steps to @configs/account-rename.yaml that allows to bump up the dependency versions of go.mod (see GX-110).
  LegacyExternalID: GX-252
- [x] [I003] Add steps to @configs/account-rename.yaml to upgrade go version in go.mod to `go 1.25.4`.
  LegacyExternalID: GX-253
- [x] [I004] Embed license templates and wire the license workflow preset to render them per repository.
  LegacyExternalID: GX-254


## Maintenance

- [ ] [M001R] (P2) Backlog hygiene and archive.
  Goal:
  Keep the issue tracker reliable, readable, and focused on active work while preserving resolved history in the appropriate archive.
  Requirements:
  - Cadence: run weekly during active development and before each release cut.
  - Validate section names, identifier prefixes, recurrence suffixes, priority markers, dependencies, and duplicate IDs against the current `issues-md-format.md`.
  - Reconcile stale statuses, duplicate issues, broken references, obsolete instructions, and entries filed under the wrong section.
  - Move completed non-recurring history to the repository issue archive or durable documentation when the active tracker becomes noisy.
  - Keep active, blocked, planning, and recurring entries visible in `.mprlab/ISSUES.md`.
  Deliverables:
  - Normalized `.mprlab/ISSUES.md` structure and statuses.
  - Updated issue archive or docs when completed entries are removed from the active tracker.
  - A short `Last run:` note summarizing the cleanup and any follow-up issues filed.
  Validation:
  - Re-read `.mprlab/ISSUES.md` after edits and confirm every issue is under the right section with a unique section-aware ID.
  - Confirm recurring entries remain open and keep the `R` suffix.
  - Confirm no active, blocked, recurring, or planning work was archived.
  Last run: 2026-06-29.
- [ ] [M002R] (P2) Polish open issues.
  Goal:
  Keep unresolved work executable by making each open issue concrete, ordered, and testable.
  Requirements:
  - Cadence: run weekly during active development and before handing a repo to automated execution.
  - Review every unresolved non-recurring issue for missing context, dependencies, repro steps, acceptance criteria, and validation expectations.
  - Make priorities concrete and ensure each open issue has actionable deliverables.
  - Merge duplicate open issues or add explicit dependency links when separate entries must remain.
  - Do not close or implement issues as part of this polish pass unless that work is separately requested.
  Deliverables:
  - Open issues with enough detail for a person or agent to execute without rediscovery.
  - New or updated dependency markers where ordering matters.
  - A short `Last run:` note listing the number of issues polished and any blockers found.
  Validation:
  - Sample the open entries after the pass and confirm each has clear next actions and validation expectations.
  - Confirm no recurring runbook was marked complete.
  - Confirm duplicates were merged or explicitly cross-referenced.
- [ ] [M003R] (P2) Architecture and policy review.
  Goal:
  Catch architecture, policy, and workflow drift before it becomes hidden maintenance debt.
  Requirements:
  - Cadence: run monthly, before large refactors, and after major framework or runtime changes.
  - Review the codebase, docs, and workflow against `AGENTS.md`, `.mprlab/POLICY.md`, relevant `.mprlab/AGENTS.*.md` guides, and the current architecture notes.
  - Look for drift from forward-only contracts, edge-validation boundaries, smart-constructor usage, testing policy, and module ownership.
  - Record findings as new Maintenance issues with concrete scope, priority, and validation.
  - Close the pass with a no-action note only when the review finds no actionable drift.
  Deliverables:
  - New Maintenance issues for each actionable architecture or policy drift finding.
  - Updated notes on areas reviewed and areas intentionally left unchanged.
  - A short `Last run:` note with the review scope and outcome.
  Validation:
  - Confirm every finding is represented as an issue with owner-readable context and validation criteria.
  - Confirm no implementation changes were mixed into the review runbook unless separately requested.
  - Confirm all recurring runbooks remain open.
- [ ] [M004R] (P1) Dependency and security audit.
  Goal:
  Keep third-party dependencies, runtime versions, and security-sensitive configuration within the current supported contract.
  Requirements:
  - Cadence: run weekly for active apps and before each release cut.
  - Inspect package managers, lockfiles, language toolchains, container bases, and generated clients for known vulnerabilities or stale direct dependencies.
  - Review auth, secret, CORS, CSP, SQL, network, and permission-sensitive configuration for drift from the current contract.
  - Prefer current supported dependencies; do not add compatibility shims for obsolete dependency behavior.
  - File separate Maintenance or BugFix issues for each actionable vulnerability, unsupported runtime, or security-contract gap.
  Deliverables:
  - Documented audit commands or data sources used for the pass.
  - Updated issues for each actionable dependency or security finding.
  - A short `Last run:` note with clean result or follow-up issue IDs.
  Validation:
  - Rerun the repository-native audit, lint, or dependency checks used for the pass.
  - Confirm every finding is either filed, fixed under a separate issue, or explicitly marked not applicable with evidence.
  - Confirm no secrets or private payloads were written into the tracker.
- [ ] [M005R] (P1) CI, release, and artifact health.
  Goal:
  Keep the repository's validation, release, publication, and generated artifact surfaces trustworthy.
  Requirements:
  - Cadence: run before every release, publish, or deploy, and weekly for critical services.
  - Verify repository-native CI, lint, format, coverage, release, publish, Docker image, Pages, and artifact workflows still match the documented contract.
  - Check generated artifacts, release tags, published images, and Pages outputs for source-to-public drift.
  - File concrete follow-up issues for failing gates, stale artifacts, missing release prerequisites, or undocumented workflow changes.
  - Do not perform production deployment from this runbook unless the operator explicitly requests that deployment.
  Deliverables:
  - Recorded gate status and artifact surfaces inspected.
  - Follow-up issues for each reproducible CI, release, publish, or artifact drift problem.
  - A short `Last run:` note with commands run and any skipped surfaces.
  Validation:
  - Use repository-native `make` targets or documented release helpers for checks.
  - Confirm release and deployment ownership boundaries remain separate.
  - Confirm public or published artifacts match the intended source revision when that surface is inspected.
- [ ] [M006R] (P1) Code contract and static hygiene.
  Goal:
  Keep source contracts explicit, current, and statically guarded against policy drift.
  Requirements:
  - Cadence: run monthly and before large refactors.
  - Scan for dead code, unused exports, duplicated literals, silent fallbacks, legacy aliases, compatibility reads, and zero-but-invalid domain states.
  - Check static analysis, coverage, schema, and contract guards that are supposed to prevent drift.
  - File focused Maintenance issues for each concrete violation instead of broad cleanup placeholders.
  - Keep the current canonical contract only; do not preserve obsolete behavior unless a product requirement explicitly says so.
  Deliverables:
  - Issue entries for each actionable static hygiene or contract violation.
  - Notes on static tools, searches, and contract guards used during the pass.
  - A short `Last run:` note with clean result or follow-up issue IDs.
  Validation:
  - Rerun the relevant static checks, contract tests, or repository searches used to identify drift.
  - Confirm every finding has a narrow follow-up issue and does not duplicate existing backlog work.
  - Confirm no implementation changes were mixed into the audit unless separately requested.
- [ ] [M007R] (P1) Production drift and health.
  Goal:
  Detect when production, public, or scheduled runtime state has drifted from the intended repository contract.
  Requirements:
  - Cadence: run weekly for deployed services and after each publish or deploy.
  - Compare current source, runtime configuration, published images, public routes, scheduled jobs, and health checks for drift.
  - Inspect real operator-facing surfaces rather than assuming merged source is deployed.
  - File follow-up issues for stale images, stale Pages output, missing routes, failed monitors, invalid production config, or undocumented runtime differences.
  - Stop before production deploy or destructive operator actions unless the operator explicitly requests them.
  Deliverables:
  - Recorded source revision, public artifact, route, image, or health surfaces inspected.
  - Follow-up issues for each source-to-runtime drift finding.
  - A short `Last run:` note with evidence links or commands used.
  Validation:
  - Verify inspected production or public surfaces directly where access is available.
  - Confirm any deploy-required finding is filed with the exact publish/deploy boundary and owner.
  - Confirm no production state was changed by the audit unless explicitly requested.
- [ ] [M008R] (P2) Documentation and runbook hygiene.
  Goal:
  Keep durable documentation and runbooks aligned with the current behavior users and operators actually rely on.
  Requirements:
  - Cadence: run before release cuts and after merge bursts that change user-facing or operator-facing behavior.
  - Review README, ARCHITECTURE, PRD, CHANGELOG, docs, runbooks, setup guides, and local workflow notes for stale behavior or missing new contracts.
  - Update docs when closed issues changed durable behavior, public APIs, operator workflows, release semantics, or deployment expectations.
  - Remove or rewrite stale instructions instead of preserving obsolete alternatives.
  - File separate issues for documentation gaps that require product or implementation decisions.
  Deliverables:
  - Updated documentation or filed follow-up issues for each gap.
  - A short `Last run:` note listing docs inspected and changes made.
  - Cross-references from archived issue history to durable docs when useful.
  Validation:
  - Check links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.
- [ ] [M009R] `make ci`/`check-format` emits a Go parse error for `tools/llm-tasks/tasks/sort/task_test.go`.
  LegacyExternalID: GX-424
  ## Observation
  - `gofmt -l` prints `tools/llm-tasks/tasks/sort/task_test.go:60:2: expected declaration, found base`.
  ## Deliverable
  - Fix the invalid test file or exclude it from `check-format` so formatting checks run cleanly.
- [x] [M010R] Consolidate MPR Lab governance under `.mprlab/`.
  Requested on 2026-06-29.
  ## Resolution
  - Moved the active tracker, archive, and process notes into `.mprlab/`.
  - Generated current governor-managed policy, planning, Git, Go, and frontend guides.
  - Removed legacy `issues.md/` and `.mprl/` folders.
  - Discarded the stale tracked active plan and obsolete Docker, Python, and legacy root agent guides that do not match this repository profile.
  - Updated root guidance, docs, and changelog references to the current `.mprlab/` paths.
  ## Validation
  - `/Users/tyemirov/Development/Smith/mprlab-governor/scripts/normalize-mprlab --repo /Users/tyemirov/Development/gix --check`
  - `git diff --check`
- [x] [M011R] (P2) Document user config initialization and configuration scope.
  Requested on 2026-06-29 after reviewing whether the advertising page and README explain `gix init --user`, `$HOME/.gix/config.yaml`, and what config controls.
  ## Observation
  - README listed user config initialization and basic precedence, but did not explain the config's practical control surface.
  - The docs site omitted the user config path and linked to the old root `ISSUES.md`.
  ## Resolution
  - Updated README Quick Start and configuration essentials to describe `gix init --user`, local `gix init`, `--force yes`, and `$HOME/.gix/config.yaml`.
  - Documented that config controls shared logging, confirmation, clean-worktree behavior, roots/remotes, sync PR metadata, LLM transport/provider settings, release/audit defaults, and workflow defaults.
  - Updated the docs site getting-started flow, developer-tooling copy, architecture copy, and roadmap link to reflect current config and `.mprlab/ISSUES.md` contracts.
- [x] [M012R] (P2) Add a global LLM transport switch for user config.
  Requested on 2026-06-29 after the operation-level LLM Proxy config felt too involved for a user who only wants gix to use MPR LLM Proxy.
  ## Observation
  - User initialization generated separate LLM transport, provider, env, base URL, and model settings under each message operation.
  - A user had to edit multiple fields to express one global preference.
  - Changing only `transport` in an operation could leave inherited env/base settings from another transport.
  ## Resolution
  - Added a top-level `llm` config block for shared LLM defaults.
  - Updated message, changelog, sync, and web LLM config assembly so `llm.transport: llm_proxy` selects `LLM_PROXY_SECRET` and the MPR LLM Proxy endpoint automatically.
  - Kept per-operation LLM fields as overrides and made transport-only operation overrides reset env/base defaults to the selected transport.
  - Preserved `llm.provider` as the upstream provider value passed through to LLM Proxy without requiring a provider-specific API key in gix.
  - Updated generated config, README, architecture notes, docs site copy, changelog, and focused application configuration tests.
- [x] [M013R] (P2) Keep user configuration under `$HOME`.
  Requested on 2026-06-29 after deciding not to honor `$XDG_CONFIG_HOME` for gix user configuration.
  ## Observation
  - The application searched XDG-derived paths before `$HOME/.gix/config.yaml`, including the indirect `os.UserConfigDir()` path.
  - README, architecture notes, the docs site, CLI help, and changelog notes still described XDG-based user config paths.
  ## Resolution
  - Removed XDG-derived user config discovery and kept `$HOME/.gix/config.yaml` as the only user-level config path.
  - Updated README, architecture notes, docs site copy, CLI design notes, CLI help text, changelog, and application tests to reflect the HOME-only contract.


## Features

- [x] [F001] Add a step that allows running an arbitrary command, such as `go get -u ./...` and `go mod tidy`.
  LegacyExternalID: GX-110
  The changed files need to be committed after this step. Deliver both the DSL and the implementation.
- [x] [F002] Add a local web interface for `gix`.
  Requested on 2026-03-05.
  ### Summary
  Launching `gix --web <port:-8080>` should start a local HTTP server that exposes the existing CLI command surface through a browser UI, rather than requiring users to compose all operations in the terminal.
  ### Constraints
  - Reuse the existing command implementations rather than forking a second execution stack.
  - Bind locally by default and treat the web UI as a localhost tool, not a remote multi-user service.
  - Preserve the current CLI behavior and tests while adding the web mode behind an explicit flag.
  - Support every current CLI command at least through a generic command runner, even if richer typed forms arrive incrementally.
  ### Deliverables
  - [ ] Add a root `--web` launch mode with default port `8080`.
  - [ ] Serve a browser UI and JSON API from a local embedded HTTP server.
  - [ ] Execute existing `gix` commands in-process from the web layer and stream logs/results back to the browser.
  - [ ] Expose command metadata so the UI can render flags, args, defaults, and help text.
  - [ ] Add integration coverage for web launch, command catalog, and at least one end-to-end command execution path.
- [x] [F003] (P1) Duplicate logging.
  LegacyExternalID: GX-111
  ### Summary
  When the `gix cd` command fails (for example, due to local changes blocking a branch switch), the error message is printed twice in the terminal. This duplicate logging clutters the output and violates the repository's principle of structured, single-entry reporting.
  ### Analysis
  The duplication is caused by a lack of coordination between the domain service, the workflow executor, and the CLI entry point:
  1.  **Improper Error Types**: `internal/branches/syncflow/service.go` returns standard errors via `fmt.Errorf` instead of the structured `repoerrors.OperationError` type. This prevents the workflow layer from identifying the error as a handled repository event.
  2.  **Fallback Printing**: In `internal/workflow/executor_runner.go`, the function `executeRepositoryStageForRepository` attempts to log the error. Because it is not an `OperationError`, it fails the check in `logRepositoryOperationError` (found in `internal/workflow/error_handling.go`) and falls back to a manual `fmt.Fprintln` to `stderr`.
  3.  **CLI Exit Redundancy**: The error is then bubbled up to `main.go`. Since the command's `RunE` returns the error, the main function prints it a second time before exiting.
  4.  **Context Loss**: The `collectOperationErrors` helper in `internal/workflow/executor.go` unwraps the error chain too aggressively, resulting in the same underlying Git error being printed in both instances, stripped of its high-level context (e.g., "Switch branch to master").
  ### Deliverables
  - [ ] **Structured Error Implementation**: Refactor `internal/branches/syncflow/service.go` to utilize `repoerrors.Wrap` for all Git-related failures.
  - [ ] **Reporting Logic Alignment**: Update `internal/workflow/error_handling.go` to ensure that all repository-scoped errors are processed via the `StructuredReporter`, eliminating the need for manual fallback printing.
  - [ ] **CLI Exit Refinement**: Adjust the CLI execution flow to ensure that errors already emitted by the reporter do not trigger a second print at the application exit point.
  - [ ] **Verification**: Add an integration test case that triggers a predictable Git failure and asserts that the resulting error message appears exactly once in the combined output stream.
- [x] [F004] Audit in the web client needs a first-class inspection flow with user-selected roots instead of parsing CLI stdout.
  Requested on 2026-03-08 while refining the `gix --web` audit UX.
  Resolved on 2026-03-08 by adding `POST /api/audit/inspect`, wiring `audit.Service.DiscoverInspections` into the web launcher, adding explicit audit-root controls in the browser, and rendering typed audit rows directly in the web table while keeping the generic runner available for raw CLI execution.
  ### Summary
  The current web interface treats audit as a generic command run and renders the CSV only after `gix audit` executes through the runner. That blocks the next UX steps because the browser has no typed audit model, no independent root selection, and no stable contract for row-level actions.
  ### Analysis
  - `internal/web/server.go` currently exposes repositories, branches, commands, and generic runs only. There is no dedicated audit endpoint.
  - `internal/web/ui/assets/app.js` renders audit results by parsing `stdout` from a `RunSnapshot`, so the browser only sees CSV text rather than typed inspection rows.
  - `cmd/cli/application_web.go` launches the web server with repository discovery rooted in the process working directory. That startup catalog is useful as context, but it is not a substitute for explicit audit roots.
  - The existing audit service in `internal/audit/service.go` already provides the right backend primitive: `DiscoverInspections(ctx, roots, includeAll, debug, depth)` returns typed repository inspections before CSV formatting.
  - The remediation UX requested by the user depends on stable row identity (`path`, `folder_name`, remote state, branch state), which should come from JSON instead of CSV parsing.
  ### Deliverables
  - [x] Add a dedicated web audit inspection API (for example `POST /api/audit/inspect`) that accepts explicit roots plus `include_all`.
  - [x] Return typed audit rows from the backend rather than only CSV text.
  - [x] Add audit-root controls in the web UI so the operator can inspect arbitrary roots without relaunching `gix --web`.
  - [x] Preserve the generic command runner for raw CLI access; the audit workspace should not depend on it for table rendering.
  - [x] Add server/API and browser coverage for user-selected roots and typed audit table rendering.
- [x] [F005] Audit remediation in the web client must be modeled as a pending change queue before execution.
  Requested on 2026-03-08 while defining follow-up UX after the audit table landed.
  Resolved on 2026-03-08 by adding a typed pending-change queue to the web audit workspace, surfacing editable per-item options in the queue, and applying queued changes in a deterministic order so path-changing rename operations run after repository-state fixes.
  ### Summary
  Row-level fixes in the browser should never execute immediately. The user wants every remediation action to become a queued pending change first, with explicit review before apply.
  ### Analysis
  - The current web UI has draft behavior for some commands (`draft_template` usage in `internal/web/ui/assets/app.js`), but it does not have a generalized queue, conflict resolution, or a typed pending-change model.
  - Existing repository operations already exist in the workflow/task layers for rename, canonical remote update, protocol conversion, and default-branch promotion. The missing piece is a browser-side queue model plus a backend execution contract for batched pending items.
  - Some requested actions conflict by construction. Example: deleting a folder conflicts with any queued fix for the same path; multiple remote fixes for one repo should merge or be rejected; sync should be blocked when destructive changes are pending for the same repository.
  - If the queue is modeled as raw argv strings, the browser will be forced to reverse-engineer conflicts and capabilities from command text. The queue needs typed operations instead.
  ### Deliverables
  - [x] Define a typed pending-change model shared by web UI and backend execution.
  - [x] Add queue operations in the UI: add, edit options, remove, clear, and review.
  - [x] Reject or merge conflicting queued operations deterministically.
  - [x] Execute queued changes through typed backend handlers or workflow operations rather than ad hoc shell text.
  - [x] Re-run audit for affected roots after queue execution and refresh the table from typed results.
- [x] [F006] Audit output must report remote presence explicitly in both CLI and web/API contracts.
  Requested on 2026-03-08 after reviewing how missing remotes appear in audit results.
  Resolved on 2026-03-08 by extending audit and workflow CSV output with `origin_remote_status`, making missing remotes emit `missing` with `n/a` protocol/canonical fields, and updating the web inspection API plus integration coverage to surface the status directly.
  ### Summary
  When a repository has no `origin`, audit should say so explicitly. Today the CLI/web output exposes enough indirect fields to infer the situation, but the contract does not contain a dedicated remote-status field, which makes the UX ambiguous and blocks correct remediation suggestions.
  ### Analysis
  - `internal/audit/service.go` already distinguishes the no-remote case internally by returning an inspection with empty `OriginURL`, empty owner/repo fields, and `OriginMatchesCanonical = n/a`.
  - The CSV/report contract drops that distinction. Operators only see related columns such as `final_github_repo`, `remote_protocol`, and `origin_matches_canonical`, none of which explicitly say `no remote configured`.
  - The web client currently labels audit state from parsed CSV, so it inherits the same ambiguity.
  - A dedicated remote-status field is the lowest-risk contract extension because it avoids overloading `origin_matches_canonical` with two meanings.
  - This change must be wired through every audit output path, including direct audit CSV and workflow-backed audit reports in `internal/workflow/operations_audit.go`.
  ### Deliverables
  - [x] Add an explicit audit field/column for origin remote status (for example `configured`, `missing`, `n/a`).
  - [x] Ensure missing-remotes render as such in the web table and are not framed as canonical-remote mismatches.
  - [x] Update CLI and integration tests that assert audit CSV headers and rows.
  - [x] Keep current `origin_matches_canonical` semantics for real remotes; do not repurpose it to mean remote absence.
- [x] [F007] The web audit workspace needs a UX-only folder deletion operation, queued before apply.
  Requested on 2026-03-08 as part of the row-action audit UX.
  Resolved on 2026-03-08 by surfacing a web-only `delete_folder` row action in the audit table, requiring explicit queue confirmation before apply, and covering the queue/apply/refresh flow with a browser test. The current scope intentionally allows deleting any audited folder path from the web queue, including repository folders, while still blocking filesystem-root deletion in the backend.
  ### Summary
  The browser should support deleting a folder directly from the audit workspace, but only as a queued web action rather than a new immediate CLI behavior.
  ### Analysis
  - There is no existing CLI command dedicated to removing an inspected folder from disk. The existing `repo-history-remove` flow rewrites Git history and is unrelated.
  - Because folder deletion is destructive and outside the existing command surface, it should remain web-only until the UX, safety model, and scope are validated.
  - The most sensitive design choice is whether deletion applies only to non-git folders discovered via `include_all` or also to full repositories. The request currently says “delete a folder that matches the repo altogether,” which implies repository deletion may be intended, but that requires stronger guardrails than a simple button.
  - The queue model from [F005] is a prerequisite because this action must never execute immediately from a table row.
  ### Deliverables
  - [x] Define a web-only queued delete-folder operation with explicit confirmation requirements.
  - [x] Restrict or phase the scope intentionally (for example non-git folders first, then repositories if approved).
  - [x] Surface the operation only in the web audit workspace, not in the generic CLI runner.
  - [x] Add end-to-end tests that verify deletion is queued first and only applied after explicit confirmation.
- [x] [F008] The web audit queue needs repository sync and protocol-fix actions backed by existing operations.
  Requested on 2026-03-08 as part of the audit remediation UX.
  Resolved on 2026-03-08 by surfacing queued protocol-fix and sync row actions in the web audit table, adding editable target-protocol and dirty-worktree-policy controls in the queue, and covering the queue/apply/refresh flow with a browser test.
  ### Summary
  The audit table should let operators queue protocol fixes and “sync local with remote” changes directly from mismatched rows.
  ### Analysis
  - Protocol conversion already exists through `remote update-protocol` and `ProtocolConversionOperation`.
  - Local/remote sync is less direct. `gix default` promotes a branch to the remote default branch and changes GitHub configuration; that is too heavy for a generic “bring local into sync” table action. The more appropriate existing behavior is closer to `branch.change` / `gix cd`, which fetches, switches, and rebases for repository roots.
  - The queue UX therefore needs a clearer action taxonomy than the current audit columns provide. A row with `in_sync = no` should produce a queue item that describes the exact operation that will run, branch target, and dirty-worktree policy.
  - These fixes should refresh audit state after apply so the table becomes the post-change source of truth.
  ### Deliverables
  - [x] Add queued protocol-fix actions backed by the existing protocol conversion operation.
  - [x] Add queued local-sync actions backed by an explicit branch-refresh/change workflow, not default-branch migration.
  - [x] Surface per-item options needed for safe execution (branch target, dirty-worktree handling, protocol target).
  - [x] Add integration/browser coverage that verifies queueing plus execution updates the table state.
  - Update on 2026-03-08 for [F005].
  Implemented the queue foundation without closing the full remediation program.
  ### What Landed
  - Added a typed apply contract at `POST /api/audit/apply` through `internal/web/types.go`, `internal/web/server.go`, and `cmd/cli/application_web.go`. The browser now sends structured queued changes instead of argv text.
  - Added a browser-side pending-change model in `internal/web/ui/assets/app.js` with queue add/remove/clear/apply behavior, queue summary rendering, and post-apply audit reinspection against the active audit roots.
  - Added backend execution mapping for queued change kinds to existing workflow/task primitives:
    - `rename_folder` -> `workflow.RenameOperation`
    - `update_remote_canonical` -> `workflow.CanonicalRemoteOperation`
    - `convert_protocol` -> `workflow.ProtocolConversionOperation`
    - `sync_with_remote` -> `branch.change` task action with refresh enabled
    - `delete_folder` -> web-only filesystem deletion path guarded by `confirm_delete`
  - Added end-to-end coverage in `cmd/cli/application_web_test.go` and `cmd/cli/application_web_browser_test.go` for queue submission and rename apply flow.
  ### Queue Semantics Implemented
  - The queue is typed by `kind` plus repository `path`, with stable queue IDs generated in the browser.
  - Re-queueing the same `kind` for the same `path` replaces the existing queued item in place instead of duplicating it.
  - `delete_folder` is treated as exclusive for a path: if deletion is already queued, other fixes for that path are rejected; if other fixes exist, deletion is rejected until they are removed.
  - Successful apply results are removed from the pending queue; failed items remain queued.
  - After apply, the web client reruns typed audit inspection for the previously inspected roots so the table reflects post-change state instead of stale pre-apply rows.
  ### Completion Notes
  - The queue now exposes editable options for rename, delete, protocol conversion, and sync items.
  - The browser now surfaces row actions for rename, canonical-remote fixes, protocol fixes, sync, and web-only folder deletion.
  - Queue application is ordered deterministically so repository-state fixes run before rename and delete, avoiding stale-path execution during multi-step remediation.
  - Update on 2026-03-08 for review follow-up on [F005], [F007], and [F008].
  Addressed three post-review correctness regressions in the queued web audit flow and locked them with regression coverage before patching.
  ### Summary
  The initial queued-remediation implementation had three edge-case failures: apply could refresh the wrong audit scope, workflow-backed changes that reported `skipped` were surfaced as successful, and the apply endpoint accepted relative paths for destructive operations.
  ### Analysis
  - The browser stored the last successful audit rows, but `applyAuditQueue()` refreshed via `inspectAuditRoots(false)`, which rebuilt the request from the live form and repository catalog. That was incorrect because queued changes semantically belong to the last inspected scope, not whatever the controls happen to contain at apply time.
  - This mismatch was especially dangerous after path-changing operations. If the operator edited the roots input after queueing, or if the repository catalog was stale relative to rename/delete actions, the table refresh could show an unrelated scope or fail to reflect the just-applied changes.
  - The Go apply executor treated `nil` workflow errors as unconditional success. That was too optimistic because workflow/task execution already communicates `skipped` outcomes through `workflow.ExecutionOutcome.ReporterSummaryData.StepOutcomeCounts`, and some remediation actions intentionally skip without returning a hard error.
  - Surfacing `skipped` as `succeeded` caused the browser to drop queued items that had not actually been applied and to report a misleading success state to the operator.
  - `normalizeWebAuditChangePath` only trimmed and cleaned the submitted path. That allowed relative inputs such as `../sibling` to escape the inspected directory context at the API boundary, which is unacceptable now that `/api/audit/apply` can drive destructive filesystem operations.
  ### Deliverables
  - [x] Added a browser regression test that queues a rename, mutates the roots input, applies the queue, and verifies the post-apply refresh still uses the last inspected audit scope.
  - [x] Split audit inspection in the web client into “build request from controls” and “rerun a saved request,” then updated queue apply to re-inspect with `state.auditInspectionRoots` and `state.auditInspectionIncludeAll`.
  - [x] Added backend regression coverage for relative-path rejection and for mapping workflow execution outcomes to `succeeded` vs `skipped` vs `failed`.
  - [x] Changed workflow-backed apply execution to derive result status from `workflow.ExecutionOutcome`, preserving skipped items in the queue and avoiding misleading success messages.
  - [x] Hardened `/api/audit/apply` path normalization so only absolute paths are accepted for queued audit changes.
  - Update on 2026-03-08 for browser-test harness stability in CI.
  Addressed a CI-only Chrome startup failure that surfaced after the audit browser suite expanded.
  ### Summary
  The browser integration suite could fail in Linux CI even when a Chrome executable was present because the process exited during startup with crashpad-related output before DevTools became available.
  ### Analysis
  - The previous guard only skipped browser tests when no browser executable could be found. That was insufficient for CI runners where Chrome exists on disk but cannot boot cleanly in the available kernel/container environment.
  - The observed failure occurred before any page assertions ran and presented as `chrome failed to start` with crashpad file access errors under `/sys/devices/system/cpu/...`. That places the defect in the shared test harness, not in any specific browser test case.
  - Without a startup probe, each browser test assumed allocator creation implied a usable browser session. In practice, chromedp can return a startup failure only when the first action is executed, which makes the suite brittle and produces noisy failures unrelated to product behavior.
  - Disabling crash-reporting flags reduces the chance of environment-specific startup exits, while an explicit startup probe lets the suite distinguish “browser unavailable in this runner” from real DOM or workflow regressions.
  ### Deliverables
  - [x] Added a small regression test for the browser-startup skip classifier.
  - [x] Hardened the shared browser allocator with crash-reporting disable flags.
  - [x] Added an explicit `about:blank` startup probe in `newBrowserTestContext` and skip browser tests only when Chrome cannot start in the runner environment.
  - [x] Revalidated the previously failing protocol/sync browser test plus the full `make format`, `make test`, `make lint`, and `make ci` sequence.
  - Update on 2026-03-08 for web audit runner UX.
  Fixed a mismatch between the main Run button and the action-capable audit table.
  ### Summary
  The web client exposed two audit render paths: the Audit task’s typed inspection flow, which supports row actions, and the generic runner flow, which only parsed stdout into a read-only table. Operators could therefore click the main Run button while `gix audit` was selected, see an audit table, and still have no remediation actions available.
  ### Analysis
  - This was a real product bug, not just a discoverability issue. The initial command selection is `gix audit`, and the main runner affordance remained active, so the UI naturally suggested that running audit from there should produce the same actionable table as the task panel.
  - The generic runner path cannot support queued remediation safely because parsed audit stdout does not carry the typed row contract the browser uses for path-based actions.
  - Leaving both paths active meant the same visible command produced two materially different audit experiences, one actionable and one read-only, depending only on which button the user clicked.
  - The correct fix is to route `gix audit` through the typed inspection API regardless of whether the user clicks the Audit task button or the main Run button, and to label the button accordingly.
  ### Deliverables
  - [x] Added a browser regression that selects `gix audit`, clicks the main Run button, and asserts the resulting table includes row-action controls.
  - [x] Routed main-button audit execution through the typed inspection flow instead of the generic run API.
  - [x] Updated the main button label to `Inspect audit table` when `gix audit` is the selected command so the UX matches the actual behavior.
  - Update on 2026-03-08 for audit root selection from the editable draft.
  Fixed a mismatch between the editable `gix audit` argument draft and the actual inspection scope used by the Audit task.
  ### Summary
  The web UI allowed operators to edit the audit draft in the arguments textarea, but `Inspect audit table` still read only the dedicated Audit controls. Changing `--roots` in the visible draft therefore did not change the inspected folder, which made the UI show one audit command and execute another.
  ### Analysis
  - This was a state divergence bug inside the browser client. The audit task generated a draft command from `audit-roots-input` and `audit-include-all`, but manual edits to the shared arguments editor were not reflected back into those controls.
  - Once the main Run button was routed through typed audit inspection, this mismatch became more visible because the web interface effectively had two editable audit surfaces pointing at one operation.
  - The correct fix is to treat the supported typed audit flags in the draft (`--roots` and `--all`) as another valid edit surface for the Audit task and resolve inspection requests from that parsed draft when available.
  - Unsupported or incomplete draft arguments should not clobber the task controls; the parser therefore only synchronizes when it can read a coherent audit request.
  ### Deliverables
  - [x] Added a browser regression that edits `gix audit` arguments directly, clicks `Inspect audit table`, and verifies the inspected root matches the edited `--roots` value.
  - [x] Added browser-side parsing for `gix audit` draft arguments covering `--roots` and `--all`.
  - [x] Synchronized parsed audit draft arguments back into the Audit task controls so the visible task state and the draft command stay aligned.
  - [x] Resolved typed audit inspections from the parsed draft request when available, eliminating the scope mismatch between the draft editor and the Inspect action.
  - Update on 2026-03-08 for audit dirty-files column layout.
  Adjusted the audit table so the `Dirty Files` column stays narrow and wraps long file lists inside the cell instead of expanding the whole table width.
  ### Summary
  The typed audit table rendered `dirty_files` as an unconstrained content-width column, which let long file lists stretch the table horizontally and made the row harder to scan.
  ### Analysis
  - The table renderer already knew each logical audit column name, but it did not assign column-specific classes. Styling therefore had to treat every cell the same way.
  - The `Dirty Files` column has different content characteristics from the rest of the audit data: it is often a semicolon-separated list of paths and benefits from a constrained width with aggressive wrapping.
  - A targeted column class is the lowest-risk change because it keeps the rest of the table layout unchanged while making the path-heavy column fill vertical space instead of horizontal space.
  ### Deliverables
  - [x] Added a dedicated audit-table column class for `dirty_files` in both the typed and parsed audit table renderers.
  - [x] Applied a narrow fixed width plus line wrapping and word breaking to the `Dirty Files` column.
  - Update on 2026-03-08 for audit column-value filtering.
  Added per-column header filters to the web audit table so operators can narrow rows to exact values directly from the column headers.
  ### Summary
  The audit table had no interactive filtering, which made it cumbersome to isolate subsets like `name_matches = no`, `in_sync = no`, or `origin_remote_status = missing` before queueing remediation actions.
  ### Analysis
  - The typed audit table already had a stable set of logical columns, so the right place for filtering is in the header itself rather than in a separate global search surface.
  - Exact-value filters fit the audit data model well because many columns are categorical and operators typically want to isolate one specific state at a time.
  - The implementation needed to preserve the existing row-action UX, which means filtering should only change which rows are rendered and summarized, not mutate the underlying inspection rows or queue semantics.
  - Keeping filter state in the browser and re-rendering the table from the inspection rows is the lowest-risk approach because it does not alter the backend audit contract.
  ### Deliverables
  - [x] Added a browser regression that inspects multiple rows, filters the `Name Matches` column from the header, and verifies the table narrows to matching rows only.
  - [x] Added per-column header select controls for typed audit columns when more than one distinct value exists in the inspected rows.
  - [x] Added exact-value filtering and filtered row summaries (for example `1 of 2 rows`) without affecting queue actions or audit data.


## Planning
*do not implement yet*

- [x] [P001] Add a repository-tree explorer to the web interface so the left panel behaves like a filesystem explorer while only exposing Git repositories as selectable leaves.
  Requested on 2026-03-08.
  ### Summary
  The current web interface exposes repositories as a flat filtered list in the left panel. That does not scale well once the launch root contains many nested repositories, because path context is compressed into one line and operators cannot navigate by folder hierarchy. The requested UX is a Windows Explorer style tree in the left panel, but constrained to repository discovery: intermediate folders only exist to organize repositories, and only Git repositories are selectable targets.
  ### Current State
  - `GET /api/repos` already returns absolute repository paths in `RepositoryCatalog.Repositories`, so the browser has enough information to derive a folder hierarchy without a new backend endpoint.
  - The current frontend is a static HTML/CSS/ESM page (`internal/web/ui/index.html`, `internal/web/ui/assets/app.js`, `internal/web/ui/assets/styles.css`) with no package manifest, no bundler, and no frontend dependency pipeline.
  - The existing repo panel owns three coupled behaviors that must survive the tree migration: selected repository identity, checked-repository scope, and text filtering.
  - This issue explicitly chooses a runtime CDN dependency for the tree widget, so the implementation should treat jsDelivr-hosted Wunderbaum as the only supported tree-library path for this feature.
  ### Library Investigation
  - `Wunderbaum` is the chosen library for this issue. Its official quick-start supports direct jsDelivr loading for both CSS and JavaScript, and its tree model, keyboard support, filtering support, and plain-JS integration fit the current static frontend.
  - The official docs expose both a UMD script path and an ESM CDN path. Because the current page already loads `app.js` as a module, the cleanest implementation path is to keep `app.js` local and import Wunderbaum from jsDelivr into that module while loading the Wunderbaum stylesheet from jsDelivr in `index.html`.
  - Wunderbaum still documents itself as beta, so the browser-side tree adapter should isolate the rest of the control-surface code from direct widget-specific data shapes as much as practical.
  ### Recommendation
  - Use Wunderbaum from jsDelivr as the only tree library for this feature.
  - Derive a typed tree model in the browser from `RepositoryCatalog.Repositories`, then adapt that model into Wunderbaum nodes so repository semantics remain owned by the app instead of by the widget.
  - Load the Wunderbaum stylesheet from `https://cdn.jsdelivr.net/npm/wunderbaum@0/dist/wunderbaum.min.css`.
  - Load Wunderbaum JavaScript from the CDN ESM entrypoint `https://cdn.jsdelivr.net/npm/wunderbaum@0/+esm` from the local `app.js` module.
  ### Deliverables
  - [x] Replace the flat repo list in the left panel with a folder tree derived from repository paths.
  - [x] Preserve selected-repo behavior, checked-repo scope, and text filtering within the new tree UX.
  - [x] Ensure only repository leaves are selectable command targets; intermediate folders act as navigation/grouping nodes.
  - [x] Add browser coverage for expanding/collapsing folders, selecting a repository from the tree, and preserving checked scope state.
  - [x] Wire Wunderbaum from jsDelivr into the static web page and initialize it from the local browser-side tree model.
  ## Resolution
  - Resolved on 2026-03-08 by replacing the flat repository list with a Wunderbaum-backed folder tree in the web client, loading the widget stylesheet from jsDelivr in `index.html` and importing the ESM module from jsDelivr in `app.js`.
  - Added a browser-side folder/repository tree model derived from `RepositoryCatalog.Repositories`, kept repository selection and checked-scope state in the existing application model, and mapped those semantics onto Wunderbaum activation and checkbox events.
  - Preserved repository filtering and checked-scope workflows across tree updates, added local icon styling so the tree does not depend on an extra icon-font CDN, and covered the explorer behavior with browser and HTML integration tests.
  - Update on 2026-03-08 for [F009].
  Tightened the repository tree so the web client only exposes top-level Git repositories as selectable targets.
  ### Summary
  The initial tree implementation used the full discovered repository catalog, so a Git repository nested inside another Git repository still appeared in the left panel and leaked into `All` scope operations. That contradicted the intended UX of operating on top-level repositories only.
  ### Analysis
  - Filtering only at the tree renderer would have been insufficient, because the rest of the web client derives counts, checked scope, and `All` scope command roots from `state.repositories`.
  - The correct boundary for the fix is therefore the browser-side repository set loaded during initialization: once nested repositories are removed there, the tree, the repo count, and all repository-scoped commands stay consistent.
  - Top-level determination is path-based: a repository is excluded when its normalized path is a strict descendant of another discovered repository path.
  ### Deliverables
  - [x] Added a browser regression covering a top-level repository, a nested child repository under it, and an unrelated sibling repository.
  - [x] Filtered the browser-side repository set down to top-level repositories before selection, counts, tree rendering, and scope resolution.
  - [x] Revalidated `make format`, `make test`, `make lint`, and `make ci`.
  - Update on 2026-03-08 for [F009].
  Moved the repository explorer into a persistent left sidebar that occupies roughly one fifth of the desktop control surface and lets the folder tree fill the panel vertically.
  ### Summary
  The initial repository-tree delivery rendered the tree inside the old top-row repository card instead of as a true left explorer panel. That made the tree easy to miss, did not read as a file-explorer layout, and did not satisfy the requested 1/5-page sidebar UX.
  ### Analysis
  - The problem was structural, not data-related: the tree control already existed and rendered repository folders correctly, but it lived in the same top context row as the target cards.
  - A CSS-only tweak would not have been enough because the page DOM still treated the repository area as one card in a horizontal trio rather than as a dedicated navigation region.
  - The correct fix is to split the page into a sidebar/main workspace, keep the existing repository-state widgets inside the sidebar, and let the Wunderbaum container expand vertically inside that sidebar.
  - Desktop width should be enforced at the layout container so the explorer remains approximately 20% of the usable workspace, while mobile should still collapse to a single-column layout.
  ### Deliverables
  - [x] Added browser coverage that asserts the repository tree is rendered inside a dedicated left sidebar, appears to the left of the main workspace, and occupies approximately one fifth of the desktop layout width.
  - [x] Restructured the embedded HTML into a `workspace-layout` split with `repo-sidebar` and `workspace-main` regions.
  - [x] Updated CSS so the sidebar stays narrow on desktop, the repository tree fills the panel vertically, and mobile collapses back to one column.
  - Update on 2026-03-08 for [F009].
  Fixed current-repository launch mode so the explorer no longer collapses into a visually flat single-leaf row with no visible parent folder context.
  ### Summary
  When `gix --web` was launched from inside a repository, the explorer received a one-repository catalog and rendered it as a lone leaf named after the repo. In practice that looked like a blank panel with one thin row, which did not communicate a tree structure and made the repo entry feel non-interactive.
  ### Analysis
  - The sidebar layout itself was correct after the previous fix; the remaining problem was the browser-side tree model for `current_repo` launch mode.
  - The existing segment builder collapsed a repository whose path matched the launch path down to `[repo-name]`, so Wunderbaum had no parent folder node to render.
  - In current-repo mode the right UX is to synthesize a visible parent-folder context from the repository path itself, then expand that folder by default so the repo leaf is immediately visible.
  - Small cursor affordance changes help the tree read as interactive instead of as static text.
  ### Deliverables
  - [x] Added a browser regression covering current-repo launch mode and asserting the tree shows both the parent folder name and the repository leaf.
  - [x] Updated the tree-model builder so current-repo mode renders `parent-folder -> repository` instead of a single leaf.
  - [x] Auto-expanded current-repo folders and added pointer cursors for folder and repository tree rows.
  - Update on 2026-03-08 for [F009].
  Extended current-repo explorer navigation so clicking the top visible folder reveals the next ancestor above it instead of leaving the tree stuck at a single synthetic parent level.
  ### Summary
  After the first current-repo explorer fix, the tree showed one visible parent folder above the repository leaf, but operators still could not climb farther up the hierarchy from the tree itself. Clicking that folder did not reveal the next ancestor, so the explorer still felt truncated.
  ### Analysis
  - The browser-side current-repo tree model was still derived from a fixed two-segment window (`parent -> repo`), so higher ancestors were absent from the rendered node graph.
  - The requested behavior maps naturally to a progressive reveal model: keep the tree compact by default, but when the top visible folder is clicked, expand the visible window upward by one ancestor and rerender.
  - Folder clicks below the top visible node should remain normal tree interactions; only the top visible folder in current-repo mode needs to trigger upward reveal while hidden ancestors remain.
  ### Deliverables
  - [x] Updated browser coverage so current-repo mode starts with the immediate parent visible and reveals the next ancestor when that top folder is clicked.
  - [x] Replaced the fixed current-repo segment builder with a depth-based ancestor window that can grow upward on demand.
  - Update on 2026-03-08 for [F009].
  Changed current-repo mode so clicking the repository leaf pivots the sidebar into a real explorer rooted at the repository parent and restyled the tree to read like a filesystem pane instead of site chrome.
  ### Summary
  Even after the ancestor reveal fix, the compact current-repo tree still behaved like a synthetic path widget rather than a file explorer. Operators expected the repository leaf click to populate the full set of Git-enabled sibling folders under the parent directory, and the themed badge-like tree styling made the widget feel unlike a normal explorer.
  ### Analysis
  - The backend catalog for `current_repo` mode only exposed the current repository before the frontend pivoted into full explorer mode, so the browser had no sibling repositories to show when the leaf was clicked.
  - The right boundary for the data fix is the initial repository catalog: when launched from inside a repo, the server should still discover all repositories beneath the current repository's parent folder while keeping the current repo selected.
  - The frontend then needs an explicit compact-vs-explorer mode: compact mode can show the path-oriented current repo context, and clicking the repo leaf should switch into full explorer mode rooted at the discovered parent.
  - The tree styling should be intentionally decoupled from the rest of the site shell so the pane reads as a neutral filesystem explorer with flat rows, standard selection colors, and folder/file-style icons.
  ### Deliverables
  - [x] Added browser coverage that clicks the current-repo leaf and verifies sibling repositories appear in the tree.
  - [x] Updated the current-repo repository catalog to preload repositories beneath the current repository parent and expose that parent as the explorer root.
  - [x] Added compact/explorer switching in the browser tree model and restyled the tree pane with a flatter file-explorer visual language.
  - Update on 2026-03-08 for [F008].
  Fixed audit-table action buttons so their labels remain visible in the web UI instead of rendering as oversized blank pills.
  ### Summary
  The audit results table reused the generic `.secondary-button` styling inside the Actions column. Those buttons inherited a transparent border and full-width layout, which made the action pills look empty and pushed their labels into a visually awkward position inside the horizontally scrollable table.
  ### Analysis
  - The labels were present in the DOM; this was a presentation regression in the shared button CSS, not missing audit action data.
  - The transparent border made the controls visually weak, and the inherited `width: 100%` caused each action button to stretch across the full cell width instead of sizing to its content.
  - The correct fix is to make secondary buttons explicitly legible and override the audit-table action buttons to render as compact content-width pills with left-aligned text.
  ### Deliverables
  - [x] Added a browser regression that verifies the rename action button exposes non-transparent styling and does not consume the full width of its table cell.
  - [x] Made `.secondary-button` text and border styling explicit.
  - [x] Updated audit action pills to size to content and wrap their labels within the button.
  - Update on 2026-03-08 for [F009].
  Changed compact current-repo folder clicks so selecting the current repo parent pivots into the full sibling-repository explorer instead of only revealing one higher ancestor.
  ### Summary
  In current-repo launch mode, clicking the visible parent folder above the current repository still followed the old ancestor-reveal flow. Operators expected that click to populate the tree with all Git-enabled repositories under that parent folder, matching the behavior already available on the repository leaf itself.
  ### Analysis
  - The backend catalog already contained the discovered repositories beneath `ExplorerRoot`, so the missing behavior was entirely in the browser click handler.
  - The folder node path in compact mode is relative, so the browser needs to resolve that visible folder path back to the absolute compact-tree folder path before deciding whether it represents the current explorer root.
  - Once the clicked compact folder resolves to `ExplorerRoot`, the UI should enter the same expanded explorer mode used by the repo-leaf click and reuse the preloaded sibling repository catalog.
  ### Deliverables
  - [x] Added a browser regression that clicks the visible current-repo parent folder and verifies sibling repositories appear in the tree.
  - [x] Updated compact folder clicks so the current repo parent folder expands into the full sibling-repository explorer.
  - Update on 2026-03-08 for [F009].
  Fixed repository-tree sibling ordering and indent so mixed sibling sets render like a file explorer instead of grouping expandable folders first and offsetting leaf repos to the right.
  ### Summary
  Under one parent folder, the tree still sorted intermediate folders ahead of repository leaves and rendered repository titles farther to the right because repo rows carried a checkbox slot while folder rows did not. That made the tree look unlike a normal file explorer and broke the expected alphabetical sibling order.
  ### Analysis
  - The ordering bug was in the browser-side tree sort, which still preferred `folder` nodes over `repository` nodes before comparing titles.
  - The indent bug was structural in the rendered row chrome: repository rows include a checkbox slot for checked-scope selection, while folder rows only include the expander slot. Without a matching spacer, sibling titles cannot line up.
  - The correct fix is to sort sibling nodes purely by title and reserve the checkbox slot width on folder rows with a hidden spacer so the expander remains the only visible difference between expandable and non-expandable siblings.
  ### Deliverables
  - [x] Added a browser regression that expands a sibling set containing both a repository leaf and an intermediate folder, then asserts alphabetical order and matching title indent.
  - [x] Removed folder-first sorting from the repository tree model.
  - [x] Added a hidden checkbox-width spacer on folder rows so sibling titles align.
  - Update on 2026-03-08 for [F009].
  Restored upward ancestor reveal after current-repo expansion so the top of the explorer remains a parent folder and can keep climbing toward the filesystem root one level at a time.
  ### Summary
  The sibling-explorer change made current-repo expansion stop at the explorer root, so after promoting the current repo parent into the tree, operators no longer saw the next higher parent folder above it. That regressed the earlier expectation that the current-repo tree can continue revealing one more parent at the top on each click.
  ### Analysis
  - The repository catalog already had enough data for sibling expansion; the missing behavior was that expanded mode rendered repositories directly under `ExplorerRoot` without wrapping them in the visible ancestor chain above that root.
  - Expanded current-repo mode therefore needs its own ancestor-depth state separate from the compact `parent -> repo` reveal depth.
  - The correct browser behavior is:
    1. compact mode starts at `parent-folder -> repo`
    2. clicking the repo leaf or current parent folder switches to sibling explorer mode
    3. sibling explorer mode renders the current parent folder beneath one visible higher ancestor when available
    4. clicking the top visible ancestor reveals the next one above it
  ### Deliverables
  - [x] Extended the browser regression so expanding the current repo parent must reveal both sibling repositories and the next higher parent folder, then allow one more upward reveal on the new top folder.
  - [x] Added expanded-mode ancestor wrapping and top-folder reveal logic for current-repo explorer mode.
  - Update on 2026-03-08 for [F009].
  Simplified current-repo tree construction so compact mode and expanded sibling-explorer mode both use one ancestor-depth model anchored to a single path instead of separate compact and expanded depth implementations.
  ### Summary
  The earlier implementation worked, but it carried two different reveal counters and separate helpers for compact and expanded current-repo trees. That made a straightforward rule harder to read in code than it needed to be.
  ### Analysis
  - Both modes follow the same shape: show a visible ancestor chain above an anchor path, then append repository-relative path segments beneath that anchor.
  - In compact mode the anchor path is the selected current repository path.
  - In expanded mode the anchor path is `ExplorerRoot`.
  - A single `currentRepoAncestorDepth` plus `currentRepositoryAnchorPath()` and `currentRepositoryVisibleAncestorSegments()` is enough to express both flows.
  ### Deliverables
  - [x] Collapsed the separate compact and expanded current-repo depth logic into one shared ancestor-depth model in the browser tree renderer.
  - [x] Preserved the existing browser regressions for repo-leaf expansion, parent-folder expansion, alphabetical sibling order, and upward ancestor reveal.
  - Update on 2026-03-08 for [F009].
  Added folder-to-audit-root selection in the repository tree so clicking a folder writes that folder path into the audit draft, and Cmd-click appends more folder roots.
  ### Summary
  Operators wanted the tree to serve as the source for audit scope selection. Clicking a folder should make that folder the audit root, and Cmd-clicking additional folders should accumulate multiple roots without leaving the tree.
  ### Analysis
  - The existing audit draft plumbing already rebuilds the `gix audit` arguments from `#audit-roots-input`, so the cleanest implementation is to reuse that input rather than introduce separate tree-selection state.
  - Folder tree nodes therefore need their absolute filesystem path available in the browser, not just the relative visible path segments used for rendering.
  - Once folder nodes expose their absolute path, a container-level click handler can update the audit roots field directly while preserving the tree’s existing expand/reveal behavior.
  - The audit roots parser should accept both commas and newlines so tree-driven multi-selection and manual edits share one format.
  ### Deliverables
  - [x] Added browser coverage for plain folder click replacing the audit root and Cmd-click appending another folder root.
  - [x] Added absolute folder paths to browser tree nodes.
  - [x] Reused the audit roots input as the single source of truth, with comma-or-newline parsing and comma-separated formatting.
  - Update on 2026-03-09 for [F008].
  Fixed web audit protocol changes for GitHub SCP-style remotes so `git@github.com:owner/repo.git` is treated as SSH instead of a separate `git` protocol, and SSH conversions now write the standard `git@github.com:` form instead of `ssh://git@github.com/...`.
  ### Summary
  The web audit flow exposed a misleading protocol fix on repositories whose `origin` already used the common SSH shorthand form `git@github.com:owner/repo.git`. Audit reported that transport as `git`, the web queue offered a follow-up change to `ssh`, and applying it rewrote the remote to `ssh://git@github.com/owner/repo.git`. From the operator’s perspective that looked like the protocol change flow was not behaving properly.
  ### Analysis
  - The bug was semantic, not transport-level: the code treated GitHub SCP-style SSH URLs and `ssh://git@github.com/...` URLs as two distinct protocols.
  - That split leaked through three layers:
    - audit classified `git@github.com:` as `git`
    - workflow protocol conversion detected `git@github.com:` as `git`
    - remote builders emitted `ssh://git@github.com/...` for the `ssh` target
  - The correct behavior for GitHub remotes is to treat the SCP-style `git@github.com:` form as the SSH transport and present only `ssh` and `https` in the web UX.
  - The existing `git` token is preserved only as a backward-compatible input alias so older CLI/config values still parse, but it is normalized to SSH behavior.
  ### Deliverables
  - [x] Added failing expectations that `git@github.com:` audits as `ssh` and SSH conversions materialize as `git@github.com:owner/repo.git`.
  - [x] Normalized the shared/web/workflow protocol handling so the legacy `git` label aliases to SSH behavior instead of remaining a separate user-visible transport.
  - [x] Reduced the web protocol picker to the two meaningful user-facing choices: `ssh` and `https`.
  - Update on 2026-03-09 for [F003].
  Added explicit `--bind` and `--port` flags for `gix --web` so operators can expose the web UI on non-loopback interfaces and non-default ports without losing the legacy `gix --web [port]` form.
  ### Summary
  Operators need to launch the web UI on addresses such as `0.0.0.0:8081`, not only the default `127.0.0.1:8080`.
  ### Analysis
  - The existing `--web` flag doubled as both the launch switch and the optional port value, which left no room to configure the bind host.
  - Backward compatibility still matters because the current CLI, tests, and existing invocation examples already rely on `gix --web [port]`.
  - The cleanest contract is to keep `--web` as the explicit launch switch, add `--bind` and `--port` as web-only modifiers, and reject those network flags when `--web` is not present.
  ### Deliverables
  - [x] Added root `--bind` and `--port` flags for web launch while preserving `gix --web [port]`.
  - [x] Added CLI and compiled-binary coverage for custom bind/port launch behavior.
  - [x] Documented the new invocation forms in the README.
  - Update on 2026-03-09 for [F003].
  Removed the legacy overloaded `gix --web <port>` form so web launch uses one explicit switch plus explicit network flags.
  ### Summary
  After adding `--bind` and `--port`, the positional port form became ambiguous and easy to misread. The web launch contract is now `gix --web [--bind <host>] [--port <port>]`.
  ### Analysis
  - `--web` should behave like a mode switch, not a flag that sometimes consumes a positional value and sometimes does not.
  - Keeping `gix --web 18080` alive would leave two competing port syntaxes in the CLI and make mixed forms harder to reason about.
  - The CLI should therefore reject positional arguments in web mode and direct the operator to `--port`.
  ### Deliverables
  - [x] Removed the implicit positional-port normalization for `--web`.
  - [x] Added a targeted error when web mode is launched with positional arguments.
  - [x] Updated CLI, integration, and README coverage to use `--port`.
  - Update on 2026-03-09 for [F003].
  Wired `--roots` into `gix --web` so the initial repository catalog and left-pane tree can start from explicit launch roots instead of always discovering from the current working directory.
  ### Summary
  The `--roots` flag was globally parsed, but web launch ignored it and always seeded the browser from `cwd` or current-repository context. Operators could narrow audit roots after the page loaded, but they could not pre-scope the initial repository explorer.
  ### Analysis
  - Web launch already had the correct CLI surface because `--roots` is a persistent root flag on the root command.
  - The missing piece was startup wiring: `handleWebLaunch` built its repository catalog from `cwd` only and never consulted the resolved root flag values.
  - The cleanest fix is to reuse the shared root-flag sanitizer and feed those explicit roots into the initial repository discovery path, while preserving the existing current-repo and discovery behavior when `--roots` is absent.
  ### Deliverables
  - [x] Scoped `gix --web --roots ...` to explicit launch roots.
  - [x] Added CLI, browser, and compiled-binary coverage for the root-scoped launch behavior.
  - [x] Documented the launch-root modifier in the README.
  - Update on 2026-03-09 for [F009].
  Made queued web-audit actions toggle back to dequeue controls in the audit table so operators do not appear able to enqueue the same fix repeatedly from the row actions.
  ### Summary
  The queue already deduplicated identical path+action pairs internally, but the row button kept reading `Queue …` after the change was added. That made the UI look misleading because the visible control implied the same action could be added again.
  ### Analysis
  - The bug was presentation/state-sync, not queue storage: the queue logic replaced existing entries instead of duplicating them.
  - The audit table needed to render against current queue state and refresh whenever queue membership changed from row clicks, queue-panel removal, clear, or successful apply.
  - The clearest fix is to flip the row control label to `Dequeue …` when that exact action is queued and wire that click path directly to queue removal.
  ### Deliverables
  - [x] Toggled queued audit-row actions from `Queue …` to `Dequeue …`.
  - [x] Refreshed the audit table whenever queue membership changes so row actions stay in sync with the queue panel.
  - [x] Added browser coverage for queue-then-dequeue behavior from the same row action button.
  - Update on 2026-03-09 for [F003].
  Restored explicit launch-root folders in the web repository tree so `gix --web --roots ../` shows the configured parent folder as a real tree node instead of flattening everything directly under it.
  ### Summary
  The configured-roots launch flow passed the correct repository scope to the browser, but the tree renderer stripped `launch_path` from repository paths and never rendered the explicit root folder itself when the configured root and the common launch path were the same directory.
  ### Analysis
  - The startup catalog already knew which roots were used to launch the web UI, but it only exposed the common `launch_path`.
  - That was enough to discover repositories, but not enough for the browser to distinguish “common ancestor” from “explicit root that should appear as a node.”
  - Exposing explicit `launch_roots` in the repository catalog lets the browser render configured-root wrapper folders precisely, keep them auto-expanded, and preserve multi-root behavior without inventing extra synthetic ancestors.
  ### Deliverables
  - [x] Added explicit `launch_roots` to the web repository catalog for configured-root launches.
  - [x] Rendered configured launch roots as visible folder nodes in the browser tree.
  - [x] Added browser and compiled-binary coverage for the single-root `--roots ../` style launch case.
  - Update on 2026-03-09 for [F003].
  Adjusted the single-root configured launch tree so the explicit root folder is the active node on load and its immediate parent is rendered as the visible ancestor leaf above it.
  ### Summary
  The previous explicit-root fix made the configured folder appear, but the browser still re-activated the selected repository row on every render. That left `gix --web --roots ../` visually focused on the repository instead of the configured parent folder, and the tree still lacked the one ancestor node above that configured root.
  ### Analysis
  - Repository selection drives command defaults and repo details, but tree activation is a separate concern and needs its own state.
  - For a single configured launch root, the clearest tree shape is one visible ancestor wrapper above the explicit root, with the explicit root itself staying auto-expanded and active.
  - Preserving repository selection while independently restoring the active folder node keeps the UI accurate without changing action scope semantics.
  ### Deliverables
  - [x] Split active tree-node focus from selected repository state in the browser UI.
  - [x] Added a visible ancestor wrapper above single explicit launch roots.
  - [x] Added browser coverage for initial active-node focus and the ancestor leaf.
  - Update on 2026-03-09 for [F003].
  Extended single explicit-root browsing so folder selection can traverse upward or downward through the tree, and added table-driven browser coverage for the traversal paths.
  ### Summary
  The single explicit-root tree previously stopped at one fixed ancestor wrapper. That rendered correctly, but it did not behave like a real browser path because selecting the visible ancestor could not reveal higher parents one level at a time.
  ### Analysis
  - Current-repo mode already treats the top visible ancestor as the upward-traversal control, so explicit-root mode needed the same selection-driven progression.
  - The tree model for a single configured root now derives its visible ancestor chain from depth state instead of a fixed one-parent wrapper.
  - Table-driven browser scenarios now cover both upward traversal through ancestors and downward traversal into nested folders, which locks in the expected contract across current-repo and explicit-root launches.
  ### Deliverables
  - [x] Added upward traversal for single explicit-root launches by revealing one additional ancestor per top-folder selection.
  - [x] Kept downward traversal working through nested folder selection under the configured root.
  - [x] Added table-driven browser tests for folder-tree traversal scenarios.
  - Update on 2026-03-09 for [F003].
  Removed configured-roots fallback behavior so explicit `--roots` values are the only folder context for the web tree and audit flow, with relative roots resolved from cwd but cwd repository context otherwise ignored.
  ### Summary
  `gix --web --roots ../` still carried hidden current-repository influence: configured-root launches could inherit the current repo as the selected repository context, the browser could derive launch roots from `launch_path`, and audit tests still modeled manual root entry instead of tree-driven folder scope.
  ### Analysis
  - Configured-root launches need a strict contract: resolve `--roots` once, then use those resolved folders as the browser tree source and the default audit scope until the user selects another folder in the tree.
  - Current repository context is valid in discovery/current-repo launch modes, but it becomes an incorrect fallback when explicit roots were passed.
  - The browser needs separate state for selected repository actions and selected folder scope so repo clicks do not overwrite the folder that defines audit scope.
  ### Deliverables
  - [x] Removed current-repo selection/context fallback from configured-root repository catalogs and sorted explicit-root catalogs deterministically.
  - [x] Stopped the browser from inventing configured roots from `launch_path` and kept audit scope driven by tree folder selection plus explicit launch roots only.
  - [x] Reworked browser tests away from manual audit-root edits and added command-level and compiled-binary coverage for relative `--roots ../..` launches.
- [x] [P002] Rename the branch-change command from `cd` to `sync`.
  Requested on 2026-05-22.
  ### Summary
  The public branch-change command should be `gix sync`; `gix cd` must no longer be recognized.
  ### Resolution
  - Changed the Cobra command name and reusable operation configuration key from `cd` to `sync`.
  - Preserved the existing branch-change behavior and `switch` alias.
  - Updated embedded defaults, README, architecture notes, docs site examples, and warning matrix references.
  - Added command hierarchy and black-box CLI coverage proving `cd` is rejected while branch-change tests use `sync`.
- [x] [P003] Implement the new `gix sync` semantics and remove the old `cd` implementation.
  Requested on 2026-06-01.
  ### Summary
  `gix sync` should become the canonical simplified workflow command instead of the renamed branch-change command.
  ### Plan
  - Remove `cd` implementation naming and legacy `branch-cd` behavior.
  - Implement sync targets for remote attach, `master`, PR-backed work branches, and the current branch.
  - Keep `--stash`, `--commit`, and `--require-clean` functioning as explicit dirty-work policies.
  - Use merge-based work-branch updates and conflict stop-before-push behavior; do not rebase.
  - Update README/help/config/tests around the simplified flow.
  ### Resolution
  - Removed the public `cd` command path and kept `sync` as the canonical command.
  - Implemented strict sync targets for remote attach, remote-owned `master`, PR-backed branches, and the current branch without rebase or force-push.
  - Made dirty `gix sync` the default: changed paths are clustered, staged, described through the configured `github.com/tyemirov/utils/llm` client, committed, then synced and pushed.
  - Dirty `master` sync now creates a generated `sync/<repo>/<path>` work branch before committing and continuing through the PR flow.
  - Preserved `--stash`, kept `--commit` accepted, and made `--require-clean` the opt-in dirty-work guard.
  - Updated README/help/config defaults and added black-box coverage for dirty PR branches, dirty `master`, generated branches, and the clean-worktree guard.
  - `make test`, `make lint`, and `make ci` passed locally.
- [x] [P004] Make `gix sync` pull request descriptions explain why the PR exists.
  Requested on 2026-06-03.
  ### Summary
  `gix sync` currently opens pull requests with the body `Created by gix sync.`, which describes the tool path rather than the reason a reviewer should care about the PR.
  ### Plan
  - Locate the sync PR creation path and any workflow-level PR body defaults that can leak this placeholder into user-visible GitHub descriptions.
  - Add failing observable coverage for the sync-generated PR body.
  - Replace the placeholder with purpose-oriented body text tied to the sync target and branch context.
  - Run the required Makefile validation targets.
  ### Resolution
  - Replaced the static `Created by gix sync.` body with PR text that explains the review path from the head branch into the remote-owned base branch.
  - Updated strict-sync PR creation coverage for both explicit work branches and generated dirty-`master` branches.
  - `make test`, `make lint`, and `make ci` passed locally.
  ### Follow-up
  The first resolution still used predefined body text. The PR description must be generated from the branch code difference itself, using the configured LLM path and git diff context.
  ### Follow-up Resolution
  - Added a branch-diff PR body generator that collects commit subjects, `git diff --stat`, and patch context for `<remote>/<base>...<branch>`.
  - Threaded the existing sync LLM configuration into PR creation so `gh pr create --body` receives generated text from the code difference.
  - Updated strict-sync coverage to assert both the diff commands and the generated body passed to GitHub.
  - `make test`, `make lint`, and `make ci` passed locally after the follow-up.
  ### Review Fix
  - Moved PR body generation before `git push -u` on new PR branches so body-generation failures do not leave remote branches without pull requests.
  - Added strict-sync coverage for command ordering and the failure path where empty generated PR text stops before push.
  ### Control Follow-up
  - Unified sync-created PR metadata resolution around `title` and `body` option keys.
  - Exposed explicit metadata controls through `gix sync --title/--body` and `sync.pull_request.title/body`.
  - Kept branch-diff body generation as the default and made explicit body text bypass generation.
  - `make test`, `make lint`, and `make ci` passed locally after the control follow-up.
- [x] [P005] Unify duplicated workflow implementation paths behind shared typed builders and services.
  Requested on 2026-06-03.
  ### Summary
  The implementation has parallel CLI, web, workflow, and sync branches that construct equivalent workflow actions and Git operations with duplicated `map[string]any` mutation and helper logic. The code should converge on centralized typed builders and reusable services so each command surface delegates to one canonical implementation path.
  ### Plan
  - Introduce typed workflow option/spec builders for common operations and task actions.
  - Replace CLI preset wrappers and workflow variable override code that currently mutates nested action maps by hand.
  - Reuse the same builders from web workflow primitive construction where the web API creates equivalent operation options.
  - Extract shared branch/PR/dirty-work helpers once the builder path is stable.
  - Add focused tests that prove the shared builders serialize the existing command contracts without changing observable behavior.
  ### Resolution
  - Added workflow-owned typed option builders for common task actions, file replacement safeguards, release retag mappings, and history purge variable overrides.
  - Exported built-in task action identifiers so CLI and web surfaces stop repeating action type strings in implementation code.
  - Migrated web workflow primitives, CLI preset wrappers, and workflow history variable overrides to the shared builder path.
  - Reused the centralized option key constants inside workflow action handlers and taught option map-slice reading to accept typed mapping slices.
  - Added focused builder and override tests in `internal/workflow`.
  - `make format`, `make test`, `make lint`, and `make ci` passed locally.
  ### Follow-up
  Branch/PR/dirty-work helper extraction remains a later unification slice now that the typed option builder path is stable.
- [x] [P006] Avoid stale generated branch collisions during dirty `gix sync master`.
  Requested on 2026-06-03 after dirty `master` sync in `/Users/tyemirov/Development/llm-proxy` failed with `branch "sync/llm-proxy/readme" does not have an open pull request into master`.
  ### Summary
  Dirty `master` sync generated deterministic work branch names from the repository and first dirty path cluster, such as `sync/llm-proxy/readme`. The branch name should identify the semantic change represented by the diff instead of incidental repository, file, time, or sequence metadata.
  ### Plan
  - Add focused strict-sync coverage proving dirty `master` branch names are derived from the generated semantic diff summary.
  - Limit the semantic branch component to 56 characters, including any collision suffix.
  - Keep reusing the semantic generated branch when it still has an open PR.
  - Select the next generated branch suffix only when a semantic generated branch already exists remotely without an open PR.
  - Preserve existing local-only generated branch recovery.
  ### Resolution
  - Added strict-sync regression coverage for dirty `master` branch naming from the generated semantic diff summary.
  - Replaced `sync/<repo>/<path>` with `gix/<semantic-change>`, stripping Conventional Commit type prefixes before slugging the subject.
  - Capped the semantic branch component at 56 characters and trims at word boundaries when possible.
  - Kept collision handling as a last resort: an already-occupied semantic branch advances to the next numeric suffix before the normal commit, push, and pull-request flow continues.
  - `make test`, `make lint`, and `make ci` passed locally.
