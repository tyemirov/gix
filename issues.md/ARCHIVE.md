# ISSUES ARCHIVE

Resolved issues archived from `issues.md/ISSUES.md` during backlog cleanup.

Each issue is formatted as `- [x] [GX-<number>]`.

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
