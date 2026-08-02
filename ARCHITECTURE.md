# gix Architecture

## Overview

gix is a Go 1.25 command-line application built with Cobra and a strict YAML configuration loader. The binary exposed by `main.go` delegates all setup to `cmd/cli`, which wires logging, configuration, and command registration before executing user-facing operations. Domain logic lives in `internal` packages, each focused on a cohesive maintenance capability. Shared libraries that may be reused by external programs are published under `pkg/`.

```
.
├── main.go          # binary entrypoint
├── cmd/cli          # Cobra application, command registration, configuration bootstrap
├── internal         # feature domains (audit, repos, branches, etc.)
├── pkg              # reusable libraries (task runner adapter)
├── docs             # design notes and developer references
└── tests            # behavior-driven integration tests
```

## Execution Flow

1. The binary entrypoint (`main.go`) invokes `cli.Execute`, which builds the Cobra root command through `cmd/cli/application_bootstrap.go` (composition/flags) and `cmd/cli/application_commands.go` (command registration) after loading config from `cmd/cli/application_config.go`.
2. `cmd/cli` discovers one canonical `config.yml`, expands its `${NAME}` placeholders through `internal/utils/configuration_loader.go`, decodes the exact schema, and prepares a structured Zap logger.
3. Each namespace (`audit`, `repo`, `branch`, `commit`, `workflow`, etc.) registers subcommands via `registerCommands`, reusing shared task-runner wiring provided by `pkg/taskrunner`.
4. Domain services resolve their collaborators through `internal/repos/dependencies`, which supplies defaults for repository discovery, filesystem access, Git execution, and GitHub metadata unless tests inject fakes.
5. Commands perform work through `internal/...` packages (for example, `internal/repos/rename.Run`), returning contextual errors that bubble back to Cobra for consistent exit handling.

## Workflow Execution Outcomes

Workflow orchestration (`internal/workflow`) now splits planning, runner orchestration, and reporting and returns an `ExecutionOutcome` to every caller. The outcome captures:

- Temporal data (start, end, duration) plus the number of repositories inspected.
- Stage outcomes that list the operations executed in each DAG stage and their elapsed time. Stages are also recorded via the structured reporter’s `RecordStageDuration` API so summary data includes per-stage timings.
- Operation outcomes/failures, which the CLI surfaces as needed while still emitting the structured reporter summary to stderr.
- Snapshot of reporter summary data (`shared.SummaryData`) so automation layers (e.g., `pkg/taskrunner`, CLI commands, integration tests) can make decisions without re-parsing logs.

CLI builders run their workflows through `pkg/taskrunner`, which adapts the outcome: commands other than `gix workflow` drop the metrics, while the `workflow` command prints a stage-by-stage summary (duration and operation list) after the reporter writes its structured log. The summary data now also includes per-step outcome counts derived from `WORKFLOW_STEP_SUMMARY` events.

## Strict-Sync Conflict Resolution

`internal/branches/syncflow/merge_conflict_resolution.go` owns the operation-scoped merge transaction after Git reports unmerged paths. Marker-bearing files are first rendered through Git's diff3 conflict form, then parsed into ordered non-conflicting regions and explicit BASE/OURS/THEIRS conflict regions. Non-conflicting bytes remain local throughout resolution.

Before strict sync starts its own transaction, `strict_sync_preflight.go` lists the registered topology and resolves each live worktree's common Git directory. A worktree that already resolves to another live common repository is foreign-owned, so sync rejects it without rewriting either repository. Only a linked checkout whose canonical `.git` target is missing is passed explicitly to `git worktree repair`; strict sync then re-lists and revalidates the repaired topology. This reconnects existing linked checkouts after the primary repository moves without letting a copied primary repository take over a live sibling that still belongs to the original. A missing checkout remains prunable, and repair never removes a live directory. Ownership, repair, and validation failures stop with worktree and repository context. Preflight then resolves the exact per-worktree paths for `MERGE_HEAD`, `REVERT_HEAD`, `CHERRY_PICK_HEAD`, `rebase-merge`, `rebase-apply`, `BISECT_START`, and `sequencer`. Absence is the only idle administrative state. Present commit files must contain canonical commit identifiers, administrative directories must have the expected filesystem kind, and path, read, or validation failures stop sync. The same preflight inspects each index for unmerged entries, catching operator-owned conflicts such as a failed stash apply even when no administrative marker exists. This exact-path contract ignores ordinary branches or tags with marker-like names while detecting operator-owned merge, revert, cherry-pick, rebase, apply-mailbox, bisect, multi-command sequencer, and unmerged-index state in both the caller and adoptable siblings. After ownership-aware registration repair, preflight finishes before fetch, LLM access, or content, index, ref, checkout, or topology mutation and returns explicit recovery guidance.

