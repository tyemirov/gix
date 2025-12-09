# Changelog

## [v0.3.0-rc6]

### Features ✨
- _No changes._

### Improvements ⚙️
- `gix message changelog` now suppresses duplicate “no changes detected” lines for empty commit ranges.
- Namespace workflow rewrites register changed files, enabling `git stage-commit` to automatically include those modifications.
- Workflow file replacements honor recursive glob `**` patterns for correct module import rewrites across nested folders.

### Bug Fixes 🐛
- Avoid duplicate changelog no-change notices in the changelog generation command.
- Track namespace-mutated files to ensure proper staging of changes.
- Fixed recursive replacements in workflows to properly apply changes across nested files.

### Testing 🧪
- Added tests for changelog action to verify handling of no-change scenarios and output behavior.
- Added tests covering session prompter 'apply-all' uppercase selection.

### Docs 📚
- _No changes._

## [v0.3.0-rc4]

### Features ✨
- _No changes._

### Improvements ⚙️
- `gix cd` warnings now include untracked file names, helping users identify blocking paths without running `git status`.
- `git stage-commit` stages only files mutated by workflows, preventing unrelated changes from being committed.
- Workflow file replacements honor recursive glob `**` patterns, enabling correct module import rewrites across nested folders.
- Added an explicit `require_changes` safeguard so git stage-commit/push/pull-request steps can skip themselves when no edits are present; the account-rename preset now opts in to the safeguard instead of relying on implicit behavior.
- `gix message changelog` now suppresses duplicate “no changes detected” lines when the selected range contains no commits.

### Bug Fixes 🐛
- Fixed recursive replacements in workflows to properly apply changes across nested files.
- Corrected `gix cd` command behavior for refreshing branches with untracked files.
- Resolved `git stage-commit` committing all files issue by limiting staging to mutated files only.

### Testing 🧪
- Added tests for session prompter supporting uppercase 'apply-all' option.
- Added tests verifying `git stage-commit` correctly prefers recorded mutated files.
- Added coverage for filtering staged files by requested patterns.

### Docs 📚
- _No changes._

## [v0.3.0-rc3]

### Features ✨
- Add session apply-all prompter to enable upgrading confirmation policy during command runs.

### Improvements ⚙️
- Refactor confirmation prompts to support `[a/N/y]` template with lowercase apply-all indicator.
- Rename prompt state handling to session state for better semantic clarity.
- Update documentation and CLI design notes to reflect the new confirmation prompt behavior.
- Enhance the prompt components to auto-accept subsequent confirmations after apply-all selection.
- Lowercase the apply-all indicator in confirmation prompts for consistency.
- Auto-configure branch tracking when missing, improving `gix cd` usability.
- Improve confirmation prompts to maintain uppercase default decline (`N`) behavior.

### Bug Fixes 🐛
- Fix prompt template in branch deletion confirmation to include apply-all `[a/N/y]` option.
- Correct renamed packages and internal references related to prompt state and session state.
- Honor recursive glob patterns in workflow file replacements so presets like `configs/account-rename.yaml` actually rewrite Go module imports across nested folders.
- Include the untracked file names in `gix cd` warnings so operators can see which paths block refresh without running `git status`.
- `git stage-commit` now stages only the files mutated by the workflow (rather than `git add .`), preventing unrelated local changes from being committed.
- Ignore `git check-ignore` failures that report “not a git repository” so workflows no longer abort when encountering non-repo folders.
- Namespace workflow rewrites now register their changed files so subsequent `git stage-commit` steps pick up those modifications automatically.

### Testing 🧪
- Add tests for session prompter covering applying 'apply-all' uppercase selection.
- Update tests for renamed prompt session prompter and session state usage.

### Docs 📚
- Update `README.md` to clarify new confirmation prompt behavior including apply-all option.
- Refine CLI design documentation for confirmation prompt letter case semantics and apply-all feature.
- Adjust issue tracker documentation to mark apply-all issue as resolved with relevant implementation details.

## [v0.3.0-rc2]

### Features ✨
- Introduce `TrackingRemoteConfigurator` smart constructor and move validation to edge.
- `gix cd` now auto-configures branch tracking to the resolved remote when possible, streamlining refresh and pull workflows.

### Improvements ⚙️
- Enhance branch change action to automatically configure tracking remote if missing.
- Added detailed tracking remote name reporting in branch change results.
- Refactor test suites for branch change and tracking remote configuration logic.

### Bug Fixes 🐛
- Fix handling of branches without tracking information by auto-associating tracking remotes.
- Avoid refresh action skips by creating tracking branches automatically when remote branch exists.
- Improve error messaging and handling for missing remote branches during tracking configuration.

### Testing 🧪
- Add tests verifying automatic tracking remote configuration and related warnings.
- Extend coverage for handling branch change actions with tracking configurations.
- Test error scenarios when remote branch is missing or configuration commands fail.

### Docs 📚
- Updated `ISSUES.md` with details about automatic tracking remote configuration and related scenarios.
- _No additional documentation changes._

## [v0.3.0-rc1]

### Features ✨
- Extend confirmation prompts to include an 'accept all' option that upgrades subsequent prompts to auto-accept, mimicking the `--yes` flag behavior without restarting the command.

### Improvements ⚙️
- Update documentation and CLI to reflect the new confirmation behavior supporting the 'accept all' feature.
- Refactor prompt handling to support automatic approval after selecting the 'accept all' option.
- Enhance summary reporting for multi-repository workflow commands.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Add tests for prompt behavior to verify 'accept all' selection upgrades the prompt session to auto-accept.

### Docs 📚
- Update CLI design document and README to describe the new confirmation prompt 'accept all' option and its effect on workflow behavior.

## [v0.2.11]

### Features ✨
- Added integration tests covering the `gix cd` refresh default behavior.
- Introduced a new site and styles to showcase gix capabilities and workflows.

### Improvements ⚙️
- Hid the explicit `--refresh` flag in `gix cd`, enabling refresh only internally when `--stash` or `--commit` recovery is requested.
- Simplified CLI usage and updated documentation/examples to match the new `gix cd` behavior.
- Enhanced multi-repository workflows with refined summary reporting and deduplication of repository roots.

### Bug Fixes 🐛
- Fixed the default behavior for branch change refresh in `gix cd`.
- Suppressed noisy "tasks apply" prefixes from error messages to improve clarity.

### Testing 🧪
- Added comprehensive integration tests for `gix cd --refresh` flow with stash and commit recovery.

