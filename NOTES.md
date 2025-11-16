## Workflow Header Formatting Cleanup

- Added replace-mode tasks + sequential workflow workers so namespace rewrites run like gitignore (branch, files.apply, stage/commit, push, PR) and each repo’s workflow runs sequentially.
- Introduced explicit workflow/workflow_workers config/flag; default is sequential, concurrency requires `--workflow-workers`.
- Reworked logging (internal + CLI formatter) so repositories print `-- owner/repo (/abs/path) --` once per block, and path-only events don’t emit headers.
- Added integration coverage (`TestWorkflowProcessesRepositoriesSequentially`, `TestWorkflowLogHeaderFormatting`) plus unit tests for replace-mode and PR skip logic.
- Ran `make ci`; all lint + tests are green.
