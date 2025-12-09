# ISSUES
**Append-only section-based log**

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GIT.md, @AGENTS.DOCKER.md, AGENTS.GO.md, @AGENTS.FRONTEND.md, @POLICY.md, PLANNING.md, @NOTES.md, and @ISSUES.md under issues.md/. Read @ARCHITECTURE.md, @README.md, @PRD.md. Start working on open issues. Work autonomously and stack up PRs. Prioritize bugfixes.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes `- [x] [GX-<number>]`

## Features (110–199)

- [ ] Add a website documenting all of the benefits the gix utility has. The web site shall be served from github so follow the convention for folders/file placement

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

## Maintenance (422–499)

## Planning 
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