### Docs 📚
- Created a full new docs site with introduction, feature list, use cases, workflows, architecture, and recipes.
- Updated README and configuration defaults to reflect removal of the explicit `--refresh` flag on `gix cd`.
- Fixed formatting in ISSUES.md for improved issue checklist readability.

## [v0.2.10]

### Features ✨
- Redesign `gix cd` output to remove noisy "tasks apply" prefixes and add per-repository sections with a final summary when processing multiple repositories.
- Extend confirmation prompts so selecting `a`/`all` once upgrades the remainder of the run to auto-accept behaviour, matching the effect of passing `--yes` without requiring a new invocation.

### Improvements ⚙️
- Implement summary reporting for workflow-backed commands showing aggregated results when more than one repository is involved.
- Enhanced task runner to print execution summaries including event counts, warnings, errors, and durations.
- Deduplicate repository roots for accurate summary computations.
- Remove explicit `gix cd --refresh` flag from the CLI, keeping the stricter refresh stage internal and enabling it only when `--stash` or `--commit` recovery is requested, and update documentation/examples to reflect the simplified surface.

### Bug Fixes 🐛
- Suppress "tasks apply" operation prefix in error messages for clearer failure outputs.

### Testing 🧪
- Added tests validating summary line formatting and multiple-repository summary output.
- Coverage improvements for operation failure message formatting and task runner summary executor.

### Docs 📚
- Updated issue tracker with the new enhancement request and implementation notes for `gix cd` output redesign.

## [v0.2.9]

### Features ✨
- Support for stashing tracked files and restoring them during branch changes using `gix cd --stash`.
- Handle multiple stashes during branch change refresh to improve workflow reliability.

### Improvements ⚙️
- Enhanced `gix cd` remote selection to fall back to a single configured remote automatically, matching `git checkout` behavior.
- Updated documentation to reflect Go 1.25+ requirement and renamed module paths from `github.com/temirov/gix` to `github.com/tyemirov/gix`.
- Improved audit report columns to show repository dirty status and affected files for better visibility.

### Bug Fixes 🐛
- Fixed `gix cd` incorrectly reporting branches as untracked by aligning upstream tracking logic with `git checkout`.
- Corrected branch switching to always proceed even when worktrees are dirty, guarding only refresh/pull stages.
- Resolved incomplete line replacement in files during workflow runs to apply changes to all targeted Go files.
- Fixed outdated CLI design documentation to refer to the correct binary and module path after rename.

### Testing 🧪
- Added regression tests covering tracked-file stash/restore flow during branch changes.
- Coverage improved for fallback behavior in remote selection and dirty skip scenarios.

### Docs 📚
- Updated README and architecture docs for Go 1.25+ requirement.
- Revised CLI design document to reflect new binary, environment variables, and config paths.
- Adjusted installation instructions and badges to use the updated repository and module paths.

## [v0.2.8]

### Features ✨
- Added shared worktree clean inspector to normalize `require_clean` semantics; filters untracked/ignored files and surfaces tracked file details in warnings.
- Aligned branch change behavior to match `git switch` semantics: branches always switch even on dirty worktrees, with `require_clean` gating refresh/pull stages.
- Workflows can now capture and restore branches or commits explicitly using new DSL blocks for better automation control.

### Improvements ⚙️
- Updated branch change refresh to skip pulls on dirty worktrees, issuing structured warnings with tracked dirty files to inform users.
- Refactored workflow presets and safeguards to relax branch cleanliness requirements while still guarding against unwanted changes.
- Enhanced branch change commands and rename executors to use shared clean-check helpers, improving consistency across CLI and workflows.
- Workflow actions now report repository events for untracked/ignored files allowing refresh to continue with warnings instead of failures.

### Bug Fixes 🐛
- Fixed issue where untracked files blocked refresh operations by excluding them from cleanliness checks.
- Corrected branch change refresh expectations to properly handle dirty worktree states without interrupting branch switching.
- Reduced false hard stops on workflow steps by honoring `ignore_dirty_paths` in worktree filters and safegaurds.

### Testing 🧪
- Added tests covering dirty worktree skip of branch refresh, untracked files during refresh, and capturing and restoring branch state.
- Implemented scripted Git executor in tests to simulate remote and status command outputs for more reliable and comprehensive test cases.

### Docs 📚
- Documented new normalized `require_clean` semantics and branch change refresh behavior in the issue tracker.
- Updated workflow configurations to remove redundant hard stop requires_clean and reflect new relaxed worktree cleanliness guards.
- Added notes explaining role and process changes for staff engineers regarding repository refactoring and development workflows.

## [v0.2.7]

### Features ✨
- Accept yes/no toggles for all boolean flags.

### Improvements ⚙️
- Allow stash and commit refresh options to skip pre-clean failure.
- Use toggle flags consistently in command flags.

### Bug Fixes 🐛
- Prevent branch change if refresh requires a clean worktree and it is dirty.

### Testing 🧪
- Enhance tests to cover branch refresh with clean checks and dirty worktree blocking.
- Update branch change action test to verify additional status command execution.

### Docs 📚
- _No changes._

## [v0.2.6]

### Features ✨
- _No changes._

### Improvements ⚙️
- Added configuration to ignore specific dirty paths during gitignore application for cleaner workflows.

### Bug Fixes 🐛
- Honor ignore_dirty_paths setting in the gitignore application process.

### Testing 🧪
- _No changes._

### Docs 📚
- _No changes._

## [v0.2.5]

### Features ✨
- Accept string values for `require_clean` directive enabling refined workspace cleanliness checks.
- Support nested `require_clean` options including `enabled` and `ignore_dirty_paths` for flexible configuration.
- Add capture and restore for initial branch state in account rename and gitignore workflows for improved branch handling.

### Improvements ⚙️
- Default change directory refresh is now enabled with safe skip when no tracking remote exists.
- Worktree cleanliness checks now filter ignored dirty file patterns before deciding cleanness.
- Enhanced workflow safeguards to parse and evaluate complex `require_clean` configurations.
- Refactor default config and YAML workflows to align with new `require_clean` map structure.
- Expand branch change action to conditionally refresh based on tracking remote presence.
- Improve error reporting consistency in clean worktree guard with better status information.
- Update account rename replacements to generic username substitution improving maintainability.
- Internal test coverage extended to verify dirty ignore patterns and clean worktree guard behavior.

### Bug Fixes 🐛
- Fixed incorrect initial commit capture name to initial branch in workflows to restore correct branch.
- Prevent false dirty statuses when only ignored dirty paths are modified.

### Testing 🧪
- Added comprehensive unit tests for action guards enforcing ignore patterns on dirty worktrees.
- Workflow safeguard tests enhanced to cover nested `require_clean` maps and ignore path behavior.
- Added branch change action test asserts for tracking remote config checks and refresh skip.

