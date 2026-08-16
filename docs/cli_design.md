# Historical Git Maintenance CLI Transition Design

> **Historical record — not a current contract.** This document records the pre-release Bash-to-Go transition and earlier design choices. It may describe commands, configuration, and dependencies that no longer exist. Use [README.md](../README.md) for the current operator contract, [ARCHITECTURE.md](../ARCHITECTURE.md) for the current design, and [`.mprlab/ISSUES.md`](../.mprlab/ISSUES.md) for active work.

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
| Inputs & flags | Positional scan roots (defaults to `.`). Flags: `--rename`, `--update-remote`, `--protocol-from {https\|git\|ssh}`, `--protocol-to {https\|git\|ssh}`, ``, `--yes` (`-y`), `--require-clean`, `--debug`. |
| Environment variables | `GIT_TERMINAL_PROMPT=0` (set within script to disable interactive credential prompts). |
| External dependencies | `git`, `gh`, `jq`, `find`, `readlink`/`realpath`, `mv`, `sed`, `awk`, standard GNU coreutils. Requires authenticated `gh` session. |
| Network/API usage | `gh api` to resolve repository canonical metadata; `gh repo view` to determine default branch; optional `git fetch` for sync detection; `git remote set-url` for updates. |
| Side effects | File-system renames of repository directories; remote URL changes; Git fetches; optional prompts; stdout/stderr logging. With ``, operations are read-only. |
| Outputs | The dedicated audit command emits a terminal table by default. `--format csv` emits a header plus one line per repository, and `--format html` emits a standalone HTML report. Operational modes emit plan/action messages (`PLAN-OK`, `UPDATE-REMOTE-DONE`, etc.) to stdout and error messages to stderr. Debug logging when `--debug` is set. |

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

### 2.3 `gix default <target-branch>`
| Aspect | Details |
| --- | --- |
| Primary purpose | Promote an explicit branch to the repository default branch. Update workflows, GitHub Pages, pull-request targets, local branches, and remote settings. |
| Inputs & flags | Required target branch as a positional argument plus the shared command flags. The command detects the current default branch from repository data. |
| Environment variables | Relies on Git configuration for remote URLs and GitHub CLI authentication context. |
| External dependencies | `git`, `gh`, `jq`, `sed`, `find`, standard GNU coreutils. Requires authenticated `gh` session. |
| Network/API usage | Extensive `gh api` usage: fetch repository metadata, GitHub Pages config, branch protection checks, open PRs. Uses `gh pr list`, `gh pr close`, and `gh pr edit`. Pushes branches via `git push`. |
| Side effects | Creates or updates the target branch, edits workflow files, updates GitHub Pages, and changes the default branch. Closes a pull request only when its head repository and head branch match the target repository and branch. Changes the base of other pull requests. It can delete the old default branch when the operator selects that option. |
| Outputs | Logs the promoted source and target branches and reports whether the source branch is safe to delete. |

### 2.4 `gix packages delete`
| Aspect | Details |
| --- | --- |
| Primary purpose | Preserve an explicit number of newest container image versions and delete every older version from GitHub Container Registry (GHCR) for a given owner/package. |
| Inputs & flags | Required positive `--keep <count>`; optional `--package <name>` override; repository roots, API endpoint, and concrete credential come from the `packages delete` operation. |
| Environment variables | Configuration placeholders may interpolate a GitHub token already present in the process environment. The resolved token requires package read and delete access. |
| External dependencies | Git for repository discovery and GitHub metadata resolution. GHCR requests use the native HTTP client. |
| Network/API usage | GitHub REST API `GET` on every package versions page, followed by `DELETE` for versions older than the retained set. |
| Side effects | Deletes tagged and untagged GHCR image versions outside the retained set. |
| Outputs | Logs retention progress and final total, retained, and deleted counts. API errors stop the affected repository with a non-zero result. |

## 3. Command Equivalence Plan
The CLI (released as **`gix`**) uses Cobra for command/flag parsing. The root binary lives at `cmd/cli/main.go` and exposes the following hierarchy:

- `gix audit`
- `gix repo-folders-rename`
- `gix repo-remote-update`
- `gix repo-protocol-convert`
- `gix repo-prs-purge`
- `gix default <target-branch>`
- `gix packages delete --keep <count>`

### 3.1 Flag and Behavior Mapping
The table below maps current script switches to Cobra equivalents and documents planned `gh` interactions.

| Script behavior | Cobra command | Flags & arguments | `gh` usage strategy |
| --- | --- | --- | --- |
| Retain newest GHCR versions | `gix packages delete` | Required positive `--keep` selects the newest version count; `--package` optionally overrides the package name. The command resolves owner metadata from each repository and reads its GitHub API endpoint and interpolated credential from the `packages delete` operation in `config.yml`. | Snapshot and validate all version pages through the native GHCR HTTP client, order versions by `created_at` and version ID, then delete every older tagged or untagged version. |

### 3.2 Shared command behavior
- All `repo` subcommands support `--debug` to raise Zap logging level to `Debug`.
- `--yes` maps to `--confirm` boolean flag in Cobra (`--yes` alias) to allow scripted runs.
- Roots accept multiple entries via `--roots`; commands fall back to configured defaults when provided and otherwise return an error.
- The audit command adds `--all` to report top-level directories lacking Git repositories for each root, marking git-specific columns as `n/a`.
- Commands that mutate Git state will request clean worktrees when `--require-clean` is provided (rename) or by default when destructive (branch flip).
- Exit codes mirror existing scripts: non-zero on invalid flag combinations or fatal errors; continue processing across repositories when possible.

