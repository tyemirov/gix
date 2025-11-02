# Git Maintenance CLI Transition Design

## 1. Purpose and Scope
This document inventories the existing Bash automation in this repository and captures the design decisions required to migrate those capabilities into a Go-based command-line interface (CLI). It covers:

- Inputs, flags, dependencies, side effects, and outputs for each script slated for parity.
- A proposed Cobra-based command surface that preserves current behaviors while providing an extensible structure.
- Module path and package layout decisions that respect our Go project guidelines.
- Non-functional requirements covering logging, configuration, and testing (including integration coverage expectations).
- Open items that require approval before implementation begins.

## 2. Script Inventory
The following tables document each script. "Inputs" include both positional arguments and configuration settings. All commands assume execution from a Unix-like shell.

### 2.1 `audit_repos.sh`
| Aspect | Details |
| --- | --- |
| Primary purpose | Audit GitHub repositories across one or more directories, optionally renaming local folders, updating remotes to canonical URLs, or converting remote protocols. |
| Inputs & flags | Positional scan roots (defaults to `.`). Flags: `--rename`, `--update-remote`, `--protocol-from {https\|git\|ssh}`, `--protocol-to {https\|git\|ssh}`, `--dry-run`, `--yes` (`-y`), `--require-clean`, `--debug`. |
| Environment variables | `GIT_TERMINAL_PROMPT=0` (set within script to disable interactive credential prompts). |
| External dependencies | `git`, `gh`, `jq`, `find`, `readlink`/`realpath`, `mv`, `sed`, `awk`, standard GNU coreutils. Requires authenticated `gh` session. |
| Network/API usage | `gh api` to resolve repository canonical metadata; `gh repo view` to determine default branch; optional `git fetch` for sync detection; `git remote set-url` for updates. |
| Side effects | File-system renames of repository directories; remote URL changes; Git fetches; optional prompts; stdout/stderr logging. With `--dry-run`, operations are read-only. |
| Outputs | When the dedicated audit command runs, it emits CSV (header + one line per repo) to stdout. Operational modes emit plan/action messages (`PLAN-OK`, `UPDATE-REMOTE-DONE`, etc.) to stdout and error messages to stderr. Debug logging when `--debug` is set. |

### 2.2 `delete_merged_branches.sh`
| Aspect | Details |
| --- | --- |
| Primary purpose | Delete remote and local branches whose associated pull requests are closed on GitHub. |
| Inputs & flags | No flags or positional arguments. Operates on the current Git repository. |
| Environment variables | None explicitly; relies on Git configuration for `origin`. |
| External dependencies | `git`, `gh`, `awk`, `sed`, `grep`. Requires authenticated `gh` session. |
| Network/API usage | `git ls-remote` to enumerate origin branches; `gh pr list --state closed` to enumerate closed PR head branches. |
| Side effects | Deletes remote branches via `git push origin --delete`; deletes local branches via `git branch -D`. Writes progress messages to stdout. |
| Outputs | Logs deletions or skips to stdout. Errors from Git commands surface on stderr but are guarded with `|| true` to continue processing. |

### 2.3 `main_to_master.sh`
| Aspect | Details |
| --- | --- |
| Primary purpose | Safely migrate a repository’s default branch from `main` to `master`, updating workflows, GitHub Pages, PR targets, local branches, and remote settings. |
| Inputs & flags | Target branch as positional argument (for example, `master`) plus optional `--debug` flag (enables shell tracing). Must run inside target repository. |
| Environment variables | Relies on Git configuration for remote URLs and GitHub CLI authentication context. |
| External dependencies | `git`, `gh`, `jq`, `sed`, `find`, standard GNU coreutils. Requires authenticated `gh` session. |
| Network/API usage | Extensive `gh api` usage: fetch repository metadata, GitHub Pages config, branch protection checks, open PRs. Uses `gh pr list`/`gh pr edit`. Pushes branches via `git push`. |
| Side effects | Alters Git history (creates/fast-forwards `master`, rebases branches, force-pushes); edits workflow files and commits/pushes changes; updates GitHub Pages default branch; retargets PRs; flips default branch via API; deletes `main` when safe. |
| Outputs | Logs progress with `▶` prefix on stdout; warnings/errors to stderr; aborts with `ERROR:` prefix. |