### Docs 📚
- Update README to clarify `require_clean` new nested syntax and default behavior.
- Document new `capture` and `restore` semantics for branch variables in README examples.

## [v0.2.4]

### Features ✨
- Add capture and restore blocks with variable names and kind (branch or commit) for workflow DSL to enable saving and reverting the current state.
- Support branch change action to capture and restore branch or commit state within workflows.
- Refine capture DSL with named variables and align it with the `kind` keyword for clarity and validation.

### Improvements ⚙️
- Protect capture state per repository to avoid conflicts during concurrent workflow executions.
- Update capture DSL and improve coverage.
- Restore original commit after gitignore and account rename flows to maintain repository state consistency.

### Bug Fixes 🐛
- Add missing imports for capture storage to ensure functionality.
- Prevent branch.change action from capturing and restoring simultaneously to avoid conflicts.

### Testing 🧪
- Add extensive tests for branch change action capture and restore behavior, including overwrite protection and restore using branch or commit.
- Implement branchCaptureExecutor stub to simulate git commands for reliable testing.

### Docs 📚
- Document new `capture` and `restore` workflow options with examples in the README.
- Update ISSUE.md with resolutions related to capture and restore features.
- Add examples for restoring original commit in account-rename and gitignore workflow configs.

## [v0.2.3]

### Features ✨
- Allow `ignore_dirty_paths` safeguard to permit ignoring specific dirty files/directories when `require_clean` is true.
- `.gitignore` workflow now includes managed entries for additional service files like `.env`, `.DS_Store`, `qodana.yaml`, `.idea/`, `tools/`, and `bin/`.

### Improvements ⚙️
- Improved account renaming flow.
- Enhanced `.gitignore` workflow to add and ensure proper ignore entries.
- Refined safeguard evaluation to respect status codes and ignore specified dirty paths when filtering dirty ignores.

### Bug Fixes 🐛
- Fixed issue where `require_clean` safeguard incorrectly reported failure when only managed dirty files (e.g., `.DS_Store`, `.env`) were present.

### Testing 🧪
- Added tests for `ignore_dirty_paths` safeguard behavior in workflow and repository state.
- Integration tests verifying `require_clean` with ignored paths functioning correctly.
- Additional coverage for filtering status entries and safeguard evaluations.

### Docs 📚
- Updated README to document `ignore_dirty_paths` safeguard option and usage examples.
- Documented new gitignore managed entries and safeguard behavior in configuration files.

## [v0.2.2]

### Features ✨
- Added audit report columns to show worktree dirty status and list files needing attention.

### Improvements ⚙️
- Improved account renaming flows for better consistency and coverage.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Enhanced tests to cover new worktree dirty state and dirty files in audit reports.

### Docs 📚
- Updated ISSUE tracking with new audit worktree visibility feature.

## [v0.2.1]

### Features ✨
- _No changes._

### Improvements ⚙️
- Updated CLI design and documentation for enhanced user experience.
- Clarified Go 1.25 baseline requirements.
- Aligned module namespaces for improved consistency.
- Converted multiple repo commands (rename, remote update, protocol convert, files add/replace, release) to workflow presets with CLI shims for streamlined execution.
- Separated safeguards into hard-stop and soft-skip categories for clearer task failure behavior.
- Enhanced workflow logging with human-readable formats and parallelized execution for repo-scoped operations.

### Bug Fixes 🐛
- Fixed issue with recursive replacements to apply across all files in namespace rewrites.
- Resolved workflow tasks skipping or failing due to improper handling of repository skip signals.
- Prevented unnecessary git pull rebase warnings when creating new branches without remotes.
- Corrected append-if-missing template behavior for multi-line file updates.
- Fixed module path and repository remote URL mismatch to ensure correct installation and badge links.
- Fixed partial application of file replacements during workflows for comprehensive changes.

### Testing 🧪
- Added regression tests covering recursive replacements.
- Added coverage to ensure skipped repositories prevent subsequent failing steps.
- Updated and added tests for CLI workflow shim commands.

### Docs 📚
- Refreshed CLI design documentation.
- Updated issues log formatting and instructions.
- Enhanced README and architecture docs with updated environment and usage details.

## [v0.2.0]

### Features ✨
- _No changes._

### Improvements ⚙️
- Force workflow commands to always use human-readable formatter, removing legacy structured logs.
- Support replace mode globs in taskfiles to correctly handle file patterns.
- Use replace action for Go file globs in cleanup tasks to avoid creating literal `**/*.go` files.
- Suppress workflow logging during changelog and commit message commands for cleaner output.

### Bug Fixes 🐛
- Removed legacy workflow logging outputs (TASK_PLAN/TASK_APPLY) across commands.
- Disabled legacy CLI structured report formatter, ensuring only human-readable output remains.
- Fixed changelog and commit message commands to avoid emitting workflow logs.
  
### Testing 🧪
- Added tests to confirm workflow logging suppression in changelog and commit message commands.

### Docs 📚
- Updated changelog to mention issue GX-419 related to glob support in replace mode.

## [v0.2.0-rc.13]

### Features ✨
- Enforce typed options in history and rename commands.
- Add concurrent repository execution to workflow.
- Add preset workflow helpers and convert workflow commands to presets.

### Improvements ⚙️
- Redesigned workflow logging and improved safeguard tracking between hard-stop and soft-skip.
- Synced repository playbook documentation and enhanced audit fallback handling.
- Added typed preset builders and preset workflow helper utilities.
- Propagate owner variable to canonical workflow preset.
- Ensure folder-rename preset uses boolean options.
- Clean workflow logging headers.
- Validate retag mapping inputs.
- Gate history variable overrides to history actions.
- Preserve severity indicators for phase events.

### Bug Fixes 🐛
- Refined workflow human-readable logging.
- Add hard-stop vs soft-skip safeguards.
- Converted repo-files-add, repo-files-replace, remote update, and release commands to workflow presets.
- Removed bespoke repo workflow helpers.
- Ensure repo files add skips pushes when disabled.
- Fixed repo workflow executor wiring.

### Testing 🧪
- Covered negative paths in files-add tests.
- Updated audit fallback expectations.
- Added integration coverage for workflow preset variables.

### Docs 📚
- Added GX-412 refactor plan and marked GX-417 issue as resolved.
- Updated Go backend and git agent coding standard documents.
- Improved repo playbook documentation to align with new features and workflows.

## [v0.2.0-rc.12]

