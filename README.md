# gix, a Git/GitHub helper CLI

[![GitHub release](https://img.shields.io/github/release/temirov/gix.svg)](https://github.com/temirov/gix/releases)

gix keeps large fleets of Git repositories in a healthy state. It bundles the day-to-day tasks every maintainer repeats: normalising folder names, aligning remotes, pruning stale branches, scrubbing GHCR images, and shipping consistent release notes.

## Highlights

- Run trusted maintenance commands across many repositories from one terminal session.
- Preview every action with `--dry-run` before touching remotes or the filesystem.
- Reuse discovery, prompting, and logging whether you call a single command or an entire workflow file.
- Lean on AI-assisted helpers for commit messages and changelog summaries when you want them.

## Quick Start

1. Install the CLI: `go install github.com/temirov/gix@latest` (Go 1.24+).
2. Explore the available commands: `gix --help`.
3. Bootstrap defaults in your workspace: `gix --init LOCAL` (or `gix --init user` for a per-user config).
4. Run a dry-run audit to confirm your environment: `gix audit --roots ~/Development --dry-run`.

## Everyday workflows

### Keep local folders canonical

```shell
gix repo folder rename --roots ~/Development --yes
```

Automatically rename each repository directory so it matches the canonical GitHub name.

### Ensure remotes point to the canonical URL

```shell
gix repo remote update-to-canonical --roots ~/Development --dry-run
```

Preview and apply remote URL fixes across every repository under one or more roots.

### Convert remote protocols in bulk

```shell
gix repo remote update-protocol --from https --to ssh --roots ~/Development --yes
```

Switch entire directory trees over to the protocol that matches your credential strategy.

### Prune branches that already merged

```shell
gix repo prs delete --roots ~/Development --limit 100
```

Delete local and remote branches whose pull requests are already closed.

### Clear out stale GHCR images

```shell
gix repo packages delete --roots ~/Development/containers --yes
```

Remove untagged GitHub Container Registry versions in one sweep.

### Generate audit CSVs for reporting

```shell
gix audit --roots ~/Development --all > audit.csv
```

Capture metadata (default branches, owners, remotes, protocol mismatches) for every repository in scope.

### Draft commit messages and changelog entries

```shell
gix branch commit message --roots .
gix repo changelog message --since-tag v1.2.0 --version v1.3.0
```

Use the reusable LLM client to summarise staged changes or recent history.

## Automate sequences with workflows

When you need several operations in one pass, describe them in YAML or JSON and execute them with the workflow runner:

```shell
gix workflow maintenance.yml --roots ~/Development --yes
```

Workflows reuse repository discovery, confirmation prompts, and logging so you can hand teammates a repeatable playbook.

## Shared command options

- `--roots <path>` — target one or more directories; nested repositories are ignored automatically.
- `--dry-run` — print the proposed actions without mutating anything.
- `--yes` (`-y`) — accept confirmations when you are ready to apply the plan.
- `--config path/to/config.yaml` — load persisted defaults for flags such as roots, owners, or log level.
- `--log-level`, `--log-format` — control Zap logging output (structured JSON or console).

Additional shared flags:

- `--remote <name>` — override the remote name used by commands that push or fetch (default `origin`).
- `--version` — print the gix version (works at the root or with any command).
- `--init [local|user] [--force]` — write an embedded default config (to `./config.yaml` or `$XDG_CONFIG_HOME/gix/config.yaml`), overwriting when `--force` is provided.

## Command Reference

Top-level commands and their subcommands. Aliases are shown in parentheses.

- `gix version`
  - Prints the current release. Also available as `gix --version`.

- `gix audit [--roots <dir>...] [--all] [--dry-run] [-y]` (alias `a`)
  - Flags: `--roots` (repeatable), `--all` to include non-git folders in output.

- `gix workflow <configuration> [--roots <dir>...] [--require-clean] [--dry-run] [-y]` (alias `w`)
  - Runs tasks from a YAML/JSON workflow file.
  - Flags: `--require-clean` sets the default safeguard for operations that support it.

- `gix repo` (alias `r`)
  - `gix repo folder rename [--owner] [--require-clean] [--roots <dir>...] [--dry-run] [-y]`
    - Renames repository directories to canonical GitHub names.
    - Flags: `--owner` include the owner in directory path; `--require-clean` enforce clean worktrees.
  - `gix repo remote update-to-canonical [--owner <slug>] [--roots <dir>...] [--dry-run] [-y]` (alias `canonical`)
    - Updates `origin` URLs to the canonical GitHub repository; optional `--owner` constraint.
  - `gix repo remote update-protocol --from <git|ssh|https> --to <git|ssh|https> [--roots <dir>...] [--dry-run] [-y]` (alias `convert`)
    - Converts remote protocols in bulk.
  - `gix repo prs delete [--limit <N>] [--remote <name>] [--roots <dir>...] [--dry-run] [-y]` (alias `purge`)
    - Deletes branches whose pull requests are closed. Flags: `--limit`, `--remote`.
  - `gix repo packages delete [--package <name>] [--roots <dir>...] [--dry-run] [-y]` (alias `prune`)
    - Removes untagged GHCR versions. Flag: `--package` for the container name.
  - `gix repo files replace --find <string> [--replace <string>] [--pattern <glob>...] [--command "<shell>"] [--require-clean] [--branch <name>] [--require-path <rel>...] [--roots <dir>...] [--dry-run] [-y]` (alias `sub`)
    - Performs text substitutions across matched files. Safeguards via `--require-clean`, `--branch`, `--require-path`.
  - `gix repo namespace rewrite --old <module/prefix> --new <module/prefix> [--branch-prefix <prefix>] [--remote <name>] [--push] [--commit-message <text>] [--roots <dir>...] [--dry-run] [-y]` (alias `ns`)
    - Rewrites Go module namespaces and imports.
  - `gix repo rm <path>... [--remote <name>] [--push] [--restore] [--push-missing] [--roots <dir>...] [--dry-run] [-y]` (alias `purge`)
    - Purges paths from history using git-filter-repo and optionally force-pushes updates.
  - `gix repo release <tag> [--message <text>] [--remote <name>] [--roots <dir>...] [--dry-run] [-y]` (alias `rel`)
    - Creates and pushes an annotated tag for each repository root.
  - `gix repo changelog message [--version <v>] [--release-date YYYY-MM-DD] [--since-tag <ref>] [--since-date <ts>] [--max-tokens <N>] [--temperature <0-2>] [--model <id>] [--base-url <url>] [--api-key-env <NAME>] [--timeout-seconds <N>] [--roots <dir>...]` (aliases `section`, `msg`)
    - Generates a changelog section from git history using the configured LLM.

- `gix branch` (alias `b`)
  - `gix branch default <target-branch> [--roots <dir>...] [--dry-run] [-y]`
    - Promotes the default branch across repositories.
  - `gix branch cd <branch> [--remote <name>] [--roots <dir>...] [--dry-run]` (alias `switch`)
    - Switches repositories to the selected branch, creating it if missing and rebasing onto the remote.
  - `gix branch refresh --branch <name> [--stash | --commit] [--roots <dir>...]`
    - Fetches, checks out, and pulls a branch; optionally stashes or commits local changes.
  - `gix branch commit message [--diff-source staged|worktree] [--max-tokens <N>] [--temperature <0-2>] [--model <id>] [--base-url <url>] [--api-key-env <NAME>] [--timeout-seconds <N>] [--roots <dir>...]` (alias `msg`)
    - Drafts a Conventional Commit subject and optional bullets using the configured LLM.

## Configuration essentials

- `gix --init LOCAL` writes an embeddable starter `config.yaml` to the current directory; `gix --init user` places it under `$XDG_CONFIG_HOME/gix` or `$HOME/.gix`.
- Configuration precedence is: CLI flags → environment variables prefixed with `GIX_` → local config → user config.
- Default settings include log level, log format, dry-run behaviour, confirmation prompts, and reusable workflow definitions.

## Need more depth?

- Detailed architecture, package layout, and command wiring: [ARCHITECTURE.md](ARCHITECTURE.md)
- Historical roadmap and design notes: [docs/cli_design.md](docs/cli_design.md)
- Recent changes: [CHANGELOG.md](CHANGELOG.md)

## Developer notes

- Repository services accept domain types from `internal/repos/shared` (paths, owners, remotes, branches); CLI edges construct them so executors run without defensive validation.
- Executor errors surface via the contextual catalog in `internal/repos/errors`, which prints `PLAN-*`, `*-DONE`, and `*-SKIP` banners through the shared reporter.
- Confirmation prompts respect the `[a/N/y]` contract everywhere; passing `--yes` (or setting `assume_yes: true` in workflows) flips the shared confirmation policy to auto-accept.
- Run `make ci` before submitting patches; it enforces formatting plus `go vet`, `staticcheck`, `ineffassign`, and the unit/integration test suites.