### 2.4 `remove_github_packages.sh`
| Aspect | Details |
| --- | --- |
| Primary purpose | Delete untagged container image versions from GitHub Container Registry (GHCR) for a given owner/package. |
| Inputs & flags | Configuration provided via environment variables or in-script defaults: `GITHUB_OWNER`, `PACKAGE_NAME`, `OWNER_TYPE` (`user` or `org`), `GITHUB_PACKAGES_TOKEN` (exported as `TOKEN`). Optional `DRY_RUN` env flag (`1` = preview). |
| Environment variables | As above; requires GitHub Personal Access Token with `read:packages`, `write:packages`, `delete:packages`. |
| External dependencies | `curl`, `jq`. |
| Network/API usage | GitHub REST API `GET` on package versions endpoint; `DELETE` for untagged versions. |
| Side effects | Deletes GHCR image versions when `DRY_RUN` is not set to `1`; increments local counter for reporting. |
| Outputs | Logs deletions and final summary to stdout. API errors surface to stderr via `curl` exit behavior. |

## 3. Command Equivalence Plan
The new CLI (working name **`git-maintenance`**) will use Cobra for command/flag parsing. The root binary lives at `cmd/cli/main.go` and exposes the following hierarchy:

- `git-maintenance audit`
- `git-maintenance repo-folders-rename`
- `git-maintenance repo-remote-update`
- `git-maintenance repo-protocol-convert`
- `git-maintenance repo-prs-purge`
- `git-maintenance branch-default <target-branch>`
- `git-maintenance repo-packages-purge`

### 3.1 Flag and Behavior Mapping
The table below maps current script switches to Cobra equivalents and documents planned `gh` interactions.

| Script behavior | Cobra command | Flags & arguments | `gh` usage strategy |
| --- | --- | --- | --- |
| Remove untagged GHCR packages | `git-maintenance repo-packages-purge` | `--package` (optional override), `--dry-run`, `--page-size` (default 100). The command resolves the owner, owner type, and default package name from each repository's origin remote and requires a token with GitHub Packages scopes. Configurable via Viper with env prefix `GITMAINT`. | Prefer direct HTTP using `go-github` REST client authenticated with token. If we reuse `gh`, we would invoke `gh api` with `--method`. The design chooses native HTTP to avoid shelling out where OAuth token is already provided. |

### 3.2 Shared command behavior
- All `repo` subcommands support `--debug` to raise Zap logging level to `Debug`.
- `--yes` maps to `--confirm` boolean flag in Cobra (`--yes` alias) to allow scripted runs.
- Roots accept multiple entries via `--roots`; commands fall back to configured defaults when provided and otherwise return an error.
- The audit command adds `--all` to report top-level directories lacking Git repositories for each root, marking git-specific columns as `n/a`.
- Commands that mutate Git state will request clean worktrees when `--require-clean` is provided (rename) or by default when destructive (branch flip).
- Exit codes mirror existing scripts: non-zero on invalid flag combinations or fatal errors; continue processing across repositories when possible.

## 4. Module Path and Project Layout
### 4.1 Module path
Adopt the Go module path **`github.com/temirov/git-maintenance`**. This preserves the current GitHub namespace and describes the tool’s purpose clearly.

### 4.2 Directory structure
```
cmd/
  cli/
    main.go              # Cobra root command setup and Viper bootstrap
internal/
  repo/
    audit.go             # Audit scanning orchestration
    rename.go            # Rename operations
    remote.go            # Canonical remote updates and protocol conversions
    filesystem/
      mover.go           # Filesystem-safe rename helpers
    gitinfo/
      detection.go       # Local Git info gathering
    githubmeta/
      client.go          # Canonical metadata resolution via gh/api
  branches/
    cleanup.go           # Closed PR branch cleanup logic
  branchflip/
    flip.go              # Main→master migration workflow
    workflows.go         # Workflow retargeting helpers
    pages.go             # GitHub Pages adjustments
  packages/
    purge.go             # GHCR purge logic
  utils/
    exec.go              # Shared command execution helpers (shelling out to git/gh)
    concurrency.go       # Worker pools / goroutine utilities (if needed)
  config/
    loader.go            # Viper integration and configuration defaults
  constants/
    strings.go           # Centralized string constants & enums (command names, default values)

pkg/
  ghclient/
    client.go            # Optional reusable GitHub API wrapper when not tied to internal state

tests/
  integration/
    repo_audit_test.go   # Black-box CLI runs using fixture repos
    branchflip_test.go   # End-to-end branch migration tests
    packages_test.go     # GHCR purge dry-run tests (mock server)
```

