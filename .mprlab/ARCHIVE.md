# ISSUES ARCHIVE

Resolved issues archived from `.mprlab/ISSUES.md` during backlog cleanup.

Each issue retains the canonical identifier it had when archived. Older `GX-<number>` entries predate the current section-aware identifier scheme.

## Features (110–199)

- [x] [GX-110] Add a website documenting all of the benefits the gix utility has. The web site shall be served from github so follow the convention for folders/file placement (Static docs site now lives under `docs/index.html` with a marketing overview, workflows, and recipes, wired for GitHub Pages.)
- [x] [GX-111] Add a step that allows running an arbitrary command, such as `go get -u ./...` and `go mod tidy`. (Added `command run` workflow step with tests and docs; originally tracked as GX-110.)

## Improvements (251–299)

- [x] [GX-251] `gix cd` doesnt work with --stash flag the way I would like it to: I want it to stash the modified tracked files, switch to the destination branch and restore the files. (Implemented tracked-file stashing around branch change plus restoration, with new regression coverage.)
- [x] [GX-252] `gix cd` output is noisy (“tasks apply …”) and lacks summaries when run against multiple roots. Redesign the reporter so workflow-backed commands keep per-repo sections, drop the “tasks apply” prefixes, and print a final summary only when more than one repository is processed.
- [x] [GX-253] Hide explicit `--refresh` flag on `gix cd`, keeping refresh behaviour wired internally for `--stash` and `--commit` flows only (removed the flag from CLI/config/docs, relying on stash/commit to opt into the stricter refresh stage).
- [x] [GX-254] Add an `a` option to confirmation prompts that, when selected in a non-`--yes` run, treats all subsequent confirmation questions in the session as accepted (equivalent to having passed `--yes`), so operators can promote a single “accept all” decision without restarting the command.
- [x] [GX-255] In cases when There is no tracking information for the current branch, create and associate the branch so I wouldnt need to do it manually.
- [x] [GX-256] When `gix cd` reports “untracked files present; refresh will continue”, include the untracked file names/status entries in the warning output so operators can see exactly which files are untracked without running a separate git status.
- [x] [GX-257] Ensure that we commit only the files that we have changed. When running @configs/account-rename.yaml it looks like we are committing all uncommitted files in a tree.
- [x] [GX-258] When running namespace rewrite workflows (for example, @configs/account-rename.yaml), avoid leaving behind empty automation branches in repositories where the workflow produced no file edits.
- [x] [GX-261] Migrate (move) the llm package unter tyemirov/utils. Deliverable: Use tyemirov/utils/llm instead of pkg/llm.
- [x] [GX-262] Improve the workflow summary. (Updated summary formatting to report duration only, drop specified counters, and add step outcome counts; originally tracked as GX-251.)
- [x] [GX-263] Add steps to @configs/account-rename.yaml that allows to bump up the dependency versions of go.mod (see GX-110). (Added go get/go mod tidy workflow steps with go.mod safeguards; originally tracked as GX-252.)
- [x] [GX-264] Add steps to @configs/account-rename.yaml to upgrade go version in go.mod to `go 1.25.4`. (Added go mod edit step before go mod tidy; originally tracked as GX-253.)

## BugFixes (340–399)

- [x] [GX-340] Audit this: I think I saw a few times when `gix cd` command was telling me that the branch was untract when in fact git co <branch> worked perfectly fine.
- [x] [GX-341] Workflow replacement did not execute across nested folders; `go vet`/`make lint` failed after account-rename.
- [x] [GX-342] `git check-ignore` “not a git repository” failures should not halt workflows; errors should be contextual and non-catastrophic.
- [x] [GX-343] `gix message changelog` prints “no changes detected for changelog generation” twice.
- [x] [GX-344] Missing step names and per-step logging in workflow output.
- [x] [GX-346] Split logging formats by command: keep human logs for singular/non-workflow commands, emit YAML step summaries for `gix workflow` runs.
- [x] [GX-347] Restore end-of-run workflow summary output for `gix workflow`.
- [x] [GX-348] Ensure workflow step summaries surface destructive outcomes and never emit blank `reason` fields.
- [x] [GX-349] Stop emitting workflow step summary YAML for non-workflow commands by only installing the YAML event formatter for `gix workflow`.
- [x] [GX-350] Restore succinct non-workflow console logging by suppressing workflow-internal `TASK_PLAN`/`TASK_APPLY`/`WORKFLOW_STEP_SUMMARY` events and omitting machine payload output.
- [x] [GX-351] Fix `safeguards.*.require_changes` to remain true after `git stage-commit` so `git push` / `pull-request open` are not skipped when commits were created. (Implemented workflow change tracking and updated safeguards + tests.)
- [x] [GX-352] Fix `gix cd --stash` popping extra stashes when untracked files are present. (Pop only when a stash was actually pushed; added regression coverage.)
- [x] [GX-353] Trim blank stderr lines when formatting `execshell.CommandFailedError` to avoid trailing `|` delimiters in user-facing error messages.

## Maintenance (422–499)

- [x] [GX-423] Cleanup docs and backlog. (Reviewed README/ARCHITECTURE for accuracy, updated workflow summary notes, and archived completed issues.)

