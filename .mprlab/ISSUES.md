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
- [x] [B020] (P1) Dirty `gix sync <new-branch>` should create the new branch from the current checkout before committing clustered changes.
  Requested on 2026-07-09 after explicit new-branch sync proved unreliable when the current non-base branch had uncommitted files.
  ## Observation
  - Dirty sync creates a missing explicit target from `origin/master` unless the current branch itself is `master`.
  - Switching a dirty non-base checkout to a new branch at `origin/master` can fail because tracked changes would be overwritten.
  - When the switch succeeds, the new branch can still discard the current branch's committed ancestry instead of matching normal `git switch -c <new-branch>` semantics.
  ## Deliverable
  - Create a missing explicit dirty-sync target at the current checkout.
  - Preserve the current branch's committed ancestry and split stageable dirty paths into a linear sequence of top-level-area commits on the new branch.
  - Continue through the existing merge-based push and pull-request flow after the dirty work is committed.
  - Add black-box CLI coverage from a dirty non-base branch where switching back to `origin/master` would overwrite the local work.
  ## Validation
  - Focused failing-then-passing sync integration regression.
  - `make test`
  - `make lint`
  - `make ci`
  ## Resolution
  - Missing explicit sync targets now use one canonical `git switch -c <new-branch>` path rooted at the current branch's `HEAD` for both dirty and clean worktrees.
  - Dirty sync creates that branch before staging, message generation, and top-level cluster commits, then merges the configured remote base before push and pull-request creation.
  - Added a public-CLI regression proving the source branch remains unchanged and ancestral, two dirty clusters form an exact linear parent chain, the remote-base merge occurs after both commits, and the resulting branch is clean, tracked, pushed, and PR-backed.
  - Extended the same regression through a clean child-branch run so the separate clean missing-target path is also proven to start at the current `HEAD` without adding commits.
  ## Validation Result
  - Initial `make test` failed at `switch -c feature/clustered-work origin/master` with Git's tracked-file overwrite error.
  - `env GOFLAGS=-count=1 make test-slow`
  - `make test`
  - `make lint`
  - `make ci`
  - `git diff --check`


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
  -