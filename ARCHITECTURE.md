# gix Architecture

## Overview

gix is a Go 1.24 command-line application built with Cobra and Viper. The binary exposed by `main.go` delegates all setup to `cmd/cli`, which wires logging, configuration, and command registration before executing user-facing operations. Domain logic lives in `internal` packages, each focused on a cohesive maintenance capability. Shared libraries that may be reused by external programs are published under `pkg/`.

```
.
├── main.go          # binary entrypoint
├── cmd/cli          # Cobra application, command registration, configuration bootstrap
├── internal         # feature domains (audit, repos, branches, etc.)
├── pkg              # reusable libraries (currently LLM automation)
├── docs             # design notes and developer references
└── tests            # behavior-driven integration tests
```

## Execution Flow

1. The binary entrypoint (`main.go`) invokes `cli.Execute`, which builds the Cobra root command inside `cmd/cli/application.go`.
2. `cmd/cli` initialises Viper, loads configuration files via `internal/utils/flags`, and prepares a structured Zap logger.
3. Each namespace (`audit`, `repo`, `branch`, `commit`, `workflow`, etc.) registers subcommands that accept shared flags (`--roots`, ``, `--yes`) before delegating to domain services.
4. Domain services resolve their collaborators through `internal/repos/dependencies`, which supplies defaults for repository discovery, filesystem access, Git execution, and GitHub metadata unless tests inject fakes.
5. Commands perform work through `internal/...` packages (for example, `internal/repos/rename.Run`), returning contextual errors that bubble back to Cobra for consistent exit handling.

## Command Surface

The Cobra application (`cmd/cli/application.go`) initialises the root command and nests feature namespaces below it (`audit`, `repo`, `branch`, `commit`, `workflow`, and others). Each namespace hosts subcommands that ultimately depend on injected services from `internal/...` packages. Commands share common flag parsing helpers (`internal/utils/flags`) and prompt utilities.

- `cmd/cli/repos` registers multi-command groups such as `folder rename`, `remote update-to-canonical`, `prs delete`, and `files replace`.
- `cmd/cli/repos/release` contains the `release` tagging workflow.
- `cmd/cli/changelog`, `cmd/cli/commit`, and `cmd/cli/workflow` expose focused entrypoints for changelog generation, AI-assisted commit messaging, and workflow execution.
- `cmd/cli/default_configuration.go` houses the embedded default YAML used by the `gix --init` flag.

All commands accept shared flags for log level, log format, previews, repository roots, and confirmation prompts. Validation occurs in Cobra `PreRunE` functions, aligning with the confident-programming rules in `POLICY.md`.

## Domain Packages

Each feature area resides in `internal/<domain>` and exposes structs with methods instead of package-level functions. The primary packages are:

- `internal/audit`: Repository discovery, metadata reconciliation, CSV export, and CLI integration (`internal/audit/cli`).
- `internal/branches`: Branch maintenance commands (`cd`, `refresh`, default promotion) and supporting adapters.
- `internal/changelog`, `internal/commitmsg`: Generators that transform Git history and staged changes into formatted text.
- `internal/repos`: Subpackages for repository workflows:
  - `dependencies`: Dependency resolution for discovery, filesystem, Git, and GitHub integrations.
  - `discovery`: Filesystem scanning for Git repositories.
  - `filesystem`: Filesystem abstractions used by rename/history flows.
  - `history`: Wrapper around git-filter-repo operations for `rm`.
  - `prompt`: End-user confirmation and message formatting.
  - `protocol`, `remotes`, `rename`: Operations that update remotes, protocols, and directory names.
  - `shared`: Shared interfaces (Git executor, GitHub resolver, repository manager).
- `internal/packages`: GitHub Packages purge workflow including GHCR API clients.
- `internal/releases`: Annotated tag creation and push orchestration used by `release`.
- `internal/workflow`: YAML/JSON workflow runner, step registry, and execution environment.
- `internal/execshell`, `internal/gitrepo`, `internal/githubcli`: Adapters for running Git commands, interacting with repositories, and resolving metadata through the GitHub CLI.
- `internal/utils`: Logging factories, command flag helpers, filesystem path utilities, and repository root deduplication.
- `internal/ghcr`, `internal/version`, `internal/migrate`: Specialized helpers for GHCR interactions, version embedding, and repository migration flows.

