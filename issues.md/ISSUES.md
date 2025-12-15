# ISSUES
**Append-only section-based log**

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @POLICY.md, PLANNING.md, @NOTES.md, and @ISSUES.md under issues.md/. Read @ARCHITECTURE.md, @README.md, @PRD.md. Start working on open issues. Work autonomously and stack up PRs. Prioritize bugfixes.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes `- [x] [GX-<number>]`

## Features (110–199)

- [x] [GX-110] Add a website documenting all of the benefits the gix utility has. The web site shall be served from github so follow the convention for folders/file placement (Static docs site now lives under `docs/index.html` with a marketing overview, workflows, and recipes, wired for GitHub Pages.)

## Improvements (251–299)

- [x] [GX-251] `gix cd` doesnt work with --stash flag the way I would like it to: I want it to stash the modified tracked files, switch to the destination branch and restore the files. (Implemented tracked-file stashing around branch change plus restoration, with new regression coverage.)
- [x] [GX-252] `gix cd` output is noisy (“tasks apply …”) and lacks summaries when run against multiple roots. Redesign the reporter so workflow-backed commands keep per-repo sections, drop the “tasks apply” prefixes, and print a final summary only when more than one repository is processed.
- [x] [GX-253] Hide explicit `--refresh` flag on `gix cd`, keeping refresh behaviour wired internally for `--stash` and `--commit` flows only (removed the flag from CLI/config/docs, relying on stash/commit to opt into the stricter refresh stage).
- [x] [GX-254] Add an `a` option to confirmation prompts that, when selected in a non-`--yes` run, treats all subsequent confirmation questions in the session as accepted (equivalent to having passed `--yes`), so operators can promote a single “accept all” decision without restarting the command. (Implemented shared session prompt state + `[a/N/y]` templates; `make test`, `make lint`, and `make ci` all pass.)
- [x] [GX-255] In cases when There is no tracking information for the current branch, create and associate the branch so I wouldnt need to do it manually: `gix cd` now auto-configures tracking to the resolved remote when possible, so refresh/pull proceed without manual `git branch --set-upstream-to` steps (logs tested locally; automated suites blocked by sandbox bind/proxy limits).
```
15:24:28 tyemirov@Vadyms-MacBook-Pro:~/Development/tyemirov/Research/ISSUES.md - [bugfix/IM-310-log-handler] $ gix cd bugfix/IM-312-secure-join
-- tyemirov/ISSUES.md (/Users/tyemirov/Development/tyemirov/Research/ISSUES.md) --
  issues:
    - ⚠ refresh skipped (no tracking remote)
    - ⚠ PULL-SKIP: There is no tracking information for the current branch.
  • branch:
    - bugfix/IM-312-secure-join
15:25:52 tyemirov@Vadyms-MacBook-Pro:~/Development/tyemirov/Research/ISSUES.md - [bugfix/IM-312-secure-join] $ git pull
There is no tracking information for the current branch.
Please specify which branch you want to merge with.
See git-pull(1) for details.

    git pull <remote> <branch>

If you wish to set tracking information for this branch you can do so with:

    git branch --set-upstream-to=origin/<branch> bugfix/IM-312-secure-join

15:26:03 tyemirov@Vadyms-MacBook-Pro:~/Development/tyemirov/Research/ISSUES.md - [bugfix/IM-312-secure-join] $ git branch --set-upstream-to=origin/bugfix/IM-312-secure-join bugfix/IM-312-secure-join
branch 'bugfix/IM-312-secure-join' set up to track 'origin/bugfix/IM-312-secure-join' by rebasing.
```
- [x] [GX-256] When `gix cd` reports “untracked files present; refresh will continue”, include the untracked file names/status entries in the warning output so operators can see exactly which files are untracked without running a separate git status. (Warnings now list the precise untracked paths.)

- [x] [GX-257] Ensure that we commit only the files that we have changed. When running @configs/account-rename.yaml it looks like we are committing all uncommitted files in a tree. (`git stage-commit` stages only workflow-mutated files, so existing local work stays untouched, and namespace rewrite steps now register their changed files so downstream commits see them.)

- [x] [GX-258] When running namespace rewrite workflows (for example, @configs/account-rename.yaml), avoid leaving behind empty automation branches in repositories where the workflow produced no file edits. (Introduced a generic `git branch-cleanup` workflow command that inspects the mutated-files registry to detect repositories with no workflow-edited files, deletes the corresponding automation branch locally when present, logs the outcome as an explicit no-op or deletion under the `git` phase, and wired it into the account-rename preset after branch restore so branches like `automation/ns-rewrite/<repo>-<workflow_run_id>` are removed automatically when nothing changed.)

- [x] [GX-259] The logging should be per step. Each step has a name and we want to see the name of this step instead rather meaningless current rubrics
instead of
```shell
-- tyemirov/zync (/home/tyemirov/Development/tyemirov/zync) --
  • remote/folder:
    - already canonical
    - already normalized
  • branch:
    - master
    - automation/ns-rewrite/zync-20251212T171313 (created)
  • files:
    - Rewrite module namespace
  • git:
    - ⚠ Git Stage Commit (no-op: no workflow-edited files to commit for this repository (require_changes safeguard; clean worktree))
    - ⚠ Git Push (no-op: no commit produced by this workflow (require_changes safeguard))
    - ⚠ Open Pull Request (no-op: no branch changes to review for this workflow (require_changes safeguard))
    - Restore initial branch
    - Restore initial branch
```
we need
- step name: remotes
  reason: already canonical