### Features ✨
- Execute workflow tasks per repository with repository-scoped stages and deduplication.
- Added atomic git command steps and first-class workflow commands for fine-grained git/file operations.
- Seed run IDs for branch templates enable improved workflow tracking.

### Improvements ⚙️
- Decomposed workflow executor into discrete actions with reusable guard helpers for better independence and failure reporting.
- Enhanced workflow logging with concise summaries per repository; eliminated verbose per-stage logs.
- Command failures now include invoked arguments and first stderr line for improved error clarity.
- Added support for normalized carriage-return line endings in append-if-missing mode.
- Removed legacy preview mode and improved configuration namespaces for commit and changelog message generation.
- Legacy commands and configuration keys now alias to new workflows with deprecation warnings.
- Improved safeguard and skip logic to halt workflow execution on repository skips.
- Normalized and renamed modes from line-edit to append-if-missing.

### Bug Fixes 🐛
- Fix append-if-missing mode to append all lines, not just the first, with regression tests.
- Prevent running unnecessary git pulls on new branches without remotes.
- Stop workflow execution after repository-scoped TASK_SKIP events to avoid failed operations on skipped repos.
- Correct branch change logic to avoid tracking remote refs for new automation branches.
- Workflow commands now execute directly, removing unsupported command errors.
- Fixed output path order and included path in workflow event summaries.
- Fixed literal line matching for append-if-missing to prevent incorrect line omission.
- Fixed log formatter to include event summaries properly.

### Testing 🧪
- Added comprehensive tests for append-if-missing mode and task planning/execution.
- Enhanced executor and workflow command unit tests to cover new action decompositions and skipping behavior.

### Docs 📚
- Updated documentation for pkg/llm usage and workflow command orchestration.
- Added detailed design notes in ARCHITECTURE.md regarding embedded workflows.
- Refactored plans and improved README to explain new workflow capabilities and logging improvements.

## [v0.2.0-rc.11]

### Features ✨
- Added `append-if-missing` file mode for workflow-managed files to append missing lines without overwriting existing content, ideal for `.gitignore` enforcement.
- Renamed legacy `line-edit` mode to `append-if-missing` with backward compatibility acceptance and new `ensure-lines` file mode added.
- Introduced reusable `pkg/llm` package for large language model integrations with lightweight interfaces and configurable HTTP plumbing.

### Improvements ⚙️
- Refactored workflow tasks and task executor to support the new `append-if-missing` mode with proper planning and applying logic.
- Updated CLI options and documentation to include the new file mode `append-if-missing` for commands like `gix files add` and workflow license preset.
- Removed legacy `line-edit` mode references and replaced with `append-if-missing` and `ensure-lines` naming for clarity.
- Enhanced testing coverage for task planning and execution of the new file modes, ensuring correct behavior and idempotence.
- Added detailed documentation for `pkg/llm` explaining configuration, usage, and design principles.
- Decomposed the workflow executor into discrete actions (`git.branch.prepare`, `files.apply`, `git.stage`, `git.commit`, `git.push`, `pull-request.create`) with reusable guard helpers so file edits, commits, and pushes run independently and emit precise failures.
- Added first-class workflow commands for each action (`git stage`, `git commit`, `git stage-commit`, `git push`, `pull-request create`, `pull-request open`) plus per-task `steps` configuration, enabling workflows to orchestrate small git/file operations explicitly (see the updated gitignore workflow).
- Shell command failures now report the invoked arguments and the first line of stderr, making `git`/`gh` workflow errors actionable.

### Bug Fixes 🐛
- Fixed legacy mode acceptance issues by supporting the renamed `append-if-missing` mode in parser and executor components.
- `gix workflow` now executes workflow operations directly, so git action steps such as `git stage-commit` run without triggering “unsupported workflow command” errors (the gitignore preset works again).
- `branch.change` no longer attempts to create new automation branches with `--track origin/<branch>` when the remote ref does not exist, allowing presets like `gitignore` to create/push fresh branches without invalid reference errors.
- `append-if-missing` now normalizes carriage-return line endings before evaluating and applying changes so templates written on Windows append every line instead of collapsing into a single entry.
- `append-if-missing` now compares literal line content (whitespace intact) so `.envrc`, `*.env`, or indented variants no longer satisfy the `.env` check and prevent the line from being appended.
- Repository-scoped `TASK_SKIP` events (for example, dirty worktrees) now propagate a skip sentinel so later workflow steps stop executing against that repository instead of running `git stage-commit`/push commands on a repo that was already skipped.
- Removed verbose per-stage workflow logging so only the final summary is printed when running `gix workflow`.
- Added concise workflow logging that groups events per repository, collapses `TASK_PLAN`/`TASK_APPLY` noise, and highlights warnings/errors without the previous wall of text.
- `branch.change` no longer runs `git pull --rebase` immediately after creating a brand new local branch without a tracking remote, eliminating spurious `PULL-SKIP` warnings during workflows.

### Testing 🧪
- Added comprehensive unit tests for parsing task file modes including the new `append-if-missing`.
- Added tests verifying task planner skips when lines are already present and executor correctly appends missing lines only.
- Expanded integration tests for task executor's append-if-missing functionality in workflow-managed files.

### Docs 📚
- Documented `append-if-missing` mode in README, schema highlights, and CLI command descriptions with examples.
- Created detailed `pkg/llm` integration guide with overview, configuration, and usage instructions.
- Updated workflow packages documentation reflecting new file modes and usage patterns.

## [v0.2.0-rc.10]

### Features ✨
- _No changes._

### Improvements ⚙️
- Removed all legacy CLI command wrappers including `gix commit`, `gix changelog`, and `gix repo-license-apply`; only canonical namespaces remain.
- Replaced deprecated `repo-license-apply` command with the `workflow license` preset using runtime variables.
- Simplified configuration by dropping alias normalization and warning layers related to removed commands.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- _No changes._

### Docs 📚
- Updated README and ARCHITECTURE.md to remove references to legacy commands and promote workflow presets.
- Revised command tables and help output to reflect streamlined CLI surface without deprecated wrappers.

## [v0.2.0-rc.9]