Merged-review handoff is derived from forge state rather than local stack metadata alone. After the open pull-request lookup is empty, sync reads the selected branch's merged pull request without assuming its base, then follows each merged parent to its actual base. A surviving branch is considered merged only when the pull request's recorded head OID matches the fetched remote tip and the local branch has no local-only commits; a reused or advanced head remains active review work instead of inheriting historical merged state. When neither local nor remote ref survives, the immutable pull-request head OID remains the historical identity for continued traversal. An active open parent takes precedence over historical merged records; otherwise traversal continues even when matching merged remote refs still exist. The configured base or first parent with no later matching merged pull request becomes the single prompted handoff target. A visited-branch set rejects review-base cycles before checkout mutation, and dirty branches perform the merged-state lookup before commit generation.

The immutable preflight plan identifies the caller and target sibling that may need snapshots. `strict_sync_transaction.go` captures each touched checkout and commit, plus a transaction-owned Git stash that preserves the exact index, tracked contents, and untracked contents whenever the worktree is dirty. Snapshot acquisition becomes transaction-owned only after Git returns the backup object; a failure before that point finalizes earlier temporary snapshots without resetting the unowned worktree. The transaction then journals only refs and worktrees each mutating Git command owns, advancing the expected ref value after successful commands. Sibling adoption commits locally but defers its push to the normal target publication path. Before publication, a cancellation-independent bounded rollback clears gix-owned conflict/index state, compare-and-swaps only journaled refs from their expected value to the starting value, recreates only adopted worktrees, reapplies mutated snapshots with `--index`, and removes only the exact transaction stashes. Unrelated refs and worktrees are never restoration targets; an unexpected change to an owned ref stops cleanup rather than overwriting external work. Cleanup reports `SYNC_SWITCH_ROLLBACK`; failure of that restoration retains recovery objects under `SYNC_SWITCH_HANDOFF`.

Clustered dirty commits add a narrower checkpoint around every slow LLM request. After staging, `dirty_sync.go` compares the complete cached path set with the selected cluster, then records the active branch or detached checkout, `HEAD`, exact per-worktree index path, cache entries and their skip-worktree or assume-unchanged flags, intent-to-add state, and resolve-undo records. Every post-model inspection runs through a cancellation-independent bounded context. The final inspection begins only after Gix acquires the canonical per-worktree `index.lock`, so a normal Git writer either mutates first and is detected by the recheck or loses the lock and cannot enter the commit. Gix copies the validated live index into that locked path and invokes `git commit` with the copy as `GIT_INDEX_FILE`; Git creates the commit from the checked state while the live index remains untouched. Checkout or semantic index drift marks local ownership as lost before commit; the transaction skips reset/clean/rollback, preserves its recovery snapshot and the outside writer's current state, releases its lock, and emits one actionable `SYNC_SWITCH_HANDOFF`.

Strict-sync pushes request Git's porcelain status. Only a status that reports an actual remote ref creation, update, or deletion marks the Git publication boundary; an up-to-date push leaves the transaction rollback-capable. A malformed successful response fails closed under the published handoff contract because Gix cannot prove whether a remote write occurred. Successful creation of a previously missing pull request also marks publication. A later failure does not claim to roll back a proven or possible remote write or review request: the transaction preserves the published checkout and invocation-owned recovery stash, removes internal snapshots when safe, emits `SYNC_SWITCH_HANDOFF`, and withholds `SYNCED`. `strict_sync_stash.go` restores an explicit `--stash` by exact object identifier with `--index`; an unmerged apply enters the same deterministic and bounded semantic resolver with commit completion disabled. Only a validated apply, exact stash drop, and transaction finalization permit success reporting.

