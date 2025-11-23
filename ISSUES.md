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
- [ ] [GX-254] Add an `A` option to confirmation prompts that, when selected in a non-`--yes` run, treats all subsequent confirmation questions in the session as accepted (equivalent to having passed `--yes`), so operators can promote a single “accept all” decision without restarting the command.

## BugFixes (340–399)

- [x] [GX-340] Audit this: I think I saw a few times when `gix cd` command was telling me that the branch was untract when in fact git co <branch> worked perfectly fine. I maybe off, so it's a maybe bug. (Fixed by letting `gix cd` fall back to the lone configured remote so branch switches track upstreams just like `git checkout`; added regression coverage.)

## Maintenance (422–499)

## Planning 
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