### Improvements ⚙️
- Renamed the `branch-cd` command to `cd`, added deprecation warnings when the legacy name is used, and allowed the branch argument to default to the repository's detected default or configured fallback.
- Folded the `branch-refresh` behaviour into `gix cd` via `--refresh`/`--stash`/`--commit` flags and removed the standalone command while preserving migration warnings for legacy configuration.
- Renamed the `branch-default` command to `default`, added workflow/config alias warnings, and updated CLI defaults, docs, and workflow tests to reference the new surface while keeping `branch-default` as an alias.
- Introduced a top-level `message` namespace and moved `changelog message` to `message changelog`, keeping the legacy path as a deprecated alias with config/CLI warnings.
- Added embedded workflow presets so `gix workflow --list-presets` and `gix workflow <preset>` can run bundled automation (initially `license`) without a separate YAML file.
- Added workflow runtime variables (`gix workflow --var key=value` / `--var-file path.yaml`) so presets and file-based configs can consume user-supplied values.
- Added an `append-if-missing` file mode for workflow-managed files so recipes like the gitignore preset can append missing entries without overwriting existing content.
- Deprecated `gix repo-license-apply` in favor of the `workflow license` preset; the CLI prints a warning and forwards legacy flags as workflow variables.
- Removed the remaining legacy CLI wrappers (`gix commit`, `gix changelog`, and `gix repo-license-apply`) along with the configuration alias/warning layer so only the canonical namespaces remain.
- Moved history purge under `gix files rm` and removed the legacy `gix rm` command, updating configuration defaults/docs/tests to the new namespace.
- Added an embedded `namespace` preset and removed the `gix namespace rewrite` CLI; `gix workflow namespace` is now the single supported entrypoint.
- Added explicit `make test-fast`/`test-slow` targets (with `ci` wiring) so fast unit packages can run independently of the slower `./tests` integration suite.

### Testing 🧪
- Added unit coverage for branch change task actions to verify repository-default and configured fallback branch resolution.
- Expanded workflow command coverage to exercise preset execution, preset listing, runtime variable parsing, and the reusable configuration parser.
- Refactored integration Git fixtures into shared helpers (stubbed GH binary builder + git repository factory), updated repos/no-remote suites to reuse them, and added tests ensuring the helpers create remotes and PATH stubs correctly.

## [v0.2.0-rc.8]

### Features ✨
- _No changes._

### Improvements ⚙️
- Updated branch migration logic to skip further checks when the remote destination branch matches the target branch.
- Enhanced test coverage for branch migration operations, including scenarios for skipping remote push and skipping migration when remote already matches the target.
- Refined default branch detection in tests to use "main" instead of "master" for better alignment with modern defaults.

### Bug Fixes 🐛
- Fixed issue where branch migration would perform unnecessary checks even if the remote destination branch already matched the target.

### Testing 🧪
- Added tests to verify branch migration skips when remote branch matches target.
- Improved scripted executor mocks to handle default branch names dynamically.
- Adjusted existing tests to reflect updated branch naming conventions and migration logic.

### Docs 📚
- Added a note in ISSUES.md regarding removal of the `--preview` flag and associated logic.

## [v0.2.0-rc.7]

### Features ✨
- Exposed LLM configuration and task variables in workflow steps.
- Restructured CLI command surface to flatten namespaces, removing legacy `repo` and `branch` wrappers.
- Added workflow `tasks apply` step to build LLM clients and capture action output into workflow variables.

### Improvements ⚙️
- Fixed workflow summary totals to correctly count repositories without metadata and emit human-readable durations.
- Enhanced task executor skip logs to include git status entries for dirty worktrees.
- Clarified namespace rewrite skip reasons distinguishing no references versus gitignored files.
- Documented resolution improvements and updated workflow DSL to use command path arrays.
- Workflow execution now continues after failures to improve robustness.

### Bug Fixes 🐛
- Fixed aggregation error in reporting.
- Fixed panic caused by missing GitHub client in `prs delete`.
- Ensured changelog summary counts repositories correctly.
- Corrected CLI roots handling and warnings on positional arguments for file commands.

### Testing 🧪
- Added extensive tests for workflow executor, LLM configuration, and task actions.
- Improved coverage for CLI commands including rename, workflow, and release commands.
- Added integration tests for branch default, migrate, no-remote, packages, repos, and workflow scenarios.

### Docs 📚
- Added detailed refactor plan for workflow capture and task execution.
- Updated CLI command documentation to reflect flattened command structure.
- Improved workflow guidance with templating details and sample YAML for `apply-tasks`.
- Clarified namespace rewrite and resolution documentation.
Summary: total.repos=1 WARN=0 ERROR=0 duration_human=6.635s duration_ms=6635

## [v0.2.0-rc.6]

### Features ✨
- Added `gix files add` command to seed files with configurable content, permissions, branch, and push settings.
- Introduced `gix release retag` command and workflow action to remap tags to new commits with force-push.
- Added `gix license apply` command to distribute license files via workflow tasks.
- Refactored workflow DSL to use command path arrays instead of legacy operation keys.

### Improvements ⚙️
- Swapped workflow and configuration schemas to use `command` path arrays; updated CLI defaults, docs, and tests accordingly.
- Clarified namespace rewrite skip reasons and improved tolerance for missing metadata.
- Workflow executor now skips GitHub metadata lookups when disabled to avoid panics.
- Updated workflow examples and documentation to reflect command-based DSL.
- Fixed staticcheck warnings and documented linting steps.
- Workflow `tasks apply` can now build LLM clients from configuration and capture action output into workflow variables for later steps.
- Task executor skip logs now include git status entries so dirty repositories list the blocking files.
- Summary reporter now counts repositories without metadata and emits `duration_human` alongside `duration_ms`.
- Flattened CLI so repository and branch operations are exposed directly (`folder rename`, `remote update-*`, `branch-*`) without the legacy `repo`/`branch` wrappers.

### Bug Fixes 🐛
- Fixed panic caused by `gix prs delete --yes yes` due to nil GitHub client.
- Corrected namespace rewrite skip messages to distinguish between no references and gitignored files.
- Ensured `gix files add` respects CLI roots and warns on positional arguments.
- Emitted retag mappings as generic arrays for workflow compatibility.

### Testing 🧪
- Added regression tests covering license apply, release retag, and files add commands.
- Covered edge cases for `--yes` flag in branch commands.
- Added unit and integration tests for workflow command path DSL changes.
- Expanded tests for namespace rewrite skip scenarios.

### Docs 📚
- Updated README and ARCHITECTURE.md to document new command-based workflow DSL.
- Added examples for new commands and workflows.
- Improved ISSUE.md with resolved feature statuses and detailed resolutions.
- Enhanced error message documentation for release tag creation and namespace rewrite.
Summary: total.repos=0 duration_ms=0

## [v0.2.0-rc.4]

### Features ✨
- _No changes._

### Improvements ⚙️
- Added workflow-level repository deduplication to ensure changelog actions run once per path.
- Enhanced branch switch command to fetch all remotes and pull when the configured remote is missing, emitting a fetch fallback warning.
- Improved namespace rewrite to update Go test files, rewriting imports to the new module prefix and staging changes.