Resolution is a ranked fidelity ladder. Byte-identical sides and unilateral changes resolve directly because they contain no two-sided semantic choice; marker-free conflicts retain their exact current-stage decision. Every marker-bearing region changed by both sides requires semantic LLM audit. Empty-BASE concurrent insertions start from an exact OURS-then-THEIRS candidate, including for `.mprlab/ISSUES.md` and root `CHANGELOG.md`. `merge_conflict_strategy.go` tokenizes other unresolved regions without normalizing their bytes, derives BASE-to-side edits with a bounded longest-common-subsequence matrix, and combines only non-overlapping edit spans into a candidate. Local strategies construct high-fidelity proposals; they do not make the final semantic decision.

Every two-sided marker-bearing region reaches the model, but untouched file content does not. Each candidate is region-scoped and sentinel-wrapped so transport trimming cannot change boundary whitespace. Local validation requires the non-whitespace replacement intent derived from both BASE-to-OURS and BASE-to-THEIRS edits. A candidate then enters a distinct semantic-auditor request; an approval accepts it, while a corrected candidate must pass local validation and another audit. A genuinely overlapping region without a safe local candidate starts with model generation under the same validation contract. Rejection details feed the next attempt. Each of four bounded attempts gives every credentialed provider one configured request timeout, avoiding transport retries that would consume another provider's budget.

Only after every reconstructed file is staged, Git reports no unmerged paths, and `git diff --cached --check` succeeds does Gix create the merge commit and push. Validation or request failures reach rollback only after the semantic ladder is exhausted. Caller cancellation and unrecoverable Git/filesystem failures enter the same operation-owned final recovery boundary immediately because no further safe candidate can be produced. The merge abort and enclosing local-state restoration run from bounded cleanup contexts before publication; post-publication failures preserve forward recovery state instead of claiming a remote rollback.

## Workflow Task Operations

Declarative repository tasks are layered across dedicated modules inside `internal/workflow`:

- `task_parser.go` converts YAML/JSON options into strongly typed `TaskDefinition` values (files, actions, commit metadata, pull request templates, safeguards, and optional LLM configuration).
- `task_plan.go` renders templates against repository + environment data, plans file writes/actions, and records the resulting DAG as `taskPlan` instances that report intent through the structured reporter.
- `task_execute.go` applies the plan: it evaluates safeguards, guards against dirty worktrees (respecting per-task overrides), manages branch checkout/push lifecycles, executes task actions, and emits structured events only (no `fmt.Fprintf` calls).
- `task_operation.go` stitches the lifecycle together so the workflow executor can run the parsed tasks across every discovered repository.

This separation keeps parsing/templating logic pure, isolates Git/GitHub side effects, and guarantees that every user-facing log flows through `shared.StructuredReporter`, which now also records per-stage durations for telemetry and CLI summaries. Audit-mode consumers (for example `gix audit`) set `Dependencies.DisableWorkflowLogging` so the executor instantiates reporters that write to `io.Discard`, preserving the selected raw audit report output for integration tests.

### Workflow Runtime Variables

`gix workflow` (and embedded presets such as `license` or `namespace`) accept runtime variables via `--var key=value`, `--var-file path.yaml`, and configuration defaults. The CLI normalizes keys, merges them (configuration → var-files → CLI flags), and passes the resulting map through `workflow.RuntimeOptions`. The executor seeds those variables into `Environment.Variables` before any task plans execute, marking them as user-provided so downstream actions (for example, LLM `capture_as`) cannot overwrite them. Captured values can still populate new keys or override previously captured ones, but seeded entries always win, ensuring preset templates always honor operator-specified values.

## Command Surface

The Cobra application (split across `cmd/cli/application_bootstrap.go`, `cmd/cli/application_commands.go`, and `cmd/cli/application_config.go`) initialises the root command and nests feature namespaces below it (`audit`, `repo`, `branch`, `commit`, `workflow`, and others). Each namespace hosts subcommands that ultimately depend on injected services from `internal/...` packages. Commands share common flag parsing helpers (`internal/utils/flags`), prompt utilities, and the central dependency builder from `pkg/taskrunner`.

