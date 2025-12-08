# ISSUES
**Append-only section-based log**

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.GIT.md @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes `- [x] [GX-<number>]`

## Features (110–199)

## Improvements (251–299)

- [x] [GX-251] `gix cd` doesnt work with --stash flag the way I would like it to: I want it to stash the modified tracked files, switch to the destination branch and restore the files. (Implemented tracked-file stashing around branch change plus restoration, with new regression coverage.)
- [ ] [GX-252] `gix cd` output is noisy (“tasks apply …”) and lacks summaries when run against multiple roots. Redesign the reporter so workflow-backed commands keep per-repo sections, drop the “tasks apply” prefixes, and print a final summary only when more than one repository is processed.
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

## BugFixes (340–399)

- [x] [GX-340] Audit this: I think I saw a few times when `gix cd` command was telling me that the branch was untract when in fact git co <branch> worked perfectly fine. I maybe off, so it's a maybe bug. (Fixed by letting `gix cd` fall back to the lone configured remote so branch switches track upstreams just like `git checkout`; added regression coverage.)

## Maintenance (422–499)

## Planning 
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