## 2026-07-24 Backlog and Documentation Audit

The entries below were moved from the active tracker after verification. Their durable operator and architecture contracts are maintained in [README.md](../README.md), [ARCHITECTURE.md](../ARCHITECTURE.md), [the web audit workspace guide](../docs/web-audit-workspace.md), and [CHANGELOG.md](../CHANGELOG.md); implementation-only regressions remain covered by the repository test suite.

### BugFixes

- [x] [B001] Make LLM Proxy transport explicit; the default connection no longer requires an LLM Proxy credential.
- [x] [B002] Support chained pull requests by resolving and syncing each pull request’s actual base branch.
- [x] [B003] Hand off a merged pull-request branch to its base branch instead of reporting a missing open pull request twice.
- [x] [B004] Restore tracked ignored generated paths so a completed sync leaves the worktree clean.
- [x] [B005] Exclude tracked files under ignored parents from dirty-sync staging.
- [x] [B006] Exclude ignored generated paths from dirty auto-commit staging.
- [x] [B007] Restore linked-worktree adoption to strict sync.
- [x] [B008] Replace the blanket `.gitignore` with explicit project ignore rules.
- [x] [B009] Adopt a sibling worktree when the requested branch is already checked out there.
- [x] [B010] Permit safe fast-forward refreshes while preserving tracked local edits.
- [x] [B011] Emit per-repository discovery progress before workflow work begins.
- [x] [B012] Make `gix prs delete --yes` report its per-repository outcome.
- [x] [B013] Apply workflow file replacement globs consistently, including `**/` patterns.
- [x] [B014] Treat already-absent local PR branches as no-ops rather than failures.
- [x] [B015] Add canonical user and system configuration initialization.
- [x] [B016] Push local-ahead work branches during sync.
- [x] [B017] Keep dirty explicit `gix sync master` on the master commit-and-push path.
- [x] [B018] Open review for an existing remote branch with real unreviewed changes.
- [x] [B019] Preserve merged-branch handoff and linked-worktree safety during sync.
- [x] [B021] Create a missing explicit dirty-sync branch at the current checkout before committing clusters.
- [x] [B022] Reject incomplete AI merge resolutions before any commit or push.
- [x] [B023] Make release preparation self-contained and fail closed on missing or invalid artifacts.
- [x] [B024] Preserve the syncflow builder’s canonical `gix sync --help` contract.
- [x] [B025] Create missing explicit branches as a stacked pull-request chain from the current branch.
- [x] [B026] Preserve distinct Pages `source_commit` and release `release_commit` identities and verify each at its owning boundary.
- [x] [B027] Publish dirty remote-backed branches that lack an open pull request.
- [x] [B028] Commit dirty explicit sync work to the named target branch.
- [x] [B029] Stage NUL-delimited Git status paths as literal filenames.
- [x] [B030] Prune stale linked-worktree metadata before adoption.
- [x] [B031] Make AI merge resolution visible, deadline-bounded, and safe to hand off.

### Improvements

- [x] [I001] Report workflow summaries with per-step outcomes and readable duration.
- [x] [I002] Add dependency-upgrade workflow steps to `configs/account-rename.yaml`.
- [x] [I003] Add the Go-version update step to `configs/account-rename.yaml`.
- [x] [I004] Embed and render BSL, MIT, and proprietary license templates through the license preset.
- [x] [I005] Use one strict interpolated `config.yml` runtime contract.
- [x] [I006] Model LLM routing as provider-owned ordered connection profiles.
- [x] [I007] Keep dotenv loading outside gix and use only inherited process environment values.
- [x] [I008] Make terminal audit tables the default and provide strict CSV and HTML exports.

### Maintenance

- [x] [M009] Remove the obsolete formatting failure report: the referenced legacy file is absent from the current repository and the complete CI gate is clean.
- [x] [M010] Consolidate MPR Lab governance under `.mprlab/`.
- [x] [M011] Document user configuration initialization and its control surface.
- [x] [M012] Add the global provider-owned LLM configuration contract.
- [x] [M013] Keep user configuration under `$HOME/.gix/config.yml` only.

### Features

- [x] [F001] Add the `command run` workflow step for arbitrary repository commands.
- [x] [F002] Add the local `gix --web` browser interface and JSON API.
- [x] [F003] Eliminate duplicate CLI error logging.
- [x] [F004] Add typed web audit inspection with operator-selected roots.
- [x] [F005] Add a review-before-apply typed web audit remediation queue.
- [x] [F006] Expose explicit origin-remote status in CLI and web audit contracts.
- [x] [F007] Add the confirmation-gated web-only queued folder-deletion action.
- [x] [F008] Add queued web audit sync and protocol-conversion actions.

### Planning

- [x] [P001] Add the repository-tree explorer to the web interface.
- [x] [P002] Rename the branch-change command from `cd` to `sync`.
- [x] [P003] Implement the current `gix sync` contract and remove the former `cd` implementation.
- [x] [P004] Generate explanatory pull-request descriptions for `gix sync`.
- [x] [P005] Unify workflow implementation paths behind shared typed builders and services.
- [x] [P006] Advance generated branch names when a prior dirty-sync branch name is occupied.
