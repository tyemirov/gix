# gix, a Git/GitHub helper CLI

[![GitHub release](https://img.shields.io/github/release/tyemirov/gix.svg)](https://github.com/tyemirov/gix/releases)

gix keeps large fleets of Git repositories in a healthy state. It bundles the day-to-day tasks every maintainer repeats: normalising folder names, aligning remotes, pruning stale branches, scrubbing GHCR images, and shipping consistent release notes.

## Highlights

- Run trusted maintenance commands across many repositories from one terminal session.
- Reuse discovery, prompting, and logging whether you call a single command or an entire workflow file.
- Lean on AI-assisted helpers for commit messages and changelog summaries when you want them.

## Quick Start

1. Install the CLI: `go install github.com/tyemirov/gix@latest` (Go 1.25+).
2. Explore the available commands: `gix --help`.
3. Bootstrap defaults in your workspace: `gix --init LOCAL` (or `gix --init user` for a per-user config).
4. Run an audit to confirm your environment: `gix audit --roots ~/Development`.

## Everyday workflows

### Keep local folders canonical

```shell
gix folder rename --roots ~/Development --yes
```

Automatically rename each repository directory so it matches the canonical GitHub name.

### Ensure remotes point to the canonical URL

```shell
gix remote update-to-canonical --roots ~/Development
```

Preview and apply remote URL fixes across every repository under one or more roots.

### Convert remote protocols in bulk

```shell
gix remote update-protocol --from https --to ssh --roots ~/Development --yes
```

Switch entire directory trees over to the protocol that matches your credential strategy.

### Prune branches that already merged

```shell
gix prs delete --roots ~/Development --limit 100
```

Delete local and remote branches whose pull requests are already closed.

### Clear out stale GHCR images

```shell
gix packages delete --roots ~/Development/containers --yes
```

Remove untagged GitHub Container Registry versions in one sweep.

### Generate audit CSVs for reporting

```shell
gix audit --roots ~/Development --all > audit.csv
```

Capture metadata (default branches, owners, remotes, protocol mismatches) for every repository in scope.

### Draft commit messages and changelog entries

```shell
gix message commit --roots .
gix message changelog --since-tag v1.2.0 --version v1.3.0
```

Use the reusable LLM client (`github.com/tyemirov/utils/llm`) to summarise staged changes or recent history.

## Automate sequences with workflows

When you need several operations in one pass, describe them in YAML or JSON and execute them with the workflow runner:

```shell
gix workflow maintenance.yml --roots ~/Development --yes
```

Workflows reuse repository discovery, confirmation prompts, and logging so you can hand teammates a repeatable playbook.

### Workflow output

`gix workflow` emits YAML step summaries (one per repository) and prints a final summary line at the end of the run. Other commands keep the existing human-readable console logs.

### Embedded workflows

In addition to external YAML/JSON files, you can run bundled presets:

```shell
gix workflow --list-presets
gix workflow license --roots ~/Development --yes
gix workflow folder-rename --var folder_require_clean=true --var folder_include_owner=false --roots ~/Development --yes
gix workflow remote-update-to-canonical --var owner=canonical --roots ~/Development --yes
```

Embedded workflows ship with the binary so you can hand teammates a stable command (for example, `license`, `namespace`, `folder-rename`, `remote-update-to-canonical`, `remote-update-protocol`, `history-remove`, `files-add`, `files-replace`, `release-tag`, or `release-retag`) without distributing a separate configuration file.

### Atomic git helpers

Workflows can now compose individual git/file operations as standalone steps:

- `tasks apply` with `steps: ["files.apply"]` — perform only the file mutation stage (no automatic stage/commit/push); add `safeguards.soft_skip.paths` to insist the file already exists.
- `git stage-commit` — run `git add` for templated paths and immediately commit with a templated message (optionally `allow_empty`).
- `git push` — push a templated branch to a templated remote with remote validation (useful when you truly need a push without a PR).
- `pull-request open` — push (warning when no remote) and open a PR in one step, using templated title/body/base/head values.
- `pull-request create` — open a PR without touching remotes (legacy behavior).
- Every workflow automatically exposes `.Environment.workflow_run_id` (UTC `YYYYMMDDHHMMSS`) so you can build unique branch names like `automation/{{ .Repository.Name }}-{{ index .Environment "workflow_run_id" }}` without passing extra variables.

Combine these steps to build fully custom git flows without relying on one monolithic `tasks apply`. See `configs/gitignore.yaml` for a concrete example that splits branch creation, file editing, staging, commit, push, and PR creation into discrete workflow steps.

### Workflow variables

Use runtime variables to parameterize presets or external configs:

```shell
gix workflow license --var template=apache --var branch=chore/license --roots ~/Development --yes
gix workflow namespace --var namespace_old=github.com/old/org --var namespace_new=github.com/new/org --roots ~/Research
```

- `--var key=value` sets a single variable (repeat the flag for multiple values).
- `--var-file path/to/file.yaml` loads variables from a YAML/JSON map.

Variables appear inside task templates via `{{ index .Environment "key" }}` and merge with captured values (`capture_as`), with runtime inputs taking precedence.

#### License preset variables

`gix workflow license` recognizes the following keys (pass via `--var` or `--var-file`):

| Variable | Description |
| --- | --- |
| `license_content` | Required license text (inline or loaded from `--var template=...`). |
| `license_target` | Relative path for the output file (defaults to `LICENSE`). |
| `license_mode` | File handling mode (`overwrite`, `skip-if-exists`, or `append-if-missing`). |
| `license_branch` | Branch name template for the license changes. |
| `license_start_point` | Start point for the license branch (defaults to the repository default). |
| `license_remote` | Remote used for pushes (defaults to `origin`). |
| `license_commit_message` | Commit message template. |

#### Namespace preset variables

`gix workflow namespace` recognizes the following keys:

| Variable | Description |
| --- | --- |
| `namespace_old` | Required old module prefix (e.g., `github.com/old/org`). |
| `namespace_new` | Required new module prefix (e.g., `github.com/new/org`). |
| `namespace_branch_prefix` | Optional branch prefix for rewrite branches (defaults to `namespace-rewrite`). |
| `namespace_remote` | Optional push remote (defaults to `origin` when pushing). |
| `namespace_push` | Optional boolean (`true`/`false`) controlling whether rewritten branches push. Defaults to `true`. |
| `namespace_commit_message` | Optional commit message template for the rewrite commit. |

Use the `gix workflow license` preset (with `--var template=...`, `--var license_branch=...`, etc.) to distribute license content; the old `gix repo-license-apply` wrapper has been removed.

### Workflow syntax

Workflows are YAML or JSON files with a top-level `workflow` sequence. Each entry wraps a `step` describing one command path, optional dependencies, and command-specific options.

```yaml
workflow:
 - step:
   name: rename
   command: ["folder", "rename"]
   with:
    require_clean: true
    include_owner: false

 - step:
   name: remotes
   after: ["rename"]
   command: ["remote", "update-to-canonical"]
   with:
    owner: tyemirov

 - step:
   name: protocols
   after: ["remotes"]
   command: ["remote", "update-protocol"]
   with:
    from: https
    to: ssh

 - step:
   name: default-branch
   command: ["default"]
   with:
    targets:
     - remote_name: origin
      # if omitted, source_branch is discovered from remote or local
      target_branch: master
      push_to_remote: true
      delete_source_branch: false

 - step:
   name: audit
   after: ["default-branch"]
   command: ["audit", "report"]
   with:
    output: ./reports/audit.csv
```

- `name` is optional; if omitted a stable name is generated (e.g., `convert-protocol-1`).
- `after` lists step names this step depends on. If omitted, each step depends on the previous step, preserving sequential order.
- `command` selects a built-in workflow command path (see below).
- `with` carries command-specific options.

Run with: `gix workflow path/to/file.yaml --roots ~/Development [-y] [--require-clean]`.

- Repositories run sequentially so each workflow prints as a contiguous block per repo. Pass `--workflow-workers <N>` (or set `workflow_workers`) to allow the orchestrator to process up to `N` repositories in parallel; each repository still executes its steps sequentially.

### Workflow logging

`gix workflow` emits a single header per repository (`-- owner/repo (/path) --`) followed by YAML step summaries so automation can parse results easily. Example:

```
-- tyemirov/scheduler (/tmp/repos/scheduler) --
- stepName: convert-protocol
  outcome: applied
  reason: 'ssh'
- stepName: switch-branch
  outcome: applied
  reason: 'master'
```

Other commands keep the existing human-readable console logs and suppress workflow-internal noise such as `TASK_PLAN`/`TASK_APPLY`.

### Built-in workflow commands

- `remote update-protocol`
 - with: `from: <git|ssh|https>`, `to: <git|ssh|https>` (required, must differ)
- `remote update-to-canonical`
 - with: `owner: <slug>` (optional owner constraint)
- `folder rename`
 - with: `require_clean: <bool>`, `include_owner: <bool>`
 - CLI `--require-clean` provides a default when not specified.
- `default`
 - with: `targets: [{ remote_name, source_branch, target_branch, push_to_remote, delete_source_branch }]`
 - Defaults: `remote_name: origin`, `target_branch: master`, `push_to_remote: true`, `delete_source_branch: false`; `source_branch` auto-detected from remote/local if omitted.
- `audit report`
 - with: `output: <path>` (optional). When provided, writes a CSV file; otherwise prints to stdout.
- `tasks apply`
 - with: `tasks: [...]` (see below) for fine-grained file changes, commits, PRs, and built-in actions.

### Example: Canonicalize after owner rename

This example updates remotes to canonical, renames folders to include owners, switches branch to `master` only when the worktree is clean, and rewrites Go module namespaces from `github.com/temirov` to `github.com/tyemirov`, creating a branch and pushing changes.

```yaml
workflow:
 - step:
   name: remotes
   command: ["remote", "update-to-canonical"]

 - step:
   name: folders
   after: ["remotes"]
   command: ["folder", "rename"]
   with:
    include_owner: true
    require_clean: false

 - step:
   name: protocol-to-git-https
   after: ["folders"]
   command: ["remote", "update-protocol"]
   with:
    from: https
    to: git

 - step:
   name: protocol-to-git-ssh
   after: ["folders"]
   command: ["remote", "update-protocol"]
   with:
    from: ssh
    to: git

 - step:
   name: switch-branch
   after: ["protocol-to-git-https", "protocol-to-git-ssh"]
   command: ["tasks", "apply"]
   with:
    tasks:
     - name: "Switch to master if clean"
      actions:
       - type: branch.change
        options:
         branch: master
         remote: origin
         create_if_missing: false
      safeguards:
       hard_stop:
        require_clean: true

 - step:
   name: namespace-branch
   after: ["switch-branch"]
   command: ["tasks", "apply"]
   with:
    tasks:
     - name: "Create namespace branch"
      actions:
       - type: branch.change
        options:
         branch: "automation/ns-rewrite/{{ .Repository.Name }}-{{ index .Environment \"workflow_run_id\" }}"
         remote: origin
         create_if_missing: true
      safeguards:
       hard_stop:
        require_clean: true

 - step:
   name: namespace-rewrite
   after: ["namespace-branch"]
   command: ["tasks", "apply"]
   with:
    tasks:
     - name: "Rewrite module namespace"
      steps:
       - files.apply
      files:
       - path: go.mod
        mode: replace
        replacements:
         - from: github.com/temirov
           to: github.com/tyemirov
       - path: go.sum
        mode: replace
        replacements:
         - from: github.com/temirov
           to: github.com/tyemirov
       - path: "**/*.go"
        mode: replace
        replacements:
         - from: github.com/temirov
           to: github.com/tyemirov

 - step:
   name: namespace-stage-commit
   after: ["namespace-rewrite"]
   command: ["git", "stage-commit"]
   with:
    paths:
     - "."
    commit_message: "refactor: rewrite module namespace after owner rename"

 - step:
   name: namespace-push
   after: ["namespace-stage-commit"]
   command: ["git", "push"]
   with:
    branch: "automation/ns-rewrite/{{ .Repository.Name }}-{{ index .Environment \"workflow_run_id\" }}"
    push_remote: origin

 - step:
   name: namespace-open-pr
   after: ["namespace-push"]
   command: ["pull-request", "open"]
   with:
    branch: "automation/ns-rewrite/{{ .Repository.Name }}-{{ index .Environment \"workflow_run_id\" }}"
    title: "refactor({{ .Repository.Name }}): rewrite module namespace"
    body: |
      Rewrites Go module imports from `github.com/temirov` to `github.com/tyemirov` after the owner rename.
    base: "{{ .Repository.DefaultBranch }}"
    push_remote: origin
```

Notes:

- The namespace rewrite step commits and pushes changes when `push: true` is set.
- Generating the commit message via LLM inside a workflow is not yet supported. You can either supply a static `commit_message` (as above) or generate one per repository using `gix message commit` before running the workflow. See ISSUES.md for the improvement request to support LLM in workflows and piping outputs between steps.

### Apply tasks (custom sequences)

The `apply-tasks` operation lets you define repository-local tasks with optional templating and safeguards.

Schema highlights:

- Task: `{ name, ensure_clean, branch, files[], actions[], commit, pull_request, safeguards }`
- Branch: `{ name, start_point, push_remote }` where `name`/`start_point` are Go text/templates rendered with repository data; default `push_remote: origin`.
- Files: `{ path, content, mode: overwrite|skip-if-exists|append-if-missing|replace, permissions, replacements }` with templated `path`/`content`.
  - `mode: overwrite` rewrites the entire file.
  - `mode: skip-if-exists` leaves existing files untouched.
  - `mode: append-if-missing` preserves existing content and appends each missing line from `content`, making it ideal for `.gitignore`-style enforcement.
  - `mode: replace` rewrites matching substrings using `replacements: [{ from, to }]` (templated). File paths accept glob patterns, including recursive `**/*.ext`, so you can update many files with one entry.
- Actions: `{ type, options }` where `type` is one of:
 - `repo.remote.update`, `repo.remote.convert-protocol`, `repo.folder.rename`, `branch.default`, `repo.release.tag`, `audit.report`, `repo.history.purge`, `repo.files.replace`, `repo.namespace.rewrite`
- LLM: optional `{ model, base_url, api_key_env, timeout_seconds, max_completion_tokens, temperature }` block. When present, commit/changelog actions reuse the configured client instead of requiring a programmatic injector.
- Commit: `{ message }` (templated). Defaults to `Apply task <name>` when empty.
- Pull request: `{ title, body, base, draft }` (templated; optional).
- Safeguards: `{ hard_stop: {...}, soft_skip: {...} }` blocks that control whether a violation aborts the repository (`hard_stop`) or just skips the current task/action (`soft_skip`). Legacy flat maps are treated as `hard_stop`.
- Steps: optional ordered list (`branch.prepare`, `files.apply`, `git.stage`, `git.commit`, `git.push`, `pull-request.create`, `actions`) that restricts which internal actions run. When omitted, file-backed tasks run the entire branch/commit/push pipeline by default.
- Execution steps are now explicit actions: `git.branch.prepare` (creates the work branch), `files.apply`, `git.stage`, `git.commit`, `git.push`, and `pull-request.create`. Each action evaluates its own safeguards so workflows fail fast with actionable errors (for example, dirty worktrees or missing remotes).

Example task-only workflow step:

```yaml
- step:
  name: apply-task
  command: ["repo", "tasks", "apply"]
  with:
   tasks:
    - name: "Bump license header"
     ensure_clean: true
     branch:
      name: "chore/{{ .Repository.Name }}/license"
     files:
      - path: "LICENSE"
       content: "Copyright (c) {{ .Repository.Owner }}"
       mode: overwrite
     commit:
      message: "chore: update license"
     safeguards:
      hard_stop:
       require_clean:
        enabled: true
      soft_skip:
       branch_in: [master]
       paths: [".git"]
```

Templating supports Go text/template with `.Task.*`, `.Repository.*`, and `.Environment` fields. Available repository fields include: `Path`, `Owner`, `Name`, `FullName`, `DefaultBranch`, `PathDepth`, `InitialClean`, `HasNestedRepositories`. Capture outputs from LLM actions with `capture_as: <variable>` and reference them in later tasks or workflow steps using `{{ index .Environment "variable" }}`.

### Safeguards

Safeguards gate tasks (and are also used internally by some actions). Supported keys:

- `require_clean.enabled: <bool>` — skip when the worktree is dirty (defaults to true when `require_clean` is declared).
- `require_clean.ignore_dirty_paths: [".DS_Store", ".env.*", "bin/"]` — optional glob/prefix list applied only when `require_clean` is enabled; useful for workflows that add matching entries to `.gitignore`.
- `capture: { name: "<name>", value: branch|commit, overwrite: <bool> }` — record the current branch or HEAD commit into a workflow variable so later steps can restore it. Captured values are also available under `.Environment["Captured.<name>"]` for templating, and `overwrite` defaults to false to preserve the first recorded value during a workflow run.
- `restore: { from: "<name>", value: branch|commit }` — jump back to a previously captured branch/commit. Validation fails if the capture name is missing, and `value` defaults to the original capture kind when omitted.
- `branch: <name>` — require current branch to match exactly.
- `branch_in: [<name>...]` — require current branch to be one of the listed values.
- `paths: [<relative/path>...]` — require listed paths to exist in the repository.

### Execution model and defaults

- Steps form a DAG: `after` defines dependencies; independent steps run in parallel stages; omitted `after` implies sequential chaining.
- `` prints plans and skips mutations; confirmations respect `--yes`.
- `--require-clean` sets the default `require_clean` for rename operations when not specified in `with`.
- Repository discovery honors `--roots` and ignores nested repositories by default; certain operations may enable nested processing when appropriate.

## Shared command options

- `--roots <path>` — target one or more directories; nested repositories are ignored automatically.
- `` — print the proposed actions without mutating anything.
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

- `gix audit [--roots <dir>...] [--all] [-y]` (alias `a`)

 - Flags: `--roots` (repeatable), `--all` to include non-git folders in output.

- `gix workflow <configuration> [--roots <dir>...] [--require-clean] [-y]` (alias `w`)

 - Runs tasks from a YAML/JSON workflow file.
 - Flags: `--require-clean` sets the default safeguard for operations that support it.

- `gix folder rename [--owner] [--require-clean] [--roots <dir>...] [-y]`
 - Renames repository directories to canonical GitHub names. Flags: `--owner` includes the owner segment; `--require-clean` enforces clean worktrees.
- `gix remote update-to-canonical [--owner <slug>] [--roots <dir>...] [-y]` (alias `canonical`)
 - Updates `origin` URLs to the canonical GitHub repository; optional `--owner` constraint.
- `gix remote update-protocol --from <git|ssh|https> --to <git|ssh|https> [--roots <dir>...] [-y]` (alias `convert`)
 - Converts remote protocols in bulk.
- `gix prs delete [--limit <N>] [--remote <name>] [--roots <dir>...] [-y]` (alias `purge`)
 - Deletes branches whose pull requests are closed. Flags: `--limit`, `--remote`.
- `gix packages delete [--package <name>] [--roots <dir>...] [-y]` (alias `prune`)
 - Removes untagged GHCR versions. Flag: `--package` for the container name.
- `gix files replace --find <string> [--replace <string>] [--pattern <glob>...] [--command "<shell>"] [--require-clean] [--branch <name>] [--require-path <rel>...] [--roots <dir>...] [-y]` (alias `sub`)
 - Performs text substitutions across matched files with optional safeguards.
- `gix files add --template <path> [--content <text>] [--mode overwrite|skip-if-exists|append-if-missing] [--branch <template>] [--remote <name>] [--commit-message <text>] [--roots <dir>...] [-y]` (alias `seed`)
 - Seeds or updates files across repositories, creating branches and pushes when configured.
- `gix workflow license --var template=LICENSE --var license_branch=chore/license --roots <dir>... [-y]`
 - Runs the embedded license preset; see “License preset variables” for supported options.
- `gix workflow namespace --var namespace_old=... --var namespace_new=... [--roots <dir>...] [-y]`
 - Runs the embedded namespace rewrite preset; see “Namespace preset variables” for supported options.
- `gix files rm <path>... [--remote <name>] [--push] [--restore] [--push-missing] [--roots <dir>...] [-y]` (alias `purge`)
 - Purges paths from history using git-filter-repo and optionally force-pushes updates.
- `gix release <tag> [--message <text>] [--remote <name>] [--roots <dir>...] [-y]` (alias `rel`)
 - Creates and pushes an annotated tag for each repository root.
- `gix release retag --map <tag=ref> [--map <tag=ref>...] [--message-template <text>] [--remote <name>] [--roots <dir>...] [-y]` (alias `fix`)
 - Reassigns existing release tags to provided commits and force-pushes updates.
- `gix message changelog [--version <v>] [--release-date YYYY-MM-DD] [--since-tag <ref>] [--since-date <ts>] [--max-tokens <N>] [--temperature <0-2>] [--model <id>] [--base-url <url>] [--api-key-env <NAME>] [--timeout-seconds <N>] [--roots <dir>...]` (aliases `section`)
 - Generates a changelog section from git history using the configured LLM.
- `gix message commit [--diff-source staged|worktree] [--max-tokens <N>] [--temperature <0-2>] [--model <id>] [--base-url <url>] [--api-key-env <NAME>] [--timeout-seconds <N>] [--roots <dir>...]` (alias `msg`)
 - Drafts Conventional Commit subjects and optional bullets using the configured LLM.
- `gix default <target-branch> [--roots <dir>...] [-y]`
 - Promotes the default branch across repositories.
- `gix cd [branch] [--remote <name>] [--stash | --commit] [--require-clean] [--roots <dir>...]` (alias `switch`)
 - Switches repositories to the selected branch when provided, or the repository default when omitted. Creates the branch if missing, rebases onto the remote, and, when `--stash` or `--commit` are enabled, performs an additional refresh cycle that fetches/pulls with stash/commit-based recovery.
## Configuration essentials

- `gix --init LOCAL` writes an embeddable starter `config.yaml` to the current directory; `gix --init user` places it under `$XDG_CONFIG_HOME/gix` or `$HOME/.gix`.
- Configuration precedence is: CLI flags → environment variables prefixed with `GIX_` → local config → user config.
- Default settings include log level, log format, behaviour, confirmation prompts, and reusable workflow definitions.

## Need more depth?

- Detailed architecture, package layout, and command wiring: [ARCHITECTURE.md](ARCHITECTURE.md)
- Historical roadmap and design notes: [docs/cli_design.md](docs/cli_design.md)
- Recent changes: [CHANGELOG.md](CHANGELOG.md)

## Prerequisites

- Go 1.25 or newer (matching the version pinned in CI).
- Git 2.40+ (history rewrite features rely on modern plumbing commands).
- [`git-filter-repo`](https://github.com/newren/git-filter-repo) installed on your `PATH`. It is required for `gix files rm` and for running the repository integration tests locally (`pip install git-filter-repo` on Linux/macOS, or `brew install git-filter-repo` when using Homebrew).

## Developer notes

- Repository services accept domain types from `internal/repos/shared` (paths, owners, remotes, branches); CLI edges construct them so executors run without defensive validation.
- Executor errors surface via the contextual catalog in `internal/repos/errors`, which prints `PLAN-*`, `*-DONE`, and `*-SKIP` banners through the shared reporter.
- Confirmation prompts respect the `[a/N/y]` contract everywhere (uppercase `N` remains the default decline); passing `--yes` (or setting `assume_yes: true` in workflows) flips the shared confirmation policy to auto-accept, and selecting `a`/`all` at a prompt upgrades the remainder of the run to behave as if `--yes` had been provided (uppercase responses continue to work as well).
- Run `make ci` before submitting patches; it enforces formatting plus `go vet`, `staticcheck`, `ineffassign`, and the unit/integration test suites. At minimum, run `go run honnef.co/go/tools/cmd/staticcheck@master ./...` so lint blocks (SA1006, etc.) surface before you commit.
    - `mode: append-if-missing` preserves existing content and appends each missing line from `content`, making it ideal for `.gitignore` enforcement.