Notes:
- Business logic resides in `internal/<domain>` packages, aligning with user guidance.
- Shared constants (command names, default config keys) sit in `internal/constants` to avoid string literals scattered across packages.
- Utilities that execute external commands (`git`, `gh`) are isolated in `internal/utils`, enabling mocking in tests.
- Domain packages expose structs with methods (e.g., `type AuditService struct { ... }`) per the preference for struct-based organization.

## 5. Non-Functional Requirements
### 5.1 Logging
- Use Uber’s Zap in production (`zap.NewProduction` baseline) with console encoding tuned for CLI readability.
- Provide `--debug` to switch to `zap.NewDevelopment` or dynamic level change.
- All domain services receive a structured logger via dependency injection; no package-level globals.

### 5.2 Configuration precedence (Viper)
- Viper initialized with prefix `GITMAINT`.
- Precedence: **command-line flags > environment variables > config file > defaults**.
- Config file search order: `./git-maintenance.yaml`, `$XDG_CONFIG_HOME/git-maintenance/config.yaml`, `$HOME/.config/git-maintenance/config.yaml`.
- Flags bind to Viper keys so environment/config seamlessly fill defaults.
- Sensitive values (tokens) read from env/flag only; we will not persist secrets to disk by default.

### 5.3 Error handling and UX
- Consistent error formatting via `fmt.Errorf` with `%w` for wrapping; Cobra `SilenceUsage` set after validation passes to avoid noisy usage output on runtime errors.
- All destructive operations require explicit confirmation flags or interactive prompts. Prompts use survey-style confirmers with defaults matching current scripts (`No`).

## 6. Testing Strategy
### 6.1 Unit tests
- Table-driven tests in `_test` packages (e.g., `repo_test`) residing outside implementation packages (`package repo_test`).
- Focus on behavior, not implementation details—tests operate via exported interfaces (e.g., `AuditService.Run`).
- Use fake adapters for Git/GitHub interactions (interfaces injected into services). No single-letter identifiers in tests.

### 6.2 Integration tests
Integration tests live in `tests/integration` and execute the compiled CLI binary against controlled fixtures.

| Feature area | Scenario | Git / GitHub setup | Expected assertions |
| --- | --- | --- | --- |
| Repo audit | Canonical rename detection | Local repo with simulated redirect via mocked `gh api` response | CSV output includes canonical name mismatch and `origin_matches_canonical=no`. |
| Repo audit | Protocol conversion dry-run | Repo using HTTPS remote | Command logs `PLAN-CONVERT` without modifying remote. |
| Repo rename | Dry-run and execute | Case-only rename on case-insensitive filesystem simulation | Dry-run prints `PLAN-CASE-ONLY`; execute performs two-step rename. |
| Remote update | Redirected repository | `origin` pointing to old owner; mocked `gh api` returns new owner | Remote URL updated; message `UPDATE-REMOTE-DONE`. |
| Branch cleanup | Closed PR branch removal | Temp repo with local+remote branches; stubbed `gh pr list` output | Command deletes matching branches, leaves others. |
| Branch flip | Workflow rewrite & Pages update | Fixture repo with workflows referencing `main`, GitHub Pages in legacy mode (mocked) | Workflows retargeted; API call made to update Pages; default branch switched; safety gates respected. |
| Branch flip | Safety gate triggered | Repo with open PR targeting `main` (mocked) | Command exits gracefully, logs skip for main deletion. |
| Packages purge | Dry-run | Mock GHCR API server returning tagged/untagged versions | Untagged IDs listed; no DELETE requests issued. |
| Packages purge | Deletion | Same as above with `--dry-run=false` | DELETE invoked for untagged IDs only; summary count matches. |

Integration harness responsibilities:
- Use temporary directories and `git init` to avoid mutating real repositories.
- Mock `gh` and GitHub APIs via httptest servers or fake executables placed earlier in `$PATH`.
- Provide OS matrix coverage for Linux and macOS via CI (GitHub Actions workflow matrix `ubuntu-latest`, `macos-latest`).