### Bug Fixes 🐛
- Fixed skipping of gitignored nested repositories and files during namespace rewrite.
- Fixed namespace rewrite to update Go test files, ensuring `_test.go` imports follow the new module prefix.
- Fixed workflow apply-tasks to skip duplicate repositories so changelog actions emit a single section per path.
- Fixed namespace workflows to warn and proceed when the configured start point branch is missing, defaulting to the current HEAD.
- Fixed branch change workflows deduplicating relative roots so each repository logs `REPO_SWITCHED` exactly once.
- Fixed namespace task log formatting to emit actual newlines instead of escaped `\n`.
- Fixed namespace push failures by capturing git stderr and degrading push/auth errors into actionable skip messages.
- Fixed branch default command to fail fast with clear error when GitHub token is missing.

### Testing 🧪
- Added regression tests to prevent duplicate changelog message generation.
- Added tests covering namespace rewrite handling for Go test files.
- Added tests ensuring workflow executor canonicalizes repository paths to avoid duplicate branch switch logs.
- Expanded unit and integration tests for workflows skipping gitignored nested repositories.

### Docs 📚
- Updated ISSUES.md with resolutions for namespace rewrite, repository discovery, changelog duplication, missing start branch, and branch switch improvements.
Summary: total.repos=0 duration_ms=0

## [v0.2.0-rc.3]

### Features ✨
- Added `namespace rewrite` command with a namespace rewrite service and workflow action to update Go module paths across repositories.
- Added structured workflow logging with aligned human-readable columns and machine-parseable key/value pairs.
- Added `branch default` command enhancements to create missing branches and accept target branch as a positional argument.
- Added reusable workflow safeguards to declaratively skip repositories before mutating operations.

### Improvements ⚙️
- Introduced validated domain types for repository paths, remotes, and branch names, improving executor and workflow option consistency.
- Standardized error schema across commands with centralized workflow error formatter and stable sentinel codes.
- Enhanced workflow execution with DAG-based parallel execution of independent operations and improved error handling.
- Remote workflow operations now emit standardized skip and warning messages for missing remotes and metadata.
- Added support for workflow task-level reusable safeguards including clean worktree and branch checks.
- Normalized GitHub token environment variables and improved CLI version command consistency.
- Centralized remote identity normalization and improved branch-default handling for inaccessible remotes.
- Updated CI and release workflows to use Go 1.25 with caching and latest version checks.

### Bug Fixes 🐛
- Fixed skipping of gitignored nested repositories and files during namespace rewrite.
- Fixed namespace rewrite to update Go test files, ensuring `_test.go` imports follow the new module prefix.
- Fixed workflow apply-tasks to skip duplicate repositories so changelog actions emit a single section per path.
- Fixed namespace workflows to warn and proceed when the configured start branch is missing, defaulting to the current HEAD.
- Fixed branch change workflows deduplicating relative roots so each repository logs `REPO_SWITCHED` exactly once.
- Fixed namespace task log formatting to emit actual newlines instead of escaped `\n`.
- Fixed namespace push failures by capturing git stderr and degrading push/auth errors into actionable skip messages.
- Fixed branch default command to fail fast with clear error when GitHub token is missing.
- Fixed workflow logs to render namespace logs with real newlines and handle namespace push failures gracefully.
- Fixed `gix r prs delete --yes` hang by skipping GitHub metadata lookups when token is missing.
- Fixed handling of repositories without remotes to avoid failures across commands.
- Fixed detection of namespace root in `go.mod` and retention of namespace commit messages during merges.
- Fixed acceptance of branch prefix hyphen option and rewriting of `go.mod` block entries.
- Fixed bug causing workflow operation prefixes to be redundant and standardized repository error messaging.

### Testing 🧪
- Added extensive regression coverage for namespace rewrite service, workflow safeguards, error formatting, and push failure handling.
- Added integration tests covering no-remote branch workflows and namespace safeguards.
- Added tests for canonical assume-yes flag in namespace CLI and workflow DAG execution.
- Added coverage for default branch updates on mixed nested repositories and branch default token validation.

### Docs 📚
- Updated README with full list of gix commands and workflow descriptions.
- Added CLI design documentation and acknowledged no-remote handling coverage in issues.
- Noted namespace push safeguards in issues log and documented workflow examples.

## [v0.2.0-rc.1]

### Features ✨
- Added `namespace rewrite` command backed by a namespace rewrite service and workflow action to update Go module paths across repositories.
- Added `files replace` command for file replacement tasks across repositories.
- Added `rm` command with task-runner orchestration and preview previews.
- Routed the workflow CLI through the shared task runner so declarative workflow steps execute as orchestrated tasks.
- `branch default` command now accepts the target branch as a positional argument (`gix b default master`) while retaining configuration fallbacks and removing the legacy `--to` flag.

### Improvements ⚙️
- Workflow tasks now support reusable safeguards (clean worktree, branch, path checks) so repositories can be skipped declaratively before mutating operations.
- Introduced validated domain types for repository paths, owner/repo tuples, remotes, and branch names, refactoring repository executors and workflow options to consume the new constructors.
- Added a contextual error catalog and updated repository executors/workflow bridges to emit stable sentinel codes instead of ad-hoc failure strings.
- Consolidated repository helper utilities (optional owner parsing, confirmation policies, shared reporter) and removed duplicated normalization across workflows.
- Downgraded GitHub Pages configuration failures encountered during `branch default` to warnings so branch promotion proceeds when Pages is not configured.
- `branch-cd` reports network issues as `FETCH-SKIP`/`PULL-SKIP` warnings instead of aborting when remotes are missing or offline.
- Refined repository executors and workflow bridges to use the new domain constructors and error handling.

### Bug Fixes 🐛
- Prevented `branch-cd` from aborting when repositories lack remotes by skipping network operations and creating untracked branches.
- Fixed history purge test alignment with multi-path commands.
- Fixed audit roots handling after renames and improved test coverage.

### Testing 🧪
- Expanded regression coverage for repository domain constructors, protocol conversion edge cases, dependency resolvers, and workflow canonical messaging to enforce policy guarantees.
- Added coverage for task executor behavior and workflow command unit tests.
- Updated `make ci` to run additional linters (`go vet`, `staticcheck`, `ineffassign`) before tests and cleaned up legacy unused helpers to keep the new gates green.
- Covered policy regressions related to repository domain and workflow messaging.