## 4. Module Path and Project Layout
### 4.1 Module path
Adopt the Go module path **`github.com/tyemirov/gix`** so it matches the published repository and binary name.

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
    packages_test.go     # GHCR retention tests (mock server)
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

### 5.2 Configuration contract
- Decode exactly one `config.yml` using the canonical schema.
- Config file search order: explicit `--config`, `/etc/gix/config.yml`, `$HOME/.gix/config.yml`.
- Offer to create the user file when neither discovered file exists.
- Expand `${NAME}` only inside the selected file and only from the inherited process environment. Never discover or parse `.env` files; preserve literal configuration values unchanged.
- Keep complete `openai` and `llm_proxy` profiles under the top-level `llm` block. Each owns a positive unique priority, endpoint, credential, and its routing fields: `openai.model` or `llm_proxy.provider` plus `llm_proxy.model`.
- Attempt credentialed profiles in ascending priority order, continue after request failure, and return the first successful response. Require at least one active credential.
- Flags may override `llm_proxy.provider` and `llm_proxy.model` for one invocation but may not replace endpoints, credentials, or connection priorities.
- Pass GitHub and GHCR credentials as concrete decoded values; runtime clients do not resolve environment-variable names.

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
| Repo audit | Canonical rename detection | Local repo with simulated redirect via mocked `gh api` response | Default table and explicit CSV/HTML exports include the canonical name mismatch and `origin_matches_canonical=no`. |
| Repo audit | Protocol conversion | Repo using HTTPS remote | Command logs `PLAN-CONVERT` without modifying remote. |
| Repo rename |  and execute | Case-only rename on case-insensitive filesystem simulation |  prints `PLAN-CASE-ONLY`; execute performs two-step rename. |
| Remote update | Redirected repository | `origin` pointing to old owner; mocked `gh api` returns new owner | Remote URL updated; message `UPDATE-REMOTE-DONE`. |
| Branch cleanup | Closed PR branch removal | Temp repo with local+remote branches; stubbed `gh pr list` output | Command deletes matching branches, leaves others. |
| Branch flip | Workflow rewrite & Pages update | Fixture repo with workflows referencing `main`, GitHub Pages in legacy mode (mocked) | Workflows retargeted; API call made to update Pages; default branch switched; safety gates respected. |
| Branch flip | Safety gate triggered | Repo with open PR targeting `main` (mocked) | Command exits gracefully, logs skip for main deletion. |
| Packages retention | Missing or non-positive `--keep` | Mock GHCR API server | Command fails before any list or delete request. |
| Packages retention | `--keep 3` with five mixed tagged/untagged versions | Mock GHCR API server returning complete version metadata | Three newest IDs are preserved and both older IDs are deleted regardless of tags. |

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
1. Module path `github.com/tyemirov/gix` and directory layout meet expectations.
2. Command hierarchy and flag mapping provide the right developer experience.
3. Continued reliance on `gh` via subprocess (except GHCR retention, which uses native HTTP) is acceptable.
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

Safeguards act as boolean gates evaluated before a task mutates a repository. A task (or command wrapper) specifies `safeguards` as two optional maps—`hard_stop` and `soft_skip`. Each map accepts keys such as `require_clean: true`, `branch: master`, or `paths: ["go.mod"]`. The shared evaluator runs these checks using the injected collaborators (`RepositoryManager`, `FileSystem`, etc.). Hard-stop failures abort the repository immediately, while soft-skip failures simply mark the current task/action as skipped so later steps can continue. Extending safeguard coverage only requires adding a handler to the evaluator, keeping individual operations free of bespoke guard logic.

### 8.3 Contextual error catalog
`internal/repos/errors` defines sentinel codes (for example `origin_owner_missing`, `remote_update_failed`, `history_rewrite_failed`) and wraps them in `OperationError`. Executors use `errors.Wrap`/`errors.WrapMessage` to attach:

- the operation identifier (`repo.protocol.convert`, `repo.remote.update`, `repo.folder.rename`, `repo.history.purge`);
- the repository path subject; and
- the human-readable message emitted through the shared reporter.

Callers can inspect `OperationError.Code()` to branch on behaviour, while CLI layers render the formatted text (e.g. `ERROR: failed to set origin`). This catalog is the single source of truth for automation hooks and integration tests.

### 8.4 Prompting and structured output
Executors share two cross-cutting utilities from `internal/repos/shared`:

- `ConfirmationPolicy` expresses whether to prompt (`ConfirmationPrompt`) or auto-accept (`ConfirmationAssumeYes`). Workflow edges enable `assume_yes` by flipping this policy; CLI surfaces map `--yes` to the same behaviour, and selecting `a`/`all` at one prompt upgrades the shared policy for the remainder of the run.
- `Reporter` is a tiny interface that writes plan, skip, and success banners (`PLAN-CONVERT`, `UPDATE-REMOTE-DONE`, `CONVERT-SKIP`, etc.) to an `io.Writer`. Both CLI commands and workflow runners pass a writer backed by `cmd.OutOrStdout()` so tests can assert against deterministic strings.

Prompts always expose the same template (`Convert 'origin' in '<path>' (https → ssh)? [a/N/y] `), with uppercase `N` signalling the default decline. They record “apply to all” selections in the shared `ConfirmationResult`, and propagate that choice into the shared confirmation policy so subsequent prompts auto-accept. Declines print a `*-SKIP` banner; approvals continue with the executor flow.