- `cmd/cli/repos` registers multi-command groups such as `folder rename`, `remote update-to-canonical`, `prs delete`, and `files rm/replace/add` (history purge plus file seeding).
- `cmd/cli/repos/release` contains the `release` tagging workflow.
- `cmd/cli/changelog`, `cmd/cli/commit`, and `cmd/cli/workflow` expose focused entrypoints for changelog generation, AI-assisted commit messaging, and workflow execution.
- `cmd/cli/default_configuration.go` houses the embedded default YAML used by `gix init`.
- `cmd/cli/workflow/presets` embeds reusable workflows (for example, license distribution and namespace rewrite) so direct commands and the `workflow` command share identical task graphs.
- `internal/licenses` owns the canonical embedded license bundles. The reviewed fleet boundary lives in `configs/licensing/fleet.json`, while `scripts/licensing/license_rollout.py` verifies live GitHub drift, resolves each default branch to one immutable commit, pins every isolated sparse clone to that inspected commit, and validates any deterministic rollout draft against the same base, one canonical commit, and the exact rendered bundle before `configs/license-rollout.yaml` creates or skips pull requests.

All commands accept shared flags for log level, log format, previews, repository roots, and confirmation prompts. Validation occurs in Cobra `PreRunE` functions, aligning with the confident-programming rules in `.mprlab/POLICY.md`.

## Domain Packages

Each feature area resides in `internal/<domain>` and exposes structs with methods instead of package-level functions. The primary packages are:

- `internal/audit`: Repository discovery, metadata reconciliation, terminal-width-responsive table reporting (with Unicode-aware truncation and a field/value layout when a grid cannot fit), CSV/HTML full-value export, and CLI integration (`internal/audit/cli`).
- `internal/branches`: Branch maintenance commands (`sync`, `refresh`, default promotion) and supporting adapters.
- `internal/changelog`, `internal/commitmsg`: Generators that transform Git history and staged changes into formatted text.
- `internal/repos`: Subpackages for repository workflows:
  - `dependencies`: Dependency resolution for discovery, filesystem, Git, and GitHub integrations.
  - `discovery`: Filesystem scanning for Git repositories.
  - `filesystem`: Filesystem abstractions used by rename/history flows.
  - `history`: Wrapper around git-filter-repo operations for `rm`.
  - `prompt`: End-user confirmation and message formatting.
  - `protocol`, `remotes`, `rename`: Operations that update remotes, protocols, and directory names.
  - `shared`: Shared interfaces (Git executor, GitHub resolver, repository manager).
- `internal/packages`: GitHub Packages retention workflow including GHCR API clients.
- `internal/releases`: Annotated tag creation and push orchestration used by `release`.
- `internal/web`: Embedded local browser UI, repository explorer, typed audit API, and queued remediation boundary.
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

Configuration is decoded from exactly one `config.yml`. The search order is:

1. Explicit `--config` path, if provided.
2. `/etc/gix/config.yml`.
3. `$HOME/.gix/config.yml`.

When neither discovered file exists, gix offers to create `$HOME/.gix/config.yml`. `gix init` performs that user-level initialization directly; `gix init --system` writes `/etc/gix/config.yml`. Gix does not discover working-directory files, merge layers, accept `.yaml` aliases, or bind `GIX_*` environment overrides.

The loader parses the selected file once, then expands `${NAME}` placeholders in YAML value scalars from the process environment inherited when gix starts. Substituted text remains literal scalar content even when it contains YAML-significant quotes, backslashes, newlines, colons, or hash characters. The loader never searches for or parses `.env` files; literal configuration values pass through unchanged. The top-level `llm` block defines complete `openai` and `llm_proxy` connection profiles shared by message, changelog, sync, workflow-task, and web helpers. Each profile owns its routing fields, endpoint, credential, and positive unique priority. Lower priority numbers run first, request failures continue to the next credentialed connection, and the first successful response wins. An empty interpolated credential disables that connection, but at least one connection must remain active. Operation-specific selections can override `llm_proxy.provider` and `llm_proxy.model`; endpoints, credentials, and connection priority stay in the top-level profiles. Logging relies on Uber's Zap; format is configurable (structured JSON or console) through a flag or configuration.

The `workflow` command is special-cased: without a positional configuration, it executes the typed top-level `workflow` block produced by that single application-config decode rather than reopening the selected file. It uses a YAML formatter that emits machine-friendly step summaries (one per repository) and prints a final end-of-run summary line. Non-workflow commands continue to use the existing human-readable console logging format.

## Local Web Workspace