### Docs 📚
- Added `ARCHITECTURE.md` documenting command wiring, package layout, configuration internals, and current package responsibilities with workflow step registration details.
- Re-centered README on user workflows and refreshed CLI design documentation with repository domain model coverage, prompt/reporting semantics, and cross-links from `POLICY.md`.
- Added the GX-402 refactor roadmap capturing policy gaps, domain/error refactors, and test expansion tasks.

## [v0.1.4]

### Features ✨
- Renamed `branch migrate` command to `branch default` to promote a branch as the repository default.
- Auto-detect current default branch via GitHub metadata, removing the need to specify source branch.
- Updated CLI, workflow, tests, configs, and documentation to reflect the new `branch default` command.

### Improvements ⚙️
- Refreshed README and workflow examples to use `branch default` instead of `branch migrate`.
- Enhanced safety gates and automation for default branch promotion.
- Streamlined configuration and command hierarchy for branch management commands.
- Added `rm` command to purge history via git-filter-repo with task-runner orchestration and preview previews.
- Routed the workflow CLI through the shared task runner so declarative steps execute via workflow tasks while retaining legacy audit report file output and stdout banners.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Updated internal and integration tests to cover the new `branch default` command behavior.
- Refactored tests to remove references to `branch migrate`.
- Added task executor coverage guarding against action-only apply logs and rewrote workflow command unit tests to assert emitted task definitions.

### Docs 📚
- Updated README and CLI design docs to document `branch default` command usage.
- Added issue tracking entry for branch command rename and behavior changes.

## [v0.1.3]

### Improvements ⚙️
- Refined the `--init` flag help to present `<LOCAL|user>` with the default scope highlighted and clearer destination details.

### Testing 🧪
- Added choice placeholder formatting coverage and ensured CLI configuration precedence honors explicit `--config` flags.

### Docs 📚
- Documented the capitalized `LOCAL` scope in the configuration initialization section of the README.

## [v0.1.2]

### Features ✨
- _No changes._

### Improvements ⚙️
- Rewrote README command catalog table to reflect current commands, removing legacy references.

### Bug Fixes 🐛
- Removed owner equality guard for canonical remotes to allow updates when repository ownership has changed.

### Testing 🧪
- Adjusted tests to cover the removal of the owner constraint guard on canonical remote updates.
- Added coverage for command hierarchy and alias resolution.

### Docs 📚
- Updated ISSUES.md with new task planning details and resolutions related to command catalog and logging changes.

## [v0.1.1]

### Features ✨
- _No changes._

### Improvements ⚙️
- Improved autonomous flow for better operation.

### Bug Fixes 🐛
- Clarified owner constraint skip message for better understanding.
- Logged configuration banner at debug level for cleaner logs.
- Various bug fixes to enhance stability.
- Restored the `--owner` flag for `remote update-to-canonical` so CLI workflows can keep owner-scoped folder plans aligned while still tolerating canonical owner migrations.

### Testing 🧪
- Added tests and improved test coverage in CLI application and remotes.

### Docs 📚
- Updated AGENTS.md with detailed front-end coding standards and backend principles.
- Enhanced documentation on validation policies and project structure.
- Added review checklist and assistant workflow guidelines.

## [v0.1.0]

### Features ✨
- Added a `commit message` command that summarizes staged or worktree changes with the shared LLM client and returns a Conventional Commit draft.
- Added a `changelog message` command that turns tagged or time-based git history into Markdown release notes using the shared LLM client.
- Added a `branch-cd` command that fetches, switches, and rebases repositories onto the requested branch, creating it from the remote when missing.
- Added a `release` command that annotates tags with customizable messages and pushes them to the selected remote across repositories.

### Improvements ⚙️
- Introduced hierarchical command namespaces (`repo`, `branch`) with short aliases (`r`, `b`, `a`, `w`) and removed the legacy hyphenated commands.
- Updated CLI bootstrap to register alias-aware help so the new paths and shortcuts surface in command discovery.
- Nested `commit message` under the `branch` namespace and `changelog message` under `repo` to keep related commands grouped.

### Bug Fixes 🐛
- Updated `release` help to surface the required `<tag>` argument along with usage guidance and examples across the CLI.
- Updated `branch-cd` help to surface the required `<branch>` argument along with usage guidance and examples.
- Ensured `release` falls back to the embedded `.` repository root when user configuration omits the operation defaults.
- Updated `workflow` help text to surface the required configuration path and example usage.
- Disabled default CLI info logging and set the default log level to `error` so commands run silently unless verbosity is explicitly requested.
- Downgraded the configuration initialization banner to DEBUG so standard operations continue logging at INFO severity only.
- Clarified the remote owner constraint skip message to spell out the required `--owner` value and detected repository owner.
- Allowed canonical remote updates to proceed regardless of the configured `--owner` constraint, supporting repositories that migrated between accounts.
- Added `SKIP (already normalized)` messaging to `folder rename` so re-running normalization reports repositories that already match canonical naming.

### Testing 🧪
- Added application command hierarchy coverage to ensure aliases and nested commands resolve to the existing operations.
- Added task operation planner/executor unit tests and a workflow CLI integration test covering the new `apply-tasks` step.
- Added unit coverage for the LLM client wrapper, commit message generator, changelog generator, and CLI preview flows.
- Added branch-cd service and command tests covering fetch/switch/create flows and CLI execution.
- Added release service and CLI tests verifying tag annotation, push behavior, and preview handling.
- Added CLI and command unit tests to enforce the `<branch>` usage template for `branch-cd`.
- Added configuration and CLI tests confirming the `release` command retains default roots without explicit configuration.
- Added branch refresh coverage to exercise the command-level `--branch` flag after removing the global variant.

### Docs 📚
- Documented the new CLI syntax and shortcuts in `README.md`, including refreshed quick-start examples.
- Added `apply-tasks` workflow guidance to `README.md`, including templating details and sample YAML.
- Documented the `commit message` assistant, configuration knobs, and usage examples.
- Documented the `changelog message` assistant, baseline controls, and sample invocations in `README.md`.
- Documented the `branch-cd` helper with usage notes and remote/preview options.
- Documented the `release` helper including remote overrides, custom messages, and preview support.
- Documented the branch command expectations now that the global `--branch` flag is removed.
- Refreshed the README command catalog with up-to-date command paths and shortcuts.

## [v0.0.8] - 2025-10-07

### Features ✨
- Added a `branch-refresh` command that fetches, checks out, and pulls branches with optional recovery strategies and clean-worktree enforcement.
- Introduced a root `--version` flag that prints the detected gix version and exits before executing subcommands.

### Improvements ⚙️
- Branch refresh now survives intermediate rebase checkpoints by attempting to recover from checkpoint commits.
- Version output messages use the `gix` prefix for consistent CLI presentation.

