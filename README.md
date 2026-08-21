# gix, a Git/GitHub helper CLI

[![GitHub release](https://img.shields.io/github/release/tyemirov/gix.svg)](https://github.com/tyemirov/gix/releases)

gix makes branch-targeted Git synchronization mechanical: explicit branch targets receive pending commits, work branches are remote-backed and PR-backed, and local state is synchronized without rebasing or force-pushing.

## Highlights

- Commit pending work to an explicitly named branch, resume a PR branch, or create a new PR branch with one command.
- Merge the remote default branch into work branches and push the result without rewriting shared history.
- Save dirty work automatically by clustering changed paths, drafting commit messages through the configured LLM client, and pushing through the PR flow.
- Reuse discovery, prompting, and logging whether you call a single command or an entire workflow file.

## Quick Start

1. Install the CLI: `go install github.com/tyemirov/gix@latest` (Go 1.25+).
2. Create the canonical user configuration: `gix init`.
3. Either replace the generated credential placeholders with literal values in `$HOME/.gix/config.yml`, or export `GH_TOKEN`, `GITHUB_PACKAGES_TOKEN`, and `LLM_PROXY_SECRET_KEY` before launching gix. Gix interpolates only its inherited process environment and never loads `.env` files.
4. Attach or verify a workspace: `gix sync https://github.com/OWNER/REPO.git`.
5. Synchronize the repository default branch with `gix sync <default-branch>`.
6. With dirty work ready to commit, start a PR branch: `gix sync feature/my-change`; use the same command later to resume it.
7. Sync the current branch later with plain `gix sync`.

## Release, Publish, Deploy

```bash
make release
make publish
make deploy
```

These three targets accept no release flags, arguments, or lifecycle overrides. The repository release helper supplies its release policy when it calls `gix release next`. The command validates that invocation policy and selects the next version without reading MPR Lab repository files.

For an established SemVer sequence, the command uses an LLM to examine all committed changes after the latest SemVer tag. The evidence includes commit messages, the diff summary, range-scoped changelog changes, and a bounded diff excerpt. The model classifies each packet by its effect on a supported public contract. A second model call audits each candidate against the same evidence. Standard SemVer maps incompatible, additive, and compatible effects to `major`, `minor`, and `patch`. Commit labels and implementation changes cannot set a higher release level by themselves.

Invalid or unavailable model output stops the release before version selection. A SemVer repository without tags starts at `v1.0.0`. A CalVer repository uses the canonical UTC release timestamp. At an exact release tag, `make release` is the idempotent retry command.

An artifact producer can supply different previous and candidate release output identities for the same tagged source commit. Gix binds both SHA-256 identities into deterministic evidence and selects the next patch version without an LLM request. An ordinary SemVer decision with an empty commit range remains invalid.

The Gix release helper invokes `gix release next semver --fixed-major 1`. Under this policy, incompatible and additive Gix public contract changes select a minor release. Compatible fixes and internal changes select a patch release. Version selection uses only `v1` tags. Other callers can select standard SemVer or CalVer explicitly.

`make release` runs CI. It prepares the binaries, checksums, Pages archive, release metadata commit, one annotated tag, and release manifest. The local `.git/mprlab-release` directory contains the sealed receipt. New release preparation does not write to a remote repository.

At an exact release tag, the command verifies and reuses the complete local receipt without another CI run. If the receipt is incomplete, the command reconstructs it from the matching GitHub Release. Reconstruction verifies the manifest, notes, hashes, annotated tag, source parent, and exact release metadata files.

New release preparation uses a separate candidate receipt. A preparation failure preserves the prior receipt and rolls back the transaction-owned commit and tags. `make publish` pushes the exact prepared Git refs and GitHub Release assets through canonical `origin`. `make deploy` activates the published Pages archive only when the downloaded manifest matches the prepared release. The CLI has no runtime rollout.

The release manifest records two revisions and one version. `source_commit` identifies the source that builds the Pages archive. `release_commit` identifies the release metadata commit and its tag. The decision, tag, manifest, binary, and GitHub Release use the same version. Pages deployment verifies the public archive marker against `source_commit` and the published tag against `release_commit`.

Pages remains configured through GitHub's legacy branch publishing contract at `gh-pages:/`; the repository does not own a Pages Actions workflow. Deployment reconciles that configuration only when it is missing or different, then follows the GitHub Pages build for the exact deployed branch commit. A changed branch or configuration is the build trigger. An unchanged retry reuses a built, queued, or building record and requests one rebuild only when the matching build is absent or terminally errored. Public marker verification begins after that build succeeds, and failures report the Pages build status, error, commit, and URL.

These maintainer targets use the repository-owned helpers under `scripts/release`. They require Bash 4+, Python 3.10+, GNU `timeout`, `rsync`, `tar`, `shasum`, `curl`, and an authenticated GitHub CLI in addition to the normal Go and Git prerequisites.

## The sync flow

`gix sync` has four forms:

| Command | Meaning |
| --- | --- |
| `gix sync <remote-url>` | Clone into an empty directory or verify the current workspace already points at that remote. |
| `gix sync <default-branch>` | Clean tree: restore the local default branch from its remote. Dirty tree: commit, merge the remote ref, and push directly. |
| `gix sync <branch>` | Use the named branch as the target. Switch to it and commit dirty work, or create its pull-request stack. |
| `gix sync` | Sync the current branch. A dirty current default branch without an explicit target keeps the generated PR rescue flow. |

An explicit branch target controls sync. With dirty work, sync carries the pending files to that branch when a switch is required. Sync clusters the changed paths and commits each cluster on the named branch. If the target is the repository default branch, sync merges its remote ref and pushes the branch directly. Plain `gix sync` remains the current-branch form. It creates a generated rescue branch when the implicitly resolved current branch is the dirty default branch.

Tracked files remain authoritative dirty work even when their paths match `.gitignore`. Sync stages those exact tracked paths, while ordinary untracked files continue through Git's normal ignore-respecting behavior; sync never restores a tracked path merely because an ignore rule also matches it.

Strict sync first lists registered worktrees and validates that every live checkout resolves to the caller's common Git directory. It repairs only linked checkouts whose canonical `.git` target is missing, passing those exact paths to Git and then re-listing and revalidating the topology. This reconnects a checkout after its primary repository moves, while a checkout that still belongs to another live common repository is rejected without mutation; copying a primary repository cannot take over the original repository's sibling. Missing Git-prunable registrations remain on the established prune path. Ownership, repair, and validation failures stop with worktree and repository context. Strict sync then builds one preflight plan and refuses to begin while any valid registered worktree contains an operator-owned merge, revert, cherry-pick, rebase, apply-mailbox, bisect, sequencer operation, or unmerged index. It resolves the exact per-worktree Git administrative paths (`MERGE_HEAD`, `REVERT_HEAD`, `CHERRY_PICK_HEAD`, `rebase-merge`, `rebase-apply`, `BISECT_START`, and `sequencer`) instead of resolving ambiguous revisions, so ordinary branches or tags with those names do not impersonate operation state. Present commit markers must contain canonical commit identifiers, administrative directories must have the expected kind, and every inspection failure stops sync. An unmerged index is rejected even when no administrative marker remains, as with a conflicted `git stash apply`. Rejection occurs before fetch, stash, checkout/worktree content changes, index or ref mutation, LLM dispatch, commit, or push and reports the matching explicit recovery action.

When an explicit branch does not exist, sync requires dirty work for the new branch commits. Sync creates the branch at the current branch's `HEAD`. If the current branch is not the repository default branch, sync first publishes its committed `HEAD`. Sync preserves an existing open pull request. If the pull request is missing, sync opens it against its recorded review base. If no parent is recorded, sync uses the repository default branch. After the parent pull request exists, sync creates the child branch and commits each changed-path cluster. The configured `github.com/tyemirov/utils/llm` client supplies each Conventional Commit message. Sync aligns the child with `origin/<parent-branch>`, pushes it, and opens a pull request against the parent branch.

Sync rejects clean or `--stash` creation of a missing branch. Such a child has no committed delta against its parent. Before child creation, sync records the selected parent in local `branch.<child>.gix-review-base` Git config. A retry after push or pull-request failure uses the same review base. Deeper stacks retain each recorded link. An existing remote-backed branch with no pull request remains publishable review work. Sync rejects current merged state, saves dirty work, and opens the missing pull request through the same base-delta path. A historical merged record is current only when its head OID matches the surviving branch tip. The local branch must also have no local-only commits. Reuse or advancement opens a new pull request instead of the old handoff. Once the child pull request merges, its actual merged base drives the normal handoff. Sync follows every matching merged parent pull request regardless of whether its remote branch ref remains. It stops at the first active branch or at the repository default branch. Sync uses one standard handoff prompt for that terminal branch. Uncommitted work on a known-merged branch is rejected before commit. Rerun with `--stash` to carry that work through the merged handoff. Then create its new review branch from the surviving base. `--stash` remains available when syncing an existing branch. `--commit` explicitly selects the default dirty auto-commit policy. `--require-clean` requires a clean worktree. Explicit `--title` and `--body` values apply to the requested child. An automatically opened parent uses its default title and diff-generated body.

When the target branch is held by a linked worktree, sync first prunes stale registration and preserves sibling changes before retrying the switch. Sibling adoption may commit locally to release the checkout, but it does not publish from the sibling; an actual remote ref update reported by the normal target-branch push or successful pull-request creation is the publication boundary. Before its first local mutation, the strict-sync transaction snapshots the caller and target sibling checkout, commit, index, tracked contents, untracked contents, stash list, and topology. It journals only branch refs and worktrees the invocation mutates. A pre-publication failure compare-and-swaps those owned refs back to their starting commits, restores the exact files and staged/unstaged distinction, and recreates only adopted topology; unrelated branch advances and worktrees remain untouched. An up-to-date push performs no remote write and therefore remains rollback-capable until another publication occurs. A failure after publication cannot undo the remote write: Gix preserves the published checkout and any invocation-owned recovery stash, emits `SYNC_SWITCH_HANDOFF`, and never reports `SYNCED`.

Dirty-cluster commit-message requests are also ownership boundaries. Immediately after staging one cluster, Gix verifies that the complete staged path set belongs to that cluster and checkpoints the active checkout, `HEAD`, exact per-worktree index path, cache entries, skip-worktree and assume-unchanged flags, intent-to-add state, and resolve-undo records. Every post-model ownership inspection uses a cancellation-independent bounded context. For the final inspection, Gix first acquires the worktree's canonical `index.lock`, rechecks the checkpoint while normal Git index writers are excluded, copies the validated index into the private locked file, and commits from that copy through `GIT_INDEX_FILE`; the live index is never replaced by the commit. A writer that wins before the lock is detected as drift, while one that arrives after the lock cannot stage into the commit. Either ownership loss stops before commit or push without reset, clean, or restoration across outside state, retains the transaction snapshot, emits one `SYNC_SWITCH_HANDOFF`, and directs the operator to stop the other writer before retrying.

`--stash` is invocation-owned. Gix reapplies it with `--index`, resolves a conflicted apply through the same bounded semantic conflict engine, validates the resulting index, and drops only that exact stash. `SYNCED` is emitted only after restoration and transaction-snapshot cleanup succeed. A failed pre-publication restoration returns to the original caller state; a failed post-publication restoration retains the stash and conflicted recovery state under the explicit handoff contract.

For marker-bearing merge conflicts, Gix renders Git's diff3 form, parses each conflict region, and reconstructs the complete file locally from byte-exact non-conflicting regions. It directly accepts only cases with no two-sided semantic choice: byte-identical sides or a change on only one side. Marker-free modify/delete or rename/delete conflicts preserve the exact current-stage decision, including deletion. Every marker-bearing region changed by both sides requires semantic LLM audit. Empty-BASE concurrent insertions—including `.mprlab/ISSUES.md` and root `CHANGELOG.md` entries—start with an exact OURS-then-THEIRS candidate. Compatible token edits start with a byte-preserving local merge candidate. Conflicting replacements start with the local alternative plus all compatible incoming edits. Same-position insertions use a deterministic order and preserve both insertions. Gix sends every derived candidate directly to semantic audit. These deterministic strategies protect fidelity, but they do not replace the model's semantic decision.

Each audit sends only the conflict's BASE, OURS, THEIRS, and candidate. It never sends the complete file or untouched regions. The model can approve the candidate or return a semantic correction. When no safe local candidate exists, the model generates one from the three conflict regions. Every returned candidate must use the required content envelope. Hard validation rejects conflict markers, exact BASE content, and incomplete concurrent insertions. Local token validation proves exact replacement intent when possible. A locally valid audit correction completes immediately. If replacement-intent proof is unavailable, Gix retains the structurally valid correction as the exact candidate for the next repair request. An approval cannot accept that candidate. Only a later locally valid correction completes it. Responses that fail hard validation supply exact feedback for the next bounded attempt. Before commit, Gix also requires no unmerged paths and validates each staged path against both merge parents. A path passes when Git reports no new whitespace error relative to one parent. Whitespace errors that are new to both parents cause rollback. Rollback starts after four semantic attempts fail, the caller cancels, or an unrecoverable Git or filesystem error occurs. A bounded merge abort restores the target's pre-merge state. Before publication, the transaction then restores the complete caller and target-sibling snapshot. After publication, recovery remains forward-only and preserves the published state under an explicit handoff. The PR branch is "ours". Its remote review base is "theirs".

## Other maintenance workflows

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
gix packages delete --keep 3 --roots ~/Development/containers --yes
```

Keep the three newest GitHub Container Registry versions for each discovered package and delete every older version, whether tagged or untagged.

### Inspect repositories or export an audit report

```shell
gix audit --roots ~/Development --all
```

The default terminal table captures default branches, owners, remotes, protocol mismatches, and worktree state for every repository in scope. It reads the active terminal width: a horizontal grid is used when it fits, while narrower terminals use a field/value table so no audit column disappears. Constrained cells use a Unicode-aware `…` instead of wrapping the table. When a terminal-size query is unavailable, `COLUMNS` supplies the table width for captured output. The same responsive renderer is used by table-format `audit report` workflow steps that write to stdout. CSV and HTML retain every full value for automation or review:

```shell
gix audit --roots ~/Development --all --format csv > audit.csv
gix audit --roots ~/Development --all --format html > audit.html
```

### Use the local audit workspace

```shell
gix --web --roots ~/Development
```

`gix --web` starts a local browser workspace on `127.0.0.1:8080` by default. It includes a repository explorer and a typed audit table for operator-selected roots; it does not parse terminal output to construct audit results. Remediation actions are queued for review and editing before they run, then the workspace re-inspects the exact audited scope. The web-only folder-deletion action requires an explicit confirmation in that queue. Keep the default loopback bind for local use; a non-loopback `--bind` exposes a mutating local tool to the network. See [the web audit workspace guide](docs/web-audit-workspace.md) for the action, queue, and safety contract.

### Draft commit messages and changelog entries

```shell
gix message commit --roots .
gix message changelog --since-tag v1.2.0 --version v1.3.0
```

Use the reusable LLM client (`github.com/tyemirov/utils/llm`) to summarize staged changes or recent history. `gix sync` uses the configured provider order only for genuinely overlapping strict-sync regions. For semantic resolution, `timeout_seconds` is the request budget for each provider. Each candidate or audit request can use one complete provider round. A provider round with no response stops semantic repair and starts rollback. A transport or authentication error never becomes model feedback. Only a returned candidate rejection can start the next of four bounded attempts. Gix reports the active region, strategy, attempt, and deadline.

The generated configuration defaults to Meta Muse through MPR LLM Proxy and declares both available connections:

```yaml
llm:
  openai:
    priority: 2
    model: gpt-5.6-terra
    base_url: "https://api.openai.com/v1"
    credential: "${OPENAI_API_KEY}"
    effort: "high"
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy-api.mprlab.com"
    credential: "${LLM_PROXY_SECRET_KEY}"
  max_completion_tokens: 4800
  effort: "high"
  timeout_seconds: 60
```

Each connection owns its routing data. Direct OpenAI owns its `model`; `llm_proxy` owns the upstream `provider` and `model`. Both connections require a positive, unique `priority`, and the lower number is attempted first. If that request fails, gix tries the next connection and returns the first successful response. Completion-token budgets resolve from the command configuration first, then the selected connection profile, then the top-level `llm.max_completion_tokens`. The top-level value is required and must be positive. The generated configuration declares one 4,800-token global default for both reasoning connections; provider and command values remain optional explicit overrides.

Direct OpenAI treats an exhausted sequence of empty completions as reasoning-budget exhaustion, not as a usable response. It preserves the resolved request and performs one additional bounded retry cycle. This recovery is limited to the typed empty-response condition. Authentication, HTTP, transport, and cancellation failures keep their normal behavior. Configure an explicit direct OpenAI completion budget when it is the backup for large semantic regions.

If every configured connection fails, gix reports each attempted connection by name with its contextual error. LLM Proxy HTTP failures preserve a typed status and stable proxy code. They do not expose the raw response body. Joined failures remain available to programmatic callers through standard Go error traversal.

`llm_proxy.provider` is required. `llm_proxy.model` is optional and uses the selected provider's server-side default when omitted; `openai.model` defaults to `gpt-5.6-terra` when omitted. A connection whose interpolated `credential` is empty is excluded, and at least one connection must have a credential. `--provider` and `--model` override the llm-proxy upstream for one invocation; they do not change connection priority. Endpoints and credentials are configuration-only and have no CLI or late environment-variable-name override.

## Automate sequences with workflows

When you need several operations in one pass, describe them in YAML or JSON and execute them with the workflow runner:

```shell
gix workflow maintenance.yml --roots ~/Development --yes
```

Workflows reuse repository discovery, confirmation prompts, and logging so you can hand teammates a repeatable playbook.

### Workflow output

`gix workflow` emits an initial discovery summary for each repository, then YAML step summaries and a final summary line at the end of the run. The summary includes event counters, per-step outcomes (`STEP_<STEP>_<OUTCOME>`), WARN/ERROR counts, and a human duration. Other commands keep the existing human-readable console logs.

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
- `pull-request create` — open a PR without touching remotes.
- Every workflow automatically exposes `.Environment.workflow_run_id` (UTC `YYYYMMDDHHMMSS`) so you can build unique branch names like `automation/{{ .Repository.Name }}-{{ index .Environment "workflow_run_id" }}` without passing extra variables.

Combine these steps to build fully custom git flows without relying on one monolithic `tasks apply`. See `configs/gitignore.yaml` for a concrete example that splits branch creation, file editing, staging, commit, push, and PR creation into discrete workflow steps.

### Workflow variables

Use runtime variables to parameterize presets or external configs:

```shell
gix workflow license --var license_template=mit --var license_branch=chore/license --roots ~/Development --yes
gix workflow namespace --var namespace_old=github.com/old/org --var namespace_new=github.com/new/org --roots ~/Research
```

- `--var key=value` sets a single variable (repeat the flag for multiple values).
- `--var-file path/to/file.yaml` loads variables from a YAML/JSON map.

Variables appear inside task templates via `{{ index .Environment "key" }}` and merge with captured values (`capture_as`), with runtime inputs taking precedence.

#### License preset variables

`gix workflow license` recognizes the following keys (pass via `--var` or `--var-file`):

| Variable | Description |
| --- | --- |
| `license_template` | Embedded template name (`bsl`, `mit`, `polyform-noncommercial`, or `proprietary`). When set, `license_content` is derived. |
| `license_content` | License text (required when no template is set). |
| `license_target` | Relative path for the output file (defaults to `LICENSE`). |
| `license_commercial_target` | Optional commercial-notice path for templates that include one (defaults to `COMMERCIAL_LICENSE.md`). |
| `license_mode` | File handling mode (`overwrite`, `skip-if-exists`, or `append-if-missing`). |
| `license_year` | MIT, PolyForm-notice, or proprietary year override (defaults to the current year). |
| `license_author` | MIT author override (defaults to the repository owner). |
| `license_company` | Proprietary company override (defaults to the repository owner). |
| `license_licensor` | BSL or PolyForm licensor override (defaults to the repository owner). |
| `license_contact` | Commercial-license contact (defaults to `legal@mprlab.com` for templates that use it). |
| `license_project_name` | BSL project name override (defaults to the repository name). |
| `license_change_date` | BSL change date (defaults to `2029-01-01`). |
| `license_change_license` | BSL change license (defaults to `Apache License 2.0`). |
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

Use the `gix workflow license` preset with the canonical `license_template`
variable (`bsl`, `mit`, `polyform-noncommercial`, or `proprietary`) or
`license_content` plus the `license_*` overrides to distribute license content.
The old `template` variable and `gix repo-license-apply` wrapper have been
removed.

The reviewed personal and MPR Lab fleet policy, legal holds, frozen inventory,
and one-command draft-PR handoff are documented in
[`docs/licensing-rollout.md`](docs/licensing-rollout.md). Run
`make license-rollout-plan` for the read-only drift check. After review,
`make license-rollout-apply` creates the eligible draft pull requests.
The plan resolves each default branch to one commit, reads its license blobs
from that revision, and pins apply clones to the same commit even if the
remote branch advances before the clone starts.
Apply accepts an already-open deterministic draft only after its base commit,
single rollout commit, complete changed-file set, and rendered license blobs
match the reviewed plan and a final pull-request snapshot remains unchanged.

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
      target_branch: release
      push_to_remote: true
      delete_source_branch: false

 - step:
   name: audit
   after: ["default-branch"]
   command: ["audit", "report"]
   with:
    output: ./reports/audit.csv
    format: csv
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
 - `target_branch` is required. Defaults: `remote_name: origin`, `push_to_remote: true`, and `delete_source_branch: false`. The command detects `source_branch` from remote or local data when you omit it.
- `audit report`
 - with: `output: <path>` (optional), `format: <table|csv|html>` (optional; defaults to `csv`).
- `tasks apply`
 - with: `tasks: [...]` (see below) for fine-grained file changes, commits, PRs, and built-in actions.
- `command run`
 - with: `command: <string|list>` (required), `working_directory: <path>` (optional), `ensure_clean: <bool>` (optional), `safeguards: <map>` (optional)

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
       - type: branch.sync
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
       - type: branch.sync
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
- Generating the commit message via LLM inside a workflow is not yet supported. You can either supply a static `commit_message` (as above) or generate one per repository using `gix message commit` before running the workflow. See `.mprlab/ISSUES.md` for the improvement request to support LLM in workflows and piping outputs between steps.

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
- LLM: optional `{ llm_proxy: { provider, model }, timeout_seconds, max_completion_tokens, effort }` block. When the block is present, `llm_proxy.provider` is required and `llm_proxy.model` is optional. The nested selection overrides only the configured llm-proxy upstream; connection endpoints, credentials, and priority cannot be declared inside workflow tasks.
- Commit: `{ message }` (templated). Defaults to `Apply task <name>` when empty.
- Pull request: `{ title, body, base, draft }` (templated; optional).
- Safeguards: `{ hard_stop: {...}, soft_skip: {...} }` blocks that control whether a violation aborts the repository (`hard_stop`) or just skips the current task/action (`soft_skip`).
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
- `--config path/to/config.yml` — load one explicit canonical configuration instead of performing system/user discovery.
- `--log-level`, `--log-format` — control Zap logging output (structured JSON or console).

Additional shared flags:

- `--remote <name>` — override the remote name used by commands that push or fetch (default `origin`).
- `--version` — print the gix version (works at the root or with any command).
- `--web` — launch the local browser UI on `127.0.0.1:8080` by default.
- `--bind <host>`, `--port <port>` — override the web bind address or port when used with `--web`.
- `--roots <dir>...` — when used with `--web`, scope the initial repository tree to the provided roots.

## Command Reference

Top-level commands and their subcommands. Aliases are shown in parentheses.

- `gix version`

 - Prints the current release. Also available as `gix --version`.

- `gix init [--system] [--force]`

 - Writes the canonical configuration to `$HOME/.gix/config.yml`, or to `/etc/gix/config.yml` with `--system`. Use `--force` to replace an existing generated config.

- `gix --web [--bind <host>] [--port <port>] [--roots <dir>...]`

 - Starts a local HTTP server on `127.0.0.1:8080` by default and serves the embedded browser UI plus JSON API for running gix commands in-process.
 - Use `--bind` and `--port` to expose the UI on a different interface or port, for example `gix --web --bind 0.0.0.0 --port 8081`.
 - Use `--roots` to pre-scope the initial left-pane repository catalog, for example `gix --web --roots ~/Development/fleet`.
 - The UI exposes the command catalog, accepts one argument per line, and captures stdout/stderr for each run. Its audit workspace uses typed inspection rows and a review-before-apply remediation queue; [the web audit workspace guide](docs/web-audit-workspace.md) defines its actions and deletion confirmation.

- `gix audit [--roots <dir>...] [--all] [--format <table|csv|html>] [-y]` (alias `a`)

 - Flags: `--roots` (repeatable), `--all` to include non-git folders in output, `--format` to select `table` (default), `csv`, or `html`.

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
- `gix packages delete --keep <count> [--package <name>] [--roots <dir>...] [-y]` (alias `prune`)
 - Preserves the newest positive `--keep` count by `created_at` and deletes every older tagged or untagged GHCR version. Equal timestamps are ordered by version ID, newest first. Flag: `--package` optionally overrides the container name.
- `gix files replace --find <string> [--replace <string>] [--pattern <glob>...] [--command "<shell>"] [--require-clean] [--branch <name>] [--require-path <rel>...] [--roots <dir>...] [-y]` (alias `sub`)
 - Performs text substitutions across matched files with optional safeguards.
- `gix files add --template <path> [--content <text>] [--mode overwrite|skip-if-exists|append-if-missing] [--branch <template>] [--remote <name>] [--commit-message <text>] [--roots <dir>...] [-y]` (alias `seed`)
 - Seeds or updates files across repositories, creating branches and pushes when configured.
- `gix workflow license --var license_template=mit --var license_branch=chore/license --roots <dir>... [-y]`
 - Runs the embedded license preset; see “License preset variables” for supported options.
- `gix workflow namespace --var namespace_old=... --var namespace_new=... [--roots <dir>...] [-y]`
 - Runs the embedded namespace rewrite preset; see “Namespace preset variables” for supported options.
- `gix files rm <path>... [--remote <name>] [--push] [--restore] [--push-missing] [--roots <dir>...] [-y]` (alias `purge`)
 - Purges paths from history using git-filter-repo and optionally force-pushes updates.
- `gix release <tag> [--message <text>] [--remote <name>] [--roots <dir>...] [-y]` (alias `rel`)
 - Creates and pushes an annotated tag for each repository root.
- `gix release next <semver|calver> [--fixed-major <major>] [--format text|json] [--release-timestamp <RFC3339>] [--exclude-tag <tag>...] [--previous-release-output <sha256>] [--candidate-release-output <sha256>]`
 - Selects the next canonical version from the explicit invocation policy. The release output flags require SemVer, different SHA-256 identities, and the latest tag at `HEAD`. `--fixed-major` applies only to SemVer. `--release-timestamp` applies only to CalVer. JSON output records the complete applied policy in `mprlab.version-decision/v2`.
- `gix release retag --map <tag=ref> [--map <tag=ref>...] [--message-template <text>] [--remote <name>] [--roots <dir>...] [-y]` (alias `fix`)
 - Reassigns existing release tags to provided commits and force-pushes updates.
- `gix message changelog [--version <v>] [--release-date YYYY-MM-DD] [--since-tag <ref>] [--since-date <ts>] [--max-tokens <N>] [--effort <low|medium|high>] [--provider <provider>] [--model <id>] [--timeout-seconds <N>] [--roots <dir>...]` (aliases `section`)
 - Generates a changelog section from git history using the configured LLM.
- `gix message commit [--diff-source staged|worktree] [--max-tokens <N>] [--effort <low|medium|high>] [--provider <provider>] [--model <id>] [--timeout-seconds <N>] [--roots <dir>...]` (alias `msg`)
 - Drafts Conventional Commit subjects and optional bullets using the configured LLM.
- `gix default <target-branch> [--roots <dir>...] [-y]`
 - Promotes the default branch across repositories. Gix closes a pull request only when its head repository and head branch match the target repository and branch. Gix changes the base of other pull requests. Gix fetches the remote source and target before it evaluates deletion safety. The delete request includes the verified source commit. Git rejects deletion if the source changes. After all safety gates pass, Gix deletes the local and remote source branches. Gix retains a source branch that contains changes absent from the target branch. The result reports both `safe_to_delete` and `source_deleted`.
- `gix sync [remote-url|branch] [--remote <name>] [--title <text>] [--body <markdown>] [--stash | --commit] [--require-clean] [--roots <dir>...]` (alias `switch`)
 - Synchronizes the current workspace through the Gix flow. An explicit branch is the dirty-commit target. If that target is the repository default branch, sync merges its remote ref and pushes directly. Existing pull-request branches sync against their current pull-request base. Merged branches follow their merged parents to the first active branch or repository default branch. A dirty missing target starts at the current `HEAD`. If the current branch is not the default branch, sync publishes it before the child pull request. Clean or `--stash` creation of a missing branch is rejected because it has no child review delta. Dirty work is clustered, described, committed, and pushed by default. Known-merged branches require a stashed handoff before new review work is created. Plain `gix sync` on a dirty current default branch keeps the generated pull-request rescue flow. Sync validates linked-worktree ownership and rejects operator-owned Git operations before mutation. Before publication, failures restore the exact local state. After publication, failures retain forward recovery state. Sync never rebases or force-pushes. Pull-request body text comes from the branch diff unless an explicit body is configured. The title defaults to the branch unless an explicit title is configured. `--stash` restores the exact index before success. `--commit` selects the auto-commit policy. `--require-clean` requires a clean worktree when no dirty-work policy is selected.
## Configuration essentials

- On every launch, gix uses an explicit `--config <path>.yml` when supplied; otherwise it checks `/etc/gix/config.yml` and then `$HOME/.gix/config.yml`.
- If neither discovered file exists, gix offers to create `$HOME/.gix/config.yml`. `--yes` accepts that prompt non-interactively.
- Gix decodes `config.yml` once into the current typed YAML schema and rejects unknown fields. It does not pass a generic YAML map through a second schema decoder.
- `gix init` writes `$HOME/.gix/config.yml`; `gix init --system` writes `/etc/gix/config.yml`. Add `--force` only when you intentionally want to replace an existing generated config.
- Working-directory config files, `.yaml` aliases, `GIX_*` overrides, embedded runtime defaults, and layered configuration merging are not supported.
- `${NAME}` placeholders in YAML values are expanded only from the process environment inherited when gix starts. Substituted text remains literal scalar content, including quotes, backslashes, newlines, colons, and hash characters. Gix never discovers or loads `.env` files; users of dotenv tooling must load those values into the process environment before launching gix.
- Literal values in `config.yml`, including literal credentials, are used as written.
- `github.credential` supplies the concrete token injected into GitHub CLI calls. The `packages delete` operation similarly owns its concrete `base_url` and `credential`; neither integration performs a later environment lookup. Retention is intentionally invocation-owned: every `packages delete` call must provide a positive `--keep` value.
- The `openai` and `llm_proxy` connections store their own routing fields, positive unique `priority`, concrete `base_url`, and interpolated `credential` values in `config.yml`. Lower priority numbers run first; failed requests continue to the next credentialed connection.
- A connection with an empty interpolated credential is inactive. At least one connection credential is required.
- The config controls shared behavior such as `log_level`, `log_format`, `assume_yes`, and `require_clean`.
- The top-level `llm` block controls generated commit-message, changelog, sync, workflow-task, and web LLM clients globally. `openai.model` belongs to the direct connection; `llm_proxy.provider` and `llm_proxy.model` belong to the proxy connection.
- Operation defaults can set recurring values for commands, including `roots`, `remote`, sync pull request `title`/`body`, nested `llm_proxy` provider/model overrides, release remotes, audit options, and workflow defaults.
- `gix workflow` without a positional configuration executes the already-decoded top-level `workflow` block from the selected `config.yml`; it does not reopen that file through a second configuration path.

## Need more depth?

- Detailed architecture, package layout, and command wiring: [ARCHITECTURE.md](ARCHITECTURE.md)
- Local browser audit workspace, queued remediation, and safety contract: [docs/web-audit-workspace.md](docs/web-audit-workspace.md)
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