- step: folders
  outcome: no-op  
  reason: already normalized
- step name: protocol-to-git-https
  outcome: no-op
  reason: 

etc etc

Prepare a deltails planed of the sane loggin
Workflow logging now propagates each configuration step name into the environment and attaches it to all emitted events, so the human formatter can label lines by step instead of generic rubrics: task apply events render as `step: task`, phase events include the step when present, and safeguard-driven git skips use the step name as their label (for example, `namespace-stage-commit (no-op: …)` instead of `Git Stage Commit`). Issues retain a dedicated section but now prefer the step label when present, so each reported problem is clearly associated with its originating step.

- [x] [GX-260] Branch cleanup behavior for namespace workflows now also uses the repository default branch as the base (via `.Repository.DefaultBranch` in `configs/account-rename.yaml`), so automation branches created by account-rename are deleted when they have no commits beyond the default branch; this path is covered by regression tests.

- [x] [GX-261] Migrate (move) the llm package unter tyemirov/utils. Use tools/utils folder to add the changes. Ensure that tools/utils/llm will be automous self-encapsulated package easy for integration. Have full test coverage. Deliverable: Use tyemirov/utils/llm instead of pkg/llm (gix now imports `github.com/tyemirov/utils/llm` and `pkg/llm` is removed; `github.com/tyemirov/utils/llm` has full statement coverage in its own repo.)

## BugFixes (340–399)

- [x] [GX-340] Audit this: I think I saw a few times when `gix cd` command was telling me that the branch was untract when in fact git co <branch> worked perfectly fine. I maybe off, so it's a maybe bug. (Fixed by letting `gix cd` fall back to the lone configured remote so branch switches track upstreams just like `git checkout`; added regression coverage.)

- [x] [GX-341] I ran the workflow [text](configs/account-rename.yaml) but I am then getting errors
```
  Run make lint
Error: main.go:7:2: no required module provides package github.com/temirov/ghttp/internal/app; to add it:
	go get github.com/temirov/ghttp/internal/app
go vet ./...
Error: main.go:7:2: no required module provides package github.com/temirov/ghttp/internal/app; to add it:
	go get github.com/temirov/ghttp/internal/app
make: *** [Makefile:22: lint] Error 1
Error: Process completed with exit code 2.
```
It means the replacement were not executed

(Resolved by teaching workflow file replacements to honor recursive `**` glob patterns so account-rename rewrites Go modules across nested folders; `make test` now passes.)

- [x] [GX-342] An error stopped running the workflow, even though the intermediate errors shall not be catastrophic and shall not halt the execution. Moreover, the rror is cryptic and dioesnt tell what has ahppened and how to fix it. Not a git repository is never a problem and we shall just ignore such cases.
```shell
10:06:31 tyemirov@computercat:~/Development $ gix w tyemirov/gix/configs/account-rename.yaml --yes
failed to inspect repositories: git command exited with code 128 (check-ignore --stdin): fatal: not a git repository: /home/tyemirov/Development/moving_map/images/CrunchyData/pg_tileserv/../../.git/modules/images/pg_tileserv
```
(Now we swallow `git check-ignore` “not a git repository” failures so workflows skip those folders instead of aborting.)

- [x] [GX-343] `gix message changelog` prints “no changes detected for changelog generation” twice. (The command now treats the empty-range case as informational, prints the notice once, and exits successfully.)

- [x] [GX-344] There are no step names and per step logging despite GX-259 marked as done. (Workflow now injects step names into all emitted events (including repo executors that report directly), so human logs consistently prefix entries with the active step; tests updated and `make ci` passes.)
```shell
12:14:25 tyemirov@computercat:~/Development $ gix w tyemirov/gix/configs/account-rename.yaml --yes
-- MarcoPoloResearchLab/PhotoFriend (/home/tyemirov/Development/tyemirov/Research/MarcoPoloResearchLab/PhotoFriend) --
  • remote/folder:
    - already canonical
  • files:
    - already normalized
  • branch:
    - master
    - automation/ns-rewrite/PhotoFriend-20251212T201557 (created)
  • files:
    - namespace-rewrite: Rewrite module namespace
  • git:
    - ⚠ namespace-stage-commit (no-op: no workflow-edited files to commit for this repository (require_changes safeguard; clean worktree))
    - ⚠ namespace-push (no-op: no commit produced by this workflow (require_changes safeguard))
    - ⚠ namespace-open-pr (no-op: no branch changes to review for this workflow (require_changes safeguard))
    - Restore initial branch
    - namespace-branch-cleanup: Restore initial branch
```

- [ ] [GX-345] First output appears late when running gix against 20–30 repositories because repository discovery/inspection emits no user-facing progress until the first repository finishes its first workflow step. (Unresolved: stream discovery/inspection progress or emit an initial discovery step summary.)

- [x] [GX-346] Split logging formats by command: keep existing human-readable logs for singular/non-workflow commands, but emit YAML step summaries for `gix workflow` runs. (The `workflow` command now forces the YAML step-summary formatter while other commands keep human logs.)

- [x] [GX-347] Restore end-of-run workflow summary output (when more than one repository is processed) for `gix workflow`. (Workflow now prints a final summary line using `pkg/taskrunner` summary rendering.)

- [x] [GX-348] Ensure workflow step summaries can surface destructive outcomes explicitly (e.g., `deleted`/`kept` for `git branch-cleanup`) and never emit blank `reason` fields. (Step summary events now include explicit outcomes and always render a non-empty `reason` scalar.)

## Maintenance (422–499)

## Planning 
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