### 6.3 Tooling
- `go test ./...` for unit coverage.
- `golangci-lint` enforced via pre-commit or CI.
- Integration tests run as separate job (`go test ./tests/integration -tags=integration`).

## 7. Open Questions & Approval Checklist
Before implementation starts, please review and confirm:
1. Module path `github.com/temirov/git-maintenance` and directory layout meet expectations.
2. Command hierarchy and flag mapping provide the right developer experience.
3. Continued reliance on `gh` via subprocess (except GHCR purge, which uses native HTTP) is acceptable.
4. Logging (Zap), configuration precedence (Viper), and testing strategies align with requirements.
5. Integration scenarios capture the necessary coverage; suggest additions if specific edge cases are missing.

Once these points are approved, we will proceed with Cobra scaffolding and incremental porting of each script into Go services following this design.

## 8. Repository domain model and executor contracts (GX-403 – GX-406)

### 8.1 Smart constructors and invariants
Repository-facing services now consume domain types defined in `internal/repos/shared`. Each type rejects invalid input at construction time so executors and workflows operate on validated values only.

- `RepositoryPath` (`NewRepositoryPath`) normalises absolute paths and rejects newline characters.
- `OwnerSlug`, `RepositoryName`, and `OwnerRepository` enforce GitHub slug rules and canonicalise whitespace.
- `RemoteURL`, `RemoteName`, and `BranchName` guard against embedded whitespace and empty input.
- `RemoteProtocol` parses protocol identifiers (`git`, `ssh`, `https`, `other`) and exposes a `Validate` helper for stored values.

CLI commands, workflow operations, and dependency resolvers are responsible for constructing these types. Once constructed, services assume the invariants hold, matching the confident-programming policy.

### 8.2 Edge validation workflow
1. Cobra edges trim and validate flag/argument strings before building domain types.
2. Workflow task runners read repository inspection data, call `shared.Parse*Optional` helpers, and propagate typed values into executor `Options`.
3. Executors accept the domain structs and focus on orchestration (calls to Git, filesystem, confirmation prompts).
4. Tests cover both constructor success paths and error scenarios so new validation rules cannot regress silently.

Safeguards act as boolean gates evaluated before a task mutates a repository. A task (or command wrapper) may include a `safeguards` map with keys such as `require_clean: true`, `branch: master`, or `paths: ["go.mod"]`. The shared evaluator runs these checks using the injected collaborators (`RepositoryManager`, `FileSystem`, etc.); when a condition fails the repository is logged with a `*-SKIP` banner and the workflow advances to the next target without running the task body. Extending safeguard coverage only requires adding a handler to the evaluator, keeping individual operations free of bespoke guard logic.

### 8.3 Contextual error catalog
`internal/repos/errors` defines sentinel codes (for example `origin_owner_missing`, `remote_update_failed`, `history_rewrite_failed`) and wraps them in `OperationError`. Executors use `errors.Wrap`/`errors.WrapMessage` to attach:

- the operation identifier (`repo.protocol.convert`, `repo.remote.update`, `repo.folder.rename`, `repo.history.purge`);
- the repository path subject; and
- the human-readable message emitted through the shared reporter.

Callers can inspect `OperationError.Code()` to branch on behaviour, while CLI layers render the formatted text (e.g. `ERROR: failed to set origin`). This catalog is the single source of truth for automation hooks and integration tests.

### 8.4 Prompting and structured output
Executors share two cross-cutting utilities from `internal/repos/shared`:

- `ConfirmationPolicy` expresses whether to prompt (`ConfirmationPrompt`) or auto-accept (`ConfirmationAssumeYes`). Workflow edges enable `assume_yes` by flipping this policy; CLI surfaces map `--yes` to the same behaviour.
- `Reporter` is a tiny interface that writes plan, skip, and success banners (`PLAN-CONVERT`, `UPDATE-REMOTE-DONE`, `CONVERT-SKIP`, etc.) to an `io.Writer`. Both CLI commands and workflow runners pass a writer backed by `cmd.OutOrStdout()` so tests can assert against deterministic strings.

Prompts always expose the same template (`Convert 'origin' in '<path>' (https → ssh)? [a/N/y] `) and record “apply to all” selections in the shared `ConfirmationResult`. Declines print a `*-SKIP` banner; approvals continue with the executor flow.
