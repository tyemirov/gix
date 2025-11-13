# Changelog

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