### Bug Fixes 🐛
- `repo-prs-purge` prompts before deleting branches, respecting `--yes`, apply-to-all decisions, and reuse of confirmations during batch cleanup.
- Nested Git repositories are renamed before their parents during directory normalization to avoid conflicting rename sequences.

## [v0.0.7] - 2025-10-06

### Improvements ⚙️
- Guarded destructive repo-prs-purge operations behind confirmation prompts, centralizing apply-all handling and `--yes` defaults.
- Updated AGENTS and configuration guidance to minimize unnecessary output streaming.

### Testing 🧪
- Expanded coverage for nested rename ordering, branch cleanup prompting, and integration workflows.

### Features ✨
- Added a `branch-refresh` command that fetches, checks out, and pulls branches with optional recovery strategies and clean-worktree enforcement.
- Introduced a root `--version` flag that prints the detected gix version and exits before executing subcommands.

### Improvements ⚙️
- Branch refresh now survives intermediate rebase checkpoints by attempting to recover from checkpoint commits.
- Version output messages use the `gix` prefix for consistent CLI presentation.

### Bug Fixes 🐛
- `repo-prs-purge` prompts before deleting branches, respecting `--yes`, apply-to-all decisions, and reuse of confirmations during batch cleanup.
- Nested Git repositories are renamed before their parents during directory normalization to avoid conflicting rename sequences.

## [v0.0.6] - 2025-10-06

### Highlights
- Audit reports now surface non-repository directories with `--all` while presenting folder names relative to their configured roots for quicker scanning.
- Root sanitization trims nested duplicates so CLI commands and workflows operate on a predictable set of repositories.

### Features ✨
- Added an `--all` toggle to `audit` to include top-level directories lacking Git metadata, filling Git-specific columns with `n/a` in CSV output and workflow reports.

### Improvements ⚙️
- Reordered audit CSV columns to lead with `folder_name` and emit paths relative to each root, preserving canonical-name checks through the basename.
- Centralized root sanitation to deduplicate nested entries, expand tildes, and enforce the new singular `--root` flag across commands and workflows.
- Surfaced usage guidance whenever `audit` is invoked without required roots to clarify flag expectations.

### Bug Fixes 🐛
- Corrected repository containment detection so nested Git repositories are not skipped when scanning with `--all`.

### Docs 📚
- Expanded installation guidance and refreshed flag examples in `README.md` to reflect the singular `--root` flag and new audit behaviors.

### CI & Maintenance
- Updated the release workflow to build and publish the `gix` binary from this repository.

## [v0.0.5] - 2025-10-06

### Highlights
- CLI configuration discovery now honors `XDG_CONFIG_HOME` while normalizing resolved paths for consistent behavior across platforms and tests.

### Improvements ⚙️
- Updated the application bootstrap to expand XDG-aware configuration search paths and align emitted logs with the resolved directories.

### Testing 🧪
- Normalized temporary path expectations in repository and application tests so macOS `/private` prefixes no longer cause false negatives.

## [v0.0.4] - 2025-10-03

### Highlights
- Owner-aware repository rename workflows create missing owner directories and keep remotes aligned with canonical metadata.
- Boolean CLI toggles now accept yes/no/on/off forms everywhere thanks to shared parsing utilities.
- Operations audit reliably writes reports into nested output directories without manual setup.

### Features ✨
- Added an `--owner` toggle to `repo-folders-rename`, planned via a new directory planner that joins owner and repository segments and ensures parent directories exist.
- Propagated owner preferences through workflow configuration and remote update execution, including owner-constraint enforcement when rewriting origin URLs.
- Introduced reusable toggle flag helpers that register boolean flags accepting multiple literal forms and normalize command-line arguments before parsing.
- Added an `--all` flag to `audit` so directories without Git metadata appear in reports with git fields marked as `n/a`.

### Improvements ⚙️
- Normalized toggle arguments across commands so `--flag value` and `--flag=value` behave consistently for all boolean options.
- Refined rename workflow execution to skip no-op renames and to honor include-owner preferences sourced from configuration files.
- Ensured audit operations create nested target directories before emitting CSV reports.

### Docs 📚
- Documented owner-aware rename and remote update options in `README.md` and `docs/cli_design.md` examples.

### CI & Maintenance
- Added extensive unit coverage for toggle parsing, rename planners and executors, remote owner constraints, and workflow inspection helpers.

## [v0.0.3] - 2025-10-03

### Highlights
- Added a configuration initialization workflow that writes embedded defaults to either local or user scopes.
- Expanded configuration search paths so embedded defaults and user overrides are discovered automatically.

### Features ✨
- Introduced `--init` and `--force` flags that materialize the embedded configuration content with safe directory handling and conflict detection.
- Added integration coverage that exercises initialization end-to-end and verifies configuration loader behavior with new scopes.

### Improvements ⚙️
- Refined configuration loading to merge embedded defaults while tracking duplicates and missing operation definitions precisely.
- Strengthened CLI wiring with richer validation, clearer error surfaces, and deterministic command registration ordering.

### Docs 📚
- _No updates._

### CI & Maintenance
- Expanded unit and integration tests around configuration initialization and loader path resolution.

## [v0.0.2] - 2025-10-03

### Highlights
- Standardized global CLI flags so `--roots`, `--preview`, `--yes`, and `--require-clean` behave consistently across commands.
- Embedded configuration defaults and extended search paths improve out-of-the-box repository discovery.
- Enhanced branch and audit workflows with cleaner logging defaults and additional safeguards.

### Features ✨
- Enabled a shared root-resolution context that exposes `--roots` on every command and centralizes flag handling.
- Added `--from` and `--to` options for branch migration, alongside enforceable clean-worktree checks for branch-level operations.
- Embedded default configuration content into the binary and merged it with user configuration files discovered on disk.

### Improvements ⚙️
- Introduced apply-all confirmation tracking and structured prompt results to streamline batch confirmations.
- Added minimal audit inspection depth, optional branch data skipping, and normalized repository discovery paths for more predictable workflows.
- Defaulted console logging formats and eliminated redundant GitHub CLI view logging to reduce noise.

### Docs 📚
- _No updates._

### CI & Maintenance
- Broadened unit coverage for configuration loaders, CLI application wiring, and integration helpers supporting workflow tests.

## [v0.0.1] - 2025-09-28

### What's New 🎉

1. Bash scripts replaced with Go implementation
2. The config.yaml file stores the defaults 
3. The config.yaml file defines a runnable workflow, chaining multiple commands