`gix --web` is an explicitly local browser surface. `cmd/cli` validates the bind/port flags, assembles the repository catalog, and injects the typed audit collaborators; `internal/web` owns the embedded HTTP server, static UI, and JSON boundary. The default bind is `127.0.0.1:8080`. Supplying a non-loopback bind deliberately makes the same mutating surface reachable over the network, so deployments must keep it inside a trusted boundary.

The web server exposes the repository catalog and folder browser along with `POST /api/audit/inspect` and `POST /api/audit/apply`. Inspection accepts explicit roots and returns typed rows, including explicit origin-remote status; the browser never reconstructs audit state from command stdout. The repository tree presents selectable top-level repositories and folders, while the typed audit workspace is independently scoped to the roots the operator selects.

Audit remediations are represented as typed queued changes rather than argv text. Canonical-remote updates, protocol conversion, sync, rename, changelog, and commit actions reuse owned application/workflow primitives. The web-only `delete_folder` action requires an absolute path, an explicit `confirm_delete` value, and cannot target a filesystem root. Queue conflicts are deterministic: a repeated kind/path replaces its earlier item, deletion is exclusive for a path, successful changes leave the queue, and skipped or failed changes remain visible for operator review. After apply, the browser re-inspects the last audited roots so the table reflects the operation’s real scope. The user-facing details are maintained in [docs/web-audit-workspace.md](docs/web-audit-workspace.md).

## Workflow configuration example

The example below matches the configuration used in the documentation tests. It demonstrates how CLI defaults and workflow steps can share anchored maps so one file drives both direct commands and declarative workflows.

```yaml
# config.yml
common:
  log_level: error
  log_format: structured

github:
  credential: "${GH_TOKEN}"

llm:
  openai:
    priority: 2
    model: gpt-4.1
    base_url: "https://api.openai.com/v1"
    credential: "${OPENAI_API_KEY}"
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy-api.mprlab.com"
    credential: "${LLM_PROXY_SECRET_KEY}"
  max_completion_tokens: 1200
  temperature: 0
  timeout_seconds: 60

operations:
  - command: ["audit"]
    with: &audit_defaults
      roots:
        - ~/Development
      debug: false

  - command: ["packages", "delete"]
    with: &packages_retention_defaults
      # Retention is explicit at invocation time: gix packages delete --keep 3
      # package: my-image  # Optional override; defaults to the repository name
      base_url: "https://api.github.com"
      credential: "${GITHUB_PACKAGES_TOKEN}"
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

  - command: ["default"]
    with: &branch_default_defaults
      debug: false
      roots:
        - ~/Development

workflow:
  - step:
      command: ["remote", "update-protocol"]
      with:
        <<: *repo_protocol_defaults

  - step:
      command: ["remote", "update-to-canonical"]
      with:
        <<: *repo_remotes_defaults

  - step:
      command: ["folder", "rename"]
      with:
        <<: *repo_rename_defaults

  - step:
      command: ["default"]
      with:
        <<: *branch_default_defaults
        targets:
          - remote_name: origin
            target_branch: master
            push_to_remote: true
            delete_source_branch: false

  - step:
      command: ["audit", "report"]
      with:
        output: ./reports/audit-latest.csv
        format: csv
```

Package retention is supplied at the command boundary, for example `gix packages delete --keep 3`. Gix first snapshots and validates every package-version page, then preserves the newest requested count by `created_at` and deletes all older tagged or untagged versions.

## Reusable Packages

`github.com/tyemirov/utils/llm` contains reusable client abstractions for LLM-backed features such as commit message and changelog generators. The package exposes an interface-based design so that other programs can reuse the same client without duplicating API plumbing. `pkg/taskrunner` hosts the shared workflow dependency resolver and task-runner adapter used by CLI commands and tests to consistently wire Git, GitHub, filesystem, and confirmation collaborators.

## Testing Strategy

Domain packages rely on table-driven unit tests using injected fakes for Git, GitHub, and filesystem interactions. Integration coverage lives under `tests/`, where high-level flows execute through the public CLI surfaces to ensure behavior matches the documented commands. All tests are designed to run in isolated temporary directories (`t.TempDir`) without polluting the developer filesystem.

Documentation tests in `docs/readme_config_test.go` ensure the workflow configuration referenced above stays in sync with the executable configuration loader.