External integrations (for example, Git/GitHub shells and GHCR APIs) are isolated behind interfaces, enabling injection of fakes or mocks in tests.

## Workflow Runner and Step Registration

The workflow command consumes declarative YAML or JSON plans describing ordered actions. `internal/workflow` resolves steps into concrete executors registered through `internal/repos/dependencies` and other domain services. Discovery of repositories, confirmation prompts, and logging contexts are reused across steps to minimise duplicate code.

- Workflow steps call domain executors such as `folder rename`, `remote update-protocol`, `tasks apply`, and audit report generation.
- Additional utilities (for example, template rendering or safeguards) live alongside the executors so they can be reused across CLI and workflow entrypoints.
Each workflow step enforces previews and respects the global confirmation strategy. Discovery and prompting are shared with direct CLI invocations so adopters can migrate between ad-hoc and scripted automation without rewriting plumbing.

## Configuration and Logging

Configuration is managed by Viper with an uppercase `GIX` environment prefix. The search order is:

1. Explicit `--config` path, if provided.
2. `config.yaml` in the working directory.
3. `$XDG_CONFIG_HOME/gix/config.yaml`.
4. `$HOME/.gix/config.yaml`.

`gix --init` bootstraps either a local `./config.yaml` or a user-level configuration directory when invoked with `--init LOCAL` (default) or `--init user`. Logging relies on Uber's Zap; structured JSON is the default, and console mode is available through a flag or configuration.

## Workflow configuration example

The example below matches the configuration used in the documentation tests. It demonstrates how CLI defaults and workflow steps can share anchored maps so one file drives both direct commands and declarative workflows.

```yaml
# config.yaml
common:
  log_level: error
  log_format: structured

operations:
  - command: ["audit"]
    with: &audit_defaults
      roots:
        - ~/Development
      debug: false

  - command: ["packages", "delete"]
    with: &packages_purge_defaults
      # package: my-image  # Optional override; defaults to the repository name
      roots:
        - ~/Development

  - command: ["prs", "delete"]
    with: &branch_cleanup_defaults
      remote: origin
      limit: 100
      roots:
        - ~/Development

  - command: ["remote", "update-to-canonical"]
    with: &repo_remotes_defaults
      assume_yes: true
      owner: canonical
      roots:
        - ~/Development

  - command: ["remote", "update-protocol"]
    with: &repo_protocol_defaults
      assume_yes: true
      roots:
        - ~/Development
      from: https
      to: git

  - command: ["folder", "rename"]
    with: &repo_rename_defaults
      assume_yes: true
      require_clean: true
      include_owner: false
      roots:
        - ~/Development

  - command: ["workflow"]
    with: &workflow_command_defaults
      roots:
        - ~/Development
      assume_yes: false

  - command: ["branch-default"]
    with: &branch_default_defaults
      debug: false
      roots:
        - ~/Development

workflow:
  - step:
      order: 1
      command: ["remote", "update-protocol"]
      with:
        <<: *repo_protocol_defaults

  - step:
      order: 2
      command: ["remote", "update-to-canonical"]
      with:
        <<: *repo_remotes_defaults

  - step:
      order: 3
      command: ["folder", "rename"]
      with:
        <<: *repo_rename_defaults

  - step:
      order: 4
      command: ["branch-default"]
      with:
        <<: *branch_default_defaults
        targets:
          - remote_name: origin
            target_branch: master
            push_to_remote: true
            delete_source_branch: false

  - step:
      order: 5
      command: ["audit", "report"]
      with:
        output: ./reports/audit-latest.csv
```

## Reusable Packages

`pkg/llm` contains the reusable client abstractions for LLM-backed features such as commit message and changelog generators. The package exposes an interface-based design so that other programs can reuse the same client without duplicating API plumbing.

## Testing Strategy

Domain packages rely on table-driven unit tests using injected fakes for Git, GitHub, and filesystem interactions. Integration coverage lives under `tests/`, where high-level flows execute through the public CLI surfaces to ensure behavior matches the documented commands. All tests are designed to run in isolated temporary directories (`t.TempDir`) without polluting the developer filesystem.

Documentation tests in `docs/readme_config_test.go` ensure the workflow configuration referenced above stays in sync with the executable configuration loader.
